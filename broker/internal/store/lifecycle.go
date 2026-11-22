package store

import (
	"context"
	"fmt"

	"k8s-internal-platform/broker/internal/platform"
)

// TransitionDeleteOperation atomically marks the operation succeeded and sets
// deleted=1 on the application in the same SQLite transaction. This closes the
// non-atomic window that existed when TransitionOperation and MarkApplicationDeleted
// were called sequentially: a crash between the two left the app visible with no
// active lock and no deleted flag.
func (s *Store) TransitionDeleteOperation(ctx context.Context, operationID string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var appID string
	if err := tx.QueryRowContext(ctx, `SELECT app_id FROM operations WHERE id = ?`, operationID).Scan(&appID); err != nil {
		return fmt.Errorf("delete transition: %w", err)
	}

	now := s.nowString()
	if _, err := tx.ExecContext(ctx,
		`UPDATE operations SET state = ?, stage = ?, updated_at = ? WHERE id = ?`,
		string(platform.OperationSucceeded), "succeeded", now, operationID); err != nil {
		return fmt.Errorf("delete transition update op: %w", err)
	}

	// Release the active-operation lock and set deleted in the same row update.
	if _, err := tx.ExecContext(ctx,
		`UPDATE applications SET active_operation_id = NULL, deleted = 1, updated_at = ? WHERE id = ?`,
		now, appID); err != nil {
		return fmt.Errorf("delete transition update app: %w", err)
	}

	return tx.Commit()
}

// GetApplicationWithRevision fetches an application and its current desired
// revision in two sequential reads. It is used by mutation handlers that need
// both the application version (for the optimistic lock) and the current spec
// (for restart/reapply, which carry the spec forward unchanged).
func (s *Store) GetApplicationWithRevision(ctx context.Context, appID string) (platform.Application, platform.Revision, error) {
	app, err := s.GetApplication(ctx, appID)
	if err != nil {
		return platform.Application{}, platform.Revision{}, err
	}
	if app.DesiredRevisionID == "" {
		return app, platform.Revision{}, nil
	}
	rev, err := s.GetRevision(ctx, app.DesiredRevisionID)
	if err != nil {
		return platform.Application{}, platform.Revision{}, err
	}
	return app, rev, nil
}

// RestoreApplication atomically records a failed candidate, restores desired
// intent and releases the lock only after external restoration was verified.
func (s *Store) RestoreApplication(ctx context.Context, operationID, revisionID string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var appID, target string
	if err := tx.QueryRowContext(ctx, `SELECT app_id, target_revision FROM operations WHERE id = ?`, operationID).Scan(&appID, &target); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE applications SET desired_revision_id = ?, active_operation_id = NULL, updated_at = ? WHERE id = ? AND active_operation_id = ? AND last_successful_revision = ?`, revisionID, s.nowString(), appID, operationID, revisionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return platform.Errorf(platform.CodeConflict, "application recovery state changed")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE revisions SET result = 'failed' WHERE id = ?`, target); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE operations SET state = 'failed', stage = 'restored', recovery_outcome = 'previous successful revision restored and verified', updated_at = ? WHERE id = ?`, s.nowString(), operationID); err != nil {
		return err
	}
	return tx.Commit()
}

// RetryRecovery requeues a blocked operation under its existing identity. The
// application version prevents double-clicks from scheduling another retry.
func (s *Store) RetryRecovery(ctx context.Context, appID string, version int64) (platform.AcceptResult, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return platform.AcceptResult{}, err
	}
	defer tx.Rollback()
	var current int64
	var opID, state string
	if err := tx.QueryRowContext(ctx, `SELECT a.version, o.id, o.state FROM applications a JOIN operations o ON o.id = a.active_operation_id WHERE a.id = ? AND a.deleted = 0`, appID).Scan(&current, &opID, &state); err != nil {
		return platform.AcceptResult{}, platform.Errorf(platform.CodeConflict, "there is no blocked operation to recover")
	}
	if current != version {
		return platform.AcceptResult{}, platform.Errorf(platform.CodeVersionConflict, "application version has changed")
	}
	if state != string(platform.OperationNeedsAttention) {
		return platform.AcceptResult{}, platform.Errorf(platform.CodeConflict, "operation is not awaiting recovery")
	}
	now := s.nowString()
	if _, err := tx.ExecContext(ctx, `UPDATE operations SET state = 'recovering', stage = 'recovering', updated_at = ? WHERE id = ?`, now, opID); err != nil {
		return platform.AcceptResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE applications SET version = version + 1, updated_at = ? WHERE id = ?`, now, appID); err != nil {
		return platform.AcceptResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO operation_events (operation_id, at, stage, message) VALUES (?, ?, 'recovering', 'administrator requested recovery retry')`, opID, now); err != nil {
		return platform.AcceptResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return platform.AcceptResult{}, err
	}
	return platform.AcceptResult{ApplicationID: appID, OperationID: opID, State: platform.OperationRecovering}, nil
}
