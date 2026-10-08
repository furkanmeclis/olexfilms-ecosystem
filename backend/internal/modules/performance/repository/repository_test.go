package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testPeriod = "2026-09"

type fixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	store    *Store
	prefix   string
	seq      int
	brandID  int64
	centerID int64
}

func newFixture(t *testing.T) *fixture {
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

	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	return &fixture{
		ctx: ctx, tx: tx, q: q, store: FromQueries(q),
		prefix:  fmt.Sprintf("t490-%d", time.Now().UnixNano()),
		brandID: brand.ID, centerID: center.ID,
	}
}

func (f *fixture) org(t *testing.T, typ, name string, parent int64) db.Organization {
	t.Helper()
	f.seq++
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("%s-%s-%d", f.prefix, typ, f.seq), Name: f.prefix + " " + name,
		Status: "active", Type: typ, ParentID: pgtype.Int8{Int64: parent, Valid: true}, BrandID: f.brandID,
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func (f *fixture) user(t *testing.T, name string) db.User {
	t.Helper()
	f.seq++
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: name, Surname: "T490", Status: "active",
		Email: pgtype.Text{String: fmt.Sprintf("%s-%s-%d@example.test", f.prefix, name, f.seq), Valid: true},
	})
	if err != nil {
		t.Fatalf("user %s: %v", name, err)
	}
	return u
}

func (f *fixture) metric(t *testing.T, orgID int64, period, metric, value string) {
	t.Helper()
	arg := db.UpsertPerformanceMetricParams{
		OrganizationID: orgID, BrandID: f.brandID, Period: period, Scope: "org", Metric: metric, Value: numeric(t, value),
	}
	for _, m := range model.MoneyMetrics {
		if m == metric {
			arg.Currency = pgtype.Text{String: "TRY", Valid: true}
		}
	}
	if _, err := f.q.UpsertPerformanceMetric(f.ctx, arg); err != nil {
		t.Fatalf("metric %s=%s: %v", metric, value, err)
	}
}

func numeric(t *testing.T, raw string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(raw); err != nil {
		t.Fatalf("numeric %s: %v", raw, err)
	}
	return n
}

func nstr(t *testing.T, n pgtype.Numeric) string {
	t.Helper()
	if !n.Valid {
		return "null"
	}
	f, err := n.Float64Value()
	if err != nil {
		t.Fatalf("numeric: %v", err)
	}
	return fmt.Sprintf("%.2f", f.Float64)
}

func ids(rows []db.ListPerformanceRankingRow) []int64 {
	out := make([]int64, len(rows))
	for i, r := range rows {
		out[i] = r.OrganizationID
	}
	return out
}

func sameIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Every ranking sort key (12 metrics + name) in both directions: equal
// values fall back to id (asc: id asc, desc: id desc) and organizations
// without a computed value sort last in both directions.
func TestRankingSortEveryMetricAndTiebreak(t *testing.T) {
	f := newFixture(t)
	// a and c share the value (and name for the name key); b is lower; d
	// has no metric row at all.
	a := f.org(t, "dealer", "same", f.centerID)
	b := f.org(t, "dealer", "alpha", f.centerID)
	c := f.org(t, "dealer", "same", f.centerID)
	d := f.org(t, "dealer", "zulu", f.centerID)
	for _, m := range model.Metrics {
		f.metric(t, a.ID, testPeriod, m, "5")
		f.metric(t, b.ID, testPeriod, m, "1.5")
		f.metric(t, c.ID, testPeriod, m, "5")
		// Another month must not leak into the ranking.
		f.metric(t, d.ID, "2026-08", m, "99")
	}
	scope := []int64{a.ID, b.ID, c.ID, d.ID}

	keys := append([]string{"name"}, model.Metrics...)
	if len(RankingSort.Columns) != len(keys) {
		t.Fatalf("ranking sort columns = %d, want %d", len(RankingSort.Columns), len(keys))
	}
	for _, key := range keys {
		wantAsc := []int64{b.ID, a.ID, c.ID, d.ID}
		wantDesc := []int64{c.ID, a.ID, b.ID, d.ID}
		if key == "name" {
			// alpha < same(a,c) < zulu; d is not NULL-last for names.
			wantDesc = []int64{d.ID, c.ID, a.ID, b.ID}
		}
		for _, desc := range []bool{false, true} {
			rows, err := f.store.ListRanking(f.ctx, db.ListPerformanceRankingParams{
				BrandID: f.brandID, Period: testPeriod, OrgIds: scope,
			}, []apiquery.SortField{{Field: key, Desc: desc}})
			if err != nil {
				t.Fatalf("%s desc=%v: %v", key, desc, err)
			}
			want := wantAsc
			if desc {
				want = wantDesc
			}
			if got := ids(rows); !sameIDs(got, want) {
				t.Fatalf("%s desc=%v order = %v, want %v", key, desc, got, want)
			}
			if rows[0].TotalCount != 4 {
				t.Fatalf("%s total = %d", key, rows[0].TotalCount)
			}
		}
	}

	// Default sort is -services_count.
	rows, err := f.store.ListRanking(f.ctx, db.ListPerformanceRankingParams{
		BrandID: f.brandID, Period: testPeriod, OrgIds: scope,
	}, nil)
	if err != nil {
		t.Fatalf("default sort: %v", err)
	}
	if got := ids(rows); !sameIDs(got, []int64{c.ID, a.ID, b.ID, d.ID}) {
		t.Fatalf("default order = %v", got)
	}
	if nstr(t, rows[0].ServicesCount) != "5.00" || rows[3].ServicesCount.Valid {
		t.Fatalf("services_count columns = %s / %s", nstr(t, rows[0].ServicesCount), nstr(t, rows[3].ServicesCount))
	}

	// Unknown key is a list-contract validation error.
	if _, err := f.store.ListRanking(f.ctx, db.ListPerformanceRankingParams{
		BrandID: f.brandID, Period: testPeriod, OrgIds: scope,
	}, []apiquery.SortField{{Field: "bogus"}}); err == nil {
		t.Fatal("unknown sort key accepted")
	}
}

// q, org_type, distributor (the distributor and its dealers) and province
// filters are multi-valued and combine with the scope.
func TestRankingFilters(t *testing.T) {
	f := newFixture(t)
	dist := f.org(t, "distributor", "dist", f.centerID)
	under := f.org(t, "dealer", "under", dist.ID)
	direct := f.org(t, "dealer", "direct", f.centerID)
	var provinceID, countryID int64
	if err := f.tx.QueryRow(f.ctx, `SELECT id, country_id FROM provinces ORDER BY id LIMIT 1`).Scan(&provinceID, &countryID); err != nil {
		t.Fatalf("province: %v", err)
	}
	if _, err := f.tx.Exec(f.ctx, `UPDATE organizations SET country_id = $2, province_id = $3 WHERE id = $1`,
		direct.ID, countryID, provinceID); err != nil {
		t.Fatalf("set province: %v", err)
	}
	scope := []int64{dist.ID, under.ID, direct.ID}
	list := func(arg db.ListPerformanceRankingParams) []int64 {
		t.Helper()
		arg.BrandID, arg.Period, arg.OrgIds = f.brandID, testPeriod, scope
		rows, err := f.store.ListRanking(f.ctx, arg, []apiquery.SortField{{Field: "name"}})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		return ids(rows)
	}
	if got := list(db.ListPerformanceRankingParams{DistributorIds: []int64{dist.ID}}); !sameIDs(got, []int64{dist.ID, under.ID}) {
		t.Fatalf("distributor filter = %v", got)
	}
	if got := list(db.ListPerformanceRankingParams{OrgTypes: []string{"dealer"}}); !sameIDs(got, []int64{direct.ID, under.ID}) {
		t.Fatalf("org_type filter = %v", got)
	}
	if got := list(db.ListPerformanceRankingParams{ProvinceIds: []int64{provinceID}}); !sameIDs(got, []int64{direct.ID}) {
		t.Fatalf("province filter = %v", got)
	}
	if got := list(db.ListPerformanceRankingParams{Q: pgtype.Text{String: " UNDER ", Valid: true}}); !sameIDs(got, []int64{under.ID}) {
		t.Fatalf("q filter = %v", got)
	}
}

// Target achievement sums the metric over the months of the target period
// (same currency for order volume); a distributor sets targets only below
// itself.
func TestTargetsAchievementAndOwnership(t *testing.T) {
	f := newFixture(t)
	dist := f.org(t, "distributor", "dist", f.centerID)
	dealer := f.org(t, "dealer", "dealer", dist.ID)
	other := f.org(t, "dealer", "other", f.centerID)

	f.metric(t, dealer.ID, "2026-07", model.MetricServicesCount, "4")
	f.metric(t, dealer.ID, "2026-08", model.MetricServicesCount, "6")
	f.metric(t, dealer.ID, "2026-09", model.MetricServicesCount, "5")
	f.metric(t, dealer.ID, "2026-10", model.MetricServicesCount, "100") // next quarter
	f.metric(t, dealer.ID, "2026-07", model.MetricOrderVolume, "1000")

	create := func(owner, target int64, metric, kind, start, value string, currency string) (db.PerformanceTarget, error) {
		st, _ := time.Parse("2006-01-02", start)
		arg := db.CreatePerformanceTargetParams{
			OrganizationID: owner, BrandID: f.brandID, TargetOrgID: target, Metric: metric,
			PeriodKind: kind, PeriodStart: pgtype.Date{Time: st, Valid: true}, Value: numeric(t, value),
		}
		if currency != "" {
			arg.Currency = pgtype.Text{String: currency, Valid: true}
		}
		return f.q.CreatePerformanceTarget(f.ctx, arg)
	}
	monthly, err := create(dist.ID, dealer.ID, model.MetricServicesCount, model.PeriodMonthly, "2026-07-01", "8", "")
	if err != nil {
		t.Fatalf("monthly: %v", err)
	}
	quarterly, err := create(f.centerID, dealer.ID, model.MetricServicesCount, model.PeriodQuarterly, "2026-07-01", "30", "")
	if err != nil {
		t.Fatalf("quarterly: %v", err)
	}
	volumeEUR, err := create(f.centerID, dealer.ID, model.MetricOrderVolume, model.PeriodMonthly, "2026-07-01", "500", "EUR")
	if err != nil {
		t.Fatalf("volume: %v", err)
	}

	rows, err := f.store.ListTargets(f.ctx, db.ListPerformanceTargetsParams{
		BrandID: f.brandID, TargetOrgIds: []int64{dealer.ID},
	}, []apiquery.SortField{{Field: "achievement_pct", Desc: true}})
	if err != nil {
		t.Fatalf("list targets: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("targets = %d", len(rows))
	}
	got := map[int64][2]string{}
	order := []int64{}
	for _, r := range rows {
		got[r.ID] = [2]string{nstr(t, r.Actual), nstr(t, r.AchievementPct)}
		order = append(order, r.ID)
	}
	if got[monthly.ID] != [2]string{"4.00", "50.00"} || got[quarterly.ID] != [2]string{"15.00", "50.00"} {
		t.Fatalf("achievement = %v", got)
	}
	// TRY volume does not count against a EUR target.
	if got[volumeEUR.ID] != [2]string{"null", "null"} {
		t.Fatalf("currency mismatch counted: %v", got[volumeEUR.ID])
	}
	if order[2] != volumeEUR.ID || order[0] != quarterly.ID {
		t.Fatalf("achievement desc order (tiebreak id desc, nulls last) = %v", order)
	}

	if err := f.try(t, func() error {
		_, err := create(dist.ID, other.ID, model.MetricServicesCount, model.PeriodMonthly, "2026-07-01", "5", "")
		return err
	}); !isCode(err, "23514") {
		t.Fatalf("distributor target outside subtree err = %v", err)
	}
	if err := f.try(t, func() error {
		_, err := create(f.centerID, dealer.ID, model.MetricServicesCount, model.PeriodQuarterly, "2026-08-01", "5", "")
		return err
	}); !isCode(err, "23514") {
		t.Fatalf("misaligned quarter err = %v", err)
	}
	if err := f.try(t, func() error {
		_, err := create(f.centerID, dealer.ID, model.MetricServicesCount, model.PeriodQuarterly, "2026-07-01", "9", "")
		return err
	}); !isCode(err, "23505") {
		t.Fatalf("duplicate target err = %v", err)
	}
}

// Only one automatic task per subject x rule x month: the second insert
// violates uq_tasks_auto; another month is allowed.
func TestAutoTaskUniquePerRuleAndMonth(t *testing.T) {
	f := newFixture(t)
	dealer := f.org(t, "dealer", "weak", f.centerID)
	rule, err := f.q.CreateWeakDealerRule(f.ctx, db.CreateWeakDealerRuleParams{
		OrganizationID: f.centerID, BrandID: f.brandID, Name: "target < 50%",
		Metric: model.RuleMetricTargetAchievement, Operator: model.OpLT, Threshold: numeric(t, "50"),
		CreateTask: true, Notify: true, Active: true,
	})
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	insert := func(period string) error {
		_, err := f.q.InsertAutoPerformanceTask(f.ctx, db.InsertAutoPerformanceTaskParams{
			OrganizationID: f.centerID, BrandID: f.brandID, SubjectOrgID: dealer.ID,
			Title: "Weak dealer", Priority: "normal", AutoRuleID: pgtype.Int8{Int64: rule.ID, Valid: true},
			AutoPeriod: pgtype.Text{String: period, Valid: true},
		})
		return err
	}
	if err := insert(testPeriod); err != nil {
		t.Fatalf("first auto task: %v", err)
	}
	err = f.try(t, func() error { return insert(testPeriod) })
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "uq_tasks_auto" {
		t.Fatalf("second auto task err = %v", err)
	}
	if err := insert("2026-10"); err != nil {
		t.Fatalf("next month auto task: %v", err)
	}
	// The rule that opened tasks is not deletable (deactivate instead).
	if n, err := f.q.DeleteWeakDealerRule(f.ctx, db.DeleteWeakDealerRuleParams{ID: rule.ID, BrandID: f.brandID}); err != nil || n != 0 {
		t.Fatalf("delete rule with tasks = %d, %v", n, err)
	}
}

// Distributor rules only notify; a center rule's assignee is a center
// member.
func TestWeakDealerRuleOwnerGuards(t *testing.T) {
	f := newFixture(t)
	dist := f.org(t, "distributor", "dist", f.centerID)
	outsider := f.user(t, "outsider")
	cases := []struct {
		name string
		arg  db.CreateWeakDealerRuleParams
	}{
		{"distributor task", db.CreateWeakDealerRuleParams{OrganizationID: dist.ID, CreateTask: true, Notify: true}},
		{"assignee not member", db.CreateWeakDealerRuleParams{
			OrganizationID: f.centerID, CreateTask: true,
			AssigneeUserID: pgtype.Int8{Int64: outsider.ID, Valid: true},
		}},
	}
	for _, tc := range cases {
		tc.arg.BrandID, tc.arg.Name, tc.arg.Active = f.brandID, "r", true
		tc.arg.Metric, tc.arg.Operator, tc.arg.Threshold = model.MetricServicesCount, model.OpBelowMedianPct, numeric(t, "30")
		if err := f.try(t, func() error {
			_, err := f.q.CreateWeakDealerRule(f.ctx, tc.arg)
			return err
		}); !isCode(err, "23514") {
			t.Fatalf("%s err = %v", tc.name, err)
		}
	}
	if _, err := f.q.CreateWeakDealerRule(f.ctx, db.CreateWeakDealerRuleParams{
		OrganizationID: dist.ID, BrandID: f.brandID, Name: "notify", Metric: model.MetricServicesCount,
		Operator: model.OpBelowMedianPct, Threshold: numeric(t, "30"), Notify: true, Active: true,
	}); err != nil {
		t.Fatalf("distributor notify rule: %v", err)
	}
}

// Recalculating a month refreshes a calculated accrual and leaves an
// approved one untouched.
func TestBonusAccrualRecalculation(t *testing.T) {
	f := newFixture(t)
	dealer := f.org(t, "dealer", "bonus", f.centerID)
	staff := f.user(t, "staff")
	if _, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{
		OrganizationID: dealer.ID, UserID: staff.ID, Role: "staff",
	}); err != nil {
		t.Fatalf("member: %v", err)
	}
	if _, err := f.q.UpsertStaffTarget(f.ctx, db.UpsertStaffTargetParams{
		OrganizationID: dealer.ID, BrandID: f.brandID, UserID: staff.ID, Period: testPeriod,
		Metric: model.MetricServicesCount, Value: numeric(t, "20"),
	}); err != nil {
		t.Fatalf("staff target: %v", err)
	}
	rule, err := f.q.CreateBonusRule(f.ctx, db.CreateBonusRuleParams{
		OrganizationID: dealer.ID, BrandID: f.brandID, Name: "100%", Metric: model.MetricServicesCount,
		ThresholdPct: numeric(t, "100"), Kind: model.BonusFixed, Amount: numeric(t, "1000"),
		Currency: pgtype.Text{String: "TRY", Valid: true}, Active: true,
	})
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	upsert := func(pct, amount string) (db.BonusAccrual, error) {
		return f.q.UpsertBonusAccrual(f.ctx, db.UpsertBonusAccrualParams{
			OrganizationID: dealer.ID, BrandID: f.brandID, UserID: staff.ID, Period: testPeriod,
			RuleID: rule.ID, AchievementPct: numeric(t, pct), Amount: numeric(t, amount), Currency: "TRY",
		})
	}
	first, err := upsert("100", "1000")
	if err != nil {
		t.Fatalf("accrual: %v", err)
	}
	second, err := upsert("110", "1000")
	if err != nil || second.ID != first.ID || nstr(t, second.AchievementPct) != "110.00" {
		t.Fatalf("recalc = %+v, %v", second, err)
	}
	if _, err := f.q.ApproveBonusAccrual(f.ctx, db.ApproveBonusAccrualParams{ID: first.ID, OrganizationID: dealer.ID}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := upsert("120", "1000"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("recalc after approval err = %v", err)
	}
}

// try runs fn inside a savepoint so an expected database error does not
// abort the test transaction.
func (f *fixture) try(t *testing.T, fn func() error) error {
	t.Helper()
	if _, err := f.tx.Exec(f.ctx, "SAVEPOINT try"); err != nil {
		t.Fatal(err)
	}
	err := fn()
	stmt := "RELEASE SAVEPOINT try"
	if err != nil {
		stmt = "ROLLBACK TO SAVEPOINT try"
	}
	if _, xerr := f.tx.Exec(f.ctx, stmt); xerr != nil {
		t.Fatal(xerr)
	}
	return err
}

func isCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
