package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/platform"
	"k8s-internal-platform/broker/internal/store"
)

// fakeSecretManager records created versions and returns deterministic names.
type fakeSecretManager struct{ created int }

func (f *fakeSecretManager) CreateImmutableSecret(_ context.Context, secretID string, version int, _ map[string][]byte) (string, string, error) {
	f.created++
	return fmt.Sprintf("k8s-%s-v%d", secretID, version), fmt.Sprintf("uid-%d", f.created), nil
}

func newSecretsRouter(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	db, err := store.Open(context.Background(), t.TempDir()+"/platform.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	h := NewRouter(Config{
		Logger:  discardLogger(),
		Store:   st,
		Factory: applications.Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}},
		Secrets: &fakeSecretManager{},
	}).Handler()
	return h, st
}

func postJSON(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("X-Requested-With", "fetch")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateSecretStoresMetadataOnly(t *testing.T) {
	h, _ := newSecretsRouter(t)
	rec := postJSON(h, "/api/v1/secrets", `{"name":"db-password","data":{"value":"s3cr3t"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	// The plaintext value must never appear in the response: only metadata (keys).
	if strings.Contains(rec.Body.String(), "s3cr3t") {
		t.Fatalf("secret value leaked into response: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"value"`) { // the key name is fine
		t.Fatalf("expected key metadata in response: %s", rec.Body.String())
	}
}

func TestListSecretsNeverReturnsValues(t *testing.T) {
	h, _ := newSecretsRouter(t)
	if rec := postJSON(h, "/api/v1/secrets", `{"name":"s","data":{"value":"top-secret"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "top-secret") {
		t.Fatalf("secret value leaked in list: %s", rec.Body.String())
	}
}

func TestAddSecretVersionIncrements(t *testing.T) {
	h, _ := newSecretsRouter(t)
	rec := postJSON(h, "/api/v1/secrets", `{"name":"s","data":{"value":"v1"}}`)
	id := extractSecretID(t, rec)

	rec = postJSON(h, "/api/v1/secrets/"+id+"/versions", `{"data":{"value":"v2"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add version status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"version":2`) {
		t.Fatalf("expected version 2: %s", rec.Body.String())
	}
}

func TestDeleteSecretRejectedWhileReferenced(t *testing.T) {
	h, st := newSecretsRouter(t)
	rec := postJSON(h, "/api/v1/secrets", `{"name":"s","data":{"value":"v1"}}`)
	id := extractSecretID(t, rec)

	// Simulate an active application revision that references the secret version by
	// freezing a revision whose frozen secret-versions JSON names the secret ID.
	seedFrozenRevisionReferencing(t, st, id)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+id, nil)
	req.Header.Set("X-Requested-With", "fetch")
	del := httptest.NewRecorder()
	h.ServeHTTP(del, req)
	if del.Code != http.StatusConflict {
		t.Fatalf("delete status = %d, want 409 (secret in use); body=%s", del.Code, del.Body.String())
	}
}

func extractSecretID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusCreated {
		t.Fatalf("create secret: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.ID
}

// seedFrozenRevisionReferencing creates an application and a frozen revision whose
// recorded secret versions reference secretID, exercising the delete-protection
// path in the store.
func seedFrozenRevisionReferencing(t *testing.T, st *store.Store, secretID string) {
	t.Helper()
	ctx := context.Background()
	_, err := st.AcceptOperation(ctx, platform.AcceptRequest{
		Type:        platform.OperationCreate,
		Application: platform.Application{ID: "app_ref", Name: "refapp"},
		IsNewApp:    true,
		Revision:    platform.Revision{ID: "rev_ref", AppID: "app_ref", Number: 1, Result: platform.RevisionPending, Actor: "t"},
		Op:          platform.Operation{ID: "op_ref", AppID: "app_ref", Type: platform.OperationCreate, TargetRevision: "rev_ref"},
		RequestHash: "h",
	})
	if err != nil {
		t.Fatalf("seed accept: %v", err)
	}
	// Freeze with a secret reference so the revision counts as referencing it.
	err = st.FreezeExecutionSpec(ctx, "rev_ref", platform.FrozenSpec{
		ImageDigest:    "img@sha256:a",
		SecretVersions: []platform.SecretRef{{EnvironmentVariable: "V", SecretID: secretID, Version: 1}},
	})
	if err != nil {
		t.Fatalf("seed freeze: %v", err)
	}
	// Mark the revision succeeded so it is rollback-eligible (and thus protected).
	if err := st.MarkRevisionResult(ctx, "rev_ref", platform.RevisionSucceeded); err != nil {
		t.Fatalf("seed mark: %v", err)
	}
}
