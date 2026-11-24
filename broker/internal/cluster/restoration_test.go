package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kt "k8s.io/client-go/testing"
)

func TestRestorationClearsFailedCandidateFieldsAndRoutes(t *testing.T) {
	for _, lan := range []bool{false, true} {
		t.Run(fmt.Sprint("targetLAN=", lan), func(t *testing.T) {
			ctx := context.Background()
			cs, manifest := verifiedFixture(t, lan)
			c := NewWithClientset(cs, ns)
			d, _ := cs.AppsV1().Deployments(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			original := d.DeepCopy()
			d.ResourceVersion = "42"
			d.Spec.Template.Spec.Containers[0].Env = append(d.Spec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "CANDIDATE_ONLY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "candidate-only"}, Key: "value"}}})
			d.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "candidate-only"}}
			d.Spec.Template.Annotations = map[string]string{"platform.k8s.io/restart-token": "candidate-only"}
			cs.AppsV1().Deployments(ns).Update(ctx, d, metav1.UpdateOptions{})
			svc, _ := cs.CoreV1().Services(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			svc.ResourceVersion = "43"
			originalIP, originalUID := svc.Spec.ClusterIP, svc.UID
			svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Name: "extra", Port: 9090})
			cs.CoreV1().Services(ns).Update(ctx, svc, metav1.UpdateOptions{})
			var route *networkingv1.Ingress
			if lan {
				route, _ = cs.NetworkingV1().Ingresses(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			} else {
				other, _ := driftFixture(t, true)
				route, _ = other.NetworkingV1().Ingresses(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
				cs.NetworkingV1().Ingresses(ns).Create(ctx, route, metav1.CreateOptions{})
			}
			route.ResourceVersion = "44"
			route.Spec.Rules[0].Host = "candidate.home.arpa"
			route.Spec.TLS = []networkingv1.IngressTLS{{Hosts: []string{"candidate.home.arpa"}, SecretName: "candidate-only"}}
			route.Annotations["traefik.ingress.kubernetes.io/router.tls"] = "true"
			cs.NetworkingV1().Ingresses(ns).Update(ctx, route, metav1.UpdateOptions{})
			cs.ClearActions()
			if err := c.PrepareRestoration(ctx, "app_1", manifest); err != nil {
				t.Fatal(err)
			}
			got, _ := cs.AppsV1().Deployments(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			if len(got.Spec.Template.Spec.Containers[0].Env) != len(original.Spec.Template.Spec.Containers[0].Env) || len(got.Spec.Template.Spec.ImagePullSecrets) != 0 || got.Spec.Template.Annotations["platform.k8s.io/restart-token"] != "" {
				t.Fatal("failed candidate configuration survived restoration")
			}
			actualSvc, _ := cs.CoreV1().Services(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			if actualSvc.Spec.ClusterIP != originalIP || actualSvc.UID != originalUID || len(actualSvc.Spec.Ports) != 1 {
				t.Fatal("Service identity or restored ports differ")
			}
			actualRoute, err := cs.NetworkingV1().Ingresses(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			if lan {
				if err != nil || actualRoute.Spec.Rules[0].Host != "demo.home.arpa" || len(actualRoute.Spec.TLS) != 0 || actualRoute.Annotations["traefik.ingress.kubernetes.io/router.tls"] != "" {
					t.Fatal("hostname/TLS fields were not restored")
				}
			} else if !apierrors.IsNotFound(err) {
				t.Fatal("newly introduced route was not removed")
			}
			for _, action := range cs.Actions() {
				if action.GetVerb() == "patch" {
					var operations []map[string]any
					if err := json.Unmarshal(action.(kt.PatchAction).GetPatch(), &operations); err != nil {
						t.Fatal(err)
					}
					if operations[0]["op"] != "test" || operations[0]["path"] != "/metadata/uid" || operations[1]["op"] != "test" || operations[1]["path"] != "/metadata/resourceVersion" {
						t.Fatal("repair omitted identity/version preconditions")
					}
				}
				if action.GetVerb() == "delete" {
					p := action.(kt.DeleteAction).GetDeleteOptions().Preconditions
					if p == nil || p.UID == nil || p.ResourceVersion == nil {
						t.Fatal("route deletion omitted identity/version preconditions")
					}
				}
			}
		})
	}
}

func TestRestorationRefusesForeignOrChangedResourcesBeforeRepair(t *testing.T) {
	for _, scenario := range []string{"foreign-route", "missing-version", "selector", "concurrent-patch"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			cs, manifest := verifiedFixture(t, false)
			c := NewWithClientset(cs, ns)
			d, _ := cs.AppsV1().Deployments(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			d.ResourceVersion = "42"
			if scenario == "missing-version" {
				d.ResourceVersion = ""
			}
			if scenario == "selector" {
				d.Spec.Selector.MatchLabels = map[string]string{"foreign": "selector"}
			}
			cs.AppsV1().Deployments(ns).Update(ctx, d, metav1.UpdateOptions{})
			if scenario == "foreign-route" {
				m := diagnosticMetadata("app-app-1", "route")
				m.ResourceVersion = "43"
				m.Annotations["meta.helm.sh/release-name"] = "foreign"
				cs.NetworkingV1().Ingresses(ns).Create(ctx, &networkingv1.Ingress{ObjectMeta: m}, metav1.CreateOptions{})
			}
			if scenario == "concurrent-patch" {
				svc, _ := cs.CoreV1().Services(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
				svc.ResourceVersion = "44"
				cs.CoreV1().Services(ns).Update(ctx, svc, metav1.UpdateOptions{})
				cs.PrependReactor("patch", "deployments", func(kt.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, "app-app-1", fmt.Errorf("concurrent change"))
				})
			}
			cs.ClearActions()
			if err := c.PrepareRestoration(ctx, "app_1", manifest); err == nil {
				t.Fatal("unsafe restoration was accepted")
			}
			if scenario != "concurrent-patch" {
				for _, a := range cs.Actions() {
					if a.GetVerb() != "get" && a.GetVerb() != "list" {
						t.Fatal("repair began before identity checks completed")
					}
				}
			}
		})
	}
}
