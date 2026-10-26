//go:build integration

package integration

import (
	"fmt"
	"testing"

	"helm.sh/helm/v3/pkg/action"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// kubeClientset builds a client-go clientset from the ambient KUBECONFIG, for
// observing the cluster during the integration test.
func kubeClientset(t *testing.T) kubernetes.Interface {
	t.Helper()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("build clientset: %v", err)
	}
	return cs
}

// kubeconfigActionConfig returns a Helm action-configuration factory backed by
// the ambient KUBECONFIG and the given namespace, for driving the broker's Helm
// adapter against the real cluster.
func kubeconfigActionConfig(t *testing.T, namespace string) func(string) (*action.Configuration, error) {
	t.Helper()
	return func(ns string) (*action.Configuration, error) {
		if ns == "" {
			ns = namespace
		}
		flags := genericclioptions.NewConfigFlags(false)
		flags.Namespace = &ns
		actionConfig := new(action.Configuration)
		if err := actionConfig.Init(flags, ns, "secret", func(string, ...interface{}) {}); err != nil {
			return nil, fmt.Errorf("init helm: %w", err)
		}
		return actionConfig, nil
	}
}
