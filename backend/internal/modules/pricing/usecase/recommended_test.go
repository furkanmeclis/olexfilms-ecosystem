package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	library "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/library/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- fakes -------------------------------------------------------------------------

type fakeOutbox struct{ events []events.Event }

func (o *fakeOutbox) Enqueue(_ context.Context, _ pgx.Tx, ev events.Event) error {
	o.events = append(o.events, ev)
	return nil
}

func (o *fakeOutbox) named(name string) []events.Event {
	var out []events.Event
	for _, e := range o.events {
		if e.Name == name {
			out = append(out, e)
		}
	}
	return out
}

type fakeSettings struct {
	threshold int
	auto      bool
}

func (s fakeSettings) PricingDeviationWarningPct(context.Context) int { return s.threshold }
func (s fakeSettings) PricingPriceListAutoPublish(context.Context) bool {
	return s.auto
}

type fakePriceListQueue struct{ tasks []PriceListTask }

func (q *fakePriceListQueue) EnqueuePriceList(_ context.Context, t PriceListTask) error {
	q.tasks = append(q.tasks, t)
	return nil
}

type fakeRenderer struct {
	loader PriceListLoader
	calls  []string
	vars   []map[string]string
}

func (r *fakeRenderer) RenderSource(ctx context.Context, kind, sourceID, locale string) ([]byte, error) {
	if kind != docmodel.KindPriceList {
		return nil, fmt.Errorf("kind %s", kind)
	}
	src, err := r.loader.Load(ctx, docmodel.Viewer{System: true}, sourceID, locale)
	if err != nil {
		return nil, err
	}
	r.calls = append(r.calls, locale)
	r.vars = append(r.vars, src.Vars)
	return []byte("%PDF-1.4 price list " + locale), nil
}

type fakeFeatures struct{ off map[string]bool }

func (f fakeFeatures) Enabled(_ context.Context, _ int64, key string) (bool, error) {
	return !f.off[key], nil
}

// --- fixture -------------------------------------------------------------------------

type recFixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	svc      *Recommended
	box      *fakeOutbox
	queue    *fakePriceListQueue
	now      time.Time
	center   db.Organization
	brandID  int64
	catID    int64
	prefix   string
	seq      int
	today    time.Time
	tr, de   int64
	fr       int64
	settings *fakeSettings
}

func newRecFixture(t *testing.T) *recFixture {
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
	f := &recFixture{
		ctx: ctx, tx: tx, q: q, box: &fakeOutbox{}, queue: &fakePriceListQueue{},
		center: center, brandID: brand.ID, prefix: fmt.Sprintf("t506-%d", time.Now().UnixNano()),
		settings: &fakeSettings{threshold: 15, auto: true},
	}
	f.now = time.Now()
	f.today = LocalDay(f.now, center.Timezone)
	f.svc = NewRecommended(tx, q, f.box, f.settings, nil)
	f.svc.SetClock(func() time.Time { return f.now })
	f.svc.SetPriceListQueue(f.queue)
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: brand.ID, Name: f.prefix + "-cat", AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	f.catID = cat.ID
	if err := tx.QueryRow(ctx, `SELECT (SELECT id FROM countries WHERE iso2 = 'TR'), (SELECT id FROM countries WHERE iso2 = 'DE'), (SELECT id FROM countries WHERE iso2 = 'FR')`).
		Scan(&f.tr, &f.de, &f.fr); err != nil {
		t.Fatalf("countries: %v", err)
	}
	return f
}

func (f *recFixture) product(t *testing.T, name string) db.Product {
	t.Helper()
	f.seq++
	p, err := f.q.CreateProduct(f.ctx, db.CreateProductParams{
		OrganizationID: f.center.ID, BrandID: f.brandID, CategoryID: f.catID,
		Sku: fmt.Sprintf("%s-%d", f.prefix, f.seq), Name: f.prefix + " " + name, Images: []byte("[]"),
		UnitType: "piece", Active: true,
	})
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	return p
}

func (f *recFixture) org(t *testing.T, typ string, parent int64, country int64, currency, locale string) db.Organization {
	t.Helper()
	f.seq++
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("%s-%s-%d", f.prefix, typ, f.seq), Name: fmt.Sprintf("%s %s %d", f.prefix, typ, f.seq),
		Status: "active", Type: typ, ParentID: pgtype.Int8{Int64: parent, Valid: true}, BrandID: f.brandID,
		Currency: currency, Locale: locale, Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		CountryID:      pgtype.Int8{Int64: country, Valid: country != 0},
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	return o
}

func (f *recFixture) owner(t *testing.T, o db.Organization) db.User {
	t.Helper()
	f.seq++
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Owner", Surname: "T506", Status: "active",
		Email: pgtype.Text{String: fmt.Sprintf("%s-%d@example.test", f.prefix, f.seq), Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	if _, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{OrganizationID: o.ID, UserID: u.ID, Role: "owner"}); err != nil {
		t.Fatalf("member: %v", err)
	}
	return u
}

func (f *recFixture) dealerPrice(t *testing.T, o db.Organization, p db.Product, price string) {
	t.Helper()
	n, err := numericOf(price)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.UpsertDealerProductPrice(f.ctx, db.UpsertDealerProductPriceParams{
		OrganizationID: o.ID, BrandID: f.brandID, ProductID: p.ID, SalePrice: n, Currency: o.Currency,
	}); err != nil {
		t.Fatalf("dealer price: %v", err)
	}
}

func (f *recFixture) centerViewer() Viewer {
	return Viewer{OrgID: f.center.ID, OrgType: OrgCenter, BrandID: f.brandID, RecommendedRead: true, RecommendedWrite: true, SaleWrite: true}
}

func (f *recFixture) publish(t *testing.T, day time.Time, rows ...PublishRow) PublishResult {
	t.Helper()
	out, err := f.svc.Publish(f.ctx, f.centerViewer(), PublishInput{EffectiveFrom: day.Format(time.DateOnly), Rows: rows})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return out
}

func row(p db.Product, country, currency, price string) PublishRow {
	id := p.Uuid
	return PublishRow{ProductUUID: &id, Country: country, Currency: currency, Price: price}
}

// current is the projection price of a key ("" when none) and its updated_at.
func (f *recFixture) current(t *testing.T, productID, country int64, currency string) (string, time.Time) {
	t.Helper()
	var price pgtype.Numeric
	var updated time.Time
	err := f.tx.QueryRow(f.ctx, `SELECT price, updated_at FROM recommended_prices_current
		WHERE product_id = $1 AND country_id IS NOT DISTINCT FROM $2 AND currency = $3`,
		productID, pgtype.Int8{Int64: country, Valid: country != 0}, currency).Scan(&price, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}
	}
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	return money(price), updated
}

func (f *recFixture) listPrice(t *testing.T, productID int64, currency string) string {
	t.Helper()
	var n pgtype.Numeric
	err := f.tx.QueryRow(f.ctx, `SELECT recommended_sale_price FROM product_prices WHERE product_id = $1 AND currency = $2`, productID, currency).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatalf("product_prices: %v", err)
	}
	p := numericText(n)
	if p == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimRight(*p, "0"), ".")
}

// --- acceptance tests --------------------------------------------------------------

// A version dated tomorrow leaves the projection alone until its day; the
// tick of that day applies it, a second tick changes nothing (one change);
// product_prices.recommended_sale_price follows the currency-wide price.
func TestFutureVersionAppliesOnItsDayOnce(t *testing.T) {
	f := newRecFixture(t)
	p := f.product(t, "film")
	f.publish(t, f.today, row(p, "", "EUR", "100.00"))
	if got, _ := f.current(t, p.ID, 0, "EUR"); got != "100.00" {
		t.Fatalf("today's version must be current in the publishing tx, got %q", got)
	}
	if got := f.listPrice(t, p.ID, "EUR"); got != "100" {
		t.Fatalf("product_prices.recommended_sale_price = %q, want 100", got)
	}

	tomorrow := f.today.AddDate(0, 0, 1)
	res := f.publish(t, tomorrow, row(p, "", "EUR", "120.00"))
	if res.ScheduledCount != 1 || res.AppliedCount != 0 || res.Versions[0].Status != VersionScheduled {
		t.Fatalf("result = %+v", res)
	}
	if got, _ := f.current(t, p.ID, 0, "EUR"); got != "100.00" {
		t.Fatalf("a future version must not change the projection, got %q", got)
	}
	// Today's tick: nothing due.
	if n, err := f.svc.ApplyDue(f.ctx, f.brandID, f.today); err != nil || n != 0 {
		t.Fatalf("today tick = %d, %v", n, err)
	}
	if got, _ := f.current(t, p.ID, 0, "EUR"); got != "100.00" {
		t.Fatalf("projection changed before the day: %q", got)
	}

	// The day comes: the hourly tick (center timezone) applies it.
	f.queue.tasks = nil
	f.now = f.now.Add(24 * time.Hour)
	if err := f.svc.DailyTask(f.ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
	got, updated := f.current(t, p.ID, 0, "EUR")
	if got != "120.00" {
		t.Fatalf("projection = %q, want 120.00", got)
	}
	if lp := f.listPrice(t, p.ID, "EUR"); lp != "120" {
		t.Fatalf("product_prices not synced: %q", lp)
	}
	if len(f.queue.tasks) == 0 || f.queue.tasks[0].Currency != "EUR" || f.queue.tasks[0].CountryID != 0 {
		t.Fatalf("the applied version must queue its price list, got %+v", f.queue.tasks)
	}
	// Second tick of the same day: no second change.
	if n, err := f.svc.ApplyDue(f.ctx, f.brandID, LocalDay(f.now, f.center.Timezone)); err != nil || n != 0 {
		t.Fatalf("second tick = %d, %v", n, err)
	}
	if err := f.svc.DailyTask(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got2, updated2 := f.current(t, p.ID, 0, "EUR"); got2 != "120.00" || !updated2.Equal(updated) {
		t.Fatalf("second tick changed the projection: %q %v -> %v", got2, updated, updated2)
	}
	var live int
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM recommended_price_versions WHERE product_id = $1 AND superseded_at IS NULL`, p.ID).Scan(&live); err != nil || live != 1 {
		t.Fatalf("live versions = %d, %v", live, err)
	}
}

// Publication validates every row (past day, unknown country, duplicate)
// and writes the event with the owners of the country's organizations.
func TestPublishValidatesAndNotifiesCountryOwners(t *testing.T) {
	f := newRecFixture(t)
	p := f.product(t, "film")
	dist := f.org(t, OrgDistributor, f.center.ID, f.tr, "TRY", "tr")
	dealer := f.org(t, OrgDealer, dist.ID, f.tr, "TRY", "tr")
	other := f.org(t, OrgDealer, dist.ID, f.de, "EUR", "de")
	distOwner, dealerOwner := f.owner(t, dist), f.owner(t, dealer)
	otherOwner := f.owner(t, other)

	cases := []PublishInput{
		{EffectiveFrom: f.today.AddDate(0, 0, -1).Format(time.DateOnly), Rows: []PublishRow{row(p, "TR", "TRY", "10")}},
		{Rows: []PublishRow{row(p, "XX", "TRY", "10")}},
		{Rows: []PublishRow{row(p, "TR", "TRY", "10"), row(p, "tr", "try", "11")}},
		{Rows: []PublishRow{row(p, "TR", "TRY", "-1")}},
		{Rows: nil},
	}
	for i, in := range cases {
		var ve *ValidationError
		if _, err := f.svc.Publish(f.ctx, f.centerViewer(), in); !errors.As(err, &ve) {
			t.Errorf("case %d: err = %v, want validation error", i, err)
		}
	}
	if _, err := f.svc.Publish(f.ctx, Viewer{OrgID: dist.ID, OrgType: OrgDistributor, BrandID: f.brandID, RecommendedWrite: true},
		PublishInput{Rows: []PublishRow{row(p, "TR", "TRY", "10")}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor publish err = %v", err)
	}
	if len(f.box.events) != 0 {
		t.Fatalf("rejected publications wrote events: %d", len(f.box.events))
	}

	res := f.publish(t, f.today, row(p, "TR", "TRY", "4500.50"))
	evs := f.box.named(events.PricingRecommendedPublished)
	if len(evs) != 1 {
		t.Fatalf("events = %d", len(evs))
	}
	ids, _ := evs[0].Payload["notify_user_ids"].([]int64)
	want := map[int64]bool{distOwner.ID: true, dealerOwner.ID: true}
	if len(ids) != 2 || !want[ids[0]] || !want[ids[1]] {
		t.Fatalf("recipients = %v, want the TR owners (not %d)", ids, otherOwner.ID)
	}
	if evs[0].Payload["batch_id"] != res.BatchID.String() || evs[0].Payload["applied"] != true {
		t.Fatalf("payload = %+v", evs[0].Payload)
	}
	// The listener queues the TR/TRY price list of the batch.
	if err := f.svc.EnqueuePublished(f.ctx, evs[0]); err != nil {
		t.Fatal(err)
	}
	if len(f.queue.tasks) != 1 || f.queue.tasks[0].CountryID != f.tr || f.queue.tasks[0].Token != res.BatchID.String() {
		t.Fatalf("tasks = %+v", f.queue.tasks)
	}
	// Auto publication off: nothing queued.
	f.queue.tasks, f.settings.auto = nil, false
	if err := f.svc.EnqueuePublished(f.ctx, evs[0]); err != nil || len(f.queue.tasks) != 0 {
		t.Fatalf("auto publish off queued %v (%v)", f.queue.tasks, err)
	}
}

// A dealer sees its country's price, a dealer of a country without one the
// currency-wide price; the deviation compares its own sale price; without
// pricing.recommended.read there is no recommended block.
func TestRecommendedBlockFallsBackAndNeedsPermission(t *testing.T) {
	f := newRecFixture(t)
	p := f.product(t, "film")
	f.publish(t, f.today, row(p, "", "EUR", "100.00"), row(p, "DE", "EUR", "120.00"))
	dist := f.org(t, OrgDistributor, f.center.ID, f.de, "EUR", "de")
	deDealer := f.org(t, OrgDealer, dist.ID, f.de, "EUR", "de")
	frDealer := f.org(t, OrgDealer, dist.ID, f.fr, "EUR", "fr")
	f.dealerPrice(t, deDealer, p, "138.00")
	f.dealerPrice(t, frDealer, p, "90.00")

	svc := New(f.q)
	view := func(o db.Organization, read bool) EffectivePrice {
		t.Helper()
		v, err := svc.ProductView(f.ctx, Viewer{OrgID: o.ID, OrgType: o.Type, BrandID: f.brandID, RecommendedRead: read}, p.Uuid)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range v.Prices {
			if e.Currency == "EUR" {
				return e
			}
		}
		return EffectivePrice{}
	}
	de := view(deDealer, true)
	if de.Recommended == nil || de.Recommended.Price != "120.00" || de.Recommended.Scope != ScopeCountry || de.Recommended.CountryISO2 != "DE" {
		t.Fatalf("DE dealer recommended = %+v", de.Recommended)
	}
	if de.DeviationPct == nil || *de.DeviationPct != "15.00" {
		t.Fatalf("DE deviation = %v, want 15.00", de.DeviationPct)
	}
	fr := view(frDealer, true)
	if fr.Recommended == nil || fr.Recommended.Price != "100.00" || fr.Recommended.Scope != ScopeCurrency || fr.Recommended.CountryISO2 != "" {
		t.Fatalf("FR dealer must fall back to the currency-wide price, got %+v", fr.Recommended)
	}
	if fr.DeviationPct == nil || *fr.DeviationPct != "-10.00" {
		t.Fatalf("FR deviation = %v, want -10.00", fr.DeviationPct)
	}
	if d := view(dist, true); d.Recommended == nil || d.Recommended.Price != "120.00" || d.DeviationPct != nil {
		t.Fatalf("distributor block = %+v / %v", d.Recommended, d.DeviationPct)
	}
	noPerm := view(deDealer, false)
	if noPerm.Recommended != nil || noPerm.DeviationPct != nil {
		t.Fatal("a dealer without pricing.recommended.read must not get the recommended block")
	}
	raw, _ := json.Marshal(noPerm)
	if strings.Contains(string(raw), "recommended") || strings.Contains(string(raw), "deviation_pct") {
		t.Fatalf("json leaks the block: %s", raw)
	}

	// The price list in force: DE row for a DE dealer, currency-wide for FR.
	items, total, err := f.svc.Current(f.ctx, Viewer{OrgID: frDealer.ID, OrgType: OrgDealer, BrandID: f.brandID, RecommendedRead: true}, CurrentFilter{ProductUUIDs: []uuid.UUID{p.Uuid}})
	if err != nil || total != 1 || items[0].Price != "100.00" || items[0].Scope != ScopeCurrency {
		t.Fatalf("FR current = %+v (%d, %v)", items, total, err)
	}
	items, _, err = f.svc.Current(f.ctx, f.centerViewer(), CurrentFilter{Country: "DE", Currency: "EUR", ProductUUIDs: []uuid.UUID{p.Uuid}})
	if err != nil || len(items) != 1 || items[0].Price != "120.00" || items[0].CountryISO2 != "DE" {
		t.Fatalf("center DE current = %+v (%v)", items, err)
	}
	if _, _, err := f.svc.Current(f.ctx, Viewer{OrgID: frDealer.ID, OrgType: OrgDealer, BrandID: f.brandID}, CurrentFilter{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("current without permission: %v", err)
	}
}

func TestDeviationPct(t *testing.T) {
	cases := []struct{ price, rec, want string }{
		{"115", "100", "15.00"}, {"85", "100", "-15.00"}, {"100", "100", "0.00"},
		{"1", "3", "-66.67"}, {"2", "3", "-33.33"}, {"100.005", "100", "0.01"},
	}
	for _, c := range cases {
		got := DeviationPct(c.price, c.rec)
		if got == nil || *got != c.want {
			t.Errorf("DeviationPct(%s, %s) = %v, want %s", c.price, c.rec, got, c.want)
		}
	}
	if DeviationPct("10", "0") != nil || DeviationPct("", "10") != nil {
		t.Error("zero or missing recommendation has no deviation")
	}
}

// The daily snapshot records the deviation of each list price against the
// price in force for the organization's country; the list, summary and the
// threshold agree; the snapshot is written once a day; the Monday snapshot
// writes one digest for the center and one per distributor.
func TestDisciplineSnapshotSummaryAndDigest(t *testing.T) {
	f := newRecFixture(t)
	// A Monday 10:00 in the center's timezone.
	loc, _ := time.LoadLocation(f.center.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	monday := time.Date(2026, 10, 12, 10, 0, 0, 0, loc)
	f.now = monday
	f.today = LocalDay(monday, f.center.Timezone)
	p1, p2 := f.product(t, "a"), f.product(t, "b")
	f.publish(t, f.today, row(p1, "", "TRY", "100.00"), row(p1, "TR", "TRY", "200.00"), row(p2, "", "TRY", "50.00"))
	dist := f.org(t, OrgDistributor, f.center.ID, f.tr, "TRY", "tr")
	d1 := f.org(t, OrgDealer, dist.ID, f.tr, "TRY", "tr")
	d2 := f.org(t, OrgDealer, dist.ID, f.tr, "TRY", "tr")
	d3 := f.org(t, OrgDealer, f.center.ID, f.de, "TRY", "tr")
	f.dealerPrice(t, d1, p1, "230.00") // +15.00 (TR price 200): over
	f.dealerPrice(t, d2, p1, "210.00") // +5.00
	f.dealerPrice(t, d2, p2, "40.00")  // -20.00: over
	f.dealerPrice(t, d3, p1, "100.00") // DE falls back to 100: 0.00
	for _, o := range []db.Organization{dist, d1, d2} {
		u := f.owner(t, o)
		if err := f.q.AssignMemberRoleBySlug(f.ctx, db.AssignMemberRoleBySlugParams{
			MemberID: memberID(t, f, o.ID, u.ID), Slug: map[string]string{OrgDistributor: "distributor_owner", OrgDealer: "dealer_owner"}[o.Type],
		}); err != nil {
			t.Fatal(err)
		}
	}
	cu := f.owner(t, f.center)
	if err := f.q.AssignMemberRoleBySlug(f.ctx, db.AssignMemberRoleBySlugParams{MemberID: memberID(t, f, f.center.ID, cu.ID), Slug: "center_staff"}); err != nil {
		t.Fatal(err)
	}

	ok, err := f.svc.SnapshotDiscipline(f.ctx, f.center, f.today)
	if err != nil || !ok {
		t.Fatalf("snapshot = %v, %v", ok, err)
	}
	if again, err := f.svc.SnapshotDiscipline(f.ctx, f.center, f.today); err != nil || again {
		t.Fatalf("a second snapshot the same day = %v, %v", again, err)
	}
	scope := DisciplineScope{BrandID: f.brandID, OrgIDs: []int64{dist.ID, d1.ID, d2.ID, d3.ID}}
	rows, total, day, threshold, err := f.svc.ListDiscipline(f.ctx, scope, DisciplineFilter{Limit: 50})
	if err != nil || total != 4 || day != f.today.Format(time.DateOnly) || threshold != 15 {
		t.Fatalf("list = %d rows, total %d, day %s, threshold %d, %v", len(rows), total, day, threshold, err)
	}
	dev := map[string]string{}
	for _, r := range rows {
		dev[fmt.Sprintf("%s/%s", r.OrganizationUUID, r.ProductSKU)] = *r.DeviationPct + fmt.Sprintf("/%v", r.OverThreshold)
	}
	want := map[string]string{
		d1.Uuid.String() + "/" + p1.Sku: "15.00/true", d2.Uuid.String() + "/" + p1.Sku: "5.00/false",
		d2.Uuid.String() + "/" + p2.Sku: "-20.00/true", d3.Uuid.String() + "/" + p1.Sku: "0.00/false",
	}
	for k, v := range want {
		if dev[k] != v {
			t.Errorf("%s = %q, want %q", k, dev[k], v)
		}
	}
	over := true
	if _, n, _, _, _ := f.svc.ListDiscipline(f.ctx, scope, DisciplineFilter{OverThreshold: &over, Limit: 50}); n != 2 {
		t.Errorf("over threshold rows = %d, want 2", n)
	}
	// Distributor subtree only.
	sub := DisciplineScope{BrandID: f.brandID, OrgIDs: []int64{dist.ID, d1.ID, d2.ID}}
	if _, n, _, _, _ := f.svc.ListDiscipline(f.ctx, sub, DisciplineFilter{Limit: 50}); n != 3 {
		t.Errorf("subtree rows = %d, want 3", n)
	}
	sum, err := f.svc.Summary(f.ctx, scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	var tr, de *DisciplineCountry
	for i := range sum.Countries {
		switch sum.Countries[i].CountryISO2 {
		case "TR":
			tr = &sum.Countries[i]
		case "DE":
			de = &sum.Countries[i]
		}
	}
	if tr == nil || tr.OrgCount != 2 || tr.RowCount != 3 || *tr.AvgDeviationPct != "0.00" || *tr.MedianDeviationPct != "5.00" || tr.OverThresholdOrgCount != 2 {
		t.Fatalf("TR summary = %+v", tr)
	}
	if de == nil || de.OrgCount != 1 || de.OverThresholdOrgCount != 0 {
		t.Fatalf("DE summary = %+v", de)
	}

	// The export carries the same rows (CSV / XLSX through the io engine).
	ds, err := NewDisciplineExportAdapter(f.svc, ParseDisciplineFilter).Export(f.ctx,
		DisciplineExportQuery(sub, url.Values{"over_threshold": {"true"}}), "tr")
	if err != nil || len(ds.Rows) != 2 || ds.Resource != ResourceDisciplineExport {
		t.Fatalf("export = %d rows, %v", len(ds.Rows), err)
	}

	digests := f.box.named(events.PricingDisciplineDigest)
	if len(digests) != 2 {
		t.Fatalf("digests = %d, want center + distributor", len(digests))
	}
	for _, d := range digests {
		ids, _ := d.Payload["notify_user_ids"].([]int64)
		if len(ids) == 0 || d.Payload["org_count"] != "2" {
			t.Errorf("digest %+v", d.Payload)
		}
	}
}

func memberID(t *testing.T, f *recFixture, orgID, userID int64) int64 {
	t.Helper()
	var id int64
	if err := f.tx.QueryRow(f.ctx, `SELECT id FROM organization_members WHERE organization_id = $1 AND user_id = $2`, orgID, userID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// The price list PDF lands as a new version of the same library item on
// every publication, in the languages of the organizations it reaches.
func TestPriceListPublishAddsVersionToSameItem(t *testing.T) {
	f := newRecFixture(t)
	p := f.product(t, "film")
	f.publish(t, f.today, row(p, "TR", "TRY", "4500.00"))
	f.publish(t, f.today.AddDate(0, 0, 3), row(p, "TR", "TRY", "4700.00"))
	dist := f.org(t, OrgDistributor, f.center.ID, f.tr, "TRY", "de")
	_ = dist
	lib := library.New(f.q, storage.NewMemory())
	render := &fakeRenderer{loader: NewPriceListLoader(f.q)}
	pub := NewPriceListPublisher(f.q, render, lib, fakeFeatures{}, nil)
	pub.SetClock(func() time.Time { return f.now })
	task := PriceListTask{BrandID: f.brandID, CountryID: f.tr, Currency: "TRY"}

	first, err := pub.Publish(f.ctx, task)
	if err != nil || first.Skipped {
		t.Fatalf("publish: %+v, %v", first, err)
	}
	second, err := pub.Publish(f.ctx, task)
	if err != nil {
		t.Fatal(err)
	}
	if first.ItemUUID != second.ItemUUID {
		t.Fatal("a republication must add a version to the same item")
	}
	actor := library.Actor{OrganizationID: f.center.ID, BrandID: f.brandID, OrgType: OrgCenter}
	versions, err := lib.ListVersions(f.ctx, actor, first.ItemUUID)
	if err != nil {
		t.Fatal(err)
	}
	perLocale := map[string][]int32{}
	for _, v := range versions {
		perLocale[v.Locale] = append(perLocale[v.Locale], v.VersionNo)
	}
	if len(perLocale["de"]) != 2 || len(perLocale[docmodel.NormalizeLanguage(f.center.Locale)]) != 2 {
		t.Fatalf("versions per locale = %v (center %s)", perLocale, f.center.Locale)
	}
	items, _, err := lib.ListItems(f.ctx, actor, library.ListInput{Tags: []string{"price-list:tr:try"}})
	if err != nil || len(items) != 1 || items[0].AccessLevel != library.AccessAllNetwork {
		t.Fatalf("items = %+v, %v", items, err)
	}
	// The distributor sees it in the document center.
	distItems, _, err := lib.ListItems(f.ctx, library.Actor{OrganizationID: dist.ID, BrandID: f.brandID, OrgType: OrgDistributor},
		library.ListInput{Tags: []string{"price-list:tr:try"}})
	if err != nil || len(distItems) != 1 {
		t.Fatalf("distributor items = %d, %v", len(distItems), err)
	}
	// The table holds the price in force and the scheduled change.
	vars := render.vars[0]
	if !strings.Contains(vars["prices_table"], p.Sku) || !strings.Contains(vars["prices_table"], "4500.00") ||
		!strings.Contains(vars["prices_table"], "4700.00") || vars["currency"] != "TRY" {
		t.Fatalf("vars = %v", vars)
	}
}

// With the library (announcements) module off for the center the
// publication is skipped: no folder, no item, no render.
func TestPriceListSkippedWhenLibraryOff(t *testing.T) {
	f := newRecFixture(t)
	p := f.product(t, "film")
	f.publish(t, f.today, row(p, "", "EUR", "10.00"))
	lib := library.New(f.q, storage.NewMemory())
	render := &fakeRenderer{loader: NewPriceListLoader(f.q)}
	pub := NewPriceListPublisher(f.q, render, lib, fakeFeatures{off: map[string]bool{features.ModuleAnnouncements: true}}, nil)
	res, err := pub.Publish(f.ctx, PriceListTask{BrandID: f.brandID, Currency: "EUR"})
	if err != nil || !res.Skipped {
		t.Fatalf("res = %+v, %v", res, err)
	}
	if len(render.calls) != 0 {
		t.Fatal("nothing must be rendered")
	}
	var n int
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM library_items WHERE brand_id = $1 AND 'price-list' = ANY(tags) AND deleted_at IS NULL`, f.brandID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("library items = %d, %v", n, err)
	}
}

// The staged import: preview classifies without writing; confirm publishes
// one batch (batch_id = job) and a second confirm writes nothing.
func TestRecommendedImportStageApply(t *testing.T) {
	f := newRecFixture(t)
	p := f.product(t, "film")
	u := f.owner(t, f.center)
	job, err := f.q.CreateImportJob(f.ctx, db.CreateImportJobParams{
		Resource: ResourceRecommendedImport, ActorID: u.ID, Format: "csv", Locale: "tr",
		OrganizationID: pgtype.Int8{Int64: f.center.ID, Valid: true}, SourceFilename: "prices.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	im := NewRecommendedImporter(f.svc)
	ij := ioengine.ImportJob{ID: job.ID, UUID: job.Uuid, ActorID: u.ID, OrganizationID: f.center.ID, Locale: "tr"}
	rows := []map[string]any{
		{"sku": p.Sku, "country": "tr", "currency": "try", "price": "4500.5"},
		{"sku": p.Sku, "country": "TR", "currency": "TRY", "price": "4600"},
		{"sku": "nope", "currency": "TRY", "price": "1"},
		{"sku": p.Sku, "currency": "EUR", "price": float64(99), "effective_from": f.today.AddDate(0, 0, 2).Format(time.DateOnly)},
	}
	sum, err := im.Stage(f.ctx, ij, rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Valid != 2 || sum.Invalid != 2 || sum.Counts[ImportRowDuplicate] != 1 || sum.Counts[ImportRowInvalid] != 1 {
		t.Fatalf("stage = %+v", sum)
	}
	if got, _ := f.current(t, p.ID, f.tr, "TRY"); got != "" {
		t.Fatal("the dry run must not write")
	}
	body, _ := json.Marshal(sum)
	if _, err := f.tx.Exec(f.ctx, `UPDATE import_jobs SET preview_json = $2 WHERE id = $1`, job.ID, body); err != nil {
		t.Fatal(err)
	}
	applied, err := im.Apply(f.ctx, ij)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Counts[ImportRowApplied] != 2 {
		t.Fatalf("apply = %+v", applied.Counts)
	}
	if got, _ := f.current(t, p.ID, f.tr, "TRY"); got != "4500.50" {
		t.Fatalf("TR current = %q", got)
	}
	batch, err := f.q.ListRecommendedPriceVersionsByBatch(f.ctx, db.ListRecommendedPriceVersionsByBatchParams{BrandID: f.brandID, BatchID: job.Uuid})
	if err != nil || len(batch) != 2 {
		t.Fatalf("batch versions = %d, %v", len(batch), err)
	}
	if _, err := im.Apply(f.ctx, ij); err != nil {
		t.Fatal(err)
	}
	again, _ := f.q.ListRecommendedPriceVersionsByBatch(f.ctx, db.ListRecommendedPriceVersionsByBatchParams{BrandID: f.brandID, BatchID: job.Uuid})
	if len(again) != 2 || len(f.box.named(events.PricingRecommendedPublished)) != 1 {
		t.Fatalf("a second confirm wrote again: %d versions, %d events", len(again), len(f.box.named(events.PricingRecommendedPublished)))
	}
	if _, err := im.Undo(f.ctx, ij); !errors.Is(err, ioengine.ErrImportRejected) {
		t.Fatalf("undo = %v", err)
	}
}

// The version history follows the list contract (filters, sort, statuses).
func TestListVersionsContract(t *testing.T) {
	f := newRecFixture(t)
	p1, p2 := f.product(t, "a"), f.product(t, "b")
	f.publish(t, f.today, row(p1, "", "EUR", "10.00"), row(p2, "DE", "EUR", "30.00"))
	f.publish(t, f.today.AddDate(0, 0, 5), row(p1, "", "EUR", "20.00"))
	v := f.centerViewer()
	all, total, err := f.svc.ListVersions(f.ctx, v, VersionFilter{ProductUUIDs: []uuid.UUID{p1.Uuid, p2.Uuid}, Limit: 50})
	if err != nil || total != 3 {
		t.Fatalf("total = %d, %v", total, err)
	}
	if all[0].EffectiveFrom != f.today.AddDate(0, 0, 5).Format(time.DateOnly) || all[0].Status != VersionScheduled {
		t.Fatalf("default sort -effective_from: first = %+v", all[0])
	}
	none, n, _ := f.svc.ListVersions(f.ctx, v, VersionFilter{ProductUUIDs: []uuid.UUID{p1.Uuid, p2.Uuid}, Countries: []string{CountryNone}, Limit: 50})
	if n != 2 || none[0].CountryISO2 != "" {
		t.Fatalf("country=none = %d", n)
	}
	de, n, _ := f.svc.ListVersions(f.ctx, v, VersionFilter{ProductUUIDs: []uuid.UUID{p1.Uuid, p2.Uuid}, Countries: []string{"DE"}, Limit: 50})
	if n != 1 || de[0].ProductUUID != p2.Uuid || de[0].Status != VersionCurrent {
		t.Fatalf("country=DE = %+v", de)
	}
	page, n, _ := f.svc.ListVersions(f.ctx, v, VersionFilter{ProductUUIDs: []uuid.UUID{p1.Uuid, p2.Uuid}, Limit: 1, Offset: 1})
	if n != 3 || len(page) != 1 {
		t.Fatalf("page = %d/%d", len(page), n)
	}
	if _, _, err := f.svc.ListVersions(f.ctx, Viewer{OrgID: v.OrgID, OrgType: OrgDistributor, BrandID: f.brandID, RecommendedRead: true}, VersionFilter{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor history = %v", err)
	}
}
