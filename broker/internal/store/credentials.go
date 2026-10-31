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
	res, err := s.db.sql.ExecContext(ctx, `DELETE FROM registry_credentials WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete registry credential: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return platform.Errorf(platform.CodeNotFound, "registry credential %q not found", id)
	}
	return nil
}
