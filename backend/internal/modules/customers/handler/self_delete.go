package handler

// TEC-1257: the customer deletes their own account (portal and mobile app,
// App Store 5.1.1(v), KVKK). Two steps: a code to the account's phone
// (purpose account_delete), then the code confirms the anonymization.

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	cu "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Error codes of the self-delete endpoints. The OTP codes are the ones of
// the phone sign-in (auth handler), so the clients reuse their messages.
const (
	CodeNoPhone           = "ACCOUNT_HAS_NO_PHONE"
	CodeInvalidOTPCode    = "INVALID_OTP_CODE"
	CodeOTPLocked         = "OTP_LOCKED"
	CodeOTPDeliveryFailed = "OTP_DELIVERY_FAILED"
	CodeOTPUnavailable    = "RATE_LIMITER_UNAVAILABLE"
)

// OTP sends and checks phone codes (*otp.Service).
type OTP interface {
	Request(ctx context.Context, in otp.RequestInput) (otp.RequestResult, error)
	Verify(ctx context.Context, in otp.VerifyInput) (otp.Verified, error)
}

// WithOTP enables the self-delete endpoints (without it they answer 503).
func (h *Handler) WithOTP(o OTP) *Handler {
	h.otp = o
	return h
}

type deleteCodeBody struct {
	Locale string `json:"locale"`
}

// RequestAccountDeleteCode (POST /v1/portal/me/delete/code, 202) sends the
// confirmation code to the account's own phone.
func (h *Handler) RequestAccountDeleteCode(w http.ResponseWriter, r *http.Request) {
	if h.otp == nil {
		response.ServiceUnavailable(w, r, CodeOTPDeliveryFailed, "Account deletion is not available")
		return
	}
	var b deleteCodeBody
	if r.ContentLength != 0 && !decode(w, r, &b) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	phone, err := h.svc.SelfDeletePhone(r.Context(), p.UserInternal)
	if err != nil {
		writeSelfDeleteError(w, r, err)
		return
	}
	locale := b.Locale
	if _, ok := i18n.Parse(locale); !ok {
		locale = string(i18n.FromContext(r.Context()).Locale)
	}
	meta := activity.MetaFromRequest(r)
	res, err := h.otp.Request(r.Context(), otp.RequestInput{
		Phone: phone, Purpose: otp.PurposeAccountDelete, Locale: locale, IP: metaIP(meta), UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeSelfDeleteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, map[string]any{
		"status": "sent", "channel": res.Channel, "expires_at": res.ExpiresAt, "resend_at": res.ResendAt,
	})
}

type deleteAccountBody struct {
	Code string `json:"code"`
}

// DeleteOwnAccount (POST /v1/portal/me/delete): the code confirms the
// anonymization of the signed-in customer's account. Every session of the
// account ends; the client signs out.
func (h *Handler) DeleteOwnAccount(w http.ResponseWriter, r *http.Request) {
	if h.otp == nil {
		response.ServiceUnavailable(w, r, CodeOTPDeliveryFailed, "Account deletion is not available")
		return
	}
	var b deleteAccountBody
	if !decode(w, r, &b) {
		return
	}
	code := strings.TrimSpace(b.Code)
	if code == "" {
		writeError(w, r, &cu.ValidationError{Field: "code", Message: "is required"})
		return
	}
	p := authctx.MustPrincipal(r.Context())
	phone, err := h.svc.SelfDeletePhone(r.Context(), p.UserInternal)
	if err != nil {
		writeSelfDeleteError(w, r, err)
		return
	}
	meta := activity.MetaFromRequest(r)
	if _, err := h.otp.Verify(r.Context(), otp.VerifyInput{
		Phone: phone, Purpose: otp.PurposeAccountDelete, Code: code, IP: metaIP(meta),
	}); err != nil {
		writeSelfDeleteError(w, r, err)
		return
	}
	res, err := h.svc.DeleteOwnAccount(r.Context(), p.UserInternal, p.UserID, meta)
	if err != nil {
		writeSelfDeleteError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

func metaIP(m activity.Meta) string {
	if m.IP == nil {
		return ""
	}
	return m.IP.String()
}

func writeSelfDeleteError(w http.ResponseWriter, r *http.Request, err error) {
	var le *otp.LimitError
	switch {
	case errors.Is(err, cu.ErrNoPhone):
		response.Conflict(w, r, CodeNoPhone, "The account has no phone number for the confirmation code")
	case errors.As(err, &le):
		if d := time.Until(le.RetryAt); d > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		}
		response.ErrorWithDetails(w, r, http.StatusTooManyRequests, response.CodeRateLimited,
			"Too many code requests. Try again later.",
			[]response.Detail{{Field: "resend_at", Message: le.RetryAt.UTC().Format(time.RFC3339), Code: le.Reason}})
	case errors.Is(err, otp.ErrInvalidCode):
		response.Error(w, r, http.StatusUnauthorized, CodeInvalidOTPCode, "Code is invalid or expired")
	case errors.Is(err, otp.ErrTooManyAttempts):
		response.Error(w, r, http.StatusTooManyRequests, CodeOTPLocked, "Too many wrong codes. Request a new code.")
	case errors.Is(err, otp.ErrLimiterUnavailable):
		response.ServiceUnavailable(w, r, CodeOTPUnavailable, "Account deletion is temporarily unavailable")
	case errors.Is(err, otp.ErrDeliveryFailed):
		response.ServiceUnavailable(w, r, CodeOTPDeliveryFailed, "The code could not be delivered. Try again later.")
	default:
		writePrivacyError(w, r, err)
	}
}
