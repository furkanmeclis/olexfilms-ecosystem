package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Roll split error codes (TEC-184); frontend/src/locales/*/errors.json
// translates them.
const (
	CodeSplitNotRoll            = "STOCK_SPLIT_NOT_ROLL"
	CodeSplitInsufficientMeters = "STOCK_SPLIT_INSUFFICIENT_METERS"
	CodeSplitUnitElsewhere      = "STOCK_SPLIT_UNIT_ELSEWHERE"
	CodeSplitUnitInUse          = "STOCK_SPLIT_UNIT_IN_USE"
	CodeSplitKeyConflict        = "STOCK_SPLIT_KEY_CONFLICT"
)

var splitErrors = []reclassError{
	{ledger.ErrSplitNotRoll, http.StatusUnprocessableEntity, CodeSplitNotRoll, "Only rolls can be split"},
	{ledger.ErrInsufficientMeters, http.StatusUnprocessableEntity, CodeSplitInsufficientMeters, "The roll has fewer meters left than requested"},
	{ledger.ErrSplitUnitElsewhere, http.StatusConflict, CodeSplitUnitElsewhere, "The roll is not on hand at the organization"},
	{stockusecase.ErrReclassUnitInUse, http.StatusConflict, CodeSplitUnitInUse, "The roll is on an open service or order"},
	{ledger.ErrIdempotencyConflict, http.StatusConflict, CodeSplitKeyConflict, "The idempotency key was used for another split"},
}

// Split serves POST /v1/stock/splits (TEC-184).
type Split struct {
	svc *stockusecase.Splits
}

// NewSplit creates the handler.
func NewSplit(svc *stockusecase.Splits) *Split { return &Split{svc: svc} }

type splitBody struct {
	Barcode        string `json:"barcode"`
	UnitUUID       string `json:"unit_uuid"`
	Meters         string `json:"meters"`
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
}

// Create serves POST /v1/stock/splits: 201 with the split, 200 when the
// idempotency key was already used (the earlier split, nothing written).
func (h *Split) Create(w http.ResponseWriter, r *http.Request) {
	var body splitBody
	if !decodeOptional(w, r, &body) {
		return
	}
	key := body.IdempotencyKey
	if key == "" {
		key = r.Header.Get("Idempotency-Key")
	}
	out, replayed, err := h.svc.Split(r.Context(), reclassCaller(r), stockusecase.SplitInput{
		Barcode: body.Barcode, UnitUUID: body.UnitUUID, Meters: body.Meters,
		IdempotencyKey: key, Reason: body.Reason,
	})
	if err != nil {
		writeSplitError(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	response.JSON(w, r, status, out)
}

func writeSplitError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *stockusecase.ValidationError
	if errors.As(err, &ve) {
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
		return
	}
	for _, e := range splitErrors {
		if errors.Is(err, e.err) {
			response.Error(w, r, e.status, e.code, e.msg)
			return
		}
	}
	switch {
	case errors.Is(err, ledger.ErrInvalidMovement):
		response.ValidationError(w, r, []response.Detail{{Field: "meters", Message: err.Error()}})
	case errors.Is(err, stockusecase.ErrNotFound), errors.Is(err, ledger.ErrUnitNotFound):
		response.NotFound(w, r, "Unit not found")
	case errors.Is(err, ledger.ErrConcurrentUpdate):
		response.Conflict(w, r, response.CodeConflict, "The unit changed concurrently, retry")
	default:
		response.InternalErr(w, r, err, "roll split failed")
	}
}
