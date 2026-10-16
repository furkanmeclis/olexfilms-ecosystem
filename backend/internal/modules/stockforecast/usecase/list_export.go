package usecase

// TEC-755: stock forecast list export (I/O engine, queue "exports").
// POST /v1/stock-forecasts/export queues a job of the active organization
// with the list parameters of GET /v1/stock-forecasts (status, days_left
// range, q, sort). The list reads only the organization's own forecasts,
// so the worker rebuilds the caller from the job organization and reads the
// rows through List; the file holds exactly what the list shows.

import (
	"context"
	"net/url"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
)

// ResourceListExport is the export_jobs.resource of the stock forecast list.
const ResourceListExport = "stock_forecasts.list"

const listExportPage = 500

var listExportKeys = []string{"q", "sort", "status", "days_left_min", "days_left_max"}

// ListExportValues keeps the list parameters of a job query.
func ListExportValues(q map[string]string) url.Values {
	out := url.Values{}
	for _, k := range listExportKeys {
		if v := strings.TrimSpace(q[k]); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

// ListExportQuery validates and stores the list parameters for an export
// job, so a bad value is a 400 at request time.
func ListExportQuery(body map[string]string) (ioengine.ExportQuery, error) {
	values := ListExportValues(body)
	if _, err := ParseListFilter(values); err != nil {
		return nil, err
	}
	out := ioengine.ExportQuery{}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

// ListExportAdapter exports the stock forecast list.
type ListExportAdapter struct{ svc *Service }

// NewListExportAdapter creates the stock forecast list export adapter.
func NewListExportAdapter(svc *Service) *ListExportAdapter { return &ListExportAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) Resource() string { return ResourceListExport }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "product_sku", LabelKey: "stock_forecasts.export.product_sku", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "product_name", LabelKey: "stock_forecasts.export.product_name", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "category_name", LabelKey: "stock_forecasts.export.category_name", Type: ioengine.ColumnTypeString, Weight: 1},
		{Key: "status", LabelKey: "stock_forecasts.export.status", Type: ioengine.ColumnTypeEnum, Weight: 0.8},
		{Key: "on_hand_qty", LabelKey: "stock_forecasts.export.on_hand_qty", Type: ioengine.ColumnTypeString, Weight: 0.7},
		{Key: "on_hand_meters", LabelKey: "stock_forecasts.export.on_hand_meters", Type: ioengine.ColumnTypeString, Weight: 0.7},
		{Key: "avg_daily_30", LabelKey: "stock_forecasts.export.avg_daily_30", Type: ioengine.ColumnTypeString, Weight: 0.7},
		{Key: "avg_daily_90", LabelKey: "stock_forecasts.export.avg_daily_90", Type: ioengine.ColumnTypeString, Weight: 0.7},
		{Key: "seasonality_factor", LabelKey: "stock_forecasts.export.seasonality_factor", Type: ioengine.ColumnTypeString, Weight: 0.7},
		{Key: "days_left", LabelKey: "stock_forecasts.export.days_left", Type: ioengine.ColumnTypeString, Weight: 0.7},
		{Key: "depletion_date", LabelKey: "stock_forecasts.export.depletion_date", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "vehicles_left", LabelKey: "stock_forecasts.export.vehicles_left", Type: ioengine.ColumnTypeString, Weight: 0.7},
		{Key: "suggested_qty", LabelKey: "stock_forecasts.export.suggested_qty", Type: ioengine.ColumnTypeString, Weight: 0.7},
		{Key: "suggested_meters", LabelKey: "stock_forecasts.export.suggested_meters", Type: ioengine.ColumnTypeString, Weight: 0.7},
	}
}

// ImportSchema implements ioengine.ResourceAdapter (export only).
func (a *ListExportAdapter) ImportSchema() []ioengine.ImportField { return nil }

// ApplyRow implements ioengine.ResourceAdapter (export only).
func (a *ListExportAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (export only).
func (a *ListExportAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}

// Export implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	c, err := a.svc.jobCaller(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := ParseListFilter(ListExportValues(q))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Limit, f.Offset = listExportPage, 0
	rows := []map[string]any{}
	for {
		page, total, err := a.svc.List(ctx, c, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, row := range page {
			rows = append(rows, map[string]any{
				"product_sku":        row.Product.SKU,
				"product_name":       row.Product.Name,
				"category_name":      row.Product.CategoryName,
				"status":             row.Status, // enum column: stock_forecasts.status.<value>
				"on_hand_qty":        row.OnHandQty,
				"on_hand_meters":     row.OnHandMeters,
				"avg_daily_30":       row.AvgDaily30,
				"avg_daily_90":       row.AvgDaily90,
				"seasonality_factor": row.SeasonalityFactor,
				"days_left":          deref(row.DaysLeft),
				"depletion_date":     deref(row.DepletionDate),
				"vehicles_left":      deref(row.VehiclesLeft),
				"suggested_qty":      derefInt(row.SuggestedQty),
				"suggested_meters":   deref(row.SuggestedMeters),
			})
		}
		f.Offset += int32(len(page)) //nolint:gosec // bounded by listExportPage
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceListExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(n *int32) any {
	if n == nil {
		return ""
	}
	return *n
}

var _ ioengine.ResourceAdapter = (*ListExportAdapter)(nil)
