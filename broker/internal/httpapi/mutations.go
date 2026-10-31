package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/platform"
)

func (r *Router) registerApplicationMutations() {
	r.mux.HandleFunc("PATCH /api/v1/apps/{id}", r.protect(r.handleUpdateApp))
	r.mux.HandleFunc("DELETE /api/v1/apps/{id}", r.protect(r.handleDeleteApp))
	r.mux.HandleFunc("POST /api/v1/apps/{id}/restart", r.protect(r.handleRestartApp))
	r.mux.HandleFunc("POST /api/v1/apps/{id}/reapply", r.protect(r.handleReapplyApp))
	r.mux.HandleFunc("POST /api/v1/apps/{id}/rollback", r.protect(r.handleRollbackApp))
	r.mux.HandleFunc("GET /api/v1/apps/{id}/revisions", r.protect(r.handleListRevisions))
}

// --- PATCH /api/v1/apps/{id} ---

type updateAppBody struct {
	Image                *string           `json:"image"`
	RegistryCredentialID *string           `json:"registryCredentialId"`
	ContainerPort        *int              `json:"containerPort"`
	Replicas             *int              `json:"replicas"`
	Environment          map[string]string `json:"environment"`
	SecretRefs           []secretRefBody   `json:"secretRefs"`
	Resources            *resourceBody     `json:"resources"`
	Health               *healthBody       `json:"health"`
	Exposure             *exposureBody     `json:"exposure"`
	Version              int64             `json:"version"`
}

func (r *Router) handleUpdateApp(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	id := req.PathValue("id")

	idemKey := req.Header.Get("Idempotency-Key")
	if idemKey == "" {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput,
			"Idempotency-Key", "the Idempotency-Key header is required for this mutation"))
		return
	}

	var body updateAppBody
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	app, curRev, err := r.cfg.Store.GetApplicationWithRevision(req.Context(), id)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	in := toUpdateInput(body, curRev.Spec)
	newSpec, err := applications.ValidateUpdate(curRev.Spec, in)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	raw, _ := json.Marshal(body)
	hash := hashBytes(raw)
	acceptReq := r.cfg.Factory.NewUpdateRequest(app, newSpec, body.Version, idemKey, hash, actorFrom(req), reqID)

	res, err := r.cfg.Store.AcceptOperation(req.Context(), acceptReq)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if r.cfg.Enqueuer != nil {
		r.cfg.Enqueuer.Wake()
	}
	w.Header().Set("Location", "/api/v1/operations/"+res.OperationID)
	writeJSON(w, r.cfg.Logger, http.StatusAccepted, acceptResult(res))
}

func toUpdateInput(b updateAppBody, cur platform.RevisionSpec) applications.UpdateInput {
	in := applications.UpdateInput{
		Image:                b.Image,
		RegistryCredentialID: b.RegistryCredentialID,
		ContainerPort:        b.ContainerPort,
		Replicas:             b.Replicas,
	}
	if b.Environment != nil {
		in.Environment = b.Environment
	}
	if b.SecretRefs != nil {
		refs := make([]platform.SecretRef, len(b.SecretRefs))
		for i, s := range b.SecretRefs {
			refs[i] = platform.SecretRef{
				EnvironmentVariable: s.EnvironmentVariable,
				SecretID:            s.SecretID,
				Version:             s.Version,
			}
		}
		in.SecretRefs = refs
	}
	if b.Resources != nil {
		res := platform.ResourceRequirements{
			CPURequest: b.Resources.CPURequest, CPULimit: b.Resources.CPULimit,
			MemoryRequest: b.Resources.MemoryRequest, MemoryLimit: b.Resources.MemoryLimit,
		}
		in.Resources = &res
	}
	if b.Health != nil {
		in.ReadinessPath = &b.Health.ReadinessPath
	}
	if b.Exposure != nil {
		exp := platform.Exposure{Mode: platform.ExposureMode(b.Exposure.Mode), Hostname: b.Exposure.Hostname}
		in.Exposure = &exp
	}
	return in
}

// --- DELETE /api/v1/apps/{id} ---

type deleteAppBody struct {
	Version int64 `json:"version"`
}

func (r *Router) handleDeleteApp(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	id := req.PathValue("id")

	idemKey := req.Header.Get("Idempotency-Key")
	if idemKey == "" {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput,
			"Idempotency-Key", "the Idempotency-Key header is required for this mutation"))
		return
	}

	var body deleteAppBody
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	app, err := r.cfg.Store.GetApplication(req.Context(), id)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	raw, _ := json.Marshal(body)
	hash := hashBytes(raw)
	acceptReq := r.cfg.Factory.NewDeleteRequest(app, body.Version, idemKey, hash, actorFrom(req), reqID)

	res, err := r.cfg.Store.AcceptOperation(req.Context(), acceptReq)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if r.cfg.Enqueuer != nil {
		r.cfg.Enqueuer.Wake()
	}
	w.Header().Set("Location", "/api/v1/operations/"+res.OperationID)
	writeJSON(w, r.cfg.Logger, http.StatusAccepted, acceptResult(res))
}

// --- POST /api/v1/apps/{id}/restart ---

type lifecycleBody struct {
	Version int64 `json:"version"`
}

func (r *Router) handleRestartApp(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	id := req.PathValue("id")

	idemKey := req.Header.Get("Idempotency-Key")
	if idemKey == "" {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput,
			"Idempotency-Key", "the Idempotency-Key header is required for this mutation"))
		return
	}

	var body lifecycleBody
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	app, curRev, err := r.cfg.Store.GetApplicationWithRevision(req.Context(), id)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	raw, _ := json.Marshal(body)
	hash := hashBytes(raw)
	acceptReq := r.cfg.Factory.NewRestartRequest(app, curRev.Spec, body.Version, idemKey, hash, actorFrom(req), reqID)

	res, err := r.cfg.Store.AcceptOperation(req.Context(), acceptReq)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if r.cfg.Enqueuer != nil {
		r.cfg.Enqueuer.Wake()
	}
	w.Header().Set("Location", "/api/v1/operations/"+res.OperationID)
	writeJSON(w, r.cfg.Logger, http.StatusAccepted, acceptResult(res))
}

// --- POST /api/v1/apps/{id}/reapply ---

func (r *Router) handleReapplyApp(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	id := req.PathValue("id")

	idemKey := req.Header.Get("Idempotency-Key")
	if idemKey == "" {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput,
			"Idempotency-Key", "the Idempotency-Key header is required for this mutation"))
		return
	}

	var body lifecycleBody
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	app, curRev, err := r.cfg.Store.GetApplicationWithRevision(req.Context(), id)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	raw, _ := json.Marshal(body)
	hash := hashBytes(raw)
	acceptReq := r.cfg.Factory.NewReapplyRequest(app, curRev.Spec, body.Version, idemKey, hash, actorFrom(req), reqID)

	res, err := r.cfg.Store.AcceptOperation(req.Context(), acceptReq)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if r.cfg.Enqueuer != nil {
		r.cfg.Enqueuer.Wake()
	}
	w.Header().Set("Location", "/api/v1/operations/"+res.OperationID)
	writeJSON(w, r.cfg.Logger, http.StatusAccepted, acceptResult(res))
}

// --- POST /api/v1/apps/{id}/rollback ---

type rollbackBody struct {
	RevisionID string `json:"revisionId"`
	Version    int64  `json:"version"`
}

func (r *Router) handleRollbackApp(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	id := req.PathValue("id")

	idemKey := req.Header.Get("Idempotency-Key")
	if idemKey == "" {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput,
			"Idempotency-Key", "the Idempotency-Key header is required for this mutation"))
		return
	}

	var body rollbackBody
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if body.RevisionID == "" {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput,
			"revisionId", "revisionId is required"))
		return
	}

	app, err := r.cfg.Store.GetApplication(req.Context(), id)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	targetRev, err := r.cfg.Store.GetRevision(req.Context(), body.RevisionID)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if targetRev.AppID != app.ID {
		writeError(w, r.cfg.Logger, reqID, platform.Errorf(platform.CodeNotFound,
			"revision %q does not belong to application %q", body.RevisionID, id))
		return
	}
	if targetRev.Result != platform.RevisionSucceeded {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput,
			"revisionId", "only succeeded revisions are eligible for rollback"))
		return
	}

	raw, _ := json.Marshal(body)
	hash := hashBytes(raw)
	acceptReq := r.cfg.Factory.NewRollbackRequest(app, targetRev, body.Version, idemKey, hash, actorFrom(req), reqID)

	res, err := r.cfg.Store.AcceptOperation(req.Context(), acceptReq)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if r.cfg.Enqueuer != nil {
		r.cfg.Enqueuer.Wake()
	}
	w.Header().Set("Location", "/api/v1/operations/"+res.OperationID)
	writeJSON(w, r.cfg.Logger, http.StatusAccepted, acceptResult(res))
}

// --- GET /api/v1/apps/{id}/revisions ---

func (r *Router) handleListRevisions(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	id := req.PathValue("id")

	if _, err := r.cfg.Store.GetApplication(req.Context(), id); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	revs, err := r.cfg.Store.ListRevisions(req.Context(), id)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	items := make([]map[string]any, 0, len(revs))
	for _, rev := range revs {
		item := map[string]any{
			"id":        rev.ID,
			"number":    rev.Number,
			"result":    rev.Result,
			"createdAt": rev.Created,
			"spec": map[string]any{
				"image":    rev.Spec.Image,
				"replicas": rev.Spec.Replicas,
			},
		}
		if rev.Frozen != nil {
			item["frozenAt"] = rev.Frozen.FrozenAt
		}
		items = append(items, item)
	}
	writeJSON(w, r.cfg.Logger, http.StatusOK, map[string]any{"items": items})
}

// --- shared helpers ---

func acceptResult(res platform.AcceptResult) map[string]any {
	return map[string]any{
		"applicationId": res.ApplicationID,
		"operationId":   res.OperationID,
		"state":         res.State,
		"deduplicated":  res.Deduplicated,
	}
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
