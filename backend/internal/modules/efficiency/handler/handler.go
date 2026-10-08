package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/efficiency/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
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

type Importer interface {
	Upload(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ImportFormat, locale string, filename string, r io.Reader) (importusecase.ImportJobView, error)
	Sample(ctx context.Context, resource string, format ioengine.ImportFormat, locale string) ([]byte, string, error)
}

// Settings reads the efficiency system settings (sysconfig).
type Settings interface {
	EfficiencyWarningWasteRatio(ctx context.Context) string
}

type Handler struct {
	svc      *usecase.Service
	exports  Exporter
	imports  Importer
	settings Settings
}

func New(svc *usecase.Service, exports Exporter, imports Importer) *Handler {
	return &Handler{svc: svc, exports: exports, imports: imports}
}

// WithSettings wires the sysconfig reader for GET /v1/efficiency/settings.
func (h *Handler) WithSettings(s Settings) *Handler {
	h.settings = s
	return h
}

func caller(r *http.Request) usecase.Caller {
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
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
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "Efficiency access denied")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Efficiency resource not found")
	default:
		response.InternalErr(w, r, err, "efficiency request failed")
	}
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	f, ok := analyticsFilter(w, r)
	if !ok {
		return
	}
	dim := strings.TrimSpace(r.URL.Query().Get("dimension"))
	if dim == "" {
		dim = "dealer"
	}
	rows, total, err := h.svc.Summary(r.Context(), caller(r), dim, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(rows, total, f.Limit, f.Offset))
}

func (h *Handler) Trend(w http.ResponseWriter, r *http.Request) {
	f, ok := analyticsFilter(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.Trend(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rows)
}

func (h *Handler) Compare(w http.ResponseWriter, r *http.Request) {
	f, ok := analyticsFilter(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.Compare(r.Context(), caller(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rows)
}

func (h *Handler) Rolls(w http.ResponseWriter, r *http.Request) {
	f, ok := rollFilter(w, r)
	if !ok {
		return
	}
	rows, total, err := h.svc.Rolls(r.Context(), caller(r), f, nil)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(rows, total, f.Limit, f.Offset))
}

func (h *Handler) Roll(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("unit_uuid"))
	if err != nil {
		response.NotFound(w, r, "Efficiency roll not found")
		return
	}
	row, err := h.svc.Roll(r.Context(), caller(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, row)
}

// Settings (GET /v1/efficiency/settings): the read-only analysis settings
// the screens need, e.g. the waste ratio above which rows are highlighted
// (TEC-489).
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	ratio := usecase.DefaultWarningWasteRatio
	if h.settings != nil {
		if v := strings.TrimSpace(h.settings.EfficiencyWarningWasteRatio(r.Context())); v != "" {
			ratio = v
		}
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"warning_waste_ratio": ratio})
}

func (h *Handler) Expectations(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	rows, total, err := h.svc.ListExpectations(r.Context(), caller(r), usecase.ExpectationFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, Sort: q.Sort,
		Source: r.URL.Query().Get("source"), BodyType: r.URL.Query().Get("body_type"),
		PartKeys: apiquery.CSVValues(r.URL.Query(), "part_key"),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(rows, total, q.Limit, q.Offset))
}

func (h *Handler) CreateExpectation(w http.ResponseWriter, r *http.Request) {
	var in usecase.ExpectationInput
	if !decode(w, r, &in) {
		return
	}
	row, err := h.svc.CreateExpectation(r.Context(), caller(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, row)
}

func (h *Handler) UpdateExpectation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Expectation not found")
		return
	}
	var in usecase.ExpectationInput
	if !decode(w, r, &in) {
		return
	}
	row, err := h.svc.UpdateExpectation(r.Context(), caller(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, row)
}

func (h *Handler) DeleteExpectation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Expectation not found")
		return
	}
	if err := h.svc.DeleteExpectation(r.Context(), caller(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ImportExpectations(w http.ResponseWriter, r *http.Request) {
	if h.imports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "imports are not configured")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, (10<<20)+(1<<20))
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	format := ioengine.ImportFormat(strings.ToLower(strings.TrimSpace(r.FormValue("format"))))
	if format == "" {
		format = ioengine.ImportCSV
	}
	locale := strings.TrimSpace(r.FormValue("locale"))
	if locale == "" {
		locale = "tr"
	}
	orgID := org.InternalID
	job, err := h.imports.Upload(r.Context(), p.UserInternal, &orgID, usecase.ResourceExpectations, format, locale, header.Filename, file)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, job)
}

func (h *Handler) ImportExpectationsSample(w http.ResponseWriter, r *http.Request) {
	if h.imports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "imports are not configured")
		return
	}
	format := ioengine.ImportFormat(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))))
	if format == "" {
		format = ioengine.ImportXLSX
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	if locale == "" {
		locale = "tr"
	}
	data, ct, err := h.imports.Sample(r.Context(), usecase.ResourceExpectations, format, locale)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename=\"part-consumption-expectations-sample."+string(format)+"\"")
	_, _ = w.Write(data)
}

func (h *Handler) ExportSummary(w http.ResponseWriter, r *http.Request) {
	h.export(w, r, usecase.ResourceSummaryExport)
}

func (h *Handler) ExportRolls(w http.ResponseWriter, r *http.Request) {
	h.export(w, r, usecase.ResourceRollsExport)
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request, resource string) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	var body struct {
		Format string            `json:"format"`
		Query  map[string]string `json:"query"`
		Locale string            `json:"locale"`
	}
	if !decode(w, r, &body) {
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(body.Format)))
	if format != ioengine.ExportCSV && format != ioengine.ExportXLSX && format != ioengine.ExportPDF {
		response.ValidationError(w, r, []response.Detail{{Field: "format", Message: "must be csv, xlsx or pdf"}})
		return
	}
	c := caller(r)
	query, err := usecase.ExportQuery(c, body.Query)
	if err != nil {
		writeError(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(body.Locale); ok {
		locale = string(l)
	}
	orgID := c.Org.InternalID
	job, err := h.exports.RequestExport(r.Context(), c.Principal.UserInternal, &orgID, resource, format, query, locale)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}

func analyticsFilter(w http.ResponseWriter, r *http.Request) (usecase.AnalyticsFilter, bool) {
	q := apiquery.Parse(r.URL.Query())
	from, ok := parseDay(w, r, "period_from")
	if !ok {
		return usecase.AnalyticsFilter{}, false
	}
	to, ok := parseDay(w, r, "period_to")
	if !ok {
		return usecase.AnalyticsFilter{}, false
	}
	waste, err := apiquery.NumRange(r.URL.Query(), "waste_ratio")
	if err != nil {
		writeError(w, r, err)
		return usecase.AnalyticsFilter{}, false
	}
	return usecase.AnalyticsFilter{
		From: from, To: to.AddDate(0, 0, 1), Limit: q.Limit, Offset: q.Offset, Sort: q.Sort,
		WasteRatioMin: waste.Min, WasteRatioMax: waste.Max,
	}, true
}

func rollFilter(w http.ResponseWriter, r *http.Request) (usecase.RollFilter, bool) {
	q := apiquery.Parse(r.URL.Query())
	waste, err := apiquery.NumRange(r.URL.Query(), "waste_ratio")
	if err != nil {
		writeError(w, r, err)
		return usecase.RollFilter{}, false
	}
	used, err := apiquery.DateRange(r.URL.Query(), "last_used")
	if err != nil {
		writeError(w, r, err)
		return usecase.RollFilter{}, false
	}
	return usecase.RollFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, Sort: q.Sort,
		WasteRatioMin: waste.Min, WasteRatioMax: waste.Max, LastUsedFrom: used.From, LastUsedTo: used.Before,
	}, true
}

func parseDay(w http.ResponseWriter, r *http.Request, key string) (time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		if key == "period_to" {
			return time.Now().UTC(), true
		}
		return time.Now().UTC().AddDate(0, -1, 0), true
	}
	t, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: key, Message: "must be YYYY-MM-DD"}})
		return time.Time{}, false
	}
	return t.UTC(), true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}
