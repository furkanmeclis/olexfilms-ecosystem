package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

type Handler struct {
	svc *usecase.Service
	now func() time.Time
}

func New(svc *usecase.Service) *Handler {
	return &Handler{svc: svc, now: time.Now}
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

func caller(r *http.Request) usecase.Caller {
	return usecase.Caller{Org: orgctx.MustScope(r.Context())}
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, "Invalid performance map query")
	default:
		response.InternalErr(w, r, err, "performance map request failed")
	}
}
