package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"

	"k8s-internal-platform/broker/internal/platform"
)

func (r *Router) registerDiagnostics() {
	r.mux.HandleFunc("GET /api/v1/apps/{id}/logs", r.protect(r.handleLogs))
	r.mux.HandleFunc("GET /api/v1/apps/{id}/events", r.protect(r.handleWorkloadEvents))
	r.mux.HandleFunc("GET /api/v1/apps/{id}/drift", r.protect(r.handleDrift))
}

func queryNumber(req *http.Request, key string, fallback, minValue, maxValue int64) (int64, error) {
	value := req.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < minValue || n > maxValue {
		return 0, platform.FieldErrorf(platform.CodeInvalidInput, key, "query limit is outside the supported range")
	}
	return n, nil
}

func diagnosticQuery(req *http.Request, allowed ...string) error {
	for key, values := range req.URL.Query() {
		ok := false
		for _, candidate := range allowed {
			if candidate == key {
				ok = true
			}
		}
		if !ok || len(values) != 1 {
			return platform.Errorf(platform.CodeInvalidInput, "unsupported or repeated diagnostic query parameter")
		}
	}
	return nil
}

func (r *Router) handleLogs(w http.ResponseWriter, req *http.Request) {
	if err := diagnosticQuery(req, "pod", "tailLines", "sinceSeconds", "previous"); err != nil {
		writeError(w, r.cfg.Logger, requestID(req), err)
		return
	}
	options := platform.LogOptions{Pod: req.URL.Query().Get("pod")}
	if options.Pod != "" && len(validation.IsDNS1123Subdomain(options.Pod)) > 0 {
		writeError(w, r.cfg.Logger, requestID(req), platform.FieldErrorf(platform.CodeInvalidInput, "pod", "invalid Pod name"))
		return
	}
	var err error
	options.TailLines, err = queryNumber(req, "tailLines", 100, 1, platform.MaxLogLines)
	if err != nil {
		writeError(w, r.cfg.Logger, requestID(req), err)
		return
	}
	options.SinceSeconds, err = queryNumber(req, "sinceSeconds", 3600, 1, 86400)
	if err != nil {
		writeError(w, r.cfg.Logger, requestID(req), err)
		return
	}
	if value := req.URL.Query().Get("previous"); value != "" {
		options.Previous, err = strconv.ParseBool(value)
		if err != nil {
			writeError(w, r.cfg.Logger, requestID(req), platform.FieldErrorf(platform.CodeInvalidInput, "previous", "expected a boolean"))
			return
		}
	}
	r.diagnosticResponse(w, req, func(ctx context.Context) (any, error) {
		return r.cfg.Diagnostics.Logs(ctx, req.PathValue("id"), options)
	})
}

func (r *Router) handleWorkloadEvents(w http.ResponseWriter, req *http.Request) {
	if err := diagnosticQuery(req, "limit"); err != nil {
		writeError(w, r.cfg.Logger, requestID(req), err)
		return
	}
	limit, err := queryNumber(req, "limit", 50, 1, platform.MaxDiagnosticEvents)
	if err != nil {
		writeError(w, r.cfg.Logger, requestID(req), err)
		return
	}
	r.diagnosticResponse(w, req, func(ctx context.Context) (any, error) {
		return r.cfg.Diagnostics.Events(ctx, req.PathValue("id"), int(limit))
	})
}

func (r *Router) handleDrift(w http.ResponseWriter, req *http.Request) {
	if err := diagnosticQuery(req); err != nil {
		writeError(w, r.cfg.Logger, requestID(req), err)
		return
	}
	r.diagnosticResponse(w, req, func(ctx context.Context) (any, error) { return r.cfg.Diagnostics.Drift(ctx, req.PathValue("id")) })
}

func (r *Router) diagnosticResponse(w http.ResponseWriter, req *http.Request, read func(context.Context) (any, error)) {
	if r.cfg.Diagnostics == nil {
		writeError(w, r.cfg.Logger, requestID(req), platform.Errorf(platform.CodeDependencyUnavailable, "application diagnostics are unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	result, err := read(ctx)
	if err != nil {
		writeError(w, r.cfg.Logger, requestID(req), err)
		return
	}
	writeJSON(w, r.cfg.Logger, http.StatusOK, result)
}
