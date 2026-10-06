package usecase

import (
	"context"
	"fmt"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/orglist"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
)

// ResourcePlatformList is the platform organizations list export
// (TEC-365, POST /v1/platform/organizations/export).
const ResourcePlatformList = "platform.organizations"

// listExportPage is the page size the export walks the list with.
const listExportPage int32 = 500

// ListExportAdapter exports the platform organizations list with the list
// filters, q and sort; the job query carries the request brand as
// orglist.QueryBrandID.
type ListExportAdapter struct{ svc *Service }

// NewListExportAdapter creates the platform organizations export adapter.
func NewListExportAdapter(svc *Service) *ListExportAdapter { return &ListExportAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) Resource() string { return ResourcePlatformList }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "uuid", LabelKey: "organizations.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "slug", LabelKey: "organizations.slug", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "organizations.name", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "type", LabelKey: "organizations.type", Type: ioengine.ColumnTypeEnum},
		{Key: "parent", LabelKey: "organizations.parent", Type: ioengine.ColumnTypeString, Weight: 1.4},
		{Key: "city", LabelKey: "organizations.city", Type: ioengine.ColumnTypeString},
		{Key: "phone", LabelKey: "organizations.phone", Type: ioengine.ColumnTypeString},
		{Key: "status", LabelKey: "organizations.status", Type: ioengine.ColumnTypeEnum},
		{Key: "plan_code", LabelKey: "organizations.plan_code", Type: ioengine.ColumnTypeString},
		{Key: "access_ends_at", LabelKey: "organizations.access_ends_at", Type: ioengine.ColumnTypeDatetime},
		{Key: "created_at", LabelKey: "organizations.created_at", Type: ioengine.ColumnTypeDatetime},
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
func (a *ListExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	brandID, err := strconv.ParseInt(q[orglist.QueryBrandID], 10, 64)
	if err != nil || brandID <= 0 {
		return ioengine.Dataset{}, fmt.Errorf("%w: export brand is missing", ErrInvalidRequest)
	}
	f, err := orglist.Parse(orglist.QueryValues(q))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	ctx = brandctx.WithBrand(ctx, brandctx.Brand{ID: brandID})
	rows := []map[string]any{}
	var offset int32
	for {
		page, total, err := a.svc.List(ctx, listExportPage, offset, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, o := range page {
			rows = append(rows, listExportRow(o))
		}
		offset += int32(len(page)) //nolint:gosec // bounded by the page size
		if len(page) == 0 || int64(offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourcePlatformList, Columns: a.ExportColumns(), Rows: rows}, nil
}

func listExportRow(o Organization) map[string]any {
	row := map[string]any{
		"uuid": o.UUID.String(), "slug": o.Slug, "name": o.Name, "type": o.Type,
		"city": o.City, "phone": o.Phone, "status": o.Status, "created_at": o.CreatedAt,
		"parent": "", "plan_code": "", "access_ends_at": nil,
	}
	if o.Parent != nil {
		row["parent"] = o.Parent.Name
	}
	if o.PlanCode != nil {
		row["plan_code"] = *o.PlanCode
	}
	if o.AccessEndsAt != nil {
		row["access_ends_at"] = *o.AccessEndsAt
	}
	return row
}
