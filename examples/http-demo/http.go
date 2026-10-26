package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type identityResponse struct {
	Version  string `json:"version"`
	Hostname string `json:"hostname"`
}

type healthResponse struct {
	Status string `json:"status"`
}

func newHandler(cfg config, hostname string, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	identity := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, identityResponse{Version: cfg.version, Hostname: hostname})
	}
	mux.HandleFunc("GET /{$}", identity)
	mux.HandleFunc("GET /version", identity)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if cfg.readinessFail {
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ready"})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		mux.ServeHTTP(recorder, r)
		logger.Info("http_request", "method", r.Method, "path", r.URL.Path,
			"status", recorder.status, "duration_ms", time.Since(started).Milliseconds())
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// Responses contain fixed scalar fields; the only possible encoding failure
	// is a disconnected client, for which another response cannot be sent.
	_ = json.NewEncoder(w).Encode(body)
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (w *responseRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
