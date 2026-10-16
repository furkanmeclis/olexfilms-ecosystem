package usecase

// TEC-761 (F2.3): stock exports of the warehouse screens (I/O engine,
// queue "exports"): the product stock of an organization
// (organization_product_stocks; the center reaches every organization of
// its brand), the product stock of one location, the movement history of
// one barcode and the barcode batch list (center only). Each job stores
// the caller's resolved stock.read scope; the worker rebuilds the viewer
// against the job organization (unitsJobViewer) and reads through the same
// use cases as the screens, so the reach and the owner masking of the
// barcode history apply again.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Export resources (export_jobs.resource).
const (
	ResourceProductStockExport  = "stock.products"
	ResourceLocationStockExport = "stock.location_stock"
	ResourceUnitHistoryExport   = "stock.unit_history"
	ResourceBatchesExport       = "stock.barcode_batches"
)

// Job query keys set by the server.
const (
	queryLocationUUID = "_location_uuid"
	queryBarcode      = "_barcode"
	queryBatchUUIDs   = "uuid"
)

const stockExportPage = 500

// StockExporter builds the stock export jobs and their datasets.
type StockExporter struct {
	svc      *Service
	barcodes *Barcodes
}

// NewStockExporter builds the exporter (SQL search, no index).
func NewStockExporter(pool TxBeginner, svc *Service) *StockExporter {
	return &StockExporter{svc: svc, barcodes: NewBarcodes(pool, svc.q)}
}

// ParseStockFilter reads the product stock list parameters (product_uuid,
// category_uuid, status, q, sort). Errors are *apiquery.ValidationError.
func ParseStockFilter(values url.Values) (model.StockFilter, error) {
	q := apiquery.Parse(values)
	f := model.StockFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	for name, dst := range map[string]**uuid.UUID{"product_uuid": &f.ProductUUID, "category_uuid": &f.CategoryUUID} {
		raw := strings.TrimSpace(values.Get(name))
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: name, Message: "is invalid", Code: "invalid"}}}
		}
		*dst = &id
	}
	switch status := strings.TrimSpace(values.Get("status")); status {
	case "", model.StatusInStock, model.StatusOutOfStock:
		f.Status = status
	default:
		return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
			Field: "status", Message: "must be in_stock or out_of_stock", Code: "invalid",
		}}}
	}
	sort, err := apiquery.ResolveSort(q.Sort, ProductStockSort)
	f.Sort = sort
	return f, err
}

func stockValues(m map[string]string) url.Values {
	v := url.Values{}
	for k, val := range m {
		if strings.HasPrefix(k, "_") || strings.TrimSpace(val) == "" {
			continue
		}
		v.Set(k, val)
	}
	return v
}

func scopeQuery(f scopefilter.Filter) (ioengine.ExportQuery, error) {
	scope, ok := ioengine.EncodeScopeFilter(f)
	if !ok {
		return nil, ErrNotFound
	}
	return ioengine.ExportQuery{ioengine.QueryScopeFilter: scope}, nil
}

// ProductStockExportQuery builds the job query of the product stock of
// organization orgUUID (inside the scope, like the list).
func (x *StockExporter) ProductStockExportQuery(ctx context.Context, f scopefilter.Filter, orgUUID uuid.UUID, body map[string]string) (ioengine.ExportQuery, error) {
	values := stockValues(body)
	if _, err := ParseStockFilter(values); err != nil {
		return nil, err
	}
	o, err := x.svc.q.GetOrganizationByUUID(ctx, orgUUID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (o.DeletedAt.Valid || !f.AllowsOrg(o.ID, o.BrandID))) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stock export: organization: %w", err)
	}
	out, err := scopeQuery(f)
	if err != nil {
		return nil, err
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	out[QueryListedOrganization] = orgUUID.String()
	return out, nil
}

// LocationStockExportQuery builds the job query of the product stock of
// location locUUID (its organization inside the scope).
func (x *StockExporter) LocationStockExportQuery(ctx context.Context, f scopefilter.Filter, locUUID uuid.UUID, body map[string]string) (ioengine.ExportQuery, error) {
	values := stockValues(body)
	if _, err := ParseStockFilter(values); err != nil {
		return nil, err
	}
	in := model.StockFilter{Limit: 1}
	if _, _, err := x.svc.LocationStock(ctx, f, locUUID, in); err != nil {
		return nil, err
	}
	out, err := scopeQuery(f)
	if err != nil {
		return nil, err
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	out[queryLocationUUID] = locUUID.String()
	return out, nil
}

// UnitHistoryExportQuery builds the job query of the movement history of
// barcode (the viewer must reach the unit, like the lookup).
func (x *StockExporter) UnitHistoryExportQuery(ctx context.Context, v UnitViewer, barcode string) (ioengine.ExportQuery, error) {
	if _, err := x.svc.UnitHistory(ctx, v.Org, v.Filter, barcode, 1, 0); err != nil {
		return nil, err
	}
	out, err := scopeQuery(v.Filter)
	if err != nil {
		return nil, err
	}
	out[queryBarcode] = strings.TrimSpace(barcode)
	return out, nil
}

// BatchesExportQuery builds the job query of the barcode batch list (center
// only, like the list).
func (x *StockExporter) BatchesExportQuery(c Caller, body map[string]string) (ioengine.ExportQuery, error) {
	if _, err := centerOrg(c); err != nil {
		return nil, err
	}
	values := stockValues(body)
	if _, err := ParseBatchListFilter(values); err != nil {
		return nil, err
	}
	if err := batchSelection(values, nil); err != nil {
		return nil, err
	}
	out, err := scopeQuery(c.Filter)
	if err != nil {
		return nil, err
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

func batchSelection(values url.Values, dst map[uuid.UUID]bool) error {
	for _, raw := range apiquery.CSVValues(values, queryBatchUUIDs) {
		id, err := uuid.Parse(raw)
		if err != nil {
			return &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: queryBatchUUIDs, Message: "must be a list of uuids", Code: "invalid",
			}}}
		}
		if dst != nil {
			dst[id] = true
		}
	}
	return nil
}

// Adapters returns the stock export adapters of the warehouse screens.
func (x *StockExporter) Adapters() []ioengine.ResourceAdapter {
	return []ioengine.ResourceAdapter{
		&stockExportAdapter{resource: ResourceProductStockExport, columns: productStockColumns, build: x.productStock, x: x},
		&stockExportAdapter{resource: ResourceLocationStockExport, columns: productStockColumns, build: x.locationStock, x: x},
		&stockExportAdapter{resource: ResourceUnitHistoryExport, columns: unitHistoryColumns, build: x.unitHistory, x: x},
		&stockExportAdapter{resource: ResourceBatchesExport, columns: batchColumns, build: x.batchList, x: x},
	}
}

type stockExportAdapter struct {
	resource string
	columns  []ioengine.Column
	build    func(ctx context.Context, v UnitViewer, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error)
	x        *StockExporter
}

func (a *stockExportAdapter) Resource() string                 { return a.resource }
func (a *stockExportAdapter) ExportColumns() []ioengine.Column { return a.columns }
func (a *stockExportAdapter) ImportSchema() []ioengine.ImportField {
	return nil
}
func (a *stockExportAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}
func (a *stockExportAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}

func (a *stockExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	v, err := a.x.svc.unitsJobViewer(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	ds, err := a.build(ctx, v, q, loc)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	ds.Resource, ds.Columns = a.resource, a.columns
	if ds.Rows == nil {
		ds.Rows = []map[string]any{}
	}
	return ds, nil
}

var productStockColumns = []ioengine.Column{
	{Key: "product", LabelKey: "stock.export.product", Type: ioengine.ColumnTypeString, Weight: 1.6},
	{Key: "sku", LabelKey: "stock.export.sku", Type: ioengine.ColumnTypeString},
	{Key: "category", LabelKey: "stock.export.category", Type: ioengine.ColumnTypeString},
	{Key: "quantity", LabelKey: "stock.export.quantity", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "meters", LabelKey: "stock.export.meters", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "fixed_barcodes", LabelKey: "stock.export.fixed_barcodes", Type: ioengine.ColumnTypeString, Weight: 1.4},
	{Key: "updated_at", LabelKey: "stock.export.updated_at", Type: ioengine.ColumnTypeDatetime},
}

var unitHistoryColumns = []ioengine.Column{
	{Key: "created_at", LabelKey: "stock.export.date", Type: ioengine.ColumnTypeDatetime},
	{Key: "type", LabelKey: "stock.export.movement_type", Type: ioengine.ColumnTypeString, Weight: 1.1},
	{Key: "quantity_delta", LabelKey: "stock.export.quantity_delta", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "meters_delta", LabelKey: "stock.export.meters_delta", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "from_status", LabelKey: "stock.export.from_status", Type: ioengine.ColumnTypeString, Weight: 0.8},
	{Key: "to_status", LabelKey: "stock.export.to_status", Type: ioengine.ColumnTypeString, Weight: 0.8},
	{Key: "from_owner", LabelKey: "stock.export.from_owner", Type: ioengine.ColumnTypeString, Weight: 1.3},
	{Key: "to_owner", LabelKey: "stock.export.to_owner", Type: ioengine.ColumnTypeString, Weight: 1.3},
	{Key: "organization", LabelKey: "stock.export.organization", Type: ioengine.ColumnTypeString, Weight: 1.1},
	{Key: "reason", LabelKey: "stock.export.reason", Type: ioengine.ColumnTypeString},
}

var batchColumns = []ioengine.Column{
	{Key: "created_at", LabelKey: "stock.export.date", Type: ioengine.ColumnTypeDatetime},
	{Key: "product", LabelKey: "stock.export.product", Type: ioengine.ColumnTypeString, Weight: 1.5},
	{Key: "sku", LabelKey: "stock.export.sku", Type: ioengine.ColumnTypeString},
	{Key: "prefix", LabelKey: "stock.export.prefix", Type: ioengine.ColumnTypeString, Weight: 0.7},
	{Key: "first_barcode", LabelKey: "stock.export.first_barcode", Type: ioengine.ColumnTypeString, Weight: 1.2},
	{Key: "last_barcode", LabelKey: "stock.export.last_barcode", Type: ioengine.ColumnTypeString, Weight: 1.2},
	{Key: "quantity", LabelKey: "stock.export.quantity", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "meters", LabelKey: "stock.export.meters", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "print_count", LabelKey: "stock.export.print_count", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "last_printed_at", LabelKey: "stock.export.last_printed_at", Type: ioengine.ColumnTypeDatetime},
}

func stockRows(items []model.ProductStock) []map[string]any {
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		fixed := make([]string, 0, len(it.FixedBarcodes))
		for _, b := range it.FixedBarcodes {
			fixed = append(fixed, b.Barcode+": "+strconv.Itoa(int(b.Quantity)))
		}
		rows = append(rows, map[string]any{
			"product":        it.Product.Name,
			"sku":            it.Product.SKU,
			"category":       it.Product.Category.Name,
			"quantity":       strconv.Itoa(int(it.Quantity)),
			"meters":         it.Meters,
			"fixed_barcodes": strings.Join(fixed, ", "),
			"updated_at":     it.UpdatedAt.UTC(),
		})
	}
	return rows
}

// pagedStock reads every page of a product stock list.
func pagedStock(in model.StockFilter, list func(model.StockFilter) ([]model.ProductStock, int64, error)) ([]map[string]any, error) {
	in.Limit, in.Offset = stockExportPage, 0
	var rows []map[string]any
	for {
		page, total, err := list(in)
		if err != nil {
			return nil, err
		}
		rows = append(rows, stockRows(page)...)
		in.Offset += int32(len(page)) //nolint:gosec // bounded by stockExportPage
		if len(page) == 0 || int64(in.Offset) >= total {
			return rows, nil
		}
	}
}

func (x *StockExporter) productStock(ctx context.Context, v UnitViewer, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgUUID, err := uuid.Parse(q[QueryListedOrganization])
	if err != nil {
		return ioengine.Dataset{}, errors.New("stock export: organization is required")
	}
	in, err := ParseStockFilter(stockValues(q))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := pagedStock(in, func(f model.StockFilter) ([]model.ProductStock, int64, error) {
		return x.svc.OrganizationStock(ctx, v.Filter, orgUUID, f)
	})
	if err != nil {
		return ioengine.Dataset{}, err
	}
	o, err := x.svc.q.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return ioengine.Dataset{}, fmt.Errorf("stock export: organization: %w", err)
	}
	return ioengine.Dataset{Rows: rows, Info: []ioengine.InfoLine{{LabelKey: "stock.export.organization", Value: o.Name}}}, nil
}

func (x *StockExporter) locationStock(ctx context.Context, v UnitViewer, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	locUUID, err := uuid.Parse(q[queryLocationUUID])
	if err != nil {
		return ioengine.Dataset{}, ErrNotFound
	}
	in, err := ParseStockFilter(stockValues(q))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := pagedStock(in, func(f model.StockFilter) ([]model.ProductStock, int64, error) {
		return x.svc.LocationStock(ctx, v.Filter, locUUID, f)
	})
	if err != nil {
		return ioengine.Dataset{}, err
	}
	loc, err := x.svc.q.GetWarehouseLocationByUUID(ctx, locUUID)
	if err != nil {
		return ioengine.Dataset{}, fmt.Errorf("stock export: location: %w", err)
	}
	code := loc.Code
	if loc.FullCode.Valid {
		code = loc.FullCode.String
	}
	return ioengine.Dataset{Rows: rows, Info: []ioengine.InfoLine{
		{LabelKey: "stock.export.location", Value: code + " · " + loc.Name},
	}}, nil
}

func (x *StockExporter) unitHistory(ctx context.Context, v UnitViewer, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	barcode := q[queryBarcode]
	var rows []map[string]any
	var head model.UnitHistory
	for offset := int32(0); ; offset += stockExportPage {
		h, err := x.svc.UnitHistory(ctx, v.Org, v.Filter, barcode, stockExportPage, offset)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		if offset == 0 {
			head = h
		}
		for _, m := range h.Movements.Items {
			rows = append(rows, movementRow(m, loc))
		}
		if len(h.Movements.Items) == 0 || int64(offset)+int64(len(h.Movements.Items)) >= h.Movements.Total {
			break
		}
	}
	holder := ""
	if head.Current != nil {
		holder = ownerLabel(head.Current.Owner, loc)
	}
	return ioengine.Dataset{Rows: rows, Info: []ioengine.InfoLine{
		{LabelKey: "stock.export.barcode", Value: head.Unit.Barcode},
		{LabelKey: "stock.export.product", Value: head.Unit.Product.SKU + " · " + head.Unit.Product.Name},
		{LabelKey: "stock.export.status", Value: statusLabel(head.Unit.Status, loc)},
		{LabelKey: "stock.export.holder", Value: holder},
	}}, nil
}

func movementRow(m model.Movement, loc i18n.Locale) map[string]any {
	org := ""
	switch {
	case m.OrganizationMasked:
		org = i18n.Translate(loc, "stock.export.masked")
	case m.Organization != nil:
		org = m.Organization.Name
	}
	status := func(p *string) string {
		if p == nil {
			return ""
		}
		return statusLabel(*p, loc)
	}
	owner := func(o *model.Owner) string {
		if o == nil {
			return ""
		}
		return ownerLabel(*o, loc)
	}
	reason := ""
	if m.Reason != nil {
		reason = *m.Reason
	}
	return map[string]any{
		"created_at":     m.CreatedAt.UTC(),
		"type":           translated(loc, "warehouse.eod.type.", m.Type),
		"quantity_delta": strconv.Itoa(int(m.QuantityDelta)),
		"meters_delta":   m.MetersDelta,
		"from_status":    status(m.FromStatus),
		"to_status":      status(m.ToStatus),
		"from_owner":     owner(m.FromOwner),
		"to_owner":       owner(m.ToOwner),
		"organization":   org,
		"reason":         reason,
	}
}

// ownerLabel names an owner: the organization (and location code), the
// service / trash owner, or "hidden" for a masked owner (TEC-94 masking).
func ownerLabel(o model.Owner, loc i18n.Locale) string {
	if o.Masked {
		return i18n.Translate(loc, "stock.export.masked")
	}
	var parts []string
	if o.Organization != nil {
		parts = append(parts, o.Organization.Name)
	}
	if o.Location != nil {
		parts = append(parts, o.Location.Code)
	}
	if o.Type == ownerService || o.Type == ownerTrash {
		parts = append([]string{translated(loc, "stock.export.owner.", o.Type)}, parts...)
	}
	return strings.Join(parts, " · ")
}

func statusLabel(s string, loc i18n.Locale) string { return translated(loc, "stock.export.status.", s) }

func translated(loc i18n.Locale, prefix, value string) string {
	if value == "" {
		return ""
	}
	if s := i18n.Translate(loc, prefix+value); s != prefix+value {
		return s
	}
	return value
}

func (x *StockExporter) batchList(ctx context.Context, v UnitViewer, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	values := stockValues(q)
	f, err := ParseBatchListFilter(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	sel := map[uuid.UUID]bool{}
	if err := batchSelection(values, sel); err != nil {
		return ioengine.Dataset{}, err
	}
	c := Caller{Org: v.Org, Filter: v.Filter}
	f.Limit, f.Offset = stockExportPage, 0
	var rows []map[string]any
	for {
		page, total, err := x.barcodes.List(ctx, c, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, b := range page {
			if len(sel) > 0 && !sel[b.UUID] {
				continue
			}
			var printed any
			if b.LastPrintedAt != nil {
				printed = b.LastPrintedAt.UTC()
			}
			meters := ""
			if b.Meters != nil {
				meters = *b.Meters
			}
			rows = append(rows, map[string]any{
				"created_at": b.CreatedAt.UTC(), "product": b.Product.Name, "sku": b.Product.SKU,
				"prefix": b.Prefix, "first_barcode": b.FirstBarcode, "last_barcode": b.LastBarcode,
				"quantity": strconv.Itoa(int(b.Quantity)), "meters": meters,
				"print_count": strconv.Itoa(int(b.PrintCount)), "last_printed_at": printed,
			})
		}
		f.Offset += int32(len(page)) //nolint:gosec // bounded by stockExportPage
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Rows: rows}, nil
}

var _ ioengine.ResourceAdapter = (*stockExportAdapter)(nil)
