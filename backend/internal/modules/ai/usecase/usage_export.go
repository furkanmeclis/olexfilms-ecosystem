package usecase

// TEC-389 (F4-01g): usage report export (I/O engine, CSV / XLSX) with the
// filters and sort of the usage list. The request handler resolves the
// reach and stores it in the job query (QueryUsageScope / QueryUsageOrg);
// the worker re-authorizes an organization reach against the job
// organization before reading.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
)

const (
	// ResourceUsageExport is the export resource of the usage report.
	ResourceUsageExport = "ai.usage"
	// QueryUsageScope is the reach of the job: UsageScopePlatform (every
	// organization) or UsageScopeOrg (QueryUsageOrg only).
	QueryUsageScope = "_ai_usage_scope"
	// QueryUsageOrg is the internal id of the reported organization.
	QueryUsageOrg = "_ai_usage_org"

	UsageScopePlatform = "platform"
	UsageScopeOrg      = "org"

	usageExportPage int32 = 500
)

// usageExportKeys are the client query keys an export keeps (the list
// parameters without limit / offset).
var usageExportKeys = []string{"created_from", "created_to", "channel", "purpose", "model", "pool", "user", "sort"}

// ErrUsageExportScope: the job reach is missing or outside the job
// organization.
var ErrUsageExportScope = errors.New("ai usage export: scope is outside the job organization")

// UsageExportQuery validates the client query like the list does and
// returns the job query with the reach: orgID > 0 reports one organization,
// orgID 0 every organization (platform; organization filter allowed).
func UsageExportQuery(client map[string]string, orgID int64) (ioengine.ExportQuery, error) {
	platform := orgID == 0
	keys := usageExportKeys
	if platform {
		keys = append(append([]string(nil), keys...), "organization")
	}
	out := ioengine.ExportQuery{}
	values := url.Values{}
	for _, k := range keys {
		if v, ok := client[k]; ok && v != "" {
			out[k] = v
			values.Set(k, v)
		}
	}
	if _, err := ParseUsageQuery(values, platform); err != nil {
		return nil, err
	}
	if platform {
		out[QueryUsageScope] = UsageScopePlatform
	} else {
		out[QueryUsageScope] = UsageScopeOrg
		out[QueryUsageOrg] = strconv.FormatInt(orgID, 10)
	}
	return out, nil
}

// UsageExportAdapter exports the usage report.
type UsageExportAdapter struct{ admin *Admin }

// NewUsageExportAdapter creates the adapter.
func NewUsageExportAdapter(admin *Admin) *UsageExportAdapter {
	return &UsageExportAdapter{admin: admin}
}

// Resource implements ioengine.ResourceAdapter.
func (a *UsageExportAdapter) Resource() string { return ResourceUsageExport }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *UsageExportAdapter) ExportColumns() []ioengine.Column {
	num := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "ai.usage.export." + key, Type: ioengine.ColumnTypeString, AlignRight: true}
	}
	return []ioengine.Column{
		{Key: "created_at", LabelKey: "ai.usage.export.created_at", Type: ioengine.ColumnTypeDatetime},
		{Key: "organization", LabelKey: "ai.usage.export.organization", Type: ioengine.ColumnTypeString, Weight: 1.5},
		{Key: "user", LabelKey: "ai.usage.export.user", Type: ioengine.ColumnTypeString, Weight: 1.3},
		{Key: "pool", LabelKey: "ai.usage.export.pool", Type: ioengine.ColumnTypeEnum},
		{Key: "channel", LabelKey: "ai.usage.export.channel", Type: ioengine.ColumnTypeEnum},
		{Key: "purpose", LabelKey: "ai.usage.export.purpose", Type: ioengine.ColumnTypeEnum},
		{Key: "model", LabelKey: "ai.usage.export.model", Type: ioengine.ColumnTypeString, Weight: 1.3},
		num("input_tokens"), num("output_tokens"), num("cache_read_tokens"), num("cache_write_tokens"), num("tokens"),
	}
}

// ImportSchema implements ioengine.ResourceAdapter (export only).
func (a *UsageExportAdapter) ImportSchema() []ioengine.ImportField { return nil }

// ApplyRow implements ioengine.ResourceAdapter (export only).
func (a *UsageExportAdapter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "export only"}, nil
}

// RevertRow implements ioengine.ResourceAdapter (export only).
func (a *UsageExportAdapter) RevertRow(context.Context, string, string, map[string]any) error {
	return nil
}

// Export implements ioengine.ResourceAdapter.
func (a *UsageExportAdapter) Export(ctx context.Context, q ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgIDs, platform, err := a.reach(ctx, q)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	values := url.Values{}
	for _, k := range usageExportKeys {
		if v := q[k]; v != "" {
			values.Set(k, v)
		}
	}
	if platform && q["organization"] != "" {
		values.Set("organization", q["organization"])
	}
	uq, err := ParseUsageQuery(values, platform)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	uq.Limit, uq.Offset = usageExportPage, 0
	rows := []map[string]any{}
	for {
		page, total, err := a.admin.ListUsage(ctx, orgIDs, uq)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, r := range page {
			rows = append(rows, usageExportRow(r))
		}
		uq.Offset += int32(len(page))
		if len(page) == 0 || int64(uq.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: ResourceUsageExport, Columns: a.ExportColumns(), Rows: rows}, nil
}

// reach re-authorizes the job: a platform job has no organization; an
// organization job reports the job organization, one below it or, for a
// center job, one of its brand.
func (a *UsageExportAdapter) reach(ctx context.Context, q ioengine.ExportQuery) ([]int64, bool, error) {
	jobOrg, _ := strconv.ParseInt(q[ioengine.QueryOrganizationID], 10, 64)
	switch q[QueryUsageScope] {
	case UsageScopePlatform:
		if jobOrg > 0 {
			return nil, false, ErrUsageExportScope
		}
		return nil, true, nil
	case UsageScopeOrg:
	default:
		return nil, false, ErrUsageExportScope
	}
	target, err := strconv.ParseInt(q[QueryUsageOrg], 10, 64)
	if err != nil || target <= 0 || jobOrg <= 0 {
		return nil, false, ErrUsageExportScope
	}
	if target == jobOrg {
		return []int64{target}, false, nil
	}
	qs := a.admin.Store.Queries()
	job, err := qs.GetOrganizationByID(ctx, jobOrg)
	if err != nil {
		return nil, false, fmt.Errorf("ai usage export: job organization: %w", err)
	}
	org, err := qs.GetOrganizationByID(ctx, target)
	if err != nil {
		return nil, false, fmt.Errorf("ai usage export: organization: %w", err)
	}
	if job.Type == "center" && job.BrandID == org.BrandID {
		return []int64{target}, false, nil
	}
	below, err := a.admin.Store.Descendants(ctx, jobOrg)
	if err != nil {
		return nil, false, err
	}
	for _, o := range below {
		if o.ID == target {
			return []int64{target}, false, nil
		}
	}
	return nil, false, ErrUsageExportScope
}

func usageExportRow(r UsageRow) map[string]any {
	user := ""
	if r.User != nil {
		user = r.User.Name + " " + r.User.Surname
	}
	n := func(v int64) string { return strconv.FormatInt(v, 10) }
	return map[string]any{
		"created_at": r.CreatedAt, "organization": r.Organization.Name, "user": user,
		"pool": r.Pool, "channel": r.Channel, "purpose": r.Purpose, "model": r.Model,
		"input_tokens": n(r.InputTokens), "output_tokens": n(r.OutputTokens),
		"cache_read_tokens": n(r.CacheReadTokens), "cache_write_tokens": n(r.CacheWriteTokens),
		"tokens": n(r.Tokens),
	}
}

var _ ioengine.ResourceAdapter = (*UsageExportAdapter)(nil)
