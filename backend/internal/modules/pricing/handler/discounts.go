package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	pricing "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// DiscountsHandler serves the buyer discount endpoints (TEC-1064).
type DiscountsHandler struct {
	h   *Handler
	svc *pricing.Discounts
}

// Discounts creates the discount handler sharing h's activity recorder.
func (h *Handler) Discounts(svc *pricing.Discounts) *DiscountsHandler {
	return &DiscountsHandler{h: h, svc: svc}
}

func (d *DiscountsHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pricing.ErrBuyerNotFound) {
		response.NotFound(w, r, "Organization not found under the active organization")
		return
	}
	writeError(w, r, err)
}

// Get (GET /v1/tenant/pricing/discounts/{org_uuid}).
func (d *DiscountsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "org_uuid")
	if !ok {
		return
	}
	v, err := d.svc.Get(r.Context(), viewer(r), id)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

type discountBody struct {
	StandardDiscountPct   *json.Number `json:"standard_discount_pct"`
	FirstOrderDiscountPct *json.Number `json:"first_order_discount_pct"`
}

func numberText(n *json.Number) *string {
	if n == nil {
		return nil
	}
	s := n.String()
	return &s
}

// Set (PUT /v1/tenant/pricing/discounts/{org_uuid}): both discounts; null
// clears one.
func (d *DiscountsHandler) Set(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "org_uuid")
	if !ok {
		return
	}
	var body discountBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	v, err := d.svc.Set(r.Context(), viewer(r), id, pricing.DiscountInput{
		StandardDiscountPct: numberText(body.StandardDiscountPct), FirstOrderDiscountPct: numberText(body.FirstOrderDiscountPct),
	})
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	if d.h.activity != nil {
		var actor *int64
		if p, ok := authctx.PrincipalFrom(r.Context()); ok && p.UserInternal != 0 {
			uid := p.UserInternal
			actor = &uid
		}
		payload := map[string]any{
			"standard_discount_pct":    v.StandardDiscountPct,
			"first_order_discount_pct": v.FirstOrderDiscountPct,
		}
		if org, ok := orgctx.ScopeFrom(r.Context()); ok {
			payload["organization_uuid"] = org.UUID.String()
		}
		d.h.activity.Record(r.Context(), actor, "pricing.discounts_set", "organizations", &id, payload, r)
	}
	response.JSON(w, r, http.StatusOK, v)
}
