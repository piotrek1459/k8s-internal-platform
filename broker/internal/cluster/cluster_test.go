package cluster

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"k8s-internal-platform/broker/internal/platform"
)

const ns = "platform-apps"

func ptrInt32(v int32) *int32 { return &v }
func ptrBool(v bool) *bool    { return &v }

func deployment(appID string, generation, observed int64, desired, ready int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       appID,
			Namespace:  ns,
			Generation: generation,
			Labels:     map[string]string{appIDLabel: appID},
		},
		Spec:   appsv1.DeploymentSpec{Replicas: ptrInt32(desired)},
		Status: appsv1.DeploymentStatus{ObservedGeneration: observed, ReadyReplicas: ready},
	}
}

func TestObserveReady(t *testing.T) {
	cs := fake.NewSimpleClientset(
		deployment("app1", 1, 1, 2, 2),
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: ns, Labels: map[string]string{appIDLabel: "app1"}}},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{Name: "app1-x", Namespace: ns, Labels: map[string]string{"kubernetes.io/service-name": "app1"}},
			Endpoints:  []discoveryv1.Endpoint{{Conditions: discoveryv1.EndpointConditions{Ready: ptrBool(true)}}},
		},
	)
	c := NewWithClientset(cs, ns)
	st, err := c.Observe(context.Background(), "app1")
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if st.Phase != platform.WorkloadReady || !st.WorkloadReady {
		t.Fatalf("phase = %s ready=%v, want ready", st.Phase, st.WorkloadReady)
	}
	if !st.RouteReady {
		t.Fatal("expected RouteReady from a ready endpoint")
	}
	if st.ObservedAt.IsZero() {
		t.Fatal("ObservedAt not set")
	}
}

func TestObservePhases(t *testing.T) {
	tests := []struct {
		name                              string
		generation, observed              int64
		desired, ready                    int32
		wantPhase                         platform.WorkloadPhase
	}{
		{"stopped", 1, 1, 0, 0, platform.WorkloadStopped},
		{"rollout in progress", 2, 1, 1, 1, platform.WorkloadUpdating},
		{"deploying", 1, 1, 2, 0, platform.WorkloadDeploying},
		{"degraded", 1, 1, 3, 1, platform.WorkloadDegraded},
		{"ready", 1, 1, 1, 1, platform.WorkloadReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset(deployment("app1", tt.generation, tt.observed, tt.desired, tt.ready))
			c := NewWithClientset(cs, ns)
			st, err := c.Observe(context.Background(), "app1")
			if err != nil {
				t.Fatalf("observe: %v", err)
			}
			if st.Phase != tt.wantPhase {
				t.Fatalf("phase = %s, want %s", st.Phase, tt.wantPhase)
			}
		})
	}
}

func TestObserveUnknownWhenNoWorkload(t *testing.T) {
	cs := fake.NewSimpleClientset()
	c := NewWithClientset(cs, ns)
	st, err := c.Observe(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if st.Phase != platform.WorkloadUnknown {
		t.Fatalf("phase = %s, want unknown", st.Phase)
	}
}

func TestCreateImmutableSecretIsIdempotent(t *testing.T) {
	cs := fake.NewSimpleClientset()
	c := NewWithClientset(cs, ns)
	data := map[string][]byte{"value": []byte("s3cr3t")}

	name, _, err := c.CreateImmutableSecret(context.Background(), "sec_01", 1, data)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// The Secret must be marked immutable and carry no plaintext in its name.
	got, err := cs.CoreV1().Secrets(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Immutable == nil || !*got.Immutable {
		t.Fatal("secret is not immutable")
	}

	// Re-creating the same version converges rather than failing, so a retried
	// operation is safe.
	name2, _, err := c.CreateImmutableSecret(context.Background(), "sec_01", 1, data)
	if err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if name2 != name {
		t.Fatalf("idempotent create returned a different name: %s vs %s", name2, name)
	}
}
