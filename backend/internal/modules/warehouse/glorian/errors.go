package glorian

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Sentinel errors. An *APIError matches the sentinel of its class with
// errors.Is, so callers branch without parsing codes.
var (
	ErrUnauthorized       = errors.New("glorian: unauthorized")
	ErrForbidden          = errors.New("glorian: forbidden")
	ErrNotFound           = errors.New("glorian: not found")
	ErrConflict           = errors.New("glorian: conflict")
	ErrValidation         = errors.New("glorian: validation error")
	ErrRateLimited        = errors.New("glorian: rate limited")
	ErrServer             = errors.New("glorian: server error")
	ErrUnsupportedVersion = errors.New("glorian: unsupported api version")
	ErrNotImplemented     = errors.New("glorian: not implemented on hub")
	ErrInvalidResponse    = errors.New("glorian: invalid response")
	ErrTransport          = errors.New("glorian: transport error")
	ErrInvalidInput       = errors.New("glorian: invalid input")
	ErrMisconfigured      = errors.New("glorian: client misconfigured")

	// ErrInactiveConnection is returned by ClientResolver when the
	// connection is missing, disabled or lacks a base URL / API key. There
	// is no fallback to another connection (design §7, HYDRA rollback).
	ErrInactiveConnection = errors.New("glorian: inventory connection is missing or inactive")
)

// Error codes of the hub's error envelope (error.code).
const (
	CodeValidation         = "VALIDATION_ERROR"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeNotFound           = "NOT_FOUND"
	CodeConflict           = "CONFLICT"
	CodeForbidden          = "FORBIDDEN"
	CodeInternal           = "INTERNAL"
	CodeUnsupportedVersion = "UNSUPPORTED_API_VERSION"
	// CodeRateLimited is synthesised for a 429: Laravel's throttle answers
	// outside the envelope.
	CodeRateLimited = "RATE_LIMITED"
	// CodeHTTPError is synthesised when a non-2xx carries no envelope.
	CodeHTTPError = "HTTP_ERROR"
)

// APIError is the typed form of the hub's error envelope
// ({success:false, error:{code,message,details}, meta:{request_id}}).
type APIError struct {
	Status     int
	Code       string
	Message    string
	Details    map[string]any
	RequestID  string
	RetryAfter time.Duration // parsed Retry-After of a 429, zero otherwise
	Method     string
	Path       string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("glorian: %s %s: HTTP %d %s: %s", e.Method, e.Path, e.Status, e.Code, msg)
}

// Is maps the error onto its sentinel class.
func (e *APIError) Is(target error) bool {
	return target == e.class()
}

func (e *APIError) class() error {
	switch strings.ToUpper(e.Code) {
	case CodeUnsupportedVersion:
		return ErrUnsupportedVersion
	case CodeValidation:
		return ErrValidation
	case CodeUnauthorized:
		return ErrUnauthorized
	case CodeForbidden:
		return ErrForbidden
	case CodeNotFound:
		return ErrNotFound
	case CodeConflict:
		return ErrConflict
	case CodeRateLimited:
		return ErrRateLimited
	}
	switch {
	case e.Status == http.StatusUnauthorized:
		return ErrUnauthorized
	case e.Status == http.StatusForbidden:
		return ErrForbidden
	case e.Status == http.StatusNotFound:
		return ErrNotFound
	case e.Status == http.StatusConflict:
		return ErrConflict
	case e.Status == http.StatusUnprocessableEntity:
		return ErrValidation
	case e.Status == http.StatusTooManyRequests:
		return ErrRateLimited
	case e.Status == http.StatusNotImplemented:
		return ErrNotImplemented
	case e.Status >= 500:
		return ErrServer
	}
	return nil
}
