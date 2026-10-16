package usecase

// TEC-761: stock entry and warehouse transfer exports. Both write one row
// per line with the document header repeated (a document without lines is
// one row with empty line cells). The transfer summary is the aggregate
// report of the old warehouse app: per product, the transfers and units
// shipped / delivered under the list filters plus a product filter.

import (
	"context"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var entryColumns = []ioengine.Column{
	{Key: "created_at", LabelKey: "warehouse.export.created_at", Type: ioengine.ColumnTypeDatetime},
	{Key: "warehouse", LabelKey: "warehouse.export.warehouse", Type: ioengine.ColumnTypeString, Weight: 1.2},
	{Key: "mode", LabelKey: "warehouse.export.mode", Type: ioengine.ColumnTypeString, Weight: 0.9},
	{Key: "status", LabelKey: "warehouse.export.status", Type: ioengine.ColumnTypeString, Weight: 0.8},
	{Key: "product", LabelKey: "warehouse.export.product", Type: ioengine.ColumnTypeString, Weight: 1.5},
	{Key: "sku", LabelKey: "warehouse.export.sku", Type: ioengine.ColumnTypeString},
	{Key: "barcode", LabelKey: "warehouse.export.barcode", Type: ioengine.ColumnTypeString, Weight: 1.2},
	{Key: "quantity", LabelKey: "warehouse.export.quantity", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "meters", LabelKey: "warehouse.export.meters", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "location", LabelKey: "warehouse.export.location", Type: ioengine.ColumnTypeString},
	{Key: "placed", LabelKey: "warehouse.export.placed", Type: ioengine.ColumnTypeBoolean, Weight: 0.6},
	{Key: "note", LabelKey: "warehouse.export.note", Type: ioengine.ColumnTypeString},
}

var transferColumns = []ioengine.Column{
	{Key: "transfer_no", LabelKey: "warehouse.export.transfer_no", Type: ioengine.ColumnTypeString},
	{Key: "created_at", LabelKey: "warehouse.export.created_at", Type: ioengine.ColumnTypeDatetime},
	{Key: "from_warehouse", LabelKey: "warehouse.export.from_warehouse", Type: ioengine.ColumnTypeString, Weight: 1.1},
	{Key: "to_warehouse", LabelKey: "warehouse.export.to_warehouse", Type: ioengine.ColumnTypeString, Weight: 1.1},
	{Key: "status", LabelKey: "warehouse.export.status", Type: ioengine.ColumnTypeString, Weight: 0.8},
	{Key: "product", LabelKey: "warehouse.export.product", Type: ioengine.ColumnTypeString, Weight: 1.5},
	{Key: "sku", LabelKey: "warehouse.export.sku", Type: ioengine.ColumnTypeString},
	{Key: "barcode", LabelKey: "warehouse.export.barcode", Type: ioengine.ColumnTypeString, Weight: 1.2},
	{Key: "source_location", LabelKey: "warehouse.export.source_location", Type: ioengine.ColumnTypeString},
	{Key: "target_location", LabelKey: "warehouse.export.target_location", Type: ioengine.ColumnTypeString},
	{Key: "line_status", LabelKey: "warehouse.export.line_status", Type: ioengine.ColumnTypeString, Weight: 0.9},
	{Key: "shipped_at", LabelKey: "warehouse.export.shipped_at", Type: ioengine.ColumnTypeDatetime},
	{Key: "completed_at", LabelKey: "warehouse.export.completed_at", Type: ioengine.ColumnTypeDatetime},
	{Key: "note", LabelKey: "warehouse.export.note", Type: ioengine.ColumnTypeString},
}

var transferSummaryColumns = []ioengine.Column{
	{Key: "product", LabelKey: "warehouse.export.product", Type: ioengine.ColumnTypeString, Weight: 1.6},
	{Key: "sku", LabelKey: "warehouse.export.sku", Type: ioengine.ColumnTypeString},
	{Key: "transfer_count", LabelKey: "warehouse.export.transfer_count", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "unit_count", LabelKey: "warehouse.export.unit_count", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "shipped", LabelKey: "warehouse.export.shipped", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "delivered", LabelKey: "warehouse.export.delivered", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "in_transit", LabelKey: "warehouse.export.in_transit", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "cancelled", LabelKey: "warehouse.export.cancelled", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
}

// Transfer line states of the export (derived from the line movements).
const (
	transferLinePending   = "pending"
	transferLineInTransit = "in_transit"
	transferLineReceived  = "received"
	transferLinePlaced    = "placed"
	transferLineRestored  = "restored"
)

func numStr(n pgtype.Numeric) string { return str(stockusecase.MetersString(n)) }

// entryList exports the stock entry list (filters, q, sort, selection),
// one row per line.
func (x *Exporter) entryList(ctx context.Context, c Caller, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	org, err := guard(c)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	values := exportValuesOf(q)
	f, err := ParseEntryListFilter(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Sort = orDefault(f.Sort, EntrySort)
	sel := selectedRows(values)
	warehouses, err := x.warehouseNames(ctx, org)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	cp := db.CountStockEntriesParams{
		OrganizationID: org, Statuses: f.Statuses, Modes: f.Modes, WarehouseUuids: f.WarehouseUUIDs,
		CreatedFrom: listTS(f.CreatedFrom), CreatedBefore: listTS(f.CreatedBefore), Q: listQ(f.Q),
	}
	var rows []map[string]any
	for offset := int32(0); ; offset += exportPage {
		page, err := x.q.ListStockEntries(ctx, db.ListStockEntriesParams{
			OrganizationID: cp.OrganizationID, Statuses: cp.Statuses, Modes: cp.Modes, WarehouseUuids: cp.WarehouseUuids,
			CreatedFrom: cp.CreatedFrom, CreatedBefore: cp.CreatedBefore, Q: cp.Q,
			SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, PageLimit: exportPage, PageOffset: offset,
		})
		if err != nil {
			return ioengine.Dataset{}, err
		}
		var entries []db.StockEntry
		var ids []int64
		for _, e := range page {
			if sel.has(e.Uuid) {
				entries = append(entries, e)
				ids = append(ids, e.ID)
			}
		}
		lines := map[int64][]db.ListStockEntryExportLinesRow{}
		if len(ids) > 0 {
			all, err := x.q.ListStockEntryExportLines(ctx, db.ListStockEntryExportLinesParams{EntryIds: ids, OrganizationID: org})
			if err != nil {
				return ioengine.Dataset{}, err
			}
			for _, l := range all {
				lines[l.EntryID] = append(lines[l.EntryID], l)
			}
		}
		for _, e := range entries {
			head := map[string]any{
				"created_at": whenTS(e.CreatedAt),
				"warehouse":  label(loc, "warehouse.export.no_warehouse"),
				"mode":       tr(loc, "warehouse.export.entry_mode.", e.Mode),
				"status":     tr(loc, "warehouse.export.entry_status.", e.Status),
				"note":       txt(e.Note),
			}
			if e.WarehouseID.Valid {
				head["warehouse"] = warehouses[e.WarehouseID.Int64]
			}
			if len(lines[e.ID]) == 0 {
				rows = append(rows, head)
				continue
			}
			for _, l := range lines[e.ID] {
				row := clone(head)
				row["product"], row["sku"], row["barcode"] = l.ProductName, l.ProductSku, l.Barcode
				row["quantity"] = itoa(int64(l.Quantity))
				row["meters"] = numStr(l.InitialMeters)
				row["location"] = locationCode(l.LocationFullCode, l.LocationCode)
				row["placed"] = l.LocationID.Valid
				rows = append(rows, row)
			}
		}
		if len(page) < exportPage {
			break
		}
	}
	return ioengine.Dataset{Rows: rows}, nil
}

// transferList exports the warehouse transfer list (filters, q, sort,
// selection), one row per line.
func (x *Exporter) transferList(ctx context.Context, c Caller, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	org, err := guard(c)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	values := exportValuesOf(q)
	f, err := ParseTransferListFilter(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Sort = orDefault(f.Sort, TransferSort)
	sel := selectedRows(values)
	warehouses, err := x.warehouseNames(ctx, org)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	var rows []map[string]any
	for offset := int32(0); ; offset += exportPage {
		page, err := x.q.ListWarehouseTransfers(ctx, db.ListWarehouseTransfersParams{
			OrganizationID: org, Statuses: f.Statuses,
			FromWarehouseUuids: f.FromWarehouseUUIDs, ToWarehouseUuids: f.ToWarehouseUUIDs,
			CreatedFrom: listTS(f.CreatedFrom), CreatedBefore: listTS(f.CreatedBefore), Q: listQ(f.Q),
			SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, PageLimit: exportPage, PageOffset: offset,
		})
		if err != nil {
			return ioengine.Dataset{}, err
		}
		var docs []db.WarehouseTransfer
		var ids []int64
		for _, t := range page {
			if sel.has(t.Uuid) {
				docs = append(docs, t)
				ids = append(ids, t.ID)
			}
		}
		lines := map[int64][]db.ListWarehouseTransferExportLinesRow{}
		if len(ids) > 0 {
			all, err := x.q.ListWarehouseTransferExportLines(ctx, db.ListWarehouseTransferExportLinesParams{TransferIds: ids, OrganizationID: org})
			if err != nil {
				return ioengine.Dataset{}, err
			}
			for _, l := range all {
				lines[l.TransferID] = append(lines[l.TransferID], l)
			}
		}
		for _, t := range docs {
			head := map[string]any{
				"transfer_no":    t.TransferNo,
				"created_at":     whenTS(t.CreatedAt),
				"from_warehouse": warehouses[t.FromWarehouseID],
				"to_warehouse":   warehouses[t.ToWarehouseID],
				"status":         tr(loc, "warehouse.export.transfer_status.", t.Status),
				"shipped_at":     whenTS(t.ShippedAt),
				"completed_at":   whenTS(t.CompletedAt),
				"note":           txt(t.Note),
			}
			if len(lines[t.ID]) == 0 {
				rows = append(rows, head)
				continue
			}
			for _, l := range lines[t.ID] {
				row := clone(head)
				row["product"], row["sku"], row["barcode"] = l.ProductName, l.ProductSku, l.Barcode
				row["source_location"] = locationCode(l.SourceFullCode, l.SourceCode)
				row["target_location"] = locationCode(l.TargetFullCode, l.TargetCode)
				row["line_status"] = tr(loc, "warehouse.export.transfer_line.", transferLineState(l))
				rows = append(rows, row)
			}
		}
		if len(page) < exportPage {
			break
		}
	}
	return ioengine.Dataset{Rows: rows}, nil
}

func transferLineState(l db.ListWarehouseTransferExportLinesRow) string {
	switch {
	case l.Restored:
		return transferLineRestored
	case l.Placed:
		return transferLinePlaced
	case l.Received:
		return transferLineReceived
	case l.Shipped:
		return transferLineInTransit
	default:
		return transferLinePending
	}
}

// transferSummary exports the transfer aggregate report: the list filters
// (status, from / to warehouse, created range, q), product_uuid and the
// selection (uuid) narrow the transfers; one row per product.
func (x *Exporter) transferSummary(ctx context.Context, c Caller, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	org, err := guard(c)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	values := exportValuesOf(q)
	f, err := ParseTransferListFilter(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	products, err := uuidList(values, "product_uuid")
	if err != nil {
		return ioengine.Dataset{}, err
	}
	selected, _ := uuidList(values, QueryExportUUIDs)
	sum, err := x.q.WarehouseTransferProductSummary(ctx, db.WarehouseTransferProductSummaryParams{
		OrganizationID: org, Statuses: f.Statuses,
		FromWarehouseUuids: f.FromWarehouseUUIDs, ToWarehouseUuids: f.ToWarehouseUUIDs,
		ProductUuids: products, TransferUuids: selected,
		CreatedFrom: listTS(f.CreatedFrom), CreatedBefore: listTS(f.CreatedBefore), Q: listQ(f.Q),
	})
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(sum))
	var totals [6]int64
	for _, s := range sum {
		rows = append(rows, map[string]any{
			"product": s.ProductName, "sku": s.ProductSku,
			"transfer_count": itoa(s.TransferCount), "unit_count": itoa(s.UnitCount),
			"shipped": itoa(s.ShippedCount), "delivered": itoa(s.DeliveredCount),
			"in_transit": itoa(s.InTransitCount), "cancelled": itoa(s.CancelledCount),
		})
		for i, n := range []int64{s.TransferCount, s.UnitCount, s.ShippedCount, s.DeliveredCount, s.InTransitCount, s.CancelledCount} {
			totals[i] += n
		}
	}
	info, err := x.transferSummaryInfo(ctx, org, f, products, values, loc)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	ds := ioengine.Dataset{Rows: rows, Info: info}
	if len(rows) > 0 {
		// transfer_count is not additive (a transfer holds several products).
		ds.Totals = map[string]any{
			"product":    label(loc, "warehouse.export.total"),
			"unit_count": itoa(totals[1]), "shipped": itoa(totals[2]), "delivered": itoa(totals[3]),
			"in_transit": itoa(totals[4]), "cancelled": itoa(totals[5]),
		}
	}
	return ds, nil
}

func (x *Exporter) transferSummaryInfo(ctx context.Context, org int64, f TransferListFilter, products []uuid.UUID,
	values map[string][]string, loc i18n.Locale) ([]ioengine.InfoLine, error) {
	names := func(ids []uuid.UUID) (string, error) {
		if len(ids) == 0 {
			return label(loc, "warehouse.export.all"), nil
		}
		ws, err := x.q.ListWarehousesByUUIDs(ctx, db.ListWarehousesByUUIDsParams{Uuids: ids, OrganizationID: org})
		if err != nil {
			return "", err
		}
		out := make([]string, 0, len(ws))
		for _, w := range ws {
			out = append(out, warehouseLabel(w.Code, w.Name))
		}
		return strings.Join(out, ", "), nil
	}
	from, err := names(f.FromWarehouseUUIDs)
	if err != nil {
		return nil, err
	}
	to, err := names(f.ToWarehouseUUIDs)
	if err != nil {
		return nil, err
	}
	productNames := label(loc, "warehouse.export.all")
	if len(products) > 0 {
		ps, err := x.q.ListProductsByUUIDsAnyBrand(ctx, products)
		if err != nil {
			return nil, err
		}
		parts := make([]string, 0, len(ps))
		for _, p := range ps {
			parts = append(parts, warehouseLabel(p.Sku, p.Name))
		}
		productNames = strings.Join(parts, ", ")
	}
	statuses := make([]string, 0, len(f.Statuses))
	for _, s := range f.Statuses {
		statuses = append(statuses, tr(loc, "warehouse.export.transfer_status.", s))
	}
	period := dateRange(values)
	if period == "" {
		period = label(loc, "warehouse.export.all")
	}
	status := label(loc, "warehouse.export.all")
	if len(statuses) > 0 {
		status = strings.Join(statuses, ", ")
	}
	return []ioengine.InfoLine{
		{LabelKey: "warehouse.export.period", Value: period},
		{LabelKey: "warehouse.export.from_warehouse", Value: from},
		{LabelKey: "warehouse.export.to_warehouse", Value: to},
		{LabelKey: "warehouse.export.products", Value: productNames},
		{LabelKey: "warehouse.export.status", Value: status},
	}, nil
}

// warehouseNames maps the organization's warehouse ids to "CODE · Name".
func (x *Exporter) warehouseNames(ctx context.Context, org int64) (map[int64]string, error) {
	ws, err := x.q.ListWarehouses(ctx, db.ListWarehousesParams{OrganizationID: org})
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(ws))
	for _, w := range ws {
		out[w.ID] = warehouseLabel(w.Code, w.Name)
	}
	return out, nil
}

func locationCode(full, code pgtype.Text) string {
	if s := txt(full); s != "" {
		return s
	}
	return txt(code)
}

func clone(m map[string]any) map[string]any {
	out := make(map[string]any, len(m)+8)
	for k, v := range m {
		out[k] = v
	}
	return out
}
