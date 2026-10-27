package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s-internal-platform/broker/internal/applications"
	"k8s-internal-platform/broker/internal/auth"
	"k8s-internal-platform/broker/internal/platform"
	"k8s-internal-platform/broker/internal/store"
)

// newAuthRouter wires the real auth service and store so the HTTP tests exercise
// genuine authentication, CSRF, and session cookies end to end.
func newAuthRouter(t *testing.T, bootstrapToken string) http.Handler {
	t.Helper()
	db, err := store.Open(context.Background(), t.TempDir()+"/platform.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := store.New(db)
	svc := auth.NewService(st, platform.RandomIDGenerator{}, platform.SystemClock{}, bootstrapToken, time.Hour)
	return NewRouter(Config{
		Logger:  discardLogger(),
		Store:   st,
		Factory: applications.Factory{IDs: platform.RandomIDGenerator{}, Clock: platform.SystemClock{}},
		Auth:    svc,
	}).Handler()
}

func TestSetupRejectsInvalidPasswordAndKeepsTokenUsable(t *testing.T) {
	h := newAuthRouter(t, "boot-token")

	// An empty body (missing password) with a valid bootstrap token must be
	// rejected, must not create an administrator, and must leave setup available.
	for _, body := range []string{`{}`, `{"password":""}`, `{"password":"short"}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer boot-token")
		req.Header.Set("X-Requested-With", "fetch")
		rec := do(h, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("setup with %s: status=%d, want 400", body, rec.Code)
		}
	}

	// Setup must still be available, and a valid password must now succeed.
	status := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil))
	if !strings.Contains(status.Body.String(), `"setupAvailable":true`) {
		t.Fatalf("setup no longer available after invalid attempts: %s", status.Body.String())
	}
	ok := httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(`{"password":"a-strong-password"}`))
	ok.Header.Set("Authorization", "Bearer boot-token")
	ok.Header.Set("X-Requested-With", "fetch")
	if rec := do(h, ok); rec.Code != http.StatusCreated {
		t.Fatalf("valid setup after invalid attempts: status=%d", rec.Code)
	}

	// A subsequent login with an empty password must not yield a session.
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"password":""}`))
	login.Header.Set("X-Requested-With", "fetch")
	rec := do(h, login)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("empty-password login: status=%d, want 401", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			t.Fatal("empty-password login set a session cookie")
		}
	}
}

func TestUnauthenticatedMutationIsRejected(t *testing.T) {
	h := newAuthRouter(t, "boot")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(validCreateJSON))
	req.Header.Set("Idempotency-Key", "k1")
	req.Header.Set("X-Requested-With", "fetch") // CSRF header present, but no session
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

func TestStateChangingRequestRequiresCSRFHeader(t *testing.T) {
	h := newAuthRouter(t, "boot")
	// Even with a (hypothetical) session cookie, a missing CSRF header is rejected
	// before authentication, so a cross-site form submission cannot mutate state.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(validCreateJSON))
	req.Header.Set("Idempotency-Key", "k1")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "whatever"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (missing CSRF header)", rec.Code)
	}
}

func TestSetupLoginAndAuthenticatedDeployFlow(t *testing.T) {
	h := newAuthRouter(t, "boot-token")

	// setup/status reports availability.
	rec := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"setupAvailable":true`) {
		t.Fatalf("setup status: %d %s", rec.Code, rec.Body.String())
	}

	// Create the administrator with the bootstrap bearer token.
	setupReq := httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(`{"password":"a-strong-password"}`))
	setupReq.Header.Set("Authorization", "Bearer boot-token")
	setupReq.Header.Set("X-Requested-With", "fetch")
	if rec := do(h, setupReq); rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}

	// Log in; capture the session cookie.
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"password":"a-strong-password"}`))
	loginReq.Header.Set("X-Requested-With", "fetch")
	loginRec := do(h, loginReq)
	if loginRec.Code != http.StatusNoContent {
		t.Fatalf("login: %d %s", loginRec.Code, loginRec.Body.String())
	}
	cookie := sessionCookieFrom(t, loginRec)

	// An authenticated create now succeeds.
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(validCreateJSON))
	createReq.Header.Set("Idempotency-Key", "deploy-1")
	createReq.Header.Set("X-Requested-With", "fetch")
	createReq.AddCookie(cookie)
	if rec := do(h, createReq); rec.Code != http.StatusAccepted {
		t.Fatalf("authenticated create: %d %s", rec.Code, rec.Body.String())
	}

	// me reports the session.
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.AddCookie(cookie)
	if rec := do(h, meReq); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "administratorId") {
		t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
	}

	// Setup is no longer available.
	if rec := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)); !strings.Contains(rec.Body.String(), `"setupAvailable":false`) {
		t.Fatalf("setup still available after completion: %s", rec.Body.String())
	}
}

func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return c
		}
	}
	t.Fatal("no session cookie set on login response")
	return nil
}
