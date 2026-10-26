package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"k8s-internal-platform/broker/internal/platform"
)

// ClaimNextOperation atomically selects and marks the oldest claimable operation.
// An operation is claimable when it is Queued or Recovering and its application is
// not blocked by an unresolved NeedsAttention operation. The claim bumps the
// operation to Running (Queued) or keeps Recovering, records claimed_at, and
// returns it. ok is false when nothing is claimable.
func (s *Store) ClaimNextOperation(ctx context.Context) (platform.Operation, bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return platform.Operation{}, false, err
	}
	defer tx.Rollback() //nolint:errcheck // rolled back unless we commit

	// Apps with an unresolved NeedsAttention op are excluded entirely so a blocked
	// application never has a second operation executed against it.
	row := tx.QueryRowContext(ctx,
		`SELECT `+operationColumns+` FROM operations o
		 WHERE o.state IN (?, ?)
		   AND NOT EXISTS (
		       SELECT 1 FROM operations b
		       WHERE b.app_id = o.app_id AND b.state = ? AND b.id <> o.id
		   )
		 ORDER BY o.created_at
		 LIMIT 1`,
		string(platform.OperationQueued), string(platform.OperationRecovering),
		string(platform.OperationNeedsAttention))
	op, err := scanOperation(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return platform.Operation{}, false, nil
		}
		return platform.Operation{}, false, err
	}

	now := s.nowString()
	next := platform.OperationRunning
	if op.State == platform.OperationRecovering {
		next = platform.OperationRecovering
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE operations SET state = ?, claimed_at = ?, updated_at = ? WHERE id = ?`,
		string(next), now, now, op.ID); err != nil {
		return platform.Operation{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return platform.Operation{}, false, err
	}
	op.State = next
	return op, true, nil
}

// FreezeExecutionSpec records the immutable execution spec on a revision. It must
// be committed before any external cluster mutation so a retry reuses it verbatim.
// Freezing is one-shot: a revision already frozen is left unchanged.
func (s *Store) FreezeExecutionSpec(ctx context.Context, revisionID string, spec platform.FrozenSpec) error {
	secretVersJSON := ""
	if len(spec.SecretVersions) > 0 {
		secretVersJSON = mustJSON(spec.SecretVersions)
	}
	res, err := s.db.sql.ExecContext(ctx,
		`UPDATE revisions
		 SET frozen = 1, image_digest = ?, chart_version = ?, chart_artifact_digest = ?,
		     secret_versions_json = ?, values_hash = ?, frozen_at = ?
		 WHERE id = ? AND frozen = 0`,
		spec.ImageDigest, spec.ChartVersion, spec.ChartArtifactDigest,
		secretVersJSON, spec.ValuesHash, formatTime(spec.FrozenAt), revisionID)
	if err != nil {
		return fmt.Errorf("freeze revision: %w", err)
	}
	// Zero rows means the revision was already frozen (or missing); both are safe
	// to treat as idempotent no-ops for retry, but a missing revision is a bug.
	if n, _ := res.RowsAffected(); n == 0 {
		var exists int
		_ = s.db.sql.QueryRowContext(ctx, `SELECT COUNT(1) FROM revisions WHERE id = ?`, revisionID).Scan(&exists)
		if exists == 0 {
			return platform.Errorf(platform.CodeNotFound, "revision %q not found", revisionID)
		}
	}
	return nil
}

func (s *Store) AppendOperationEvent(ctx context.Context, ev platform.OperationEvent) error {
	at := ev.At
	if at.IsZero() {
		at = s.db.now()
	}
	_, err := s.db.sql.ExecContext(ctx,
		`INSERT INTO operation_events (operation_id, at, stage, message, request_id)
		 VALUES (?, ?, ?, ?, ?)`,
		ev.OperationID, formatTime(at), ev.Stage, ev.Message, ev.RequestID)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

// TransitionOperation updates an operation's state/stage. Terminal states release
// the application's active-operation lock; NeedsAttention retains it so
// conflicting mutations keep returning 409 until a documented recovery clears it.
func (s *Store) TransitionOperation(ctx context.Context, operationID string, to platform.OperationState, stage, recoveryOutcome string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var appID string
	if err := tx.QueryRowContext(ctx, `SELECT app_id FROM operations WHERE id = ?`, operationID).Scan(&appID); err != nil {
		return notFound(err, "operation", operationID)
	}

	now := s.nowString()
	if _, err := tx.ExecContext(ctx,
		`UPDATE operations SET state = ?, stage = ?, recovery_outcome = ?, updated_at = ? WHERE id = ?`,
		string(to), stage, recoveryOutcome, now, operationID); err != nil {
		return err
	}

	// Release the lock only on a terminal transition. NeedsAttention is nonterminal
	// and must keep the application blocked.
	if to.IsTerminal() {
		if _, err := tx.ExecContext(ctx,
			`UPDATE applications SET active_operation_id = NULL, updated_at = ?
			 WHERE id = ? AND active_operation_id = ?`,
			now, appID, operationID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MarkRevisionResult records a revision's terminal result and, on success,
// advances the application's last-successful pointer.
func (s *Store) MarkRevisionResult(ctx context.Context, revisionID string, result platform.RevisionResult) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var appID string
	if err := tx.QueryRowContext(ctx, `SELECT app_id FROM revisions WHERE id = ?`, revisionID).Scan(&appID); err != nil {
		return notFound(err, "revision", revisionID)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE revisions SET result = ? WHERE id = ?`, string(result), revisionID); err != nil {
		return err
	}
	if result == platform.RevisionSucceeded {
		if _, err := tx.ExecContext(ctx,
			`UPDATE applications SET last_successful_revision = ?, updated_at = ? WHERE id = ?`,
			revisionID, s.nowString(), appID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SaveObservedStatus(ctx context.Context, appID string, status platform.ObservedStatus) error {
	_, err := s.db.sql.ExecContext(ctx,
		`INSERT INTO observed_status (app_id, status_json, observed_at) VALUES (?, ?, ?)
		 ON CONFLICT(app_id) DO UPDATE SET status_json = excluded.status_json, observed_at = excluded.observed_at`,
		appID, mustJSON(status), formatTime(status.ObservedAt))
	if err != nil {
		return fmt.Errorf("save observed status: %w", err)
	}
	return nil
}

func (s *Store) GetObservedStatus(ctx context.Context, appID string) (platform.ObservedStatus, error) {
	var statusJSON string
	err := s.db.sql.QueryRowContext(ctx,
		`SELECT status_json FROM observed_status WHERE app_id = ?`, appID).Scan(&statusJSON)
	if err != nil {
		return platform.ObservedStatus{}, notFound(err, "observed status for application", appID)
	}
	var status platform.ObservedStatus
	if err := json.Unmarshal([]byte(statusJSON), &status); err != nil {
		return platform.ObservedStatus{}, err
	}
	return status, nil
}
