package handler

// TEC-1007: container entry endpoints (delivery header, arrival scan,
// suppliers, saved import mappings).
import (
	"errors"
	"net/http"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

type entryHeaderBody struct {
	SupplierUUID        *string `json:"supplier_uuid"`
	ContainerNo         *string `json:"container_no"`
	WaybillNo           *string `json:"waybill_no"`
	ArrivalDate         *string `json:"arrival_date"`
	DefaultLocationUUID *string `json:"default_location_uuid"`
	Note                *string `json:"note"`
}

func (b entryHeaderBody) input() wh.EntryHeaderInput {
	return wh.EntryHeaderInput{
		SupplierUUID: b.SupplierUUID, ContainerNo: b.ContainerNo, WaybillNo: b.WaybillNo,
		ArrivalDate: b.ArrivalDate, DefaultLocationUUID: b.DefaultLocationUUID, Note: b.Note,
	}
}

func writeContainerError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, wh.ErrSupplierNotFound):
		response.NotFound(w, r, "Supplier not found")
	case errors.Is(err, wh.ErrMappingTemplateNotFound):
		response.NotFound(w, r, "Mapping template not found")
	default:
		writeEntryError(w, r, err)
	}
}

// UpdateHeader (PATCH /v1/warehouse/stock-entries/{uuid}).
func (h *Entries) UpdateHeader(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b entryHeaderBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.UpdateHeader(r.Context(), entryCaller(r), id, b.input())
	if err != nil {
		writeContainerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

type arrivalScanBody struct {
	Barcode string `json:"barcode"`
}

// Scan (POST /v1/warehouse/stock-entries/{uuid}/arrival-scans).
func (h *Entries) Scan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var b arrivalScanBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.Scan(r.Context(), entryCaller(r), id, b.Barcode)
	if err != nil {
		writeContainerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Unscan (DELETE /v1/warehouse/stock-entries/{uuid}/arrival-scans?barcode=).
func (h *Entries) Unscan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Unscan(r.Context(), entryCaller(r), id, r.URL.Query().Get("barcode"))
	if err != nil {
		writeContainerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ArrivalReport (GET /v1/warehouse/stock-entries/{uuid}/arrival-report).
func (h *Entries) ArrivalReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Report(r.Context(), entryCaller(r), id)
	if err != nil {
		writeContainerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Suppliers (GET /v1/warehouse/suppliers?q&limit&offset).
func (h *Entries) Suppliers(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Suppliers(r.Context(), entryCaller(r), q.Q, q.Limit, q.Offset)
	if err != nil {
		writeContainerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

type supplierBody struct {
	Name  string  `json:"name"`
	TaxNo *string `json:"tax_no"`
}

// CreateSupplier (POST /v1/warehouse/suppliers).
func (h *Entries) CreateSupplier(w http.ResponseWriter, r *http.Request) {
	var b supplierBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.CreateSupplier(r.Context(), entryCaller(r), wh.SupplierInput{Name: b.Name, TaxNo: b.TaxNo})
	if err != nil {
		writeContainerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// MappingTemplates (GET /v1/warehouse/entry-import-templates).
func (h *Entries) MappingTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.MappingTemplates(r.Context(), entryCaller(r))
	if err != nil {
		writeContainerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.FromSlice(items, int32(len(items)), 0))
}

type mappingTemplateBody struct {
	Name         string            `json:"name"`
	SupplierUUID *string           `json:"supplier_uuid"`
	Mapping      map[string]string `json:"mapping"`
}

// SaveMappingTemplate (POST /v1/warehouse/entry-import-templates).
func (h *Entries) SaveMappingTemplate(w http.ResponseWriter, r *http.Request) {
	var b mappingTemplateBody
	if !decode(w, r, &b) {
		return
	}
	out, err := h.svc.SaveMappingTemplate(r.Context(), entryCaller(r), wh.MappingTemplateInput{Name: b.Name, SupplierUUID: b.SupplierUUID, Mapping: b.Mapping})
	if err != nil {
		writeContainerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteMappingTemplate (DELETE /v1/warehouse/entry-import-templates/{uuid}).
func (h *Entries) DeleteMappingTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteMappingTemplate(r.Context(), entryCaller(r), id); err != nil {
		writeContainerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
