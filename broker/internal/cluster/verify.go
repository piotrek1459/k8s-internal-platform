package cluster

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"k8s-internal-platform/broker/internal/platform"
)

// VerifyManifest checks the workload and routes stored by Helm against live
// objects, including rollout generation, replicas and pod-template content.
func (c *Client) VerifyManifest(ctx context.Context, manifest string) error {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	deployments, services := 0, 0
	mismatch := func(reason string) error {
		return platform.Errorf(platform.CodeConflict, "live release verification failed: %s", reason)
	}
	for {
		var raw map[string]any
		if err := decoder.Decode(&raw); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if len(raw) == 0 {
			continue
		}
		// Decode through JSON to preserve the API's typed default handling.
		body, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		switch raw["kind"] {
		case "Deployment":
			var want appsv1.Deployment
			if err := json.Unmarshal(body, &want); err != nil {
				return err
			}
			if want.Namespace != c.namespace {
				return mismatch("resource identity or count differs")
			}
			got, err := c.cs.AppsV1().Deployments(c.namespace).Get(ctx, want.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			n := int32(1)
			if want.Spec.Replicas != nil {
				n = *want.Spec.Replicas
			}
			if got.Spec.Replicas == nil || *got.Spec.Replicas != n {
				return mismatch("desired replica count differs")
			}
			defaultProbeThresholds(&want.Spec.Template)
			if !equality.Semantic.DeepDerivative(want.Spec.Template, got.Spec.Template) {
				return mismatch("Pod template differs")
			}
			if !equality.Semantic.DeepDerivative(want.Spec.Selector, got.Spec.Selector) {
				return mismatch("Deployment selector differs")
			}
			if got.Status.ObservedGeneration < got.Generation {
				return mismatch("Deployment generation is not observed")
			}
			if got.Status.UpdatedReplicas != n || got.Status.ReadyReplicas != n || got.Status.Replicas != n {
				return mismatch("Deployment replicas are not fully ready")
			}
			deployments++
		case "Service":
			var want corev1.Service
			if err := json.Unmarshal(body, &want); err != nil {
				return err
			}
			if want.Namespace != c.namespace {
				return mismatch("resource identity or count differs")
			}
			got, err := c.cs.CoreV1().Services(c.namespace).Get(ctx, want.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if !equality.Semantic.DeepEqual(want.Spec.Selector, got.Spec.Selector) || !equality.Semantic.DeepDerivative(want.Spec.Ports, got.Spec.Ports) || got.Spec.Type != corev1.ServiceTypeClusterIP {
				return mismatch("Service selector, ports or type differs")
			}
			services++
		case "Ingress":
			var want networkingv1.Ingress
			if err := json.Unmarshal(body, &want); err != nil {
				return err
			}
			if want.Namespace != c.namespace {
				return mismatch("resource identity or count differs")
			}
			got, err := c.cs.NetworkingV1().Ingresses(c.namespace).Get(ctx, want.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if !equality.Semantic.DeepDerivative(want.Spec, got.Spec) {
				return mismatch("resource identity or count differs")
			}
		default:
			return mismatch("resource identity or count differs")
		}
	}
	if deployments != 1 || services != 1 {
		return mismatch("resource identity or count differs")
	}
	return nil
}

// The API defaults an omitted probe successThreshold to 1. Helm's stored
// manifest is not defaulted, so normalize this server-owned default before
// comparing typed Pod templates; readiness, startup and image fields remain checked.
func defaultProbeThresholds(template *corev1.PodTemplateSpec) {
	for i := range template.Spec.Containers {
		container := &template.Spec.Containers[i]
		for _, probe := range []*corev1.Probe{container.StartupProbe, container.ReadinessProbe, container.LivenessProbe} {
			if probe != nil && probe.SuccessThreshold == 0 {
				probe.SuccessThreshold = 1
			}
		}
	}
}
