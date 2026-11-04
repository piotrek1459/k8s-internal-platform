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
