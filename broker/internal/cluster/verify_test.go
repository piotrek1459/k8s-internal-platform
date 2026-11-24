package cluster

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/fake"
)

func verifiedFixture(t *testing.T, lan bool) (*fake.Clientset, string) {
	t.Helper()
	cs, manifest := driftFixture(t, lan)
	d, err := cs.AppsV1().Deployments(ns).Get(context.Background(), "app-app-1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	d.Status = appsv1.DeploymentStatus{ObservedGeneration: d.Generation, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1}
	if _, err := cs.AppsV1().Deployments(ns).Update(context.Background(), d, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	return cs, manifest
}

func TestVerifyManifestRejectsLiveDriftAndUnreadyGeneration(t *testing.T) {
	for _, scenario := range []string{"image", "unobserved", "unready", "extra-env", "env-from", "sidecar", "pull-secret", "restart-token", "service-port", "unexpected-route", "foreign-canonical-route", "extra-owned-route"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			cs, manifest := verifiedFixture(t, false)
			c := NewWithClientset(cs, ns)
			if err := c.VerifyManifest(ctx, manifest); err != nil {
				t.Fatalf("defaulted healthy baseline: %v", err)
			}
			d, _ := cs.AppsV1().Deployments(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			svc, _ := cs.CoreV1().Services(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			switch scenario {
			case "image":
				d.Spec.Template.Spec.Containers[0].Image = "example.com/wrong@sha256:wrong"
			case "unobserved":
				d.Status.ObservedGeneration--
			case "unready":
				d.Status.ReadyReplicas = 0
			case "extra-env":
				d.Spec.Template.Spec.Containers[0].Env = append(d.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "CANDIDATE_ONLY", Value: "removed"})
			case "env-from":
				d.Spec.Template.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "candidate-only"}}}}
			case "sidecar":
				d.Spec.Template.Spec.Containers = append(d.Spec.Template.Spec.Containers, corev1.Container{Name: "unexpected", Image: "example.com/wrong"})
			case "pull-secret":
				d.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "candidate-only"}}
			case "restart-token":
				d.Spec.Template.Annotations = map[string]string{"platform.k8s.io/restart-token": "candidate-only"}
			case "service-port":
				svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Name: "extra", Port: 9090})
			case "unexpected-route", "foreign-canonical-route", "extra-owned-route":
				meta := diagnosticMetadata("app-app-1", "unexpected-route")
				if scenario == "foreign-canonical-route" {
					meta.Annotations["meta.helm.sh/release-name"] = "foreign"
				}
				if scenario == "extra-owned-route" {
					meta.Name = "extra"
				}
				cs.NetworkingV1().Ingresses(ns).Create(ctx, &networkingv1.Ingress{ObjectMeta: meta}, metav1.CreateOptions{})
			}
			cs.AppsV1().Deployments(ns).Update(ctx, d, metav1.UpdateOptions{})
			cs.CoreV1().Services(ns).Update(ctx, svc, metav1.UpdateOptions{})
			if err := c.VerifyManifest(ctx, manifest); err == nil {
				t.Fatal("unverified live state accepted")
			}
		})
	}
}

func TestVerifyManifestRejectsEnvironmentAbsentFromTarget(t *testing.T) {
	cs, manifest := verifiedFixture(t, false)
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	var documents []string
	for {
		var object map[string]any
		err := decoder.Decode(&object)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(object) == 0 {
			continue
		}
		if object["kind"] == "Deployment" {
			template := object["spec"].(map[string]any)["template"].(map[string]any)
			container := template["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)
			delete(container, "env")
		}
		body, _ := json.Marshal(object)
		documents = append(documents, string(body))
	}
	if err := NewWithClientset(cs, ns).VerifyManifest(context.Background(), strings.Join(documents, "\n---\n")); err == nil {
		t.Fatal("a removed environment field was treated as a wildcard")
	}
}

func TestVerifyManifestRejectsTerminatingResources(t *testing.T) {
	for _, kind := range []string{"Deployment", "Service", "Ingress"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			cs, manifest := verifiedFixture(t, kind == "Ingress")
			mark := func(meta *metav1.ObjectMeta) {
				now := metav1.Now()
				meta.DeletionTimestamp = &now
				meta.Finalizers = []string{"fixture.platform.test/hold"}
			}
			switch kind {
			case "Deployment":
				object, _ := cs.AppsV1().Deployments(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
				mark(&object.ObjectMeta)
				cs.AppsV1().Deployments(ns).Update(ctx, object, metav1.UpdateOptions{})
			case "Service":
				object, _ := cs.CoreV1().Services(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
				mark(&object.ObjectMeta)
				cs.CoreV1().Services(ns).Update(ctx, object, metav1.UpdateOptions{})
			case "Ingress":
				object, _ := cs.NetworkingV1().Ingresses(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
				mark(&object.ObjectMeta)
				cs.NetworkingV1().Ingresses(ns).Update(ctx, object, metav1.UpdateOptions{})
			}
			if err := NewWithClientset(cs, ns).VerifyManifest(ctx, manifest); err == nil {
				t.Fatal("matching fields and stale ready status accepted a terminating resource")
			}
		})
	}
}
