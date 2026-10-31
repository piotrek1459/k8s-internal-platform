package store

import (
	"context"
	"path/filepath"
	"testing"
)

// openTestDB opens a fresh database in a temp file. A file (not :memory:) is used
// so WAL and the single-connection pool behave as in production.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "platform.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigrateCreatesSchema(t *testing.T) {
	db := openTestDB(t)
	// Every table the data model defines must exist after migration.
	wantTables := []string{
		"administrators", "sessions", "applications", "revisions", "operations",
		"operation_events", "audit_events", "registry_credentials", "secrets",
		"secret_versions", "volumes", "settings", "observed_status",
		"schema_migrations",
	}
	for _, name := range wantTables {
		var got string
		err := db.sql.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
		).Scan(&got)
		if err != nil {
			t.Fatalf("table %q missing after migration: %v", name, err)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "platform.db")
	// Opening twice migrates over an already-migrated database without error and
	// without reapplying migrations.
	db1, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	_ = db1.Close()

	db2, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("second open (migrate over existing): %v", err)
	}
	defer db2.Close()

	var applied int
	if err := db2.sql.QueryRow(`SELECT COUNT(1) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied migrations = %d, want 2", applied)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	db := openTestDB(t)
	// A revision referencing a nonexistent application must be rejected, proving
	// PRAGMA foreign_keys is active on the connection.
	_, err := db.sql.Exec(
		`INSERT INTO revisions (id, app_id, number, spec_json, actor, created_at)
		 VALUES ('rev_x', 'app_missing', 1, '{}', 'tester', '2026-01-01T00:00:00Z')`)
	if err == nil {
		t.Fatal("insert with dangling foreign key was accepted; foreign keys not enforced")
	}
}
