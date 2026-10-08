package usecase

// TEC-477: fleet list export (I/O engine, queue "exports"). POST
// /v1/fleets/export queues a job of the active organization with the list
// parameters of GET /v1/fleets (status, q, sort, vehicle_count_min/_max)
// and the caller's resolved fleets.read scope (the dealer organizations it
// reaches). The worker rebuilds the caller against the job organization
// (ioengine.JobScopeFilter) and reads the rows through List, so the file
// holds exactly what the list shows. Poll and download through
// /v1/tenant/exports/{uuid}.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// ResourceList is the export_jobs.resource of the fleet list.
const ResourceList = "tenant.fleets"

const listExportPage = 200

// listKeys are the list parameters an export job carries.
var listKeys = []string{"q", "sort", "status", "vehicle_count_min", "vehicle_count_max"}

// ListValues keeps the list parameters of a job query (or export body).
func ListValues(q map[string]string) url.Values {
	out := url.Values{}
	for _, k := range listKeys {
		if v := strings.TrimSpace(q[k]); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

// ParseListValues reads the GET /v1/fleets parameters (docs/list-contract.md):
// q, status (link status CSV: pending | active | ended),
// vehicle_count_min/_max, sort, limit and offset. Errors are
// *apiquery.ValidationError (400); an unknown sort field is reported by List.
func ParseListValues(values url.Values) (ListFilter, error) {
	q := apiquery.Parse(values)
	statuses, err := apiquery.EnumList(values, "status", model.LinkPending, model.LinkActive, model.LinkEnded)
	if err != nil {
		return ListFilter{}, err
	}
	rng, err := apiquery.NumRange(values, "vehicle_count")
	if err != nil {
		return ListFilter{}, err
	}
	f := ListFilter{Statuses: statuses, Q: q.Q, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset}
	if rng.Min != nil {
		v := int64(*rng.Min)
		f.VehicleCountMin = &v
	}
	if rng.Max != nil {
		v := int64(*rng.Max)
		f.VehicleCountMax = &v
	}
	return f, nil
}

// ListExportQuery builds the job query of an export request: the list
// parameters of body (validated with ParseListValues, so a bad value is a
// 400 at request time) plus the caller's scope. A scope that reaches no
// organization is forbidden.
func ListExportQuery(c Caller, body map[string]string) (ioengine.ExportQuery, error) {
	values := ListValues(body)
	if _, err := ParseListValues(values); err != nil {
		return nil, err
	}
	scope, ok := ioengine.EncodeScopeFilter(c.Filter)
	if !ok || c.Filter.Scope == rbac.ScopeCustomer || c.BrandID == 0 {
		return nil, ErrForbidden
	}
	out := ioengine.ExportQuery{ioengine.QueryScopeFilter: scope}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

// LinkStatusLabelKey is the i18n key of a dealer link status.
func LinkStatusLabelKey(status string) string { return "fleet.link_status." + status }

// ListExportAdapter exports the fleet list.
type ListExportAdapter struct{ svc *Service }

// NewListExportAdapter creates the fleet list export adapter.
func NewListExportAdapter(svc *Service) *ListExportAdapter { return &ListExportAdapter{svc: svc} }

var _ ioengine.ResourceAdapter = (*ListExportAdapter)(nil)

// Resource implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) Resource() string { return ResourceList }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "name", LabelKey: "fleet.export.name", Type: ioengine.ColumnTypeString, Weight: 1.2},
		{Key: "legal_name", LabelKey: "fleet.export.legal_name", Type: ioengine.ColumnTypeString, Weight: 1.4},
		{Key: "tax_number", LabelKey: "fleet.export.tax_number", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "vehicle_count", LabelKey: "fleet.export.vehicle_count", Type: ioengine.ColumnTypeString, Weight: 0.6, AlignRight: true},
		{Key: "last_service_at", LabelKey: "fleet.export.last_service_at", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "link_status", LabelKey: "fleet.export.link_status", Type: ioengine.ColumnTypeEnum, Weight: 0.7},
		{Key: "dealer", LabelKey: "fleet.export.dealer", Type: ioengine.ColumnTypeString, Weight: 1.2},
		{Key: "started_at", LabelKey: "fleet.export.started_at", Type: ioengine.ColumnTypeString, Weight: 0.8},
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
	c, err := a.svc.listJobCaller(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := ParseListValues(ListValues(q))
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
		for _, v := range page {
			rows = append(rows, listExportRow(v, loc))
		}
		f.Offset += int32(len(page)) //nolint:gosec // bounded by listExportPage
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceList, Columns: a.ExportColumns(), Rows: rows}, nil
}

func listExportRow(v FleetListItem, loc i18n.Locale) map[string]any {
	return map[string]any{
		"name":            v.Name,
		"legal_name":      v.LegalName,
		"tax_number":      v.TaxNumber,
		"vehicle_count":   strconv.FormatInt(v.VehicleCount, 10),
		"last_service_at": exportDate(v.LastServiceAt),
		"link_status":     i18n.Translate(loc, LinkStatusLabelKey(v.Link.Status)),
		"dealer":          v.Link.DealerName,
		"started_at":      exportDate(v.Link.StartedAt),
	}
}

func exportDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.DateOnly)
}

// listJobCaller rebuilds the caller of a list export job: the job
// organization as the active organization and the stored scope
// re-authorized against it.
func (s *Service) listJobCaller(ctx context.Context, q ioengine.ExportQuery) (Caller, error) {
	orgID, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || orgID <= 0 {
		return Caller{}, errors.New("fleet list export: organization is required")
	}
	queries := db.New(s.conn)
	org, err := queries.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return Caller{}, fmt.Errorf("fleet list export: organization: %w", err)
	}
	f, err := ioengine.JobScopeFilter(ctx, queries, org, q[ioengine.QueryScopeFilter])
	if err != nil {
		return Caller{}, err
	}
	return Caller{
		Org:   orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID},
		OrgID: org.ID, BrandID: org.BrandID, OrgType: org.Type, Filter: f,
	}, nil
}
