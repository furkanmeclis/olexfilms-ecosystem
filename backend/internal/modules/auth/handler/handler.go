package handler

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	githubusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/github/usecase"
	oauthproviderusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/oauthprovider/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/stepup"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler exposes auth HTTP endpoints.
type Handler struct {
	uc             *usecase.AuthUseCase
	oauth          *usecase.OAuthUseCase
	github         *githubusecase.Service
	oauthProviders *oauthproviderusecase.Service
	authSettings   usecase.AuthSettingsGate
	adapterSecret  string
	stepUp         *stepup.Service
	activity       *activity.Recorder
	limiter        *ratelimit.Limiter
}

// New creates an auth handler.
func New(
	uc *usecase.AuthUseCase,
	oauth *usecase.OAuthUseCase,
	github *githubusecase.Service,
	oauthProviders *oauthproviderusecase.Service,
	authSettings usecase.AuthSettingsGate,
	adapterSecret string,
	stepUp *stepup.Service,
	activity *activity.Recorder,
) *Handler {
	return &Handler{
		uc:             uc,
		oauth:          oauth,
		github:         github,
		oauthProviders: oauthProviders,
		authSettings:   authSettings,
		adapterSecret:  adapterSecret,
		stepUp:         stepUp,
		activity:       activity,
	}
}

// SetRateLimiter attaches Redis auth throttling (optional).
func (h *Handler) SetRateLimiter(l *ratelimit.Limiter) {
	h.limiter = l
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Surname  string `json:"surname"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if !h.allowAuthRate(w, r, func() (bool, time.Duration) {
		return h.limiter.AllowRegister(r.Context(), sessionMeta(r).IP)
	}) {
		return
	}
	user, err := h.uc.Register(r.Context(), model.RegisterInput{
		Email: in.Email, Password: in.Password, Name: in.Name, Surname: in.Surname,
	})
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{
		"user": model.ToPublicUser(user, false),
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email            string `json:"email"`
		Password         string `json:"password"`
		TOTPCode         string `json:"totp_code"`
		OrganizationSlug string `json:"organization_slug"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if h.limiter != nil {
		ok, retry := h.limiter.AllowLogin(r.Context(), sessionMeta(r).IP, in.Email)
		if h.writeRateLimited(w, r, ok, retry) {
			return
		}
	}
	tokens, err := h.uc.Login(r.Context(), in.Email, in.Password, in.TOTPCode, in.OrganizationSlug, sessionMeta(r))
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tokens)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	tokens, err := h.uc.Refresh(r.Context(), in.RefreshToken, sessionMeta(r))
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidCredentials) {
			response.Unauthorized(w, r, "Refresh token is invalid")
			return
		}
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tokens)
}

func (h *Handler) SwitchOrganizationContext(w http.ResponseWriter, r *http.Request) {
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
	tokens, err := h.uc.SwitchOrganizationContext(r.Context(), p.UserID, slug, sessionMeta(r))
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tokens)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.uc.Logout(r.Context(), in.RefreshToken); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	// Without the refresh token the access token's session is revoked, and
	// the access token stops working now (see EndAccessSession).
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		h.uc.EndAccessSession(r.Context(), in.RefreshToken, p.UserInternal, p.SessionID)
	}
	if h.stepUp != nil {
		p, ok := authctx.PrincipalFrom(r.Context())
		if ok {
			_ = h.stepUp.RevokeGrant(r.Context(), p.UserID)
		}
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	me, err := h.uc.Me(r.Context(), p.UserID, p.ImpersonatorUserID, p.OrganizationUUID)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, me)
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		Name    *string `json:"name"`
		Surname *string `json:"surname"`
		Locale  *string `json:"locale"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	me, err := h.uc.UpdateProfile(r.Context(), p.UserID, p.ImpersonatorUserID, p.OrganizationUUID, in.Name, in.Surname, in.Locale)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, me)
}

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if h.limiter != nil {
		ok, retry := h.limiter.AllowForgotPassword(r.Context(), sessionMeta(r).IP, in.Email)
		if h.writeRateLimited(w, r, ok, retry) {
			return
		}
	}
	if err := h.uc.ForgotPassword(r.Context(), in.Email); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "accepted"})
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if h.limiter != nil {
		ok, retry := h.limiter.AllowResetPassword(r.Context(), sessionMeta(r).IP)
		if h.writeRateLimited(w, r, ok, retry) {
			return
		}
	}
	if err := h.uc.ResetPassword(r.Context(), in.Email, in.Code, in.Password); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "password_reset"})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.uc.ChangePassword(r.Context(), p.UserID, in.CurrentPassword, in.NewPassword); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	if h.stepUp != nil {
		_ = h.stepUp.RevokeGrant(r.Context(), p.UserID)
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "password_changed"})
}

func (h *Handler) VerifyEmailRequest(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	var in struct {
		Email string `json:"email"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in)
	if h.limiter != nil {
		ok, retry := h.limiter.Allow(r.Context(), "verify_request", p.UserID.String(), 5, 15*time.Minute)
		if h.writeRateLimited(w, r, ok, retry) {
			return
		}
	}
	if err := h.uc.RequestEmailVerification(r.Context(), p.UserID, in.Email); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "accepted"})
}

func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if h.limiter != nil {
		ok, retry := h.limiter.AllowResetPassword(r.Context(), sessionMeta(r).IP)
		if h.writeRateLimited(w, r, ok, retry) {
			return
		}
	}
	if err := h.uc.VerifyEmail(r.Context(), in.Email, in.Code); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "verified"})
}

func (h *Handler) PlatformUsersMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformUsers())
}

func (h *Handler) ListPlatformUsers(w http.ResponseWriter, r *http.Request) {
	if banned := apiquery.ForbiddenParams(r.URL.Query()); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(r.URL.Query())
	if err := apiquery.ValidateSort(q.Sort, apiquery.UsersSort); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	items, total, err := h.uc.ListPlatformUsers(r.Context(), q.Limit, q.Offset, q.Q, status, role)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) CreatePlatformUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email     string   `json:"email"`
		Password  string   `json:"password"`
		Name      string   `json:"name"`
		Surname   string   `json:"surname"`
		Status    string   `json:"status"`
		RoleUUIDs []string `json:"role_uuids"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	roleUUIDs, err := parseUUIDList(in.RoleUUIDs)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "role_uuids contains invalid uuid")
		return
	}
	user, err := h.uc.CreatePlatformUser(r.Context(), model.CreatePlatformUserInput{
		Email: in.Email, Password: in.Password, Name: in.Name, Surname: in.Surname,
		Status: in.Status, RoleUUIDs: roleUUIDs,
	})
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, user)
}

func (h *Handler) GetPlatformUser(w http.ResponseWriter, r *http.Request) {
	uid, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "user uuid is invalid")
		return
	}
	user, err := h.uc.GetPlatformUser(r.Context(), uid)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, user)
}

func (h *Handler) PatchPlatformUser(w http.ResponseWriter, r *http.Request) {
	uid, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "user uuid is invalid")
		return
	}
	var in struct {
		Name      *string  `json:"name"`
		Surname   *string  `json:"surname"`
		Status    *string  `json:"status"`
		RoleUUIDs []string `json:"role_uuids"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	var roleUUIDs *[]uuid.UUID
	if in.RoleUUIDs != nil {
		parsed, err := parseUUIDList(in.RoleUUIDs)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "role_uuids contains invalid uuid")
			return
		}
		roleUUIDs = &parsed
	}
	user, err := h.uc.PatchPlatformUser(r.Context(), uid, model.PatchPlatformUserInput{
		Name: in.Name, Surname: in.Surname, Status: in.Status, RoleUUIDs: roleUUIDs,
	})
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, user)
}

func (h *Handler) SetPlatformUserPassword(w http.ResponseWriter, r *http.Request) {
	uid, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "user uuid is invalid")
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.uc.SetPlatformUserPassword(r.Context(), uid, in.Password); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "password_set"})
}

func (h *Handler) PlatformRolesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformRoles())
}

func (h *Handler) ListPlatformRoles(w http.ResponseWriter, r *http.Request) {
	if banned := apiquery.ForbiddenParams(r.URL.Query()); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.uc.ListPlatformRoles(r.Context(), q.Limit, q.Offset, q.Q)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) CreatePlatformRole(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name            string            `json:"name"`
		Slug            string            `json:"slug"`
		Description     *string           `json:"description"`
		PermissionSlugs []string          `json:"permission_slugs"`
		Grants          map[string]string `json:"grants"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	role, err := h.uc.CreatePlatformRole(r.Context(), model.CreateRoleInput{
		Name: in.Name, Slug: in.Slug, Description: in.Description, PermissionSlugs: in.PermissionSlugs,
		Grants: in.Grants,
	})
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, role)
}

func (h *Handler) GetPlatformRole(w http.ResponseWriter, r *http.Request) {
	rid, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "role uuid is invalid")
		return
	}
	role, err := h.uc.GetPlatformRole(r.Context(), rid)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, role)
}

func (h *Handler) PatchPlatformRole(w http.ResponseWriter, r *http.Request) {
	rid, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "role uuid is invalid")
		return
	}
	var in struct {
		Name            *string            `json:"name"`
		Description     *string            `json:"description"`
		PermissionSlugs []string           `json:"permission_slugs"`
		Grants          *map[string]string `json:"grants"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	var permSlugs *[]string
	if in.PermissionSlugs != nil {
		permSlugs = &in.PermissionSlugs
	}
	role, err := h.uc.PatchPlatformRole(r.Context(), rid, model.PatchRoleInput{
		Name: in.Name, Description: in.Description, PermissionSlugs: permSlugs, Grants: in.Grants,
	})
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, role)
}

func (h *Handler) DeletePlatformRole(w http.ResponseWriter, r *http.Request) {
	rid, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "role uuid is invalid")
		return
	}
	if err := h.uc.DeletePlatformRole(r.Context(), rid); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) ListPlatformPermissions(w http.ResponseWriter, r *http.Request) {
	if banned := apiquery.ForbiddenParams(r.URL.Query()); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.uc.ListPlatformPermissions(r.Context(), q.Limit, q.Offset, q.Q)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func parseUUIDList(raw []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return err
	}
	return nil
}

func sessionMeta(r *http.Request) model.SessionMeta {
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		ip = strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			ip = host
		} else {
			ip = r.RemoteAddr
		}
	}
	return model.SessionMeta{UserAgent: r.UserAgent(), IP: ip}
}

func writeUsecaseError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *apiquery.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrInvalidCredentials):
		response.Error(w, r, http.StatusUnauthorized, response.CodeInvalidCredentials, "Email or password is incorrect")
	case errors.Is(err, usecase.ErrMFARequired):
		response.Error(w, r, http.StatusForbidden, response.CodeMFARequired, "Authenticator code required")
	case errors.Is(err, usecase.ErrMFANotEnrolled):
		response.Error(w, r, http.StatusForbidden, response.CodeMFANotEnrolled, "Two-factor authentication must be enabled before password sign-in")
	case errors.Is(err, usecase.ErrInvalidMFACode):
		response.Error(w, r, http.StatusUnauthorized, response.CodeInvalidMFACode, "Authenticator code is invalid")
	case errors.Is(err, usecase.ErrTOTPAlreadyOn):
		response.Conflict(w, r, response.CodeConflict, "Authenticator is already enabled")
	case errors.Is(err, usecase.ErrTOTPSetupNeeded):
		response.BadRequest(w, r, response.CodeValidationError, "Start authenticator setup first")
	case errors.Is(err, usecase.ErrTOTPNotEnabled):
		response.BadRequest(w, r, response.CodeValidationError, "Authenticator is not enabled")
	case errors.Is(err, usecase.ErrInvalidResetCode):
		response.BadRequest(w, r, response.CodeInvalidResetCode, "Reset code is invalid or expired")
	case errors.Is(err, usecase.ErrInvalidVerifyCode):
		response.BadRequest(w, r, response.CodeInvalidVerifyCode, "Verification code is invalid or expired")
	case errors.Is(err, usecase.ErrPasswordResetFail):
		response.Error(w, r, http.StatusInternalServerError, response.CodePasswordResetFailed, "Password reset failed")
	case errors.Is(err, usecase.ErrUserDisabled):
		response.Forbidden(w, r, "User is disabled")
	case errors.Is(err, usecase.ErrLastSuperAdmin):
		response.Conflict(w, r, response.CodeConflict, "Cannot demote the last super admin")
	case errors.Is(err, usecase.ErrSystemRole):
		response.Conflict(w, r, response.CodeConflict, "System role cannot be modified")
	case errors.Is(err, usecase.ErrAlreadyImpersonating):
		response.Conflict(w, r, response.CodeConflict, "Stop the current impersonation session first")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "You do not have access to this resource")
	case errors.Is(err, usecase.ErrNoTenantMembership):
		response.Error(w, r, http.StatusForbidden, response.CodeNoTenantMembership, "No membership for this organization")
	case errors.Is(err, usecase.ErrOrganizationAccessExpired):
		response.Error(w, r, http.StatusForbidden, response.CodeOrganizationAccessExpired, "Organization access has expired")
	case errors.Is(err, usecase.ErrOrganizationSuspended):
		response.Forbidden(w, r, "Organization is suspended")
	case errors.Is(err, usecase.ErrBrandMismatch):
		response.Error(w, r, http.StatusForbidden, response.CodeBrandMismatch, "Organization does not belong to this domain's brand")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, usecase.ErrConflict):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, usecase.ErrInvalidRequest), errors.Is(err, password.ErrInvalidPassword):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
