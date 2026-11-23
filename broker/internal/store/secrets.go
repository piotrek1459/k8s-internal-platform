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
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var exists, deleting int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(1), COALESCE(MAX(deleting), 0) FROM secrets WHERE id = ?`, version.SecretID).Scan(&exists, &deleting); err != nil {
		return err
	}
	if exists == 0 {
		return platform.Errorf(platform.CodeNotFound, "secret %q not found", version.SecretID)
	}
	if deleting != 0 {
		return platform.Errorf(platform.CodeConflict, "secret deletion is pending; retry deletion")
	}
	if err := insertSecretVersion(ctx, tx, version.SecretID, version, s.nowString()); err != nil {
		return err
	}
	return tx.Commit()
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
		`SELECT id, owner, name, created_at, deleting FROM secrets WHERE id = ?`, id).
		Scan(&meta.ID, &meta.Owner, &meta.Name, &createdAt, &meta.Deleting)
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
		`SELECT id, owner, name, created_at, deleting FROM secrets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []platform.SecretMetadata
	for rows.Next() {
		var m platform.SecretMetadata
		var created string
		if err := rows.Scan(&m.ID, &m.Owner, &m.Name, &created, &m.Deleting); err != nil {
			return nil, err
		}
		m.CreatedAt = parseTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteSecret removes a secret only when nothing still depends on it. Two kinds
// of reference block deletion:
//
//   - A frozen revision that is pending or succeeded (running or
//     rollback-eligible), via its recorded secret versions.
//   - A revision attached to a nonterminal operation (queued, running,
//     recovering, or NeedsAttention), via its requested spec — even before it is
//     frozen — so an accepted deployment cannot lose the secret it will use.
//
// Retained history of a deleted application does not keep a live dependency.
// Reference checks and the deletion reservation share a transaction with the
// single-writer acceptance path; matching uses exact JSON fields, not SQL LIKE.
func (s *Store) DeleteSecret(ctx context.Context, id string) error {
	return s.changeSecretDeletion(ctx, id, false)
}

func (s *Store) BeginSecretDeletion(ctx context.Context, id string) error {
	return s.changeSecretDeletion(ctx, id, true)
}

func (s *Store) changeSecretDeletion(ctx context.Context, id string, reserve bool) error {
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

	var refs int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM revisions r JOIN applications a ON a.id = r.app_id
		 WHERE a.deleted = 0
		 AND (r.result IN ('pending','succeeded') OR EXISTS (
		   SELECT 1 FROM operations o WHERE o.target_revision = r.id AND o.state NOT IN ('succeeded','failed')))
		 AND (EXISTS (SELECT 1 FROM json_each(r.spec_json, '$.SecretRefs')
		   WHERE json_extract(value, '$.SecretID') = ?)
		 OR EXISTS (SELECT 1 FROM json_each(COALESCE(NULLIF(r.secret_versions_json, ''), '[]'))
		   WHERE json_extract(value, '$.SecretID') = ?))`, id, id).Scan(&refs); err != nil {
		return err
	}

	if refs > 0 {
		return platform.Errorf(platform.CodeSecretInUse,
			"secret %q is referenced by an active, rollback-eligible or in-flight revision", id)
	}
	if reserve {
		if _, err := tx.ExecContext(ctx, `UPDATE secrets SET deleting = 1 WHERE id = ?`, id); err != nil {
			return err
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM secret_versions WHERE secret_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM secrets WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
