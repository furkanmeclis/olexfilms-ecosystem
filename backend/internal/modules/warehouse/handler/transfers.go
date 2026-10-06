package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the move and warehouse transfer endpoints (TEC-205).
const (
	CodeWarehouseTransferState    = "WAREHOUSE_TRANSFER_STATE"
	CodeWarehouseTransferEmpty    = "WAREHOUSE_TRANSFER_EMPTY"
	CodeWarehouseTransferUnplaced = "WAREHOUSE_TRANSFER_UNPLACED"
	CodeWarehouseUnitUnavailable  = "WAREHOUSE_UNIT_UNAVAILABLE"
	CodeWarehouseUnitBusy         = "WAREHOUSE_UNIT_BUSY"
	CodeWarehouseLedgerRefused    = "WAREHOUSE_LEDGER_REFUSED"
	CodeWarehouseOrderNotReceived = "WAREHOUSE_ORDER_NOT_RECEIVED"
)

// Transfers serves /v1/warehouse/moves, /v1/warehouse/transfers and
// /v1/warehouse/orders/{uuid}/place.
type Transfers struct {
	svc *wh.WarehouseTransfers
}

// NewTransfers builds the handler.
func NewTransfers(svc *wh.WarehouseTransfers) *Transfers { return &Transfers{svc: svc} }

func writeTransferError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, wh.ErrTransferNotFound):
		response.NotFound(w, r, "Warehouse transfer not found")
	case errors.Is(err, wh.ErrTransferLineNotFound):
		response.NotFound(w, r, "Warehouse transfer line not found")
	case errors.Is(err, wh.ErrOrderNotFound):
		response.NotFound(w, r, "Order not found")
	case errors.Is(err, wh.ErrTransferState):
		response.Conflict(w, r, CodeWarehouseTransferState, "The warehouse transfer is not in a status that allows this step")
	case errors.Is(err, wh.ErrTransferEmpty):
		response.Conflict(w, r, CodeWarehouseTransferEmpty, "The warehouse transfer has no lines")
	case errors.Is(err, wh.ErrTransferUnplaced):
		response.Conflict(w, r, CodeWarehouseTransferUnplaced, "Every line needs a target location in the target warehouse")
	case errors.Is(err, wh.ErrUnitBusy):
		response.Conflict(w, r, CodeWarehouseUnitBusy, "The unit is in transit or held by another transfer, request or order")
	case errors.Is(err, wh.ErrUnitUnavailable):
		response.Conflict(w, r, CodeWarehouseUnitUnavailable, "The unit is not on hand in the expected warehouse")
	case errors.Is(err, wh.ErrOrderNotReceived):
		response.Conflict(w, r, CodeWarehouseOrderNotReceived, "The order is not received yet")
	case errors.Is(err, wh.ErrTransferLedger):
		msg := "The stock ledger refused the movement"
		if errors.Is(err, ledger.ErrOwnerNotAllowed) || errors.Is(err, ledger.ErrTransitionNotAllowed) || errors.Is(err, ledger.ErrOwnerMismatch) {
			msg += ": a unit or location no longer fits"
		}
		response.Conflict(w, r, CodeWarehouseLedgerRefused, msg)
	default:
		writeError(w, r, err)
	}
}

type moveBody struct {
	Barcodes     []string   `json:"barcodes"`
	LocationUUID *uuid.UUID `json:"location_uuid"`
	LocationCode string     `json:"location_code"`
}

type transferBody struct {
	FromWarehouseUUID string     `json:"from_warehouse_uuid"`
	ToWarehouseUUID   string     `json:"to_warehouse_uuid"`
	ToLocationUUID    *uuid.UUID `json:"to_location_uuid"`
	Note              *string    `json:"note"`
}

type transferLinesBody struct {
	Barcodes []string `json:"barcodes"`
}

type completeBody struct {
	LocationUUID *uuid.UUID `json:"location_uuid"`
	LocationCode string     `json:"location_code"`
}

// Move (POST /v1/warehouse/moves).
func (h *Transfers) Move(w http.ResponseWriter, r *http.Request) {
	var b moveBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.Move(r.Context(), entryCaller(r), wh.MoveInput{Barcodes: b.Barcodes, LocationUUID: b.LocationUUID, LocationCode: b.LocationCode})
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PlaceOrder (POST /v1/warehouse/orders/{uuid}/place).
func (h *Transfers) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b moveBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.PlaceOrder(r.Context(), entryCaller(r), id, wh.MoveInput{Barcodes: b.Barcodes, LocationUUID: b.LocationUUID, LocationCode: b.LocationCode})
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List (GET /v1/warehouse/transfers?status&from_warehouse_uuid&to_warehouse_uuid&created_from&created_to&q&sort&limit&offset).
func (h *Transfers) List(w http.ResponseWriter, r *http.Request) {
	f, err := wh.ParseTransferListFilter(r.URL.Query())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.ListTransfers(r.Context(), entryCaller(r), f)
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// Create (POST /v1/warehouse/transfers).
func (h *Transfers) Create(w http.ResponseWriter, r *http.Request) {
	var b transferBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.CreateTransfer(r.Context(), entryCaller(r), wh.TransferInput{
		FromWarehouseUUID: b.FromWarehouseUUID, ToWarehouseUUID: b.ToWarehouseUUID, ToLocationUUID: b.ToLocationUUID, Note: b.Note,
	})
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Get (GET /v1/warehouse/transfers/{uuid}).
func (h *Transfers) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetTransfer(r.Context(), entryCaller(r), id)
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// AddLines (POST /v1/warehouse/transfers/{uuid}/lines).
func (h *Transfers) AddLines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b transferLinesBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.AddTransferLines(r.Context(), entryCaller(r), id, b.Barcodes)
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteLine (DELETE /v1/warehouse/transfers/{uuid}/lines/{line_uuid}).
func (h *Transfers) DeleteLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	lid, ok := lineUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.DeleteTransferLine(r.Context(), entryCaller(r), id, lid)
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Place (POST /v1/warehouse/transfers/{uuid}/place).
func (h *Transfers) Place(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b placeBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.PlaceTransferLines(r.Context(), entryCaller(r), id, wh.PlaceInput{
		LineUUIDs: b.LineUUIDs, Barcodes: b.Barcodes, LocationUUID: b.LocationUUID, LocationCode: b.LocationCode,
	})
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Ship (POST /v1/warehouse/transfers/{uuid}/ship).
func (h *Transfers) Ship(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Ship(r.Context(), entryCaller(r), id)
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Complete (POST /v1/warehouse/transfers/{uuid}/complete). The body is
// optional.
func (h *Transfers) Complete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b completeBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	out, err := h.svc.Complete(r.Context(), entryCaller(r), id, wh.CompleteInput{LocationUUID: b.LocationUUID, LocationCode: b.LocationCode})
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Cancel (POST /v1/warehouse/transfers/{uuid}/cancel).
func (h *Transfers) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Cancel(r.Context(), entryCaller(r), id)
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
