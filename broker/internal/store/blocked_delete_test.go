package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/platform"
)

func TestBlockedFirstDeploymentDeletion(t *testing.T) {
	for _, scenario := range []string{"remove", "stale", "running", "queued", "recovering", "previous-success", "update", "acceptance-failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			s := newStore(t)
			if _, err := s.AcceptOperation(ctx, newAccept("app_1", "broken", "op_1", "rev_1", "create-key", "create-hash")); err != nil {
				t.Fatal(err)
			}
			frozen := platform.FrozenSpec{ImageDigest: "example/app@sha256:original", ChartVersion: "original", ValuesHash: "original"}
			if err := s.FreezeExecutionSpec(ctx, "rev_1", frozen); err != nil {
				t.Fatal(err)
			}
			state := platform.OperationNeedsAttention
			switch scenario {
			case "running":
				state = platform.OperationRunning
			case "queued":
				state = platform.OperationQueued
			case "recovering":
				state = platform.OperationRecovering
			case "previous-success":
				if err := s.MarkRevisionResult(ctx, "rev_1", platform.RevisionSucceeded); err != nil {
					t.Fatal(err)
				}
			case "update":
				if _, err := s.db.sql.Exec(`UPDATE operations SET type = 'update' WHERE id = 'op_1'`); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.TransitionOperation(ctx, "op_1", state, "original", "original evidence"); err != nil {
				t.Fatal(err)
			}
			app, err := s.GetApplication(ctx, "app_1")
			if err != nil {
				t.Fatal(err)
			}
			before, _ := s.GetRevision(ctx, "rev_1")
			events, _ := s.ListOperationEvents(ctx, "op_1")
			factory := applications.Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}}
			req := factory.NewDeleteRequest(app, app.Version, "delete-key", "delete-hash", "admin", "request")
			if scenario == "stale" {
				req.ExpectVersion--
			}
			if scenario == "acceptance-failure" {
				req.Op.ID = "op_1" // Fail after supersession to prove full rollback.
			}
			res, err := s.AcceptOperation(ctx, req)
			if scenario != "remove" {
				if err == nil {
					t.Fatal("unsafe/stale/failed deletion was accepted")
				}
				if scenario != "acceptance-failure" {
					var domain *platform.Error
					want := platform.CodeConflict
					if scenario == "stale" {
						want = platform.CodeVersionConflict
					}
					if !errors.As(err, &domain) || domain.Code != want {
						t.Fatalf("unexpected rejection: %v", err)
					}
				}
				after, _ := s.GetApplication(ctx, "app_1")
				op, _ := s.GetOperation(ctx, "op_1")
				revision, _ := s.GetRevision(ctx, "rev_1")
				afterEvents, _ := s.ListOperationEvents(ctx, "op_1")
				if !reflect.DeepEqual(app, after) || op.State != state || !reflect.DeepEqual(before, revision) || !reflect.DeepEqual(events, afterEvents) {
					t.Fatal("rejected deletion changed original state/evidence")
				}
				return
			}
			after, _ := s.GetApplication(ctx, "app_1")
			op, _ := s.GetOperation(ctx, "op_1")
			revision, _ := s.GetRevision(ctx, "rev_1")
			if after.ActiveOperationID != res.OperationID || after.Version != app.Version+1 || after.Deleted || op.State != platform.OperationFailed || op.Stage != "removal_requested" || revision.Result != platform.RevisionFailed || !reflect.DeepEqual(before.Frozen, revision.Frozen) {
				t.Fatal("deletion did not retain evidence and transfer the lock to queued cleanup")
			}
			if replay, err := s.AcceptOperation(ctx, req); err != nil || !replay.Deduplicated || replay.OperationID != res.OperationID {
				t.Fatalf("deletion replay: %+v %v", replay, err)
			}
			claimed, ok, err := s.ClaimNextOperation(ctx)
			if err != nil || !ok || claimed.ID != res.OperationID || claimed.Type != platform.OperationDelete {
				t.Fatalf("cleanup claim: %+v %v %v", claimed, ok, err)
			}
			if err := s.TransitionDeleteOperation(ctx, claimed.ID); err != nil {
				t.Fatal(err)
			}
			after, _ = s.GetApplication(ctx, "app_1")
			if !after.Deleted || after.ActiveOperationID != "" {
				t.Fatal("completed cleanup did not tombstone the application")
			}
		})
	}
}
