// Package handler exposes the dealer showcase API (TEC-467, F5-01b): the
// panel editor under /v1/showcase, the center review queue under
// /v1/platform/showcases and the public gallery files.
package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Store keeps gallery objects.
type Store interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// Handler serves the showcase endpoints.
type Handler struct {
	svc   *usecase.Service
	store Store
}

// New creates the handler; store may be nil (uploads answer 503).
func New(svc *usecase.Service, store Store) *Handler { return &Handler{svc: svc, store: store} }

const photoField = "photo"

func caller(r *http.Request) usecase.Caller {
	p := authctx.MustPrincipal(r.Context())
	org := orgctx.MustScope(r.Context())
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{UserID: p.UserInternal, OrganizationID: org.InternalID, BrandID: org.BrandID, Filter: f}
}

// targetOrg reads ?org= (a dealer of the caller's scope); absent means the
// caller's own organization.
func targetOrg(w http.ResponseWriter, r *http.Request) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("org"))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "org", Message: "must be an organization uuid"}})
		return nil, false
	}
	return &id, true
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: "must be a uuid"}})
		return uuid.Nil, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
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

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	var qe *apiquery.ValidationError
	var le *repository.PhotoLimitError
	var re *usecase.RatingRangeError
	switch {
	case errors.As(err, &qe):
		response.QueryValidation(w, r, err)
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.As(err, &le):
		response.ErrorWithData(w, r, http.StatusUnprocessableEntity, usecase.CodePhotoLimit,
			"The gallery is full", nil, map[string]any{"count": le.Count, "max": le.Max})
	case errors.As(err, &re):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, usecase.CodeRatingOutOfRange,
			"The Google rating is out of range", []response.Detail{{Field: re.Field, Message: re.Message}})
	case errors.Is(err, usecase.ErrRatingManagedByPlaces):
		response.Conflict(w, r, usecase.CodeRatingManagedByPlaces,
			"The Google rating is refreshed from Google Places; clear the place id to enter it by hand")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Showcase not found")
	case errors.Is(err, usecase.ErrFeatureDisabled):
		response.Error(w, r, http.StatusForbidden, response.CodeFeatureDisabled,
			"The dealer showcase is not enabled for this organization")
	case errors.Is(err, usecase.ErrInvalidTransition):
		response.Conflict(w, r, usecase.CodeInvalidTransition, "The showcase cannot move to that status")
	case errors.Is(err, usecase.ErrReviewNoteRequired):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, usecase.CodeReviewNoteRequired,
			"A rejection needs a note", []response.Detail{{Field: "note", Message: "is required to reject"}})
	case errors.Is(err, usecase.ErrDuplicatePhoto):
		response.Conflict(w, r, response.CodeConflict, "This photo is already in the gallery")
	case errors.Is(err, usecase.ErrDuplicateService):
		response.Conflict(w, r, response.CodeConflict, "This category is already listed")
	default:
		response.InternalErr(w, r, err, "showcase request failed")
	}
}

// Get serves GET /v1/showcase.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), caller(r), org)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Save serves PUT /v1/showcase.
func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	var body usecase.Input
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.Save(r.Context(), caller(r), org, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// SetGoogleRating serves PUT /v1/showcase/google-rating (TEC-469).
func (h *Handler) SetGoogleRating(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	var body usecase.RatingInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.SetGoogleRating(r.Context(), caller(r), org, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Submit serves POST /v1/showcase/submit.
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Submit(r.Context(), caller(r), org)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListServices serves GET /v1/showcase/services.
func (h *Handler) ListServices(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ListServices(r.Context(), caller(r), org)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// CreateService serves POST /v1/showcase/services.
func (h *Handler) CreateService(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	var body usecase.ServiceInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.CreateService(r.Context(), caller(r), org, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// UpdateService serves PUT /v1/showcase/services/{uuid}.
func (h *Handler) UpdateService(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body usecase.ServiceInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.UpdateService(r.Context(), caller(r), org, id, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteService serves DELETE /v1/showcase/services/{uuid}.
func (h *Handler) DeleteService(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteService(r.Context(), caller(r), org, id); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type orderBody struct {
	UUIDs []uuid.UUID `json:"uuids"`
}

// ReorderServices serves PUT /v1/showcase/services/order.
func (h *Handler) ReorderServices(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	var body orderBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.ReorderServices(r.Context(), caller(r), org, body.UUIDs)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

func tooLarge(w http.ResponseWriter, r *http.Request) {
	response.ErrorWithDetails(w, r, http.StatusRequestEntityTooLarge, response.CodeValidationError, "photo must be at most 5 MiB",
		[]response.Detail{{Field: photoField, Message: "photo must be at most 5 MiB", Code: "too_large"}})
}

// UploadPhoto serves POST /v1/showcase/photos (multipart "photo" and an
// optional "caption" JSON object of locale → text): JPEG/PNG/WebP by
// content sniffing (anything else 415), at most 5 MiB. The bytes are
// stored as uploaded (no re-encoding).
func (h *Handler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	if h.store == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "storage is not configured")
		return
	}
	const limit = usecase.MaxPhotoUploadBytes + (1 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			tooLarge(w, r)
			return
		}
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile(photoField)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, photoField+" is required")
		return
	}
	defer func() { _ = file.Close() }()
	if header.Size > usecase.MaxPhotoUploadBytes {
		tooLarge(w, r)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, usecase.MaxPhotoUploadBytes+1))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid photo")
		return
	}
	if len(data) == 0 {
		response.BadRequest(w, r, response.CodeValidationError, "photo file is required")
		return
	}
	if len(data) > usecase.MaxPhotoUploadBytes {
		tooLarge(w, r)
		return
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	mime, err := storage.DetectLogoMIME(header.Header.Get("Content-Type"), head)
	if err != nil {
		response.Error(w, r, http.StatusUnsupportedMediaType, usecase.CodeUnsupportedMedia,
			"photo must be image/jpeg, image/png, or image/webp")
		return
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		response.Error(w, r, http.StatusUnsupportedMediaType, usecase.CodeUnsupportedMedia, err.Error())
		return
	}
	caption := map[string]string{}
	if raw := strings.TrimSpace(r.FormValue("caption")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &caption); err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "caption", Message: "must be a JSON object of locale to text"}})
			return
		}
	}
	c := caller(r)
	slot, err := h.svc.PreparePhoto(r.Context(), c, org, ext)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	sum := sha256.Sum256(data)
	if err := h.store.Upload(r.Context(), storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: mime, Filename: header.Filename,
	}, slot.ObjectKey); err != nil {
		response.InternalErr(w, r, err, "photo upload failed")
		return
	}
	out, err := h.svc.AddPhoto(r.Context(), c, org, usecase.PhotoInput{
		ObjectKey: slot.ObjectKey, Mime: mime, SizeBytes: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), Caption: caption,
	})
	if err != nil {
		_ = h.store.Delete(r.Context(), slot.ObjectKey)
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

type captionBody struct {
	Caption map[string]string `json:"caption"`
}

// UpdatePhoto serves PUT /v1/showcase/photos/{uuid} (caption).
func (h *Handler) UpdatePhoto(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body captionBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.SetPhotoCaption(r.Context(), caller(r), org, id, body.Caption)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeletePhoto serves DELETE /v1/showcase/photos/{uuid}.
func (h *Handler) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeletePhoto(r.Context(), caller(r), org, id); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReorderPhotos serves PUT /v1/showcase/photos/order.
func (h *Handler) ReorderPhotos(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	var body orderBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.ReorderPhotos(r.Context(), caller(r), org, body.UUIDs)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// PhotoFile serves GET /v1/showcase/photos/{uuid}/file (panel preview of
// draft photos).
func (h *Handler) PhotoFile(w http.ResponseWriter, r *http.Request) {
	org, ok := targetOrg(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	key, mime, err := h.svc.PhotoObject(r.Context(), caller(r), org, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	h.stream(w, r, key, mime, "private, max-age=300")
}

// PublicPhoto serves GET /v1/public/dealers/{code}/photos/{uuid}: a photo
// of the dealer's published showcase (404 otherwise, also with the module
// off).
func (h *Handler) PublicPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Photo not found")
		return
	}
	brand, err := orgusecase.RequestBrand(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "failed to resolve request brand")
		return
	}
	code := strings.ToLower(strings.TrimSpace(r.PathValue("code")))
	key, mime, err := h.svc.PublicPhoto(r.Context(), brand.ID, code, id)
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			response.NotFound(w, r, "Photo not found")
			return
		}
		writeErr(w, r, err)
		return
	}
	h.stream(w, r, key, mime, "public, max-age=3600")
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request, key, mime, cache string) {
	if h.store == nil {
		response.NotFound(w, r, "Photo not found")
		return
	}
	rc, size, err := h.store.Download(r.Context(), key)
	if err != nil {
		response.NotFound(w, r, "Photo not found")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", cache)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// ListReviews serves GET /v1/platform/showcases (the center review queue,
// docs/list-contract.md).
func (h *Handler) ListReviews(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseReviewFilter(r.URL.Query())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out, err := h.svc.ListReviews(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ReviewDetail serves GET /v1/platform/showcases/{org_uuid}.
func (h *Handler) ReviewDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "org_uuid")
	if !ok {
		return
	}
	out, err := h.svc.ReviewDetail(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Review serves POST /v1/platform/showcases/{org_uuid}/review.
func (h *Handler) Review(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "org_uuid")
	if !ok {
		return
	}
	var body usecase.ReviewInput
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.Review(r.Context(), caller(r), id, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
