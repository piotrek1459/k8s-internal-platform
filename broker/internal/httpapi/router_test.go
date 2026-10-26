package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s-internal-platform/broker/internal/platform"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

type stubPinger struct{ err error }

func (p stubPinger) Ping(context.Context) error { return p.err }

func TestHealthz(t *testing.T) {
	router := NewRouter(Config{Logger: discardLogger()})
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status field = %q, want ok", body["status"])
	}
}

func TestReadyzReportsClusterConnectivitySeparately(t *testing.T) {
	tests := []struct {
		name        string
		cluster     ClusterPinger
		wantCluster string
	}{
		{name: "no pinger", cluster: nil, wantCluster: "unknown"},
		{name: "reachable", cluster: stubPinger{err: nil}, wantCluster: "available"},
		{name: "unreachable", cluster: stubPinger{err: errors.New("dial tcp: timeout")}, wantCluster: "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(Config{Logger: discardLogger(), Cluster: tt.cluster})
			rec := httptest.NewRecorder()
			router.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			// Readiness stays 200 even when the cluster is unavailable: the API must
			// remain usable to inspect and recover applications.
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body["cluster"] != tt.wantCluster {
				t.Fatalf("cluster = %q, want %q", body["cluster"], tt.wantCluster)
			}
		})
	}
}

func TestWriteErrorMapsCodesToStatuses(t *testing.T) {
	tests := []struct {
		code       platform.ErrorCode
		wantStatus int
	}{
		{platform.CodeInvalidInput, http.StatusBadRequest},
		{platform.CodeUnauthenticated, http.StatusUnauthorized},
		{platform.CodeForbidden, http.StatusForbidden},
		{platform.CodeNotFound, http.StatusNotFound},
		{platform.CodeConflict, http.StatusConflict},
		{platform.CodeIdempotencyConflict, http.StatusConflict},
		{platform.CodeVersionConflict, http.StatusPreconditionFailed},
		{platform.CodeUnsupportedSpec, http.StatusUnprocessableEntity},
		{platform.CodeImageArchUnsupported, http.StatusUnprocessableEntity},
		{platform.CodeRegistryUnavailable, http.StatusServiceUnavailable},
		{platform.CodeInternal, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeError(rec, discardLogger(), "req_test", &platform.Error{Code: tt.code, Message: "x"})
			if rec.Code != tt.wantStatus {
				t.Fatalf("status for %s = %d, want %d", tt.code, rec.Code, tt.wantStatus)
			}
			var body errorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Error.Code != tt.code || body.Error.RequestID != "req_test" {
				t.Fatalf("envelope = %+v", body.Error)
			}
		})
	}
}

func TestWriteErrorHidesNonDomainErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, discardLogger(), "req_1", errors.New("sensitive internal detail"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != platform.CodeInternal {
		t.Fatalf("code = %q, want INTERNAL", body.Error.Code)
	}
	if got := rec.Body.String(); strings.Contains(got, "sensitive internal detail") {
		t.Fatal("internal error detail leaked into the response body")
	}
}
