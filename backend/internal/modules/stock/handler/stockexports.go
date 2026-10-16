package handler

// TEC-761: stock exports of the warehouse screens (I/O engine). Same
// permission and scope as the lists they export (stock.read); the job
// stores the resolved scope and the worker re-authorizes it. Poll and
// download through /v1/tenant/exports/{uuid}.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// StockExports serves the stock export endpoints of the warehouse screens.
type StockExports struct {
	x       *stockusecase.StockExporter
	exports Exports
}

// NewStockExports builds the handler; without exports the endpoints
// answer 503.
func NewStockExports(x *stockusecase.StockExporter, exports Exports) *StockExports {
	return &StockExports{x: x, exports: exports}
}

// ProductStock (POST /v1/stock/organizations/{uuid}/products/export, 202):
// the product stock of an organization with the list filters.
func (h *StockExports) ProductStock(w http.ResponseWriter, r *http.Request) {
	h.withPath(w, r, stockusecase.ResourceProductStockExport, func(b unitsExportBody) (ioengine.ExportQuery, error) {
		id, err := uuid.Parse(r.PathValue("uuid"))
		if err != nil {
			return nil, stockusecase.ErrNotFound
		}
		f, _ := scopefilter.From(r.Context())
		return h.x.ProductStockExportQuery(r.Context(), f, id, b.Query)
	})
}

// LocationStock (POST /v1/stock/locations/{uuid}/products/export, 202):
// the product stock of one location.
func (h *StockExports) LocationStock(w http.ResponseWriter, r *http.Request) {
	h.withPath(w, r, stockusecase.ResourceLocationStockExport, func(b unitsExportBody) (ioengine.ExportQuery, error) {
		id, err := uuid.Parse(r.PathValue("uuid"))
		if err != nil {
			return nil, stockusecase.ErrNotFound
		}
		f, _ := scopefilter.From(r.Context())
		return h.x.LocationStockExportQuery(r.Context(), f, id, b.Query)
	})
}

// UnitHistory (POST /v1/stock/units/by-barcode/{barcode}/export, 202): the
// movement history of one barcode, masked like the lookup.
func (h *StockExports) UnitHistory(w http.ResponseWriter, r *http.Request) {
	h.withPath(w, r, stockusecase.ResourceUnitHistoryExport, func(unitsExportBody) (ioengine.ExportQuery, error) {
		f, _ := scopefilter.From(r.Context())
		v := stockusecase.UnitViewer{Org: orgctx.MustScope(r.Context()), Filter: f}
		return h.x.UnitHistoryExportQuery(r.Context(), v, r.PathValue("barcode"))
	})
}

// Batches (POST /v1/stock/barcodes/export, 202): the barcode batch list
// (center only) with its filters and selection.
func (h *StockExports) Batches(w http.ResponseWriter, r *http.Request) {
	h.withPath(w, r, stockusecase.ResourceBatchesExport, func(b unitsExportBody) (ioengine.ExportQuery, error) {
		return h.x.BatchesExportQuery(reclassCaller(r), b.Query)
	})
}

func (h *StockExports) withPath(w http.ResponseWriter, r *http.Request, resource string,
	build func(unitsExportBody) (ioengine.ExportQuery, error)) {
	if h.exports == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "exports are not configured")
		return
	}
	if _, ok := filterFrom(w, r); !ok {
		return
	}
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		response.Unauthorized(w, r, "Authentication is required")
		return
	}
	var b unitsExportBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(b.Format)))
	switch format {
	case ioengine.ExportCSV, ioengine.ExportXLSX, ioengine.ExportPDF:
	default:
		response.ValidationError(w, r, []response.Detail{{Field: "format", Message: "must be csv, xlsx or pdf"}})
		return
	}
	query, err := build(b)
	if errors.Is(err, stockusecase.ErrBarcodesCenterOnly) {
		writeLabelError(w, r, err)
		return
	}
	if err != nil {
		writeQueryError(w, r, err)
		return
	}
	locale := string(i18n.FromContext(r.Context()).Locale)
	if l, ok := i18n.Parse(b.Locale); ok {
		locale = string(l)
	}
	orgID := orgctx.MustScope(r.Context()).InternalID
	job, err := h.exports.RequestExport(r.Context(), p.UserInternal, &orgID, resource, format, query, locale)
	if errors.Is(err, exportusecase.ErrInvalidRequest) {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, job)
}
