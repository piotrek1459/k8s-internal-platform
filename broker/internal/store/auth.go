package store

import (
	"context"
	"database/sql"
	"fmt"

	"k8s-internal-platform/broker/internal/platform"
)

// CreateAdministrator stores the single administrator account. A second attempt
// (setup already completed) is rejected as a conflict, so the setup endpoint is
// genuinely single-use even if its bootstrap token were replayed.
func (s *Store) CreateAdministrator(ctx context.Context, admin platform.Administrator) error {
	now := s.nowString()
	// The data model permits one administrator; enforce it by refusing to insert
	// when any row already exists.
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM administrators`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return platform.Errorf(platform.CodeConflict, "administrator already exists")
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO administrators (id, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		admin.ID, admin.PasswordHash, now, now); err != nil {
		return fmt.Errorf("insert administrator: %w", err)
	}
	return tx.Commit()
}

// GetAdministrator returns the administrator account, if one has been created.
func (s *Store) GetAdministrator(ctx context.Context) (platform.Administrator, bool, error) {
	var (
		admin                platform.Administrator
		createdAt, updatedAt string
	)
	err := s.db.sql.QueryRowContext(ctx,
		`SELECT id, password_hash, created_at, updated_at FROM administrators LIMIT 1`).
		Scan(&admin.ID, &admin.PasswordHash, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return platform.Administrator{}, false, nil
	}
	if err != nil {
		return platform.Administrator{}, false, err
	}
	admin.CreatedAt = parseTime(createdAt)
	admin.UpdatedAt = parseTime(updatedAt)
	return admin, true, nil
}

func (s *Store) CreateSession(ctx context.Context, sess platform.Session) error {
	_, err := s.db.sql.ExecContext(ctx,
		`INSERT INTO sessions (id, admin_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		sess.ID, sess.AdminID, formatTime(sess.CreatedAt), formatTime(sess.ExpiresAt))
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func (s *Store) GetSession(ctx context.Context, id string) (platform.Session, error) {
	var (
		sess                 platform.Session
		createdAt, expiresAt string
		revokedAt            sql.NullString
	)
	err := s.db.sql.QueryRowContext(ctx,
		`SELECT id, admin_id, created_at, expires_at, revoked_at FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.AdminID, &createdAt, &expiresAt, &revokedAt)
	if err != nil {
		return platform.Session{}, notFound(err, "session", id)
	}
	sess.CreatedAt = parseTime(createdAt)
	sess.ExpiresAt = parseTime(expiresAt)
	sess.RevokedAt = parseTimePtr(revokedAt)
	return sess, nil
}

func (s *Store) RevokeSession(ctx context.Context, id string) error {
	_, err := s.db.sql.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE id = ?`, s.nowString(), id)
	return err
}
