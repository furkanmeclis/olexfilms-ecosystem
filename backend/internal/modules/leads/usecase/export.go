package usecase

// TEC-371 (DT-BE-4): lead list export (I/O engine, queue "exports").
// POST /v1/leads/export queues a job of the active organization with the
// list filters and sort of GET /v1/leads plus the caller's resolved scope
// (EncodeScope) and user id. The worker rebuilds the caller with JobCaller,
// which re-authorizes the scope against the job organization, and reads the
// rows through List, so the file holds exactly what the list shows. The job
// is polled and downloaded through /v1/tenant/exports.

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/jackc/pgx/v5"
)

// ResourceListExport is the export_jobs.resource of the lead list.
const ResourceListExport = "leads.list"

// Job query keys written by the server (never taken from the request).
const (
	// QueryScope is "brand" or the comma separated organization ids.
	QueryScope = "scope"
	// QueryActorUserID is the requesting user (internal id).
	QueryActorUserID = "actor_user_id"
)

const listExportPage = 500

// ListExportQuery builds the job query of an export request: the list
// parameters of body (validated with ParseListFilter, so a bad value is a
// 400 at request time) plus the caller's scope and user.
func ListExportQuery(c Caller, body map[string]string) (ioengine.ExportQuery, error) {
	values := ListValues(body)
	if _, err := ParseListFilter(values); err != nil {
		return nil, err
	}
	scope, err := EncodeScope(c.Filter)
	if err != nil {
		return nil, err
	}
	out := ioengine.ExportQuery{
		QueryScope:       scope,
		QueryActorUserID: strconv.FormatInt(c.Principal.UserInternal, 10),
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

// ListExportAdapter exports the lead list.
type ListExportAdapter struct{ svc *Service }

// NewListExportAdapter creates the lead list export adapter.
func NewListExportAdapter(svc *Service) *ListExportAdapter { return &ListExportAdapter{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) Resource() string { return ResourceListExport }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *ListExportAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "name", LabelKey: "leads.export.name", Type: ioengine.ColumnTypeString, Weight: 1.6},
		{Key: "phone", LabelKey: "leads.export.phone", Type: ioengine.ColumnTypeString},
		{Key: "email", LabelKey: "leads.export.email", Type: ioengine.ColumnTypeString, Weight: 1.4},
		{Key: "target_type", LabelKey: "leads.export.target_type", Type: ioengine.ColumnTypeEnum},
		{Key: "source", LabelKey: "leads.export.source", Type: ioengine.ColumnTypeEnum},
		{Key: "temperature", LabelKey: "leads.export.temperature", Type: ioengine.ColumnTypeEnum, Weight: 0.7},
		{Key: "status", LabelKey: "leads.export.status", Type: ioengine.ColumnTypeEnum, Weight: 0.8},
		{Key: "follow_up_date", LabelKey: "leads.export.follow_up_date", Type: ioengine.ColumnTypeString, Weight: 0.8},
		{Key: "assignee", LabelKey: "leads.export.assignee", Type: ioengine.ColumnTypeString},
		{Key: "created_at", LabelKey: "leads.export.created_at", Type: ioengine.ColumnTypeString, Weight: 0.8},
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
	orgID, err := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || orgID <= 0 {
		return ioengine.Dataset{}, errors.New("leads list export: organization is required")
	}
	actor, _ := strconv.ParseInt(q[QueryActorUserID], 10, 64)
	c, err := JobCaller(ctx, a.svc.q, orgID, q[QueryScope], actor)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f, err := ParseListFilter(ListValues(q))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Limit = listExportPage
	names := map[int64]string{}
	rows := []map[string]any{}
	for {
		page, total, err := a.svc.List(ctx, c, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, l := range page {
			assignee, err := a.assigneeName(ctx, names, l.AssigneeUserID)
			if err != nil {
				return ioengine.Dataset{}, err
			}
			rows = append(rows, listExportRow(l, assignee, loc))
		}
		f.Offset += int32(len(page)) //nolint:gosec // page length is bounded by listExportPage
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceListExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

// assigneeName resolves (and caches) the assignee's display name.
func (a *ListExportAdapter) assigneeName(ctx context.Context, cache map[int64]string, id *int64) (string, error) {
	if id == nil {
		return "", nil
	}
	if n, ok := cache[*id]; ok {
		return n, nil
	}
	u, err := a.svc.q.GetUserByID(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) {
		cache[*id] = ""
		return "", nil
	}
	if err != nil {
		return "", err
	}
	n := strings.TrimSpace(u.Name + " " + u.Surname)
	cache[*id] = n
	return n, nil
}

func listExportRow(l Lead, assignee string, loc i18n.Locale) map[string]any {
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return strings.TrimSpace(*p)
	}
	name := str(l.CandidateCompanyName)
	if contact := str(l.CandidateContactName); contact != "" {
		if name != "" {
			name += " · " + contact
		} else {
			name = contact
		}
	}
	follow := ""
	if l.FollowUpDate != nil {
		follow = l.FollowUpDate.UTC().Format(time.DateOnly)
	}
	return map[string]any{
		"name":           name,
		"phone":          str(l.CandidatePhoneE164),
		"email":          str(l.CandidateEmail),
		"target_type":    i18n.Translate(loc, "leads.export.target_type."+l.TargetType),
		"source":         i18n.Translate(loc, "leads.export.source."+l.Source),
		"temperature":    i18n.Translate(loc, "leads.export.temperature."+l.Temperature),
		"status":         i18n.Translate(loc, "leads.export.status."+l.Status),
		"follow_up_date": follow,
		"assignee":       assignee,
		"created_at":     l.CreatedAt.UTC().Format(time.DateOnly),
	}
}

var _ ioengine.ResourceAdapter = (*ListExportAdapter)(nil)
