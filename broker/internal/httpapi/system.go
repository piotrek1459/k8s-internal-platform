package httpapi

import (
	"net/http"
	"runtime"
)

func (r *Router) registerSystem() {
	r.mux.HandleFunc("GET /api/v1/system", r.protect(r.handleSystem))
}

// handleSystem reports version, host architecture, and cluster reachability. It
// is the minimal M2 surface of the richer system summary; capabilities and
// capacity reporting expand in later milestones.
func (r *Router) handleSystem(w http.ResponseWriter, req *http.Request) {
	clusterReachable := false
	if r.cfg.Cluster != nil && r.cfg.Cluster.Ping(req.Context()) == nil {
		clusterReachable = true
	}
	version := r.cfg.Version
	if version == "" {
		version = "dev"
	}
	response := map[string]any{
		"version":          version,
		"architecture":     runtime.GOARCH,
		"clusterReachable": clusterReachable,
		"capabilities":     capabilities(r.cfg),
	}
	if r.cfg.PrivateNetwork.ApplicationDomain != "" {
		response["privateNetwork"] = r.cfg.PrivateNetwork
	}
	writeJSON(w, r.cfg.Logger, http.StatusOK, response)
}

// capabilities lists the optional features this installation has configured, so a
// client can adapt (for example, hide image inspection when it is unavailable).
func capabilities(cfg Config) []string {
	caps := []string{}
	if cfg.PrivateIngressEnabled {
		caps = append(caps, "private-ingress")
	}
	if cfg.Inspector != nil {
		caps = append(caps, "image-inspection")
	}
	if cfg.Secrets != nil {
		caps = append(caps, "configuration-secrets")
	}
	if cfg.Credentials != nil {
		caps = append(caps, "registry-credentials")
	}
	if cfg.Cluster != nil {
		caps = append(caps, "cluster-observation")
	}
	if cfg.Diagnostics != nil {
		caps = append(caps, "workload-diagnostics", "drift-inspection")
	}
	return caps
}
