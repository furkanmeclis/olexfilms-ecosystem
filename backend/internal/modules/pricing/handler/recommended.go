package handler

// TEC-506 (F5-09b): recommended price publication, history, the price in
// force and the price discipline reads.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	pricing "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Exporter queues export jobs (exports usecase).
type Exporter interface {
	RequestExport(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ExportFormat, query ioengine.ExportQuery, locale string) (exportusecase.ExportJobView, error)
}

// Importer stores uploaded import files (imports usecase).
type Importer interface {
	Upload(ctx context.Context, actorID int64, organizationID *int64, resource string,
		format ioengine.ImportFormat, locale string, filename string, r io.Reader) (importusecase.ImportJobView, error)
	UpdateMapping(ctx context.Context, jobUUID uuid.UUID, actorID int64, orgID *int64, mapping, defaults map[string]string) (importusecase.ImportJobView, error)
	Sample(ctx context.Context, resource string, format ioengine.ImportFormat, locale string) ([]byte, string, error)
}

// RecommendedHandler serves the recommended price endpoints.
type RecommendedHandler struct {
	svc      *pricing.Recommended
	exports  Exporter
	imports  Importer
	activity *activity.Recorder
}

// NewRecommended creates the handler. exports, imports and rec may be nil.
func NewRecommended(svc *pricing.Recommended, exports Exporter, imports Importer, rec *activity.Recorder) *RecommendedHandler {
	return &RecommendedHandler{svc: svc, exports: exports, imports: imports, activity: rec}
}

func writeIOError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, importusecase.ErrInvalidRequest), errors.Is(err, exportusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		writeError(w, r, err)
	}
}

type publishRowBody struct {
	ProductUUID   uuid.UUID `json:"product_uuid"`
	Country       *string   `json:"country"`
	Currency      string    `json:"currency"`
	Price         string    `json:"price"`
	EffectiveFrom *string   `json:"effective_from"`
}

type publishBody struct {
	EffectiveFrom *string          `json:"effective_from"`
	Note          *string          `json:"note"`
	Rows          []publishRowBody `json:"rows"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Publish (POST /v1/tenant/pricing/recommended/publish).
func (h *RecommendedHandler) Publish(w http.ResponseWriter, r *http.Request) {
	var in publishBody
	if !decode(w, r, &in) {
		return
	}
	rows := make([]pricing.PublishRow, 0, len(in.Rows))
	for _, row := range in.Rows {
		id := row.ProductUUID
		pr := pricing.PublishRow{Country: str(row.Country), Currency: row.Currency, Price: row.Price, EffectiveFrom: str(row.EffectiveFrom)}
		if id != uuid.Nil {
			pr.ProductUUID = &id
		}
		rows = append(rows, pr)
	}
	out, err := h.svc.Publish(r.Context(), viewer(r), pricing.PublishInput{
		EffectiveFrom: str(in.EffectiveFrom), Note: str(in.Note), Rows: rows,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		var actor *int64
		if p, ok := authctx.PrincipalFrom(r.Context()); ok && p.UserInternal != 0 {
			id := p.UserInternal
			actor = &id
		}
		batch := out.BatchID
		h.activity.Record(r.Context(), actor, "pricing.recommended_published", "recommended_price_batches", &batch,
			map[string]any{"price_count": out.PriceCount, "applied_count": out.AppliedCount}, r)
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Import (POST /v1/tenant/pricing/recommended/import): multipart file,
// format, locale and the optional effective_from / note job defaults.
func (h *RecommendedHandler) Import(w http.ResponseWriter, r *http.Request) {
	if h.imports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "imports are not configured")
		return
	}
	v := viewer(r)
	if v.OrgType != pricing.OrgCenter || !v.RecommendedWrite {
		writeError(w, r, pricing.ErrForbidden)
		return
	}
	const maxFile = 10 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxFile+(1<<20))
	if err := r.ParseMultipartForm(maxFile); err != nil {
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
	if format != ioengine.ImportCSV && format != ioengine.ImportXLSX {
		response.ValidationError(w, r, []response.Detail{{Field: "format", Message: "must be csv or xlsx"}})
		return
	}
	locale := strings.TrimSpace(r.FormValue("locale"))
	if locale == "" {
		locale = "tr"
	}
	orgID := v.OrgID
	job, err := h.imports.Upload(r.Context(), v.UserID, &orgID, pricing.ResourceRecommendedImport, format, locale, header.Filename, file)
	if err != nil {
		writeIOError(w, r, err)
		return
	}
	defaults := map[string]string{}
	if d := strings.TrimSpace(r.FormValue("effective_from")); d != "" {
		defaults[pricing.ImportDefaultEffectiveFrom] = d
	}
	if n := strings.TrimSpace(r.FormValue("note")); n != "" {
		defaults[pricing.ImportDefaultNote] = n
	}
	if len(defaults) > 0 {
		mapping := job.Mapping
		if mapping == nil {
			mapping = map[string]string{}
		}
		if job, err = h.imports.UpdateMapping(r.Context(), job.UUID, v.UserID, &orgID, mapping, defaults); err != nil {
			writeIOError(w, r, err)
			return
		}
	}
	response.JSON(w, r, http.StatusCreated, job)
}

// ImportSample (GET /v1/tenant/pricing/recommended/import/sample).
func (h *RecommendedHandler) ImportSample(w http.ResponseWriter, r *http.Request) {
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
	data, ct, err := h.imports.Sample(r.Context(), pricing.ResourceRecommendedImport, format, locale)
	if err != nil {
		writeIOError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename=\"recommended-prices-sample."+string(format)+"\"")
	_, _ = w.Write(data)
}

func uuidList(w http.ResponseWriter, r *http.Request, name string) ([]uuid.UUID, bool) {
	var out []uuid.UUID
	for _, raw := range apiquery.CSVValues(r.URL.Query(), name) {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be a list of UUIDs", Code: "invalid"}})
			return nil, false
		}
		out = append(out, id)
	}
	return out, true
}

func currencyList(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var out []string
	for _, raw := range apiquery.CSVValues(r.URL.Query(), "currency") {
		cur, err := pricing.NormalizeCurrency(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "currency", Message: "must be a list of three-letter ISO-4217 codes", Code: "invalid"}})
			return nil, false
		}
		out = append(out, cur)
	}
	return out, true
}

// Versions (GET /v1/tenant/pricing/recommended/versions).
func (h *RecommendedHandler) Versions(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	if _, err := apiquery.ResolveSort(q.Sort, pricing.VersionSort); response.QueryValidation(w, r, err) {
		return
	}
	products, ok := uuidList(w, r, "product")
	if !ok {
		return
	}
	currencies, ok := currencyList(w, r)
	if !ok {
		return
	}
	f := pricing.VersionFilter{
		ProductUUIDs: products, Countries: apiquery.CSVValues(values, "country"), Currencies: currencies,
		Sources: apiquery.CSVValues(values, "source"), Q: q.Q, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	}
	for _, src := range f.Sources {
		if !pricing.IsVersionSource(src) {
			response.ValidationError(w, r, []response.Detail{{Field: "source", Message: "must be publish, price_list or migration", Code: "invalid"}})
			return
		}
	}
	batch, ok := queryUUID(w, r, "batch_id")
	if !ok {
		return
	}
	f.BatchID = batch
	rng, err := apiquery.DateRange(values, "effective_from")
	if response.QueryValidation(w, r, err) {
		return
	}
	f.EffectiveFrom = rng
	items, total, err := h.svc.ListVersions(r.Context(), viewer(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Current (GET /v1/tenant/pricing/recommended/current?country=&currency=).
func (h *RecommendedHandler) Current(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := apiquery.Parse(values)
	if _, err := apiquery.ResolveSort(q.Sort, pricing.CurrentSort); response.QueryValidation(w, r, err) {
		return
	}
	products, ok := uuidList(w, r, "product")
	if !ok {
		return
	}
	f := pricing.CurrentFilter{
		Country: values.Get("country"), Currency: values.Get("currency"), ProductUUIDs: products,
		Q: q.Q, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	}
	switch strings.TrimSpace(values.Get("active")) {
	case "":
	case "true":
		t := true
		f.Active = &t
	case "false":
		b := false
		f.Active = &b
	default:
		response.ValidationError(w, r, []response.Detail{{Field: "active", Message: "must be true or false"}})
		return
	}
	items, total, err := h.svc.Current(r.Context(), viewer(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Settings (GET /v1/tenant/pricing/recommended/settings): the deviation
// threshold the price screens color their badge with.
func (h *RecommendedHandler) Settings(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]int{"deviation_warning_pct": h.svc.ThresholdPct(r.Context())})
}

// disciplineScope is the caller's reach of pricing.discipline.read.
func disciplineScope(r *http.Request) (pricing.DisciplineScope, bool) {
	org := orgctx.MustScope(r.Context())
	f, ok := scopefilter.From(r.Context())
	if !ok {
		return pricing.DisciplineScope{}, false
	}
	scope := pricing.DisciplineScope{BrandID: org.BrandID}
	if f.OrgIDs != nil {
		scope.OrgIDs = append([]int64{}, f.OrgIDs...)
	}
	return scope, true
}

// Discipline (GET /v1/pricing/discipline).
func (h *RecommendedHandler) Discipline(w http.ResponseWriter, r *http.Request) {
	scope, ok := disciplineScope(r)
	if !ok {
		writeError(w, r, pricing.ErrForbidden)
		return
	}
	f, err := pricing.ParseDisciplineFilter(r.URL.Query())
	if err != nil {
		if response.QueryValidation(w, r, err) {
			return
		}
		writeError(w, r, err)
		return
	}
	items, total, _, _, err := h.svc.ListDiscipline(r.Context(), scope, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// DisciplineSummary (GET /v1/pricing/discipline/summary?date=).
func (h *RecommendedHandler) DisciplineSummary(w http.ResponseWriter, r *http.Request) {
	scope, ok := disciplineScope(r)
	if !ok {
		writeError(w, r, pricing.ErrForbidden)
		return
	}
	f, err := pricing.ParseDisciplineFilter(r.URL.Query())
	if err != nil {
		if response.QueryValidation(w, r, err) {
			return
		}
		writeError(w, r, err)
		return
	}
	out, err := h.svc.Summary(r.Context(), scope, f.Date)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DisciplineExport (POST /v1/pricing/discipline/export): {format, query,
// locale} → 202 export job.
func (h *RecommendedHandler) DisciplineExport(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	scope, ok := disciplineScope(r)
	if !ok {
		writeError(w, r, pricing.ErrForbidden)
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
	values := map[string][]string{}
	for k, v := range body.Query {
		values[k] = []string{v}
	}
	if _, err := pricing.ParseDisciplineFilter(values); err != nil {
		if response.QueryValidation(w, r, err) {
			return
		}
		writeError(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(body.Locale); ok {
		locale = string(l)
	}
	p := authctx.MustPrincipal(r.Context())
	orgID := orgctx.MustScope(r.Context()).InternalID
	job, err := h.exports.RequestExport(r.Context(), p.UserInternal, &orgID, pricing.ResourceDisciplineExport, format,
		pricing.DisciplineExportQuery(scope, values), locale)
	if err != nil {
		writeIOError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}
