package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/model"
	storageusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

type Handler struct {
	svc *storageusecase.Service
}

func New(svc *storageusecase.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformStorage())
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	if err := apiquery.ValidateSort(q.Sort, apiquery.StorageSort); err != nil {
		var ve *apiquery.ValidationError
		if errors.As(err, &ve) {
			details := make([]response.Detail, 0, len(ve.Details))
			for _, d := range ve.Details {
				details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
			}
			response.ValidationError(w, r, details)
			return
		}
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	sortSpec := "name"
	if field, desc, ok := apiquery.PrimarySort(q.Sort, apiquery.StorageSort); ok {
		if desc {
			sortSpec = "-" + field
		} else {
			sortSpec = field
		}
	}
	vals := r.URL.Query()
	out, err := h.svc.List(r.Context(), actor(r), model.ListInput{
		Prefix:       vals.Get("prefix"),
		View:         vals.Get("view"),
		Q:            q.Q,
		Kind:         vals.Get("kind"),
		Access:       vals.Get("access"),
		Sort:         sortSpec,
		Limit:        q.Limit,
		Offset:       q.Offset,
		ModifiedFrom: vals.Get("modified_from"),
		ModifiedTo:   vals.Get("modified_to"),
		Recursive:    vals.Get("recursive") == "true",
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	obj, err := h.svc.Get(r.Context(), actor(r), r.URL.Query().Get("key"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, obj)
}

func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Usage(r.Context())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	var in model.CreateFolderInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	obj, err := h.svc.CreateFolder(r.Context(), actor(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, obj)
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	obj, err := h.svc.Upload(r.Context(), actor(r), model.UploadMeta{
		Prefix:      r.FormValue("prefix"),
		Key:         r.FormValue("key"),
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Size:        header.Size,
	}, file)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, obj)
}

func (h *Handler) CreateUploadSession(w http.ResponseWriter, r *http.Request) {
	var in model.UploadMeta
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	out, err := h.svc.CreateUploadSession(r.Context(), actor(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string `json:"key"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	obj, err := h.svc.CompleteUpload(r.Context(), actor(r), in.Key)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, obj)
}

func (h *Handler) Copy(w http.ResponseWriter, r *http.Request) {
	h.copyMove(w, r, false)
}

func (h *Handler) Move(w http.ResponseWriter, r *http.Request) {
	h.copyMove(w, r, true)
}

func (h *Handler) copyMove(w http.ResponseWriter, r *http.Request, move bool) {
	var in model.CopyMoveInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	var (
		obj model.Object
		err error
	)
	if move {
		obj, err = h.svc.Move(r.Context(), actor(r), in)
	} else {
		obj, err = h.svc.Copy(r.Context(), actor(r), in)
	}
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, obj)
}

func (h *Handler) Rename(w http.ResponseWriter, r *http.Request) {
	var in model.RenameInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	obj, err := h.svc.Rename(r.Context(), actor(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, obj)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	var in model.KeysInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.svc.Delete(r.Context(), actor(r), in.Keys); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "trashed"})
}

func (h *Handler) Restore(w http.ResponseWriter, r *http.Request) {
	var in model.KeysInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.svc.Restore(r.Context(), actor(r), in.Keys); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "restored"})
}

func (h *Handler) Purge(w http.ResponseWriter, r *http.Request) {
	var in model.KeysInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.svc.Purge(r.Context(), actor(r), in.Keys); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	h.stream(w, r, true)
}

func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	h.stream(w, r, false)
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request, download bool) {
	opened, err := h.svc.Open(
		r.Context(),
		r.URL.Query().Get("key"),
		r.URL.Query().Get("version_id"),
		download,
	)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = opened.Body.Close() }()
	writeFile(w, opened)
}

func (h *Handler) Versions(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Versions(r.Context(), r.URL.Query().Get("key"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) RestoreVersion(w http.ResponseWriter, r *http.Request) {
	var in model.RestoreVersionInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	obj, err := h.svc.RestoreVersion(r.Context(), actor(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, obj)
}

func (h *Handler) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	var in model.RestoreVersionInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.svc.DeleteVersion(r.Context(), actor(r), in.Key, in.VersionID); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Activity(r.Context(), r.URL.Query().Get("key"), q.Limit, q.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) Star(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string `json:"key"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if err := h.svc.Star(r.Context(), actor(r), in.Key); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "starred"})
}

func (h *Handler) Unstar(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Unstar(r.Context(), actor(r), r.URL.Query().Get("key")); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "unstarred"})
}

func (h *Handler) Shares(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Shares(r.Context(), r.URL.Query().Get("key"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Share(w http.ResponseWriter, r *http.Request) {
	var in model.CreateShareInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	item, err := h.svc.Share(r.Context(), actor(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) Unshare(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Unshare(r.Context(), actor(r), r.PathValue("uuid")); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handler) Links(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Links(r.Context(), r.URL.Query().Get("key"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateLink(w http.ResponseWriter, r *http.Request) {
	var in model.CreateLinkInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	item, err := h.svc.CreateLink(r.Context(), actor(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) RevokeLink(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RevokeLink(r.Context(), actor(r), r.PathValue("uuid")); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *Handler) PublicSlug(w http.ResponseWriter, r *http.Request) {
	download := r.URL.Query().Get("download") == "1"
	opened, err := h.svc.ResolvePublic(r.Context(), r.PathValue("slug"), download)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = opened.Body.Close() }()
	writeFile(w, opened)
}

func (h *Handler) PublicSigned(w http.ResponseWriter, r *http.Request) {
	download := r.URL.Query().Get("download") == "1"
	opened, err := h.svc.ResolveSigned(r.Context(), r.PathValue("token"), download)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = opened.Body.Close() }()
	writeFile(w, opened)
}

// inlineSafeMIME reports whether a stored MIME type may render inline on the
// app origin. Files are served same-origin through the BFF, so HTML, SVG,
// XML and other active types would execute script as the viewer
// (stored XSS via public links / preview); those are forced to download.
func inlineSafeMIME(mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/avif", "image/bmp",
		"application/pdf", "text/plain", "text/csv":
		return true
	}
	return strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "audio/")
}

func writeFile(w http.ResponseWriter, opened model.OpenObject) {
	w.Header().Set("Content-Type", opened.Object.MimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if opened.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(opened.Size, 10))
	}
	disp := "inline"
	if opened.Download || !inlineSafeMIME(opened.Object.MimeType) {
		disp = "attachment"
	}
	name := strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, opened.Object.Name)
	w.Header().Set("Content-Disposition", disp+`; filename="`+name+`"`)
	_, _ = io.Copy(w, opened.Body)
}

func actor(r *http.Request) model.Actor {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		return model.Actor{}
	}
	return model.Actor{UserID: p.UserInternal, Name: p.Email}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return err
	}
	return nil
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, storageusecase.ErrNotFound):
		response.NotFound(w, r, "object not found")
	case errors.Is(err, storageusecase.ErrInvalidKey):
		response.BadRequest(w, r, response.CodeValidationError, "invalid object key")
	case errors.Is(err, storageusecase.ErrConflict):
		response.Conflict(w, r, response.CodeConflict, "object already exists")
	case errors.Is(err, storageusecase.ErrForbidden):
		response.Forbidden(w, r, "access denied")
	case errors.Is(err, storageusecase.ErrLinkExpired):
		response.Forbidden(w, r, "link expired")
	case errors.Is(err, storageusecase.ErrLinkRevoked):
		response.Forbidden(w, r, "link revoked")
	default:
		response.InternalErr(w, r, err, "storage operation failed")
	}
}
