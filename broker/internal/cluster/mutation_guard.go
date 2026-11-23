package cluster

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"k8s-internal-platform/broker/internal/platform"
)

// GuardMutation checks existing and target manifest resources, including routes
// Helm will remove. It also guards restoration and uninstall: a recovery retry
// cannot bypass an ownership conflict reported by explicit Reapply.
func (c *Client) GuardMutation(ctx context.Context, appID, manifest string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	checked := map[string]bool{}
	for {
		var raw map[string]any
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return platform.Errorf(platform.CodeConflict, "release manifest identity is invalid")
		}
		if len(raw) == 0 {
			continue
		}
		body, _ := json.Marshal(raw)
		var desired struct {
			Metadata metav1.ObjectMeta `json:"metadata"`
		}
		kind, _ := raw["kind"].(string)
		if json.Unmarshal(body, &desired) != nil || desired.Metadata.Namespace != c.namespace || desired.Metadata.Name != releaseFor(appID) || desired.Metadata.Labels[appIDLabel] != appID {
			return platform.Errorf(platform.CodeConflict, "release resource ownership is invalid")
		}
		key := kind + "/" + desired.Metadata.Name
		if checked[key] {
			continue
		}
		checked[key] = true
		var live metav1.Object
		switch kind {
		case "Deployment":
			object, readErr := c.cs.AppsV1().Deployments(c.namespace).Get(ctx, desired.Metadata.Name, metav1.GetOptions{})
			live, err = object, readErr
		case "Service":
			object, readErr := c.cs.CoreV1().Services(c.namespace).Get(ctx, desired.Metadata.Name, metav1.GetOptions{})
			live, err = object, readErr
		case "Ingress":
			object, readErr := c.cs.NetworkingV1().Ingresses(c.namespace).Get(ctx, desired.Metadata.Name, metav1.GetOptions{})
			live, err = object, readErr
		default:
			return platform.Errorf(platform.CodeConflict, "release resource kind is unsupported")
		}
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return platform.Errorf(platform.CodeDependencyUnavailable, "could not verify release resource ownership")
		}
		meta := metav1.ObjectMeta{Name: live.GetName(), Namespace: live.GetNamespace(), UID: live.GetUID(), Labels: live.GetLabels(), Annotations: live.GetAnnotations()}
		if !c.owned(meta, appID) {
			return platform.Errorf(platform.CodeConflict, "release resource ownership changed; mutation refused")
		}
	}
	return nil
}
