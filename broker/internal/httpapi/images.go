package httpapi

import (
	"net/http"

	"k8s-internal-platform/broker/internal/platform"
)

func (r *Router) registerImages() {
	r.mux.HandleFunc("POST /api/v1/images/inspect", r.protect(r.handleInspectImage))
}

// handleInspectImage reports an image's digest, architectures, and a suggested
// port. It is advisory for the creation form; the worker re-resolves and freezes
// the digest before any cluster mutation, so this result is never the execution
// authority.
func (r *Router) handleInspectImage(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	if r.cfg.Inspector == nil {
		writeError(w, r.cfg.Logger, reqID, platform.Errorf(platform.CodeDependencyUnavailable,
			"image inspection is not configured"))
		return
	}
	var body struct {
		Image                string `json:"image"`
		RegistryCredentialID string `json:"registryCredentialId"`
	}
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if body.Image == "" {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput, "image",
			"an image reference is required"))
		return
	}
	report, err := r.cfg.Inspector.Inspect(req.Context(), body.Image, body.RegistryCredentialID)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	writeJSON(w, r.cfg.Logger, http.StatusOK, map[string]any{
		"digest":        report.Digest,
		"architectures": report.Architectures,
		"suggestedPort": report.SuggestedPort,
	})
}
