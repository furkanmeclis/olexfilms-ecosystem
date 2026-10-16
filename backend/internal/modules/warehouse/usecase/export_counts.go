package usecase

// TEC-761: stock count exports. The count report (one completed count:
// header info lines + every line with expected / counted values and the
// difference) and the count list (one row per count).

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/google/uuid"
)

var countReportColumns = []ioengine.Column{
	{Key: "location", LabelKey: "warehouse.export.location", Type: ioengine.ColumnTypeString},
	{Key: "product", LabelKey: "warehouse.export.product", Type: ioengine.ColumnTypeString, Weight: 1.6},
	{Key: "sku", LabelKey: "warehouse.export.sku", Type: ioengine.ColumnTypeString},
	{Key: "barcode", LabelKey: "warehouse.export.barcode", Type: ioengine.ColumnTypeString, Weight: 1.2},
	{Key: "expected_location", LabelKey: "warehouse.export.expected_location", Type: ioengine.ColumnTypeString},
	{Key: "expected_quantity", LabelKey: "warehouse.export.expected_quantity", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "counted_quantity", LabelKey: "warehouse.export.counted_quantity", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "quantity_diff", LabelKey: "warehouse.export.quantity_diff", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "expected_meters", LabelKey: "warehouse.export.expected_meters", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "counted_meters", LabelKey: "warehouse.export.counted_meters", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "meters_diff", LabelKey: "warehouse.export.meters_diff", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.7},
	{Key: "result", LabelKey: "warehouse.export.result", Type: ioengine.ColumnTypeString},
	{Key: "resolution", LabelKey: "warehouse.export.resolution", Type: ioengine.ColumnTypeString},
	{Key: "note", LabelKey: "warehouse.export.note", Type: ioengine.ColumnTypeString},
}

var countListColumns = []ioengine.Column{
	{Key: "warehouse", LabelKey: "warehouse.export.warehouse", Type: ioengine.ColumnTypeString, Weight: 1.3},
	{Key: "scope", LabelKey: "warehouse.export.scope", Type: ioengine.ColumnTypeString, Weight: 1.2},
	{Key: "method", LabelKey: "warehouse.export.method", Type: ioengine.ColumnTypeString},
	{Key: "visibility", LabelKey: "warehouse.export.visibility", Type: ioengine.ColumnTypeString, Weight: 0.8},
	{Key: "status", LabelKey: "warehouse.export.status", Type: ioengine.ColumnTypeString, Weight: 0.9},
	{Key: "lines", LabelKey: "warehouse.export.lines", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "differences", LabelKey: "warehouse.export.differences", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "created_at", LabelKey: "warehouse.export.created_at", Type: ioengine.ColumnTypeDatetime},
	{Key: "completed_at", LabelKey: "warehouse.export.completed_at", Type: ioengine.ColumnTypeDatetime},
	{Key: "approved_at", LabelKey: "warehouse.export.approved_at", Type: ioengine.ColumnTypeDatetime},
	{Key: "note", LabelKey: "warehouse.export.note", Type: ioengine.ColumnTypeString},
}

// countReport exports one completed count (query _count_uuid).
func (x *Exporter) countReport(ctx context.Context, c Caller, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	id, err := uuid.Parse(q[queryCountUUID])
	if err != nil {
		return ioengine.Dataset{}, ErrCountNotFound
	}
	rep, err := x.counts.Report(ctx, ScanCaller{Caller: c}, id)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	cnt, err := x.q.GetStockCountByUUID(ctx, db.GetStockCountByUUIDParams{Uuid: id, OrganizationID: c.Org.InternalID})
	if err != nil {
		return ioengine.Dataset{}, notFound(err, ErrCountNotFound)
	}
	counted, approved, err := x.countPeople(ctx, cnt)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(rep.Lines))
	for _, l := range rep.Lines {
		rows = append(rows, countReportRow(l, loc))
	}
	v := rep.Count
	info := []ioengine.InfoLine{
		{LabelKey: "warehouse.export.warehouse", Value: warehouseLabel(v.Warehouse.Code, v.Warehouse.Name)},
		{LabelKey: "warehouse.export.scope", Value: countScopeLabel(v, loc)},
		{LabelKey: "warehouse.export.method", Value: tr(loc, "warehouse.export.count_method.", v.Method)},
		{LabelKey: "warehouse.export.visibility", Value: tr(loc, "warehouse.export.count_visibility.", v.Visibility)},
		{LabelKey: "warehouse.export.status", Value: tr(loc, "warehouse.export.count_status.", v.Status)},
		{LabelKey: "warehouse.export.created_at", Value: infoTimePtr(&v.CreatedAt)},
		{LabelKey: "warehouse.export.started_at", Value: infoTimePtr(v.StartedAt)},
		{LabelKey: "warehouse.export.completed_at", Value: infoTimePtr(v.CompletedAt)},
		{LabelKey: "warehouse.export.approved_at", Value: infoTimePtr(v.ApprovedAt)},
		{LabelKey: "warehouse.export.counted_by", Value: counted},
		{LabelKey: "warehouse.export.approved_by", Value: approved},
		{LabelKey: "warehouse.export.note", Value: str(v.Note)},
	}
	return ioengine.Dataset{Rows: rows, Info: info}, nil
}

func countReportRow(l CountLine, loc i18n.Locale) map[string]any {
	locCode := func(p *CountLocation) string {
		if p == nil {
			return ""
		}
		return p.FullCode
	}
	barcode := ""
	if l.Unit != nil {
		barcode = l.Unit.Barcode
	}
	location := locCode(l.CountedLocation)
	if location == "" {
		location = locCode(l.ExpectedLocation)
	}
	resolution := ""
	if l.Resolution != nil {
		resolution = tr(loc, "warehouse.export.count_resolution.", *l.Resolution)
	}
	return map[string]any{
		"location":          location,
		"product":           l.Product.Name,
		"sku":               l.Product.SKU,
		"barcode":           barcode,
		"expected_location": locCode(l.ExpectedLocation),
		"expected_quantity": itoa(int64(l.ExpectedQuantity)),
		"counted_quantity":  itoa(int64(l.CountedQuantity)),
		"quantity_diff":     signed(int64(l.CountedQuantity) - int64(l.ExpectedQuantity)),
		"expected_meters":   str(l.ExpectedMeters),
		"counted_meters":    str(l.CountedMeters),
		"meters_diff":       metersDiff(l.ExpectedMeters, l.CountedMeters),
		"result":            tr(loc, "warehouse.export.count_result.", l.Result),
		"resolution":        resolution,
		"note":              str(l.Note),
	}
}

// signed prints a difference with its sign (+2, -1, 0).
func signed(n int64) string {
	if n > 0 {
		return "+" + itoa(n)
	}
	return itoa(n)
}

// metersDiff is counted - expected meters with two decimals; empty when
// neither side has meters.
func metersDiff(expected, counted *string) string {
	if expected == nil && counted == nil {
		return ""
	}
	parse := func(p *string) *big.Rat {
		r := new(big.Rat)
		if p != nil {
			if _, ok := r.SetString(*p); !ok {
				return new(big.Rat)
			}
		}
		return r
	}
	d := new(big.Rat).Sub(parse(counted), parse(expected))
	s := d.FloatString(2)
	if d.Sign() > 0 {
		return "+" + s
	}
	return s
}

func countScopeLabel(v StockCount, loc i18n.Locale) string {
	scope := tr(loc, "warehouse.export.count_scope.", v.ScopeType)
	var detail string
	switch {
	case v.ScopeRoom != nil:
		detail = warehouseLabel(v.ScopeRoom.Code, v.ScopeRoom.Name)
	case v.ScopeLocation != nil:
		detail = v.ScopeLocation.FullCode
	case v.ScopeProduct != nil:
		detail = warehouseLabel(v.ScopeProduct.SKU, v.ScopeProduct.Name)
	}
	if detail == "" {
		return scope
	}
	return scope + ": " + detail
}

// countPeople names the users who scanned the count and the approver.
func (x *Exporter) countPeople(ctx context.Context, cnt db.StockCount) (string, string, error) {
	scans, err := x.q.ListStockCountScans(ctx, cnt.ID)
	if err != nil {
		return "", "", fmt.Errorf("warehouse export: count scans: %w", err)
	}
	var ids []int64
	for _, sc := range scans {
		if sc.ScannedByUserID.Valid && !slices.Contains(ids, sc.ScannedByUserID.Int64) {
			ids = append(ids, sc.ScannedByUserID.Int64)
		}
	}
	if len(ids) == 0 && cnt.CompletedByUserID.Valid {
		ids = append(ids, cnt.CompletedByUserID.Int64)
	}
	all := slices.Clone(ids)
	if cnt.ApprovedByUserID.Valid {
		all = append(all, cnt.ApprovedByUserID.Int64)
	}
	if len(all) == 0 {
		return "", "", nil
	}
	users, err := x.q.ListUsersByIDs(ctx, all)
	if err != nil {
		return "", "", fmt.Errorf("warehouse export: users: %w", err)
	}
	names := make(map[int64]string, len(users))
	for _, u := range users {
		names[u.ID] = strings.TrimSpace(u.Name + " " + u.Surname)
	}
	counted := make([]string, 0, len(ids))
	for _, id := range ids {
		if n := names[id]; n != "" {
			counted = append(counted, n)
		}
	}
	approved := ""
	if cnt.ApprovedByUserID.Valid {
		approved = names[cnt.ApprovedByUserID.Int64]
	}
	return strings.Join(counted, ", "), approved, nil
}

// countList exports the count list (filters, q, sort, selection).
func (x *Exporter) countList(ctx context.Context, c Caller, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	values := exportValuesOf(q)
	f, err := ParseCountListFilter(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	sel := selectedRows(values)
	org := c.Org.InternalID
	var rows []map[string]any
	f.Limit, f.Offset = exportPage, 0
	for {
		page, total, err := x.counts.List(ctx, ScanCaller{Caller: c}, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, v := range page {
			if !sel.has(v.UUID) {
				continue
			}
			lines, diffs := "", ""
			if v.CompletedAt != nil {
				n, d, err := x.countLineTotals(ctx, org, v.UUID)
				if err != nil {
					return ioengine.Dataset{}, err
				}
				lines, diffs = itoa(n), itoa(d)
			}
			rows = append(rows, map[string]any{
				"warehouse":    warehouseLabel(v.Warehouse.Code, v.Warehouse.Name),
				"scope":        countScopeLabel(v, loc),
				"method":       tr(loc, "warehouse.export.count_method.", v.Method),
				"visibility":   tr(loc, "warehouse.export.count_visibility.", v.Visibility),
				"status":       tr(loc, "warehouse.export.count_status.", v.Status),
				"lines":        lines,
				"differences":  diffs,
				"created_at":   when(&v.CreatedAt),
				"completed_at": when(v.CompletedAt),
				"approved_at":  when(v.ApprovedAt),
				"note":         str(v.Note),
			})
		}
		f.Offset += int32(len(page)) //nolint:gosec // bounded by exportPage
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Rows: rows}, nil
}

func (x *Exporter) countLineTotals(ctx context.Context, org int64, id uuid.UUID) (int64, int64, error) {
	cnt, err := x.q.GetStockCountByUUID(ctx, db.GetStockCountByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return 0, 0, notFound(err, ErrCountNotFound)
	}
	lines, err := x.q.ListStockCountLines(ctx, cnt.ID)
	if err != nil {
		return 0, 0, fmt.Errorf("warehouse export: count lines: %w", err)
	}
	var diffs int64
	for _, l := range lines {
		if l.Result != CountResultMatched {
			diffs++
		}
	}
	return int64(len(lines)), diffs, nil
}

// infoTimeLayout prints info line times (UTC, as the datetime cells).
const infoTimeLayout = "02.01.2006 15:04 UTC"

func infoTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(infoTimeLayout)
}
