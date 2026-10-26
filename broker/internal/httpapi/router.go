package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/platform"
)

// ClusterPinger reports whether the Kubernetes API is reachable. Readiness
// reports this separately from process health, per docs/api-and-data.md: the API
// process can be up and serving while the cluster is temporarily unreachable.
type ClusterPinger interface {
	Ping(ctx context.Context) error
}

// Enqueuer wakes the operation worker after new work is accepted. Persistence is
// the source of truth; this is only a latency optimization, so a nil Enqueuer is
// valid (the worker's ticker will pick the work up).
type Enqueuer interface {
	Wake()
}

// Config wires the HTTP layer's dependencies. The accept path (Step 5) needs the
// Store and the applications Factory; later steps add auth and more handlers.
type Config struct {
	Logger   *slog.Logger
	Cluster  ClusterPinger // optional; nil means cluster connectivity is reported unknown
	Store    platform.Store
	Factory  applications.Factory
	Enqueuer Enqueuer // optional
}

// Router builds the HTTP handler with all registered routes.
type Router struct {
	cfg Config
	mux *http.ServeMux
}

// NewRouter constructs the API router. Health and readiness probes are registered
// here; management routes are added by later steps.
func NewRouter(cfg Config) *Router {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	r := &Router{cfg: cfg, mux: http.NewServeMux()}
	r.registerHealth()
	r.registerApplications()
	return r
}

// Handler returns the composed http.Handler with request-ID correlation applied.
func (r *Router) Handler() http.Handler { return withRequestID(r.mux) }

func (r *Router) registerHealth() {
	// Liveness: the process is running. No sensitive information, no cluster call.
	r.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, r.cfg.Logger, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Readiness: the API can serve requests. Cluster connectivity is reported as a
	// separate field and does not by itself fail readiness, so the API stays
	// available to inspect and recover applications while the cluster is degraded.
	r.mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, req *http.Request) {
		cluster := "unknown"
		if r.cfg.Cluster != nil {
			if err := r.cfg.Cluster.Ping(req.Context()); err != nil {
				cluster = "unavailable"
			} else {
				cluster = "available"
			}
		}
		writeJSON(w, r.cfg.Logger, http.StatusOK, map[string]string{
			"status":  "ready",
			"cluster": cluster,
		})
	})
}
