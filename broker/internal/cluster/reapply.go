package cluster

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"

	"k8s-internal-platform/broker/internal/platform"
)

// PrepareReapply repairs only the managed ClusterIP Service's ports. Changing a
// Service port changes its strategic-merge key, so Helm can otherwise keep the
// old entry and add a second entry with the same name. JSON Patch replaces this
// owned list while preserving the Service UID, allocated addresses and all other
// fields. The operation worker already holds the application mutation lock and
// has committed its frozen execution snapshot before calling this method.
func (c *Client) PrepareReapply(ctx context.Context, appID, manifest string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	var target *corev1.Service
	for {
		var raw map[string]any
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return platform.Errorf(platform.CodeConflict, "recorded reapply manifest is invalid")
		}
		if raw["kind"] != "Service" {
			continue
		}
		if target != nil {
			return platform.Errorf(platform.CodeConflict, "recorded Service ownership is ambiguous")
		}
		body, _ := json.Marshal(raw)
		target = &corev1.Service{}
		if json.Unmarshal(body, target) != nil || target.Namespace != c.namespace || target.Name != releaseFor(appID) || target.Labels[appIDLabel] != appID || target.Spec.Type != corev1.ServiceTypeClusterIP || len(target.Spec.Ports) != 1 {
			return platform.Errorf(platform.CodeConflict, "recorded Service ownership is invalid")
		}
	}
	if target == nil {
		return platform.Errorf(platform.CodeConflict, "recorded Service is unavailable")
	}
	live, err := c.cs.CoreV1().Services(c.namespace).Get(ctx, target.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	} // Helm recreates the missing resource.
	if err != nil {
		return platform.Errorf(platform.CodeDependencyUnavailable, "could not read the owned Service for reapply")
	}
	if !c.owned(live.ObjectMeta, appID) || live.Spec.Type != corev1.ServiceTypeClusterIP {
		return platform.Errorf(platform.CodeConflict, "Service ownership/type does not permit reapply repair")
	}
	if live.ResourceVersion == "" {
		return platform.Errorf(platform.CodeConflict, "Service identity is incomplete; reapply repair refused")
	}
	if equality.Semantic.DeepEqual(live.Spec.Ports, target.Spec.Ports) {
		return nil
	}
	patch, _ := json.Marshal([]map[string]any{
		{"op": "test", "path": "/metadata/uid", "value": string(live.UID)},
		{"op": "test", "path": "/metadata/resourceVersion", "value": live.ResourceVersion},
		{"op": "replace", "path": "/spec/ports", "value": target.Spec.Ports},
	})
	if _, err := c.cs.CoreV1().Services(c.namespace).Patch(ctx, live.Name, types.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
		return platform.Errorf(platform.CodeConflict, "Service changed or could not be repaired; recovery is required")
	}
	return nil
}
