package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// CacheControlProductImage is the cache policy of the public image route.
// An image key is random and never reused for other bytes, so a day of
// caching is safe; a removed image or a deactivated product stops being
// served once caches expire.
const CacheControlProductImage = "public, max-age=86400"

// imageField is the multipart field of the upload route.
const imageField = "image"

func (h *Handler) deleteObjects(ctx context.Context, productUUID uuid.UUID, keys ...string) {
	if h.store == nil {
		return
	}
	for _, k := range keys {
		if catalogusecase.IsUploadedImageKey(k) {
			_ = h.store.Delete(ctx, storage.ProductImageObjectKey(productUUID, k))
		}
	}
}

// UploadProductImage serves POST /v1/catalog/products/{uuid}/images
// (multipart "image"). Center only (K4); JPEG/PNG/WebP by content sniffing
// (SVG and anything else 400), at most 5 MiB (413) and
// catalogusecase.MaxProductImages per product (422).
func (h *Handler) UploadProductImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	org := orgctx.MustScope(r.Context())
	if err := h.svc.CheckImageSlot(r.Context(), org, id); err != nil {
		writeError(w, r, err)
		return
	}
	if h.store == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "storage is not configured")
		return
	}
	// Cap the whole body: ParseMultipartForm only bounds memory.
	const limit = storage.MaxProductImageBytes + (1 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			tooLargeError(w, r)
			return
		}
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile(imageField)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, imageField+" is required")
		return
	}
	defer func() { _ = file.Close() }()
	if header.Size > storage.MaxProductImageBytes {
		tooLargeError(w, r)
		return
	}
	if header.Size <= 0 {
		response.BadRequest(w, r, response.CodeValidationError, "image file is required")
		return
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime, err := storage.DetectLogoMIME(header.Header.Get("Content-Type"), head[:n])
	if err != nil || n == 0 {
		response.BadRequest(w, r, response.CodeValidationError, "image must be image/jpeg, image/png, or image/webp")
		return
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	key := catalogusecase.NewImageKey(ext)
	if err := h.store.Upload(r.Context(), storage.File{
		Body: io.MultiReader(bytes.NewReader(head[:n]), file), Size: header.Size,
		ContentType: mime, Filename: header.Filename,
	}, storage.ProductImageObjectKey(id, key)); err != nil {
		response.InternalErr(w, r, err, "image upload failed")
		return
	}
	item, err := h.svc.AddProductImage(r.Context(), org, id, key)
	if err != nil {
		h.deleteObjects(r.Context(), id, key)
		writeError(w, r, err)
		return
	}
	h.record(r, "catalog.product.image_added", &item.UUID, map[string]any{"key": key})
	response.JSON(w, r, http.StatusCreated, item)
}

func tooLargeError(w http.ResponseWriter, r *http.Request) {
	response.ErrorWithDetails(w, r, http.StatusRequestEntityTooLarge, response.CodeValidationError, "image must be at most 5 MiB",
		[]response.Detail{{Field: imageField, Message: "image must be at most 5 MiB", Code: "too_large"}})
}

// DeleteProductImage serves DELETE /v1/catalog/products/{uuid}/images/{key}.
func (h *Handler) DeleteProductImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	key := r.PathValue("key")
	item, err := h.svc.RemoveProductImage(r.Context(), orgctx.MustScope(r.Context()), id, key)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.deleteObjects(r.Context(), id, key)
	h.record(r, "catalog.product.image_removed", &item.UUID, map[string]any{"key": key})
	response.JSON(w, r, http.StatusOK, item)
}

// ReorderProductImages serves PUT /v1/catalog/products/{uuid}/images/order.
func (h *Handler) ReorderProductImages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in struct {
		Keys []string `json:"keys"`
	}
	if _, ok := readBody(w, r, &in); !ok {
		return
	}
	item, err := h.svc.ReorderProductImages(r.Context(), orgctx.MustScope(r.Context()), id, in.Keys)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "catalog.product.images_reordered", &item.UUID, nil)
	response.JSON(w, r, http.StatusOK, item)
}

// productImages returns the images of the product before a delete so their
// objects can be removed afterwards.
func (h *Handler) productImages(ctx context.Context, org orgctx.Scope, id uuid.UUID) []model.Image {
	if h.store == nil {
		return nil
	}
	p, err := h.svc.GetProduct(ctx, org, id)
	if err != nil {
		return nil
	}
	return p.Images
}

// --- Public images ----------------------------------------------------------

func imageETag(key string) string {
	sum := sha256.Sum256([]byte(key))
	return `"` + hex.EncodeToString(sum[:10]) + `"`
}

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

// PublicProductImage serves GET /v1/public/product-images/{key} without
// auth: an image of an active product with ETag + Cache-Control and 304 on
// If-None-Match. Inactive products, removed and unknown keys are 404.
func (h *Handler) PublicProductImage(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	productUUID, err := h.svc.PublicImageProduct(r.Context(), key)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.store == nil {
		response.NotFound(w, r, "Catalog record not found")
		return
	}
	etag := imageETag(key)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", CacheControlProductImage)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	rc, size, err := h.store.Download(r.Context(), storage.ProductImageObjectKey(productUUID, key))
	if err != nil {
		response.NotFound(w, r, "Catalog record not found")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", CacheControlProductImage)
	w.Header().Set("Content-Type", storage.MIMEFromLogoKey(key))
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, rc)
	}
}
