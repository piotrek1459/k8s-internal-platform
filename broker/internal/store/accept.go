package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"k8s-internal-platform/broker/internal/platform"
)

// AcceptOperation durably accepts one mutation in a single transaction. It is the
// only path that admits new work, and it enforces three invariants from
// docs/api-and-data.md:
//
//   - Idempotency: a prior operation with the same (app, idempotency key) returns
//     the original result. If the stored request hash differs, the key is being
//     reused for a different body and IDEMPOTENCY_CONFLICT (409) is returned.
//   - Optimistic concurrency: for an existing application the caller's expected
//     version must match the stored version, else VERSION_CONFLICT (412).
//   - Single active operation: an application already holding an active operation
//     rejects new work with CONFLICT (409); this is also the NeedsAttention block.
//
// On success it writes {application (new or version-bumped), candidate revision,
// operation, first operation event, audit event} and sets the application's
// active operation. Any failure rolls the whole transaction back, so no partial
// state is observable.
func (s *Store) AcceptOperation(ctx context.Context, req platform.AcceptRequest) (platform.AcceptResult, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return platform.AcceptResult{}, fmt.Errorf("begin: %w", err)
	}
	// Roll back on any early return; a successful Commit makes this a no-op.
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Idempotency replay: same key on the same app returns the original operation.
	if req.IdempotencyKey != "" && !req.IsNewApp {
		if res, found, err := lookupIdempotent(ctx, tx, req.Application.ID, req.IdempotencyKey, req.RequestHash); err != nil {
			return platform.AcceptResult{}, err
		} else if found {
			committed = true // nothing written; release the deferred rollback cleanly
			_ = tx.Rollback()
			return res, nil
		}
	}

	now := s.nowString()

	if req.IsNewApp {
		if err := insertApplication(ctx, tx, req.Application, now); err != nil {
			return platform.AcceptResult{}, err
		}
	} else {
		// Lock + version check + active-operation check in one guarded update.
		if err := guardExistingApplication(ctx, tx, req.Application.ID, req.ExpectVersion); err != nil {
			return platform.AcceptResult{}, err
		}
	}

	if err := insertRevision(ctx, tx, req.Revision, now); err != nil {
		return platform.AcceptResult{}, err
	}
	if err := insertOperation(ctx, tx, req, now); err != nil {
		return platform.AcceptResult{}, err
	}
	// Point the application at the new desired revision and take the active-op lock.
	if _, err := tx.ExecContext(ctx,
		`UPDATE applications SET desired_revision_id = ?, active_operation_id = ?, updated_at = ?
		 WHERE id = ?`,
		req.Revision.ID, req.Operation().ID, now, req.Application.ID); err != nil {
		return platform.AcceptResult{}, fmt.Errorf("link operation: %w", err)
	}
	if err := insertFirstEvent(ctx, tx, req, now); err != nil {
		return platform.AcceptResult{}, err
	}
	if err := insertAudit(ctx, tx, req, now); err != nil {
		return platform.AcceptResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return platform.AcceptResult{}, fmt.Errorf("commit: %w", err)
	}
	committed = true
	return platform.AcceptResult{
		ApplicationID: req.Application.ID,
		OperationID:   req.Operation().ID,
		State:         platform.OperationQueued,
	}, nil
}

func (s *Store) nowString() string { return formatTime(s.db.now()) }

func lookupIdempotent(ctx context.Context, tx *sql.Tx, appID, key, reqHash string) (platform.AcceptResult, bool, error) {
	var opID, storedHash, state string
	err := tx.QueryRowContext(ctx,
		`SELECT id, request_hash, state FROM operations WHERE app_id = ? AND idempotency_key = ?`,
		appID, key).Scan(&opID, &storedHash, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return platform.AcceptResult{}, false, nil
	}
	if err != nil {
		return platform.AcceptResult{}, false, err
	}
	if storedHash != reqHash {
		return platform.AcceptResult{}, false, platform.Errorf(platform.CodeIdempotencyConflict,
			"idempotency key reused with a different request body")
	}
	return platform.AcceptResult{
		ApplicationID: appID,
		OperationID:   opID,
		State:         platform.OperationState(state),
		Deduplicated:  true,
	}, true, nil
}

func insertApplication(ctx context.Context, tx *sql.Tx, app platform.Application, now string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO applications (id, name, version, deleted, created_at, updated_at)
		 VALUES (?, ?, 1, 0, ?, ?)`,
		app.ID, app.Name, now, now)
	if err != nil {
		// A UNIQUE violation on name is a client conflict, not a server fault.
		if isUniqueViolation(err) {
			return platform.FieldErrorf(platform.CodeConflict, "name",
				"an application named %q already exists", app.Name)
		}
		return fmt.Errorf("insert application: %w", err)
	}
	return nil
}

// guardExistingApplication enforces the version (412) and active-operation (409)
// invariants and bumps the version, all within the caller's transaction. The
// single guarded UPDATE is the atomic lock: if its WHERE clause does not match,
// exactly one of the two preconditions failed, and a follow-up read distinguishes
// which so the correct status is returned.
func guardExistingApplication(ctx context.Context, tx *sql.Tx, appID string, expectVersion int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE applications SET version = version + 1
		 WHERE id = ? AND version = ? AND active_operation_id IS NULL AND deleted = 0`,
		appID, expectVersion)
	if err != nil {
		return fmt.Errorf("guard application: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	// Distinguish missing / stale version / active operation for an accurate code.
	var version int64
	var activeOp sql.NullString
	var deleted int
	err = tx.QueryRowContext(ctx,
		`SELECT version, active_operation_id, deleted FROM applications WHERE id = ?`, appID).
		Scan(&version, &activeOp, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return platform.Errorf(platform.CodeNotFound, "application %q not found", appID)
	}
	if err != nil {
		return err
	}
	if deleted != 0 {
		return platform.Errorf(platform.CodeNotFound, "application %q is deleted", appID)
	}
	if activeOp.Valid && activeOp.String != "" {
		return platform.Errorf(platform.CodeConflict,
			"application %q has an active operation", appID)
	}
	return platform.Errorf(platform.CodeVersionConflict,
		"application %q was modified concurrently (expected version %d, found %d)",
		appID, expectVersion, version)
}

func insertRevision(ctx context.Context, tx *sql.Tx, rev platform.Revision, now string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO revisions (id, app_id, number, spec_json, frozen, result, actor, created_at)
		 VALUES (?, ?, ?, ?, 0, 'pending', ?, ?)`,
		rev.ID, rev.AppID, rev.Number, mustJSON(rev.Spec), rev.Actor, now)
	if err != nil {
		return fmt.Errorf("insert revision: %w", err)
	}
	return nil
}

func insertOperation(ctx context.Context, tx *sql.Tx, req platform.AcceptRequest, now string) error {
	op := req.Operation()
	var idemKey any
	if req.IdempotencyKey != "" {
		idemKey = req.IdempotencyKey
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO operations
		 (id, app_id, type, source_revision, target_revision, state, stage,
		  idempotency_key, request_hash, recovery_outcome, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, '', ?, ?, '', ?, ?)`,
		op.ID, op.AppID, string(op.Type), nullIfEmpty(op.SourceRevision), op.TargetRevision,
		string(platform.OperationQueued), idemKey, req.RequestHash, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			// Concurrent submission with the same key raced us; report conflict.
			return platform.Errorf(platform.CodeIdempotencyConflict,
				"a concurrent request used the same idempotency key")
		}
		return fmt.Errorf("insert operation: %w", err)
	}
	return nil
}

func insertFirstEvent(ctx context.Context, tx *sql.Tx, req platform.AcceptRequest, now string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO operation_events (operation_id, at, stage, message, request_id)
		 VALUES (?, ?, 'queued', 'operation accepted', ?)`,
		req.Operation().ID, now, req.RequestID)
	if err != nil {
		return fmt.Errorf("insert first event: %w", err)
	}
	return nil
}

func insertAudit(ctx context.Context, tx *sql.Tx, req platform.AcceptRequest, now string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO audit_events (actor, action, target, at, outcome, request_id, operation_id)
		 VALUES (?, ?, ?, ?, 'accepted', ?, ?)`,
		req.Actor, "operation."+string(req.Type), req.Application.ID, now, req.RequestID, req.Operation().ID)
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
