package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s-internal-platform/broker/internal/platform"
)

type fakeDiagnostics struct {
	calls   int
	options platform.LogOptions
}

func (f *fakeDiagnostics) Logs(_ context.Context, id string, options platform.LogOptions) (platform.LogResult, error) {
	f.calls++
	f.options = options
	return platform.LogResult{Pod: "owned-pod", Text: "plain <script> text"}, nil
}
func (f *fakeDiagnostics) Events(context.Context, string, int) (platform.EventResult, error) {
	f.calls++
	return platform.EventResult{Items: []platform.WorkloadEvent{}}, nil
}
func (f *fakeDiagnostics) Drift(context.Context, string) (platform.DriftReport, error) {
	f.calls++
	return platform.DriftReport{State: "in_sync", Differences: []platform.DriftDifference{}}, nil
}

func TestDiagnosticQueriesAreBoundedAndCannotSelectNamespaceOrContainer(t *testing.T) {
	diagnostics := &fakeDiagnostics{}
	h := NewRouter(Config{Diagnostics: diagnostics, Logger: discardLogger()}).Handler()
	for _, path := range []string{
		"logs?namespace=kube-system", "logs?container=broker", "logs?follow=true", "logs?tailLines=501", "logs?tailLines=0", "logs?tailLines=not-a-number", "logs?sinceSeconds=86401", "logs?previous=invalid", "logs?pod=../../foreign", "logs?pod=a&pod=b", "events?limit=101", "events?limit=-1", "drift?namespace=other",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/apps/app_1/"+path, nil))
		if rec.Code != 400 {
			t.Fatalf("%s status=%d", path, rec.Code)
		}
	}
	if diagnostics.calls != 0 {
		t.Fatal("invalid query reached diagnostics backend")
	}
	for _, path := range []string{"logs", "events?limit=100", "drift"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/apps/app_1/"+path, nil))
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("valid query %s status=%d", path, rec.Code)
		}
	}
	if diagnostics.options.TailLines != 100 || diagnostics.options.SinceSeconds != 3600 {
		t.Fatal("log defaults are not bounded")
	}
}

func TestDiagnosticsAreAuthenticatedAndUnavailableWhenUnconfigured(t *testing.T) {
	diagnostics := &fakeDiagnostics{}
	// Use the normal auth gate: an absent cookie must be rejected before reads.
	h := newAuthRouter(t, "diagnostic-bootstrap")
	for _, path := range []string{"logs", "events", "drift"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/apps/app_1/"+path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s status=%d", path, rec.Code)
		}
	}
	if diagnostics.calls != 0 {
		t.Fatal("unauthenticated read reached diagnostics")
	}
	h = NewRouter(Config{Logger: discardLogger()}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/apps/app_1/logs", nil))
	if rec.Code != 503 {
		t.Fatalf("unconfigured status=%d", rec.Code)
	}
}
