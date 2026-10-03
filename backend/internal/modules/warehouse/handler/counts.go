package handler

import (
	"errors"
	"net/http"
	"strings"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the stock count endpoints (TEC-206).
const (
	CodeCountNotFound          = "COUNT_NOT_FOUND"
	CodeCountScanNotFound      = "COUNT_SCAN_NOT_FOUND"
	CodeCountInvalidStatus     = "COUNT_INVALID_STATUS"
	CodeCountStartApproval     = "COUNT_START_APPROVAL_REQUIRED"
	CodeCountUnitScanned       = "COUNT_UNIT_ALREADY_SCANNED"
	CodeCountStale             = "COUNT_STALE"
	CodeCountLocationOutScope  = "COUNT_LOCATION_OUT_OF_SCOPE"
	CodeCountProductOutScope   = "COUNT_PRODUCT_OUT_OF_SCOPE"
	CodeCountScanKind          = "COUNT_SCAN_KIND_NOT_ALLOWED"
	CodeCountLocationRequired  = "COUNT_LOCATION_REQUIRED"
	CodeCountAdjustNotGranted  = "COUNT_ADJUST_FORBIDDEN"
	countUnprocessableFallback = "The scan does not fit this count"
)

// Counts serves /v1/warehouse/stock-counts.
type Counts struct {
	svc *wh.Counts
}

// NewCounts builds the stock count handler.
func NewCounts(svc *wh.Counts) *Counts { return &Counts{svc: svc} }

func countCaller(r *http.Request) wh.ScanCaller {
	return wh.ScanCaller{Caller: caller(r), Principal: authctx.MustPrincipal(r.Context())}
}

func writeCountError(w http.ResponseWriter, r *http.Request, err error) {
	unprocessable := func(code string) {
		msg := err.Error()
		if msg == "" {
			msg = countUnprocessableFallback
		}
		response.Error(w, r, http.StatusUnprocessableEntity, code, strings.TrimPrefix(msg, "warehouse: "))
	}
	switch {
	case errors.Is(err, wh.ErrCountNotFound):
		response.Error(w, r, http.StatusNotFound, CodeCountNotFound, "Stock count not found")
	case errors.Is(err, wh.ErrCountScanNotFound):
		response.Error(w, r, http.StatusNotFound, CodeCountScanNotFound, "Scan not found")
	case errors.Is(err, wh.ErrScanNoMatch):
		response.Error(w, r, http.StatusNotFound, CodeScanNoMatch, "The scanned code matches no location, unit or product you can see")
	case errors.Is(err, wh.ErrScanLocationNotFound):
		response.Error(w, r, http.StatusNotFound, CodeScanLocationNotFound, "The scanned location QR is not a location of this organization")
	case errors.Is(err, wh.ErrCountStatus):
		response.Conflict(w, r, CodeCountInvalidStatus, "The count's status does not allow this action")
	case errors.Is(err, wh.ErrCountStartApproval):
		response.Conflict(w, r, CodeCountStartApproval, "An initial placement count starts after its start approval")
	case errors.Is(err, wh.ErrCountUnitScanned):
		response.Conflict(w, r, CodeCountUnitScanned, "This unit is already counted; delete its scan to count it again")
	case errors.Is(err, wh.ErrCountStale):
		response.Conflict(w, r, CodeCountStale, strings.TrimPrefix(err.Error(), "warehouse: "))
	case errors.Is(err, wh.ErrCountLocationOutOfScope):
		unprocessable(CodeCountLocationOutScope)
	case errors.Is(err, wh.ErrCountProductOutOfScope):
		unprocessable(CodeCountProductOutScope)
	case errors.Is(err, wh.ErrCountScanKind):
		unprocessable(CodeCountScanKind)
	case errors.Is(err, wh.ErrCountLocationRequired):
		unprocessable(CodeCountLocationRequired)
	case errors.Is(err, wh.ErrCountAdjustForbidden):
		response.Error(w, r, http.StatusForbidden, CodeCountAdjustNotGranted, "Approving a stock count needs stock.adjust")
	default:
		writeError(w, r, err)
	}
}

type countBody struct {
	WarehouseUUID string  `json:"warehouse_uuid"`
	Method        string  `json:"method"`
	Visibility    string  `json:"visibility"`
	ScopeType     string  `json:"scope_type"`
	RoomUUID      *string `json:"room_uuid"`
	LocationUUID  *string `json:"location_uuid"`
	ProductUUID   *string `json:"product_uuid"`
	Note          *string `json:"note"`
}

type countScanBody struct {
	Code         string  `json:"code"`
	LocationUUID *string `json:"location_uuid"`
	Quantity     *int32  `json:"quantity"`
	Meters       *string `json:"meters"`
}

type countResolutionBody struct {
	LineUUID   string  `json:"line_uuid"`
	Resolution string  `json:"resolution"`
	Note       *string `json:"note"`
}

type countApproveBody struct {
	Resolutions []countResolutionBody `json:"resolutions"`
}

// List (GET /v1/warehouse/stock-counts?status&limit&offset).
func (h *Counts) List(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.List(r.Context(), countCaller(r), strings.TrimSpace(r.URL.Query().Get("status")), q.Limit, q.Offset)
	if err != nil {
		writeCountError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Create (POST /v1/warehouse/stock-counts).
func (h *Counts) Create(w http.ResponseWriter, r *http.Request) {
	var b countBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.Create(r.Context(), countCaller(r), wh.CountInput{
		WarehouseUUID: b.WarehouseUUID, Method: b.Method, Visibility: b.Visibility, ScopeType: b.ScopeType,
		RoomUUID: b.RoomUUID, LocationUUID: b.LocationUUID, ProductUUID: b.ProductUUID, Note: b.Note,
	})
	if err != nil {
		writeCountError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// Get (GET /v1/warehouse/stock-counts/{uuid}).
func (h *Counts) Get(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) { return h.svc.Get(r.Context(), countCaller(r), id) })
}

// ApproveStart (POST /v1/warehouse/stock-counts/{uuid}/approve-start).
func (h *Counts) ApproveStart(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) { return h.svc.ApproveStart(r.Context(), countCaller(r), id) })
}

// Start (POST /v1/warehouse/stock-counts/{uuid}/start).
func (h *Counts) Start(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) { return h.svc.Start(r.Context(), countCaller(r), id) })
}

// Complete (POST /v1/warehouse/stock-counts/{uuid}/complete).
func (h *Counts) Complete(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) { return h.svc.Complete(r.Context(), countCaller(r), id) })
}

// Cancel (POST /v1/warehouse/stock-counts/{uuid}/cancel).
func (h *Counts) Cancel(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) { return h.svc.Cancel(r.Context(), countCaller(r), id) })
}

// Report (GET /v1/warehouse/stock-counts/{uuid}/report).
func (h *Counts) Report(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) { return h.svc.Report(r.Context(), countCaller(r), id) })
}

// Scans (GET /v1/warehouse/stock-counts/{uuid}/scans).
func (h *Counts) Scans(w http.ResponseWriter, r *http.Request) {
	h.withID(w, r, func(id uuid.UUID) (any, error) {
		items, err := h.svc.Scans(r.Context(), countCaller(r), id)
		return listResponse[wh.CountScan]{Items: items}, err
	})
}

// Scan (POST /v1/warehouse/stock-counts/{uuid}/scans).
func (h *Counts) Scan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b countScanBody
	if !decode(w, r, &b) {
		return
	}
	v, err := h.svc.Scan(r.Context(), countCaller(r), id, wh.CountScanInput{
		Code: b.Code, LocationUUID: b.LocationUUID, Quantity: b.Quantity, Meters: b.Meters,
	})
	if err != nil {
		writeCountError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// DeleteScan (DELETE /v1/warehouse/stock-counts/{uuid}/scans/{scan_uuid}).
func (h *Counts) DeleteScan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	scanID, err := uuid.Parse(r.PathValue("scan_uuid"))
	if err != nil {
		response.Error(w, r, http.StatusNotFound, CodeCountScanNotFound, "Scan not found")
		return
	}
	if err := h.svc.DeleteScan(r.Context(), countCaller(r), id, scanID); err != nil {
		writeCountError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Approve (POST /v1/warehouse/stock-counts/{uuid}/approve).
func (h *Counts) Approve(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b countApproveBody
	if !decode(w, r, &b) {
		return
	}
	in := make([]wh.CountResolution, 0, len(b.Resolutions))
	for _, x := range b.Resolutions {
		in = append(in, wh.CountResolution{LineUUID: x.LineUUID, Resolution: x.Resolution, Note: x.Note})
	}
	v, err := h.svc.Approve(r.Context(), countCaller(r), id, in)
	if err != nil {
		writeCountError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Export (GET /v1/warehouse/stock-counts/{uuid}/export): the report lines
// as CSV.
func (h *Counts) Export(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	// Validate before the first byte so errors stay JSON.
	if _, err := h.svc.Report(r.Context(), countCaller(r), id); err != nil {
		writeCountError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="stock-count-`+id.String()+`.csv"`)
	w.WriteHeader(http.StatusOK)
	_ = h.svc.Export(r.Context(), countCaller(r), id, w)
}

func (h *Counts) withID(w http.ResponseWriter, r *http.Request, fn func(id uuid.UUID) (any, error)) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	v, err := fn(id)
	if err != nil {
		writeCountError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}
