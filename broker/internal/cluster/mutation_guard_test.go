package cluster

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMutationGuardRefusesChangedOwnershipIncludingExistingRoutes(t *testing.T) {
	for _, kind := range []string{"Deployment", "Service", "Ingress"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			cs, manifest := driftFixture(t, true)
			c := NewWithClientset(cs, ns)
			if err := c.GuardMutation(ctx, "app_1", manifest); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "Deployment":
				obj, _ := cs.AppsV1().Deployments(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
				obj.Annotations["meta.helm.sh/release-name"] = "foreign"
				cs.AppsV1().Deployments(ns).Update(ctx, obj, metav1.UpdateOptions{})
			case "Service":
				obj, _ := cs.CoreV1().Services(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
				obj.Annotations["meta.helm.sh/release-name"] = "foreign"
				cs.CoreV1().Services(ns).Update(ctx, obj, metav1.UpdateOptions{})
			case "Ingress":
				obj, _ := cs.NetworkingV1().Ingresses(ns).Get(ctx, "app-app-1", metav1.GetOptions{})
				obj.Annotations["meta.helm.sh/release-name"] = "foreign"
				cs.NetworkingV1().Ingresses(ns).Update(ctx, obj, metav1.UpdateOptions{})
			}
			cs.ClearActions()
			if err := c.GuardMutation(ctx, "app_1", manifest+"\n---\n"+manifest); err == nil {
				t.Fatal("foreign resource accepted for mutation")
			}
			for _, action := range cs.Actions() {
				if action.GetVerb() != "get" {
					t.Fatal("ownership guard mutated cluster state")
				}
			}
		})
	}
}
