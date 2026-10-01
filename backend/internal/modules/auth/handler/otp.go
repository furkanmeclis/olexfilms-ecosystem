package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Error codes of the phone OTP endpoints.
const (
	CodeInvalidOTPCode               = "INVALID_OTP_CODE"
	CodeOTPLocked                    = "OTP_LOCKED"
	CodeOTPDeliveryFailed            = "OTP_DELIVERY_FAILED"
	CodeRateLimiterUnavailable       = "RATE_LIMITER_UNAVAILABLE"
	CodeInvalidPhone                 = "INVALID_PHONE"
	otpPublicPurpose                 = otp.PurposeCustomerLogin
	otpRequestBodyLimit        int64 = 4 << 10
)

// OTPService is the phone OTP port (platform/otp.Service).
type OTPService interface {
	Request(ctx context.Context, in otp.RequestInput) (otp.RequestResult, error)
	Verify(ctx context.Context, in otp.VerifyInput) (otp.Verified, error)
}

// SetOTP enables POST /v1/auth/otp/request and /v1/auth/otp/verify.
func (h *Handler) SetOTP(svc OTPService) {
	h.otp = svc
}

type otpRequestBody struct {
	Phone   string `json:"phone"`
	Purpose string `json:"purpose"`
	Country string `json:"country"`
	Locale  string `json:"locale"`
}

func requestLocale(r *http.Request, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	al := r.Header.Get("Accept-Language")
	if i := strings.IndexAny(al, ",;"); i >= 0 {
		al = al[:i]
	}
	return strings.TrimSpace(al)
}

// OTPRequest sends a login code to a phone over WhatsApp (SMS fallback).
// It answers 202 whether or not an account exists for the number.
func (h *Handler) OTPRequest(w http.ResponseWriter, r *http.Request) {
	if h.otp == nil {
		response.ServiceUnavailable(w, r, CodeOTPDeliveryFailed, "Phone sign-in is not available")
		return
	}
	var in otpRequestBody
	r.Body = http.MaxBytesReader(w, r.Body, otpRequestBodyLimit)
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	purpose := strings.TrimSpace(in.Purpose)
	if purpose == "" {
		purpose = otpPublicPurpose
	}
	if purpose != otpPublicPurpose {
		// contract_sign codes are issued by the contracts flow (TEC-112).
		response.BadRequest(w, r, response.CodeValidationError, "Unsupported purpose")
		return
	}
	meta := sessionMeta(r)
	res, err := h.otp.Request(r.Context(), otp.RequestInput{
		Phone: in.Phone, Region: in.Country, Purpose: purpose,
		Locale: requestLocale(r, in.Locale), IP: meta.IP, UserAgent: meta.UserAgent,
	})
	if err != nil {
		writeOTPError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, map[string]any{
		"status":     "sent",
		"channel":    res.Channel,
		"expires_at": res.ExpiresAt,
		"resend_at":  res.ResendAt,
	})
}

type otpVerifyBody struct {
	Phone   string `json:"phone"`
	Code    string `json:"code"`
	Purpose string `json:"purpose"`
	Country string `json:"country"`
	Locale  string `json:"locale"`
}

// OTPVerify checks the code and issues a session for the phone's owner
// (a new customer account for an unknown number).
func (h *Handler) OTPVerify(w http.ResponseWriter, r *http.Request) {
	if h.otp == nil {
		response.ServiceUnavailable(w, r, CodeOTPDeliveryFailed, "Phone sign-in is not available")
		return
	}
	var in otpVerifyBody
	r.Body = http.MaxBytesReader(w, r.Body, otpRequestBodyLimit)
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if p := strings.TrimSpace(in.Purpose); p != "" && p != otpPublicPurpose {
		response.BadRequest(w, r, response.CodeValidationError, "Unsupported purpose")
		return
	}
	meta := sessionMeta(r)
	v, err := h.otp.Verify(r.Context(), otp.VerifyInput{
		Phone: in.Phone, Region: in.Country, Purpose: otpPublicPurpose, Code: in.Code, IP: meta.IP,
	})
	if err != nil {
		writeOTPError(w, r, err)
		return
	}
	tokens, _, err := h.uc.LoginWithVerifiedPhone(r.Context(), v.Phone, requestLocale(r, in.Locale), meta)
	if err != nil {
		if errors.Is(err, usecase.ErrUserDisabled) {
			// Locked / disabled account: generic message, no status detail.
			response.Forbidden(w, r, "Sign-in is not possible with this number")
			return
		}
		writeUsecaseError(w, r, err)
		return
	}
	// Same token pair shape as POST /v1/auth/login (aud=portal).
	response.JSON(w, r, http.StatusOK, tokens)
}

func writeOTPError(w http.ResponseWriter, r *http.Request, err error) {
	var le *otp.LimitError
	switch {
	case errors.As(err, &le):
		if d := time.Until(le.RetryAt); d > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		}
		response.ErrorWithDetails(w, r, http.StatusTooManyRequests, response.CodeRateLimited,
			"Too many code requests. Try again later.",
			[]response.Detail{{Field: "resend_at", Message: le.RetryAt.UTC().Format(time.RFC3339), Code: le.Reason}})
	case errors.Is(err, otp.ErrInvalidPhone):
		response.ErrorWithDetails(w, r, http.StatusBadRequest, CodeInvalidPhone, "Phone number is invalid",
			[]response.Detail{{Field: "phone", Message: "invalid phone number", Code: "invalid"}})
	case errors.Is(err, otp.ErrInvalidPurpose):
		response.BadRequest(w, r, response.CodeValidationError, "Unsupported purpose")
	case errors.Is(err, otp.ErrInvalidCode):
		response.Error(w, r, http.StatusUnauthorized, CodeInvalidOTPCode, "Code is invalid or expired")
	case errors.Is(err, otp.ErrTooManyAttempts):
		response.Error(w, r, http.StatusTooManyRequests, CodeOTPLocked, "Too many wrong codes. Request a new code.")
	case errors.Is(err, otp.ErrLimiterUnavailable):
		response.ServiceUnavailable(w, r, CodeRateLimiterUnavailable, "Phone sign-in is temporarily unavailable")
	case errors.Is(err, otp.ErrDeliveryFailed):
		response.ServiceUnavailable(w, r, CodeOTPDeliveryFailed, "The code could not be delivered. Try again later.")
	default:
		response.InternalErr(w, r, err, "otp failed")
	}
}
