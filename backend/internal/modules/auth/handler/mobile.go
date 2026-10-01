package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// PushTokenService is the Expo token store of TEC-87 (device_push_tokens);
// the mobile push-token routes write through the same use case as
// /v1/notifications/push-devices.
type PushTokenService interface {
	RegisterPushDevice(ctx context.Context, userID int64, in notifusecase.PushDeviceInput) (notifmodel.PushDevice, error)
	RevokePushDevice(ctx context.Context, userID int64, token string) error
	RevokePushDevicesForDevice(ctx context.Context, userID int64, deviceID string) error
	RevokeAllPushDevices(ctx context.Context, userID int64) error
}

// MobileHandler serves /v1/mobile/* (Bearer, aud=mobile) and the web side of
// the QR sign-in (/v1/auth/qr/*), TEC-91.
type MobileHandler struct {
	uc      *usecase.AuthUseCase
	push    PushTokenService
	limiter *ratelimit.Limiter
}

// NewMobile creates the mobile handler. push and limiter may be nil.
func NewMobile(uc *usecase.AuthUseCase, push PushTokenService, limiter *ratelimit.Limiter) *MobileHandler {
	return &MobileHandler{uc: uc, push: push, limiter: limiter}
}

// mobileLoginResponse is the token pair plus the session hydration, so the
// app needs no second round trip after sign-in.
type mobileLoginResponse struct {
	model.Tokens
	Me mobileMe `json:"me"`
}

// mobileMe is /v1/auth/me plus the mobile session (device) and the warehouse
// context reserved for F1 (always null for now).
type mobileMe struct {
	model.Me
	Session          *mobileSessionView `json:"session"`
	CurrentWarehouse *struct{}          `json:"current_warehouse"`
}

type mobileSessionView struct {
	UUID      string           `json:"uuid"`
	Device    model.DeviceInfo `json:"device"`
	ExpiresAt time.Time        `json:"expires_at"`
}

func (h *MobileHandler) buildMe(ctx context.Context, p authctx.Principal) (mobileMe, error) {
	me, err := h.uc.Me(ctx, p.UserID, p.ImpersonatorUserID, p.OrganizationUUID)
	if err != nil {
		return mobileMe{}, err
	}
	out := mobileMe{Me: me}
	if s, err := h.uc.MobileSession(ctx, p.UserInternal, p.SessionID); err == nil && s.Device != nil {
		out.Session = &mobileSessionView{UUID: s.UUID.String(), Device: *s.Device, ExpiresAt: s.ExpiresAt}
	}
	return out, nil
}

// Login is POST /v1/mobile/auth/login.
func (h *MobileHandler) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email            string           `json:"email"`
		Password         string           `json:"password"`
		TOTPCode         string           `json:"totp_code"`
		OrganizationSlug string           `json:"organization_slug"`
		Device           model.DeviceInfo `json:"device"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	meta := sessionMeta(r)
	if h.limiter != nil {
		ok, retry := h.limiter.AllowLogin(r.Context(), meta.IP, in.Email)
		if writeRetryAfter(w, r, ok, retry) {
			return
		}
	}
	tokens, err := h.uc.MobileLogin(r.Context(), in.Email, in.Password, in.TOTPCode, in.OrganizationSlug, in.Device, meta)
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	p, err := h.uc.PrincipalFromAccess(r.Context(), tokens.AccessToken)
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	me, err := h.buildMe(r.Context(), p)
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, mobileLoginResponse{Tokens: tokens, Me: me})
}

// Refresh is POST /v1/mobile/auth/refresh.
func (h *MobileHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
		AppVersion   string `json:"app_version"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	tokens, err := h.uc.MobileRefresh(r.Context(), in.RefreshToken, in.AppVersion, sessionMeta(r))
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidCredentials) {
			response.Unauthorized(w, r, "Refresh token is invalid")
			return
		}
		writeMobileError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tokens)
}

// Me is GET /v1/mobile/auth/me.
func (h *MobileHandler) Me(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	me, err := h.buildMe(r.Context(), p)
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, me)
}

// Logout is POST /v1/mobile/auth/logout: ends this device's sessions and
// drops its push tokens.
func (h *MobileHandler) Logout(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	deviceID, err := h.uc.MobileLogout(r.Context(), p.UserInternal, p.SessionID)
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	if h.push != nil && deviceID != "" {
		_ = h.push.RevokePushDevicesForDevice(r.Context(), p.UserInternal, deviceID)
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "logged_out"})
}

// LogoutAll is POST /v1/mobile/auth/logout-all: ends every session of the
// user (all devices and browsers) and drops every push token.
func (h *MobileHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if err := h.uc.MobileLogoutAll(r.Context(), p.UserInternal, p.SessionID); err != nil {
		writeMobileError(w, r, err)
		return
	}
	if h.push != nil {
		_ = h.push.RevokeAllPushDevices(r.Context(), p.UserInternal)
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "logged_out"})
}

// SwitchOrganization is POST /v1/mobile/auth/organization-context.
func (h *MobileHandler) SwitchOrganization(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OrganizationSlug string `json:"organization_slug"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	slug := strings.TrimSpace(in.OrganizationSlug)
	if slug == "" {
		response.BadRequest(w, r, response.CodeValidationError, "organization_slug is required")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	tokens, err := h.uc.MobileSwitchOrganization(r.Context(), p.UserID, p.SessionID, slug, sessionMeta(r))
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tokens)
}

type pushTokenInput struct {
	ExpoPushToken string `json:"expo_push_token"`
	Platform      string `json:"platform"`
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	AppVersion    string `json:"app_version"`
}

// PutPushToken is PUT /v1/mobile/push-token (same shape as the previous
// mobile API: expo_push_token + platform). device_id, platform and
// app_version default to the session's device.
func (h *MobileHandler) PutPushToken(w http.ResponseWriter, r *http.Request) {
	if h.push == nil {
		response.Error(w, r, http.StatusServiceUnavailable, response.CodeNotImplemented, "Push tokens are not available")
		return
	}
	var in pushTokenInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if strings.TrimSpace(in.ExpoPushToken) == "" {
		response.ValidationError(w, r, []response.Detail{{Field: "expo_push_token", Message: "required", Code: "required"}})
		return
	}
	p := authctx.MustPrincipal(r.Context())
	s, err := h.uc.MobileSession(r.Context(), p.UserInternal, p.SessionID)
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	dev := notifusecase.PushDeviceInput{
		ExpoToken: in.ExpoPushToken, DeviceID: in.DeviceID, Platform: in.Platform, AppVersion: in.AppVersion,
	}
	if strings.TrimSpace(dev.DeviceID) == "" {
		dev.DeviceID = s.Device.ID
	}
	if strings.TrimSpace(dev.Platform) == "" {
		dev.Platform = s.Device.Platform
	}
	if strings.TrimSpace(dev.AppVersion) == "" {
		dev.AppVersion = s.Device.AppVersion
	}
	out, err := h.push.RegisterPushDevice(r.Context(), p.UserInternal, dev)
	if err != nil {
		if errors.Is(err, notifusecase.ErrInvalidRequest) {
			response.BadRequest(w, r, response.CodeValidationError, err.Error())
			return
		}
		response.InternalErr(w, r, err, "Push token could not be saved")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeletePushToken is DELETE /v1/mobile/push-token {expo_push_token}.
func (h *MobileHandler) DeletePushToken(w http.ResponseWriter, r *http.Request) {
	if h.push == nil {
		response.Error(w, r, http.StatusServiceUnavailable, response.CodeNotImplemented, "Push tokens are not available")
		return
	}
	var in struct {
		ExpoPushToken string `json:"expo_push_token"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if strings.TrimSpace(in.ExpoPushToken) == "" {
		response.ValidationError(w, r, []response.Detail{{Field: "expo_push_token", Message: "required", Code: "required"}})
		return
	}
	p := authctx.MustPrincipal(r.Context())
	if err := h.push.RevokePushDevice(r.Context(), p.UserInternal, in.ExpoPushToken); err != nil {
		response.InternalErr(w, r, err, "Push token could not be deleted")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

// QRScan is GET /v1/mobile/auth/qr/{code}: what the app shows before the
// user approves (browser IP and user agent).
func (h *MobileHandler) QRScan(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.QRScan(r.Context(), r.PathValue("code"))
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// QRApprove is POST /v1/mobile/auth/qr/{code}/approve.
func (h *MobileHandler) QRApprove(w http.ResponseWriter, r *http.Request) { h.qrDecide(w, r, true) }

// QRReject is POST /v1/mobile/auth/qr/{code}/reject.
func (h *MobileHandler) QRReject(w http.ResponseWriter, r *http.Request) { h.qrDecide(w, r, false) }

func (h *MobileHandler) qrDecide(w http.ResponseWriter, r *http.Request, approve bool) {
	p := authctx.MustPrincipal(r.Context())
	out, err := h.uc.QRDecide(r.Context(), r.PathValue("code"), approve, p.UserInternal, p.SessionID)
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// QRStart is POST /v1/auth/qr/start (web, not signed in).
func (h *MobileHandler) QRStart(w http.ResponseWriter, r *http.Request) {
	meta := sessionMeta(r)
	if h.limiter != nil {
		ok, retry := h.limiter.Allow(r.Context(), "qr_login_start", meta.IP, 20, time.Minute)
		if writeRetryAfter(w, r, ok, retry) {
			return
		}
	}
	out, err := h.uc.QRStart(r.Context(), meta)
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusCreated, out)
}

// QRStatus is GET /v1/auth/qr/{code}/status (polling fallback).
func (h *MobileHandler) QRStatus(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.QRStatus(r.Context(), r.PathValue("code"))
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// QRComplete is POST /v1/auth/qr/complete {code, secret}: the panel token
// pair. The BFF calls it server side (Auth.js credentials provider); the
// panel BFF never forwards it from the browser.
func (h *MobileHandler) QRComplete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code   string `json:"code"`
		Secret string `json:"secret"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	meta := sessionMeta(r)
	if h.limiter != nil {
		ok, retry := h.limiter.Allow(r.Context(), "qr_login_complete", meta.IP, 30, time.Minute)
		if writeRetryAfter(w, r, ok, retry) {
			return
		}
	}
	out, err := h.uc.QRComplete(r.Context(), in.Code, in.Secret, meta)
	if err != nil {
		writeMobileError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}

// CreateMeasurement is POST /v1/mobile/measurements: contract only (K28);
// the schema is finalized in F3, until then the route answers 501.
func (h *MobileHandler) CreateMeasurement(w http.ResponseWriter, r *http.Request) {
	response.Error(w, r, http.StatusNotImplemented, response.CodeNotImplemented,
		"Measurement upload is not available yet (F3)")
}

func writeRetryAfter(w http.ResponseWriter, r *http.Request, ok bool, retry time.Duration) bool {
	if ok {
		return false
	}
	secs := int(retry.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	response.Error(w, r, http.StatusTooManyRequests, response.CodeRateLimited, "Too many requests, try again later")
	return true
}

func writeMobileError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrRefreshReused):
		response.Error(w, r, http.StatusUnauthorized, response.CodeRefreshTokenReused,
			"Refresh token was already used; this device was signed out")
	case errors.Is(err, usecase.ErrNotMobileSession):
		response.Error(w, r, http.StatusForbidden, response.CodeNotMobileSession, "This is not a mobile session")
	case errors.Is(err, usecase.ErrSessionRevoked):
		response.Unauthorized(w, r, "Session is no longer valid")
	case errors.Is(err, usecase.ErrQRNotFound):
		response.Error(w, r, http.StatusNotFound, response.CodeQRLoginNotFound, "QR sign-in was not found")
	case errors.Is(err, usecase.ErrQRExpired):
		response.Error(w, r, http.StatusGone, response.CodeQRLoginExpired, "QR sign-in has expired")
	case errors.Is(err, usecase.ErrQRRejected):
		response.Error(w, r, http.StatusForbidden, response.CodeQRLoginRejected, "QR sign-in was rejected")
	case errors.Is(err, usecase.ErrQRPending):
		response.Error(w, r, http.StatusConflict, response.CodeQRLoginPending, "QR sign-in is not approved yet")
	case errors.Is(err, usecase.ErrQRClosed):
		response.Error(w, r, http.StatusConflict, response.CodeQRLoginClosed, "QR sign-in was already decided or used")
	case errors.Is(err, usecase.ErrQRSecret):
		response.Error(w, r, http.StatusUnprocessableEntity, response.CodeQRLoginInvalidSecret, "QR sign-in secret is invalid")
	case errors.Is(err, usecase.ErrQRDisabled):
		response.Error(w, r, http.StatusServiceUnavailable, response.CodeQRLoginDisabled, "QR sign-in is not available")
	default:
		writeUsecaseError(w, r, err)
	}
}
