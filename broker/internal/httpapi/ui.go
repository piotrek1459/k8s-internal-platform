package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// ui_dist is populated by scripts/build-ui.sh (cp web/build/* broker/internal/httpapi/ui_dist/).
// When the directory contains only the README placeholder (dev mode without a UI build),
// the SPA handler is not registered and /api/* routes still work normally.

//go:embed all:ui_dist
var uiFS embed.FS

// registerUI mounts the SPA under "/" if built assets are present in ui_dist.
// API paths (/api/, /healthz, /readyz) are registered on the mux before this
// call, so the more-specific patterns win and the SPA catch-all never intercepts them.
func (r *Router) registerUI() {
	sub, err := fs.Sub(uiFS, "ui_dist")
	if err != nil {
		return
	}
	// Check whether a real build exists (not just the README placeholder).
	if _, err := sub.Open("index.html"); err != nil {
		r.cfg.Logger.Info("ui_dist has no index.html — UI not served (run scripts/build-ui.sh)")
		return
	}
	fileServer := http.FileServer(http.FS(sub))
	r.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		// If the file exists, serve it directly (JS/CSS/assets).
		// Otherwise fall back to index.html so SvelteKit client-side routing works.
		if req.URL.Path != "/" {
			f, openErr := sub.Open(strings.TrimPrefix(req.URL.Path, "/"))
			if openErr == nil {
				f.Close()
				fileServer.ServeHTTP(w, req)
				return
			}
		}
		// Serve index.html for all unknown paths (SPA history fallback).
		req.URL.Path = "/"
		fileServer.ServeHTTP(w, req)
	})
	r.cfg.Logger.Info("ui served from embedded assets")
}
