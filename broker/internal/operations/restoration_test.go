package operations

import (
	"context"
	"errors"
	"testing"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/platform"
)

type restorationHelm struct {
	fakeHelm
	verificationFails bool
	preparationError  error
	prepareCalls      int
	badAttempts       int
}

func (h *restorationHelm) PrepareExecution(_ string, r platform.Revision) (platform.FrozenSpec, error) {
	f := *r.Frozen
	f.ChartVersion = "0.1.0"
	f.ChartArtifactDigest = "sha256:chart"
	f.ValuesHash = "values"
	return f, nil
}
func (h *restorationHelm) InstallOrUpgrade(ctx context.Context, id string, r platform.Revision) (platform.ReleaseStatus, error) {
	if r.Spec.Image == "example.com/app@sha256:bad" {
		h.badAttempts++
		return platform.ReleaseStatus{}, errors.New("rollout timed out")
	}
	return h.fakeHelm.InstallOrUpgrade(ctx, id, r)
}

func (h *restorationHelm) PrepareRestoration(context.Context, string, platform.Revision) error {
	h.prepareCalls++
	return h.preparationError
}
func (h *restorationHelm) VerifyTarget(context.Context, string, platform.Revision) error {
	if h.verificationFails {
		return errors.New("live resources do not match")
	}
	return nil
}
func TestFailedCandidateOnlyUnlocksAfterVerifiedRestoration(t *testing.T) {
	for _, verificationFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "restored", true: "unverified"}[verificationFails], func(t *testing.T) {
			ctx := context.Background()
			st := newStore(t)
			h := &restorationHelm{}
			w := NewWorker(st, h, nil, nil, platform.SystemClock{}, discard())
			_, original := seedCreate(t, st, "app1", "example.com/app@sha256:good")
			w.DrainOnce(ctx)
			app, rev, _ := st.GetApplicationWithRevision(ctx, "app1")
			rev.Spec.Image = "example.com/app@sha256:bad"
			factory := applications.Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}}
			req := factory.NewUpdateRequest(app, rev.Spec, app.Version, "unhealthy", "bad", "admin", "")
			accepted, err := st.AcceptOperation(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			h.verificationFails = verificationFails
			w.DrainOnce(ctx)
			operation, _ := st.GetOperation(ctx, accepted.OperationID)
			app, _ = st.GetApplication(ctx, app.ID)
			if verificationFails {
				if operation.State != platform.OperationNeedsAttention || app.ActiveOperationID == "" {
					t.Fatal("unverified state was unlocked")
				}
				retry, err := st.RetryRecovery(ctx, app.ID, app.Version)
				if err != nil {
					t.Fatal(err)
				}
				if retry.OperationID != operation.ID {
					t.Fatal("retry changed operation identity")
				}
				if _, err := st.RetryRecovery(ctx, app.ID, app.Version); err == nil {
					t.Fatal("stale duplicate recovery accepted")
				}
				h.verificationFails = false
				w.DrainOnce(ctx)
				app, _ = st.GetApplication(ctx, app.ID)
			}
			operation, _ = st.GetOperation(ctx, accepted.OperationID)
			if operation.State != platform.OperationFailed || app.DesiredRevisionID != original || app.LastSuccessfulRevision != original || app.ActiveOperationID != "" {
				t.Fatalf("restoration not committed: %+v %+v", operation, app)
			}
		})
	}
}

func TestRecoveryRetryCompletesRestorationWithoutReexecutingFailedCandidate(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	h := &restorationHelm{}
	w := NewWorker(st, h, nil, nil, platform.SystemClock{}, discard())
	_, original := seedCreate(t, st, "app1", "example.com/app@sha256:good")
	w.DrainOnce(ctx)
	app, rev, _ := st.GetApplicationWithRevision(ctx, "app1")
	rev.Spec.Image = "example.com/app@sha256:bad"
	factory := applications.Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}}
	accepted, err := st.AcceptOperation(ctx, factory.NewUpdateRequest(app, rev.Spec, app.Version, "failed-key", "failed-hash", "admin", ""))
	if err != nil {
		t.Fatal(err)
	}
	h.preparationError = errors.New("controlled restoration preparation failure")
	w.DrainOnce(ctx)
	op, _ := st.GetOperation(ctx, accepted.OperationID)
	app, _ = st.GetApplication(ctx, app.ID)
	candidate, _ := st.GetRevision(ctx, op.TargetRevision)
	if op.State != platform.OperationNeedsAttention || app.ActiveOperationID != op.ID || len(h.installed) != 1 || candidate.Frozen == nil || candidate.Result != platform.RevisionFailed {
		t.Fatal("failed preparation applied or unlocked unverified state")
	}
	retry, err := st.RetryRecovery(ctx, app.ID, app.Version)
	if err != nil || retry.OperationID != op.ID {
		t.Fatal("supported retry did not retain operation identity")
	}
	h.preparationError = nil
	w.DrainOnce(ctx)
	op, _ = st.GetOperation(ctx, op.ID)
	app, _ = st.GetApplication(ctx, app.ID)
	after, _ := st.GetRevision(ctx, candidate.ID)
	if h.badAttempts != 1 || h.prepareCalls != 2 || len(h.installed) != 2 || h.installed[1].ID != original {
		t.Fatal("retry executed a known-failed candidate instead of restoring its predecessor")
	}
	if op.State != platform.OperationFailed || op.Stage != "restored" || app.ActiveOperationID != "" || app.DesiredRevisionID != original || after.Result != platform.RevisionFailed || after.Frozen.ImageDigest != candidate.Frozen.ImageDigest || after.Frozen.ValuesHash != candidate.Frozen.ValuesHash {
		t.Fatal("restoration retry lost failed/frozen evidence or did not verify/unlock")
	}
}
