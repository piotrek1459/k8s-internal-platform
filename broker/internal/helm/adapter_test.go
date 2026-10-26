package helm

import (
	"context"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chartutil"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"

	"k8s-internal-platform/broker/internal/platform"
)

// memoryActionConfig builds a Helm action configuration backed by in-memory
// release storage and a fake Kubernetes client, so install/upgrade exercise the
// real Helm engine (templating + schema validation) without a cluster. The
// storage is shared across invocations so a later call observes an earlier
// release, matching the cluster-backed storage used in production.
func memoryActionConfig(t *testing.T) func(string) (*action.Configuration, error) {
	t.Helper()
	shared := storage.Init(driver.NewMemory())
	return func(ns string) (*action.Configuration, error) {
		regClient, err := registry.NewClient()
		if err != nil {
			return nil, err
		}
		// The chart declares kubeVersion >=1.30; advertise a matching capability so
		// the in-memory engine validates the chart as it would on the real cluster.
		caps := chartutil.DefaultCapabilities.Copy()
		caps.KubeVersion = chartutil.KubeVersion{Version: "v1.33.0", Major: "1", Minor: "33"}
		cfg := &action.Configuration{
			Releases:       shared,
			KubeClient:     &kubefake.PrintingKubeClient{Out: testWriter{t}},
			Capabilities:   caps,
			RegistryClient: regClient,
			Log:            func(string, ...interface{}) {},
		}
		return cfg, nil
	}
}

type testWriter struct{ t *testing.T }

func (testWriter) Write(p []byte) (int, error) { return len(p), nil }

func frozenRevision(appID string, spec platform.RevisionSpec) platform.Revision {
	return platform.Revision{
		ID:    "rev_1",
		AppID: appID,
		Spec:  spec,
		Frozen: &platform.FrozenSpec{
			ImageDigest:  "ghcr.io/example/app@sha256:" + "a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1",
			ChartVersion: "0.1.0",
			FrozenAt:     time.Now(),
		},
	}
}

func baseSpec() platform.RevisionSpec {
	return platform.RevisionSpec{
		Image:         "ghcr.io/example/app:1.0.0",
		Profile:       platform.ProfileHTTP,
		ContainerPort: 8080,
		Replicas:      1,
		Exposure:      platform.Exposure{Mode: platform.ExposureInternal},
	}
}

func TestEmbeddedChartLoads(t *testing.T) {
	ch, err := loadEmbeddedChart()
	if err != nil {
		t.Fatalf("load embedded chart: %v", err)
	}
	if ch.Metadata.Name != "application" {
		t.Fatalf("chart name = %q, want application", ch.Metadata.Name)
	}
	if ch.Schema == nil {
		t.Fatal("embedded chart is missing values.schema.json")
	}
}

func TestInstallRendersAndSchemaValidates(t *testing.T) {
	a, err := NewAdapter("platform-apps", memoryActionConfig(t))
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	rev := frozenRevision("app_01", baseSpec())

	status, err := a.InstallOrUpgrade(context.Background(), "app_01", rev)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if status.Revision != 1 {
		t.Fatalf("release revision = %d, want 1", status.Revision)
	}

	// A second call upgrades the existing release.
	status, err = a.InstallOrUpgrade(context.Background(), "app_01", frozenRevision("app_01", withReplicas(baseSpec(), 2)))
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if status.Revision != 2 {
		t.Fatalf("upgraded revision = %d, want 2", status.Revision)
	}
}

func TestInstallRejectsUnfrozenRevision(t *testing.T) {
	a, err := NewAdapter("platform-apps", memoryActionConfig(t))
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	rev := platform.Revision{ID: "rev_x", AppID: "app_x", Spec: baseSpec()} // not frozen
	if _, err := a.InstallOrUpgrade(context.Background(), "app_x", rev); err == nil {
		t.Fatal("install of an unfrozen revision should fail")
	}
}

func TestBuildValuesPinsDigestAndMapsExposure(t *testing.T) {
	spec := baseSpec()
	spec.Exposure = platform.Exposure{Mode: platform.ExposureLAN, Hostname: "notes.pi.home.arpa"}
	values, err := BuildValues("app_01", frozenRevision("app_01", spec))
	if err != nil {
		t.Fatalf("build values: %v", err)
	}
	image := values["image"].(map[string]any)
	if image["tag"] != "" || image["digest"] == "" {
		t.Fatalf("image must be digest-pinned with empty tag: %+v", image)
	}
	ingress := values["ingress"].(map[string]any)
	if ingress["enabled"] != true || ingress["entrypoint"] != "private" {
		t.Fatalf("LAN exposure should enable the private ingress: %+v", ingress)
	}
}

func TestBuildValuesInternalExposureHasNoRoute(t *testing.T) {
	values, err := BuildValues("app_01", frozenRevision("app_01", baseSpec()))
	if err != nil {
		t.Fatalf("build values: %v", err)
	}
	ingress := values["ingress"].(map[string]any)
	if ingress["enabled"] != false {
		t.Fatal("internal exposure must not create an ingress route")
	}
}

func withReplicas(s platform.RevisionSpec, n int) platform.RevisionSpec {
	s.Replicas = n
	return s
}
