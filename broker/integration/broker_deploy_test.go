//go:build integration

// Package integration contains tests that exercise the broker against a real
// K3s cluster. They are gated behind the "integration" build tag and the
// PLATFORM_INTEGRATION=1 environment variable so the default `go test ./...`
// stays hermetic and fast.
//
// Run with the local k3d cluster from scripts/dev-cluster.sh:
//
//	./scripts/dev-cluster.sh up
//	./scripts/build-demo.sh native
//	k3d image import platform-demo:v1 --cluster platform-dev
//	KUBECONFIG=.local/kubeconfig PLATFORM_INTEGRATION=1 \
//	    go test -tags integration ./integration/... -run TestBrokerDeploysThroughHelm -v
//
// The test drives the broker's in-process Helm adapter and cluster observer
// against the cluster, verifying the real Helm/Kubernetes behaviour the unit
// tests fake. It maps to the M2 completion evidence in docs/milestones.md: the
// broker creates a real release in platform-apps and observes it ready.
package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/cluster"
	"k8s-internal-platform/broker/internal/helm"
	"k8s-internal-platform/broker/internal/platform"
)

func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("PLATFORM_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_INTEGRATION=1 and run with -tags integration against a k3d cluster")
	}
}

func TestBrokerDeploysThroughHelm(t *testing.T) {
	requireIntegration(t)

	ns := envOr("PLATFORM_APPS_NAMESPACE", "platform-apps")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	adapter, err := helm.NewAdapter(ns, kubeconfigActionConfig(t, ns))
	if err != nil {
		t.Fatalf("helm adapter: %v", err)
	}

	appID := "itest"
	rev := platform.Revision{
		ID:    "rev_itest",
		AppID: appID,
		Spec: platform.RevisionSpec{
			Image: "platform-demo:v1", Profile: platform.ProfileHTTP,
			ContainerPort: 8080, Replicas: 1,
			Exposure: platform.Exposure{Mode: platform.ExposureInternal},
		},
		Frozen: &platform.FrozenSpec{
			// Reference the imported demo image. With a local tag, set
			// PLATFORM_DEMO_IMAGE to the digest-pinned form; the adapter requires a
			// digest. A tag import can also be referenced by its resolved digest.
			ImageDigest: envOr("PLATFORM_DEMO_IMAGE", "platform-demo@sha256:local"),
			FrozenAt:    time.Now(),
		},
	}

	status, err := adapter.InstallOrUpgrade(ctx, appID, rev)
	if err != nil {
		t.Fatalf("install through broker helm adapter: %v", err)
	}
	t.Logf("release: revision=%d status=%s", status.Revision, status.Status)

	obs := cluster.NewWithClientset(kubeClientset(t), ns)
	deadline := time.Now().Add(2 * time.Minute)
	for {
		st, oerr := obs.Observe(ctx, appID)
		if oerr != nil {
			t.Fatalf("observe: %v", oerr)
		}
		if st.Phase == platform.WorkloadReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("workload did not become ready; last phase=%s", st.Phase)
		}
		time.Sleep(3 * time.Second)
	}

	if err := adapter.Uninstall(ctx, appID); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
