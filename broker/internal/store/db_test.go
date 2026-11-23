package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
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
	if applied != 3 {
		t.Fatalf("applied migrations = %d, want 3", applied)
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

func TestMigrationFailureRollsBackSchemaDataAndLedger(t *testing.T) {
	for _, failure := range []string{"statement", "ledger"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "platform.db")
			db, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.sql.Exec(`CREATE TABLE retained (value TEXT); INSERT INTO retained VALUES ('before')`); err != nil {
				t.Fatal(err)
			}
			body := `CREATE TABLE candidate (value TEXT); INSERT INTO candidate VALUES ('partial'); UPDATE retained SET value = 'after';`
			if failure == "statement" {
				body += ` INSERT INTO missing_table VALUES ('fail');`
			} else if _, err := db.sql.Exec(`CREATE TRIGGER fail_migration_ledger BEFORE INSERT ON schema_migrations BEGIN SELECT RAISE(ABORT, 'injected ledger failure'); END`); err != nil {
				t.Fatal(err)
			}
			source := fstest.MapFS{"migrations/0004_candidate.sql": &fstest.MapFile{Data: []byte(body)}}
			if err := db.migrateFrom(ctx, source); err == nil || !strings.Contains(err.Error(), "0004_candidate.sql") {
				t.Fatalf("expected an identified migration failure, got %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			// Reopening exercises durable rollback, rather than only the failed
			// connection's view of its own uncommitted transaction.
			db, err = Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var tables, ledger int
			var retained string
			if err := db.sql.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'candidate'`).Scan(&tables); err != nil {
				t.Fatal(err)
			}
			if err := db.sql.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = '0004_candidate.sql'`).Scan(&ledger); err != nil {
				t.Fatal(err)
			}
			if err := db.sql.QueryRow(`SELECT value FROM retained`).Scan(&retained); err != nil {
				t.Fatal(err)
			}
			if tables != 0 || ledger != 0 || retained != "before" {
				t.Fatalf("partial migration survived: table=%d ledger=%d retained=%q", tables, ledger, retained)
			}
			if _, err := db.sql.Exec(`DROP TRIGGER IF EXISTS fail_migration_ledger`); err != nil {
				t.Fatal(err)
			}
			source["migrations/0004_candidate.sql"].Data = []byte(`CREATE TABLE candidate (value TEXT); INSERT INTO candidate VALUES ('complete')`)
			for range 2 {
				if err := db.migrateFrom(ctx, source); err != nil {
					t.Fatalf("corrected migration/retry: %v", err)
				}
			}
			var value string
			if err := db.sql.QueryRow(`SELECT value FROM candidate`).Scan(&value); err != nil || value != "complete" {
				t.Fatalf("corrected migration result: value=%q err=%v", value, err)
			}
		})
	}
}
