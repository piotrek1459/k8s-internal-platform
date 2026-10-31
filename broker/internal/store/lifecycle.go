package store

import (
	"context"
	"fmt"

	"k8s-internal-platform/broker/internal/platform"
)

// MarkApplicationDeleted sets deleted=1 on an application after the cluster
// resources have been successfully removed by the delete operation worker.
func (s *Store) MarkApplicationDeleted(ctx context.Context, appID string) error {
	_, err := s.db.sql.ExecContext(ctx,
		`UPDATE applications SET deleted = 1, updated_at = ? WHERE id = ?`,
		s.nowString(), appID)
	if err != nil {
		return fmt.Errorf("mark application deleted: %w", err)
	}
	return nil
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
