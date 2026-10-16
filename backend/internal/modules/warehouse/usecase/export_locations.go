package usecase

// TEC-761: location and end-of-day report list exports. The location
// export writes every typed location (aisle, shelf, bin) of the active
// organization with its warehouse / room / aisle / shelf / bin codes, the
// full code, the QR payload printed on its label (OFW:LOC:<full_code>,
// TEC-202) and its active flag.

import (
	"context"
	"net/url"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// LocationQRPrefix is the location QR payload prefix (TEC-202).
const LocationQRPrefix = "OFW:LOC:"

var locationColumns = []ioengine.Column{
	{Key: "warehouse", LabelKey: "warehouse.export.warehouse", Type: ioengine.ColumnTypeString, Weight: 1.2},
	{Key: "room", LabelKey: "warehouse.export.room", Type: ioengine.ColumnTypeString},
	{Key: "aisle", LabelKey: "warehouse.export.aisle", Type: ioengine.ColumnTypeString, Weight: 0.7},
	{Key: "shelf", LabelKey: "warehouse.export.shelf", Type: ioengine.ColumnTypeString, Weight: 0.7},
	{Key: "bin", LabelKey: "warehouse.export.bin", Type: ioengine.ColumnTypeString, Weight: 0.7},
	{Key: "type", LabelKey: "warehouse.export.location_type", Type: ioengine.ColumnTypeString, Weight: 0.7},
	{Key: "name", LabelKey: "warehouse.export.name", Type: ioengine.ColumnTypeString},
	{Key: "full_code", LabelKey: "warehouse.export.full_code", Type: ioengine.ColumnTypeString, Weight: 1.2},
	{Key: "qr", LabelKey: "warehouse.export.qr", Type: ioengine.ColumnTypeString, Weight: 1.4},
	{Key: "active", LabelKey: "warehouse.export.active", Type: ioengine.ColumnTypeBoolean, Weight: 0.6},
}

var eodListColumns = []ioengine.Column{
	{Key: "report_date", LabelKey: "warehouse.export.report_date", Type: ioengine.ColumnTypeString, Weight: 0.8},
	{Key: "scope", LabelKey: "warehouse.export.scope", Type: ioengine.ColumnTypeString, Weight: 1.3},
	{Key: "kind", LabelKey: "warehouse.export.kind", Type: ioengine.ColumnTypeString, Weight: 0.8},
	{Key: "movements", LabelKey: "warehouse.eod.movements", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "units", LabelKey: "warehouse.eod.units", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "quantity_in", LabelKey: "warehouse.eod.quantity_in", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "quantity_out", LabelKey: "warehouse.eod.quantity_out", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "meters_in", LabelKey: "warehouse.eod.meters_in", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "meters_out", LabelKey: "warehouse.eod.meters_out", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "generated_at", LabelKey: "warehouse.eod.generated_at", Type: ioengine.ColumnTypeDatetime},
}

// locationExportFilter narrows the location export.
type locationExportFilter struct {
	WarehouseUUIDs []uuid.UUID
	RoomUUIDs      []uuid.UUID
	Types          []string
	Active         *bool
	Q              string
}

// parseLocationExportFilter reads warehouse_uuid, room_uuid, type (CSV),
// active and q (code, name or full code).
func parseLocationExportFilter(values url.Values) (locationExportFilter, error) {
	f := locationExportFilter{Q: strings.ToLower(strings.TrimSpace(values.Get("q")))}
	var err error
	if f.WarehouseUUIDs, err = uuidList(values, "warehouse_uuid"); err != nil {
		return f, err
	}
	if f.RoomUUIDs, err = uuidList(values, "room_uuid"); err != nil {
		return f, err
	}
	if f.Types, err = apiquery.EnumList(values, "type", TypeAisle, TypeShelf, TypeBin); err != nil {
		return f, err
	}
	f.Active, err = apiquery.Bool(values, "active")
	return f, err
}

func (f locationExportFilter) match(l db.ListOrganizationLocationsForExportRow) bool {
	if len(f.WarehouseUUIDs) > 0 && !containsUUID(f.WarehouseUUIDs, l.WarehouseUuid) {
		return false
	}
	if len(f.RoomUUIDs) > 0 && !containsUUID(f.RoomUUIDs, l.RoomUuid) {
		return false
	}
	if len(f.Types) > 0 && !containsString(f.Types, txt(l.Type)) {
		return false
	}
	if f.Active != nil && l.Active != *f.Active {
		return false
	}
	if f.Q != "" {
		hay := strings.ToLower(l.Code + " " + l.Name + " " + txt(l.FullCode))
		if !strings.Contains(hay, f.Q) {
			return false
		}
	}
	return true
}

// locationList exports the organization's typed locations.
func (x *Exporter) locationList(ctx context.Context, c Caller, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	org, err := guard(c)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	values := exportValuesOf(q)
	f, err := parseLocationExportFilter(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	sel := selectedRows(values)
	all, err := x.q.ListOrganizationLocationsForExport(ctx, org)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	byID := make(map[int64]db.ListOrganizationLocationsForExportRow, len(all))
	for _, l := range all {
		byID[l.ID] = l
	}
	rows := make([]map[string]any, 0, len(all))
	for _, l := range all {
		if !sel.has(l.Uuid) || !f.match(l) {
			continue
		}
		codes := map[string]string{}
		// The location and its parents (bin -> shelf -> aisle).
		for cur, ok := l, true; ok; cur, ok = byID[cur.ParentID.Int64] {
			if t := txt(cur.Type); t != "" {
				codes[t] = cur.Code
			}
			if !cur.ParentID.Valid {
				break
			}
		}
		full := txt(l.FullCode)
		qr := ""
		if full != "" {
			qr = LocationQRPrefix + full
		}
		rows = append(rows, map[string]any{
			"warehouse": warehouseLabel(l.WarehouseCode, l.WarehouseName),
			"room":      warehouseLabel(l.RoomCode, l.RoomName),
			"aisle":     codes[TypeAisle],
			"shelf":     codes[TypeShelf],
			"bin":       codes[TypeBin],
			"type":      tr(loc, "warehouse.export.location_type.", txt(l.Type)),
			"name":      l.Name,
			"full_code": full,
			"qr":        qr,
			"active":    l.Active,
		})
	}
	return ioengine.Dataset{Rows: rows}, nil
}

// eodList exports the end-of-day report list (filters, sort, selection).
func (x *Exporter) eodList(ctx context.Context, c Caller, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	values := exportValuesOf(q)
	in, err := ParseEODListInput(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	sel := selectedRows(values)
	var rows []map[string]any
	in.Limit, in.Offset = exportPage, 0
	for {
		page, total, err := x.eod.List(ctx, c, in)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, r := range page {
			if !sel.has(r.UUID) {
				continue
			}
			t := r.Summary.Totals
			rows = append(rows, map[string]any{
				"report_date": r.ReportDate,
				"scope":       eodScopeLabel(r, loc),
				"kind":        tr(loc, "warehouse.eod.kind.", r.Kind),
				"movements":   itoa(t.MovementCount),
				"units":       itoa(t.UnitCount),
				"quantity_in": itoa(t.QuantityIn), "quantity_out": itoa(t.QuantityOut),
				"meters_in": t.MetersIn, "meters_out": t.MetersOut,
				"generated_at": when(&r.GeneratedAt),
			})
		}
		in.Offset += int32(len(page)) //nolint:gosec // bounded by exportPage
		if len(page) == 0 || int64(in.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Rows: rows}, nil
}

func containsUUID(list []uuid.UUID, id uuid.UUID) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
