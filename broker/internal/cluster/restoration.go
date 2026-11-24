package cluster

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"

	"k8s-internal-platform/broker/internal/platform"
)

// PrepareRestoration repairs only this chart's canonical owned resources. A
// failed candidate may have added configuration absent from Helm's last deployed
// baseline; a three-way patch against that baseline need not remove those fields.
// The frozen target and operation lock already exist before any repair starts.
func (c *Client) PrepareRestoration(ctx context.Context, appID, manifest string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	var deployment *appsv1.Deployment
	var service *corev1.Service
	var route *networkingv1.Ingress
	invalid := func() error {
		return platform.Errorf(platform.CodeConflict, "recorded restoration resource identity is invalid")
	}
	for {
		var raw map[string]any
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return invalid()
		}
		if len(raw) == 0 {
			continue
		}
		body, err := json.Marshal(raw)
		if err != nil {
			return invalid()
		}
		var meta struct {
			Metadata metav1.ObjectMeta `json:"metadata"`
		}
		if json.Unmarshal(body, &meta) != nil || meta.Metadata.Namespace != c.namespace || meta.Metadata.Name != releaseFor(appID) || meta.Metadata.Labels[appIDLabel] != appID {
			return invalid()
		}
		switch raw["kind"] {
		case "Deployment":
			if deployment != nil {
				return invalid()
			}
			deployment = &appsv1.Deployment{}
			if json.Unmarshal(body, deployment) != nil {
				return invalid()
			}
		case "Service":
			if service != nil {
				return invalid()
			}
			service = &corev1.Service{}
			if json.Unmarshal(body, service) != nil {
				return invalid()
			}
		case "Ingress":
			if route != nil {
				return invalid()
			}
			route = &networkingv1.Ingress{}
			if json.Unmarshal(body, route) != nil {
				return invalid()
			}
		default:
			return invalid()
		}
	}
	if deployment == nil || service == nil || len(deployment.Spec.Template.Spec.Containers) != 1 || deployment.Spec.Template.Spec.Containers[0].Name != "application" || deployment.Spec.Template.Labels[appIDLabel] != appID || service.Spec.Type != corev1.ServiceTypeClusterIP || len(service.Spec.Ports) != 1 {
		return invalid()
	}
	readFailure := func() error {
		return platform.Errorf(platform.CodeDependencyUnavailable, "could not read resources for restoration")
	}
	identity := func(meta metav1.ObjectMeta) error {
		if !c.owned(meta, appID) || meta.ResourceVersion == "" {
			return platform.Errorf(platform.CodeConflict, "resource ownership or identity does not permit restoration")
		}
		return nil
	}
	liveDeployment, err := c.cs.AppsV1().Deployments(c.namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		liveDeployment = nil
	} else if err != nil {
		return readFailure()
	} else {
		if err := identity(liveDeployment.ObjectMeta); err != nil {
			return err
		}
		if !equality.Semantic.DeepEqual(deployment.Spec.Selector, liveDeployment.Spec.Selector) {
			return platform.Errorf(platform.CodeConflict, "immutable Deployment selector differs; restoration refused")
		}
	}
	// Check the canonical route even when it is absent from the target manifest.
	// A copied label or foreign/recreated object never grants deletion permission.
	liveRoute, err := c.cs.NetworkingV1().Ingresses(c.namespace).Get(ctx, releaseFor(appID), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		liveRoute = nil
	} else if err != nil {
		return readFailure()
	} else if err := identity(liveRoute.ObjectMeta); err != nil {
		return err
	}
	if err := c.PrepareReapply(ctx, appID, manifest); err != nil {
		return err
	}
	patchFailure := func() error {
		return platform.Errorf(platform.CodeConflict, "resource changed or could not be repaired; restoration remains unverified")
	}
	if liveDeployment != nil {
		n := int32(1)
		if deployment.Spec.Replicas != nil {
			n = *deployment.Spec.Replicas
		}
		patch, _ := json.Marshal([]map[string]any{
			{"op": "test", "path": "/metadata/uid", "value": string(liveDeployment.UID)},
			{"op": "test", "path": "/metadata/resourceVersion", "value": liveDeployment.ResourceVersion},
			{"op": "add", "path": "/spec/template", "value": deployment.Spec.Template},
			{"op": "add", "path": "/spec/replicas", "value": n},
			{"op": "add", "path": "/spec/strategy", "value": deployment.Spec.Strategy},
		})
		if _, err := c.cs.AppsV1().Deployments(c.namespace).Patch(ctx, deployment.Name, types.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
			return patchFailure()
		}
	}
	if liveRoute == nil {
		return nil
	} // Helm creates a missing target route.
	if route == nil {
		uid, version := liveRoute.UID, liveRoute.ResourceVersion
		if err := c.cs.NetworkingV1().Ingresses(c.namespace).Delete(ctx, liveRoute.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &version}}); err != nil {
			return patchFailure()
		}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			_, err := c.cs.NetworkingV1().Ingresses(c.namespace).Get(ctx, liveRoute.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return readFailure()
			}
			select {
			case <-ctx.Done():
				return patchFailure()
			case <-ticker.C:
			}
		}
	}
	annotations := maps.Clone(liveRoute.Annotations)
	for _, key := range []string{"traefik.ingress.kubernetes.io/router.entrypoints", "traefik.ingress.kubernetes.io/router.tls"} {
		if value, ok := route.Annotations[key]; ok {
			annotations[key] = value
		} else {
			delete(annotations, key)
		}
	}
	patch, _ := json.Marshal([]map[string]any{
		{"op": "test", "path": "/metadata/uid", "value": string(liveRoute.UID)},
		{"op": "test", "path": "/metadata/resourceVersion", "value": liveRoute.ResourceVersion},
		{"op": "add", "path": "/spec", "value": route.Spec},
		{"op": "add", "path": "/metadata/annotations", "value": annotations},
	})
	if _, err := c.cs.NetworkingV1().Ingresses(c.namespace).Patch(ctx, route.Name, types.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
		return patchFailure()
	}
	return nil
}
