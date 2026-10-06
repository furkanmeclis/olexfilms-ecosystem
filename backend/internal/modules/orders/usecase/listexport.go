package usecase

// TEC-373 (DT-BE-5): order list export (I/O engine, queue "exports").
// POST /v1/orders/export queues a job of the active organization with the
// list parameters of GET /v1/orders (filters, q, sort) and the caller's
// resolved orders.read scope. The worker rebuilds the caller against the
// job organization (ioengine.JobScopeFilter) and reads the rows through
// List, so the file holds exactly what the list shows. Poll and download
// through /v1/tenant/exports/{uuid}.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
)

// ResourceListExport is the export_jobs.resource of the order list.
const ResourceListExport = "orders.list"

const listExportPage = 500

// ListExportQuery builds the job query of an export request: the list
// parameters of body (validated with ParseListFilter, so a bad value is a
// 400 at request time) plus the caller's scope. A side filter needs the
// active organization inside the scope, like the list.
func ListExportQuery(c Caller, body map[string]string) (ioengine.ExportQuery, error) {
	values := ListValues(body)
	f, err := ParseListFilter(values)
	if err != nil {
		return nil, err
	}
	if f.Side != SideAll && f.Side != SideSeller && f.Side != SideBuyer {
		return nil, invalid("side", "must be seller or buyer")
	}
	scope, ok := ioengine.EncodeScopeFilter(c.Filter)
	if !ok {
		return nil, ErrForbidden
	}
	out := ioengine.ExportQuery{ioengine.QueryScopeFilter: scope}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

// ListExportAdapter exports the order list.
type ListExportAdapter struct{ svc *Service }

// NewListExportAdapter creates the order list export adapter.
func NewListExportAdapter(svc *Service) *ListExportAdapter { return &ListExportAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) Resource() string { return ResourceListExport }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "order_no", LabelKey: "orders.export.order_no", Type: ioengine.ColumnTypeString},
		{Key: "status", LabelKey: "orders.export.status", Type: ioengine.ColumnTypeEnum, Weight: 0.8},
		{Key: "seller", LabelKey: "orders.export.seller", Type: ioengine.ColumnTypeString, Weight: 1.4},
		{Key: "buyer", LabelKey: "orders.export.buyer", Type: ioengine.ColumnTypeString, Weight: 1.4},
		{Key: "currency", LabelKey: "orders.export.currency", Type: ioengine.ColumnTypeString, Weight: 0.5},
		{Key: "subtotal", LabelKey: "orders.export.subtotal", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "tax_total", LabelKey: "orders.export.tax_total", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "total", LabelKey: "orders.export.total", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "created_at", LabelKey: "orders.export.created_at", Type: ioengine.ColumnTypeString, Weight: 0.8},
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
	f, err := ParseListFilter(ListValues(q))
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
		for _, o := range page {
			rows = append(rows, map[string]any{
				"order_no":   o.OrderNo,
				"status":     i18n.Translate(loc, StatusLabelKey(o.Status)),
				"seller":     o.Seller.Name,
				"buyer":      o.Buyer.Name,
				"currency":   o.Currency,
				"subtotal":   o.Subtotal,
				"tax_total":  o.TaxTotal,
				"total":      o.Total,
				"created_at": o.CreatedAt.UTC().Format(time.DateOnly),
			})
		}
		f.Offset += int32(len(page)) //nolint:gosec // bounded by listExportPage
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceListExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

// jobCaller rebuilds the caller of an export job: the job organization as
// the active organization and the stored scope re-authorized against it.
func (s *Service) jobCaller(ctx context.Context, q ioengine.ExportQuery) (Caller, error) {
	orgID, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || orgID <= 0 {
		return Caller{}, errors.New("orders list export: organization is required")
	}
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return Caller{}, fmt.Errorf("orders list export: organization: %w", err)
	}
	f, err := ioengine.JobScopeFilter(ctx, s.q, org, q[ioengine.QueryScopeFilter])
	if err != nil {
		return Caller{}, err
	}
	return Caller{
		Org:    orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID},
		Filter: f,
	}, nil
}

var _ ioengine.ResourceAdapter = (*ListExportAdapter)(nil)
