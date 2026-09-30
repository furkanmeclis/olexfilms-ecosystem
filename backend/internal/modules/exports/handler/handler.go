package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler exposes export HTTP endpoints.
type Handler struct {
	svc *exportusecase.Service
}

func New(svc *exportusecase.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	q := apiquery.Parse(r.URL.Query())
	admin := p.HasPermission(rbac.PermPlatformSettingsWrite)
	items, total, err := h.svc.ListJobs(r.Context(), p.UserInternal, admin, q.Limit, q.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	admin := p.HasPermission(rbac.PermPlatformSettingsWrite)
	job, err := h.svc.GetJob(r.Context(), id, p.UserInternal, admin)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, job)
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	admin := p.HasPermission(rbac.PermPlatformSettingsWrite)
	file, ct, filename, err := h.svc.Download(r.Context(), id, p.UserInternal, admin)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", contentDispositionAttachment(filename))
	_, _ = io.Copy(w, file)
}

func contentDispositionAttachment(filename string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, filename)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(filename))
}

func (h *Handler) RequestUsersExport(w http.ResponseWriter, r *http.Request) {
	h.requestExport(w, r, "platform.users", rbac.PermPlatformUsersExport)
}

func (h *Handler) RequestRolesExport(w http.ResponseWriter, r *http.Request) {
	h.requestExport(w, r, "platform.roles", rbac.PermPlatformRolesExport)
}

func (h *Handler) RequestNotificationsExport(w http.ResponseWriter, r *http.Request) {
	h.requestExport(w, r, "platform.notifications", rbac.PermPlatformNotificationsExport)
}

func (h *Handler) RequestActivityExport(w http.ResponseWriter, r *http.Request) {
	h.requestExport(w, r, "platform.activity", rbac.PermPlatformActivityRead)
}

func (h *Handler) requestExport(w http.ResponseWriter, r *http.Request, resource, perm string) {
	p := authctx.MustPrincipal(r.Context())
	if !p.HasPermission(perm) {
		response.Forbidden(w, r, "export not allowed")
		return
	}
	var in struct {
		Format string            `json:"format"`
		Query  map[string]string `json:"query"`
		Locale string            `json:"locale"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(in.Format)))
	if in.Locale == "" {
		in.Locale = "tr"
	}
	query := ioengine.ExportQuery(in.Query)
	if query == nil {
		query = ioengine.ExportQuery{}
	}
	for k, vals := range r.URL.Query() {
		if k == "q" || k == "status" || k == "role" || k == "channel" || k == "scope" || k == "user_uuid" {
			if len(vals) > 0 {
				query[k] = vals[0]
			}
		}
	}
	if resource == "platform.notifications" {
		constrainNotificationExportQuery(p, query)
	}
	job, err := h.svc.RequestExport(r.Context(), p.UserInternal, nil, resource, format, query, in.Locale)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}

func (h *Handler) ListTenant(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListOrgJobs(r.Context(), scope.InternalID, q.Limit, q.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) GetTenant(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	job, err := h.svc.GetOrgJob(r.Context(), id, scope.InternalID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, job)
}

func (h *Handler) DownloadTenant(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.MustScope(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	file, ct, filename, err := h.svc.DownloadOrg(r.Context(), id, scope.InternalID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", contentDispositionAttachment(filename))
	_, _ = io.Copy(w, file)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, exportusecase.ErrNotFound):
		response.NotFound(w, r, "export job not found")
	case errors.Is(err, exportusecase.ErrForbidden):
		response.Forbidden(w, r, "forbidden")
	case errors.Is(err, exportusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "unexpected error")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return err
	}
	return nil
}

func constrainNotificationExportQuery(p authctx.Principal, query ioengine.ExportQuery) {
	if !p.HasPermission(rbac.PermPlatformNotificationsReadAll) {
		query["user_uuid"] = p.UserID.String()
		delete(query, "scope")
		return
	}
	if strings.TrimSpace(query["user_uuid"]) != "" {
		delete(query, "scope")
		return
	}
	if strings.EqualFold(strings.TrimSpace(query["scope"]), "all") {
		delete(query, "user_uuid")
		return
	}
	query["user_uuid"] = p.UserID.String()
	delete(query, "scope")
}
