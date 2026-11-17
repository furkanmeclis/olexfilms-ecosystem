package usecase

// TEC-1226: the stock adjustment list export (one row per document; no
// price or amount).

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
)

var adjustmentColumns = []ioengine.Column{
	{Key: "adjustment_no", LabelKey: "warehouse.export.adjustment_no", Type: ioengine.ColumnTypeString},
	{Key: "type", LabelKey: "warehouse.export.adjustment_type", Type: ioengine.ColumnTypeString, Weight: 0.8},
	{Key: "status", LabelKey: "warehouse.export.status", Type: ioengine.ColumnTypeString, Weight: 0.9},
	{Key: "source", LabelKey: "warehouse.export.source", Type: ioengine.ColumnTypeString, Weight: 0.9},
	{Key: "warehouse", LabelKey: "warehouse.export.warehouse", Type: ioengine.ColumnTypeString, Weight: 1.2},
	{Key: "total_quantity", LabelKey: "warehouse.export.total_quantity", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "total_meters", LabelKey: "warehouse.export.total_meters", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "reason_code", LabelKey: "warehouse.export.reason_code", Type: ioengine.ColumnTypeString},
	{Key: "created_by", LabelKey: "warehouse.export.created_by", Type: ioengine.ColumnTypeString},
	{Key: "approved_by", LabelKey: "warehouse.export.approved_by", Type: ioengine.ColumnTypeString},
	{Key: "created_at", LabelKey: "warehouse.export.created_at", Type: ioengine.ColumnTypeDatetime},
	{Key: "posted_at", LabelKey: "warehouse.export.posted_at", Type: ioengine.ColumnTypeDatetime},
	{Key: "note", LabelKey: "warehouse.export.note", Type: ioengine.ColumnTypeString},
}

func (x *Exporter) adjustmentList(ctx context.Context, c Caller, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	values := exportValuesOf(q)
	f, err := ParseAdjustmentListFilter(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	person := func(p *AdjustmentPerson) string {
		if p == nil {
			return ""
		}
		return p.Name
	}
	var rows []map[string]any
	f.Limit, f.Offset = exportPage, 0
	for {
		page, total, err := x.adjustments.List(ctx, ScanCaller{Caller: c}, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, v := range page {
			wh := ""
			if v.Warehouse != nil {
				wh = warehouseLabel(v.Warehouse.Code, v.Warehouse.Name)
			}
			rows = append(rows, map[string]any{
				"adjustment_no":  v.AdjustmentNo,
				"type":           tr(loc, "warehouse.export.adjustment_type.", v.Type),
				"status":         tr(loc, "warehouse.export.adjustment_status.", v.Status),
				"source":         tr(loc, "warehouse.export.adjustment_source.", v.Source),
				"warehouse":      wh,
				"total_quantity": itoa(int64(v.TotalQuantity)),
				"total_meters":   v.TotalMeters,
				"reason_code":    str(v.ReasonCode),
				"created_by":     person(v.CreatedBy),
				"approved_by":    person(v.ApprovedBy),
				"created_at":     when(&v.CreatedAt),
				"posted_at":      when(v.PostedAt),
				"note":           str(v.Note),
			})
		}
		f.Offset += int32(len(page)) //nolint:gosec // bounded by exportPage
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Rows: rows}, nil
}
