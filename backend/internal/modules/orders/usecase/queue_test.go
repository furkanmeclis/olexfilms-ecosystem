package usecase

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestScheduleInputParse(t *testing.T) {
	day := "2026-12-01"
	bad := "01.12.2026"
	empty := ""
	cases := []struct {
		in      ScheduleInput
		wantErr string
		prio    string
		due     bool
	}{
		{in: ScheduleInput{}, prio: PriorityNormal},
		{in: ScheduleInput{Priority: " urgent ", DueDate: &day}, prio: PriorityUrgent, due: true},
		{in: ScheduleInput{Priority: "normal", DueDate: &empty}, prio: PriorityNormal},
		{in: ScheduleInput{Priority: "high"}, wantErr: "priority"},
		{in: ScheduleInput{DueDate: &bad}, wantErr: "due_date"},
	}
	for _, tc := range cases {
		got, err := tc.in.parse()
		if tc.wantErr != "" {
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.wantErr {
				t.Fatalf("%+v: want %s error, got %v", tc.in, tc.wantErr, err)
			}
			continue
		}
		if err != nil || got.priority != tc.prio || got.due.Valid != tc.due {
			t.Fatalf("%+v: got %+v, %v", tc.in, got, err)
		}
	}
}

func TestPackagesInputParse(t *testing.T) {
	n, zero, big := int32(3), int32(0), int32(MaxPackageCount+1)
	d, neg := 12.5, -1.0
	if p, err := (PackagesInput{PackageCount: &n, Desi: &d}).parse(); err != nil || !p.any() || p.count.Int32 != 3 || !p.desi.Valid || p.weight.Valid {
		t.Fatalf("valid: %+v %v", p, err)
	}
	if p, err := (PackagesInput{}).parse(); err != nil || p.any() {
		t.Fatalf("empty: %+v %v", p, err)
	}
	for field, in := range map[string]PackagesInput{
		"package_count": {PackageCount: &zero},
		"weight_kg":     {WeightKg: &neg},
	} {
		var ve *ValidationError
		if _, err := in.parse(); !errors.As(err, &ve) || ve.Field != field {
			t.Fatalf("%s: got %v", field, err)
		}
	}
	if _, err := (PackagesInput{PackageCount: &big}).parse(); err == nil {
		t.Fatal("too many packages accepted")
	}
}

func TestOverdue(t *testing.T) {
	zone, _ := time.LoadLocation("Europe/Istanbul")
	// 22:30 UTC on 1 Dec is already 2 Dec in Istanbul.
	now := time.Date(2026, 12, 1, 22, 30, 0, 0, time.UTC)
	due := func(s string) pgtype.Date {
		d, _ := time.Parse(time.DateOnly, s)
		return pgtype.Date{Time: d, Valid: true}
	}
	cases := []struct {
		o    db.Order
		want bool
	}{
		{db.Order{Status: StatusPreparing, DueDate: due("2026-12-01")}, true},
		{db.Order{Status: StatusPreparing, DueDate: due("2026-12-02")}, false},
		{db.Order{Status: StatusShipped, DueDate: due("2026-11-01")}, false},
		{db.Order{Status: StatusApproved}, false},
	}
	for _, tc := range cases {
		if got := overdue(tc.o, now, zone); got != tc.want {
			t.Fatalf("%s %v: got %v", tc.o.Status, tc.o.DueDate.Time, got)
		}
	}
}

// TEC-1228: the shipping note shows the package count, desi and weight and
// never a price.
func TestShippingDocumentPackages(t *testing.T) {
	d := ShippingDoc{
		Locale: i18n.LocaleTR, OrderNo: "ORD-9", Zone: time.UTC, PackageCount: "4", Desi: "12.50", WeightKg: "7.25",
		Lines:       []ShippingLine{{SKU: "S1", Name: "Film", Amount: "2 adet", UnitPrice: "987.65", LineTotal: "1975.30"}},
		GeneratedAt: time.Date(2026, 10, 1, 9, 5, 0, 0, time.UTC),
	}
	out := ShippingDocumentHTML(d)
	for _, want := range []string{i18n.Translate(i18n.LocaleTR, "orders.pdf.package_count"), ">4<", "12.50", "7.25"} {
		if !strings.Contains(out, want) {
			t.Errorf("document misses %q", want)
		}
	}
	for _, price := range []string{"987.65", "1975.30"} {
		if strings.Contains(out, price) {
			t.Fatalf("shipping note carries a price %s", price)
		}
	}
}

// queueFixture adds three approved center → dealer orders to the invoicing
// fixture: normal without a due date (oldest), normal due tomorrow and
// urgent due next week.
func queueFixture(t *testing.T) (*invoicingFixture, string, []string) {
	t.Helper()
	f := newInvoicingFixture(t)
	q := db.New(f.tx)
	o, err := q.GetOrderByUUID(f.ctx, db.GetOrderByUUIDParams{Uuid: f.order, BrandID: f.center.Org.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("Q1228-%d", time.Now().UnixNano())
	rows := []struct{ prio, due string }{{"normal", ""}, {"normal", "1"}, {"urgent", "7"}}
	var nos []string
	for i, r := range rows {
		no := fmt.Sprintf("%s-%d", prefix, i)
		var due any
		if r.due != "" {
			due = time.Now().AddDate(0, 0, map[string]int{"1": 1, "7": 7}[r.due]).Format(time.DateOnly)
		}
		if _, err := f.tx.Exec(f.ctx, `INSERT INTO orders (
			order_no, organization_id, brand_id, seller_org_id, buyer_org_id, status, currency, subtotal, total,
			created_by_seller, priority, due_date, created_at
		) VALUES ($1, $2, $3, $2, $4, 'approved', 'TRY', 50, 50, true, $5, $6::date, NOW() - make_interval(hours => $7))`,
			no, o.SellerOrgID, o.BrandID, o.BuyerOrgID, r.prio, due, 10-i); err != nil {
			t.Fatalf("order %d: %v", i, err)
		}
		nos = append(nos, no)
	}
	return f, prefix, nos
}

func TestQueueSortAndFilters(t *testing.T) {
	f, prefix, nos := queueFixture(t)
	c := asRole(f.center, rbac.RoleCenterWarehouse)
	sort, err := ParseListFilter(map[string][]string{"sort": {"queue"}, "q": {prefix}, "side": {"seller"}, "limit": {"50"}})
	if err != nil {
		t.Fatal(err)
	}
	rows, total, err := f.svc.List(f.ctx, c, sort)
	if err != nil || total != 3 {
		t.Fatalf("list: total=%d err=%v", total, err)
	}
	// Urgent first, then the nearest due date, then no due date.
	want := []string{nos[2], nos[1], nos[0]}
	for i, r := range rows {
		if r.OrderNo != want[i] {
			t.Fatalf("row %d = %s, want %s", i, r.OrderNo, want[i])
		}
	}
	if rows[0].Priority != PriorityUrgent || rows[0].DueDate == nil || rows[2].DueDate != nil || rows[0].IsOverdue {
		t.Fatalf("queue fields: %+v", rows[0])
	}
	urgent, _ := ParseListFilter(map[string][]string{"priority": {"urgent"}, "q": {prefix}, "side": {"seller"}})
	if rows, _, err := f.svc.List(f.ctx, c, urgent); err != nil || len(rows) != 1 || rows[0].OrderNo != nos[2] {
		t.Fatalf("priority filter: %d rows, %v", len(rows), err)
	}
	if _, err := ParseListFilter(map[string][]string{"picker": {"someone"}}); err == nil {
		t.Fatal("bad picker accepted")
	}
}

func TestClaimFlow(t *testing.T) {
	f, prefix, nos := queueFixture(t)
	q := db.New(f.tx)
	other, err := q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Diğer", Surname: "Depocu", Status: "active",
		Email: pgtype.Text{String: prefix + "@example.test", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	me := asRole(f.center, rbac.RoleCenterWarehouse)
	them := me
	them.Principal.UserID, them.Principal.UserInternal = other.Uuid, other.ID
	manager := asRole(f.center, rbac.RoleCenterStaff)
	manager.Principal.UserID, manager.Principal.UserInternal = other.Uuid, other.ID

	var target db.Order
	if err := f.tx.QueryRow(f.ctx, `SELECT uuid FROM orders WHERE order_no = $1`, nos[0]).Scan(&target.Uuid); err != nil {
		t.Fatal(err)
	}

	v, err := f.svc.Claim(f.ctx, me, target.Uuid, false)
	if err != nil || !v.ClaimedByMe || v.Picker == nil || v.ClaimedAt == nil {
		t.Fatalf("claim: %+v %v", v.Picker, err)
	}
	// TEC-1050 / TEC-1228: the warehouse role still sees no money (the
	// fixture order has priced lines).
	if _, err := f.tx.Exec(f.ctx, `UPDATE orders SET status = 'preparing' WHERE uuid = $1`, f.order); err != nil {
		t.Fatal(err)
	}
	priced, err := f.svc.Claim(f.ctx, me, f.order, false)
	if err != nil || len(priced.Items) == 0 {
		t.Fatalf("claim priced: %v", err)
	}
	raw, _ := json.Marshal(priced)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m["total"] != nil || m["subtotal"] != nil || priced.Items[0].UnitPrice != nil || priced.Items[0].LineTotal != nil {
		t.Fatalf("claim response carries money: total=%v", m["total"])
	}
	// Again by me: no-op.
	if _, err := f.svc.Claim(f.ctx, me, target.Uuid, false); err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	// Someone else sees who holds it.
	seen, err := f.svc.Get(f.ctx, them, target.Uuid)
	if err != nil || seen.ClaimedByMe || seen.Picker == nil || seen.Picker.Name != "T1030 Accounting" {
		t.Fatalf("other view: %+v %v", seen.Picker, err)
	}
	var ce *ClaimedError
	if _, err := f.svc.Claim(f.ctx, them, target.Uuid, false); !errors.As(err, &ce) || !errors.Is(err, ErrAlreadyClaimed) || ce.Picker.Name != "T1030 Accounting" {
		t.Fatalf("claimed by other: %v", err)
	}
	// A plain warehouse user cannot take over or release it.
	if _, err := f.svc.Claim(f.ctx, them, target.Uuid, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("takeover by staff: %v", err)
	}
	if _, err := f.svc.ReleaseClaim(f.ctx, them, target.Uuid); !errors.Is(err, ErrForbidden) {
		t.Fatalf("release by staff: %v", err)
	}
	// The manager takes it over; the timeline names the previous holder.
	v, err = f.svc.Claim(f.ctx, manager, target.Uuid, true)
	if err != nil || !v.ClaimedByMe {
		t.Fatalf("takeover: %v", err)
	}
	found := false
	for _, e := range v.Timeline {
		if e.Action == "claim_taken_over" && e.PreviousPicker != nil && *e.PreviousPicker == "T1030 Accounting" {
			found = true
		}
	}
	if !found {
		t.Fatalf("timeline misses the takeover: %+v", v.Timeline)
	}
	// "My jobs" filter.
	mine, _ := ParseListFilter(map[string][]string{"picker": {"me"}, "q": {prefix}, "side": {"seller"}})
	if rows, _, err := f.svc.List(f.ctx, manager, mine); err != nil || len(rows) != 1 || rows[0].OrderNo != nos[0] {
		t.Fatalf("mine: %d %v", len(rows), err)
	}
	if rows, _, err := f.svc.List(f.ctx, me, mine); err != nil || len(rows) != 0 {
		t.Fatalf("mine for me: %d %v", len(rows), err)
	}
	none, _ := ParseListFilter(map[string][]string{"picker": {"none"}, "q": {prefix}, "side": {"seller"}})
	if rows, _, err := f.svc.List(f.ctx, me, none); err != nil || len(rows) != 2 {
		t.Fatalf("none: %d %v", len(rows), err)
	}
	if v, err = f.svc.ReleaseClaim(f.ctx, manager, target.Uuid); err != nil || v.Picker != nil {
		t.Fatalf("release: %+v %v", v.Picker, err)
	}
	// The buyer cannot claim.
	if _, err := f.svc.Claim(f.ctx, asRole(f.dealer, rbac.RoleDealerOwner), target.Uuid, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("buyer claim: %v", err)
	}
}

func TestSetSchedule(t *testing.T) {
	f, _, nos := queueFixture(t)
	var id db.Order
	if err := f.tx.QueryRow(f.ctx, `SELECT uuid FROM orders WHERE order_no = $1`, nos[0]).Scan(&id.Uuid); err != nil {
		t.Fatal(err)
	}
	day := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	// The warehouse role (orders.ship only) cannot change it.
	if _, err := f.svc.SetSchedule(f.ctx, asRole(f.center, rbac.RoleCenterWarehouse), id.Uuid, ScheduleInput{Priority: "urgent"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("warehouse: %v", err)
	}
	// The buyer cannot change an approved order.
	if _, err := f.svc.SetSchedule(f.ctx, asRole(f.dealer, rbac.RoleDealerOwner), id.Uuid, ScheduleInput{Priority: "urgent"}); !errors.Is(err, ErrScheduleLocked) {
		t.Fatalf("buyer: %v", err)
	}
	v, err := f.svc.SetSchedule(f.ctx, asRole(f.center, rbac.RoleCenterStaff), id.Uuid, ScheduleInput{Priority: "urgent", DueDate: &day})
	if err != nil || v.Priority != PriorityUrgent || v.DueDate == nil || *v.DueDate != day || !v.IsOverdue {
		t.Fatalf("seller: %+v %v", v.DueDate, err)
	}
	last := v.Timeline[len(v.Timeline)-1]
	if last.Action != "schedule_updated" || last.Priority == nil || *last.Priority != PriorityUrgent {
		t.Fatalf("timeline: %+v", last)
	}
}
