package operations

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/platform"
	"k8s-internal-platform/broker/internal/store"
)

func discard() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

type fakeHelm struct {
	mu        sync.Mutex
	installed []platform.Revision // revisions as applied, in order
	err       error
}

func (f *fakeHelm) InstallOrUpgrade(_ context.Context, _ string, rev platform.Revision) (platform.ReleaseStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return platform.ReleaseStatus{}, f.err
	}
	f.installed = append(f.installed, rev)
	return platform.ReleaseStatus{Revision: len(f.installed), Status: "deployed"}, nil
}
func (f *fakeHelm) Uninstall(context.Context, string) error { return f.err }
func (f *fakeHelm) Status(context.Context, string) (platform.ReleaseStatus, bool, error) {
	return platform.ReleaseStatus{}, false, nil
}
func (f *fakeHelm) lastImage() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.installed) == 0 || f.installed[len(f.installed)-1].Frozen == nil {
		return ""
	}
	return f.installed[len(f.installed)-1].Frozen.ImageDigest
}

// fakeInspector returns a configurable digest, and can change it between calls to
// simulate a moved tag.
type fakeInspector struct {
	mu     sync.Mutex
	digest string
}

func (f *fakeInspector) Inspect(context.Context, string, string) (platform.ImageReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return platform.ImageReport{Digest: f.digest, Architectures: []string{"arm64", "amd64"}}, nil
}
func (f *fakeInspector) setDigest(d string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.digest = d
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(context.Background(), t.TempDir()+"/platform.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store.New(db)
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// seedCreate accepts a create operation and returns its IDs.
func seedCreate(t *testing.T, st *store.Store, appID, image string) (opID, revID string) {
	t.Helper()
	opID = "op_" + appID
	revID = "rev_" + appID
	_, err := st.AcceptOperation(context.Background(), platform.AcceptRequest{
		Type:        platform.OperationCreate,
		Application: platform.Application{ID: appID, Name: appID},
		IsNewApp:    true,
		Revision: platform.Revision{
			ID: revID, AppID: appID, Number: 1, Result: platform.RevisionPending, Actor: "t",
			Spec: platform.RevisionSpec{Image: image, Profile: platform.ProfileHTTP, ContainerPort: 8080, Replicas: 1,
				Exposure: platform.Exposure{Mode: platform.ExposureInternal}},
		},
		Op:          platform.Operation{ID: opID, AppID: appID, Type: platform.OperationCreate, TargetRevision: revID},
		RequestHash: "h",
	})
	if err != nil {
		t.Fatalf("seed create: %v", err)
	}
	return opID, revID
}

func TestWorkerExecutesCreateToSuccess(t *testing.T) {
	st := newStore(t)
	helm := &fakeHelm{}
	insp := &fakeInspector{digest: "sha256:aaaa"}
	w := NewWorker(st, helm, nil, insp, fixedClock{t: time.Unix(1, 0).UTC()}, discard())

	opID, revID := seedCreate(t, st, "app1", "ghcr.io/example/app:1.0")
	w.drain(context.Background())

	op, _ := st.GetOperation(context.Background(), opID)
	if op.State != platform.OperationSucceeded {
		t.Fatalf("operation state = %s, want succeeded", op.State)
	}
	rev, _ := st.GetRevision(context.Background(), revID)
	if rev.Frozen == nil || rev.Result != platform.RevisionSucceeded {
		t.Fatalf("revision not frozen+succeeded: %+v", rev)
	}
	// The applied image must be the digest-pinned form, not the tag.
	if got := helm.lastImage(); got != "ghcr.io/example/app@sha256:aaaa" {
		t.Fatalf("applied image = %q, want digest-pinned", got)
	}
	// The application lock is released after success.
	app, _ := st.GetApplication(context.Background(), "app1")
	if app.ActiveOperationID != "" {
		t.Fatalf("active operation lock not released: %q", app.ActiveOperationID)
	}
}

func TestWorkerFreezesDigestAgainstMovedTag(t *testing.T) {
	st := newStore(t)
	helm := &fakeHelm{}
	insp := &fakeInspector{digest: "sha256:original"}
	w := NewWorker(st, helm, nil, insp, fixedClock{t: time.Unix(1, 0).UTC()}, discard())

	opID, revID := seedCreate(t, st, "app1", "ghcr.io/example/app:latest")
	// Freeze the digest as part of the first (successful) execution.
	w.drain(context.Background())
	first, _ := st.GetRevision(context.Background(), revID)
	if first.Frozen.ImageDigest != "ghcr.io/example/app@sha256:original" {
		t.Fatalf("first frozen digest = %q", first.Frozen.ImageDigest)
	}

	// The tag now moves to a new digest in the registry.
	insp.setDigest("sha256:moved")

	// Re-running the SAME operation (e.g. a retry) must reuse the frozen digest,
	// never the newly resolved one. Reset the operation to claimable to simulate a
	// retry of the identical operation.
	if err := st.TransitionOperation(context.Background(), opID, platform.OperationRecovering, "retry", ""); err != nil {
		t.Fatalf("reset op: %v", err)
	}
	w.drain(context.Background())

	again, _ := st.GetRevision(context.Background(), revID)
	if again.Frozen.ImageDigest != "ghcr.io/example/app@sha256:original" {
		t.Fatalf("frozen digest changed after moved tag: %q", again.Frozen.ImageDigest)
	}
	if got := helm.lastImage(); got != "ghcr.io/example/app@sha256:original" {
		t.Fatalf("retry applied moved digest %q; must reuse frozen digest", got)
	}
}

func TestWorkerRecordsApplyFailure(t *testing.T) {
	st := newStore(t)
	helm := &fakeHelm{err: errors.New("rollout timed out")}
	insp := &fakeInspector{digest: "sha256:aaaa"}
	w := NewWorker(st, helm, nil, insp, fixedClock{t: time.Unix(1, 0).UTC()}, discard())

	opID, revID := seedCreate(t, st, "app1", "ghcr.io/example/app:1.0")
	w.drain(context.Background())

	op, _ := st.GetOperation(context.Background(), opID)
	if op.State != platform.OperationFailed {
		t.Fatalf("operation state = %s, want failed", op.State)
	}
	rev, _ := st.GetRevision(context.Background(), revID)
	if rev.Result != platform.RevisionFailed {
		t.Fatalf("revision result = %s, want failed", rev.Result)
	}
}

func TestWorkerConfigChangeReusesRecordedDigest(t *testing.T) {
	st := newStore(t)
	helm := &fakeHelm{}
	insp := &fakeInspector{digest: "sha256:aaaa"}
	clock := fixedClock{t: time.Unix(1, 0).UTC()}
	w := NewWorker(st, helm, nil, insp, clock, discard())

	// Create and succeed.
	_, _ = seedCreate(t, st, "app1", "ghcr.io/example/app:1.0")
	w.drain(context.Background())

	// A restart operation (no image change) must not call the inspector; even if
	// the tag moved, a restart reuses the recorded digest. Change the inspector's
	// digest to prove it is never consulted.
	insp.setDigest("sha256:should-not-be-used")
	restartRev := "rev_restart"
	restartOp := "op_restart"
	_, err := st.AcceptOperation(context.Background(), platform.AcceptRequest{
		Type:          platform.OperationRestart,
		Application:   platform.Application{ID: "app1"},
		IsNewApp:      false,
		ExpectVersion: 1,
		Revision: platform.Revision{ID: restartRev, AppID: "app1", Number: 2, Result: platform.RevisionPending, Actor: "t",
			Spec: platform.RevisionSpec{Image: "ghcr.io/example/app@sha256:aaaa", Profile: platform.ProfileHTTP, ContainerPort: 8080, Replicas: 1,
				Exposure: platform.Exposure{Mode: platform.ExposureInternal}}},
		Op:          platform.Operation{ID: restartOp, AppID: "app1", Type: platform.OperationRestart, TargetRevision: restartRev},
		RequestHash: "h2",
	})
	if err != nil {
		t.Fatalf("accept restart: %v", err)
	}
	w.drain(context.Background())

	if got := helm.lastImage(); got != "ghcr.io/example/app@sha256:aaaa" {
		t.Fatalf("restart applied %q; must reuse the recorded digest, not re-resolve", got)
	}
}
