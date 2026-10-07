package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-389 acceptance on the database. Every test runs in one rolled-back
// transaction.

type adminFixture struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	store   *repository.Store
	admin   *Admin
	suffix  string
	center  db.Organization
	dealerA db.Organization
	dealerB db.Organization
	userA   db.User
	userA2  db.User
	userB   db.User
}

type fakeTools []string

func (f fakeTools) All() []tools.Tool {
	out := make([]tools.Tool, 0, len(f))
	for _, n := range f {
		out = append(out, namedTool(n))
	}
	return out
}

type namedTool string

func (n namedTool) Spec() tools.Spec {
	return tools.Spec{Name: string(n), Kind: tools.KindRead, Realm: tools.RealmPanel}
}

func (namedTool) Run(context.Context, tools.Env, json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}

func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	f := &adminFixture{ctx: ctx, tx: tx, q: db.New(tx), store: repository.New(tx),
		suffix: fmt.Sprintf("%d", time.Now().UnixNano())}
	f.admin = NewAdmin(f.store, llm.Models{Default: "claude-sonnet-5-5", Fast: "claude-haiku-4-5",
		Allowed: []string{"claude-sonnet-5-5", "claude-haiku-4-5"}}, fakeTools{"list_tasks", "create_task"})
	brand, err := f.q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	if f.center, err = f.q.GetBrandCenter(ctx, brand.ID); err != nil {
		t.Fatalf("center: %v", err)
	}
	org := func(name string) db.Organization {
		o, err := f.q.CreateOrganization(ctx, db.CreateOrganizationParams{
			Slug: "tec389-" + name + "-" + f.suffix, Name: "TEC389 " + name + " " + f.suffix, Status: "active",
			AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
			Type:           "dealer", ParentID: pgtype.Int8{Int64: f.center.ID, Valid: true},
			BrandID: brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		})
		if err != nil {
			t.Fatalf("org %s: %v", name, err)
		}
		return o
	}
	f.dealerA, f.dealerB = org("a"), org("b")
	user := func(name string) db.User {
		u, err := f.q.CreateUser(ctx, db.CreateUserParams{
			PasswordHash: "x", Name: "TEC389", Surname: name, Status: "active",
			Email: pgtype.Text{String: "tec389-" + name + "-" + f.suffix + "@example.test", Valid: true},
		})
		if err != nil {
			t.Fatalf("user %s: %v", name, err)
		}
		return u
	}
	f.userA, f.userA2, f.userB = user("a"), user("a2"), user("b")
	return f
}

func (f *adminFixture) book(t *testing.T, org db.Organization, user *db.User, channel string, in, out int64) {
	t.Helper()
	u := repository.Usage{OrganizationID: org.ID, BrandID: org.BrandID, Pool: model.PoolOrg,
		Channel: channel, Purpose: model.PurposeChat, Model: "claude-sonnet-5-5", InputTokens: in, OutputTokens: out}
	if user != nil {
		u.UserID = &user.ID
	}
	if _, _, err := f.store.RecordUsage(f.ctx, u); err != nil {
		t.Fatalf("record usage: %v", err)
	}
}

func scopeOf(o db.Organization) orgctx.Scope {
	return orgctx.Scope{InternalID: o.ID, UUID: o.Uuid, Name: o.Name, OrgType: o.Type, BrandID: o.BrandID}
}

// Acceptance: a dealer owner (ai.usage.read managed) never sees another
// organization's usage: list, summary and export resolve it as 404; a
// missing organization looks the same. A brand-scoped center sees it.
func TestUsageForeignOrganizationIsNotFound(t *testing.T) {
	f := newAdminFixture(t)
	f.book(t, f.dealerB, &f.userB, model.UsageChannelPanel, 100, 10)
	owner := scopefilter.Filter{Permission: rbac.PermAIUsageRead, Scope: rbac.ScopeManaged,
		OrgID: f.dealerA.ID, OrgIDs: []int64{f.dealerA.ID}}

	got, err := f.admin.ResolveUsageOrg(f.ctx, owner, scopeOf(f.dealerA), nil)
	if err != nil || got.ID != f.dealerA.ID {
		t.Fatalf("own org = %v %v", got.ID, err)
	}
	if _, err := f.admin.ResolveUsageOrg(f.ctx, owner, scopeOf(f.dealerA), &f.dealerB.Uuid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign org err = %v, want ErrNotFound", err)
	}
	missing := f.userA.Uuid // not an organization
	if _, err := f.admin.ResolveUsageOrg(f.ctx, owner, scopeOf(f.dealerA), &missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing org err = %v", err)
	}
	center := scopefilter.Filter{Permission: rbac.PermAIUsageRead, Scope: rbac.ScopeBrand,
		OrgID: f.center.ID, BrandID: f.center.BrandID}
	if got, err := f.admin.ResolveUsageOrg(f.ctx, center, scopeOf(f.center), &f.dealerB.Uuid); err != nil || got.ID != f.dealerB.ID {
		t.Fatalf("center brand scope = %v %v", got.ID, err)
	}

	// The panel list of the dealer owner only holds its own rows.
	rows, total, err := f.admin.ListUsage(f.ctx, []int64{f.dealerA.ID}, UsageQuery{Limit: 50})
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("own list = %d %v", total, err)
	}

	// An export job of dealer A asking for dealer B is refused by the
	// worker re-authorization.
	q, err := UsageExportQuery(map[string]string{}, f.dealerB.ID)
	if err != nil {
		t.Fatal(err)
	}
	q[ioengine.QueryOrganizationID] = strconv.FormatInt(f.dealerA.ID, 10)
	if _, err := NewUsageExportAdapter(f.admin).Export(f.ctx, q, "tr"); !errors.Is(err, ErrUsageExportScope) {
		t.Fatalf("foreign export err = %v", err)
	}
}

// Acceptance: without an override (NULL) the platform default quota
// applies; 0 is unlimited (no remaining, no percent, no threshold).
func TestQuotaDefaultAndUnlimited(t *testing.T) {
	f := newAdminFixture(t)
	settings, err := f.store.Settings(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.book(t, f.dealerA, &f.userA, model.UsageChannelPanel, 1000, 200)

	s, err := f.admin.Summary(f.ctx, f.dealerA, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.Quota.Limit != settings.DefaultMonthlyTokenQuota || s.Quota.Used != 1200 || s.Quota.Remaining == nil ||
		*s.Quota.Remaining != settings.DefaultMonthlyTokenQuota-1200 || s.Quota.Percent == nil {
		t.Fatalf("default quota summary = %+v", s.Quota)
	}
	rows, _, err := f.admin.ListOrgQuotas(f.ctx, OrgQuotaListFilter{Q: f.suffix, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.QuotaOverride != nil || r.Quota != settings.DefaultMonthlyTokenQuota {
			t.Fatalf("NULL override row = %+v", r)
		}
	}

	zero := int64(0)
	row, err := f.admin.UpdateOrgQuota(f.ctx, f.userA.ID, f.dealerA.Uuid, OrgQuotaInput{MonthlyTokenQuota: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if row.Quota != 0 || row.QuotaOverride == nil || *row.QuotaOverride != 0 || row.Percent != nil || !row.Enabled || row.Used != 1200 {
		t.Fatalf("unlimited row = %+v", row)
	}
	s, err = f.admin.Summary(f.ctx, f.dealerA, "")
	if err != nil || s.Quota.Limit != 0 || s.Quota.Remaining != nil || s.Quota.Percent != nil {
		t.Fatalf("unlimited summary = %+v %v", s.Quota, err)
	}
	if exceeded(0, 1<<40) {
		t.Fatal("0 must never be exceeded")
	}
	if ev := thresholdEvents(0, db.AiUsage{QuotaTokens: 500}, db.AiUsageMonthly{QuotaTokens: 1 << 40}); len(ev) != 0 {
		t.Fatalf("unlimited threshold events = %v", ev)
	}

	// Back to NULL: the default applies again; enabled stays as it was.
	off := false
	if row, err = f.admin.UpdateOrgQuota(f.ctx, f.userA.ID, f.dealerA.Uuid, OrgQuotaInput{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	if row.QuotaOverride != nil || row.Quota != settings.DefaultMonthlyTokenQuota || row.Enabled {
		t.Fatalf("reset row = %+v", row)
	}
	if _, err := f.admin.UpdateOrgQuota(f.ctx, f.userA.ID, f.userA.Uuid, OrgQuotaInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing org = %v", err)
	}
	neg := int64(-1)
	var ve *ValidationError
	if _, err := f.admin.UpdateOrgQuota(f.ctx, f.userA.ID, f.dealerA.Uuid, OrgQuotaInput{MonthlyTokenQuota: &neg}); !errors.As(err, &ve) {
		t.Fatalf("negative quota = %v", err)
	}
}

// Acceptance: the export with the same filter has as many rows as the list
// total (organization and platform reach).
func TestUsageExportMatchesListTotal(t *testing.T) {
	f := newAdminFixture(t)
	f.book(t, f.dealerA, &f.userA, model.UsageChannelPanel, 10, 1)
	f.book(t, f.dealerA, &f.userA2, model.UsageChannelPanel, 20, 2)
	f.book(t, f.dealerA, &f.userA, model.UsageChannelMCP, 30, 3)
	f.book(t, f.dealerA, nil, model.UsageChannelWhatsApp, 40, 4)
	f.book(t, f.dealerB, &f.userB, model.UsageChannelPanel, 50, 5)

	client := map[string]string{"channel": "panel,mcp", "sort": "-tokens", "limit": "1"}
	values := map[string][]string{"channel": {"panel,mcp"}, "sort": {"-tokens"}}
	uq, err := ParseUsageQuery(values, false)
	if err != nil {
		t.Fatal(err)
	}
	_, total, err := f.admin.ListUsage(f.ctx, []int64{f.dealerA.ID}, uq)
	if err != nil || total != 3 {
		t.Fatalf("list total = %d %v", total, err)
	}
	q, err := UsageExportQuery(client, f.dealerA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := q["limit"]; ok {
		t.Fatal("limit must not reach the job query")
	}
	q[ioengine.QueryOrganizationID] = strconv.FormatInt(f.dealerA.ID, 10)
	ds, err := NewUsageExportAdapter(f.admin).Export(f.ctx, q, "tr")
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(ds.Rows)) != total {
		t.Fatalf("export rows = %d, list total = %d", len(ds.Rows), total)
	}
	if ds.Rows[0]["tokens"] != "33" {
		t.Fatalf("export sort: first row = %v", ds.Rows[0])
	}
	// TEC-391: the token range reaches the job query.
	rq, err := UsageExportQuery(map[string]string{"tokens_min": "20", "tokens_max": "35"}, f.dealerA.ID)
	if err != nil || rq["tokens_min"] != "20" || rq["tokens_max"] != "35" {
		t.Fatalf("token range job query = %v %v", rq, err)
	}
	rq[ioengine.QueryOrganizationID] = strconv.FormatInt(f.dealerA.ID, 10)
	if rds, err := NewUsageExportAdapter(f.admin).Export(f.ctx, rq, "tr"); err != nil || len(rds.Rows) != 2 {
		t.Fatalf("token range export rows = %d %v", len(rds.Rows), err)
	}

	// Platform reach with the organization filter.
	pv := map[string][]string{"organization": {f.dealerA.Uuid.String() + "," + f.dealerB.Uuid.String()}}
	puq, err := ParseUsageQuery(pv, true)
	if err != nil {
		t.Fatal(err)
	}
	_, ptotal, err := f.admin.ListUsage(f.ctx, nil, puq)
	if err != nil || ptotal != 5 {
		t.Fatalf("platform total = %d %v", ptotal, err)
	}
	pq, err := UsageExportQuery(map[string]string{"organization": pv["organization"][0]}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pds, err := NewUsageExportAdapter(f.admin).Export(f.ctx, pq, "en")
	if err != nil || int64(len(pds.Rows)) != ptotal {
		t.Fatalf("platform export rows = %d, total = %d, err %v", len(pds.Rows), ptotal, err)
	}
	// A platform job carrying a job organization is refused.
	pq[ioengine.QueryOrganizationID] = strconv.FormatInt(f.dealerA.ID, 10)
	if _, err := NewUsageExportAdapter(f.admin).Export(f.ctx, pq, "en"); !errors.Is(err, ErrUsageExportScope) {
		t.Fatalf("platform job with organization = %v", err)
	}
	// A bad filter is refused at request time.
	if _, err := UsageExportQuery(map[string]string{"channel": "fax"}, f.dealerA.ID); err == nil {
		t.Fatal("bad channel accepted")
	}
}

// List contract of the usage report: sort asc / desc, id tiebreak in the
// sort direction on equal values, multi-value filters, unknown sort 400.
func TestUsageListContract(t *testing.T) {
	f := newAdminFixture(t)
	f.book(t, f.dealerA, &f.userA, model.UsageChannelPanel, 10, 0)
	f.book(t, f.dealerA, &f.userA2, model.UsageChannelMCP, 30, 0)
	f.book(t, f.dealerA, &f.userA, model.UsageChannelPanel, 10, 0)
	f.book(t, f.dealerA, &f.userA, model.UsageChannelTriage, 20, 0)

	list := func(raw string) []UsageRow {
		t.Helper()
		values, _ := urlValues(raw)
		uq, err := ParseUsageQuery(values, false)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		rows, _, err := f.admin.ListUsage(f.ctx, []int64{f.dealerA.ID}, uq)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		return rows
	}
	tokens := func(rows []UsageRow) []int64 {
		var out []int64
		for _, r := range rows {
			out = append(out, r.Tokens)
		}
		return out
	}
	asc := list("sort=tokens")
	if !slices.Equal(tokens(asc), []int64{10, 10, 20, 30}) || asc[0].ID > asc[1].ID {
		t.Fatalf("asc = %v", tokens(asc))
	}
	desc := list("sort=-tokens")
	if !slices.Equal(tokens(desc), []int64{30, 20, 10, 10}) || desc[2].ID < desc[3].ID {
		t.Fatalf("desc = %v", tokens(desc))
	}
	// Same created_at in one transaction: created_at order is the id order.
	byTime := list("sort=created_at")
	for i := 1; i < len(byTime); i++ {
		if byTime[i-1].ID > byTime[i].ID {
			t.Fatalf("created_at tiebreak = %v", byTime)
		}
	}
	def := list("")
	if def[0].ID < def[len(def)-1].ID {
		t.Fatal("default sort must be -created_at")
	}
	if got := list("channel=panel,mcp"); len(got) != 3 {
		t.Fatalf("channel filter = %d", len(got))
	}
	if got := list("user=" + f.userA2.Uuid.String()); len(got) != 1 || got[0].User == nil || got[0].User.UUID != f.userA2.Uuid {
		t.Fatalf("user filter = %+v", got)
	}
	if got := list("user=" + f.dealerB.Uuid.String()); len(got) != 0 {
		t.Fatalf("unknown user must match nothing, got %d", len(got))
	}
	if got := list("purpose=title"); len(got) != 0 {
		t.Fatalf("purpose filter = %d", len(got))
	}
	from := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	if got := list("created_from=" + from); len(got) != 0 {
		t.Fatalf("created_from filter = %d", len(got))
	}
	// TEC-391: tokens_min / tokens_max (inclusive).
	if got := tokens(list("tokens_min=20&sort=tokens")); !slices.Equal(got, []int64{20, 30}) {
		t.Fatalf("tokens_min filter = %v", got)
	}
	if got := tokens(list("tokens_min=10&tokens_max=20&sort=tokens")); !slices.Equal(got, []int64{10, 10, 20}) {
		t.Fatalf("tokens range filter = %v", got)
	}
	var qe *apiquery.ValidationError
	for _, bad := range []string{"sort=-model", "channel=fax", "user=x", "created_from=yesterday",
		"tokens_min=abc", "tokens_min=30&tokens_max=10"} {
		values, _ := urlValues(bad)
		if _, err := ParseUsageQuery(values, false); !errors.As(err, &qe) {
			t.Fatalf("%s: err = %v, want validation error", bad, err)
		}
	}
}

// List contract of the platform quota table: sort usage | quota | name
// (default -usage), org_type and q filters, unlimited sorts as the largest
// quota, id tiebreak on equal usage.
func TestOrgQuotaListContract(t *testing.T) {
	f := newAdminFixture(t)
	brand := f.center.BrandID
	dealerC, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: "tec389-c-" + f.suffix, Name: "TEC389 c " + f.suffix, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "distributor", ParentID: pgtype.Int8{Int64: f.center.ID, Valid: true},
		BrandID: brand, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.book(t, f.dealerB, &f.userB, model.UsageChannelPanel, 500, 0)
	f.book(t, dealerC, nil, model.UsageChannelPanel, 100, 0)
	zero, small := int64(0), int64(1000)
	if _, err := f.admin.UpdateOrgQuota(f.ctx, 0, f.dealerA.Uuid, OrgQuotaInput{MonthlyTokenQuota: &zero}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.UpdateOrgQuota(f.ctx, 0, dealerC.Uuid, OrgQuotaInput{MonthlyTokenQuota: &small}); err != nil {
		t.Fatal(err)
	}
	names := func(sort string, types ...string) []string {
		t.Helper()
		var s []apiquery.SortField
		if sort != "" {
			s = apiquery.ParseSort(sort)
		}
		rows, total, err := f.admin.ListOrgQuotas(f.ctx, OrgQuotaListFilter{Q: f.suffix, OrgTypes: types, Sort: s, Limit: 10})
		if err != nil {
			t.Fatalf("%s: %v", sort, err)
		}
		if int(total) != len(rows) {
			t.Fatalf("%s: total %d rows %d", sort, total, len(rows))
		}
		var out []string
		for _, r := range rows {
			out = append(out, strings.Fields(r.Organization.Name)[1])
		}
		return out
	}
	if got := names(""); !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Fatalf("default -usage = %v", got)
	}
	if got := names("usage"); !slices.Equal(got, []string{"a", "c", "b"}) {
		t.Fatalf("usage = %v", got)
	}
	// a unlimited (largest), b default, c 1000.
	if got := names("quota"); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("quota = %v", got)
	}
	if got := names("-quota"); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("-quota = %v", got)
	}
	if got := names("-name"); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("-name = %v", got)
	}
	if got := names("", "distributor"); !slices.Equal(got, []string{"c"}) {
		t.Fatalf("org_type = %v", got)
	}
	rows, _, err := f.admin.ListOrgQuotas(f.ctx, OrgQuotaListFilter{Q: "tec389-c-" + f.suffix, Limit: 1})
	if err != nil || len(rows) != 1 || rows[0].Used != 100 || rows[0].Percent == nil || *rows[0].Percent != 10 {
		t.Fatalf("percent row = %+v %v", rows, err)
	}
	var qe *apiquery.ValidationError
	if _, _, err := f.admin.ListOrgQuotas(f.ctx, OrgQuotaListFilter{Sort: apiquery.ParseSort("-tokens"), Limit: 1}); !errors.As(err, &qe) {
		t.Fatalf("unknown sort = %v", err)
	}
	var ve *ValidationError
	if _, _, err := f.admin.ListOrgQuotas(f.ctx, OrgQuotaListFilter{Period: "2026-13", Limit: 1}); !errors.As(err, &ve) {
		t.Fatalf("bad period = %v", err)
	}
}

// Settings: models only from the allow list, known tools only, the
// knowledge text at most 20 KB; toggles keep only switched-off tools.
func TestSettingsValidationAndUpdate(t *testing.T) {
	f := newAdminFixture(t)
	var ve *ValidationError
	bad := "claude-opus-4-1"
	if _, err := f.admin.UpdateSettings(f.ctx, f.userA.ID, SettingsInput{DefaultModel: &bad}); !errors.As(err, &ve) || ve.Field != "default_model" {
		t.Fatalf("model outside allow list = %v", err)
	}
	unknown := map[string]bool{"drop_tables": false}
	if _, err := f.admin.UpdateSettings(f.ctx, f.userA.ID, SettingsInput{ToolToggles: &unknown}); !errors.As(err, &ve) || ve.Field != "tool_toggles" {
		t.Fatalf("unknown tool = %v", err)
	}
	big := strings.Repeat("ş", MaxKnowledgeTextBytes/2+1) // 2 bytes each
	if _, err := f.admin.UpdateSettings(f.ctx, f.userA.ID, SettingsInput{KnowledgeText: &big}); !errors.As(err, &ve) || ve.Field != "knowledge_text" {
		t.Fatalf("knowledge text over 20 KB = %v", err)
	}
	neg := int64(-5)
	if _, err := f.admin.UpdateSettings(f.ctx, f.userA.ID, SettingsInput{SystemPoolMonthlyQuota: &neg}); !errors.As(err, &ve) {
		t.Fatalf("negative quota = %v", err)
	}

	fast, quota, kt := "claude-sonnet-5-5", int64(3_000_000), "# Olex\nBilgi metni"
	toggles := map[string]bool{"create_task": false, "list_tasks": true}
	out, err := f.admin.UpdateSettings(f.ctx, f.userA.ID, SettingsInput{
		FastModel: &fast, DefaultMonthlyTokenQuota: &quota, ToolToggles: &toggles, KnowledgeText: &kt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.FastModel != fast || out.DefaultMonthlyTokenQuota != quota || out.KnowledgeText != kt ||
		len(out.ToolToggles) != 1 || out.ToolToggles["create_task"] || out.UpdatedBy == nil || out.UpdatedBy.UUID != f.userA.Uuid {
		t.Fatalf("updated settings = %+v", out)
	}
	if !slices.Equal(out.AllowedModels, []string{"claude-sonnet-5-5", "claude-haiku-4-5"}) || len(out.Tools) != 2 {
		t.Fatalf("allowed / tools = %v %v", out.AllowedModels, out.Tools)
	}
	for _, tl := range out.Tools {
		if tl.Enabled == (tl.Name == "create_task") {
			t.Fatalf("tool %s enabled = %v", tl.Name, tl.Enabled)
		}
	}
	got, err := f.admin.GetSettings(f.ctx)
	if err != nil || got.DefaultMonthlyTokenQuota != quota || got.DefaultModel != out.DefaultModel {
		t.Fatalf("get after update = %+v %v", got, err)
	}
}

// Recipients of ai.quota.threshold: org pool → the organization's
// ai.usage.read holders; system pool → global ai.settings.manage holders.
func TestQuotaNotifyRecipients(t *testing.T) {
	f := newAdminFixture(t)
	add := func(u db.User, role string) {
		m, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{OrganizationID: f.dealerA.ID, UserID: u.ID, Role: "staff"})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.q.AssignMemberRoleBySlug(f.ctx, db.AssignMemberRoleBySlugParams{MemberID: m.ID, Slug: role}); err != nil {
			t.Fatal(err)
		}
	}
	add(f.userA, "dealer_owner")
	add(f.userA2, "dealer_staff")
	if err := f.q.AssignUserRoleBySlug(f.ctx, db.AssignUserRoleBySlugParams{UserID: f.userB.ID, Slug: rbac.RoleSuperAdmin}); err != nil {
		t.Fatal(err)
	}
	ids, err := f.store.QuotaNotifyUserIDs(f.ctx, f.dealerA.ID, model.PoolOrg)
	if err != nil || !slices.Equal(ids, []int64{f.userA.ID}) {
		t.Fatalf("org pool recipients = %v %v", ids, err)
	}
	ids, err = f.store.QuotaNotifyUserIDs(f.ctx, f.center.ID, model.PoolSystem)
	if err != nil || !slices.Contains(ids, f.userB.ID) || slices.Contains(ids, f.userA.ID) {
		t.Fatalf("system pool recipients = %v %v", ids, err)
	}
}

func urlValues(raw string) (url.Values, error) { return url.ParseQuery(raw) }

// Acceptance: the 80 % notification goes out once a month. Crossing 80 %
// writes one event; after the quota is raised mid-month a second crossing
// carries the very same event id, which is the notification delivery
// idempotency key (notification_deliveries (event_id, user_id, channel)),
// so nobody is notified twice. The next month has a new id.
func TestThresholdNotifiedOncePerMonth(t *testing.T) {
	month := func(used int64) db.AiUsageMonthly {
		return db.AiUsageMonthly{OrganizationID: 7, BrandID: 1, Pool: model.PoolOrg, Period: "2026-10", QuotaTokens: used}
	}
	first := thresholdEvents(100, db.AiUsage{QuotaTokens: 85}, month(85))
	if len(first) != 1 || first[0].Payload["threshold"] != int64(80) {
		t.Fatalf("first crossing = %v", first)
	}
	if again := thresholdEvents(100, db.AiUsage{QuotaTokens: 5}, month(90)); len(again) != 0 {
		t.Fatalf("no new crossing must write nothing, got %v", again)
	}
	// Quota raised to 200: 85 → 170 crosses 80 % (160) again.
	second := thresholdEvents(200, db.AiUsage{QuotaTokens: 85}, month(170))
	if len(second) != 1 || second[0].EventID != first[0].EventID {
		t.Fatalf("re-crossing id %v, want %v", second, first[0].EventID)
	}
	if ThresholdEventID(7, model.PoolOrg, "2026-11", 80) == first[0].EventID ||
		ThresholdEventID(7, model.PoolOrg, "2026-10", 100) == first[0].EventID ||
		ThresholdEventID(7, model.PoolSystem, "2026-10", 80) == first[0].EventID {
		t.Fatal("event id must differ per month, threshold and pool")
	}
}
