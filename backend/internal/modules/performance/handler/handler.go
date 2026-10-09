package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type Handler struct {
	svc *usecase.Service
	now func() time.Time
}

func New(svc *usecase.Service) *Handler { return &Handler{svc: svc, now: time.Now} }

func caller(r *http.Request) usecase.Caller {
	f, _ := scopefilter.From(r.Context())
	return usecase.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context()), Filter: f}
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	var qe *apiquery.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message, Code: ve.Code}})
	case errors.As(err, &qe):
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.Is(err, usecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, "Invalid performance map query")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Performance record not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "This organization cannot access performance records")
	default:
		response.InternalErr(w, r, err, "performance request failed")
	}
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

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Performance record not found")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Dashboard(r.Context(), caller(r), r.URL.Query().Get("period"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) Ranking(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseRankingFilter(r.URL.Query(), h.now())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.ListRanking(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

func (h *Handler) Benchmark(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Benchmark(r.Context(), caller(r), r.URL.Query().Get("period"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) RegionMap(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseMapFilter(r.URL.Query(), h.now())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out, err := h.svc.RegionMap(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) DealersMap(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseMapFilter(r.URL.Query(), h.now())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out, err := h.svc.DealerMap(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) ListTargets(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseTargetFilter(r.URL.Query())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, total, err := h.svc.ListTargets(r.Context(), caller(r), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

func (h *Handler) CreateTarget(w http.ResponseWriter, r *http.Request) {
	var in usecase.TargetInput
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.CreateTarget(r.Context(), caller(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) UpdateTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in usecase.TargetInput
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.UpdateTarget(r.Context(), caller(r), id, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) DeleteTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteTarget(r.Context(), caller(r), id); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListStaffTargets(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListStaffTargets(r.Context(), caller(r), usecase.StaffTargetFilter{
		PeriodFrom: r.URL.Query().Get("period_from"), PeriodTo: r.URL.Query().Get("period_to"),
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) UpsertStaffTarget(w http.ResponseWriter, r *http.Request) {
	var in usecase.StaffTargetInput
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.UpsertStaffTarget(r.Context(), caller(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) DeleteStaffTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteStaffTarget(r.Context(), caller(r), id); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	active, err := apiquery.Bool(r.URL.Query(), "active")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, err := h.svc.ListRules(r.Context(), caller(r), usecase.RuleFilter{Active: active})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var in usecase.RuleInput
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.CreateRule(r.Context(), caller(r), in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in usecase.RuleInput
	if !decode(w, r, &in) {
		return
	}
	item, err := h.svc.UpdateRule(r.Context(), caller(r), id, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteRule(r.Context(), caller(r), id); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
