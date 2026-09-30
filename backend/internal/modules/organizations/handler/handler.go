package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	authmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct {
	svc     *orgusecase.Service
	auth    *authusecase.AuthUseCase
	store   storage.Driver
	limiter *ratelimit.Limiter
}

// SetRateLimiter enables per-IP limits on public business registration.
func (h *Handler) SetRateLimiter(l *ratelimit.Limiter) {
	h.limiter = l
}

func New(svc *orgusecase.Service, auth *authusecase.AuthUseCase, store storage.Driver) *Handler {
	return &Handler{svc: svc, auth: auth, store: store}
}

func (h *Handler) PublicRegister(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name             string `json:"name"`
		Surname          string `json:"surname"`
		Email            string `json:"email"`
		Password         string `json:"password"`
		OrganizationName string `json:"organization_name"`
		City             string `json:"city"`
		District         string `json:"district"`
		Phone            string `json:"phone"`
		Address          string `json:"address"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if h.limiter != nil {
		if ok, retry := h.limiter.AllowRegister(r.Context(), sessionMeta(r).IP); !ok {
			if retry > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			}
			response.TooManyRequests(w, r, "Too many attempts. Try again later.")
			return
		}
	}
	result, err := h.svc.Register(r.Context(), orgusecase.RegisterInput{
		Name: in.Name, Surname: in.Surname, Email: in.Email, Password: in.Password,
		OrganizationName: in.OrganizationName, City: in.City, District: in.District,
		Phone: in.Phone, Address: in.Address,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	tokens, err := h.auth.IssueSessionForOrganization(r.Context(), result.OwnerUUID, result.Organization.UUID, sessionMeta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	user, err := h.auth.LoadUserByUUID(r.Context(), result.OwnerUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{
		"user":         authmodel.ToPublicUser(user, false),
		"organization": result.Organization,
		"tokens":       tokens,
	})
}

func (h *Handler) PublicBySlug(w http.ResponseWriter, r *http.Request) {
	slugValue := strings.TrimSpace(r.PathValue("slug"))
	org, err := h.svc.GetPublicBySlug(r.Context(), slugValue)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, org)
}

func (h *Handler) PublicStreamLogo(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "organization uuid is invalid")
		return
	}
	key, err := h.svc.LogoObjectKey(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	rc, _, err := h.store.Download(r.Context(), key)
	if err != nil {
		response.NotFound(w, r, "logo not found")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", storage.MIMEFromLogoKey(key))
	_, _ = io.Copy(w, rc)
}

func (h *Handler) PlatformMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformOrganizations())
}

func (h *Handler) PlatformList(w http.ResponseWriter, r *http.Request) {
	if banned := apiquery.ForbiddenParams(r.URL.Query()); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(r.URL.Query())
	if err := apiquery.ValidateSort(q.Sort, apiquery.TenantsSort); err != nil {
		writeError(w, r, err)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	items, total, err := h.svc.List(r.Context(), q.Limit, q.Offset, q.Q, status)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) PlatformCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name          string `json:"name"`
		City          string `json:"city"`
		District      string `json:"district"`
		Phone         string `json:"phone"`
		Address       string `json:"address"`
		OwnerUserUUID string `json:"owner_user_uuid"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	ownerUUID, err := uuid.Parse(strings.TrimSpace(in.OwnerUserUUID))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "owner_user_uuid is invalid")
		return
	}
	owner, err := h.auth.LoadUserByUUID(r.Context(), ownerUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result, err := h.svc.RegisterOrganization(r.Context(), orgusecase.RegisterInput{
		OrganizationName: in.Name,
		City:             in.City,
		District:         in.District,
		Phone:            in.Phone,
		Address:          in.Address,
	}, owner.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, result.Organization)
}

func (h *Handler) PlatformGet(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "organization uuid is invalid")
		return
	}
	org, err := h.svc.GetByUUID(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	members, err := h.svc.ListMembers(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"organization": org,
		"members":      members,
	})
}

func (h *Handler) PlatformPatch(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "organization uuid is invalid")
		return
	}
	var in struct {
		Name           *string `json:"name"`
		City           *string `json:"city"`
		District       *string `json:"district"`
		Phone          *string `json:"phone"`
		Address        *string `json:"address"`
		Status         *string `json:"status"`
		PlanCode       *string `json:"plan_code"`
		AccessStartsAt *string `json:"access_starts_at"`
		AccessEndsAt   *string `json:"access_ends_at"`
		ClearAccessEnd *bool   `json:"clear_access_ends_at"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	patch := orgusecase.PatchInput{
		Name: in.Name, City: in.City, District: in.District, Phone: in.Phone,
		Address: in.Address, Status: in.Status, PlanCode: in.PlanCode,
	}
	if in.AccessStartsAt != nil {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*in.AccessStartsAt))
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "access_starts_at must be RFC3339")
			return
		}
		patch.AccessStartsAt = &t
	}
	if in.ClearAccessEnd != nil && *in.ClearAccessEnd {
		patch.ClearAccessEnd = true
	} else if in.AccessEndsAt != nil {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*in.AccessEndsAt))
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "access_ends_at must be RFC3339")
			return
		}
		patch.AccessEndsAt = &t
	}
	org, err := h.svc.Patch(r.Context(), id, patch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, org)
}

func (h *Handler) PlatformUploadLogo(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "organization uuid is invalid")
		return
	}
	// Cap the whole body: ParseMultipartForm only bounds memory and spills
	// the rest to temp files.
	r.Body = http.MaxBytesReader(w, r.Body, (3<<20)+(1<<20))
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("logo")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "logo is required")
		return
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime, err := storage.DetectLogoMIME(header.Header.Get("Content-Type"), head[:n])
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err := storage.ValidateLogoSize(header.Size); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	key := storage.TenantLogoObjectKey(id, ext)
	body := io.MultiReader(strings.NewReader(string(head[:n])), file)
	if err := h.store.Upload(r.Context(), storage.File{
		Body: body, Size: header.Size, ContentType: mime, Filename: header.Filename,
	}, key); err != nil {
		response.InternalErr(w, r, err, "logo upload failed")
		return
	}
	org, err := h.svc.SetLogo(r.Context(), id, key)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, org)
}

func (h *Handler) PlatformDeleteLogo(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "organization uuid is invalid")
		return
	}
	if key, err := h.svc.LogoObjectKey(r.Context(), id); err == nil {
		_ = h.store.Delete(r.Context(), key)
	}
	org, err := h.svc.ClearLogo(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, org)
}

func (h *Handler) TenantGetSettings(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	s, err := h.svc.GetTenantSettings(r.Context(), scope.InternalID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, s)
}

func (h *Handler) TenantPatchSettings(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	var in orgusecase.LetterheadPatch
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	s, err := h.svc.PatchTenantSettings(r.Context(), scope.InternalID, in)
	if err != nil {
		if errors.Is(err, orgusecase.ErrInvalidRequest) {
			response.BadRequest(w, r, response.CodeValidationError, "invalid letterhead settings")
			return
		}
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, s)
}

func (h *Handler) TenantUploadLogo(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	// Cap the whole body: ParseMultipartForm only bounds memory and spills
	// the rest to temp files.
	r.Body = http.MaxBytesReader(w, r.Body, (3<<20)+(1<<20))
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("logo")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "logo is required")
		return
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime, err := storage.DetectLogoMIME(header.Header.Get("Content-Type"), head[:n])
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err := storage.ValidateLogoSize(header.Size); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	key := storage.TenantLogoObjectKey(scope.UUID, ext)
	body := io.MultiReader(strings.NewReader(string(head[:n])), file)
	if err := h.store.Upload(r.Context(), storage.File{
		Body: body, Size: header.Size, ContentType: mime, Filename: header.Filename,
	}, key); err != nil {
		response.InternalErr(w, r, err, "logo upload failed")
		return
	}
	if _, err := h.svc.SetLogo(r.Context(), scope.UUID, key); err != nil {
		writeError(w, r, err)
		return
	}
	s, err := h.svc.GetTenantSettings(r.Context(), scope.InternalID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, s)
}

func (h *Handler) TenantDeleteLogo(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	if key, err := h.svc.LogoObjectKey(r.Context(), scope.UUID); err == nil {
		_ = h.store.Delete(r.Context(), key)
	}
	if _, err := h.svc.ClearLogo(r.Context(), scope.UUID); err != nil {
		writeError(w, r, err)
		return
	}
	s, err := h.svc.GetTenantSettings(r.Context(), scope.InternalID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, s)
}

func (h *Handler) PlatformAddMember(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "organization uuid is invalid")
		return
	}
	var in struct {
		UserUUID string `json:"user_uuid"`
		Role     string `json:"role"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	userUUID, err := uuid.Parse(strings.TrimSpace(in.UserUUID))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "user_uuid is invalid")
		return
	}
	if err := h.svc.AddMember(r.Context(), id, orgusecase.AddMemberInput{
		UserUUID: userUUID, Role: in.Role,
	}); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]string{"status": "ok"})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, orgusecase.ErrNotFound):
		response.NotFound(w, r, "Organization was not found")
	case errors.Is(err, orgusecase.ErrConflict):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, orgusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, orgusecase.ErrNoTenantMembership):
		response.Error(w, r, http.StatusForbidden, response.CodeNoTenantMembership, "No membership for this organization")
	case errors.Is(err, orgusecase.ErrOrganizationAccessExpired):
		response.Error(w, r, http.StatusForbidden, response.CodeOrganizationAccessExpired, "Organization access has expired")
	case errors.Is(err, orgusecase.ErrOrganizationSuspended):
		response.Forbidden(w, r, "Organization is suspended")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func sessionMeta(r *http.Request) authmodel.SessionMeta {
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		ip = strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if ip == "" {
		ip = r.RemoteAddr
	}
	return authmodel.SessionMeta{UserAgent: r.UserAgent(), IP: ip}
}
