// Package store implements the platform.Store port on SQLite using the pure-Go
// modernc.org/sqlite driver, so the broker cross-compiles to arm64/amd64 without
// a C toolchain. It owns the schema, the embedded forward-only migrations, and
// every SQL statement; callers never compose their own transactions.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// DB wraps a SQLite connection pool configured for the broker's single-writer
// workload.
type DB struct {
	sql   *sql.DB
	clock func() time.Time
}

// now returns the current time through the DB's clock, which tests may override.
func (d *DB) now() time.Time {
	if d.clock != nil {
		return d.clock().UTC()
	}
	return time.Now().UTC()
}

// SetClock overrides the time source, for deterministic tests.
func (d *DB) SetClock(fn func() time.Time) { d.clock = fn }

// Open opens (creating if necessary) the SQLite database at path and applies all
// pending migrations. The DSN enables WAL for concurrent readers alongside the
// single writer, a busy timeout so brief lock contention retries instead of
// failing, and foreign-key enforcement.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// The broker is a single writer; more than one open write connection on the
	// same file invites lock churn. Readers still benefit from WAL.
	sqlDB.SetMaxOpenConns(1)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	db := &DB{sql: sqlDB}
	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Close releases the database.
func (d *DB) Close() error { return d.sql.Close() }

// migrate applies every embedded migration not yet recorded, in filename order.
// It is idempotent: re-running against an up-to-date database is a no-op, and
// running against a partially migrated database applies only the remainder.
func (d *DB) migrate(ctx context.Context) error {
	if _, err := d.sql.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name       TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists int
		if err := d.sql.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM schema_migrations WHERE name = ?`, name).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if exists > 0 {
			continue
		}
		body, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		// Each migration runs in its own transaction with its bookkeeping row, so
		// a failure leaves the database at the last fully applied migration.
		tx, err := d.sql.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`,
			name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}
