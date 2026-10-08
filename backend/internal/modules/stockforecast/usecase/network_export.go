package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
)

// ResourceNetworkExport is the export_jobs.resource of the center network demand list.
const ResourceNetworkExport = "stock_forecasts.network"

const networkExportPage = 500

var networkExportKeys = []string{"q", "from_month", "months"}

// NetworkExportValues keeps the network list parameters of a job query.
func NetworkExportValues(q map[string]string) url.Values {
	out := url.Values{}
	for _, k := range networkExportKeys {
		if v := strings.TrimSpace(q[k]); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

// NetworkExportQuery validates and stores the network list parameters for an export job.
func NetworkExportQuery(body map[string]string, now time.Time) (ioengine.ExportQuery, error) {
	values := NetworkExportValues(body)
	if _, err := ParseNetworkFilter(values, now); err != nil {
		return nil, err
	}
	out := ioengine.ExportQuery{}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

// NetworkExportAdapter exports the center network demand forecast list.
type NetworkExportAdapter struct{ svc *Service }

// NewNetworkExportAdapter creates the center network demand export adapter.
func NewNetworkExportAdapter(svc *Service) *NetworkExportAdapter {
	return &NetworkExportAdapter{svc: svc}
}

// Resource implements ioengine.ResourceAdapter.
func (a *NetworkExportAdapter) Resource() string { return ResourceNetworkExport }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *NetworkExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "product_sku", LabelKey: "stock_forecasts.export.product_sku", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "product_name", LabelKey: "stock_forecasts.export.product_name", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "forecast_month", LabelKey: "stock_forecasts.export.forecast_month", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "expected_qty", LabelKey: "stock_forecasts.export.expected_qty", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "expected_meters", LabelKey: "stock_forecasts.export.expected_meters", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "network_on_hand_qty", LabelKey: "stock_forecasts.export.network_on_hand_qty", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "network_on_hand_meters", LabelKey: "stock_forecasts.export.network_on_hand_meters", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "open_order_qty", LabelKey: "stock_forecasts.export.open_order_qty", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "open_order_meters", LabelKey: "stock_forecasts.export.open_order_meters", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "suggested_production_qty", LabelKey: "stock_forecasts.export.suggested_production_qty", Type: ioengine.ColumnTypeString, Weight: 0.9},
		{Key: "suggested_production_meters", LabelKey: "stock_forecasts.export.suggested_production_meters", Type: ioengine.ColumnTypeString, Weight: 0.9},
	}
}

// ImportSchema implements ioengine.ResourceAdapter (export only).
func (a *NetworkExportAdapter) ImportSchema() []ioengine.ImportField { return nil }

// ApplyRow implements ioengine.ResourceAdapter (export only).
func (a *NetworkExportAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (export only).
func (a *NetworkExportAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}

// Export implements ioengine.ResourceAdapter.
func (a *NetworkExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	c, err := a.svc.networkJobCaller(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := ParseNetworkFilter(NetworkExportValues(q), time.Now())
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Limit, f.Offset = networkExportPage, 0
	rows := []map[string]any{}
	for {
		page, total, err := a.svc.Network(ctx, c, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, row := range page {
			rows = append(rows, map[string]any{
				"product_sku":                 row.Product.SKU,
				"product_name":                row.Product.Name,
				"forecast_month":              row.ForecastMonth,
				"expected_qty":                row.ExpectedQty,
				"expected_meters":             row.ExpectedMeters,
				"network_on_hand_qty":         row.NetworkOnHandQty,
				"network_on_hand_meters":      row.NetworkOnHandMeters,
				"open_order_qty":              row.OpenOrderQty,
				"open_order_meters":           row.OpenOrderMeters,
				"suggested_production_qty":    row.SuggestedProductionQty,
				"suggested_production_meters": row.SuggestedProductionMeters,
			})
		}
		f.Offset += int32(len(page)) //nolint:gosec // bounded by networkExportPage
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceNetworkExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (s *Service) networkJobCaller(ctx context.Context, q ioengine.ExportQuery) (Caller, error) {
	orgID, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || orgID <= 0 {
		return Caller{}, errors.New("stock forecast network export: organization is required")
	}
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return Caller{}, fmt.Errorf("stock forecast network export: organization: %w", err)
	}
	return Caller{Org: orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID}}, nil
}

var _ ioengine.ResourceAdapter = (*NetworkExportAdapter)(nil)
