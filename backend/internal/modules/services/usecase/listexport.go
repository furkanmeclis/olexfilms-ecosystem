package usecase

// TEC-377 (DT-BE-7): service list export (I/O engine, queue "exports").
// POST /v1/services/export queues a job of the active organization with the
// list parameters of GET /v1/services (filters, q, sort) and the caller's
// resolved services.read scope (organizations, plus the creator for an own
// / assigned scope). The worker rebuilds the caller against the job
// organization (ioengine.JobScopeFilter) and reads the rows through List,
// so the file holds exactly what the list shows. Income and profit are not
// exported (they depend on grants the worker does not hold). Poll and
// download through /v1/tenant/exports/{uuid}.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// ResourceListExport is the export_jobs.resource of the service list.
const ResourceListExport = "services.list"

// QueryScopeUser is the job query key of an own / assigned scope's user.
const QueryScopeUser = "_scope_user"

const listExportPage = 200

// ListExportQuery builds the job query of an export request: the list
// parameters of body (validated with ParseListFilter, so a bad value is a
// 400 at request time) plus the caller's scope. A scope that reaches no
// organization (customer) is forbidden.
func ListExportQuery(c Caller, body map[string]string) (ioengine.ExportQuery, error) {
	values := ListValues(body)
	if _, err := ParseListFilter(values); err != nil {
		return nil, err
	}
	scope, ok := ioengine.EncodeScopeFilter(c.Filter)
	if !ok || c.Filter.Scope == rbac.ScopeCustomer {
		return nil, ErrForbidden
	}
	out := ioengine.ExportQuery{ioengine.QueryScopeFilter: scope}
	if c.Filter.UserOnly() {
		out[QueryScopeUser] = strconv.FormatInt(c.Filter.UserID, 10)
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

// ListExportAdapter exports the service list.
type ListExportAdapter struct{ svc *Service }

// NewListExportAdapter creates the service list export adapter.
func NewListExportAdapter(svc *Service) *ListExportAdapter { return &ListExportAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) Resource() string { return ResourceListExport }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "service_no", LabelKey: "services.export.service_no", Type: ioengine.ColumnTypeString},
		{Key: "status", LabelKey: "services.export.status", Type: ioengine.ColumnTypeEnum, Weight: 0.8},
		{Key: "organization", LabelKey: "services.export.organization", Type: ioengine.ColumnTypeString, Weight: 1.4},
		{Key: "customer", LabelKey: "services.export.customer", Type: ioengine.ColumnTypeString, Weight: 1.2},
		{Key: "plate", LabelKey: "services.export.plate", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "vehicle", LabelKey: "services.export.vehicle", Type: ioengine.ColumnTypeString, Weight: 1.2},
		{Key: "package", LabelKey: "services.export.package", Type: ioengine.ColumnTypeString},
		{Key: "created_at", LabelKey: "services.export.created_at", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "completed_at", LabelKey: "services.export.completed_at", Type: ioengine.ColumnTypeString, Weight: 0.8},
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
	f, err := ParseListFilter(ListValues(q))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	// The worker reads SQL directly: the index order would not follow sort.
	f.SortExplicit = true
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
	return ioengine.Dataset{Resource: ResourceListExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

func listExportRow(v ServiceView, loc i18n.Locale) map[string]any {
	customer := strings.TrimSpace(v.Customer.Name + " " + v.Customer.Surname)
	if v.Customer.Anonymized {
		customer = i18n.Translate(loc, AnonymizedNameKey)
	}
	completed := ""
	if v.CompletedAt != nil {
		completed = v.CompletedAt.UTC().Format(time.DateOnly)
	}
	return map[string]any{
		"service_no":   v.ServiceNo,
		"status":       i18n.Translate(loc, StatusLabelKey(v.Status)),
		"organization": v.Organization.Name,
		"customer":     customer,
		"plate":        ptrString(v.Plate),
		"vehicle":      strings.TrimSpace(v.CarBrand.Name + " " + v.CarModel.Name),
		"package":      ptrString(v.Package),
		"created_at":   v.CreatedAt.UTC().Format(time.DateOnly),
		"completed_at": completed,
	}
}

// listJobCaller rebuilds the caller of a list export job: the job
// organization as the active organization and the stored scope
// re-authorized against it (own / assigned keep the stored user).
func (s *Service) listJobCaller(ctx context.Context, q ioengine.ExportQuery) (Caller, error) {
	orgID, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || orgID <= 0 {
		return Caller{}, errors.New("services list export: organization is required")
	}
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return Caller{}, fmt.Errorf("services list export: organization: %w", err)
	}
	f, err := ioengine.JobScopeFilter(ctx, s.q, org, q[ioengine.QueryScopeFilter])
	if err != nil {
		return Caller{}, err
	}
	if raw := strings.TrimSpace(q[QueryScopeUser]); raw != "" {
		uid, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || uid <= 0 {
			return Caller{}, errors.New("services list export: invalid scope user")
		}
		f.Scope, f.UserID = rbac.ScopeOwn, uid
	}
	return Caller{
		Org:    orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID},
		Filter: f,
	}, nil
}

var _ ioengine.ResourceAdapter = (*ListExportAdapter)(nil)
