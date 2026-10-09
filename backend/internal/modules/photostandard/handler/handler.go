package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/webp"
)

type Handler struct {
	svc   *usecase.Service
	store storage.Driver
}

func New(svc *usecase.Service, store storage.Driver) *Handler {
	return &Handler{svc: svc, store: store}
}

func caller(r *http.Request) usecase.Caller {
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Photo standard record not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "This organization cannot access the photo standard")
	case errors.Is(err, usecase.ErrValidation):
		response.ValidationError(w, r, []response.Detail{{Field: "body", Message: "invalid photo standard input", Code: "invalid"}})
	case errors.Is(err, usecase.ErrUnsupportedMedia):
		response.Error(w, r, http.StatusUnsupportedMediaType, usecase.CodeUnsupportedMedia, "Image must be JPEG, PNG, WebP or HEIC")
	case errors.Is(err, usecase.ErrFileTooLarge):
		response.Error(w, r, http.StatusRequestEntityTooLarge, usecase.CodeFileTooLarge, "Image must be at most 12 MB")
	case errors.Is(err, usecase.ErrServiceLocked):
		response.Conflict(w, r, usecase.CodeServiceLocked, "The service or its executed contract no longer allows intake photo changes")
	default:
		response.InternalErr(w, r, err, "photo standard request failed")
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

type angleBody struct {
	Key               string          `json:"key"`
	Name              json.RawMessage `json:"name"`
	Hint              json.RawMessage `json:"hint"`
	ExampleStorageKey *string         `json:"example_storage_key"`
	Required          bool            `json:"required"`
	SortOrder         int32           `json:"sort_order"`
	Active            *bool           `json:"active"`
}

func (b angleBody) input() usecase.AngleInput {
	active := true
	if b.Active != nil {
		active = *b.Active
	}
	return usecase.AngleInput{
		Key: b.Key, Name: b.Name, Hint: b.Hint, ExampleStorageKey: b.ExampleStorageKey,
		Required: b.Required, SortOrder: b.SortOrder, Active: active,
	}
}

func (h *Handler) ListAngles(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListAngles(r.Context(), caller(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateAngle(w http.ResponseWriter, r *http.Request) {
	var body angleBody
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.CreateAngle(r.Context(), caller(r), body.input())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) UpdateAngle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Photo standard record not found")
		return
	}
	var body angleBody
	if !decode(w, r, &body) {
		return
	}
	item, err := h.svc.UpdateAngle(r.Context(), caller(r), id, body.input())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) DeleteAngle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Photo standard record not found")
		return
	}
	if err := h.svc.DeleteAngle(r.Context(), caller(r), id); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type overridesBody struct {
	OrganizationUUID string `json:"organization_uuid"`
	Overrides        []struct {
		AngleKey string `json:"angle_key"`
		Required bool   `json:"required"`
		Hidden   bool   `json:"hidden"`
	} `json:"overrides"`
}

func (h *Handler) PutOverrides(w http.ResponseWriter, r *http.Request) {
	var body overridesBody
	if !decode(w, r, &body) {
		return
	}
	target, err := uuid.Parse(strings.TrimSpace(body.OrganizationUUID))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "organization_uuid", Message: "must be a UUID", Code: "invalid"}})
		return
	}
	items := make([]usecase.OverrideInput, 0, len(body.Overrides))
	for _, item := range body.Overrides {
		items = append(items, usecase.OverrideInput{AngleKey: item.AngleKey, Required: item.Required, Hidden: item.Hidden})
	}
	out, err := h.svc.PutOverrides(r.Context(), caller(r), target, items)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) GetOverrides(w http.ResponseWriter, r *http.Request) {
	target, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("organization_uuid")))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "organization_uuid", Message: "must be a UUID", Code: "invalid"}})
		return
	}
	out, err := h.svc.GetOverrides(r.Context(), caller(r), target)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) Intake(w http.ResponseWriter, r *http.Request) {
	id, ok := serviceUUID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.Intake(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	id, ok := serviceUUID(w, r)
	if !ok {
		return
	}
	if h.store == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "storage is not configured")
		return
	}
	body, header, meta, ok := readUpload(w, r)
	if !ok {
		return
	}
	slot, err := h.svc.Upload(r.Context(), caller(r), id, r.PathValue("angle_key"), meta)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.store.Upload(r.Context(), storage.File{
		Body: bytes.NewReader(body), Size: int64(len(body)), ContentType: meta.Mime, Filename: header.Filename,
	}, slot.ObjectKey); err != nil {
		response.InternalErr(w, r, err, "intake photo upload failed")
		return
	}
	if slot.OldKey != "" {
		_ = h.store.Delete(r.Context(), slot.OldKey)
	}
	response.JSON(w, r, http.StatusCreated, slot.Photo)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := serviceUUID(w, r)
	if !ok {
		return
	}
	key, err := h.svc.DeletePhoto(r.Context(), caller(r), id, r.PathValue("angle_key"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if h.store != nil {
		_ = h.store.Delete(r.Context(), key)
	}
	w.WriteHeader(http.StatusNoContent)
}

// IntakeFile serves GET /v1/services/{uuid}/intake-photos/{angle_key}/file
// (TEC-500): the active photo of the angle.
func (h *Handler) IntakeFile(w http.ResponseWriter, r *http.Request) {
	id, ok := serviceUUID(w, r)
	if !ok {
		return
	}
	key, mime, err := h.svc.IntakePhotoObject(r.Context(), caller(r), id, r.PathValue("angle_key"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	h.stream(w, r, key, mime)
}

// UploadExample serves POST /v1/platform/photo-standard/angles/{uuid}/example
// (TEC-500): multipart `image` (JPEG, PNG or WebP) replacing the example.
func (h *Handler) UploadExample(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Photo standard record not found")
		return
	}
	if h.store == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "storage is not configured")
		return
	}
	body, header, meta, ok := readUpload(w, r)
	if !ok {
		return
	}
	slot, err := h.svc.SetExample(r.Context(), caller(r), id, meta)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.store.Upload(r.Context(), storage.File{
		Body: bytes.NewReader(body), Size: int64(len(body)), ContentType: meta.Mime, Filename: header.Filename,
	}, slot.ObjectKey); err != nil {
		response.InternalErr(w, r, err, "example image upload failed")
		return
	}
	if slot.OldKey != "" && slot.OldKey != slot.ObjectKey {
		_ = h.store.Delete(r.Context(), slot.OldKey)
	}
	response.JSON(w, r, http.StatusOK, slot.Angle)
}

// Example serves GET /v1/photo-standard/angles/{uuid}/example (TEC-500):
// the example image shown on the wizard angle cards.
func (h *Handler) Example(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Photo standard record not found")
		return
	}
	key, err := h.svc.ExampleObject(r.Context(), caller(r), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	h.stream(w, r, key, storage.MIMEFromLogoKey(key))
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request, key, mime string) {
	if h.store == nil {
		response.NotFound(w, r, "Photo standard record not found")
		return
	}
	rc, size, err := h.store.Download(r.Context(), key)
	if err != nil {
		response.NotFound(w, r, "Photo standard record not found")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func serviceUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Service not found")
		return uuid.Nil, false
	}
	return id, true
}

func readUpload(w http.ResponseWriter, r *http.Request) ([]byte, *multipart.FileHeader, usecase.UploadMeta, bool) {
	const limit = usecase.MaxUploadBytes + (1 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, r, usecase.ErrFileTooLarge)
			return nil, nil, usecase.UploadMeta{}, false
		}
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return nil, nil, usecase.UploadMeta{}, false
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "image is required")
		return nil, nil, usecase.UploadMeta{}, false
	}
	defer func() { _ = file.Close() }()
	if header.Size <= 0 {
		response.BadRequest(w, r, response.CodeValidationError, "image is required")
		return nil, nil, usecase.UploadMeta{}, false
	}
	if header.Size > usecase.MaxUploadBytes {
		writeErr(w, r, usecase.ErrFileTooLarge)
		return nil, nil, usecase.UploadMeta{}, false
	}
	body, err := io.ReadAll(io.LimitReader(file, usecase.MaxUploadBytes+1))
	if err != nil {
		response.InternalErr(w, r, err, "intake photo read failed")
		return nil, nil, usecase.UploadMeta{}, false
	}
	if int64(len(body)) > usecase.MaxUploadBytes {
		writeErr(w, r, usecase.ErrFileTooLarge)
		return nil, nil, usecase.UploadMeta{}, false
	}
	meta, err := inspect(body)
	if err != nil {
		writeErr(w, r, err)
		return nil, nil, usecase.UploadMeta{}, false
	}
	return body, header, meta, true
}

func inspect(body []byte) (usecase.UploadMeta, error) {
	mime, ext := sniff(body)
	if mime == "" {
		return usecase.UploadMeta{}, usecase.ErrUnsupportedMedia
	}
	meta := usecase.UploadMeta{Mime: mime, Size: int64(len(body)), SHA256: usecase.Digest(body), Ext: ext}
	if mime == "image/webp" {
		cfg, err := webp.DecodeConfig(bytes.NewReader(body))
		if err == nil {
			w, h := int32(cfg.Width), int32(cfg.Height)
			meta.Width, meta.Height = &w, &h
		}
		return meta, nil
	}
	if mime == "image/jpeg" || mime == "image/png" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(body))
		if err == nil {
			w, h := int32(cfg.Width), int32(cfg.Height)
			meta.Width, meta.Height = &w, &h
		}
	}
	if mime == "image/jpeg" {
		readEXIF(body, &meta)
	}
	return meta, nil
}

func sniff(body []byte) (string, string) {
	if len(body) >= 3 && body[0] == 0xff && body[1] == 0xd8 && body[2] == 0xff {
		return "image/jpeg", "jpg"
	}
	if len(body) >= 8 && bytes.Equal(body[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png", "png"
	}
	if len(body) >= 12 && string(body[:4]) == "RIFF" && string(body[8:12]) == "WEBP" {
		return "image/webp", "webp"
	}
	if len(body) >= 12 && string(body[4:8]) == "ftyp" {
		brand := string(body[8:12])
		if strings.HasPrefix(brand, "hei") || strings.HasPrefix(brand, "heic") || strings.HasPrefix(brand, "heix") {
			return "image/heic", "heic"
		}
	}
	return "", ""
}

func readEXIF(body []byte, meta *usecase.UploadMeta) {
	x, err := exif.Decode(bytes.NewReader(body))
	if err != nil {
		return
	}
	if tm, err := x.DateTime(); err == nil {
		meta.ExifTakenAt = &tm
	}
	if lat, lng, err := x.LatLong(); err == nil {
		latS := strconv.FormatFloat(lat, 'f', 7, 64)
		lngS := strconv.FormatFloat(lng, 'f', 7, 64)
		meta.ExifLat, meta.ExifLng = &latS, &lngS
	}
	var parts []string
	if makeTag, err := x.Get(exif.Make); err == nil && makeTag != nil {
		parts = append(parts, makeTag.String())
	}
	if modelTag, err := x.Get(exif.Model); err == nil && modelTag != nil {
		parts = append(parts, modelTag.String())
	}
	device := strings.TrimSpace(strings.Trim(strings.Join(parts, " "), "\" "))
	if device != "" && device != "<nil> <nil>" {
		meta.ExifDevice = &device
	}
}
