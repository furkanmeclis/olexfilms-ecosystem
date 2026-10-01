// Package handler serves the TEC-145 catalog endpoints under /v1/catalog.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
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
	Sample(ctx context.Context, resource string, format ioengine.ImportFormat, locale string) ([]byte, string, error)
}

// Handler serves catalog endpoints.
type Handler struct {
	svc      *catalogusecase.Service
	exports  Exporter
	imports  Importer
	store    storage.Driver
	activity *activity.Recorder
}

// New creates the handler. exports, imports, store and rec may be nil
// (store nil: image uploads answer 503, public images 404).
func New(svc *catalogusecase.Service, exports Exporter, imports Importer, store storage.Driver, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, exports: exports, imports: imports, store: store, activity: rec}
}

const maxBody = 1 << 20

// readBody decodes a JSON body strictly and also returns the top-level keys
// it names (PATCH null handling).
func readBody(w http.ResponseWriter, r *http.Request, dst any) (map[string]json.RawMessage, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return nil, false
	}
	keys := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &keys)
	return keys, true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var verr *catalogusecase.ValidationError
	var conflict *catalogusecase.ConflictError
	switch {
	case errors.As(err, &verr):
		details := make([]response.Detail, 0, len(verr.Fields))
		for _, f := range verr.Fields {
			details = append(details, response.Detail{Field: f.Field, Message: f.Message, Code: f.Code})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, response.CodeValidationError, "Invalid catalog input", details)
	case errors.As(err, &conflict):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, "A record with this "+conflict.Field+" already exists",
			[]response.Detail{{Field: conflict.Field, Message: "already exists in this brand", Code: "duplicate"}})
	case errors.Is(err, catalogusecase.ErrCenterOnly):
		response.Forbidden(w, r, "Only the center organization writes the catalog")
	case errors.Is(err, catalogusecase.ErrNotFound):
		response.NotFound(w, r, "Catalog record not found")
	case errors.Is(err, catalogusecase.ErrImagesChanged):
		response.Conflict(w, r, response.CodeConflict, "The product images changed; reload and try again")
	case errors.Is(err, catalogusecase.ErrInUse):
		response.Conflict(w, r, response.CodeConflict, "The record is still in use")
	case errors.Is(err, exportusecase.ErrInvalidRequest), errors.Is(err, importusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "catalog request failed")
	}
}

func (h *Handler) record(r *http.Request, action string, id *uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	var actor *int64
	if p, ok := authctx.PrincipalFrom(r.Context()); ok {
		v := p.UserInternal
		actor = &v
	}
	h.activity.Record(r.Context(), actor, action, "catalog", id, payload, r)
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func queryBool(w http.ResponseWriter, r *http.Request, name string) (*bool, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		response.ErrorWithDetails(w, r, http.StatusBadRequest, response.CodeValidationError, name+" must be true or false",
			[]response.Detail{{Field: name, Message: "must be true or false", Code: "invalid"}})
		return nil, false
	}
	return &v, true
}

// --- Categories -------------------------------------------------------------

// ListCategories serves GET /v1/catalog/categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	org := orgctx.MustScope(r.Context())
	q := apiquery.Parse(r.URL.Query())
	active, ok := queryBool(w, r, "active")
	if !ok {
		return
	}
	items, total, err := h.svc.ListCategories(r.Context(), org, model.CategoryFilter{
		Q: q.Q, Active: active, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetCategory serves GET /v1/catalog/categories/{uuid}.
func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.GetCategory(r.Context(), orgctx.MustScope(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// CreateCategory serves POST /v1/catalog/categories.
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var in model.CategoryInput
	if _, ok := readBody(w, r, &in); !ok {
		return
	}
	item, err := h.svc.CreateCategory(r.Context(), orgctx.MustScope(r.Context()), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "catalog.category.created", &item.UUID, map[string]any{"name": item.Name})
	response.JSON(w, r, http.StatusCreated, item)
}

// UpdateCategory serves PATCH /v1/catalog/categories/{uuid}.
func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.CategoryInput
	if _, ok := readBody(w, r, &in); !ok {
		return
	}
	item, err := h.svc.UpdateCategory(r.Context(), orgctx.MustScope(r.Context()), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "catalog.category.updated", &item.UUID, map[string]any{"name": item.Name})
	response.JSON(w, r, http.StatusOK, item)
}

// DeleteCategory serves DELETE /v1/catalog/categories/{uuid}.
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteCategory(r.Context(), orgctx.MustScope(r.Context()), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "catalog.category.deleted", &id, nil)
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

// --- Products ---------------------------------------------------------------

// ListProducts serves GET /v1/catalog/products.
func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request) {
	org := orgctx.MustScope(r.Context())
	q := apiquery.Parse(r.URL.Query())
	active, ok := queryBool(w, r, "active")
	if !ok {
		return
	}
	f := model.ProductFilter{Q: q.Q, Active: active, Limit: q.Limit, Offset: q.Offset}
	if raw := strings.TrimSpace(r.URL.Query().Get("category_uuid")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "category_uuid is invalid")
			return
		}
		f.CategoryUUID = &id
	}
	if unit := strings.TrimSpace(r.URL.Query().Get("unit_type")); unit != "" {
		if unit != model.UnitPiece && unit != model.UnitRollMeter {
			response.BadRequest(w, r, response.CodeValidationError, "unit_type must be piece or roll_meter")
			return
		}
		f.UnitType = unit
	}
	items, total, err := h.svc.ListProducts(r.Context(), org, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetProduct serves GET /v1/catalog/products/{uuid}.
func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.GetProduct(r.Context(), orgctx.MustScope(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func productInput(w http.ResponseWriter, r *http.Request) (model.ProductInput, bool) {
	var in model.ProductInput
	keys, ok := readBody(w, r, &in)
	if !ok {
		return in, false
	}
	_, in.WarrantySet = keys["warranty_duration_months"]
	_, in.MicronSet = keys["micron_thickness"]
	return in, true
}

// CreateProduct serves POST /v1/catalog/products.
func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	in, ok := productInput(w, r)
	if !ok {
		return
	}
	item, err := h.svc.CreateProduct(r.Context(), orgctx.MustScope(r.Context()), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "catalog.product.created", &item.UUID, map[string]any{"sku": item.SKU})
	response.JSON(w, r, http.StatusCreated, item)
}

// UpdateProduct serves PATCH /v1/catalog/products/{uuid}.
func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	in, ok := productInput(w, r)
	if !ok {
		return
	}
	item, err := h.svc.UpdateProduct(r.Context(), orgctx.MustScope(r.Context()), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "catalog.product.updated", &item.UUID, map[string]any{"sku": item.SKU})
	response.JSON(w, r, http.StatusOK, item)
}

// DeleteProduct serves DELETE /v1/catalog/products/{uuid}.
func (h *Handler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	org := orgctx.MustScope(r.Context())
	var images []model.Image
	if org.OrgType == catalogusecase.OrgTypeCenter {
		images = h.productImages(r.Context(), org, id)
	}
	if err := h.svc.DeleteProduct(r.Context(), org, id); err != nil {
		writeError(w, r, err)
		return
	}
	for _, img := range images {
		h.deleteObjects(r.Context(), id, img.Key)
	}
	h.record(r, "catalog.product.deleted", &id, nil)
	response.JSON(w, r, http.StatusOK, map[string]bool{"deleted": true})
}

// BulkActive serves POST /v1/catalog/products/bulk-active.
func (h *Handler) BulkActive(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UUIDs  []uuid.UUID `json:"uuids"`
		Active *bool       `json:"active"`
	}
	if _, ok := readBody(w, r, &in); !ok {
		return
	}
	if in.Active == nil {
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, response.CodeValidationError, "active is required",
			[]response.Detail{{Field: "active", Message: "active is required", Code: "required"}})
		return
	}
	res, err := h.svc.SetProductsActive(r.Context(), orgctx.MustScope(r.Context()), in.UUIDs, *in.Active)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "catalog.product.bulk_active", nil, map[string]any{"active": *in.Active, "updated": res.Updated})
	response.JSON(w, r, http.StatusOK, res)
}

// --- Import / export (I/O engine) -------------------------------------------

// ExportProducts serves POST /v1/catalog/products/export (export job of the
// active organization; poll /v1/tenant/exports/{uuid}).
func (h *Handler) ExportProducts(w http.ResponseWriter, r *http.Request) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	var in struct {
		Format string            `json:"format"`
		Query  map[string]string `json:"query"`
		Locale string            `json:"locale"`
	}
	if _, ok := readBody(w, r, &in); !ok {
		return
	}
	query := ioengine.ExportQuery{}
	for _, k := range []string{"q", "active", "category_uuid", "unit_type"} {
		if v := strings.TrimSpace(in.Query[k]); v != "" {
			query[k] = v
		}
	}
	if in.Locale == "" {
		in.Locale = "tr"
	}
	orgID := org.InternalID
	job, err := h.exports.RequestExport(r.Context(), p.UserInternal, &orgID, catalogusecase.IOResource,
		ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(in.Format))), query, in.Locale)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}

// ImportProducts serves POST /v1/catalog/products/import (multipart file;
// preview/confirm through /v1/tenant/imports/{uuid}). Center only (K4).
func (h *Handler) ImportProducts(w http.ResponseWriter, r *http.Request) {
	if h.imports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "imports are not configured")
		return
	}
	org := orgctx.MustScope(r.Context())
	if org.OrgType != catalogusecase.OrgTypeCenter {
		writeError(w, r, catalogusecase.ErrCenterOnly)
		return
	}
	p := authctx.MustPrincipal(r.Context())
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
	job, err := h.imports.Upload(r.Context(), p.UserInternal, &orgID, catalogusecase.IOResource, format, locale, header.Filename, file)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, job)
}

// ImportSample serves GET /v1/catalog/products/import/sample.
func (h *Handler) ImportSample(w http.ResponseWriter, r *http.Request) {
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
	data, ct, err := h.imports.Sample(r.Context(), catalogusecase.IOResource, format, locale)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename=\"products-sample."+string(format)+"\"")
	_, _ = w.Write(data)
}
