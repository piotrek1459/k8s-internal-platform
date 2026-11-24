package store

import (
	"context"
	"errors"
	"testing"

	"k8s-internal-platform/broker/internal/platform"
)

func TestLANHostnameReservationIsAtomicAndRetainsRollbackOwnership(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	first := newAccept("app_owner", "owner", "op_owner", "rev_owner", "owner-key", "owner-hash")
	first.Revision.Spec.Exposure = platform.Exposure{Mode: platform.ExposureLAN, Hostname: "demo.apps.home.arpa"}
	if _, err := s.AcceptOperation(ctx, first); err != nil {
		t.Fatal(err)
	}
	if replay, err := s.AcceptOperation(ctx, first); err != nil || !replay.Deduplicated {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	other := newAccept("app_other", "other", "op_other", "rev_other", "other-key", "other-hash")
	other.Revision.Spec.Exposure = first.Revision.Spec.Exposure
	for _, stage := range []string{"pending", "succeeded", "blocked"} {
		result, state := "pending", "queued"
		if stage == "succeeded" {
			result, state = "succeeded", "succeeded"
		}
		if stage == "blocked" {
			result, state = "failed", "needs_attention"
		}
		if _, err := s.db.sql.Exec(`UPDATE revisions SET result=? WHERE id='rev_owner'`, result); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.sql.Exec(`UPDATE operations SET state=? WHERE id='op_owner'`, state); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AcceptOperation(ctx, other); !hostnameErrorCode(err, platform.CodeConflict) {
			t.Fatalf("%s reservation: %v", stage, err)
		}
		if _, err := s.GetApplication(ctx, "app_other"); !hostnameErrorCode(err, platform.CodeNotFound) {
			t.Fatalf("partial application persisted: %v", err)
		}
	}
	if _, err := s.db.sql.Exec(`UPDATE applications SET deleted=1 WHERE id='app_owner'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptOperation(ctx, other); err != nil {
		t.Fatalf("deleted owner did not release reservation: %v", err)
	}
}

func hostnameErrorCode(err error, code platform.ErrorCode) bool {
	var failure *platform.Error
	return errors.As(err, &failure) && failure.Code == code
}
