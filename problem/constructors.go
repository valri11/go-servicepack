package problem

import (
	"net/http"
	"strconv"
)

// TypeBase namespaces the problem type URIs defined here. Type URIs need not
// be dereferenceable (RFC 9457 section 3.1).
const TypeBase = "https://problems.valri11.dev/"

const (
	TypeInvalidToken    = TypeBase + "invalid-token"
	TypeMissingAuth     = TypeBase + "missing-auth"
	TypeInvalidAPIKey   = TypeBase + "invalid-api-key"
	TypeValidationError = TypeBase + "validation-error"
	TypeNotReady        = TypeBase + "not-ready"
	TypeInternal        = TypeBase + "internal-error"
	TypePanic           = TypeBase + "panic"
)

const (
	// ExtTraceID carries the trace ID so a client can correlate the response
	// with a trace.
	ExtTraceID = "trace_id"
	ExtErrors  = "errors"
)

// genericInternalDetail is the only detail ever sent for a 500. The real cause
// goes to the log and the span.
const genericInternalDetail = "An unexpected error occurred."

func BadRequest(detail string) *Problem {
	return New(http.StatusBadRequest, detail)
}

// Unauthorized sets WWW-Authenticate, which RFC 9110 section 11.6.1 requires
// on a 401.
func Unauthorized(detail string) *Problem {
	return New(http.StatusUnauthorized, detail).
		WithHeader("WWW-Authenticate", `Bearer realm="api"`)
}

func Forbidden(detail string) *Problem {
	return New(http.StatusForbidden, detail)
}

func NotFound(detail string) *Problem {
	return New(http.StatusNotFound, detail)
}

// MethodNotAllowed sets Allow, which RFC 9110 section 15.5.6 requires.
func MethodNotAllowed(allowed string) *Problem {
	return New(http.StatusMethodNotAllowed, "The request method is not supported for this resource.").
		WithHeader("Allow", allowed)
}

func Internal(cause error) *Problem {
	return New(http.StatusInternalServerError, genericInternalDetail).
		WithType(TypeInternal).
		WithCause(cause)
}

func Unavailable(detail string, retryAfterSeconds int) *Problem {
	p := New(http.StatusServiceUnavailable, detail)
	if retryAfterSeconds > 0 {
		p.WithHeader("Retry-After", strconv.Itoa(retryAfterSeconds))
	}
	return p
}

type InvalidParam struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// InvalidParams reports field-level errors in the "errors" extension member.
// RFC 9457 section 3 leaves multi-error reporting to extensions.
func InvalidParams(params ...InvalidParam) *Problem {
	return BadRequest("Request parameters failed validation.").
		WithType(TypeValidationError).
		With(ExtErrors, params)
}
