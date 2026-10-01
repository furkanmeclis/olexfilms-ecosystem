// Package response provides the standard JSON success/error envelope used by all API handlers.
//
// Envelope shape:
//
//	{"success":true,"data":{},"meta":{"request_id":"..."}}
//	{"success":false,"error":{"code","message","details":[{"field","message","code"}]},"meta":{"request_id"}}
package response

import (
	"context"
	"encoding/json"
	"net/http"
)

const (
	CodeValidationError           = "VALIDATION_ERROR"
	CodeUnauthenticated           = "UNAUTHENTICATED"
	CodeForbidden                 = "FORBIDDEN"
	CodeNotFound                  = "NOT_FOUND"
	CodeConflict                  = "CONFLICT"
	CodeInternalError             = "INTERNAL_ERROR"
	CodeInvalidCredentials        = "INVALID_CREDENTIALS"
	CodeInvalidResetCode          = "INVALID_RESET_CODE"
	CodeInvalidVerifyCode         = "INVALID_VERIFICATION_CODE"
	CodePasswordResetFailed       = "PASSWORD_RESET_FAILED"
	CodeNoTenantMembership        = "NO_TENANT_MEMBERSHIP"
	CodeOrganizationAccessExpired = "ORGANIZATION_ACCESS_EXPIRED"
	CodeOrganizationReadOnly      = "ORGANIZATION_READ_ONLY"
	CodeBrandMismatch             = "BRAND_MISMATCH"
	CodeSubscriptionReadOnly      = "SUBSCRIPTION_READ_ONLY"
	CodeRealtimeDisabled          = "REALTIME_DISABLED"
	CodeStepUpRequired            = "STEP_UP_REQUIRED"
	CodeLimitReached              = "LIMIT_REACHED"
	CodeFeatureDisabled           = "FEATURE_DISABLED"
	CodeModuleCore                = "MODULE_CORE"
	CodeModuleAdminOverride       = "MODULE_ADMIN_OVERRIDE"
	CodeMFARequired               = "MFA_REQUIRED"
	CodeMFANotEnrolled            = "MFA_NOT_ENROLLED"
	CodeRateLimited               = "RATE_LIMITED"
	CodeInvalidMFACode            = "INVALID_MFA_CODE"
	CodeTerritoryConflict         = "TERRITORY_CONFLICT"
	CodeInvalidPlate              = "INVALID_PLATE"
	CodeRateNotFound              = "RATE_NOT_FOUND"

	// Session realms (TEC-90): panel vs customer portal.
	CodeNoPortalAccess = "NO_PORTAL_ACCESS"
	CodeNoPanelAccess  = "NO_PANEL_ACCESS"
	CodeRealmForbidden = "REALM_FORBIDDEN"
)

// RequestIDFunc resolves the correlation id from request context.
// Wired by middleware to avoid an import cycle with pkg/response.
var RequestIDFunc = func(context.Context) string { return "" }

// Meta carries per-response correlation metadata.
type Meta struct {
	RequestID string `json:"request_id"`
}

// Detail is a field-level validation or structured error detail.
type Detail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// ErrorBody is the error object inside a failed envelope.
type ErrorBody struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []Detail `json:"details,omitempty"`
}

// Envelope is the standard API response wrapper.
type Envelope struct {
	Success bool       `json:"success"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
	Meta    Meta       `json:"meta"`
}

// JSON writes a successful response envelope.
func JSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	write(w, r, status, Envelope{
		Success: true,
		Data:    data,
		Meta:    metaFrom(r),
	})
}

// Error writes a failed envelope without details.
func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	ErrorWithDetails(w, r, status, code, message, nil)
}

// ErrorWithDetails writes a failed envelope with optional field-level details.
func ErrorWithDetails(w http.ResponseWriter, r *http.Request, status int, code, message string, details []Detail) {
	body := &ErrorBody{Code: code, Message: message}
	if len(details) > 0 {
		body.Details = details
	}
	write(w, r, status, Envelope{
		Success: false,
		Error:   body,
		Meta:    metaFrom(r),
	})
}

// ValidationError writes HTTP 400 with code VALIDATION_ERROR and field details.
func ValidationError(w http.ResponseWriter, r *http.Request, details []Detail) {
	msg := "Request validation failed"
	if len(details) == 1 && details[0].Message != "" {
		msg = details[0].Message
	}
	ErrorWithDetails(w, r, http.StatusBadRequest, CodeValidationError, msg, details)
}

// Unauthorized writes HTTP 401 UNAUTHENTICATED.
func Unauthorized(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Authentication is required"
	}
	Error(w, r, http.StatusUnauthorized, CodeUnauthenticated, message)
}

// Forbidden writes HTTP 403 FORBIDDEN.
func Forbidden(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "You do not have access to this resource"
	}
	Error(w, r, http.StatusForbidden, CodeForbidden, message)
}

// StepUpRequired writes HTTP 403 STEP_UP_REQUIRED.
func StepUpRequired(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusForbidden, CodeStepUpRequired, "Recent authentication is required")
}

// NotFound writes HTTP 404 NOT_FOUND.
func NotFound(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Resource was not found"
	}
	Error(w, r, http.StatusNotFound, CodeNotFound, message)
}

// Conflict writes HTTP 409 with the given code (defaults to CONFLICT).
func Conflict(w http.ResponseWriter, r *http.Request, code, message string) {
	if code == "" {
		code = CodeConflict
	}
	if message == "" {
		message = "Resource conflict"
	}
	Error(w, r, http.StatusConflict, code, message)
}

// BadRequest writes HTTP 400 with the given code (defaults to VALIDATION_ERROR).
func BadRequest(w http.ResponseWriter, r *http.Request, code, message string) {
	if code == "" {
		code = CodeValidationError
	}
	Error(w, r, http.StatusBadRequest, code, message)
}

// TooManyRequests writes HTTP 429 RATE_LIMITED.
func TooManyRequests(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "Too many attempts. Try again later."
	}
	Error(w, r, http.StatusTooManyRequests, CodeRateLimited, message)
}

// Internal writes HTTP 500 INTERNAL_ERROR.
func Internal(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "internal server error"
	}
	Error(w, r, http.StatusInternalServerError, CodeInternalError, message)
}

// ServiceUnavailable writes HTTP 503 with the given code.
func ServiceUnavailable(w http.ResponseWriter, r *http.Request, code, message string) {
	if code == "" {
		code = CodeInternalError
	}
	if message == "" {
		message = "Service unavailable"
	}
	Error(w, r, http.StatusServiceUnavailable, code, message)
}

func metaFrom(r *http.Request) Meta {
	if r == nil {
		return Meta{}
	}
	return Meta{RequestID: RequestIDFunc(r.Context())}
}

func write(w http.ResponseWriter, _ *http.Request, status int, payload Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
