package operations

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/platform"
	"k8s-internal-platform/broker/internal/store"
)

// statusHelm is a fake HelmInstaller whose Status is configurable, for driving
// the recovery classifier.
type statusHelm struct {
	status platform.ReleaseStatus
	exists bool
	err    error
}

func (s statusHelm) InstallOrUpgrade(context.Context, string, platform.Revision) (platform.ReleaseStatus, error) {
	return platform.ReleaseStatus{Revision: 1, Status: "deployed"}, nil
}
func (s statusHelm) Uninstall(context.Context, string) error { return nil }
func (s statusHelm) Status(context.Context, string) (platform.ReleaseStatus, bool, error) {
	return s.status, s.exists, s.err
}

// seedInterrupted creates an application whose create operation is left in the
// Running state, as a crash mid-apply would leave it, with the revision frozen.
func seedInterrupted(t *testing.T, st *store.Store, appID string, frozen bool) (opID, revID string) {
	t.Helper()
	opID, revID = seedCreate(t, st, appID, "ghcr.io/example/app:1.0")
	ctx := context.Background()
	if frozen {
		if err := st.FreezeExecutionSpec(ctx, revID, platform.FrozenSpec{
			ImageDigest: "ghcr.io/example/app@sha256:aaaa", FrozenAt: time.Unix(1, 0).UTC(),
		}); err != nil {
			t.Fatalf("freeze: %v", err)
		}
	}
	// Claim it so it is Running, then leave it (simulating a crash before finish).
	op, ok, err := st.ClaimNextOperation(ctx)
	if err != nil || !ok || op.ID != opID {
		t.Fatalf("claim to running: ok=%v err=%v", ok, err)
	}
	return opID, revID
}

func TestRecoveryFinishesAlreadyAppliedOperation(t *testing.T) {
	st := newStore(t)
	opID, revID := seedInterrupted(t, st, "app1", true)
	rec := NewRecovery(st, statusHelm{status: platform.ReleaseStatus{Status: "deployed"}, exists: true}, discard())

	if err := rec.Scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	op, _ := st.GetOperation(context.Background(), opID)
	if op.State != platform.OperationSucceeded {
		t.Fatalf("state = %s, want succeeded", op.State)
	}
	rev, _ := st.GetRevision(context.Background(), revID)
	if rev.Result != platform.RevisionSucceeded {
		t.Fatalf("revision result = %s, want succeeded", rev.Result)
	}
	app, _ := st.GetApplication(context.Background(), "app1")
	if app.ActiveOperationID != "" {
		t.Fatalf("lock not released after recovery success")
	}
}

func TestRecoveryResumesInterruptedInstallWithNoRelease(t *testing.T) {
	st := newStore(t)
	opID, _ := seedInterrupted(t, st, "app1", true)
	rec := NewRecovery(st, statusHelm{exists: false}, discard())

	if err := rec.Scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	op, _ := st.GetOperation(context.Background(), opID)
	if op.State != platform.OperationRecovering {
		t.Fatalf("state = %s, want recovering", op.State)
	}
	// It remains claimable so the worker finishes it.
	claimed, ok, _ := st.ClaimNextOperation(context.Background())
	if !ok || claimed.ID != opID {
		t.Fatalf("resumed operation is not claimable")
	}
}

func TestRecoveryBlocksPendingHelmRelease(t *testing.T) {
	st := newStore(t)
	opID, _ := seedInterrupted(t, st, "app1", true)
	// Helm reports a pending release left by an interrupted operation. Re-executing
	// would be rejected by the SDK, so recovery must block rather than resume.
	rec := NewRecovery(st, statusHelm{status: platform.ReleaseStatus{Status: "pending-upgrade"}, exists: true}, discard())

	if err := rec.Scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	op, _ := st.GetOperation(context.Background(), opID)
	if op.State != platform.OperationNeedsAttention {
		t.Fatalf("state = %s, want needs_attention for a pending release", op.State)
	}
	// Blocked: no claimable operation, so the worker cannot retry into the SDK's
	// "another operation in progress" rejection.
	if _, ok, _ := st.ClaimNextOperation(context.Background()); ok {
		t.Fatal("a pending-release app should have no claimable operation")
	}
}

func TestRecoveryBlocksWhenClusterUnknown(t *testing.T) {
	st := newStore(t)
	opID, _ := seedInterrupted(t, st, "app1", true)
	rec := NewRecovery(st, statusHelm{err: errors.New("api unreachable")}, discard())

	if err := rec.Scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	op, _ := st.GetOperation(context.Background(), opID)
	if op.State != platform.OperationNeedsAttention {
		t.Fatalf("state = %s, want needs_attention", op.State)
	}
	if op.RecoveryOutcome == "" {
		t.Fatal("expected a recorded recovery reason")
	}

	// The application remains blocked: a new mutation must return 409 CONFLICT,
	// and this must hold across the simulated restart recovery just performed.
	_, err := st.AcceptOperation(context.Background(), platform.AcceptRequest{
		Type:          platform.OperationUpdate,
		Application:   platform.Application{ID: "app1"},
		IsNewApp:      false,
		ExpectVersion: 1,
		Revision:      platform.Revision{ID: "rev_new", AppID: "app1", Number: 2, Result: platform.RevisionPending, Actor: "t"},
		Op:            platform.Operation{ID: "op_new", AppID: "app1", Type: platform.OperationUpdate, TargetRevision: "rev_new"},
		RequestHash:   "h2",
	})
	var derr *platform.Error
	if !errors.As(err, &derr) || derr.Code != platform.CodeConflict {
		t.Fatalf("mutation against a NeedsAttention app = %v, want CONFLICT", err)
	}

	// The blocked operation is also not claimable, so the worker cannot run a
	// second operation against the blocked application.
	if _, ok, _ := st.ClaimNextOperation(context.Background()); ok {
		t.Fatal("a blocked application should have no claimable operation")
	}
}

func TestRecoveryLeavesQueuedAndNeedsAttentionUntouched(t *testing.T) {
	st := newStore(t)
	// A freshly queued operation (never started) should be left for the worker.
	queuedOp, _ := seedCreate(t, st, "appq", "ghcr.io/example/app:1.0")
	rec := NewRecovery(st, statusHelm{}, discard())
	if err := rec.Scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	op, _ := st.GetOperation(context.Background(), queuedOp)
	if op.State != platform.OperationQueued {
		t.Fatalf("queued operation changed to %s", op.State)
	}
}
