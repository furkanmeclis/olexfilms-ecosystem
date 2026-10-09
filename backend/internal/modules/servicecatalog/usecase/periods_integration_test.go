package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-308 acceptance: the daily subscription job on a fake clock. Ledger
// rows are written only through accounting/posting; the tests read them.

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

// fakeRates answers fixed rates per "BASE/QUOTE"; set changes them.
type fakeRates struct {
	mu    sync.Mutex
	rates map[string]string
}

func (f *fakeRates) set(pair, rate string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rates[pair] = rate
}

func (f *fakeRates) ResolveRate(_ context.Context, on time.Time, base, quote string) (fxrates.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if base == quote {
		return fxrates.Snapshot{Base: base, Quote: quote, Rate: "1", RateDate: on.Format(time.DateOnly), Source: "identity"}, nil
	}
	r, ok := f.rates[base+"/"+quote]
	if !ok {
		return fxrates.Snapshot{}, fxrates.ErrRateNotFound
	}
	return fxrates.Snapshot{Base: base, Quote: quote, Rate: r, RateDate: on.Format(time.DateOnly), Source: "test"}, nil
}

// jobSvc is the worker wiring (outbox, rates, features, posting) on a fixed
// clock.
func (e *subEnv) jobSvc(rates RateResolver, today time.Time) *Service {
	out := outbox.NewStore(e.pool, e.q)
	return New(e.q).WithLifecycle(e.pool, out, rates, e.features).
		WithAccounting(posting.New(e.q, out, rates)).
		WithClock(func() time.Time { return today.Add(9 * time.Hour) })
}

func (e *subEnv) orgWithCurrency(t *testing.T, name, typ string, parent int64, currency string) db.Organization {
	t.Helper()
	o, err := e.q.CreateOrganization(e.ctx, db.CreateOrganizationParams{
		Slug: "t308-" + name + "-" + e.suffix, Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: e.center.BrandID, Currency: currency, Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func (e *subEnv) itemIn(t *testing.T, category, price, currency, recurrence, fee string) db.ServiceCatalogItem {
	t.Helper()
	it, err := e.q.CreateServiceCatalogItem(e.ctx, db.CreateServiceCatalogItemParams{
		OrganizationID: e.center.ID, BrandID: e.center.BrandID, Name: "T308 " + category + " " + uuid.NewString(),
		Category: category, DefaultPrice: num(price), Currency: currency, Recurrence: recurrence,
		CancellationFee: num(fee), IsActive: true,
	})
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if category == CategoryModuleBundle {
		if err := e.q.AddServiceCatalogModule(e.ctx, db.AddServiceCatalogModuleParams{
			ItemID: it.ID, ModuleKey: features.ModuleStockForecast,
		}); err != nil {
			t.Fatalf("bundle module: %v", err)
		}
	}
	return it
}

func (e *subEnv) assign(t *testing.T, svc *Service, item db.ServiceCatalogItem, to db.Organization, start, end string) db.ServiceSubscription {
	t.Helper()
	v, err := svc.Assign(e.ctx, e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsAssign), SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: to.Uuid, StartsOn: day(start), EndsOn: day(end),
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	return e.sub(t, v.UUID)
}

func (e *subEnv) sub(t *testing.T, id uuid.UUID) db.ServiceSubscription {
	t.Helper()
	sub, err := e.q.GetServiceSubscriptionByUUID(e.ctx, db.GetServiceSubscriptionByUUIDParams{Uuid: id, BrandID: e.center.BrandID})
	if err != nil {
		t.Fatalf("subscription: %v", err)
	}
	return sub
}

func (e *subEnv) periods(t *testing.T, sub db.ServiceSubscription) []db.ServiceSubscriptionPeriod {
	t.Helper()
	ps, err := e.q.ListServiceSubscriptionPeriods(e.ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	return ps
}

type ledgerRow struct {
	Org                      int64
	Direction, Category      string
	Currency, Amount         string
	OrigCurrency, OrigAmount string
	Counterparty             int64
	SourceUUID               uuid.UUID
}

// ledger reads the open rows of the given sources, in write order.
func (e *subEnv) ledger(t *testing.T, sourceType string, sources ...uuid.UUID) []ledgerRow {
	t.Helper()
	rows, err := e.pool.Query(e.ctx, `
		SELECT f.organization_id, f.direction, f.category, f.currency, f.amount::text,
		       f.orig_currency, f.orig_amount::text, COALESCE(c.counterparty_org_id, 0), f.source_uuid
		FROM finance_entries f
		LEFT JOIN cari_accounts c ON c.id = f.cari_id
		WHERE f.source_type = $1 AND f.source_uuid = ANY($2) AND f.reversal_of_id IS NULL
		ORDER BY f.id`, sourceType, sources)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []ledgerRow
	for rows.Next() {
		var r ledgerRow
		if err := rows.Scan(&r.Org, &r.Direction, &r.Category, &r.Currency, &r.Amount,
			&r.OrigCurrency, &r.OrigAmount, &r.Counterparty, &r.SourceUUID); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func periodUUIDs(ps []db.ServiceSubscriptionPeriod) []uuid.UUID {
	out := make([]uuid.UUID, len(ps))
	for i, p := range ps {
		out[i] = p.Uuid
	}
	return out
}

// outboxEvents returns the payload data of the subscription's events.
func (e *subEnv) outboxEvents(t *testing.T, name string, sub db.ServiceSubscription) []map[string]any {
	t.Helper()
	rows, err := e.pool.Query(e.ctx, `
		SELECT payload->'data' FROM outbox_events
		WHERE event_name = $1 AND payload->'data'->>'subscription_uuid' = $2
		ORDER BY id`, name, sub.Uuid.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

// runDaily runs the job; failures of other tests' leftover subscriptions
// in the shared database are only logged, the assertions read this test's
// rows.
func runDaily(t *testing.T, svc *Service, on string) {
	t.Helper()
	if _, err := svc.RunDailyOn(context.Background(), day(on)); err != nil {
		t.Logf("daily job on %s: %v", on, err)
	}
}

func TestDuePeriods(t *testing.T) {
	cases := []struct {
		name, rec, start, end, today string
		want                         [][2]string
	}{
		{"monthly three months", "monthly", "2026-07-01", "2027-07-01", "2026-09-15",
			[][2]string{{"2026-07-01", "2026-07-31"}, {"2026-08-01", "2026-08-31"}, {"2026-09-01", "2026-09-30"}}},
		{"not started", "monthly", "2026-07-01", "2027-07-01", "2026-06-30", nil},
		{"month end clamp", "monthly", "2026-01-31", "2026-04-15", "2026-12-31",
			[][2]string{{"2026-01-31", "2026-02-27"}, {"2026-02-28", "2026-03-30"}, {"2026-03-31", "2026-04-15"}}},
		{"ends_on is the boundary", "monthly", "2026-07-04", "2026-09-04", "2026-12-31",
			[][2]string{{"2026-07-04", "2026-08-03"}, {"2026-08-04", "2026-09-03"}}},
		{"yearly", "yearly", "2026-07-01", "2028-07-01", "2027-07-01",
			[][2]string{{"2026-07-01", "2027-06-30"}, {"2027-07-01", "2028-06-30"}}},
		{"one time", "one_time", "2026-07-01", "2026-12-31", "2026-07-01",
			[][2]string{{"2026-07-01", "2026-12-31"}}},
	}
	for _, c := range cases {
		got := DuePeriods(c.rec, day(c.start), day(c.end), day(c.today))
		if len(got) != len(c.want) {
			t.Fatalf("%s: %d periods %v, want %v", c.name, len(got), got, c.want)
		}
		for i, p := range got {
			if p.Start.Format(time.DateOnly) != c.want[i][0] || p.End.Format(time.DateOnly) != c.want[i][1] {
				t.Fatalf("%s: period %d = %s..%s, want %v", c.name, i, p.Start.Format(time.DateOnly), p.End.Format(time.DateOnly), c.want[i])
			}
		}
	}
}

// A monthly subscription advanced three months has three periods and one
// row per period on each ledger; a second run writes nothing.
func TestMonthlyPeriodsPostedOnBothLedgersOnce(t *testing.T) {
	e := newSubEnv(t)
	svc := e.jobSvc(fxrates.New(e.q, nil, nil), day("2026-07-01"))
	item := e.itemIn(t, "software", "100.00", "TRY", "monthly", "25.00")
	sub := e.assign(t, svc, item, e.dist, "2026-07-01", "2027-07-01")
	if sub.Status != StatusActive {
		t.Fatalf("status = %s, want active", sub.Status)
	}

	runDaily(t, svc, "2026-07-01")
	runDaily(t, svc, "2026-09-15")
	ps := e.periods(t, sub)
	if len(ps) != 3 {
		t.Fatalf("periods = %d, want 3", len(ps))
	}
	for i, p := range ps {
		if !p.PostedAt.Valid {
			t.Fatalf("period %d not posted", i)
		}
	}
	rows := e.ledger(t, SourceSubscriptionPeriod, periodUUIDs(ps)...)
	if len(rows) != 6 {
		t.Fatalf("ledger rows = %d, want 6 (%+v)", len(rows), rows)
	}
	for _, p := range ps {
		var seller, buyer int
		for _, r := range rows {
			if r.SourceUUID != p.Uuid {
				continue
			}
			switch {
			case r.Org == e.center.ID && r.Direction == "income" && r.Category == accounting.CategoryServiceSale && r.Counterparty == e.dist.ID:
				seller++
			case r.Org == e.dist.ID && r.Direction == "expense" && r.Category == accounting.CategoryServicePurchase && r.Counterparty == e.center.ID:
				buyer++
			default:
				t.Fatalf("unexpected row %+v", r)
			}
			if r.Amount != "100.00" || r.OrigAmount != "100.00" {
				t.Fatalf("row amount = %+v", r)
			}
		}
		if seller != 1 || buyer != 1 {
			t.Fatalf("period %s: seller=%d buyer=%d, want 1/1", p.PeriodStart.Time.Format(time.DateOnly), seller, buyer)
		}
	}

	runDaily(t, svc, "2026-09-15")
	if n, err := svc.PostDuePeriods(e.ctx, day("2026-09-15")); err != nil && n != 0 {
		t.Logf("post again: %d, %v", n, err)
	}
	if got := e.periods(t, sub); len(got) != 3 {
		t.Fatalf("periods after rerun = %d, want 3", len(got))
	}
	if got := e.ledger(t, SourceSubscriptionPeriod, periodUUIDs(ps)...); len(got) != 6 {
		t.Fatalf("ledger rows after rerun = %d, want 6", len(got))
	}
}

// EUR price, UAH receiver: every period converts at the rate frozen at
// assignment, even after the market rate moved.
func TestPeriodPostingUsesFrozenRate(t *testing.T) {
	e := newSubEnv(t)
	rates := &fakeRates{rates: map[string]string{"EUR/UAH": "45.5", "EUR/TRY": "38.25"}}
	svc := e.jobSvc(rates, day("2026-07-01"))
	uah := e.orgWithCurrency(t, "uah", "distributor", e.center.ID, "UAH")
	item := e.itemIn(t, "software", "100.00", "EUR", "monthly", "0.00")
	sub := e.assign(t, svc, item, uah, "2026-07-01", "2027-07-01")
	if sub.Currency != "EUR" {
		t.Fatalf("subscription currency = %s, want EUR", sub.Currency)
	}

	runDaily(t, svc, "2026-07-01")
	rates.set("EUR/UAH", "50")
	rates.set("EUR/TRY", "40")
	runDaily(t, svc, "2026-08-01")

	ps := e.periods(t, sub)
	if len(ps) != 2 {
		t.Fatalf("periods = %d, want 2", len(ps))
	}
	rows := e.ledger(t, SourceSubscriptionPeriod, periodUUIDs(ps)...)
	if len(rows) != 4 {
		t.Fatalf("ledger rows = %d, want 4", len(rows))
	}
	for _, r := range rows {
		if r.OrigCurrency != "EUR" || r.OrigAmount != "100.00" {
			t.Fatalf("original amount = %+v, want 100.00 EUR", r)
		}
		switch r.Org {
		case uah.ID:
			if r.Currency != "UAH" || r.Amount != "4550.00" {
				t.Fatalf("receiver row = %+v, want 4550.00 UAH", r)
			}
		case e.center.ID:
			if r.Currency != "TRY" || r.Amount != "3825.00" {
				t.Fatalf("seller row = %+v, want 3825.00 TRY", r)
			}
		default:
			t.Fatalf("unexpected row %+v", r)
		}
	}
}

// Past ends_on the subscription expires, its remaining periods are booked,
// the module bundle closes and the receiver is notified.
func TestExpiryClosesModuleBundle(t *testing.T) {
	e := newSubEnv(t)
	svc := e.jobSvc(fxrates.New(e.q, nil, nil), day("2026-07-01"))
	item := e.itemIn(t, CategoryModuleBundle, "50.00", "TRY", "monthly", "0.00")
	sub := e.assign(t, svc, item, e.dist, "2026-07-01", "2026-08-15")
	if ok, err := e.features.Enabled(e.ctx, e.dist.ID, features.ModuleStockForecast); err != nil || !ok {
		t.Fatalf("module after assignment = %v, %v", ok, err)
	}

	runDaily(t, svc, "2026-08-15")
	if got := e.sub(t, sub.Uuid); got.Status != StatusActive {
		t.Fatalf("on ends_on status = %s, want active", got.Status)
	}
	runDaily(t, svc, "2026-08-16")
	got := e.sub(t, sub.Uuid)
	if got.Status != StatusExpired || !got.ExpiredAt.Valid {
		t.Fatalf("status = %s, want expired", got.Status)
	}
	if ok, err := e.features.Enabled(e.ctx, e.dist.ID, features.ModuleStockForecast); err != nil || ok {
		t.Fatalf("module after expiry = %v, %v; want closed", ok, err)
	}
	if ps := e.periods(t, sub); len(ps) != 2 || !ps[1].PostedAt.Valid || ps[1].PeriodEnd.Time.Format(time.DateOnly) != "2026-08-15" {
		t.Fatalf("periods = %+v, want two posted, last ending 2026-08-15", ps)
	}
	evs := e.outboxEvents(t, events.ServiceSubscriptionExpired, sub)
	if len(evs) != 1 || evs[0]["notify_user_ids"] == nil {
		t.Fatalf("expired events = %+v, want one with recipients", evs)
	}
	runDaily(t, svc, "2026-08-17")
	if evs := e.outboxEvents(t, events.ServiceSubscriptionExpired, sub); len(evs) != 1 {
		t.Fatalf("expired events after rerun = %d, want 1", len(evs))
	}
}

// After the cancellation is approved the fee lands on both ledgers once
// and no later period is written.
func TestCancellationFeeBookedAndPeriodsStop(t *testing.T) {
	e := newSubEnv(t)
	svc := e.jobSvc(fxrates.New(e.q, nil, nil), day("2026-07-01"))
	item := e.itemIn(t, "software", "100.00", "TRY", "monthly", "25.00")
	sub := e.assign(t, svc, item, e.dist, "2026-07-01", "2027-07-01")
	runDaily(t, svc, "2026-07-01")

	req, err := svc.RequestCancel(e.ctx, e.caller(e.dist, rbac.ScopeManaged, rbac.PermServiceSubscriptionsCancelRequest),
		sub.Uuid, CancelRequestInput{Reason: "not needed"})
	if err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	if _, err := svc.ApproveCancel(e.ctx, e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsCancelApprove), req.UUID, DecisionInput{}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	evs := e.outboxEvents(t, events.ServiceSubscriptionCancelled, sub)
	if len(evs) != 1 || evs[0]["cancel_request_uuid"] != req.UUID.String() {
		t.Fatalf("cancelled events = %+v, want one with the request uuid", evs)
	}
	ev := events.New(events.ServiceSubscriptionCancelled).WithPayload(evs[0])
	for i := 0; i < 2; i++ {
		if err := svc.HandleSubscriptionCancelled(e.ctx, ev); err != nil {
			t.Fatalf("handle cancelled (%d): %v", i, err)
		}
	}
	rows := e.ledger(t, SourceSubscriptionCancel, req.UUID)
	if len(rows) != 2 {
		t.Fatalf("fee rows = %d, want 2 (%+v)", len(rows), rows)
	}
	for _, r := range rows {
		ok := r.Category == accounting.CategoryServiceCancellationFee && r.Amount == "25.00" &&
			((r.Org == e.center.ID && r.Direction == "income" && r.Counterparty == e.dist.ID) ||
				(r.Org == e.dist.ID && r.Direction == "expense" && r.Counterparty == e.center.ID))
		if !ok {
			t.Fatalf("fee row = %+v", r)
		}
	}

	runDaily(t, svc, "2026-09-15")
	if ps := e.periods(t, sub); len(ps) != 1 {
		t.Fatalf("periods after cancellation = %d, want 1", len(ps))
	}
}

// Decision 1: a subscription grant never destroys the manual value.
func TestSubscriptionKeepsManualModuleValue(t *testing.T) {
	e := newSubEnv(t)
	svc := e.jobSvc(fxrates.New(e.q, nil, nil), day("2026-07-01"))
	key := features.ModuleStockForecast
	enabled := func(org db.Organization) bool {
		t.Helper()
		ok, err := e.features.Enabled(e.ctx, org.ID, key)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	center := e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsCancelApprove)
	cancel := func(org db.Organization, sub db.ServiceSubscription) {
		t.Helper()
		req, err := svc.RequestCancel(e.ctx, e.caller(org, rbac.ScopeManaged, rbac.PermServiceSubscriptionsCancelRequest),
			sub.Uuid, CancelRequestInput{Reason: "done"})
		if err != nil {
			t.Fatalf("cancel request: %v", err)
		}
		if _, err := svc.ApproveCancel(e.ctx, center, req.UUID, DecisionInput{}); err != nil {
			t.Fatalf("approve: %v", err)
		}
	}

	// Manual ON (admin) -> subscription -> cancelled: still ON.
	if _, err := e.features.SetByAdmin(e.ctx, 0, e.dist.ID, key, true); err != nil {
		t.Fatal(err)
	}
	sub := e.assign(t, svc, e.itemIn(t, CategoryModuleBundle, "10.00", "TRY", "monthly", "0.00"), e.dist, "2026-07-01", "2027-07-01")
	cancel(e.dist, sub)
	if !enabled(e.dist) {
		t.Fatal("manual ON must survive the cancelled subscription")
	}
	if f, err := e.q.GetOrgModuleFlag(e.ctx, db.GetOrgModuleFlagParams{
		Scope: features.ScopeOrg, OrganizationID: pgtype.Int8{Int64: e.dist.ID, Valid: true}, ModuleKey: key,
	}); err != nil || f.Source != features.SourceAdmin || !f.Enabled {
		t.Fatalf("manual row = %+v, %v; want admin ON", f, err)
	}

	// Distributor's manual ON on its dealer -> subscription -> expired: ON.
	if err := e.features.SetForDealers(e.ctx, 0, e.dist.ID, []int64{e.dealer.ID}, key, true); err != nil {
		t.Fatal(err)
	}
	e.assign(t, svc, e.itemIn(t, CategoryModuleBundle, "10.00", "TRY", "monthly", "0.00"), e.dealer, "2026-07-01", "2026-07-31")
	runDaily(t, svc, "2026-08-01")
	if !enabled(e.dealer) {
		t.Fatal("distributor's manual ON must survive the expired subscription")
	}

	// Manual OFF (admin) -> subscription opens it -> expired: OFF again.
	if _, err := e.features.SetByAdmin(e.ctx, 0, e.other.ID, key, false); err != nil {
		t.Fatal(err)
	}
	e.assign(t, svc, e.itemIn(t, CategoryModuleBundle, "10.00", "TRY", "monthly", "0.00"), e.other, "2026-07-01", "2026-07-31")
	if !enabled(e.other) {
		t.Fatal("the subscription grant opens the module over a manual OFF")
	}
	runDaily(t, svc, "2026-08-01")
	if enabled(e.other) {
		t.Fatal("manual OFF must be back after the subscription expired")
	}

	// No manual value -> subscription -> cancelled: OFF.
	fresh := e.org(t, "fresh", "distributor", e.center.ID)
	sub = e.assign(t, svc, e.itemIn(t, CategoryModuleBundle, "10.00", "TRY", "monthly", "0.00"), fresh, "2026-07-01", "2027-07-01")
	if !enabled(fresh) {
		t.Fatal("subscription opens the module")
	}
	cancel(fresh, sub)
	if enabled(fresh) {
		t.Fatal("without a manual value the module closes with the subscription")
	}
}

// Decision 2: a future start waits as scheduled (modules closed, no
// periods); the daily job activates it on starts_on and periods start from
// there. A scheduled subscription is cancelled without a fee.
func TestScheduledSubscriptionActivatesOnStart(t *testing.T) {
	e := newSubEnv(t)
	svc := e.jobSvc(fxrates.New(e.q, nil, nil), day("2026-10-09"))
	key := features.ModuleStockForecast
	sub := e.assign(t, svc, e.itemIn(t, CategoryModuleBundle, "50.00", "TRY", "monthly", "30.00"), e.dist, "2026-11-01", "2027-11-01")
	if sub.Status != StatusScheduled {
		t.Fatalf("status = %s, want scheduled", sub.Status)
	}
	if ok, _ := e.features.Enabled(e.ctx, e.dist.ID, key); ok {
		t.Fatal("a scheduled subscription must not open its modules")
	}

	runDaily(t, svc, "2026-10-31")
	if got := e.sub(t, sub.Uuid); got.Status != StatusScheduled || len(e.periods(t, sub)) != 0 {
		t.Fatalf("before start: status %s, %d periods", got.Status, len(e.periods(t, sub)))
	}
	runDaily(t, svc, "2026-11-01")
	if got := e.sub(t, sub.Uuid); got.Status != StatusActive {
		t.Fatalf("on start status = %s, want active", got.Status)
	}
	if ok, _ := e.features.Enabled(e.ctx, e.dist.ID, key); !ok {
		t.Fatal("activation must open the module")
	}
	ps := e.periods(t, sub)
	if len(ps) != 1 || ps[0].PeriodStart.Time.Format(time.DateOnly) != "2026-11-01" || !ps[0].PostedAt.Valid {
		t.Fatalf("periods = %+v, want one posted from 2026-11-01", ps)
	}
	if evs := e.outboxEvents(t, events.ServiceSubscriptionActivated, sub); len(evs) != 1 {
		t.Fatalf("activated events = %d, want 1", len(evs))
	}

	// Cancelling a scheduled one: no fee, nothing booked, no period.
	later := e.assign(t, svc, e.itemIn(t, "software", "70.00", "TRY", "monthly", "40.00"), e.dist, "2026-12-01", "2027-12-01")
	if later.Status != StatusScheduled {
		t.Fatalf("status = %s, want scheduled", later.Status)
	}
	dist := e.caller(e.dist, rbac.ScopeManaged, rbac.PermServiceSubscriptionsCancelRequest)
	req, err := svc.RequestCancel(e.ctx, dist, later.Uuid, CancelRequestInput{Reason: "changed plans"})
	if err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	if req.CancellationFee != "0.00" && req.CancellationFee != "0" {
		t.Fatalf("scheduled cancellation fee = %s, want 0", req.CancellationFee)
	}
	if got := e.sub(t, later.Uuid); got.Status != StatusScheduled {
		t.Fatalf("pending request status = %s, want scheduled", got.Status)
	}
	if _, err := svc.RequestCancel(e.ctx, dist, later.Uuid, CancelRequestInput{Reason: "again"}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("second request = %v, want ErrInvalidStatus", err)
	}
	if _, err := svc.ApproveCancel(e.ctx, e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsCancelApprove), req.UUID, DecisionInput{}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got := e.sub(t, later.Uuid); got.Status != StatusCancelled {
		t.Fatalf("status = %s, want cancelled", got.Status)
	}
	evs := e.outboxEvents(t, events.ServiceSubscriptionCancelled, later)
	if len(evs) != 1 {
		t.Fatalf("cancelled events = %d, want 1", len(evs))
	}
	if err := svc.HandleSubscriptionCancelled(e.ctx, events.New(events.ServiceSubscriptionCancelled).WithPayload(evs[0])); err != nil {
		t.Fatal(err)
	}
	if rows := e.ledger(t, SourceSubscriptionCancel, req.UUID); len(rows) != 0 {
		t.Fatalf("fee rows = %+v, want none", rows)
	}
	runDaily(t, svc, "2026-12-05")
	if ps := e.periods(t, later); len(ps) != 0 {
		t.Fatalf("periods of cancelled scheduled subscription = %d, want 0", len(ps))
	}
	if ok, _ := e.features.Enabled(e.ctx, e.dist.ID, key); !ok {
		t.Fatal("cancelling the scheduled one must not close the running bundle's module")
	}
}

// The 30 and 7 day reminders go out once each.
func TestExpiryRemindersSentOnce(t *testing.T) {
	e := newSubEnv(t)
	svc := e.jobSvc(fxrates.New(e.q, nil, nil), day("2026-07-01"))
	sub := e.assign(t, svc, e.itemIn(t, "software", "10.00", "TRY", "monthly", "0.00"), e.dist, "2026-07-01", "2026-09-30")
	for _, on := range []string{"2026-08-30", "2026-08-31", "2026-08-31", "2026-09-10", "2026-09-23", "2026-09-23", "2026-09-26"} {
		if _, err := svc.SendExpiryReminders(e.ctx, day(on)); err != nil {
			t.Logf("reminders on %s: %v", on, err)
		}
	}
	evs := e.outboxEvents(t, events.ServiceSubscriptionExpiring, sub)
	if len(evs) != 2 || evs[0]["days_before"] != "30" || evs[1]["days_before"] != "7" {
		t.Fatalf("expiring events = %+v, want 30 then 7", evs)
	}
	if evs[0]["notify_user_ids"] == nil {
		t.Fatal("reminder has no recipients")
	}
}
