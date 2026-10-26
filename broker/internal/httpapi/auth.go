package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"k8s-internal-platform/broker/internal/platform"
)

// sessionCookieName is the name of the HttpOnly session cookie.
const sessionCookieName = "platform_session"

// Authenticator validates sessions and performs setup/login/logout. The auth
// package implements it; keeping it an interface here preserves the test seam and
// avoids a hard dependency from httpapi on auth's concrete type.
type Authenticator interface {
	SetupAvailable(ctx context.Context) (bool, error)
	Setup(ctx context.Context, bootstrapToken, password string) error
	Login(ctx context.Context, password string) (token string, expires time.Time, err error)
	Authenticate(ctx context.Context, rawToken string) (adminID string, expires time.Time, err error)
	Logout(ctx context.Context, rawToken string) error
}

func (r *Router) registerAuth() {
	r.mux.HandleFunc("GET /api/v1/setup/status", r.handleSetupStatus)
	r.mux.HandleFunc("POST /api/v1/setup", r.handleSetup)
	r.mux.HandleFunc("POST /api/v1/auth/session", r.handleLogin)
	r.mux.HandleFunc("DELETE /api/v1/auth/session", r.handleLogout)
	r.mux.HandleFunc("GET /api/v1/auth/me", r.protect(r.handleMe))
}

func (r *Router) handleSetupStatus(w http.ResponseWriter, req *http.Request) {
	available, err := r.cfg.Auth.SetupAvailable(req.Context())
	if err != nil {
		writeError(w, r.cfg.Logger, requestID(req), err)
		return
	}
	writeJSON(w, r.cfg.Logger, http.StatusOK, map[string]bool{"setupAvailable": available})
}

func (r *Router) handleSetup(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	// The bootstrap token is presented as a bearer credential, matching the
	// security scheme in the OpenAPI contract.
	token := bearerToken(req)
	if err := r.cfg.Auth.Setup(req.Context(), token, body.Password); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	writeJSON(w, r.cfg.Logger, http.StatusCreated, map[string]string{"status": "created"})
}

func (r *Router) handleLogin(w http.ResponseWriter, req *http.Request) {
	reqID := requestID(req)
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	token, expires, err := r.cfg.Auth.Login(req.Context(), body.Password)
	if err != nil {
		writeError(w, r.cfg.Logger, reqID, err)
		return
	}
	http.SetCookie(w, r.sessionCookie(token, expires))
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleLogout(w http.ResponseWriter, req *http.Request) {
	if c, err := req.Cookie(sessionCookieName); err == nil {
		_ = r.cfg.Auth.Logout(req.Context(), c.Value)
	}
	// Clear the cookie regardless, so the browser does not keep sending it.
	http.SetCookie(w, r.sessionCookie("", time.Unix(0, 0)))
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleMe(w http.ResponseWriter, req *http.Request) {
	// handleMe sits behind the auth middleware, so identity is present.
	adminID, _ := req.Context().Value(actorKey).(string)
	expires, _ := req.Context().Value(sessionExpiryKey).(time.Time)
	writeJSON(w, r.cfg.Logger, http.StatusOK, map[string]any{
		"administratorId":  adminID,
		"sessionExpiresAt": expires,
	})
}

// sessionCookie builds the session cookie. Secure is set when the request chain
// is HTTPS; plain HTTP is for local development or the loopback bootstrap only,
// per docs/api-and-data.md.
func (r *Router) sessionCookie(value string, expires time.Time) *http.Cookie {
	c := &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  expires,
		Secure:   r.cfg.SecureCookies,
	}
	if value == "" {
		c.MaxAge = -1
	}
	return c
}

func decodeJSON(req *http.Request, v any) error {
	raw, err := io.ReadAll(io.LimitReader(req.Body, maxBodyBytes))
	if err != nil {
		return platform.Errorf(platform.CodeInvalidInput, "could not read request body")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return platform.Errorf(platform.CodeInvalidInput, "invalid JSON: %v", err)
	}
	return nil
}

func bearerToken(req *http.Request) string {
	const prefix = "Bearer "
	h := req.Header.Get("Authorization")
	if len(h) > len(prefix) && h[:len(prefix)] == prefix {
		return h[len(prefix):]
	}
	return ""
}
