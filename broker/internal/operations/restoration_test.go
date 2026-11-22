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
		return platform.ReleaseStatus{}, errors.New("rollout timed out")
	}
	return h.fakeHelm.InstallOrUpgrade(ctx, id, r)
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
