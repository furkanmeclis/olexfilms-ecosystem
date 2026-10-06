package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the stock entry endpoints (TEC-204).
const (
	CodeStockEntryNotDraft      = "STOCK_ENTRY_NOT_DRAFT"
	CodeStockEntryEmpty         = "STOCK_ENTRY_EMPTY"
	CodeStockEntryUnplaced      = "STOCK_ENTRY_UNPLACED"
	CodeStockEntryCenterOnly    = "STOCK_ENTRY_CENTER_ONLY"
	CodeStockEntryUnitNotLabel  = "STOCK_ENTRY_UNIT_NOT_PRINTED"
	CodeStockEntryUnitTaken     = "STOCK_ENTRY_UNIT_TAKEN"
	CodeStockEntryLedgerRefused = "STOCK_ENTRY_LEDGER_REFUSED"
)

// Entries serves /v1/warehouse/stock-entries.
type Entries struct {
	svc *wh.StockEntries
}

// NewEntries builds the stock entry handler.
func NewEntries(svc *wh.StockEntries) *Entries { return &Entries{svc: svc} }

func entryCaller(r *http.Request) wh.EntryCaller {
	return wh.EntryCaller{Caller: caller(r), Principal: authctx.MustPrincipal(r.Context())}
}

func writeEntryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, wh.ErrEntryNotFound):
		response.NotFound(w, r, "Stock entry not found")
	case errors.Is(err, wh.ErrEntryLineNotFound):
		response.NotFound(w, r, "Stock entry line not found")
	case errors.Is(err, wh.ErrEntryNotDraft):
		response.Conflict(w, r, CodeStockEntryNotDraft, "The stock entry is no longer a draft (already confirmed or cancelled)")
	case errors.Is(err, wh.ErrEntryEmpty):
		response.Conflict(w, r, CodeStockEntryEmpty, "The stock entry has no lines")
	case errors.Is(err, wh.ErrEntryUnplaced):
		response.Conflict(w, r, CodeStockEntryUnplaced, "Place every line on a location before confirming")
	case errors.Is(err, wh.ErrEntryCenterOnly):
		response.Error(w, r, http.StatusForbidden, CodeStockEntryCenterOnly, "Stock enters and barcodes are generated at the center only")
	case errors.Is(err, wh.ErrEntryUnitNotEnterable):
		response.Conflict(w, r, CodeStockEntryUnitNotLabel, "The barcode is not a printed label waiting for stock")
	case errors.Is(err, wh.ErrEntryUnitTaken):
		response.Conflict(w, r, CodeStockEntryUnitTaken, "The barcode is already on another stock entry")
	case errors.Is(err, wh.ErrEntryLedger):
		msg := "The stock ledger refused the entry"
		if errors.Is(err, ledger.ErrOwnerNotAllowed) || errors.Is(err, ledger.ErrTransitionNotAllowed) {
			msg += ": a unit or location no longer fits"
		}
		response.Conflict(w, r, CodeStockEntryLedgerRefused, msg)
	default:
		writeError(w, r, err)
	}
}

func lineUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("line_uuid"))
	if err != nil {
		response.NotFound(w, r, "Resource not found")
		return uuid.Nil, false
	}
	return id, true
}

type entryBody struct {
	WarehouseUUID string  `json:"warehouse_uuid"`
	Mode          string  `json:"mode"`
	Note          *string `json:"note"`
}

type entryLinesBody struct {
	Barcodes      []string `json:"barcodes"`
	ProductUUID   string   `json:"product_uuid"`
	Quantity      int      `json:"quantity"`
	Meters        string   `json:"meters"`
	Prefix        string   `json:"prefix"`
	TemplateUUID  string   `json:"template_uuid"`
	FixedQuantity *int32   `json:"fixed_quantity"`
}

type placeBody struct {
	LineUUIDs    []uuid.UUID `json:"line_uuids"`
	Barcodes     []string    `json:"barcodes"`
	LocationUUID *uuid.UUID  `json:"location_uuid"`
	LocationCode string      `json:"location_code"`
}

// List (GET /v1/warehouse/stock-entries?status&mode&warehouse_uuid&created_from&created_to&q&sort&limit&offset).
func (h *Entries) List(w http.ResponseWriter, r *http.Request) {
	f, err := wh.ParseEntryListFilter(r.URL.Query())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.svc.List(r.Context(), entryCaller(r), f)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// Create (POST /v1/warehouse/stock-entries).
func (h *Entries) Create(w http.ResponseWriter, r *http.Request) {
	var b entryBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.Create(r.Context(), entryCaller(r), wh.EntryInput{WarehouseUUID: b.WarehouseUUID, Mode: b.Mode, Note: b.Note})
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// Get (GET /v1/warehouse/stock-entries/{uuid}).
func (h *Entries) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), entryCaller(r), id)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// AddLines (POST /v1/warehouse/stock-entries/{uuid}/lines).
func (h *Entries) AddLines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b entryLinesBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.AddLines(r.Context(), entryCaller(r), id, wh.EntryLinesInput{
		Barcodes: b.Barcodes, ProductUUID: b.ProductUUID, Count: b.Quantity, Meters: b.Meters,
		Prefix: b.Prefix, TemplateUUID: b.TemplateUUID, FixedQuantity: b.FixedQuantity,
	})
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteLine (DELETE /v1/warehouse/stock-entries/{uuid}/lines/{line_uuid}).
func (h *Entries) DeleteLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	lid, ok := lineUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.DeleteLine(r.Context(), entryCaller(r), id, lid)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Place (POST /v1/warehouse/stock-entries/{uuid}/place).
func (h *Entries) Place(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b placeBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.Place(r.Context(), entryCaller(r), id, wh.PlaceInput{
		LineUUIDs: b.LineUUIDs, Barcodes: b.Barcodes, LocationUUID: b.LocationUUID, LocationCode: b.LocationCode,
	})
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Confirm (POST /v1/warehouse/stock-entries/{uuid}/confirm).
func (h *Entries) Confirm(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Confirm(r.Context(), entryCaller(r), id)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Cancel (POST /v1/warehouse/stock-entries/{uuid}/cancel).
func (h *Entries) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Cancel(r.Context(), entryCaller(r), id)
	if err != nil {
		writeEntryError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
