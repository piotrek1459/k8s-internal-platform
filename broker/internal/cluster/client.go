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

// Ping checks Kubernetes API reachability for the readiness probe. It lists
// Deployments in the managed namespace with a limit of 1 — a resource the
// broker's Role already authorises — rather than reading the cluster-scoped
// Namespace object, which the chart grants no permission for. A probe against an
// unauthorised resource would report a healthy installation as unreachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cs.AppsV1().Deployments(c.namespace).List(ctx, metav1.ListOptions{Limit: 1})
	return err
}
