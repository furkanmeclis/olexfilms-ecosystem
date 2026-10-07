package handler

// TEC-389 (F4-01g): platform AI settings, the organization quota table and
// the usage report (panel and platform) with its export.

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// maxSettingsBodyBytes fits the 20 KB knowledge text and 20 000 character
// extra instructions in any script.
const maxSettingsBodyBytes = 256 << 10

// Exports queues export jobs (*exportusecase.Service).
type Exports interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
}

// Admin serves the settings, quota and usage routes.
type Admin struct {
	admin   *usecase.Admin
	exports Exports
}

// NewAdmin creates the handler (nil exports: export routes answer 503).
func NewAdmin(admin *usecase.Admin, exports Exports) *Admin {
	return &Admin{admin: admin, exports: exports}
}

// GetSettings (GET /v1/platform/ai/settings).
func (h *Admin) GetSettings(w http.ResponseWriter, r *http.Request) {
	out, err := h.admin.GetSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PutSettings (PUT /v1/platform/ai/settings): fields left out keep their
// value.
func (h *Admin) PutSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsBodyBytes)
	var in usecase.SettingsInput
	if !decode(w, r, &in, false) {
		return
	}
	out, err := h.admin.UpdateSettings(r.Context(), authctx.MustPrincipal(r.Context()).UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListOrgs (GET /v1/platform/ai/orgs): list contract, sort usage | quota |
// name (default -usage), org_type (CSV), q, period (YYYY-MM).
func (h *Admin) ListOrgs(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	types, err := apiquery.EnumList(values, "org_type", usecase.OrgTypes...)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.admin.ListOrgQuotas(r.Context(), usecase.OrgQuotaListFilter{
		Period: values.Get("period"), OrgTypes: types, Q: q.Q, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// PutOrg (PUT /v1/platform/ai/orgs/{uuid}): quota override and switch.
func (h *Admin) PutOrg(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in usecase.OrgQuotaInput
	if !decode(w, r, &in, false) {
		return
	}
	out, err := h.admin.UpdateOrgQuota(r.Context(), authctx.MustPrincipal(r.Context()).UserInternal, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// usageOrg resolves the panel report organization (`organization` query
// parameter, default the active one) inside the ai.usage.read reach.
func (h *Admin) usageOrg(r *http.Request, raw string) (db.Organization, error) {
	org, ok := orgctx.ScopeFrom(r.Context())
	if !ok {
		return db.Organization{}, usecase.ErrForbidden
	}
	f, ok := scopefilter.From(r.Context())
	if !ok {
		return db.Organization{}, usecase.ErrForbidden
	}
	var target *uuid.UUID
	if raw = strings.TrimSpace(raw); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return db.Organization{}, usecase.ErrNotFound
		}
		target = &id
	}
	return h.admin.ResolveUsageOrg(r.Context(), f, org, target)
}

// ListUsage (GET /v1/ai/usage): one organization's ledger.
func (h *Admin) ListUsage(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	uq, err := usecase.ParseUsageQuery(values, false)
	if err != nil {
		writeError(w, r, err)
		return
	}
	org, err := h.usageOrg(r, values.Get("organization"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.admin.ListUsage(r.Context(), []int64{org.ID}, uq)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, uq.Limit, uq.Offset))
}

// UsageSummary (GET /v1/ai/usage/summary?period=YYYY-MM).
func (h *Admin) UsageSummary(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	row, err := h.usageOrg(r, values.Get("organization"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.admin.Summary(r.Context(), row, values.Get("period"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PlatformListUsage (GET /v1/platform/ai/usage): every organization.
func (h *Admin) PlatformListUsage(w http.ResponseWriter, r *http.Request) {
	uq, err := usecase.ParseUsageQuery(r.URL.Query(), true)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.admin.ListUsage(r.Context(), nil, uq)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, uq.Limit, uq.Offset))
}

type exportBody struct {
	Format string            `json:"format"`
	Query  map[string]string `json:"query"`
	Locale string            `json:"locale"`
}

// ExportUsage (POST /v1/ai/usage/export, 202).
func (h *Admin) ExportUsage(w http.ResponseWriter, r *http.Request) {
	h.export(w, r, false)
}

// PlatformExportUsage (POST /v1/platform/ai/usage/export, 202).
func (h *Admin) PlatformExportUsage(w http.ResponseWriter, r *http.Request) {
	h.export(w, r, true)
}

func (h *Admin) export(w http.ResponseWriter, r *http.Request, platform bool) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	var b exportBody
	if !decode(w, r, &b, false) {
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(b.Format)))
	if format != ioengine.ExportCSV && format != ioengine.ExportXLSX {
		response.ValidationError(w, r, []response.Detail{{Field: "format", Message: "must be csv or xlsx"}})
		return
	}
	var jobOrg *int64
	var target int64
	if !platform {
		org, err := h.usageOrg(r, b.Query["organization"])
		if err != nil {
			writeError(w, r, err)
			return
		}
		target = org.ID
		active, _ := orgctx.ScopeFrom(r.Context())
		jobOrg = &active.InternalID
	}
	query, err := usecase.UsageExportQuery(b.Query, target)
	if err != nil {
		writeError(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(b.Locale); ok {
		locale = string(l)
	}
	job, err := h.exports.RequestExport(r.Context(), authctx.MustPrincipal(r.Context()).UserInternal, jobOrg,
		usecase.ResourceUsageExport, format, query, locale)
	if errors.Is(err, exportusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}
