package handler

import (
	"net/http"
	"strconv"
	"strings"

	searchusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/usecase"
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
