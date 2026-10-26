// Package audit builds sanitized audit records. Audit data answers who requested
// what and when (docs/api-and-data.md); it must never contain secret values, so
// configuration diffs are redacted here before they reach the store.
package audit

import (
	"sort"
	"strings"
)

// redacted is the placeholder substituted for any value that may be sensitive.
const redacted = "<redacted>"

// sensitiveKeys are configuration field names whose values are never recorded in
// an audit diff, regardless of where they appear.
var sensitiveKeys = map[string]bool{
	"password": true,
	"secret":   true,
	"token":    true,
	"data":     true,
}

// Diff renders a stable, human-readable summary of changed top-level fields with
// sensitive values redacted. Keys are sorted so the output is deterministic for
// tests and for comparing audit entries. Values are never included for sensitive
// keys; for others only a changed/added/removed marker and the new value (for
// non-sensitive scalars) are shown.
func Diff(before, after map[string]string) string {
	keys := map[string]bool{}
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)

	var b strings.Builder
	for _, k := range ordered {
		bv, hadBefore := before[k]
		av, hasAfter := after[k]
		if hadBefore && hasAfter && bv == av {
			continue // unchanged
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		switch {
		case !hadBefore && hasAfter:
			b.WriteString(k + " added=" + display(k, av))
		case hadBefore && !hasAfter:
			b.WriteString(k + " removed")
		default:
			b.WriteString(k + " changed=" + display(k, av))
		}
	}
	return b.String()
}

// display returns a value for audit, redacting anything whose key is sensitive.
func display(key, value string) string {
	if isSensitive(key) {
		return redacted
	}
	return value
}

func isSensitive(key string) bool {
	lower := strings.ToLower(key)
	if sensitiveKeys[lower] {
		return true
	}
	for s := range sensitiveKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
