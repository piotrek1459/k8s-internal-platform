package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"k8s-internal-platform/broker/internal/platform"
)

// Store implements platform.Store on top of DB. It is the only type that issues
// SQL; higher layers depend on the platform.Store interface.
type Store struct {
	db *DB
}

// New returns a Store backed by db. db must already be migrated (Open does this).
func New(db *DB) *Store { return &Store{db: db} }

// Verify at compile time that Store satisfies the port.
var _ platform.Store = (*Store)(nil)

const rfc3339 = time.RFC3339Nano

func formatTime(t time.Time) string { return t.UTC().Format(rfc3339) }

func parseTime(s string) time.Time {
	t, err := time.Parse(rfc3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func parseTimePtr(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

// notFound maps sql.ErrNoRows to a domain NOT_FOUND error with a useful message.
func notFound(err error, what, id string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return platform.Errorf(platform.CodeNotFound, "%s %q not found", what, id)
	}
	return err
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// The inputs are plain domain structs with no unsupported types; a failure
		// indicates a programming error, not runtime data.
		panic("store: marshal: " + err.Error())
	}
	return string(b)
}
