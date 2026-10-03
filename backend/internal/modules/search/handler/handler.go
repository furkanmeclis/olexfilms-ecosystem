package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	searchusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

type Handler struct {
	svc *searchusecase.Service
}

func New(svc *searchusecase.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Specs(w http.ResponseWriter, r *http.Request) {
	specs := h.svc.ListSpecs(r.Context())
	response.JSON(w, r, http.StatusOK, map[string]any{
		"items":   specs,
		"enabled": h.svc.Enabled(),
	})
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	spec := strings.TrimSpace(r.URL.Query().Get("spec"))
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items := h.svc.Search(r.Context(), q, spec, limit)
	if items == nil {
		items = []searchengine.Hit{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Global answers GET /v1/search/global (TEC-213): the hits of every index
// the caller may read in the active organization, grouped by spec.
func (h *Handler) Global(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	limit := searchusecase.DefaultGroupLimit
	if raw := strings.TrimSpace(v.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > searchusecase.MaxGroupLimit {
			response.ValidationError(w, r, []response.Detail{{Field: "limit", Message: "must be between 1 and 20"}})
			return
		}
		limit = n
	}
	res, err := h.svc.Global(r.Context(), orgctx.MustScope(r.Context()), v.Get("q"), v.Get("spec"), limit)
	if errors.Is(err, searchusecase.ErrQueryTooLong) {
		response.ValidationError(w, r, []response.Detail{{Field: "q", Message: "must be at most 100 characters"}})
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "global search failed")
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}
