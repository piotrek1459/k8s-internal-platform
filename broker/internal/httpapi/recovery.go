package httpapi

import (
	"net/http"
)

// handleRecover retries the existing blocked operation; no new intent is accepted.
func (r *Router) handleRecover(w http.ResponseWriter, req *http.Request) {
	var body lifecycleBody
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, requestID(req), err)
		return
	}
	result, err := r.cfg.Store.RetryRecovery(req.Context(), req.PathValue("id"), body.Version)
	if err != nil {
		writeError(w, r.cfg.Logger, requestID(req), err)
		return
	}
	if r.cfg.Enqueuer != nil {
		r.cfg.Enqueuer.Wake()
	}
	w.Header().Set("Location", "/api/v1/operations/"+result.OperationID)
	writeJSON(w, r.cfg.Logger, http.StatusAccepted, acceptResult(result))
}
