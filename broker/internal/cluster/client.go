// Package cluster adapts the Kubernetes API to the platform's ClusterObserver and
// SecretManager ports using client-go. It uses the broker pod's in-cluster service
// account; it never loads an administrator kubeconfig. All reads and writes are
// scoped to the applications namespace the broker manages.
package cluster

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Client holds a Kubernetes clientset and the namespace the broker manages.
type Client struct {
	cs        kubernetes.Interface
	namespace string
}

// NewInCluster builds a Client from the pod's in-cluster configuration. It fails
// when not running inside a cluster, which is the intended deployment mode.
func NewInCluster(namespace string) (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load in-cluster config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes client: %w", err)
	}
	return &Client{cs: cs, namespace: namespace}, nil
}

// NewWithClientset builds a Client around an existing clientset. Tests pass a
// fake clientset here; production uses NewInCluster.
func NewWithClientset(cs kubernetes.Interface, namespace string) *Client {
	return &Client{cs: cs, namespace: namespace}
}

// Ping checks Kubernetes API reachability for the readiness probe. It performs a
// cheap namespace get rather than listing workloads.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cs.CoreV1().Namespaces().Get(ctx, c.namespace, metav1.GetOptions{})
	return err
}
