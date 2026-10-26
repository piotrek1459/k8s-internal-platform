package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/platform"
)

// newAccept builds a valid create-application AcceptRequest with the given IDs.
func newAccept(appID, name, opID, revID, idemKey, reqHash string) platform.AcceptRequest {
	app := platform.Application{ID: appID, Name: name}
	rev := platform.Revision{
		ID:     revID,
		AppID:  appID,
		Number: 1,
		Spec:   platform.RevisionSpec{Image: "example/app:v1", Profile: platform.ProfileHTTP, ContainerPort: 8080, Replicas: 1},
		Result: platform.RevisionPending,
		Actor:  "admin",
	}
	op := platform.Operation{ID: opID, AppID: appID, Type: platform.OperationCreate, TargetRevision: revID}
	return platform.AcceptRequest{
		Type:           platform.OperationCreate,
		Application:    app,
		IsNewApp:       true,
		Revision:       rev,
		Op:             op,
		IdempotencyKey: idemKey,
		RequestHash:    reqHash,
		Actor:          "admin",
		RequestID:      "req_1",
	}
}

func newStore(t *testing.T) *Store {
	t.Helper()
	db := openTestDB(t)
	return New(db)
}

func TestAcceptOperationPersistsAtomically(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	res, err := s.AcceptOperation(ctx, newAccept("app_1", "notes", "op_1", "rev_1", "idem-1", "hash-a"))
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if res.ApplicationID != "app_1" || res.OperationID != "op_1" || res.State != platform.OperationQueued {
		t.Fatalf("unexpected result: %+v", res)
	}
	// The application now holds its active-operation lock and points at the revision.
	app, err := s.GetApplication(ctx, "app_1")
	if err != nil {
		t.Fatalf("get app: %v", err)
	}
	if app.ActiveOperationID != "op_1" || app.DesiredRevisionID != "rev_1" || app.Version != 1 {
		t.Fatalf("application not linked: %+v", app)
	}
	// Revision, operation, first event, and audit all exist.
	if _, err := s.GetRevision(ctx, "rev_1"); err != nil {
		t.Fatalf("revision missing: %v", err)
	}
	events, err := s.ListOperationEvents(ctx, "op_1")
	if err != nil || len(events) != 1 {
		t.Fatalf("events = %v (err %v), want 1", events, err)
	}
}

func TestAcceptOperationRollsBackOnFailure(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	// Pre-insert an operation with the ID that the second accept will try to use.
	// The duplicate primary key makes the operation insert fail midway through the
	// transaction, after the application and revision inserts. If persistence is
	// atomic, none of the new application/revision rows survive.
	if _, err := s.AcceptOperation(ctx, newAccept("app_pre", "pre", "op_dup", "rev_pre", "", "h")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	bad := newAccept("app_2", "second", "op_dup", "rev_2", "", "h2") // reuses op_dup
	_, err := s.AcceptOperation(ctx, bad)
	if err == nil {
		t.Fatal("expected the duplicate operation ID to fail the accept")
	}

	// Nothing from the failed accept must have landed.
	if _, err := s.GetApplication(ctx, "app_2"); err == nil {
		t.Fatal("application app_2 was persisted despite the failed transaction")
	}
	if _, err := s.GetRevision(ctx, "rev_2"); err == nil {
		t.Fatal("revision rev_2 was persisted despite the failed transaction")
	}
}

func TestAcceptOperationCreateIdempotentReplay(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	// A retried create must dedup to the original application and operation: the
	// client may not have received the first 202 and is retrying the same request.
	// Each attempt mints a fresh candidate app ID, so only the idempotency key
	// ties the retry to the first submission.
	first, err := s.AcceptOperation(ctx, newAccept("app_new1", "notes", "op_new1", "rev_new1", "create-key", "body-hash"))
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if first.Deduplicated {
		t.Fatal("first create should not be deduplicated")
	}

	retry, err := s.AcceptOperation(ctx, newAccept("app_new2", "notes", "op_new2", "rev_new2", "create-key", "body-hash"))
	if err != nil {
		t.Fatalf("create retry: %v", err)
	}
	if !retry.Deduplicated || retry.ApplicationID != first.ApplicationID || retry.OperationID != first.OperationID {
		t.Fatalf("create retry not deduplicated to original: %+v (original %+v)", retry, first)
	}

	// Only one application exists despite two submissions.
	apps, err := s.ListApplications(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("got %d applications, want 1 (duplicate create created a second app)", len(apps))
	}

	// Same key, different body → idempotency conflict, even for creates.
	_, err = s.AcceptOperation(ctx, newAccept("app_new3", "notes", "op_new3", "rev_new3", "create-key", "different-body"))
	assertCode(t, err, platform.CodeIdempotencyConflict)
}

func TestAcceptOperationIdempotentReplay(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	// Create an application and take its lock with an initial operation.
	if _, err := s.AcceptOperation(ctx, newAccept("app_3", "app3", "op_a", "rev_a", "", "h")); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Resolve that operation so the app is free for a keyed mutation.
	if err := s.TransitionOperation(ctx, "op_a", platform.OperationSucceeded, "done", ""); err != nil {
		t.Fatalf("transition: %v", err)
	}

	// A keyed update. The same key+hash replayed must return the original op and
	// must not create a second operation or bump the version again.
	update := func(opID string, number int64) platform.AcceptRequest {
		r := newAccept("app_3", "app3", opID, "rev_"+opID, "idem-update", "update-hash")
		r.IsNewApp = false
		r.ExpectVersion = 1
		r.Type = platform.OperationUpdate
		r.Op.Type = platform.OperationUpdate
		r.Revision.Number = number
		return r
	}
	first, err := s.AcceptOperation(ctx, update("op_u1", 2))
	if err != nil {
		t.Fatalf("first update: %v", err)
	}
	if first.Deduplicated {
		t.Fatal("first submission should not be deduplicated")
	}

	// Replay with the SAME key+body but a different candidate operation ID.
	replay, err := s.AcceptOperation(ctx, update("op_u2", 3))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replay.Deduplicated || replay.OperationID != first.OperationID {
		t.Fatalf("replay not deduplicated to original: %+v", replay)
	}

	// Same key, DIFFERENT body → idempotency conflict.
	diff := update("op_u3", 4)
	diff.RequestHash = "different-hash"
	_, err = s.AcceptOperation(ctx, diff)
	assertCode(t, err, platform.CodeIdempotencyConflict)
}

func TestAcceptOperationVersionAndActiveOpConflicts(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if _, err := s.AcceptOperation(ctx, newAccept("app_4", "app4", "op_c", "rev_c", "", "h")); err != nil {
		t.Fatalf("create: %v", err)
	}

	// An existing-app mutation while op_c still holds the lock → 409 CONFLICT.
	update := newAccept("app_4", "app4", "op_c2", "rev_c2", "", "h2")
	update.IsNewApp = false
	update.ExpectVersion = 1
	_, err := s.AcceptOperation(ctx, update)
	assertCode(t, err, platform.CodeConflict)

	// Release the lock, then submit with a stale expected version → 412.
	if err := s.TransitionOperation(ctx, "op_c", platform.OperationSucceeded, "", ""); err != nil {
		t.Fatalf("transition: %v", err)
	}
	stale := newAccept("app_4", "app4", "op_c3", "rev_c3", "", "h3")
	stale.IsNewApp = false
	stale.ExpectVersion = 99 // wrong
	_, err = s.AcceptOperation(ctx, stale)
	assertCode(t, err, platform.CodeVersionConflict)
}

func TestFreezeExecutionSpecIsOneShot(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if _, err := s.AcceptOperation(ctx, newAccept("app_5", "app5", "op_f", "rev_f", "", "h")); err != nil {
		t.Fatalf("create: %v", err)
	}
	spec := platform.FrozenSpec{ImageDigest: "example/app@sha256:aaa", ChartVersion: "0.1.0", ValuesHash: "vh", FrozenAt: time.Now().UTC()}
	if err := s.FreezeExecutionSpec(ctx, "rev_f", spec); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	rev, err := s.GetRevision(ctx, "rev_f")
	if err != nil || rev.Frozen == nil || rev.Frozen.ImageDigest != "example/app@sha256:aaa" {
		t.Fatalf("revision not frozen as expected: %+v (err %v)", rev, err)
	}
	// A second freeze with a different digest must be ignored (one-shot), so a
	// retry cannot change what runs.
	if err := s.FreezeExecutionSpec(ctx, "rev_f", platform.FrozenSpec{ImageDigest: "example/app@sha256:bbb", FrozenAt: time.Now().UTC()}); err != nil {
		t.Fatalf("second freeze: %v", err)
	}
	rev, _ = s.GetRevision(ctx, "rev_f")
	if rev.Frozen.ImageDigest != "example/app@sha256:aaa" {
		t.Fatalf("frozen digest changed on re-freeze: %s", rev.Frozen.ImageDigest)
	}
}

func assertCode(t *testing.T, err error, want platform.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", want)
	}
	var derr *platform.Error
	if !errors.As(err, &derr) {
		t.Fatalf("error %v is not a *platform.Error", err)
	}
	if derr.Code != want {
		t.Fatalf("error code = %s, want %s", derr.Code, want)
	}
}
