package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// requestIDKey is the context key under which the per-request ID is stored.
type ctxKey int

const (
	requestIDKey ctxKey = iota
	// actorKey holds the authenticated actor identity, set by the auth middleware
	// in Step 6. Until then handlers fall back to the single administrator.
	actorKey
)

// withRequestID attaches a generated request ID to every request so handlers and
// error responses can correlate logs with client-visible identifiers.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := "req_" + randomHex(8)
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestID returns the request ID attached by withRequestID, or "" if absent.
func requestID(r *http.Request) string {
	if v, ok := r.Context().Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("httpapi: cannot read random bytes: " + err.Error())
	}
	return hex.EncodeToString(b)
}
