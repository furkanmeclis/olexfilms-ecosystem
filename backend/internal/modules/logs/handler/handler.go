package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/model"
	logsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct {
	svc *logsusecase.Service
}

func New(svc *logsusecase.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformLogs())
}

func (h *Handler) RulesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformLogRules())
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	filter, err := parseListFilter(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.List(r.Context(), q, filter)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Stats(r.Context())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

func (h *Handler) Sources(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Sources(r.Context())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r)
	if !ok {
		return
	}
	item, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) Purge(w http.ResponseWriter, r *http.Request) {
	var in model.PurgeInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	out, err := h.svc.Purge(r.Context(), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListRules(r.Context())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, int64(len(items)), int32(len(items)), 0))
}

func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var in model.RuleInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	item, err := h.svc.CreateRule(r.Context(), p.UserInternal, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) GetRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r)
	if !ok {
		return
	}
	item, err := h.svc.GetRule(r.Context(), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) PatchRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r)
	if !ok {
		return
	}
	var in model.RuleInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	item, err := h.svc.PatchRule(r.Context(), id, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteRule(r.Context(), id); err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) RunRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r)
	if !ok {
		return
	}
	item, deleted, err := h.svc.RunRule(r.Context(), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"rule":    item,
		"deleted": deleted,
	})
}

func parseListFilter(r *http.Request) (model.ListFilter, error) {
	values := r.URL.Query()
	levels, err := apiquery.EnumList(values, "level", model.LevelDebug, model.LevelWarn, model.LevelError)
	if err != nil {
		return model.ListFilter{}, err
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return model.ListFilter{}, err
	}
	filter := model.ListFilter{
		Levels:      levels,
		Source:      strings.TrimSpace(values.Get("source")),
		Q:           strings.TrimSpace(values.Get("q")),
		CreatedFrom: created.From,
		CreatedTo:   created.Before,
	}
	if raw := strings.TrimSpace(values.Get("older_than_hours")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || n < 1 {
			return model.ListFilter{}, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "older_than_hours", Message: "must be a positive integer", Code: "invalid",
			}}}
		}
		v := int32(n)
		filter.OlderHours = &v
	}
	return filter, nil
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("uuid")))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid uuid")
		return uuid.UUID{}, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return err
	}
	return nil
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve *apiquery.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, logsusecase.ErrNotFound):
		response.NotFound(w, r, "Log was not found")
	case errors.Is(err, logsusecase.ErrSystemRule):
		response.Conflict(w, r, response.CodeConflict, "System purge rules cannot be deleted")
	default:
		response.InternalErr(w, r, err, "failed to process logs request")
	}
}
