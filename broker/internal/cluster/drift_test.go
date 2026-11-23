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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/fake"

	platformhelm "k8s-internal-platform/broker/internal/helm"
	"k8s-internal-platform/broker/internal/platform"
)

func driftFixture(t *testing.T, lan bool) (*fake.Clientset, string) {
	t.Helper()
	adapter, err := platformhelm.NewAdapter(ns, nil)
	if err != nil {
		t.Fatal(err)
	}
	revision := platform.Revision{ID: "rev_1", AppID: "app_1", Spec: platform.RevisionSpec{Image: "example.com/demo:tag", Profile: platform.ProfileHTTP, ContainerPort: 8080, Replicas: 1, Environment: map[string]string{"MODE": "original"}, Health: platform.Health{StartupPath: "/healthz", ReadinessPath: "/readyz"}, Exposure: platform.Exposure{Mode: platform.ExposureInternal}}, Frozen: &platform.FrozenSpec{ImageDigest: "example.com/demo@sha256:" + strings.Repeat("a", 64)}}
	if lan {
		revision.Spec.Exposure = platform.Exposure{Mode: platform.ExposureLAN, Hostname: "demo.home.arpa"}
	}
	frozen, err := adapter.PrepareExecution("app_1", revision)
	if err != nil {
		t.Fatal(err)
	}
	revision.Frozen = &frozen
	manifest, err := adapter.RenderExecution("app_1", revision)
	if err != nil {
		t.Fatal(err)
	}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	objects := []runtime.Object{}
	for {
		var raw map[string]any
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) == 0 {
			continue
		}
		body, _ := json.Marshal(raw)
		var object runtime.Object
		switch raw["kind"] {
		case "Deployment":
			d := &appsv1.Deployment{}
			json.Unmarshal(body, d)
			d.UID = "deployment"
			d.Generation = 3
			defaultDiagnosticProbes(&d.Spec.Template)
			object = d
		case "Service":
			s := &corev1.Service{}
			json.Unmarshal(body, s)
			s.UID = "service"
			s.Spec.ClusterIP = "10.43.1.1"
			object = s
		case "Ingress":
			i := &networkingv1.Ingress{}
			json.Unmarshal(body, i)
			i.UID = "ingress"
			object = i
		default:
			t.Fatal("unexpected chart resource")
		}
		meta := object.(metav1.Object)
		meta.SetAnnotations(map[string]string{"meta.helm.sh/release-name": "app-app-1", "meta.helm.sh/release-namespace": ns})
		// Keep chart-owned route annotations in addition to Helm ownership.
		if route, ok := object.(*networkingv1.Ingress); ok {
			route.Annotations["traefik.ingress.kubernetes.io/router.entrypoints"] = "private"
		}
		objects = append(objects, object)
	}
	return fake.NewSimpleClientset(objects...), manifest
}

func TestDriftComparesIntentWithoutHealthOrAllocatedAddressDefaults(t *testing.T) {
	for _, lan := range []bool{false, true} {
		cs, manifest := driftFixture(t, lan)
		c := NewWithClientset(cs, ns)
		items, err := c.CompareManifest(context.Background(), "app_1", manifest)
		if err != nil || len(items) != 0 {
			t.Fatalf("baseline drift=%+v error=%v", items, err)
		}
		for _, action := range cs.Actions() {
			if action.GetVerb() != "list" && action.GetVerb() != "get" {
				t.Fatal("inspection mutated resources")
			}
		}
	}
}

func TestDriftReportsWorkloadRouteOwnershipAndMissingResourcesWithoutValues(t *testing.T) {
	for _, scenario := range []string{"image", "env", "resources", "replicas", "probe", "service-port", "missing-service", "ownership", "ingress-host", "ingress-entrypoint"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			cs, manifest := driftFixture(t, true)
			c := NewWithClientset(cs, ns)
			d, _ := cs.AppsV1().Deployments(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			svc, _ := cs.CoreV1().Services(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			route, _ := cs.NetworkingV1().Ingresses(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			const private = "fixture-sensitive-drift-value"
			switch scenario {
			case "image":
				d.Spec.Template.Spec.Containers[0].Image = private
			case "env":
				d.Spec.Template.Spec.Containers[0].Env[0].Value = private
			case "resources":
				d.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory] = resource.MustParse("256Mi")
			case "replicas":
				d.Spec.Replicas = ptrInt32(2)
			case "probe":
				d.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Path = "/changed"
			case "ownership":
				d.Annotations["meta.helm.sh/release-name"] = "foreign"
			case "service-port":
				svc.Spec.Ports[0].Port = 81
			case "ingress-host":
				route.Spec.Rules[0].Host = "changed.home.arpa"
			case "ingress-entrypoint":
				route.Annotations["traefik.ingress.kubernetes.io/router.entrypoints"] = "web"
			}
			cs.AppsV1().Deployments(ns).Update(ctx, d, metav1.UpdateOptions{})
			cs.CoreV1().Services(ns).Update(ctx, svc, metav1.UpdateOptions{})
			cs.NetworkingV1().Ingresses(ns).Update(ctx, route, metav1.UpdateOptions{})
			if scenario == "missing-service" {
				cs.CoreV1().Services(ns).Delete(ctx, svc.Name, metav1.DeleteOptions{})
			}
			cs.ClearActions()
			items, err := c.CompareManifest(ctx, "app_1", manifest)
			if err != nil || len(items) == 0 {
				t.Fatalf("drift not reported: %+v %v", items, err)
			}
			body, _ := json.Marshal(items)
			if strings.Contains(string(body), private) || strings.Contains(string(body), "example.com/demo@") {
				t.Fatal("drift returned configuration values")
			}
			for _, action := range cs.Actions() {
				if action.GetVerb() != "list" && action.GetVerb() != "get" {
					t.Fatal("drift was automatically overwritten")
				}
			}
		})
	}
}

func TestDriftReportsUnexpectedOwnedRouteAndRejectsCrossNamespaceManifest(t *testing.T) {
	ctx := context.Background()
	cs, manifest := driftFixture(t, false)
	c := NewWithClientset(cs, ns)
	route := &networkingv1.Ingress{ObjectMeta: diagnosticMetadata("app-app-1", "route")}
	cs.NetworkingV1().Ingresses(ns).Create(ctx, route, metav1.CreateOptions{})
	items, err := c.CompareManifest(ctx, "app_1", manifest)
	if err != nil || len(items) != 1 || items[0].Change != "unexpected" {
		t.Fatalf("unexpected route=%+v %v", items, err)
	}
	if _, err := c.CompareManifest(ctx, "app_1", strings.ReplaceAll(manifest, ns, "kube-system")); err == nil {
		t.Fatal("foreign namespace manifest accepted")
	}
}
