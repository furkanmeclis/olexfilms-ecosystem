package usecase

// TEC-164: customer list export (I/O engine, queue "exports", worker-docs).
// POST /v1/customers/export queues a job of the active organization with
// the list filters (q, status) and the resolved permission scope: "brand"
// or the organization ids of the caller's scope. The worker re-authorizes
// the job like every tenant export: a brand wide scope needs a center job
// organization, organization ids are kept only when the job organization
// covers them (itself or below it, ioengine.JobOrgCovers). Rows come from
// the same query and masking as GET /v1/customers (anonymized customers
// show the anonymized label, never personal data; identity numbers are not
// exported).

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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
)

// ResourceListExport is the export_jobs.resource of the customer list.
const ResourceListExport = "customers.list"

// List export query keys (written by the HTTP handler after authorization).
const (
	QueryQ      = "q"
	QueryStatus = "status"
	// QueryScope is "brand" or a comma separated list of organization ids.
	QueryScope = "scope"
)

// ScopeBrand is the QueryScope value of a brand wide scope.
const ScopeBrand = "brand"

// listExportPage is the page size the export reads the list with.
const listExportPage = 500

// errListExportScope: the stored scope reaches no organization of the job.
var errListExportScope = errors.New("customers list export: scope is outside the job organization")

// ExportScope encodes the caller's scope for the export job query.
func ExportScope(c Caller) (string, error) {
	ids := c.orgIDs()
	if ids == nil {
		return ScopeBrand, nil
	}
	if len(ids) == 0 {
		return "", ErrForbidden
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(strs, ","), nil
}

// ListExportAdapter exports the customer list.
type ListExportAdapter struct{ svc *Service }

// NewListExportAdapter creates the customer list export adapter.
func NewListExportAdapter(svc *Service) *ListExportAdapter { return &ListExportAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) Resource() string { return ResourceListExport }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "name", LabelKey: "customers.export.name", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "phone", LabelKey: "customers.export.phone", Type: ioengine.ColumnTypeString},
		{Key: "email", LabelKey: "customers.export.email", Type: ioengine.ColumnTypeString, Weight: 1.4},
		{Key: "type", LabelKey: "customers.export.type", Type: ioengine.ColumnTypeEnum, Weight: 0.8},
		{Key: "company_name", LabelKey: "customers.export.company_name", Type: ioengine.ColumnTypeString},
		{Key: "status", LabelKey: "customers.export.status", Type: ioengine.ColumnTypeEnum, Weight: 0.8},
		{Key: "locale", LabelKey: "customers.export.locale", Type: ioengine.ColumnTypeString, Weight: 0.5},
		{Key: "created_at", LabelKey: "customers.export.created_at", Type: ioengine.ColumnTypeString, Weight: 0.8},
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
	c, err := a.svc.listExportCaller(ctx, q, loc)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := ListFilter{Q: q[QueryQ], Status: q[QueryStatus], Limit: listExportPage}
	rows := []map[string]any{}
	for {
		page, total, err := a.svc.ListCustomers(ctx, c, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, s := range page {
			rows = append(rows, listExportRow(s, loc))
		}
		f.Offset += int32(len(page))
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceListExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

// listExportCaller rebuilds the caller of an export job and re-authorizes
// the stored scope against the job organization.
func (s *Service) listExportCaller(ctx context.Context, q ioengine.ExportQuery, loc i18n.Locale) (Caller, error) {
	jobOrg, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || jobOrg <= 0 {
		return Caller{}, errors.New("customers list export: organization is required")
	}
	org, err := s.q.GetOrganizationByID(ctx, jobOrg)
	if err != nil {
		return Caller{}, fmt.Errorf("customers list export: organization: %w", err)
	}
	c := Caller{
		Org:    orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID},
		Locale: loc,
	}
	raw := strings.TrimSpace(q[QueryScope])
	if raw == ScopeBrand && org.Type == rbac.OrgTypeCenter {
		c.Filter = scopefilter.Filter{Scope: rbac.ScopeBrand, OrgID: org.ID, BrandID: org.BrandID}
		return c, nil
	}
	// The job organization covers itself and its subtree (a brand wide
	// grant outside the center falls back to exactly that).
	below, err := s.q.Descendants(ctx, org.ID)
	if err != nil {
		return Caller{}, fmt.Errorf("customers list export: descendants: %w", err)
	}
	covered := map[int64]bool{org.ID: true}
	tree := []int64{org.ID}
	for _, o := range below {
		covered[o.ID] = true
		tree = append(tree, o.ID)
	}
	var ids []int64
	if raw == ScopeBrand {
		ids = tree
	} else {
		for _, part := range strings.Split(raw, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil && covered[id] {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return Caller{}, errListExportScope
	}
	c.Filter = scopefilter.Filter{Scope: rbac.ScopeSubtree, OrgID: org.ID, OrgIDs: ids}
	return c, nil
}

func listExportRow(s CustomerSummary, loc i18n.Locale) map[string]any {
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	status := i18n.Translate(loc, "users.status."+s.Status)
	typ := i18n.Translate(loc, "customers.export.type."+s.Type)
	return map[string]any{
		"name":         strings.TrimSpace(s.Name + " " + s.Surname),
		"phone":        str(s.Phone),
		"email":        str(s.Email),
		"type":         typ,
		"company_name": str(s.CompanyName),
		"status":       status,
		"locale":       str(s.Locale),
		"created_at":   s.CreatedAt.UTC().Format(time.DateOnly),
	}
}
