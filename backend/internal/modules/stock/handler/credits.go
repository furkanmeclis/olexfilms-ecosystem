package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/credits"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeCreditsNotApplicable: the unit is a roll or a fixed barcode.
const CodeCreditsNotApplicable = "UNIT_CREDITS_NOT_APPLICABLE"

// Credits serves the TEC-1221 credit increase (platform super admin).
type Credits struct {
	svc *stockusecase.UnitCredits
}

// NewCredits creates the handler.
func NewCredits(svc *stockusecase.UnitCredits) *Credits { return &Credits{svc: svc} }

type creditIncreaseBody struct {
	Amount int32  `json:"amount"`
	Reason string `json:"reason"`
}

// Increase (POST /v1/platform/stock/units/{uuid}/credits) adds credits to
// a unit and answers its credit card.
func (h *Credits) Increase(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Stock record not found")
		return
	}
	var body creditIncreaseBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid JSON body")
		return
	}
	in := stockusecase.IncreaseInput{Amount: body.Amount, Reason: body.Reason, Meta: activity.MetaFromRequest(r)}
	if p, ok := authctx.PrincipalFrom(r.Context()); ok && p.UserInternal != 0 {
		uid := p.UserInternal
		in.ActorID = &uid
	}
	out, err := h.svc.Increase(r.Context(), id, in)
	var ve *stockusecase.ValidationError
	switch {
	case err == nil:
		response.JSON(w, r, http.StatusOK, out)
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, stockusecase.ErrCreditsNotApplicable), errors.Is(err, credits.ErrNotApplicable):
		response.Conflict(w, r, CodeCreditsNotApplicable, "The unit carries no usage credits")
	default:
		writeError(w, r, err)
	}
}
