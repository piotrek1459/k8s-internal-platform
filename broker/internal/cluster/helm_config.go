package cluster

import (
	"fmt"
	"log/slog"

	"helm.sh/helm/v3/pkg/action"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// HelmActionConfig returns a factory that builds a Helm action configuration
// bound to the broker's in-cluster credentials and the given namespace. The Helm
// SDK drives releases through the same Kubernetes API the broker already uses, so
// no administrator kubeconfig is involved. The returned function matches the
// signature the helm adapter expects.
func HelmActionConfig(namespace string, logger *slog.Logger) func(string) (*action.Configuration, error) {
	return func(ns string) (*action.Configuration, error) {
		if ns == "" {
			ns = namespace
		}
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("load in-cluster config for helm: %w", err)
		}
		actionConfig := new(action.Configuration)
		// The "secret" storage driver stores release state as Kubernetes Secrets in
		// the release namespace, which the broker's RBAC permits.
		if err := actionConfig.Init(&restGetter{cfg: cfg, namespace: ns}, ns, "secret", func(format string, v ...interface{}) {
			logger.Debug("helm", "message", fmt.Sprintf(format, v...))
		}); err != nil {
			return nil, fmt.Errorf("init helm action configuration: %w", err)
		}
		return actionConfig, nil
	}
}

// restGetter adapts an in-cluster *rest.Config to Helm's RESTClientGetter without
// reading any kubeconfig file. It builds discovery and REST-mapper clients from
// the same in-cluster credentials.
type restGetter struct {
	cfg       *rest.Config
	namespace string
}

var _ genericclioptions.RESTClientGetter = (*restGetter)(nil)

func (g *restGetter) ToRESTConfig() (*rest.Config, error) { return g.cfg, nil }

func (g *restGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(g.cfg)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(dc), nil
}

func (g *restGetter) ToRESTMapper() (meta.RESTMapper, error) {
	dc, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(dc)
	return restmapper.NewShortcutExpander(mapper, dc, func(string) {}), nil
}

func (g *restGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	// No kubeconfig file is used; provide an overrides-only loader carrying the
	// namespace so Helm clients that consult it see the right default namespace.
	overrides := &clientcmd.ConfigOverrides{Context: clientcmdapi.Context{Namespace: g.namespace}}
	return clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), overrides)
}
