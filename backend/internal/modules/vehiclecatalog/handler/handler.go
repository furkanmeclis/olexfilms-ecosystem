// Package handler serves the TEC-149 vehicle catalog: reads under
// /v1/vehicle-catalog and /v1/portal/vehicle-catalog, super_admin writes
// under /v1/platform/vehicle-catalog, and the unauthenticated, cacheable
// brand logo and hero images under /v1/public.
package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Cache policy of the public image routes. A stored image is addressed by a
// versioned object key, so its ETag changes with every upload and a day of
// caching is safe; the placeholder/default answer is cached briefly so a new
// upload replaces it soon on the bare (unversioned) URL.
const (
	CacheControlImage       = "public, max-age=86400"
	CacheControlPlaceholder = "public, max-age=3600"
	// svgPolicy locks down the embedded placeholder SVGs (no scripts, no
	// external loads) in case a browser opens them as a document.
	svgPolicy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"
)

//go:embed assets/placeholder-logo.svg
var placeholderLogo []byte

//go:embed assets/default-hero.svg
var defaultHero []byte

type staticImage struct {
	body []byte
	etag string
}

func newStatic(body []byte) staticImage {
	sum := sha256.Sum256(body)
	return staticImage{body: body, etag: `"static-` + hex.EncodeToString(sum[:8]) + `"`}
}

var (
	placeholderImage = newStatic(placeholderLogo)
	defaultHeroImage = newStatic(defaultHero)
)

// Handler serves vehicle catalog endpoints.
type Handler struct {
	svc      *usecase.Service
	store    storage.Driver
	activity *activity.Recorder
}

// New creates the handler. store and rec may be nil (no uploads / no
// activity log; public images then answer with the placeholder).
func New(svc *usecase.Service, store storage.Driver, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, store: store, activity: rec}
}

const maxBody = 1 << 20

// readBody decodes a JSON body strictly and returns the top-level keys it
// names (PATCH null handling).
func readBody(w http.ResponseWriter, r *http.Request, dst any) (map[string]bool, bool) {
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
	present := make(map[string]bool, len(keys))
	for k := range keys {
		present[k] = true
	}
	return present, true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var verr *usecase.ValidationError
	var conflict *usecase.ConflictError
	switch {
	case errors.As(err, &verr):
		details := make([]response.Detail, 0, len(verr.Fields))
		for _, f := range verr.Fields {
			details = append(details, response.Detail{Field: f.Field, Message: f.Message, Code: f.Code})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, response.CodeValidationError, "Invalid vehicle catalog input", details)
	case errors.As(err, &conflict):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, "A record with this "+conflict.Field+" already exists",
			[]response.Detail{{Field: conflict.Field, Message: "already exists", Code: "duplicate"}})
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Vehicle catalog record not found")
	case errors.Is(err, usecase.ErrInUse):
		response.Conflict(w, r, response.CodeConflict, "The record is still in use")
	default:
		response.InternalErr(w, r, err, "vehicle catalog request failed")
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
	h.activity.Record(r.Context(), actor, action, "vehicle_catalog", id, payload, r)
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

// canWrite: writers (super_admin) see inactive rows and may filter on
// active; every other reader sees active rows only.
func canWrite(r *http.Request) bool {
	p, ok := authctx.PrincipalFrom(r.Context())
	return ok && p.HasPermission(rbac.PermVehicleCatalogWrite)
}

func readerActive(w http.ResponseWriter, r *http.Request) (*bool, bool, bool) {
	if !canWrite(r) {
		t := true
		return &t, true, true
	}
	active, ok := queryBool(w, r, "active")
	return active, false, ok
}

// --- Brands -----------------------------------------------------------------

// ListBrands serves GET /v1/vehicle-catalog/brands (and the portal copy).
func (h *Handler) ListBrands(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	active, _, ok := readerActive(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.ListBrands(r.Context(), model.BrandFilter{
		Q: q.Q, Active: active, Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetBrand serves GET /v1/vehicle-catalog/brands/{uuid}.
func (h *Handler) GetBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.GetBrand(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// CreateBrand serves POST /v1/platform/vehicle-catalog/brands.
func (h *Handler) CreateBrand(w http.ResponseWriter, r *http.Request) {
	var in model.BrandInput
	keys, ok := readBody(w, r, &in)
	if !ok {
		return
	}
	in.Present = keys
	item, err := h.svc.CreateBrand(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicle_catalog.brand.created", &item.UUID, map[string]any{"name": item.Name})
	response.JSON(w, r, http.StatusCreated, item)
}

// UpdateBrand serves PATCH /v1/platform/vehicle-catalog/brands/{uuid}.
func (h *Handler) UpdateBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.BrandInput
	keys, ok := readBody(w, r, &in)
	if !ok {
		return
	}
	in.Present = keys
	item, err := h.svc.UpdateBrand(r.Context(), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicle_catalog.brand.updated", &item.UUID, map[string]any{"name": item.Name})
	response.JSON(w, r, http.StatusOK, item)
}

// DeleteBrand serves DELETE /v1/platform/vehicle-catalog/brands/{uuid}; a
// brand with models answers 409.
func (h *Handler) DeleteBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	keys, err := h.svc.DeleteBrand(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.deleteObjects(r.Context(), keys...)
	h.record(r, "vehicle_catalog.brand.deleted", &id, nil)
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// --- Models -----------------------------------------------------------------

// ListModels serves GET /v1/vehicle-catalog/models: list/search by
// "brand model" text, optionally inside one brand (brand_uuid).
func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	active, onlyActiveBrands, ok := readerActive(w, r)
	if !ok {
		return
	}
	f := model.ModelFilter{Q: q.Q, Active: active, OnlyActiveBrands: onlyActiveBrands, Limit: q.Limit, Offset: q.Offset}
	if raw := strings.TrimSpace(r.URL.Query().Get("brand_uuid")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ErrorWithDetails(w, r, http.StatusBadRequest, response.CodeValidationError, "brand_uuid is invalid",
				[]response.Detail{{Field: "brand_uuid", Message: "must be a uuid", Code: "invalid"}})
			return
		}
		f.BrandUUID = &id
	}
	items, total, err := h.svc.ListModels(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetModel serves GET /v1/vehicle-catalog/models/{uuid}.
func (h *Handler) GetModel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.GetModel(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

// CreateModel serves POST /v1/platform/vehicle-catalog/models.
func (h *Handler) CreateModel(w http.ResponseWriter, r *http.Request) {
	var in model.ModelInput
	keys, ok := readBody(w, r, &in)
	if !ok {
		return
	}
	in.Present = keys
	item, err := h.svc.CreateModel(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicle_catalog.model.created", &item.UUID, map[string]any{"name": item.Name, "brand": item.Brand.Name})
	response.JSON(w, r, http.StatusCreated, item)
}

// UpdateModel serves PATCH /v1/platform/vehicle-catalog/models/{uuid}.
func (h *Handler) UpdateModel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.ModelInput
	keys, ok := readBody(w, r, &in)
	if !ok {
		return
	}
	in.Present = keys
	item, err := h.svc.UpdateModel(r.Context(), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "vehicle_catalog.model.updated", &item.UUID, map[string]any{"name": item.Name})
	response.JSON(w, r, http.StatusOK, item)
}

// DeleteModel serves DELETE /v1/platform/vehicle-catalog/models/{uuid}.
func (h *Handler) DeleteModel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	key, err := h.svc.DeleteModel(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.deleteObjects(r.Context(), key)
	h.record(r, "vehicle_catalog.model.deleted", &id, nil)
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// --- Uploads ----------------------------------------------------------------

func (h *Handler) deleteObjects(ctx context.Context, keys ...string) {
	if h.store == nil {
		return
	}
	for _, k := range keys {
		if k != "" {
			_ = h.store.Delete(ctx, k)
		}
	}
}

func newVersion() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
}

// upload reads the multipart field, accepts only JPEG/PNG/WebP by content
// sniffing (SVG and anything else is 400, storage.DetectLogoMIME) and stores
// it under keyFor(ext). It returns the object key.
func (h *Handler) upload(w http.ResponseWriter, r *http.Request, field string, maxBytes int64,
	validateSize func(int64) error, keyFor func(ext string) string) (string, bool) {
	if h.store == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "storage is not configured")
		return "", false
	}
	// Cap the whole body: ParseMultipartForm only bounds memory.
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBytes + (1 << 20)); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return "", false
	}
	file, header, err := r.FormFile(field)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, field+" is required")
		return "", false
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime, err := storage.DetectLogoMIME(header.Header.Get("Content-Type"), head[:n])
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return "", false
	}
	if err := validateSize(header.Size); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return "", false
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return "", false
	}
	key := keyFor(ext)
	body := io.MultiReader(bytes.NewReader(head[:n]), file)
	if err := h.store.Upload(r.Context(), storage.File{
		Body: body, Size: header.Size, ContentType: mime, Filename: header.Filename,
	}, key); err != nil {
		response.InternalErr(w, r, err, "image upload failed")
		return "", false
	}
	return key, true
}

func (h *Handler) setBrandImage(w http.ResponseWriter, r *http.Request, kind string) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.GetBrand(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	var key string
	if r.Method != http.MethodDelete {
		var keyFor func(string) string
		maxBytes, validate := int64(storage.MaxLogoBytes), storage.ValidateLogoSize
		if kind == usecase.ImageLogo {
			keyFor = func(ext string) string { return storage.VehicleBrandLogoObjectKey(id, newVersion(), ext) }
		} else {
			maxBytes, validate = storage.MaxHeroBytes, storage.ValidateHeroSize
			keyFor = func(ext string) string { return storage.VehicleBrandHeroObjectKey(id, newVersion(), ext) }
		}
		if key, ok = h.upload(w, r, kind, maxBytes, validate, keyFor); !ok {
			return
		}
	}
	item, old, err := h.svc.SetBrandImage(r.Context(), id, kind, key)
	if err != nil {
		h.deleteObjects(r.Context(), key)
		writeError(w, r, err)
		return
	}
	h.deleteObjects(r.Context(), old)
	h.record(r, "vehicle_catalog.brand."+kind+"_updated", &item.UUID, map[string]any{"removed": key == ""})
	response.JSON(w, r, http.StatusOK, item)
}

// UploadBrandLogo serves PUT (multipart "logo") and DELETE
// /v1/platform/vehicle-catalog/brands/{uuid}/logo.
func (h *Handler) UploadBrandLogo(w http.ResponseWriter, r *http.Request) {
	h.setBrandImage(w, r, usecase.ImageLogo)
}

// UploadBrandHero serves PUT (multipart "hero") and DELETE
// /v1/platform/vehicle-catalog/brands/{uuid}/hero.
func (h *Handler) UploadBrandHero(w http.ResponseWriter, r *http.Request) {
	h.setBrandImage(w, r, usecase.ImageHero)
}

// UploadModelHero serves PUT (multipart "hero") and DELETE
// /v1/platform/vehicle-catalog/models/{uuid}/hero.
func (h *Handler) UploadModelHero(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.GetModel(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	var key string
	if r.Method != http.MethodDelete {
		if key, ok = h.upload(w, r, usecase.ImageHero, storage.MaxHeroBytes, storage.ValidateHeroSize,
			func(ext string) string { return storage.VehicleModelHeroObjectKey(id, newVersion(), ext) }); !ok {
			return
		}
	}
	item, old, err := h.svc.SetModelHero(r.Context(), id, key)
	if err != nil {
		h.deleteObjects(r.Context(), key)
		writeError(w, r, err)
		return
	}
	h.deleteObjects(r.Context(), old)
	h.record(r, "vehicle_catalog.model.hero_updated", &item.UUID, map[string]any{"removed": key == ""})
	response.JSON(w, r, http.StatusOK, item)
}

// --- Public images ----------------------------------------------------------

// etagMatches implements If-None-Match (weak comparison, "*" and lists).
func etagMatches(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, part := range strings.Split(header, ",") {
		if strings.TrimPrefix(strings.TrimSpace(part), "W/") == want {
			return true
		}
	}
	return false
}

func notModified(w http.ResponseWriter, r *http.Request, etag, cacheControl string) bool {
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

func serveStatic(w http.ResponseWriter, r *http.Request, img staticImage) {
	if notModified(w, r, img.etag, CacheControlPlaceholder) {
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Content-Security-Policy", svgPolicy)
	w.Header().Set("Content-Length", strconv.Itoa(len(img.body)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(img.body)
	}
}

// serveObject streams a stored image, or the fallback when there is none
// (or storage lost it).
func (h *Handler) serveObject(w http.ResponseWriter, r *http.Request, key string, fallback staticImage) {
	if key == "" || h.store == nil {
		serveStatic(w, r, fallback)
		return
	}
	etag := usecase.ETag(key)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		notModified(w, r, etag, CacheControlImage)
		return
	}
	rc, size, err := h.store.Download(r.Context(), key)
	if err != nil {
		serveStatic(w, r, fallback)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", CacheControlImage)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", storage.MIMEFromLogoKey(key))
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, rc)
	}
}

// PublicBrandLogo serves GET /v1/public/brand-logos/{uuid} without auth:
// the logo with ETag + Cache-Control, 304 on If-None-Match, a placeholder
// while the brand has no logo, 404 for an unknown brand.
func (h *Handler) PublicBrandLogo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	key, err := h.svc.BrandLogoKey(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.serveObject(w, r, key, placeholderImage)
}

// PublicBrandHero serves GET /v1/public/vehicle-heroes/brands/{uuid}:
// brand hero → default image.
func (h *Handler) PublicBrandHero(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	key, err := h.svc.BrandHeroKey(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.serveObject(w, r, key, defaultHeroImage)
}

// PublicModelHero serves GET /v1/public/vehicle-heroes/models/{uuid}:
// model hero → brand hero → default image.
func (h *Handler) PublicModelHero(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	key, err := h.svc.ModelHeroKey(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.serveObject(w, r, key, defaultHeroImage)
}

// PublicDefaultHero serves GET /v1/public/vehicle-heroes/default.
func (h *Handler) PublicDefaultHero(w http.ResponseWriter, r *http.Request) {
	serveStatic(w, r, defaultHeroImage)
}
