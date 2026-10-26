package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/platform"
)

// maxBodyBytes bounds request bodies so a client cannot exhaust memory.
const maxBodyBytes = 1 << 20 // 1 MiB

func (r *Router) registerApplications() {
	r.mux.HandleFunc("POST /api/v1/apps", r.handleCreateApp)
	r.mux.HandleFunc("GET /api/v1/apps/{id}", r.handleGetApp)
	r.mux.HandleFunc("GET /api/v1/operations/{id}", r.handleGetOperation)
}

// createAppBody mirrors the CreateApplicationRequest schema in api/openapi.yaml.
type createAppBody struct {
	Name                 string                      `json:"name"`
	Image                string                      `json:"image"`
	RegistryCredentialID string                      `json:"registryCredentialId"`
	ContainerPort        int                         `json:"containerPort"`
	Profile              string                      `json:"profile"`
	Replicas             *int                        `json:"replicas"`
	Environment          map[string]string           `json:"environment"`
	SecretRefs           []secretRefBody             `json:"secretRefs"`
	Resources            *resourceBody               `json:"resources"`
	Health               *healthBody                 `json:"health"`
	Exposure             exposureBody                `json:"exposure"`
	Storage              json.RawMessage             `json:"storage"`
}

type secretRefBody struct {
	EnvironmentVariable string `json:"environmentVariable"`
	SecretID            string `json:"secretId"`
	Version             int    `json:"version"`
}

type resourceBody struct {
	CPURequest    string `json:"cpuRequest"`
	CPULimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
}

type healthBody struct {
	ReadinessPath string `json:"readinessPath"`
}

type exposureBody struct {
	Mode     string `json:"mode"`
	Hostname string `json:"hostname"`
}

func (r *Router) handleCreateApp(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)

	idempotencyKey := req.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeInvalidInput,
			"Idempotency-Key", "the Idempotency-Key header is required for this mutation"))
		return
	}

	raw, err := io.ReadAll(io.LimitReader(req.Body, maxBodyBytes))
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, platform.Errorf(platform.CodeInvalidInput, "could not read request body"))
		return
	}

	var body createAppBody
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, r.cfg.Logger, reqID, platform.Errorf(platform.CodeInvalidInput, "invalid JSON: %v", err))
		return
	}
	// Storage is reserved for M5; reject any non-null value early.
	if len(body.Storage) > 0 && string(body.Storage) != "null" {
		writeError(w, r.cfg.Logger, reqID, platform.FieldErrorf(platform.CodeUnsupportedSpec,
			"storage", "persistent storage is not available in the core prototype"))
		return
	}

	spec, err := applications.ValidateCreate(toCreateInput(body))
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}

	// The request hash is computed over the canonical decoded body so a byte-level
	// difference in whitespace does not defeat idempotent replay, while a genuine
	// content change is detected as an idempotency conflict.
	hash := canonicalHash(body)
	acceptReq := r.cfg.Factory.NewCreateRequest(body.Name, spec, idempotencyKey, hash, actorFrom(req), reqID)

	res, err := r.cfg.Store.AcceptOperation(req.Context(), acceptReq)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	if r.cfg.Enqueuer != nil {
		r.cfg.Enqueuer.Wake()
	}

	w.Header().Set("Location", "/api/v1/operations/"+res.OperationID)
	writeJSON(w, r.cfg.Logger, http.StatusAccepted, map[string]any{
		"applicationId": res.ApplicationID,
		"operationId":   res.OperationID,
		"state":         res.State,
		"deduplicated":  res.Deduplicated,
	})
}

func (r *Router) handleGetApp(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	app, err := r.cfg.Store.GetApplication(req.Context(), req.PathValue("id"))
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	status, _ := r.cfg.Store.GetObservedStatus(req.Context(), app.ID) // zero value when none yet

	w.Header().Set("ETag", etag(app.Version))
	writeJSON(w, r.cfg.Logger, http.StatusOK, map[string]any{
		"id":                     app.ID,
		"name":                   app.Name,
		"version":                app.Version,
		"desiredRevision":        app.DesiredRevisionID,
		"lastSuccessfulRevision": app.LastSuccessfulRevision,
		"activeOperationId":      app.ActiveOperationID,
		"status":                 status,
	})
}

func (r *Router) handleGetOperation(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	op, err := r.cfg.Store.GetOperation(req.Context(), req.PathValue("id"))
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	events, err := r.cfg.Store.ListOperationEvents(req.Context(), op.ID)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	rendered := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		rendered = append(rendered, map[string]any{
			"at": ev.At, "stage": ev.Stage, "message": ev.Message,
		})
	}
	writeJSON(w, r.cfg.Logger, http.StatusOK, map[string]any{
		"id":              op.ID,
		"applicationId":   op.AppID,
		"type":            op.Type,
		"state":           op.State,
		"stage":           op.Stage,
		"recoveryOutcome": op.RecoveryOutcome,
		"events":          rendered,
	})
}

func toCreateInput(b createAppBody) applications.CreateInput {
	in := applications.CreateInput{
		Name:                 b.Name,
		Image:                b.Image,
		RegistryCredentialID: b.RegistryCredentialID,
		ContainerPort:        b.ContainerPort,
		Profile:              b.Profile,
		Replicas:             b.Replicas,
		Environment:          b.Environment,
		Exposure:             platform.Exposure{Mode: platform.ExposureMode(b.Exposure.Mode), Hostname: b.Exposure.Hostname},
	}
	for _, s := range b.SecretRefs {
		in.SecretRefs = append(in.SecretRefs, platform.SecretRef{
			EnvironmentVariable: s.EnvironmentVariable, SecretID: s.SecretID, Version: s.Version,
		})
	}
	if b.Resources != nil {
		in.Resources = platform.ResourceRequirements{
			CPURequest: b.Resources.CPURequest, CPULimit: b.Resources.CPULimit,
			MemoryRequest: b.Resources.MemoryRequest, MemoryLimit: b.Resources.MemoryLimit,
		}
	}
	if b.Health != nil {
		in.ReadinessPath = b.Health.ReadinessPath
	}
	return in
}

// canonicalHash hashes the semantic content of a create body so idempotent
// replays with identical intent match regardless of JSON formatting, while a
// changed field produces a different hash (reported as an idempotency conflict).
func canonicalHash(b createAppBody) string {
	canonical, _ := json.Marshal(b) // struct field order is stable
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func etag(version int64) string { return `"` + strconv.FormatInt(version, 10) + `"` }

// actorFrom returns the authenticated actor. Until auth (Step 6) wires identity
// into the context, the single administrator is the implied actor.
func actorFrom(req *http.Request) string {
	if v, ok := req.Context().Value(actorKey).(string); ok && v != "" {
		return v
	}
	return "administrator"
}
