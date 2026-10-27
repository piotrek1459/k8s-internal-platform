package audit

import (
	"strings"
	"testing"
)

func TestDiffRedactsSensitiveValues(t *testing.T) {
	before := map[string]string{"APP_ENV": "staging", "DB_PASSWORD": "old"}
	after := map[string]string{"APP_ENV": "production", "DB_PASSWORD": "new", "API_TOKEN": "t0ken"}
	got := Diff(before, after)

	if !strings.Contains(got, "APP_ENV changed=production") {
		t.Fatalf("non-sensitive change missing: %q", got)
	}
	// Sensitive values must be redacted regardless of add or change.
	if strings.Contains(got, "old") || strings.Contains(got, "new") || strings.Contains(got, "t0ken") {
		t.Fatalf("a sensitive value leaked into the diff: %q", got)
	}
	if !strings.Contains(got, "DB_PASSWORD changed=<redacted>") {
		t.Fatalf("expected redacted password change: %q", got)
	}
	if !strings.Contains(got, "API_TOKEN added=<redacted>") {
		t.Fatalf("expected redacted token addition: %q", got)
	}
}

func TestDiffIsDeterministicAndSkipsUnchanged(t *testing.T) {
	before := map[string]string{"B": "1", "A": "1", "C": "1"}
	after := map[string]string{"B": "1", "A": "2", "C": "1"}
	got := Diff(before, after)
	if got != "A changed=2" {
		t.Fatalf("diff = %q, want only the changed key A", got)
	}
}

func TestDiffReportsRemoval(t *testing.T) {
	got := Diff(map[string]string{"GONE": "x"}, map[string]string{})
	if got != "GONE removed" {
		t.Fatalf("diff = %q", got)
	}
}
