package handler

// TEC-294 (F3-02b): PATCH /v1/measurements/{uuid}/vin.

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

type vinBody struct {
	VIN *string `json:"vin"`
}

// CompleteVIN fills the VIN of a vin_pending measurement: 200 with the
// measurement (status accepted); an invalid VIN is 400 VALIDATION_ERROR, a
// measurement outside the reach 404, another VIN already on it 422
// MEASUREMENT_VIN_ALREADY_SET.
func (h *Handler) CompleteVIN(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "Measurement not found")
	if !ok {
		return
	}
	var b vinBody
	if !decode(w, r, &b) {
		return
	}
	vin := ""
	if b.VIN != nil {
		vin = *b.VIN
	}
	out, err := h.svc.CompleteVIN(r.Context(), panelCaller(r), id, vin)
	switch {
	case err == nil:
		response.JSON(w, r, http.StatusOK, out)
	case errors.Is(err, usecase.ErrVINAlreadySet):
		response.Error(w, r, http.StatusUnprocessableEntity, "MEASUREMENT_VIN_ALREADY_SET",
			"The measurement already has another VIN")
	default:
		writeError(w, r, err)
	}
}
