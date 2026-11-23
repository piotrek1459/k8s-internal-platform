package operations

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/platform"
)

type preparingHelm struct {
	fakeHelm
	store           platform.Store
	prepareCalls    int
	prepareError    error
	frozenCommitted bool
}

func (h *preparingHelm) PrepareReapply(ctx context.Context, _ string, revision platform.Revision) error {
	h.prepareCalls++
	recorded, err := h.store.GetRevision(ctx, revision.ID)
	h.frozenCommitted = err == nil && recorded.Frozen != nil && recorded.Frozen.ImageDigest == revision.Frozen.ImageDigest
	return h.prepareError
}

func TestWorkerPreparesOnlyExplicitReapplyAfterDurableFreeze(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "preparation-failure"}[failure], func(t *testing.T) {
			ctx := context.Background()
			st := newStore(t)
			helm := &preparingHelm{store: st}
			worker := NewWorker(st, helm, nil, &fakeInspector{digest: "sha256:original"}, fixedClock{t: time.Unix(1, 0).UTC()}, discard())
			_, revisionID := seedCreate(t, st, "app1", "example.com/app:v1")
			worker.DrainOnce(ctx)
			if helm.prepareCalls != 0 {
				t.Fatal("create used explicit reapply preparation")
			}
			app, _ := st.GetApplication(ctx, "app1")
			revision, _ := st.GetRevision(ctx, revisionID)
			factory := applications.Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}}
			request := factory.NewReapplyRequest(app, revision, app.Version, "reapply-key", "reapply-hash", "admin", "request")
			accepted, err := st.AcceptOperation(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if failure {
				helm.prepareError = errors.New("controlled preparation failure")
			}
			worker.DrainOnce(ctx)
			op, _ := st.GetOperation(ctx, accepted.OperationID)
			if helm.prepareCalls != 1 || !helm.frozenCommitted {
				t.Fatal("reapply preparation ran before frozen intent was committed")
			}
			if failure {
				current, _ := st.GetApplication(ctx, app.ID)
				if len(helm.installed) != 1 || op.State != platform.OperationNeedsAttention || current.ActiveOperationID != op.ID {
					t.Fatal("preparation failure applied or unlocked an unresolved target")
				}
			} else if len(helm.installed) != 2 || op.State != platform.OperationSucceeded {
				t.Fatal("prepared reapply was not applied successfully")
			}
		})
	}
}
