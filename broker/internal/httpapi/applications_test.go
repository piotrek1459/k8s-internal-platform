package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/platform"
)

// fakeStore implements just enough of platform.Store for the accept-path handler
// tests. Unimplemented methods panic so an accidental dependency is caught.
type fakeStore struct {
	platform.Store // embedded nil: any unimplemented method panics if called

	accept   func(context.Context, platform.AcceptRequest) (platform.AcceptResult, error)
	apps     map[string]platform.Application
	ops      map[string]platform.Operation
	events   map[string][]platform.OperationEvent
	observed map[string]platform.ObservedStatus
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		apps:     map[string]platform.Application{},
		ops:      map[string]platform.Operation{},
		events:   map[string][]platform.OperationEvent{},
		observed: map[string]platform.ObservedStatus{},
	}
}

func (f *fakeStore) AcceptOperation(ctx context.Context, req platform.AcceptRequest) (platform.AcceptResult, error) {
	return f.accept(ctx, req)
}
func (f *fakeStore) GetApplication(_ context.Context, id string) (platform.Application, error) {
	app, ok := f.apps[id]
	if !ok {
		return platform.Application{}, platform.Errorf(platform.CodeNotFound, "application %q not found", id)
	}
	return app, nil
}
func (f *fakeStore) GetObservedStatus(_ context.Context, id string) (platform.ObservedStatus, error) {
	s, ok := f.observed[id]
	if !ok {
		return platform.ObservedStatus{}, platform.Errorf(platform.CodeNotFound, "no status")
	}
	return s, nil
}
func (f *fakeStore) GetOperation(_ context.Context, id string) (platform.Operation, error) {
	op, ok := f.ops[id]
	if !ok {
		return platform.Operation{}, platform.Errorf(platform.CodeNotFound, "operation %q not found", id)
	}
	return op, nil
}
func (f *fakeStore) ListOperationEvents(_ context.Context, id string) ([]platform.OperationEvent, error) {
	return f.events[id], nil
}
func (f *fakeStore) ListApplications(_ context.Context, limit, offset int) ([]platform.Application, error) {
	out := make([]platform.Application, 0, len(f.apps))
	for _, app := range f.apps {
		out = append(out, app)
	}
	if offset >= len(out) {
		return []platform.Application{}, nil
	}
	out = out[offset:]
	if limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

type countingEnqueuer struct{ woken int }

func (c *countingEnqueuer) Wake() { c.woken++ }

func newTestRouter(store platform.Store, enq Enqueuer) http.Handler {
	return NewRouter(Config{
		Logger:   discardLogger(),
		Store:    store,
		Factory:  applications.Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}},
		Enqueuer: enq,
	}).Handler()
}

const validCreateJSON = `{
	"name": "notes",
	"image": "ghcr.io/example/notes:1.0.0",
	"containerPort": 8080,
	"profile": "http",
	"exposure": {"mode": "internal"}
}`

func postApp(t *testing.T, h http.Handler, body, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(body))
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateAppReturns202WithLocationAndWakesWorker(t *testing.T) {
	store := newFakeStore()
	store.accept = func(_ context.Context, req platform.AcceptRequest) (platform.AcceptResult, error) {
		return platform.AcceptResult{ApplicationID: req.Application.ID, OperationID: req.Op.ID, State: platform.OperationQueued}, nil
	}
	enq := &countingEnqueuer{}
	h := newTestRouter(store, enq)

	rec := postApp(t, h, validCreateJSON, "idem-1")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/api/v1/operations/") {
		t.Fatalf("Location = %q", loc)
	}
	if enq.woken != 1 {
		t.Fatalf("worker woken %d times, want 1", enq.woken)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["state"] != "queued" {
		t.Fatalf("state = %v", body["state"])
	}
}

func TestCreateAppRequiresIdempotencyKey(t *testing.T) {
	h := newTestRouter(newFakeStore(), nil)
	rec := postApp(t, h, validCreateJSON, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateAppRejectsUnknownFieldsAndBadSpec(t *testing.T) {
	h := newTestRouter(newFakeStore(), nil)

	// Unknown field.
	rec := postApp(t, h, `{"name":"notes","image":"x","containerPort":80,"profile":"http","exposure":{"mode":"internal"},"bogus":1}`, "k1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", rec.Code)
	}

	// Unsupported profile → 422 from the applications validator.
	rec = postApp(t, h, `{"name":"notes","image":"x","containerPort":80,"profile":"tcp","exposure":{"mode":"internal"}}`, "k2")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad profile status = %d, want 422", rec.Code)
	}
}

func TestCreateAppRejectsStorage(t *testing.T) {
	h := newTestRouter(newFakeStore(), nil)
	rec := postApp(t, h, `{"name":"notes","image":"x","containerPort":80,"profile":"http","exposure":{"mode":"internal"},"storage":{"size":"1Gi"}}`, "k1")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("storage status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateAppPropagatesConflictAndVersionErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		storeErr error
		want     int
	}{
		{"active operation", platform.Errorf(platform.CodeConflict, "busy"), http.StatusConflict},
		{"idempotency conflict", platform.Errorf(platform.CodeIdempotencyConflict, "reused"), http.StatusConflict},
		{"version conflict", platform.Errorf(platform.CodeVersionConflict, "stale"), http.StatusPreconditionFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			store.accept = func(context.Context, platform.AcceptRequest) (platform.AcceptResult, error) {
				return platform.AcceptResult{}, tt.storeErr
			}
			rec := postApp(t, newTestRouter(store, nil), validCreateJSON, "k")
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestGetAppSetsETagFromVersion(t *testing.T) {
	store := newFakeStore()
	store.apps["app_1"] = platform.Application{ID: "app_1", Name: "notes", Version: 7}
	store.observed["app_1"] = platform.ObservedStatus{Phase: platform.WorkloadReady, ObservedAt: time.Now()}
	h := newTestRouter(store, nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/app_1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("ETag") != `"7"` {
		t.Fatalf("ETag = %q, want \"7\"", rec.Header().Get("ETag"))
	}
}

func TestGetAppNotFound(t *testing.T) {
	h := newTestRouter(newFakeStore(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetOperationRendersEvents(t *testing.T) {
	store := newFakeStore()
	store.ops["op_1"] = platform.Operation{ID: "op_1", AppID: "app_1", Type: platform.OperationCreate, State: platform.OperationRunning, Stage: "pulling"}
	store.events["op_1"] = []platform.OperationEvent{{OperationID: "op_1", Stage: "queued", Message: "accepted", At: time.Now()}}
	h := newTestRouter(store, nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/operations/op_1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["state"] != "running" {
		t.Fatalf("state = %v", body["state"])
	}
	events, _ := body["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("events = %v, want 1", body["events"])
	}
}
