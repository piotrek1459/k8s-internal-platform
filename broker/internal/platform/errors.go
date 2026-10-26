package platform

import "fmt"

// ErrorCode is a stable, machine-readable error identifier returned to clients in
// the error envelope described in docs/api-and-data.md. Codes are stable API
// surface: rename with care.
type ErrorCode string

const (
	CodeInvalidInput         ErrorCode = "INVALID_INPUT"          // 400
	CodeUnauthenticated      ErrorCode = "UNAUTHENTICATED"        // 401
	CodeForbidden            ErrorCode = "FORBIDDEN"              // 403
	CodeNotFound             ErrorCode = "NOT_FOUND"              // 404
	CodeConflict             ErrorCode = "CONFLICT"               // 409 (active work / NeedsAttention)
	CodeVersionConflict      ErrorCode = "VERSION_CONFLICT"       // 412 (stale If-Match)
	CodeUnsupportedSpec      ErrorCode = "UNSUPPORTED_SPEC"       // 422
	CodeDependencyUnavailable ErrorCode = "DEPENDENCY_UNAVAILABLE" // 503
	CodeIdempotencyConflict  ErrorCode = "IDEMPOTENCY_CONFLICT"   // 409 (key reused with different body)

	CodeImageArchUnsupported ErrorCode = "IMAGE_ARCH_UNSUPPORTED" // 422
	CodeImageNotFound        ErrorCode = "IMAGE_NOT_FOUND"        // 422
	CodeRegistryUnavailable  ErrorCode = "REGISTRY_UNAVAILABLE"   // 503

	CodeSecretInUse          ErrorCode = "SECRET_IN_USE"          // 409
	CodeInternal             ErrorCode = "INTERNAL"               // 500
)

// Error is a domain error carrying a stable code, a human-readable message, and
// an optional field name for input errors. Adapters return these; the HTTP layer
// maps Code to a status and renders the envelope. It implements error so it can
// flow through ordinary Go error handling.
type Error struct {
	Code    ErrorCode
	Message string
	Field   string // optional: the offending input field
	// Err is an optional wrapped cause, kept out of client responses.
	Err error
}

func (e *Error) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (field %s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// Errorf builds a domain Error with a formatted message.
func Errorf(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// FieldErrorf builds a domain Error attributed to a specific input field.
func FieldErrorf(code ErrorCode, field, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Field: field}
}
