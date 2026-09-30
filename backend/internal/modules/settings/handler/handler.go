package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	settingsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/settings/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

type Handler struct {
	svc     *settingsusecase.Service
	storage storage.Driver
}

func New(svc *settingsusecase.Service, store storage.Driver) *Handler {
	return &Handler{svc: svc, storage: store}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Get(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "failed to load settings")
		return
	}
	response.JSON(w, r, http.StatusOK, s)
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	var in settingsusecase.PatchInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	s, err := h.svc.Patch(r.Context(), in)
	if err != nil {
		response.InternalErr(w, r, err, "failed to update settings")
		return
	}
	response.JSON(w, r, http.StatusOK, s)
}

func (h *Handler) UploadLogo(w http.ResponseWriter, r *http.Request) {
	// Cap the whole body: ParseMultipartForm only bounds memory and spills
	// the rest to temp files.
	r.Body = http.MaxBytesReader(w, r.Body, (3<<20)+(1<<20))
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("logo")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "logo is required")
		return
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime, err := storage.DetectLogoMIME(header.Header.Get("Content-Type"), head[:n])
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err := storage.ValidateLogoSize(header.Size); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	key := storage.AppLogoObjectKey(ext)
	body := io.MultiReader(strings.NewReader(string(head[:n])), file)
	if err := h.storage.Upload(r.Context(), storage.File{
		Body: body, Size: header.Size, ContentType: mime, Filename: header.Filename,
	}, key); err != nil {
		response.InternalErr(w, r, err, "logo upload failed")
		return
	}
	s, err := h.svc.SetLogo(r.Context(), key)
	if err != nil {
		response.InternalErr(w, r, err, "failed to save logo")
		return
	}
	response.JSON(w, r, http.StatusOK, s)
}

func (h *Handler) StreamLogo(w http.ResponseWriter, r *http.Request) {
	key, err := h.svc.LogoObjectKey(r.Context())
	if err != nil {
		if errors.Is(err, settingsusecase.ErrNotFound) {
			response.NotFound(w, r, "logo not set")
			return
		}
		response.InternalErr(w, r, err, "failed to load logo")
		return
	}
	rc, _, err := h.storage.Download(r.Context(), key)
	if err != nil {
		response.NotFound(w, r, "logo not found")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", storage.MIMEFromLogoKey(key))
	_, _ = io.Copy(w, rc)
}

func (h *Handler) DeleteLogo(w http.ResponseWriter, r *http.Request) {
	key, err := h.svc.LogoObjectKey(r.Context())
	if err == nil {
		_ = h.storage.Delete(r.Context(), key)
	}
	s, err := h.svc.ClearLogo(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "failed to clear logo")
		return
	}
	response.JSON(w, r, http.StatusOK, s)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return err
	}
	return nil
}
