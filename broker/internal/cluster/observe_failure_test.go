package cluster

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestObservePropagatesEndpointReadFailure(t *testing.T) {
	cs := fake.NewSimpleClientset(
		deployment("app1", 1, 1, 1, 1),
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: ns, Labels: map[string]string{appIDLabel: "app1"}}},
	)
	// Force EndpointSlice reads to fail. A read failure must propagate as an error
	// so the caller marks status stale rather than reporting a fresh
	// routeReady=false.
	cs.PrependReactor("list", "endpointslices", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("api unavailable")
	})
	c := NewWithClientset(cs, ns)
	if _, err := c.Observe(context.Background(), "app1"); err == nil {
		t.Fatal("expected endpoint read failure to propagate as an error")
	}
}
