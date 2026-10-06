package handler

import (
	"net/http"

	activityusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/activity/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

type Handler struct {
	svc *activityusecase.Service
}

func New(svc *activityusecase.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.Activity())
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	params, err := activity.ListParams(r.URL.Query())
	if err != nil {
		if !response.QueryValidation(w, r, err) {
			response.InternalErr(w, r, err, "failed to list activity")
		}
		return
	}
	items, total, err := h.svc.List(r.Context(), params, q.Limit, q.Offset)
	if err != nil {
		response.InternalErr(w, r, err, "failed to list activity")
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}
