package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Barcode and label error codes (TEC-202); frontend/src/locales/*/errors.json
// translates them.
const (
	CodeBarcodesCenterOnly   = "STOCK_BARCODES_CENTER_ONLY"
	CodeProductInactive      = "STOCK_PRODUCT_INACTIVE"
	CodeLabelTemplateTaken   = "STOCK_LABEL_TEMPLATE_NAME_TAKEN"
	CodeLabelRendererOffline = "STOCK_LABEL_RENDERER_UNAVAILABLE"
)

// Labels serves the barcode batch, label template and label print routes.
type Labels struct {
	batches   *stockusecase.Barcodes
	templates *stockusecase.LabelTemplates
	printer   *stockusecase.Labels
}

// NewLabels creates the handler.
func NewLabels(b *stockusecase.Barcodes, t *stockusecase.LabelTemplates, p *stockusecase.Labels) *Labels {
	return &Labels{batches: b, templates: t, printer: p}
}

func writeLabelError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *stockusecase.ValidationError
	var qe *apiquery.ValidationError
	switch {
	case errors.As(err, &qe):
		// TEC-375: list query parameters.
		details := make([]response.Detail, 0, len(qe.Details))
		for _, d := range qe.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
	case errors.As(err, &ve):
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message}})
	case errors.Is(err, stockusecase.ErrBarcodesCenterOnly):
		response.Error(w, r, http.StatusForbidden, CodeBarcodesCenterOnly, "Only the center generates barcodes")
	case errors.Is(err, stockusecase.ErrLabelsForbidden):
		response.Forbidden(w, r, "Labels are available to the center and distributors only")
	case errors.Is(err, stockusecase.ErrNotFound):
		response.NotFound(w, r, "Resource not found")
	case errors.Is(err, stockusecase.ErrProductInactive):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeProductInactive, "The product is inactive")
	case errors.Is(err, stockusecase.ErrTemplateNameTaken):
		response.Conflict(w, r, CodeLabelTemplateTaken, "A label template with this name already exists")
	case errors.Is(err, stockusecase.ErrRendererUnavailable):
		response.Error(w, r, http.StatusServiceUnavailable, CodeLabelRendererOffline, "The label PDF renderer is unavailable")
	default:
		response.InternalErr(w, r, err, "label request failed")
	}
}

func writePDF(w http.ResponseWriter, name string, pdf []byte) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

func labelUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Resource not found")
		return uuid.Nil, false
	}
	return id, true
}

// optionalUUID parses the query parameter when present.
func optionalUUID(w http.ResponseWriter, r *http.Request, key string) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: key, Message: "must be a uuid"}})
		return nil, false
	}
	return &id, true
}

// --- barcode batches -------------------------------------------------------

type batchBody struct {
	ProductUUID  string `json:"product_uuid"`
	Quantity     int    `json:"quantity"`
	Meters       string `json:"meters"`
	Prefix       string `json:"prefix"`
	TemplateUUID string `json:"template_uuid"`
}

// CreateBatch serves POST /v1/stock/barcodes.
func (h *Labels) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var body batchBody
	if !decodeOptional(w, r, &body) {
		return
	}
	out, err := h.batches.Create(r.Context(), reclassCaller(r), stockusecase.BatchInput{
		ProductUUID: body.ProductUUID, Quantity: body.Quantity, Meters: body.Meters,
		Prefix: body.Prefix, TemplateUUID: body.TemplateUUID,
	})
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// ListBatches serves GET /v1/stock/barcodes
// (?product_uuid&printed&created_from&created_to&q&sort&limit&offset).
func (h *Labels) ListBatches(w http.ResponseWriter, r *http.Request) {
	f, err := stockusecase.ParseBatchListFilter(r.URL.Query())
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	items, total, err := h.batches.List(r.Context(), reclassCaller(r), f)
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// GetBatch serves GET /v1/stock/barcodes/{uuid}.
func (h *Labels) GetBatch(w http.ResponseWriter, r *http.Request) {
	id, ok := labelUUID(w, r)
	if !ok {
		return
	}
	out, err := h.batches.Get(r.Context(), reclassCaller(r), id)
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PrintBatch serves GET /v1/stock/barcodes/{uuid}/labels.pdf.
func (h *Labels) PrintBatch(w http.ResponseWriter, r *http.Request) {
	id, ok := labelUUID(w, r)
	if !ok {
		return
	}
	tpl, ok := optionalUUID(w, r, "template")
	if !ok {
		return
	}
	pdf, err := h.printer.PrintBatch(r.Context(), reclassCaller(r), id, tpl)
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	writePDF(w, "labels-"+id.String()+".pdf", pdf)
}

// --- label templates -------------------------------------------------------

type templateBody struct {
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	Symbology    string  `json:"symbology"`
	LogoMode     string  `json:"logo_mode"`
	LogoText     string  `json:"logo_text"`
	LogoImage    string  `json:"logo_image"`
	WidthMm      *string `json:"width_mm"`
	HeightMm     *string `json:"height_mm"`
	Columns      *int    `json:"columns"`
	ShowName     *bool   `json:"show_name"`
	ShowCodeText *bool   `json:"show_code_text"`
	IsDefault    bool    `json:"is_default"`
	Active       *bool   `json:"active"`
}

func (b templateBody) input() stockusecase.TemplateInput {
	return stockusecase.TemplateInput{
		Name: b.Name, Kind: b.Kind, Symbology: b.Symbology, LogoMode: b.LogoMode,
		LogoText: b.LogoText, LogoImage: b.LogoImage, WidthMm: b.WidthMm, HeightMm: b.HeightMm,
		Columns: b.Columns, ShowName: b.ShowName, ShowCodeText: b.ShowCodeText,
		IsDefault: b.IsDefault, Active: b.Active,
	}
}

// ListTemplates serves GET /v1/stock/label-templates.
func (h *Labels) ListTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := h.templates.List(r.Context(), reclassCaller(r), r.URL.Query().Get("kind"))
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, struct {
		Items []model.LabelTemplate `json:"items"`
	}{Items: items})
}

// CreateTemplate serves POST /v1/stock/label-templates.
func (h *Labels) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var body templateBody
	if !decodeOptional(w, r, &body) {
		return
	}
	out, err := h.templates.Create(r.Context(), reclassCaller(r), body.input())
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// GetTemplate serves GET /v1/stock/label-templates/{uuid}.
func (h *Labels) GetTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := labelUUID(w, r)
	if !ok {
		return
	}
	out, err := h.templates.Get(r.Context(), reclassCaller(r), id)
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UpdateTemplate serves PUT /v1/stock/label-templates/{uuid}.
func (h *Labels) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := labelUUID(w, r)
	if !ok {
		return
	}
	var body templateBody
	if !decodeOptional(w, r, &body) {
		return
	}
	out, err := h.templates.Update(r.Context(), reclassCaller(r), id, body.input())
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteTemplate serves DELETE /v1/stock/label-templates/{uuid}.
func (h *Labels) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := labelUUID(w, r)
	if !ok {
		return
	}
	if err := h.templates.Delete(r.Context(), reclassCaller(r), id); err != nil {
		writeLabelError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- label printing --------------------------------------------------------

// PrintUnits serves GET /v1/stock/labels/units.pdf?barcode=...&barcode=...
func (h *Labels) PrintUnits(w http.ResponseWriter, r *http.Request) {
	tpl, ok := optionalUUID(w, r, "template")
	if !ok {
		return
	}
	pdf, err := h.printer.PrintUnits(r.Context(), reclassCaller(r), r.URL.Query()["barcode"], tpl)
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	writePDF(w, "unit-labels.pdf", pdf)
}

// PrintLocations serves GET /v1/warehouse/labels/locations.pdf?location=...&room=...
func (h *Labels) PrintLocations(w http.ResponseWriter, r *http.Request) {
	tpl, ok := optionalUUID(w, r, "template")
	if !ok {
		return
	}
	room, ok := optionalUUID(w, r, "room")
	if !ok {
		return
	}
	var ids []uuid.UUID
	for _, raw := range r.URL.Query()["location"] {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "location", Message: "must be a uuid"}})
			return
		}
		ids = append(ids, id)
	}
	pdf, err := h.printer.PrintLocations(r.Context(), reclassCaller(r), stockusecase.LocationPrintInput{
		LocationUUIDs: ids, RoomUUID: room, TemplateUUID: tpl,
	})
	if err != nil {
		writeLabelError(w, r, err)
		return
	}
	writePDF(w, "location-labels.pdf", pdf)
}
