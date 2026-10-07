package usecase

// TEC-389 (F4-01g): AI usage report. The panel list (GET /v1/ai/usage)
// shows one organization: the active one, or another organization inside
// the caller's ai.usage.read scope (`organization`); anything else is 404,
// never distinguished from a missing organization. The platform list
// (GET /v1/platform/ai/usage) shows every organization. Both share the
// filters, the sort contract and the export.

import (
	"context"
	"net/url"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// UsageQuery is the usage list query (list contract): created_from/_to,
// tokens_min/_max (TEC-391), channel, purpose, model, pool, user (uuid CSV) and, on the platform list,
// organization (uuid CSV); sort created_at | tokens (default -created_at).
type UsageQuery struct {
	Created       apiquery.TimeRange
	TokensMin     *int64
	TokensMax     *int64
	Channels      []string
	Purposes      []string
	Models        []string
	Pools         []string
	Users         []uuid.UUID
	Organizations []uuid.UUID
	Sort          []apiquery.SortField
	Limit, Offset int32
}

// UsageSort is the sort contract of the usage report.
var UsageSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "tokens": "tokens"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// ParseUsageQuery reads the usage list parameters; organization is read
// only when platform is true. Bad values are *apiquery.ValidationError.
func ParseUsageQuery(values url.Values, platform bool) (UsageQuery, error) {
	q := apiquery.Parse(values)
	out := UsageQuery{Sort: q.Sort, Limit: q.Limit, Offset: q.Offset}
	if _, err := apiquery.ResolveSort(q.Sort, UsageSort); err != nil {
		return UsageQuery{}, err
	}
	var err error
	if out.Created, err = apiquery.DateRange(values, "created"); err != nil {
		return UsageQuery{}, err
	}
	tokens, err := apiquery.NumRange(values, "tokens")
	if err != nil {
		return UsageQuery{}, err
	}
	out.TokensMin, out.TokensMax = int64Bound(tokens.Min), int64Bound(tokens.Max)
	if out.Channels, err = apiquery.EnumList(values, "channel", model.UsageChannels...); err != nil {
		return UsageQuery{}, err
	}
	if out.Purposes, err = apiquery.EnumList(values, "purpose", model.UsagePurposes...); err != nil {
		return UsageQuery{}, err
	}
	if out.Pools, err = apiquery.EnumList(values, "pool", model.UsagePools...); err != nil {
		return UsageQuery{}, err
	}
	out.Models = apiquery.CSVValues(values, "model")
	if out.Users, err = uuidList(values, "user"); err != nil {
		return UsageQuery{}, err
	}
	if platform {
		if out.Organizations, err = uuidList(values, "organization"); err != nil {
			return UsageQuery{}, err
		}
	}
	return out, nil
}

// int64Bound truncates a token bound to a whole number.
func int64Bound(v *float64) *int64 {
	if v == nil {
		return nil
	}
	n := int64(*v)
	return &n
}

func uuidList(values url.Values, key string) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for _, raw := range apiquery.CSVValues(values, key) {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: key, Message: "must be a UUID list", Code: "invalid"}}}
		}
		out = append(out, id)
	}
	return out, nil
}

// UsageRow is one ledger row of the report.
type UsageRow struct {
	ID               int64     `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	Organization     OrgRef    `json:"organization"`
	User             *UserRef  `json:"user"`
	Pool             string    `json:"pool"`
	Channel          string    `json:"channel"`
	Purpose          string    `json:"purpose"`
	Model            string    `json:"model"`
	InputTokens      int64     `json:"input_tokens"`
	OutputTokens     int64     `json:"output_tokens"`
	CacheReadTokens  int64     `json:"cache_read_tokens"`
	CacheWriteTokens int64     `json:"cache_write_tokens"`
	// Tokens is the quota count (input + output + cache write).
	Tokens int64 `json:"tokens"`
}

// ResolveUsageOrg returns the organization a panel caller reports on: the
// active organization when orgUUID is nil, else that organization when it
// is inside the ai.usage.read reach f. Missing and foreign organizations
// are ErrNotFound alike.
func (a *Admin) ResolveUsageOrg(ctx context.Context, f scopefilter.Filter, active orgctx.Scope, orgUUID *uuid.UUID) (db.Organization, error) {
	id := active.UUID
	if orgUUID != nil {
		id = *orgUUID
	}
	org, ok, err := a.Store.OrganizationByUUID(ctx, id)
	if err != nil {
		return db.Organization{}, err
	}
	if !ok || (org.ID != active.InternalID && !f.AllowsOrg(org.ID, org.BrandID)) {
		return db.Organization{}, ErrNotFound
	}
	return org, nil
}

// ListUsage returns a page of the usage ledger. orgIDs nil lists every
// organization (platform); a panel caller passes its resolved organization.
func (a *Admin) ListUsage(ctx context.Context, orgIDs []int64, q UsageQuery) ([]UsageRow, int64, error) {
	f := repository.UsageFilter{
		OrganizationIDs: orgIDs, Pools: q.Pools, Channels: q.Channels, Purposes: q.Purposes, Models: q.Models,
		Created: q.Created, TokensMin: q.TokensMin, TokensMax: q.TokensMax,
		Sort: q.Sort, Limit: q.Limit, Offset: q.Offset,
	}
	if _, err := apiquery.ResolveSort(q.Sort, UsageSort); err != nil {
		return nil, 0, err
	}
	if len(q.Users) > 0 {
		ids, err := a.Store.UserIDsByUUIDs(ctx, q.Users)
		if err != nil {
			return nil, 0, err
		}
		f.UserIDs = ids
	}
	if len(q.Organizations) > 0 {
		ids, err := a.Store.OrganizationIDsByUUIDs(ctx, q.Organizations)
		if err != nil {
			return nil, 0, err
		}
		if orgIDs != nil {
			ids = intersect(orgIDs, ids)
		}
		f.OrganizationIDs = ids
	}
	rows, total, err := a.Store.ListUsage(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	out, err := a.usageRows(ctx, rows)
	return out, total, err
}

func intersect(a, b []int64) []int64 {
	out := []int64{}
	for _, x := range a {
		for _, y := range b {
			if x == y {
				out = append(out, x)
				break
			}
		}
	}
	return out
}

func (a *Admin) usageRows(ctx context.Context, rows []db.AiUsage) ([]UsageRow, error) {
	var orgIDs, userIDs []int64
	for _, r := range rows {
		orgIDs = append(orgIDs, r.OrganizationID)
		if r.UserID.Valid {
			userIDs = append(userIDs, r.UserID.Int64)
		}
	}
	orgs, err := a.Store.OrganizationsByIDs(ctx, orgIDs)
	if err != nil {
		return nil, err
	}
	users, err := a.Store.UsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]UsageRow, 0, len(rows))
	for _, r := range rows {
		o := orgs[r.OrganizationID]
		row := UsageRow{
			ID: r.ID, CreatedAt: r.CreatedAt.Time, Organization: OrgRef{UUID: o.Uuid, Name: o.Name, Type: o.Type},
			Pool: r.Pool, Channel: r.Channel, Purpose: r.Purpose, Model: r.Model,
			InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
			CacheReadTokens: r.CacheReadTokens, CacheWriteTokens: r.CacheWriteTokens, Tokens: r.QuotaTokens,
		}
		if r.UserID.Valid {
			if u, ok := users[r.UserID.Int64]; ok {
				row.User = &UserRef{UUID: u.Uuid, Name: u.Name, Surname: u.Surname}
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// --- summary -----------------------------------------------------------------

// QuotaUsage is a pool's monthly quota and use.
type QuotaUsage struct {
	Limit     int64    `json:"limit"`
	Used      int64    `json:"used"`
	Remaining *int64   `json:"remaining"`
	Percent   *float64 `json:"percent"`
}

func quotaUsage(limit, used int64) QuotaUsage {
	q := QuotaUsage{Limit: limit, Used: used, Percent: Percent(used, limit)}
	if limit > 0 {
		r := max(limit-used, 0)
		q.Remaining = &r
	}
	return q
}

// UsageTotals sums tokens and model calls.
type UsageTotals struct {
	Tokens           int64 `json:"tokens"`
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	Requests         int64 `json:"requests"`
}

// UserUsage is the month of one user (User nil: calls without a user).
type UserUsage struct {
	User         *UserRef `json:"user"`
	Tokens       int64    `json:"tokens"`
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	Requests     int64    `json:"requests"`
}

// ChannelUsage is the month of one channel and pool.
type ChannelUsage struct {
	Channel string `json:"channel"`
	Pool    string `json:"pool"`
	UsageTotals
}

// UsageSummary is the monthly report of one organization.
type UsageSummary struct {
	Period       string     `json:"period"`
	Organization OrgRef     `json:"organization"`
	Enabled      bool       `json:"enabled"`
	Quota        QuotaUsage `json:"quota"`
	// SystemPool is the brand center's separate pool (customer, visitor
	// and triage calls); only on the center.
	SystemPool *QuotaUsage    `json:"system_pool"`
	Totals     UsageTotals    `json:"totals"`
	ByUser     []UserUsage    `json:"by_user"`
	ByChannel  []ChannelUsage `json:"by_channel"`
}

// Summary returns the month (YYYY-MM, empty = current) of an organization:
// the quota of its pool(s), the totals and the user and channel breakdown.
func (a *Admin) Summary(ctx context.Context, org db.Organization, rawPeriod string) (UsageSummary, error) {
	period, start, err := ParsePeriod(rawPeriod, a.now())
	if err != nil {
		return UsageSummary{}, err
	}
	end := start.AddDate(0, 1, 0)
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return UsageSummary{}, err
	}
	os, ok, err := a.Store.OrgSettings(ctx, org.ID)
	if err != nil {
		return UsageSummary{}, err
	}
	out := UsageSummary{
		Period: period, Organization: OrgRef{UUID: org.Uuid, Name: org.Name, Type: org.Type},
		Enabled: !ok || os.Enabled, ByUser: []UserUsage{}, ByChannel: []ChannelUsage{},
	}
	used, err := a.Store.MonthlyTokens(ctx, org.ID, model.PoolOrg, start)
	if err != nil {
		return UsageSummary{}, err
	}
	out.Quota = quotaUsage(EffectiveQuota(settings, os.MonthlyTokenQuota), used)
	if org.Type == "center" {
		sys, err := a.Store.MonthlyTokens(ctx, org.ID, model.PoolSystem, start)
		if err != nil {
			return UsageSummary{}, err
		}
		q := quotaUsage(settings.SystemPoolMonthlyQuota, sys)
		out.SystemPool = &q
	}
	channels, err := a.Store.UsageByChannel(ctx, org.ID, start, end)
	if err != nil {
		return UsageSummary{}, err
	}
	for _, c := range channels {
		t := UsageTotals{Tokens: c.Tokens, InputTokens: c.InputTokens, OutputTokens: c.OutputTokens,
			CacheReadTokens: c.CacheReadTokens, CacheWriteTokens: c.CacheWriteTokens, Requests: c.Requests}
		out.ByChannel = append(out.ByChannel, ChannelUsage{Channel: c.Channel, Pool: c.Pool, UsageTotals: t})
		out.Totals.Tokens += t.Tokens
		out.Totals.InputTokens += t.InputTokens
		out.Totals.OutputTokens += t.OutputTokens
		out.Totals.CacheReadTokens += t.CacheReadTokens
		out.Totals.CacheWriteTokens += t.CacheWriteTokens
		out.Totals.Requests += t.Requests
	}
	byUser, err := a.Store.UsageByUser(ctx, org.ID, start, end)
	if err != nil {
		return UsageSummary{}, err
	}
	var ids []int64
	for _, u := range byUser {
		if u.UserID.Valid {
			ids = append(ids, u.UserID.Int64)
		}
	}
	users, err := a.Store.UsersByIDs(ctx, ids)
	if err != nil {
		return UsageSummary{}, err
	}
	for _, u := range byUser {
		row := UserUsage{Tokens: u.Tokens, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, Requests: u.Requests}
		if u.UserID.Valid {
			if x, ok := users[u.UserID.Int64]; ok {
				row.User = &UserRef{UUID: x.Uuid, Name: x.Name, Surname: x.Surname}
			}
		}
		out.ByUser = append(out.ByUser, row)
	}
	return out, nil
}
