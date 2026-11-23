package cluster

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"
)

func TestExplicitReapplyRepairsServiceMergeKeyWithIdentityGuards(t *testing.T) {
	for _, scenario := range []string{"repair", "foreign", "concurrent", "missing", "unchanged"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			cs, manifest := driftFixture(t, false)
			c := NewWithClientset(cs, ns)
			service, _ := cs.CoreV1().Services(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
			service.ResourceVersion = "10"
			if scenario != "unchanged" {
				service.Spec.Ports[0].Port = 81
			}
			if scenario == "foreign" {
				service.Annotations["meta.helm.sh/release-name"] = "another-release"
			}
			cs.CoreV1().Services(ns).Update(ctx, service, metav1.UpdateOptions{})
			if scenario == "missing" {
				cs.CoreV1().Services(ns).Delete(ctx, service.Name, metav1.DeleteOptions{})
			}
			// Record and inspect the exact patch before simulating an intervening
			// resource-version change; fake clients do not assign versions themselves.
			var patch []map[string]any
			cs.PrependReactor("patch", "services", func(action ktesting.Action) (bool, runtime.Object, error) {
				json.Unmarshal(action.(ktesting.PatchAction).GetPatch(), &patch)
				if scenario == "concurrent" {
					changed := service.DeepCopy()
					changed.ResourceVersion = "11"
					cs.Tracker().Update(corev1.SchemeGroupVersion.WithResource("services"), changed, ns)
				}
				return false, nil, nil
			})
			cs.ClearActions()
			err := c.PrepareReapply(ctx, "app_1", manifest)
			if scenario == "foreign" || scenario == "concurrent" {
				if err == nil {
					t.Fatal("unsafe reapply repair accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "missing" || scenario == "unchanged" {
				if len(patch) != 0 {
					t.Fatal("unnecessary Service mutation")
				}
				return
			}
			if len(patch) != 3 || patch[0]["path"] != "/metadata/uid" || patch[0]["value"] != "service" || patch[1]["path"] != "/metadata/resourceVersion" || patch[1]["value"] != "10" || patch[2]["path"] != "/spec/ports" {
				t.Fatal("Service repair lacks identity guards")
			}
			after, _ := cs.CoreV1().Services(ns).Get(ctx, service.Name, metav1.GetOptions{})
			if after.Spec.Ports[0].Port != 80 || after.UID != service.UID || after.Spec.ClusterIP != service.Spec.ClusterIP {
				t.Fatal("Service identity/address not preserved")
			}
		})
	}
}
