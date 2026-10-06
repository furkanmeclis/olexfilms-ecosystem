package usecase

// TEC-373 (DT-BE-5): unit list export (I/O engine, queue "exports").
// POST /v1/stock/organizations/{uuid}/units/export queues a job of the
// active organization with the list parameters of the unit list (filters,
// q, sort), the listed organization and the caller's resolved stock.read
// scope. The worker rebuilds the viewer against the job organization
// (ioengine.JobScopeFilter) and reads the rows through OrganizationUnits
// with SQL search (no index), so the listed organization must still be
// inside the re-authorized scope. The purchase price column is not
// exported (its visibility needs the requesting principal's grants).

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ResourceUnitsExport is the export_jobs.resource of the unit list.
const ResourceUnitsExport = "stock.units"

// QueryListedOrganization is the job query key of the listed organization.
const QueryListedOrganization = "_organization_uuid"

const unitsExportPage = 500

// UnitsExportQuery builds the job query of a unit list export of
// organization orgUUID: the list parameters of body (validated with
// ParseUnitFilter, so a bad value is a 400 at request time), the listed
// organization and the caller's scope. The organization must be inside
// the scope (ErrNotFound otherwise, like the list).
func (s *Service) UnitsExportQuery(ctx context.Context, f scopefilter.Filter, orgUUID uuid.UUID, body map[string]string) (ioengine.ExportQuery, error) {
	values := UnitListValues(body)
	if _, err := ParseUnitFilter(values); err != nil {
		return nil, err
	}
	o, err := s.q.GetOrganizationByUUID(ctx, orgUUID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (o.DeletedAt.Valid || !f.AllowsOrg(o.ID, o.BrandID))) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stock units export: organization: %w", err)
	}
	scope, ok := ioengine.EncodeScopeFilter(f)
	if !ok {
		return nil, ErrNotFound
	}
	out := ioengine.ExportQuery{ioengine.QueryScopeFilter: scope, QueryListedOrganization: orgUUID.String()}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

// UnitsExportAdapter exports an organization's unit list.
type UnitsExportAdapter struct{ svc *Service }

// NewUnitsExportAdapter creates the unit list export adapter. svc must
// not have an index finder (the export searches SQL).
func NewUnitsExportAdapter(svc *Service) *UnitsExportAdapter { return &UnitsExportAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *UnitsExportAdapter) Resource() string { return ResourceUnitsExport }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *UnitsExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "barcode", LabelKey: "stock.export.barcode", Type: ioengine.ColumnTypeString, Weight: 1.2},
		{Key: "product", LabelKey: "stock.export.product", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "sku", LabelKey: "stock.export.sku", Type: ioengine.ColumnTypeString},
		{Key: "status", LabelKey: "stock.export.status", Type: ioengine.ColumnTypeEnum, Weight: 0.8},
		{Key: "quantity", LabelKey: "stock.export.quantity", Type: ioengine.ColumnTypeString, Weight: 0.6},
		{Key: "meters", LabelKey: "stock.export.meters", Type: ioengine.ColumnTypeString, Weight: 0.6},
		{Key: "location", LabelKey: "stock.export.location", Type: ioengine.ColumnTypeString},
		{Key: "updated_at", LabelKey: "stock.export.updated_at", Type: ioengine.ColumnTypeString, Weight: 0.8},
	}
}

// ImportSchema implements ioengine.ResourceAdapter (export only).
func (a *UnitsExportAdapter) ImportSchema() []ioengine.ImportField { return nil }

// ApplyRow implements ioengine.ResourceAdapter (export only).
func (a *UnitsExportAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (export only).
func (a *UnitsExportAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}

// Export implements ioengine.ResourceAdapter.
func (a *UnitsExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	v, err := a.svc.unitsJobViewer(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	listed, err := uuid.Parse(q[QueryListedOrganization])
	if err != nil {
		return ioengine.Dataset{}, errors.New("stock units export: organization is required")
	}
	in, err := ParseUnitFilter(UnitListValues(q))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	in.Limit, in.Offset = unitsExportPage, 0
	rows := []map[string]any{}
	for {
		page, total, err := a.svc.OrganizationUnits(ctx, v, listed, in)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, u := range page {
			meters, location := "", ""
			if u.RemainingMeters != nil {
				meters = *u.RemainingMeters
			}
			if u.Location != nil {
				location = u.Location.Code
			}
			rows = append(rows, map[string]any{
				"barcode":    u.Barcode,
				"product":    u.Product.Name,
				"sku":        u.Product.SKU,
				"status":     i18n.Translate(loc, "stock.export.status."+u.Status),
				"quantity":   strconv.Itoa(int(u.Quantity)),
				"meters":     meters,
				"location":   location,
				"updated_at": u.UpdatedAt.UTC().Format(time.DateOnly),
			})
		}
		in.Offset += int32(len(page)) //nolint:gosec // bounded by unitsExportPage
		if len(page) == 0 || int64(in.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceUnitsExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

// unitsJobViewer rebuilds the viewer of an export job: the job
// organization and the stored scope re-authorized against it.
func (s *Service) unitsJobViewer(ctx context.Context, q ioengine.ExportQuery) (UnitViewer, error) {
	orgID, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || orgID <= 0 {
		return UnitViewer{}, errors.New("stock units export: job organization is required")
	}
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return UnitViewer{}, fmt.Errorf("stock units export: organization: %w", err)
	}
	f, err := ioengine.JobScopeFilter(ctx, s.q, org, q[ioengine.QueryScopeFilter])
	if err != nil {
		return UnitViewer{}, err
	}
	return UnitViewer{
		Org:    orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID},
		Filter: f,
	}, nil
}

var _ ioengine.ResourceAdapter = (*UnitsExportAdapter)(nil)
