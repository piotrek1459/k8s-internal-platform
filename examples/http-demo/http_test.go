package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIdentityAndHealth(t *testing.T) {
	for _, unhealthy := range []bool{false, true} {
		t.Run(map[bool]string{false: "healthy", true: "unready"}[unhealthy], func(t *testing.T) {
			handler := newHandler(config{version: "v3", readinessFail: unhealthy}, "demo-pod-123",
				slog.New(slog.NewJSONHandler(io.Discard, nil)))
			for _, path := range []string{"/", "/version"} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
					t.Fatalf("%s: status=%d, content-type=%s", path, response.Code, response.Header().Get("Content-Type"))
				}
				var body identityResponse
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Version != "v3" || body.Hostname != "demo-pod-123" {
					t.Fatalf("%s: identity=%+v", path, body)
				}
			}
			for _, path := range []string{"/healthz", "/readyz"} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				want := http.StatusOK
				if path == "/readyz" && unhealthy {
					want = http.StatusServiceUnavailable
				}
				if response.Code != want {
					t.Errorf("%s: status=%d, want %d", path, response.Code, want)
				}
			}
		})
	}
}

func TestUnknownRoutesAndUnsupportedMethods(t *testing.T) {
	handler := newHandler(config{version: "v1"}, "demo", slog.New(slog.NewJSONHandler(io.Discard, nil)))
	for _, tt := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, "/not-a-route", http.StatusNotFound},
		{http.MethodGet, "/readyz/extra", http.StatusNotFound},
		{http.MethodPost, "/version", http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(tt.method, tt.path, nil))
		if response.Code != tt.status {
			t.Errorf("%s %s: status=%d, want %d", tt.method, tt.path, response.Code, tt.status)
		}
	}
}

func TestRequestLogReportsFailureWithoutQueryValues(t *testing.T) {
	var logs bytes.Buffer
	handler := newHandler(config{version: "v3", readinessFail: true}, "demo",
		slog.New(slog.NewJSONHandler(&logs, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz?token=private-value", nil))
	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatalf("log is not JSON: %v", err)
	}
	if record["status"] != float64(http.StatusServiceUnavailable) || record["path"] != "/readyz" {
		t.Fatalf("unexpected request log: %v", record)
	}
	if bytes.Contains(logs.Bytes(), []byte("private-value")) {
		t.Fatal("query value was exposed in a request log")
	}
}
