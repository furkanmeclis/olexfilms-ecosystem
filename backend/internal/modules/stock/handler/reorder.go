package handler

// TEC-1229: reorder rules (min / reorder point / max per warehouse and
// product) and the critical stock list of the active organization.

import (
	"encoding/json"
	"errors"
	"net/http"

	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

func writeReorderError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, stockusecase.ErrReorderForbidden) {
		response.Forbidden(w, r, "Reorder rules belong to the active organization")
		return
	}
	writeQueryError(w, r, err)
}

func (h *Handler) reorderList(w http.ResponseWriter, r *http.Request, critical bool) {
	in, err := stockusecase.ParseReorderFilter(r.URL.Query())
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	in.CriticalOnly = critical
	items, total, err := h.svc.ReorderRules(r.Context(), reclassCaller(r), in)
	if err != nil {
		writeReorderError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, in.Limit, in.Offset))
}

// ReorderRules serves GET /v1/stock/reorder-rules.
func (h *Handler) ReorderRules(w http.ResponseWriter, r *http.Request) { h.reorderList(w, r, false) }

// CriticalStock serves GET /v1/stock/critical: the rules whose level is not
// ok (below_min, reorder, days_low).
func (h *Handler) CriticalStock(w http.ResponseWriter, r *http.Request) { h.reorderList(w, r, true) }

// PutReorderRule serves PUT /v1/stock/reorder-rules (create or replace the
// rule of a warehouse and product).
func (h *Handler) PutReorderRule(w http.ResponseWriter, r *http.Request) {
	var in stockusecase.ReorderRuleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	out, err := h.svc.UpsertReorderRule(r.Context(), reclassCaller(r), in)
	if err != nil {
		writeReorderError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteReorderRule serves DELETE /v1/stock/reorder-rules/{uuid}.
func (h *Handler) DeleteReorderRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	if err := h.svc.DeleteReorderRule(r.Context(), reclassCaller(r), id); err != nil {
		writeReorderError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
