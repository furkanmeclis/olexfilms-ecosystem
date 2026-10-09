// Package handler serves the /v1/reports contract (TEC-495, F5-05f).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/reports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// maxLayoutBody caps a PUT /v1/reports/layout body.
const maxLayoutBody = 64 << 10

type Handler struct {
	svc *usecase.Service
}

func New(svc *usecase.Service) *Handler {
	return &Handler{svc: svc}
}

func caller(r *http.Request) usecase.Caller {
	return usecase.Caller{Principal: authctx.MustPrincipal(r.Context()), Org: orgctx.MustScope(r.Context())}
}

func input(r *http.Request) usecase.Input {
	v := r.URL.Query()
	return usecase.Input{
		Period: v.Get("period"), RangeFrom: v.Get("range_from"), RangeTo: v.Get("range_to"),
		Granularity: v.Get("granularity"), Limit: v.Get("limit"), Locale: v.Get("locale"),
		Fallback: i18n.FromContext(r.Context()),
	}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message, Code: ve.Code}})
	case errors.Is(err, usecase.ErrFeatureDisabled):
		response.Error(w, r, http.StatusForbidden, response.CodeFeatureDisabled, "This feature is not enabled for your organization")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "Report access denied")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Report not found")
	default:
		response.InternalErr(w, r, err, "report request failed")
	}
}

// Catalog: GET /v1/reports/catalog.
func (h *Handler) Catalog(w http.ResponseWriter, r *http.Request) {
	in := input(r)
	locale := in.Fallback.Locale
	if in.Locale != "" {
		l, ok := i18n.Parse(in.Locale)
		if !ok {
			response.ValidationError(w, r, []response.Detail{{Field: "locale", Message: "unsupported locale", Code: "invalid"}})
			return
		}
		locale = l
	}
	items, err := h.svc.Catalog(r.Context(), caller(r), locale)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Overview: GET /v1/reports/overview.
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	h.report(w, r, usecase.ReportOverview)
}

// Report: GET /v1/reports/{report}.
func (h *Handler) Report(w http.ResponseWriter, r *http.Request) {
	h.report(w, r, r.PathValue("report"))
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request, key string) {
	env, err := h.svc.Report(r.Context(), caller(r), key, input(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, env)
}

// GetLayout: GET /v1/reports/layout.
func (h *Handler) GetLayout(w http.ResponseWriter, r *http.Request) {
	l, err := h.svc.GetLayout(r.Context(), caller(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, l)
}

// PutLayout: PUT /v1/reports/layout (atomic replace).
func (h *Handler) PutLayout(w http.ResponseWriter, r *http.Request) {
	var in usecase.LayoutInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLayoutBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	l, err := h.svc.PutLayout(r.Context(), caller(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, l)
}
