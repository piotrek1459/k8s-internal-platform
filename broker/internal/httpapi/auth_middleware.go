package httpapi

import (
	"context"
	"net/http"
	"strings"

	"k8s-internal-platform/broker/internal/platform"
)

// protect wraps a handler with requireAuth when an Authenticator is configured.
// When Auth is nil (narrow unit tests that exercise a handler in isolation), it
// passes through unprotected. Production always configures Auth, so real
// deployments always enforce authentication and CSRF.
func (r *Router) protect(next http.HandlerFunc) http.HandlerFunc {
	if r.cfg.Auth == nil {
		return next
	}
	return r.requireAuth(next)
}

// requireAuth wraps a handler so it runs only for an authenticated session. It
// also enforces CSRF protection on state-changing requests: because the session
// is a cookie, a cross-site form could otherwise trigger a mutation. The broker
// requires a custom header (X-Requested-With) that browsers do not attach to
// simple cross-site requests and that cannot be set cross-origin without a
// pre-flighted CORS request the server never allows.
func (r *Router) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		reqID := requestID(req)

		if isStateChanging(req.Method) && !hasCSRFHeader(req) {
			writeError(w, r.cfg.Logger, reqID, platform.Errorf(platform.CodeForbidden,
				"missing CSRF header; include X-Requested-With on state-changing requests"))
			return
		}

		cookie, err := req.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, r.cfg.Logger, reqID, platform.Errorf(platform.CodeUnauthenticated, "authentication required"))
			return
		}
		adminID, expires, err := r.cfg.Auth.Authenticate(req.Context(), cookie.Value)
		if err != nil {
			writeError(w, r.cfg.Logger, reqID, err)
			return
		}
		ctx := context.WithValue(req.Context(), actorKey, adminID)
		ctx = context.WithValue(ctx, sessionExpiryKey, expires)
		next(w, req.WithContext(ctx))
	}
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// hasCSRFHeader reports whether the request carries the required CSRF header. The
// value is irrelevant; presence is what browsers cannot forge cross-origin
// without a CORS pre-flight.
func hasCSRFHeader(req *http.Request) bool {
	return strings.TrimSpace(req.Header.Get("X-Requested-With")) != ""
}
