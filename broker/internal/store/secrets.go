package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"k8s-internal-platform/broker/internal/platform"
)

// CreateSecret stores a secret's metadata and its first immutable version. The
// plaintext value is never stored here; only the Kubernetes Secret reference and
// the set of key names are recorded.
func (s *Store) CreateSecret(ctx context.Context, meta platform.SecretMetadata, first platform.SecretVersion) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := s.nowString()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO secrets (id, owner, name, created_at) VALUES (?, ?, ?, ?)`,
		meta.ID, meta.Owner, meta.Name, now); err != nil {
		if isUniqueViolation(err) {
			return platform.FieldErrorf(platform.CodeConflict, "name",
				"a secret named %q already exists", meta.Name)
		}
		return fmt.Errorf("insert secret: %w", err)
	}
	if err := insertSecretVersion(ctx, tx, meta.ID, first, now); err != nil {
		return err
	}
	return tx.Commit()
}

// AddSecretVersion appends a new immutable version to an existing secret.
func (s *Store) AddSecretVersion(ctx context.Context, version platform.SecretVersion) error {
	var exists int
	if err := s.db.sql.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM secrets WHERE id = ?`, version.SecretID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return platform.Errorf(platform.CodeNotFound, "secret %q not found", version.SecretID)
	}
	return insertSecretVersion(ctx, s.db.sql, version.SecretID, version, s.nowString())
}

// execer is satisfied by both *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertSecretVersion(ctx context.Context, ex execer, secretID string, v platform.SecretVersion, now string) error {
	created := now
	if !v.CreatedAt.IsZero() {
		created = formatTime(v.CreatedAt)
	}
	_, err := ex.ExecContext(ctx,
		`INSERT INTO secret_versions (secret_id, version, keys_json, k8s_secret_name, k8s_secret_uid, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		secretID, v.Version, mustJSON(v.Keys), v.K8sSecretName, v.K8sSecretUID, created)
	if err != nil {
		if isUniqueViolation(err) {
			return platform.Errorf(platform.CodeConflict,
				"version %d of secret %q already exists", v.Version, secretID)
		}
		return fmt.Errorf("insert secret version: %w", err)
	}
	return nil
}

func (s *Store) GetSecret(ctx context.Context, id string) (platform.SecretMetadata, []platform.SecretVersion, error) {
	var meta platform.SecretMetadata
	var createdAt string
	err := s.db.sql.QueryRowContext(ctx,
		`SELECT id, owner, name, created_at FROM secrets WHERE id = ?`, id).
		Scan(&meta.ID, &meta.Owner, &meta.Name, &createdAt)
	if err != nil {
		return platform.SecretMetadata{}, nil, notFound(err, "secret", id)
	}
	meta.CreatedAt = parseTime(createdAt)

	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT secret_id, version, keys_json, k8s_secret_name, k8s_secret_uid, created_at
		 FROM secret_versions WHERE secret_id = ? ORDER BY version`, id)
	if err != nil {
		return platform.SecretMetadata{}, nil, err
	}
	defer rows.Close()
	var versions []platform.SecretVersion
	for rows.Next() {
		var v platform.SecretVersion
		var keysJSON, created string
		if err := rows.Scan(&v.SecretID, &v.Version, &keysJSON, &v.K8sSecretName, &v.K8sSecretUID, &created); err != nil {
			return platform.SecretMetadata{}, nil, err
		}
		if err := json.Unmarshal([]byte(keysJSON), &v.Keys); err != nil {
			return platform.SecretMetadata{}, nil, err
		}
		v.CreatedAt = parseTime(created)
		versions = append(versions, v)
	}
	return meta, versions, rows.Err()
}

func (s *Store) ListSecrets(ctx context.Context) ([]platform.SecretMetadata, error) {
	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT id, owner, name, created_at FROM secrets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []platform.SecretMetadata
	for rows.Next() {
		var m platform.SecretMetadata
		var created string
		if err := rows.Scan(&m.ID, &m.Owner, &m.Name, &created); err != nil {
			return nil, err
		}
		m.CreatedAt = parseTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteSecret removes a secret only when no revision — active or
// rollback-eligible — references any of its versions. Because revisions record
// their secret versions inside the frozen spec JSON, the reference check matches
// the secret ID against every frozen revision's secret_versions_json.
func (s *Store) DeleteSecret(ctx context.Context, id string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM secrets WHERE id = ?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return platform.Errorf(platform.CodeNotFound, "secret %q not found", id)
	}

	// A frozen revision that is pending or succeeded is either running or
	// rollback-eligible; such a revision referencing this secret blocks deletion.
	var referencing int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM revisions
		 WHERE frozen = 1 AND result IN ('pending','succeeded')
		   AND secret_versions_json LIKE ?`, "%\""+id+"\"%").Scan(&referencing); err != nil {
		return err
	}
	if referencing > 0 {
		return platform.Errorf(platform.CodeSecretInUse,
			"secret %q is referenced by %d active or rollback-eligible revision(s)", id, referencing)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM secret_versions WHERE secret_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM secrets WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
