// Package httpapi exposes the broker's management API over HTTP under /api/v1,
// plus the unauthenticated /healthz and /readyz probes. Handlers translate
// domain results and platform.Error values into the JSON response envelope and
// status codes defined in docs/api-and-data.md; they never talk to Kubernetes or
// Helm directly.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"k8s-internal-platform/broker/internal/platform"
)

// statusForCode maps a stable domain error code to an HTTP status, per the
// conventions in docs/api-and-data.md.
func statusForCode(code platform.ErrorCode) int {
	switch code {
	case platform.CodeInvalidInput:
		return http.StatusBadRequest
	case platform.CodeUnauthenticated:
		return http.StatusUnauthorized
	case platform.CodeForbidden:
		return http.StatusForbidden
	case platform.CodeNotFound:
		return http.StatusNotFound
	case platform.CodeConflict, platform.CodeIdempotencyConflict, platform.CodeSecretInUse:
		return http.StatusConflict
	case platform.CodeVersionConflict:
		return http.StatusPreconditionFailed
	case platform.CodeUnsupportedSpec, platform.CodeImageArchUnsupported, platform.CodeImageNotFound:
		return http.StatusUnprocessableEntity
	case platform.CodeDependencyUnavailable, platform.CodeRegistryUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// errorBody is the JSON error envelope: {"error": {...}}.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      platform.ErrorCode `json:"code"`
	Message   string             `json:"message"`
	Field     string             `json:"field,omitempty"`
	RequestID string             `json:"requestId"`
}

// writeJSON renders v as JSON with the given status. A late encoding error (for
// example a disconnected client) cannot be reported in-band, so it is logged.
func writeJSON(w http.ResponseWriter, logger *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Error("response_encode_failed", "error", err.Error())
	}
}

// writeError renders err as the standard envelope. A platform.Error carries its
// own code/message/field; any other error is reported as an opaque INTERNAL error
// so implementation detail never leaks to clients.
func writeError(w http.ResponseWriter, logger *slog.Logger, requestID string, err error) {
	var derr *platform.Error
	if !errors.As(err, &derr) {
		logger.Error("unhandled_error", "error", err.Error(), "requestId", requestID)
		derr = &platform.Error{Code: platform.CodeInternal, Message: "internal error"}
	}
	if statusForCode(derr.Code) >= http.StatusInternalServerError {
		logger.Error("request_failed", "code", string(derr.Code), "error", derr.Error(), "requestId", requestID)
	}
	writeJSON(w, logger, statusForCode(derr.Code), errorBody{Error: errorDetail{
		Code:      derr.Code,
		Message:   derr.Message,
		Field:     derr.Field,
		RequestID: requestID,
	}})
}
