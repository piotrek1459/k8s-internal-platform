package store

import (
	"context"
	"fmt"

	"k8s-internal-platform/broker/internal/platform"
)

func (s *Store) CreateRegistryCredential(ctx context.Context, cred platform.RegistryCredential) error {
	_, err := s.db.sql.ExecContext(ctx,
		`INSERT INTO registry_credentials (id, label, registry, username, k8s_secret_name, verification_status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cred.ID, cred.Label, cred.Registry, cred.Username,
		cred.K8sSecretName, cred.VerificationStatus, formatTime(cred.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert registry credential: %w", err)
	}
	return nil
}

func (s *Store) ListRegistryCredentials(ctx context.Context) ([]platform.RegistryCredential, error) {
	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT id, label, registry, username, k8s_secret_name, verification_status, created_at
		 FROM registry_credentials ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []platform.RegistryCredential
	for rows.Next() {
		var c platform.RegistryCredential
		var created string
		if err := rows.Scan(&c.ID, &c.Label, &c.Registry, &c.Username,
			&c.K8sSecretName, &c.VerificationStatus, &created); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(created)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRegistryCredential(ctx context.Context, id string) error {
	return s.changeRegistryCredentialDeletion(ctx, id, false)
}

// BeginRegistryCredentialDeletion reserves the credential before secret removal.
// Accepted mutations and resolution reject the tombstone; cleanup is retryable.
func (s *Store) BeginRegistryCredentialDeletion(ctx context.Context, id string) error {
	return s.changeRegistryCredentialDeletion(ctx, id, true)
}

func (s *Store) changeRegistryCredentialDeletion(ctx context.Context, id string, reserve bool) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var refs int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM revisions r JOIN applications a ON a.id = r.app_id WHERE a.deleted = 0 AND json_extract(r.spec_json, '$.RegistryCredentialID') = ? AND (r.result IN ('pending','succeeded') OR EXISTS (SELECT 1 FROM operations o WHERE o.target_revision = r.id AND o.state NOT IN ('succeeded','failed')))`, id).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return platform.Errorf(platform.CodeConflict, "registry credential is referenced by an active or rollback-eligible revision")
	}
	query := `DELETE FROM registry_credentials WHERE id = ?`
	if reserve {
		query = `UPDATE registry_credentials SET verification_status = 'deleting' WHERE id = ?`
	}
	res, err := tx.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete registry credential: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return platform.Errorf(platform.CodeNotFound, "registry credential %q not found", id)
	}
	return tx.Commit()
}
