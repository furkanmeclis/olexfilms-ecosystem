package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

func (h *MobileHandler) SetOTP(svc usecase.MobileOTP) { h.otp = svc }

func (h *MobileHandler) Identify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Identifier string `json:"identifier"`
		Region     string `json:"region"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, otpRequestBodyLimit)
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	_, normalized, err := usecase.NormalizeMobileIdentifier(in.Identifier, in.Region)
	if err != nil {
		writeMobilePhoneError(w, r, err)
		return
	}
	meta := sessionMeta(r)
	if h.limiter != nil {
		ok, retry := h.limiter.AllowLogin(r.Context(), meta.IP, normalized)
		if writeRetryAfter(w, r, ok, retry) {
			return
		}
	}
	res, err := h.uc.MobileIdentify(r.Context(), h.otp, in.Identifier, in.Region, requestLocale(r, ""), meta)
	if err != nil {
		writeMobilePhoneError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

func (h *MobileHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone            string           `json:"phone"`
		Code             string           `json:"code"`
		Device           model.DeviceInfo `json:"device"`
		OrganizationSlug string           `json:"organization_slug"`
		TOTPCode         string           `json:"totp_code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, otpRequestBodyLimit)
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	tokens, err := h.uc.MobileVerifyOTP(r.Context(), h.otp, in.Phone, in.Code, in.TOTPCode, in.OrganizationSlug, in.Device, sessionMeta(r))
	if err != nil {
		writeMobilePhoneError(w, r, err)
		return
	}
	h.writeLoginResponse(w, r, tokens)
}

func writeMobilePhoneError(w http.ResponseWriter, r *http.Request, err error) {
	var limit *otp.LimitError
	if errors.As(err, &limit) || errors.Is(err, otp.ErrInvalidPhone) || errors.Is(err, otp.ErrInvalidPurpose) || errors.Is(err, otp.ErrInvalidCode) || errors.Is(err, otp.ErrTooManyAttempts) || errors.Is(err, otp.ErrLimiterUnavailable) || errors.Is(err, otp.ErrDeliveryFailed) {
		writeOTPError(w, r, err)
		return
	}
	writeMobileError(w, r, err)
}
