package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

const exportPage = 500

func ExportQuery(c Caller, body map[string]string) (ioengine.ExportQuery, error) {
	scope, ok := ioengine.EncodeScopeFilter(c.Filter)
	if !ok {
		return nil, ErrForbidden
	}
	out := ioengine.ExportQuery{ioengine.QueryScopeFilter: scope}
	for k, v := range body {
		out[k] = v
	}
	return out, nil
}

type SummaryExportAdapter struct{ svc *Service }

func NewSummaryExportAdapter(s *Service) *SummaryExportAdapter       { return &SummaryExportAdapter{svc: s} }
func (a *SummaryExportAdapter) Resource() string                     { return ResourceSummaryExport }
func (a *SummaryExportAdapter) ImportSchema() []ioengine.ImportField { return nil }
func (a *SummaryExportAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}
func (a *SummaryExportAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}
func (a *SummaryExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "dimension", LabelKey: "catalog.products.name", Type: ioengine.ColumnTypeString},
		{Key: "services", LabelKey: "customers.export.services", Type: ioengine.ColumnTypeString},
		{Key: "meters", LabelKey: "stock.export.meters", Type: ioengine.ColumnTypeString},
		{Key: "expected_meters", LabelKey: "warehouse.eod.meters_out", Type: ioengine.ColumnTypeString},
		{Key: "waste_ratio", LabelKey: "accounting.reports.margin_pct", Type: ioengine.ColumnTypeString},
	}
}
func (a *SummaryExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	c, err := a.svc.jobCaller(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	values := mapValues(q)
	f, err := analyticsFromMap(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	dim := values["dimension"]
	if dim == "" {
		dim = "dealer"
	}
	f.Limit, f.Offset = exportPage, 0
	var rows []map[string]any
	for {
		page, total, err := a.svc.Summary(ctx, c, dim, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, r := range page {
			rows = append(rows, map[string]any{
				"dimension": r.DimensionLabel, "services": r.Services, "meters": r.Meters,
				"expected_meters": strOrEmpty(r.ExpectedMeters), "waste_ratio": strOrEmpty(r.WasteRatio),
			})
		}
		f.Offset += int32(len(page)) //nolint:gosec
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceSummaryExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

type RollsExportAdapter struct{ svc *Service }

func NewRollsExportAdapter(s *Service) *RollsExportAdapter         { return &RollsExportAdapter{svc: s} }
func (a *RollsExportAdapter) Resource() string                     { return ResourceRollsExport }
func (a *RollsExportAdapter) ImportSchema() []ioengine.ImportField { return nil }
func (a *RollsExportAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}
func (a *RollsExportAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}
func (a *RollsExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "barcode", LabelKey: "stock.export.barcode", Type: ioengine.ColumnTypeString},
		{Key: "product", LabelKey: "stock.export.product", Type: ioengine.ColumnTypeString},
		{Key: "consumed_meters", LabelKey: "stock.export.meters", Type: ioengine.ColumnTypeString},
		{Key: "expected_meters", LabelKey: "warehouse.eod.meters_out", Type: ioengine.ColumnTypeString},
		{Key: "waste_ratio", LabelKey: "accounting.reports.margin_pct", Type: ioengine.ColumnTypeString},
	}
}
func (a *RollsExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	c, err := a.svc.jobCaller(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := rollFromMap(mapValues(q))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Limit, f.Offset = exportPage, 0
	var rows []map[string]any
	for {
		page, total, err := a.svc.Rolls(ctx, c, f, nil)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, r := range page {
			rows = append(rows, map[string]any{
				"barcode": r.Barcode, "product": r.ProductName, "consumed_meters": r.ConsumedMeters,
				"expected_meters": r.ExpectedMeters, "waste_ratio": strOrEmpty(r.WasteRatio),
			})
		}
		f.Offset += int32(len(page)) //nolint:gosec
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceRollsExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (s *Service) jobCaller(ctx context.Context, q ioengine.ExportQuery) (Caller, error) {
	orgID, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || orgID <= 0 {
		return Caller{}, errors.New("efficiency export: organization is required")
	}
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return Caller{}, fmt.Errorf("efficiency export organization: %w", err)
	}
	f, err := ioengine.JobScopeFilter(ctx, s.q, org, q[ioengine.QueryScopeFilter])
	if err != nil {
		return Caller{}, err
	}
	return Caller{Org: orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID}, Filter: f}, nil
}

func analyticsFromMap(v map[string]string) (AnalyticsFilter, error) {
	values := urlValues(v)
	q := apiquery.Parse(values)
	from, _ := time.Parse(time.DateOnly, v["period_from"])
	to, _ := time.Parse(time.DateOnly, v["period_to"])
	if from.IsZero() {
		from = time.Now().UTC().AddDate(0, -1, 0)
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	return AnalyticsFilter{From: from, To: to.AddDate(0, 0, 1), Sort: q.Sort}, nil
}

func rollFromMap(v map[string]string) (RollFilter, error) {
	values := urlValues(v)
	q := apiquery.Parse(values)
	waste, err := apiquery.NumRange(values, "waste_ratio")
	if err != nil {
		return RollFilter{}, err
	}
	used, err := apiquery.DateRange(values, "last_used")
	if err != nil {
		return RollFilter{}, err
	}
	return RollFilter{
		Q: q.Q, Sort: q.Sort,
		WasteRatioMin: waste.Min, WasteRatioMax: waste.Max,
		LastUsedFrom: used.From, LastUsedTo: used.Before,
	}, nil
}

func mapValues(q ioengine.ExportQuery) map[string]string {
	out := map[string]string{}
	for k, v := range q {
		out[k] = v
	}
	return out
}

func urlValues(v map[string]string) url.Values {
	out := url.Values{}
	for k, value := range v {
		out.Set(k, value)
	}
	return out
}

func strOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
