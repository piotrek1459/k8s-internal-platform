package httpapi

import (
	"net/http"

	"k8s-internal-platform/broker/internal/platform"
)

func (r *Router) registerSecrets() {
	r.mux.HandleFunc("GET /api/v1/secrets", r.protect(r.handleListSecrets))
	r.mux.HandleFunc("POST /api/v1/secrets", r.protect(r.handleCreateSecret))
	r.mux.HandleFunc("POST /api/v1/secrets/{id}/versions", r.protect(r.handleAddSecretVersion))
	r.mux.HandleFunc("DELETE /api/v1/secrets/{id}", r.protect(r.handleDeleteSecret))
}

type createSecretBody struct {
	Name string            `json:"name"`
	Data map[string]string `json:"data"`
}

type secretVersionBody struct {
	Data map[string]string `json:"data"`
}

func (r *Router) handleCreateSecret(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	var body createSecretBody
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if body.Name == "" || len(body.Data) == 0 {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput, "data",
			"a name and at least one data entry are required"))
		return
	}

	secretID := r.cfg.Factory.IDs.NewID("sec")
	// Version 1 is created as an immutable Kubernetes Secret first; only then is
	// the metadata recorded, so a dangling metadata row cannot reference a
	// nonexistent Secret.
	k8sName, uid, err := r.createSecretVersion(req, secretID, 1, body.Data)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	meta := platform.SecretMetadata{ID: secretID, Owner: actorFrom(req), Name: body.Name, CreatedAt: r.cfg.Factory.Clock.Now()}
	version := platform.SecretVersion{SecretID: secretID, Version: 1, Keys: keysOf(body.Data), K8sSecretName: k8sName, K8sSecretUID: uid, CreatedAt: r.cfg.Factory.Clock.Now()}
	if err := r.cfg.Store.CreateSecret(req.Context(), meta, version); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	writeJSON(w, r.cfg.Logger, http.StatusCreated, renderSecret(meta, []platform.SecretVersion{version}))
}

func (r *Router) handleAddSecretVersion(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	secretID := req.PathValue("id")
	var body secretVersionBody
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if len(body.Data) == 0 {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput, "data", "at least one data entry is required"))
		return
	}
	_, existing, err := r.cfg.Store.GetSecret(req.Context(), secretID)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	next := nextVersion(existing)
	k8sName, uid, err := r.createSecretVersion(req, secretID, next, body.Data)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	version := platform.SecretVersion{SecretID: secretID, Version: next, Keys: keysOf(body.Data), K8sSecretName: k8sName, K8sSecretUID: uid, CreatedAt: r.cfg.Factory.Clock.Now()}
	if err := r.cfg.Store.AddSecretVersion(req.Context(), version); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	writeJSON(w, r.cfg.Logger, http.StatusCreated, renderVersion(version))
}

func (r *Router) handleListSecrets(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	metas, err := r.cfg.Store.ListSecrets(req.Context())
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	items := make([]map[string]any, 0, len(metas))
	for _, m := range metas {
		_, versions, err := r.cfg.Store.GetSecret(req.Context(), m.ID)
		if err != nil {
			writeError(w, r.cfg.Logger, reqID, err)
			return
		}
		items = append(items, renderSecret(m, versions))
	}
	writeJSON(w, r.cfg.Logger, http.StatusOK, map[string]any{"items": items})
}

func (r *Router) handleDeleteSecret(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	if err := r.cfg.Store.DeleteSecret(req.Context(), req.PathValue("id")); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createSecretVersion writes the immutable Kubernetes Secret for a version. It
// requires a SecretManager; without one, secret creation is unavailable.
func (r *Router) createSecretVersion(req *http.Request, secretID string, version int, data map[string]string) (string, string, error) {
	if r.cfg.Secrets == nil {
		return "", "", platform.Errorf(platform.CodeDependencyUnavailable, "secret storage is not configured")
	}
	bytesData := make(map[string][]byte, len(data))
	for k, v := range data {
		bytesData[k] = []byte(v)
	}
	return r.cfg.Secrets.CreateImmutableSecret(req.Context(), secretID, version, bytesData)
}

func renderSecret(meta platform.SecretMetadata, versions []platform.SecretVersion) map[string]any {
	rendered := make([]map[string]any, 0, len(versions))
	for _, v := range versions {
		rendered = append(rendered, renderVersion(v))
	}
	return map[string]any{"id": meta.ID, "name": meta.Name, "versions": rendered}
}

func renderVersion(v platform.SecretVersion) map[string]any {
	return map[string]any{"version": v.Version, "keys": v.Keys, "createdAt": v.CreatedAt}
}

func keysOf(data map[string]string) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}

func nextVersion(existing []platform.SecretVersion) int {
	max := 0
	for _, v := range existing {
		if v.Version > max {
			max = v.Version
		}
	}
	return max + 1
}
