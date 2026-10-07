// Package handler serves the document library API.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/library/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct {
	svc *usecase.Service
}

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

type folderBody struct {
	ParentUUID *uuid.UUID `json:"parent_uuid"`
	Name       *string    `json:"name"`
	SortOrder  *int32     `json:"sort_order"`
}

type itemBody struct {
	FolderUUID  *uuid.UUID `json:"folder_uuid"`
	Name        *string    `json:"name"`
	Description *string    `json:"description"`
	Tags        []string   `json:"tags"`
	AccessLevel *string    `json:"access_level"`
	RoleSlug    *string    `json:"role_slug"`
}

func (h *Handler) ListFolders(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListFolders(r.Context(), actor(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"folders": out})
}

func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	var body folderBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.CreateFolder(r.Context(), actor(r), usecase.FolderInput(body))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) PatchFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body folderBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.UpdateFolder(r.Context(), actor(r), id, usecase.FolderInput(body))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteFolder(r.Context(), actor(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// ListItems handles GET /v1/library: folder, tag (CSV, any of),
// access_level (CSV), updated_from/updated_to, q, locale, sort, limit, offset
// (docs/list-contract.md).
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	var folderID *uuid.UUID
	if raw := qv.Get("folder"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "folder", Message: "must be a uuid"}})
			return
		}
		folderID = &id
	}
	q := apiquery.Parse(qv)
	srt, err := apiquery.ResolveSort(q.Sort, usecase.ItemsSortSpec)
	if err != nil {
		writeError(w, r, err)
		return
	}
	in := usecase.ListInput{
		FolderUUID: folderID,
		Tags:       apiquery.CSVValues(qv, "tag"),
		Query:      q.Q,
		Locale:     qv.Get("locale"),
		SortKey:    srt.Key,
		SortDesc:   srt.Desc,
		Limit:      q.Limit,
		Offset:     q.Offset,
	}
	if in.AccessLevels, err = apiquery.EnumList(qv, "access_level", usecase.AccessLevelValues...); err != nil {
		writeError(w, r, err)
		return
	}
	if in.Updated, err = apiquery.DateRange(qv, "updated"); err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.ListItems(r.Context(), actor(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var body itemBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.CreateItem(r.Context(), actor(r), usecase.ItemInput(body))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) PatchItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var body itemBody
	if !decode(w, r, &body) {
		return
	}
	out, err := h.svc.UpdateItem(r.Context(), actor(r), id, usecase.ItemInput(body))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) ArchiveItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.ArchiveItem(r.Context(), actor(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"archived": true})
}

func (h *Handler) AddVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(usecase.MaxUploadBytes + (1 << 20)); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "multipart body is invalid or too large"}})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "is required"}})
		return
	}
	defer func() { _ = file.Close() }()
	locale := r.FormValue("locale")
	if locale == "" {
		locale = r.URL.Query().Get("locale")
	}
	out, err := h.svc.AddVersion(r.Context(), actor(r), id, usecase.UploadInput{
		Locale: locale, Filename: header.Filename, Size: header.Size, Body: file,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.ListVersions(r.Context(), actor(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"versions": out})
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Download(r.Context(), actor(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func actor(r *http.Request) usecase.Actor {
	return usecase.ActorFrom(authctx.MustPrincipal(r.Context()), orgctx.MustScope(r.Context()))
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.NotFound(w, r, "Resource not found")
		return uuid.Nil, false
	}
	return id, true
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
		response.Forbidden(w, r, "You cannot manage the document library from this organization")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Library resource was not found")
	case errors.Is(err, usecase.ErrFolderNotEmpty):
		response.Conflict(w, r, "LIBRARY_FOLDER_NOT_EMPTY", "Library folder is not empty")
	default:
		response.InternalErr(w, r, err, "library request failed")
	}
}
