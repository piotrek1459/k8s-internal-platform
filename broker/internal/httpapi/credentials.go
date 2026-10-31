package httpapi

import (
	"net/http"

	"k8s-internal-platform/broker/internal/platform"
)

func (r *Router) registerCredentials() {
	r.mux.HandleFunc("GET /api/v1/registry-credentials", r.protect(r.handleListCredentials))
	r.mux.HandleFunc("POST /api/v1/registry-credentials", r.protect(r.handleCreateCredential))
	r.mux.HandleFunc("DELETE /api/v1/registry-credentials/{id}", r.protect(r.handleDeleteCredential))
}

func (r *Router) handleListCredentials(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	creds, err := r.cfg.Store.ListRegistryCredentials(req.Context())
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	items := make([]map[string]any, 0, len(creds))
	for _, c := range creds {
		items = append(items, map[string]any{
			"id":        c.ID,
			"label":     c.Label,
			"registry":  c.Registry,
			"username":  c.Username,
			"createdAt": c.CreatedAt,
		})
	}
	writeJSON(w, r.cfg.Logger, http.StatusOK, map[string]any{"items": items})
}

type createCredentialBody struct {
	Label    string `json:"label"`
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r *Router) handleCreateCredential(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	var body createCredentialBody
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if body.Label == "" || body.Registry == "" || body.Username == "" || body.Password == "" {
		writeError(w, r.cfg.Logger, reqID, platform.Errorf(platform.CodeInvalidInput,
			"label, registry, username and password are required"))
		return
	}
	id := r.cfg.Factory.IDs.NewID("cred")
	cred := platform.RegistryCredential{
		ID:                 id,
		Label:              body.Label,
		Registry:           body.Registry,
		Username:           body.Username,
		K8sSecretName:      "",
		VerificationStatus: "unverified",
		CreatedAt:          r.cfg.Factory.Clock.Now(),
	}
	if err := r.cfg.Store.CreateRegistryCredential(req.Context(), cred); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	writeJSON(w, r.cfg.Logger, http.StatusCreated, map[string]any{
		"id":        cred.ID,
		"label":     cred.Label,
		"registry":  cred.Registry,
		"username":  cred.Username,
		"createdAt": cred.CreatedAt,
	})
}

func (r *Router) handleDeleteCredential(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	id := req.PathValue("id")
	if err := r.cfg.Store.DeleteRegistryCredential(req.Context(), id); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

