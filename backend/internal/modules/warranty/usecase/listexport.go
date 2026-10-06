package usecase

// TEC-377 (DT-BE-7): warranty list export (I/O engine, queue "exports").
// POST /v1/warranties/export queues a job of the active organization with
// the list parameters of GET /v1/warranties (filters, q, sort) and the
// caller's resolved warranties.read scope (organizations, plus the service
// creator for an own / assigned scope). The worker rebuilds the caller
// against the job organization (ioengine.JobScopeFilter) and reads the rows
// through List, so the file holds exactly what the list shows. Poll and
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

// ResourceListExport is the export_jobs.resource of the warranty list.
const ResourceListExport = "warranties.list"

// QueryScopeUser is the job query key of an own / assigned scope's user.
const QueryScopeUser = "_scope_user"

// ErrExportScope: the caller's scope reaches no organization (customer).
var ErrExportScope = errors.New("warranty: the export scope reaches no organization")

const listExportPage = 500

// ListExportQuery builds the job query of an export request: the list
// parameters of body (validated with ParseListFilter, so a bad value is a
// 400 at request time) plus the caller's scope.
func ListExportQuery(c Caller, body map[string]string) (ioengine.ExportQuery, error) {
	values := ListValues(body)
	if _, err := ParseListFilter(values); err != nil {
		return nil, err
	}
	scope, ok := ioengine.EncodeScopeFilter(c.Filter)
	if !ok || c.Filter.Scope == rbac.ScopeCustomer {
		return nil, ErrExportScope
	}
	out := ioengine.ExportQuery{ioengine.QueryScopeFilter: scope}
	if c.Filter.Scope == rbac.ScopeOwn || c.Filter.Scope == rbac.ScopeAssigned {
		out[QueryScopeUser] = strconv.FormatInt(c.Filter.UserID, 10)
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

// ListExportAdapter exports the warranty list.
type ListExportAdapter struct{ r *Reader }

// NewListExportAdapter creates the warranty list export adapter.
func NewListExportAdapter(r *Reader) *ListExportAdapter { return &ListExportAdapter{r: r} }

// Resource implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) Resource() string { return ResourceListExport }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "public_code", LabelKey: "warranties.export.public_code", Type: ioengine.ColumnTypeString, Weight: 1.2},
		{Key: "status", LabelKey: "warranties.export.status", Type: ioengine.ColumnTypeEnum, Weight: 0.8},
		{Key: "product", LabelKey: "warranties.export.product", Type: ioengine.ColumnTypeString, Weight: 1.4},
		{Key: "service_no", LabelKey: "warranties.export.service_no", Type: ioengine.ColumnTypeString},
		{Key: "organization", LabelKey: "warranties.export.organization", Type: ioengine.ColumnTypeString, Weight: 1.4},
		{Key: "holder", LabelKey: "warranties.export.holder", Type: ioengine.ColumnTypeString, Weight: 1.2},
		{Key: "plate", LabelKey: "warranties.export.plate", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "start_at", LabelKey: "warranties.export.start_at", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "end_at", LabelKey: "warranties.export.end_at", Type: ioengine.ColumnTypeString, Weight: 0.8},
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
	c, err := a.r.listJobCaller(ctx, q)
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
		page, total, err := a.r.List(ctx, c, f)
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

func listExportRow(v WarrantyListView, loc i18n.Locale) map[string]any {
	holder := ""
	if v.Holder != nil {
		holder = strings.TrimSpace(v.Holder.Name + " " + v.Holder.Surname)
		if v.Holder.Anonymized {
			holder = i18n.Translate(loc, anonymizedNameKey)
		}
	}
	plate := ""
	if v.Vehicle.Plate != nil {
		plate = *v.Vehicle.Plate
	}
	return map[string]any{
		"public_code":  v.PublicCode,
		"status":       i18n.Translate(loc, "customers.export.warranty_status."+v.Status),
		"product":      v.Product.Name,
		"service_no":   v.Service.ServiceNo,
		"organization": v.Organization.Name,
		"holder":       holder,
		"plate":        plate,
		"start_at":     v.StartAt.UTC().Format(time.DateOnly),
		"end_at":       v.EndAt.UTC().Format(time.DateOnly),
	}
}

// listJobCaller rebuilds the caller of a list export job: the job
// organization as the active organization and the stored scope
// re-authorized against it (own / assigned keep the stored user).
func (r *Reader) listJobCaller(ctx context.Context, q ioengine.ExportQuery) (Caller, error) {
	orgID, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || orgID <= 0 {
		return Caller{}, errors.New("warranty list export: organization is required")
	}
	org, err := r.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return Caller{}, fmt.Errorf("warranty list export: organization: %w", err)
	}
	f, err := ioengine.JobScopeFilter(ctx, r.q, org, q[ioengine.QueryScopeFilter])
	if err != nil {
		return Caller{}, err
	}
	if raw := strings.TrimSpace(q[QueryScopeUser]); raw != "" {
		uid, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || uid <= 0 {
			return Caller{}, errors.New("warranty list export: invalid scope user")
		}
		f.Scope, f.UserID = rbac.ScopeOwn, uid
	}
	return Caller{
		Org:    orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID},
		Filter: f,
	}, nil
}

var _ ioengine.ResourceAdapter = (*ListExportAdapter)(nil)
