//go:build integration

// Package integration contains tests that exercise the broker against a real
// K3s cluster. They are gated behind the "integration" build tag and the
// PLATFORM_INTEGRATION=1 environment variable so the default `go test ./...`
// stays hermetic and fast.
//
// Run with the local k3d cluster from scripts/dev-cluster.sh. The demo image must
// be imported and referenced by its digest-pinned form, because the broker
// deploys by digest:
//
//	./scripts/dev-cluster.sh up
//	./scripts/build-demo.sh native
//	k3d image import platform-demo:v1 --cluster platform-dev
//	# resolve the imported image's digest, e.g. with: docker image inspect
//	#   --format '{{index .RepoDigests 0}}' platform-demo:v1   (or the local id)
//	KUBECONFIG=.local/kubeconfig PLATFORM_INTEGRATION=1 \
//	  PLATFORM_DEMO_IMAGE='platform-demo@sha256:<64-hex>' \
//	  go test -tags integration ./integration/... -v
//
// The test drives the broker's durable store and worker (not just the Helm
// adapter) against the cluster, then simulates a broker restart with a fresh
// worker over the same database. It maps to the M2 completion evidence in
// docs/milestones.md: the broker accepts an operation durably, a worker executes
// it into a real release in platform-apps, status is observed ready, and the
// persisted records survive a restart.
package integration

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/cluster"
	"k8s-internal-platform/broker/internal/helm"
	"k8s-internal-platform/broker/internal/operations"
	"k8s-internal-platform/broker/internal/platform"
	"k8s-internal-platform/broker/internal/store"
)

func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("PLATFORM_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_INTEGRATION=1 and run with -tags integration against a k3d cluster")
	}
}

// demoDigest returns the digest-pinned demo image reference. The broker deploys
// by digest, so a tag is not acceptable; fail clearly rather than attempting an
// invalid reference.
func demoDigest(t *testing.T) string {
	t.Helper()
	ref := os.Getenv("PLATFORM_DEMO_IMAGE")
	if !strings.Contains(ref, "@sha256:") {
		t.Fatalf("set PLATFORM_DEMO_IMAGE to a digest-pinned reference (repo@sha256:...); got %q", ref)
	}
	return ref
}

func TestBrokerDeploysThroughStoreWorkerAndSurvivesRestart(t *testing.T) {
	requireIntegration(t)

	ns := envOr("PLATFORM_APPS_NAMESPACE", "platform-apps")
	image := demoDigest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	// Durable store on a temp database the test owns. The path is kept so a restart
	// can be simulated by reopening the same file.
	dbPath := t.TempDir() + "/platform.db"
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	st := store.New(db)

	adapter, err := helm.NewAdapter(ns, kubeconfigActionConfig(t, ns))
	if err != nil {
		t.Fatalf("helm adapter: %v", err)
	}
	obs := cluster.NewWithClientset(kubeClientset(t), ns)

	// Accept a create operation through the durable store, exactly as the API
	// handler does. The image is already digest-pinned, so no registry inspection
	// is needed; the worker reuses the recorded digest.
	factory := applications.Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}}
	spec, err := applications.ValidateCreate(applications.CreateInput{
		Name: "itest", Image: image, ContainerPort: 8080, Profile: "http",
		Exposure: platform.Exposure{Mode: platform.ExposureInternal},
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	req := factory.NewCreateRequest("itest", spec, "itest-key", "itest-hash", "admin", "req-itest")
	res, err := st.AcceptOperation(ctx, req)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	// The worker drains the queued operation into a real Helm release. The demo
	// image is already digest-pinned, so no inspector is required.
	worker := operations.NewWorker(st, adapter, obs, nil, platform.SystemClock{}, logger)
	worker.DrainOnce(ctx)

	op, err := st.GetOperation(ctx, res.OperationID)
	if err != nil {
		t.Fatalf("get operation: %v", err)
	}
	if op.State != platform.OperationSucceeded {
		t.Fatalf("operation state = %s, want succeeded", op.State)
	}

	// The workload becomes ready in the cluster.
	waitReady(t, ctx, obs, res.ApplicationID)

	// Simulate a broker restart: close the database and reopen the same file, then
	// run startup recovery. The completed operation must stay terminal and the
	// application's records must be preserved.
	if err := db.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	db2, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer db2.Close()
	recovered := store.New(db2)
	if err := operations.NewRecovery(recovered, adapter, logger).Scan(ctx); err != nil {
		t.Fatalf("recovery scan: %v", err)
	}
	app, err := recovered.GetApplication(ctx, res.ApplicationID)
	if err != nil {
		t.Fatalf("application lost after restart: %v", err)
	}
	if app.LastSuccessfulRevision == "" {
		t.Fatal("successful revision not recorded after restart")
	}
	op2, err := recovered.GetOperation(ctx, res.OperationID)
	if err != nil || op2.State != platform.OperationSucceeded {
		t.Fatalf("operation not preserved as succeeded after restart: %+v (err %v)", op2, err)
	}

	// Clean up the release.
	if err := adapter.Uninstall(ctx, res.ApplicationID); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
}

func waitReady(t *testing.T, ctx context.Context, obs platform.ClusterObserver, appID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		st, err := obs.Observe(ctx, appID)
		if err != nil {
			t.Fatalf("observe: %v", err)
		}
		if st.Phase == platform.WorkloadReady {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("workload did not become ready; last phase=%s", st.Phase)
		}
		time.Sleep(3 * time.Second)
	}
}

// filepathJoin returns the path backing an open store, for reopening it. The
// store does not expose its path, so this reuses the same handle's database by
// returning an empty string, signalling the caller to reuse db instead.
func filepathJoin(_ *store.DB) string { return "" }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
