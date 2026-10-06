// Package handler exposes the document template editor (platform) and
// document render/download (tenant) endpoints.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Preview calls Gotenberg synchronously: at most previewLimit per user and
// minute.
const previewLimit = 30

// Handler serves document endpoints.
type Handler struct {
	svc     *docusecase.Service
	limiter *ratelimit.Limiter
}

// New builds the handler. limiter may be nil (no preview limit).
func New(svc *docusecase.Service, limiter *ratelimit.Limiter) *Handler {
	return &Handler{svc: svc, limiter: limiter}
}

// Kinds lists document kinds with their variable schema.
func (h *Handler) Kinds(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, h.svc.Kinds())
}

// List lists template versions.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	q := apiquery.Parse(qv)
	sort, err := apiquery.ResolveSort(q.Sort, docusecase.TemplatesSortSpec)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	f := docusecase.ListFilter{
		BrandSlug: strings.TrimSpace(qv.Get("brand")), Q: q.Q,
		CurrentOnly: qv.Get("current") != "false",
		SortKey:     sort.Key, SortDesc: sort.Desc,
		Limit: q.Limit, Offset: q.Offset,
	}
	if f.Kinds, err = apiquery.EnumList(qv, "kind", model.Kinds...); err != nil {
		writeErr(w, r, err)
		return
	}
	if f.Statuses, err = apiquery.EnumList(qv, "status", docusecase.TemplateStatuses...); err != nil {
		writeErr(w, r, err)
		return
	}
	for _, raw := range apiquery.CSVValues(qv, "language") {
		lang := model.NormalizeLanguage(raw)
		if lang == "" {
			writeErr(w, r, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "language", Message: "invalid value " + strconv.Quote(raw), Code: "invalid",
			}}})
			return
		}
		f.Languages = append(f.Languages, lang)
	}
	defaultOnly, err := apiquery.Bool(qv, "platform_default")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	f.DefaultOnly = defaultOnly != nil && *defaultOnly
	items, total, err := h.svc.ListTemplates(r.Context(), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Get returns one version with HTML and editor state.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.GetTemplate(r.Context(), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Versions lists every version of the template's kind/brand/language.
func (h *Handler) Versions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Versions(r.Context(), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, items)
}

type saveBody struct {
	Kind        string          `json:"kind"`
	BrandSlug   string          `json:"brand_slug"`
	Language    string          `json:"language"`
	Name        string          `json:"name"`
	HTML        string          `json:"html"`
	LexicalJSON json.RawMessage `json:"lexical_json"`
}

func (b saveBody) input() docusecase.SaveInput {
	return docusecase.SaveInput{
		Kind: b.Kind, BrandSlug: b.BrandSlug, Language: b.Language, Name: b.Name,
		HTML: b.HTML, LexicalJSON: b.LexicalJSON,
	}
}

// SaveDraft creates or updates the open draft of a kind/brand/language.
func (h *Handler) SaveDraft(w http.ResponseWriter, r *http.Request) {
	var in saveBody
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	v, err := h.svc.SaveDraft(r.Context(), p.UserInternal, in.input())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Update edits a draft version.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in saveBody
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	v, err := h.svc.UpdateDraft(r.Context(), id, in.input())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Publish activates a draft version.
func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Publish(r.Context(), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Preview streams a PDF of editor content or a stored version with sample
// data.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if h.limiter != nil {
		if ok, retry := h.limiter.Allow(r.Context(), "documents.preview", strconv.FormatInt(p.UserInternal, 10), previewLimit, time.Minute); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			response.TooManyRequests(w, r, "too many previews; try again shortly")
			return
		}
	}
	var in struct {
		Kind         string     `json:"kind"`
		TemplateUUID *uuid.UUID `json:"template_uuid"`
		HTML         string     `json:"html"`
		Locale       string     `json:"locale"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	pdf, err := h.svc.Preview(r.Context(), docusecase.PreviewInput{
		Kind: in.Kind, TemplateUUID: in.TemplateUUID, HTML: in.HTML, Locale: in.Locale,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="preview.pdf"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	_, _ = w.Write(pdf)
}

// RequestRender asks for a document of a business record in the active
// organization: 200 with the ready document, or 202 while it renders.
func (h *Handler) RequestRender(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind     string `json:"kind"`
		SourceID string `json:"source_id"`
		Locale   string `json:"locale"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	scope := orgctx.MustScope(r.Context())
	v, ready, err := h.svc.RequestRender(r.Context(), model.Viewer{
		UserID: p.UserInternal, OrganizationID: scope.InternalID, BrandID: scope.BrandID,
	}, docusecase.RenderInput{Kind: in.Kind, SourceID: in.SourceID, Locale: in.Locale})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	status := http.StatusAccepted
	if ready {
		status = http.StatusOK
	}
	response.JSON(w, r, status, v)
}

// GetRender returns the status of a document of the active organization.
func (h *Handler) GetRender(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	scope := orgctx.MustScope(r.Context())
	v, err := h.svc.GetRender(r.Context(), scope.InternalID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Download streams a ready PDF from storage.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	scope := orgctx.MustScope(r.Context())
	rc, name, err := h.svc.Download(r.Context(), scope.InternalID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, name, url.PathEscape(name)))
	w.Header().Set("Cache-Control", "private, max-age=0")
	_, _ = io.Copy(w, rc)
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var unknown *docusecase.UnknownVariablesError
	var ve *apiquery.ValidationError
	switch {
	case errors.As(err, &ve):
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.As(err, &unknown):
		details := make([]response.Detail, 0, len(unknown.Keys))
		for _, k := range unknown.Keys {
			details = append(details, response.Detail{Field: "html", Message: "unknown variable: " + k})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, response.CodeValidationError, unknown.Error(), details)
	case errors.Is(err, docusecase.ErrNotFound):
		response.NotFound(w, r, "document not found")
	case errors.Is(err, docusecase.ErrConflict):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, docusecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	case errors.Is(err, docusecase.ErrUnavailable):
		response.Error(w, r, http.StatusServiceUnavailable, "PDF_UNAVAILABLE", "PDF renderer is unavailable")
	default:
		response.InternalErr(w, r, err, "document request failed")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid body")
		return err
	}
	return nil
}
