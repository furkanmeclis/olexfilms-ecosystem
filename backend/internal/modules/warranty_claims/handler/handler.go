// Package handler exposes warranty claim endpoints.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type Exporter interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
}

type Handler struct {
	svc     *usecase.Service
	exports Exporter
	triage  *usecase.Triage
}

func New(svc *usecase.Service, exports Exporter) *Handler {
	return &Handler{svc: svc, exports: exports}
}

// WithTriage sets the AI triage of the manual re-trigger (TEC-392).
func (h *Handler) WithTriage(t *usecase.Triage) *Handler {
	h.triage = t
	return h
}

// AITriage is POST /v1/warranty-claims/{uuid}/ai-triage (TEC-392): re-runs
// the AI first triage and overwrites the earlier result.
func (h *Handler) AITriage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	item, err := h.svc.Retrigger(r.Context(), h.triage, caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body model.CreateInput
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.Create(r.Context(), caller(r), body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	filter, ok := listFilter(w, r)
	if !ok {
		return
	}
	out, err := h.svc.List(r.Context(), caller(r), filter)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) AddPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(usecase.MaxPhotoBytes + (1 << 20)); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart body")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		file, header, err = r.FormFile("photo")
	}
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	item, err := h.svc.AddPhoto(r.Context(), caller(r), id, usecase.PhotoInput{
		Body: file, Size: header.Size, Filename: header.Filename,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) Transition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body model.TransitionInput
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.Transition(r.Context(), caller(r), id, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) ReapplyService(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	item, err := h.svc.ReapplyService(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

// Get is GET /v1/warranty-claims/{uuid} (TEC-337: detail with the center's
// cost summary).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	item, err := h.svc.Get(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// Photo streams a claim photo (GET /v1/warranty-claims/{uuid}/photos/{photo},
// TEC-339) inside the caller's warranty_claims.read scope.
func (h *Handler) Photo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	photo, ok := pathUUID(w, r, "photo")
	if !ok {
		return
	}
	rc, size, mimeType, err := h.svc.PhotoObject(r.Context(), caller(r), id, photo)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func (h *Handler) PortalList(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	items, err := h.svc.PortalList(r.Context(), portalBrandID(r), p.UserInternal)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) FailureRateReport(w http.ResponseWriter, r *http.Request) {
	f, ok := reportFilter(w, r)
	if !ok {
		return
	}
	out, err := h.svc.FailureRateReport(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) ByDealerReport(w http.ResponseWriter, r *http.Request) {
	f, ok := reportFilter(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ByDealerReport(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) PartsReport(w http.ResponseWriter, r *http.Request) {
	f, ok := reportFilter(w, r)
	if !ok {
		return
	}
	out, err := h.svc.PartsReport(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) ExportFailureRateReport(w http.ResponseWriter, r *http.Request) {
	h.exportReport(w, r, usecase.ResourceFailureRateReport, true)
}

func (h *Handler) ExportByDealerReport(w http.ResponseWriter, r *http.Request) {
	h.exportReport(w, r, usecase.ResourceByDealerReport, false)
}

func (h *Handler) ExportPartsReport(w http.ResponseWriter, r *http.Request) {
	h.exportReport(w, r, usecase.ResourcePartsReport, false)
}

func caller(r *http.Request) usecase.Caller {
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{
		UserID: p.UserInternal, OrganizationID: org.InternalID, BrandID: org.BrandID, OrgType: org.OrgType,
		Filter: f, Permissions: p.PermissionScopes,
	}
}

func portalBrandID(r *http.Request) int64 {
	if b, ok := brandctx.From(r.Context()); ok {
		return b.ID
	}
	return 0
}

func reportFilter(w http.ResponseWriter, r *http.Request) (model.ReportFilter, bool) {
	q := r.URL.Query()
	var f model.ReportFilter
	var ok bool
	if f.From, ok = reportTime(w, r, "from"); !ok {
		return f, false
	}
	if f.To, ok = reportTime(w, r, "to"); !ok {
		return f, false
	}
	f.Group = q.Get("group")
	if f.Group == "" {
		f.Group = "product"
	}
	if f.Group != "product" && f.Group != "lot" {
		response.BadRequest(w, r, response.CodeValidationError, "group must be product or lot")
		return f, false
	}
	return f, true
}

func reportTime(w http.ResponseWriter, r *http.Request, key string) (*time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	if d, err := time.Parse(time.DateOnly, raw); err == nil {
		if key == "to" {
			d = d.AddDate(0, 0, 1)
		}
		return &d, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, key+" must be a date or RFC3339 timestamp")
		return nil, false
	}
	return &t, true
}

type exportBody struct {
	Format string            `json:"format"`
	Query  map[string]string `json:"query"`
	Locale string            `json:"locale"`
}

func (h *Handler) exportReport(w http.ResponseWriter, r *http.Request, resource string, allowGroup bool) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	var in exportBody
	if !decode(w, r, &in) {
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(in.Format)))
	if format != ioengine.ExportCSV && format != ioengine.ExportXLSX {
		response.BadRequest(w, r, response.CodeValidationError, "format must be csv or xlsx")
		return
	}
	query := ioengine.ExportQuery{}
	for _, k := range []string{usecase.QueryFrom, usecase.QueryTo} {
		if v := strings.TrimSpace(in.Query[k]); v != "" {
			query[k] = v
		}
	}
	if allowGroup {
		group := strings.TrimSpace(in.Query[usecase.QueryGroup])
		if group == "" {
			group = "product"
		}
		if group != "product" && group != "lot" {
			response.BadRequest(w, r, response.CodeValidationError, "group must be product or lot")
			return
		}
		query[usecase.QueryGroup] = group
	}
	if in.Locale == "" {
		in.Locale = "tr"
	}
	p := authctx.MustPrincipal(r.Context())
	orgID := orgctx.MustScope(r.Context()).InternalID
	job, err := h.exports.RequestExport(r.Context(), p.UserInternal, &orgID, resource, format, query, in.Locale)
	if err != nil {
		if errors.Is(err, exportusecase.ErrInvalidRequest) {
			response.BadRequest(w, r, response.CodeValidationError, err.Error())
			return
		}
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			response.BadRequest(w, r, response.CodeValidationError, "body is required")
			return false
		}
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return false
	}
	return true
}

// listFilter parses the GET /v1/warranty-claims parameters (TEC-377,
// usecase.ParseListFilter); a bad value answers 400.
func listFilter(w http.ResponseWriter, r *http.Request) (model.ListFilter, bool) {
	f, err := usecase.ParseListFilter(r.URL.Query())
	if err != nil {
		writeErr(w, r, err)
		return f, false
	}
	return f, true
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	var qe *apiquery.ValidationError
	switch {
	case errors.As(err, &qe):
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "warranty claim not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "")
	case errors.Is(err, usecase.ErrConflict):
		response.Conflict(w, r, "WARRANTY_CLAIM_EXISTS", "warranty already has a live claim")
	case errors.Is(err, usecase.ErrReapplyOpen):
		response.Conflict(w, r, "WARRANTY_CLAIM_REAPPLY_OPEN", "re-application service is still open")
	case errors.Is(err, usecase.ErrPhotoRequired):
		response.Error(w, r, http.StatusUnprocessableEntity, usecase.CodePhotoRequired, "at least one photo is required")
	case errors.Is(err, usecase.ErrUnsupportedFlow):
		response.Error(w, r, http.StatusUnprocessableEntity, "CLAIM_STATUS_FLOW", "status transition is not allowed")
	case errors.Is(err, usecase.ErrTriageDisabled):
		response.Error(w, r, http.StatusForbidden, "FEATURE_DISABLED", "the AI assistant is not enabled for the brand center")
	case errors.Is(err, usecase.ErrTriageQuota):
		response.Error(w, r, http.StatusForbidden, "AI_QUOTA_EXCEEDED", "the AI system pool of the brand center is used up")
	case errors.Is(err, usecase.ErrTriageUnavailable):
		response.Error(w, r, http.StatusServiceUnavailable, "AI_UNAVAILABLE", "the AI assistant is not available right now")
	default:
		response.InternalErr(w, r, err, "warranty claim request failed")
	}
}
