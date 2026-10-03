package usecase

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

func holds(slugs ...string) func(string) bool {
	return func(s string) bool {
		for _, v := range slugs {
			if v == s {
				return true
			}
		}
		return false
	}
}

func TestTransitionRules(t *testing.T) {
	all := holds(rbac.PermOrdersWrite, rbac.PermOrdersApprove, rbac.PermOrdersShip, rbac.PermOrdersCancel)
	cases := []struct {
		from, to string
		party    Party
		can      func(string) bool
		ok       bool
	}{
		{StatusDraft, StatusSubmitted, PartyBuyer, all, true},
		{StatusDraft, StatusSubmitted, PartySeller, all, false},
		{StatusSubmitted, StatusApproved, PartySeller, holds(rbac.PermOrdersApprove), true},
		{StatusSubmitted, StatusApproved, PartyBuyer, all, false},
		{StatusSubmitted, StatusApproved, PartySeller, holds(rbac.PermOrdersShip), false},
		{StatusApproved, StatusPreparing, PartySeller, holds(rbac.PermOrdersShip), true},
		{StatusApproved, StatusProcessing, PartySeller, holds(rbac.PermOrdersApprove), true},
		{StatusApproved, StatusCancelled, PartyBuyer, holds(rbac.PermOrdersCancel), true},
		{StatusProcessing, StatusPreparing, PartySeller, holds(rbac.PermOrdersShip), true},
		{StatusProcessing, StatusPreparing, PartySeller, holds(rbac.PermOrdersApprove), false},
		{StatusProcessing, StatusPreparing, PartyBuyer, all, false},
		{StatusProcessing, StatusCancelled, PartySeller, holds(rbac.PermOrdersCancel), true},
		{StatusProcessing, StatusCancelled, PartyBuyer, holds(rbac.PermOrdersCancel), true},
		{StatusProcessing, StatusCancelled, PartySeller, holds(rbac.PermOrdersShip), false},
		{StatusPreparing, StatusCancelled, PartySeller, holds(rbac.PermOrdersCancel), true},
		{StatusPreparing, StatusCancelled, PartySeller, holds(rbac.PermOrdersWrite), false},
		{StatusPreparing, StatusReady, PartySeller, holds(rbac.PermOrdersShip), true},
		{StatusPreparing, StatusReady, PartyBuyer, all, false},
		{StatusPreparing, StatusReady, PartySeller, holds(rbac.PermOrdersApprove), false},
		{StatusReady, StatusShipped, PartySeller, holds(rbac.PermOrdersShip), true},
		{StatusReady, StatusShipped, PartyBuyer, all, false},
		{StatusReady, StatusCancelled, PartyBuyer, holds(rbac.PermOrdersCancel), true},
		{StatusShipped, StatusReceived, PartyBuyer, holds(rbac.PermOrdersReceive), true},
		{StatusShipped, StatusReceived, PartySeller, holds(rbac.PermOrdersReceive), false},
		{StatusShipped, StatusReceived, PartyBuyer, all, false},
		{StatusShipped, StatusCancelling, PartyBuyer, holds(rbac.PermOrdersCancel), true},
		{StatusShipped, StatusCancelling, PartySeller, holds(rbac.PermOrdersCancel), true},
		{StatusShipped, StatusCancelling, PartySeller, holds(rbac.PermOrdersShip), false},
		{StatusCancelling, StatusCancelled, PartySeller, holds(rbac.PermOrdersShip), true},
		{StatusCancelling, StatusCancelled, PartySeller, holds(rbac.PermOrdersCancel), true},
		{StatusCancelling, StatusCancelled, PartyBuyer, holds(rbac.PermOrdersCancel), false},
	}
	for _, c := range cases {
		r, ok := lookupTransition(c.from, c.to)
		got := ok && r.allows(c.party, c.can)
		if got != c.ok {
			t.Errorf("%s -> %s by %s = %v, want %v", c.from, c.to, c.party, got, c.ok)
		}
	}
	// Invalid or stock-bound moves have no rule.
	for _, m := range [][2]string{
		{StatusDraft, StatusApproved}, {StatusProcessing, StatusReady}, {StatusProcessing, StatusShipped}, {StatusApproved, StatusShipped},
		{StatusCancelled, StatusDraft}, {StatusApproved, StatusReady}, {StatusSubmitted, StatusDraft},
		{StatusPreparing, StatusShipped}, {StatusShipped, StatusCancelled},
		{StatusReceived, StatusCancelling}, {StatusReceived, StatusCancelled}, {StatusShipped, StatusDelivered},
		{StatusDelivered, StatusReceived}, {StatusCancelling, StatusShipped},
	} {
		if _, ok := lookupTransition(m[0], m[1]); ok {
			t.Errorf("%s -> %s must not be allowed", m[0], m[1])
		}
	}
	if supportedTargets[StatusDelivered] {
		t.Errorf("delivered is not used (TEC-168)")
	}
	// Every allowed target is supported and has an outbox event.
	for from, m := range transitions {
		for to := range m {
			if !supportedTargets[to] || eventFor(to) == "" || events.ValidateEventName(eventFor(to)) != nil {
				t.Errorf("%s -> %s: target not wired", from, to)
			}
		}
	}
}

func TestAvailableTransitions(t *testing.T) {
	seller := holds(rbac.PermOrdersApprove, rbac.PermOrdersCancel)
	if got := availableTransitions(StatusSubmitted, PartySeller, seller); !reflect.DeepEqual(got, []string{StatusApproved, StatusCancelled}) {
		t.Fatalf("seller on submitted = %v", got)
	}
	if got := availableTransitions(StatusSubmitted, PartyBuyer, holds(rbac.PermOrdersWrite)); len(got) != 0 {
		t.Fatalf("buyer without cancel on submitted = %v", got)
	}
	if got := availableTransitions(StatusProcessing, PartySeller, holds(rbac.PermOrdersShip, rbac.PermOrdersCancel)); !reflect.DeepEqual(got, []string{StatusPreparing, StatusCancelled}) {
		t.Fatalf("seller on processing = %v", got)
	}
	if got := availableTransitions(StatusProcessing, PartyBuyer, holds(rbac.PermOrdersCancel)); !reflect.DeepEqual(got, []string{StatusCancelled}) {
		t.Fatalf("buyer on processing = %v", got)
	}
	if got := availableTransitions(StatusDraft, "", seller); len(got) != 0 {
		t.Fatalf("observer = %v", got)
	}
}

func TestMoney(t *testing.T) {
	cases := []struct {
		price  string
		amount *big.Rat
		want   string
	}{
		{"80", big.NewRat(3, 1), "240.00"},
		{"12.3456", big.NewRat(3, 1), "37.04"},
		{"0.005", big.NewRat(1, 1), "0.01"},
		{"19.99", big.NewRat(25, 2), "249.88"}, // 12.5 m
	}
	for _, c := range cases {
		got, err := lineTotal(c.price, c.amount)
		if err != nil || got != c.want {
			t.Errorf("lineTotal(%s, %s) = %s %v, want %s", c.price, c.amount.FloatString(2), got, err, c.want)
		}
	}
	if s, _ := sumTotals([]string{"240.00", "37.04"}); s != "277.04" {
		t.Fatalf("sum = %s", s)
	}
	for raw, want := range map[string]string{"12.5": "12.50", "3": "3.00"} {
		if got, ok := normalizeMeters(raw); !ok || got != want {
			t.Errorf("meters %s = %s %v", raw, got, ok)
		}
	}
	for _, raw := range []string{"0", "-1", "1.234", "abc", "1e3"} {
		if _, ok := normalizeMeters(raw); ok {
			t.Errorf("meters %q must be rejected", raw)
		}
	}
	n, _ := numeric("1.2345000000")
	if got := rateText(n); got == nil || *got != "1.2345" {
		t.Fatalf("rateText = %v", got)
	}
	if numericTextPtr(pgtype.Numeric{}, 2) != nil {
		t.Fatal("NULL stays null")
	}
}

func TestAssignedUnits(t *testing.T) {
	one := pgtype.Int4{Int32: 1, Valid: true}
	three := pgtype.Int4{Int32: 3, Valid: true}
	units, sum := assignedUnits([]db.ListOrderItemUnitsByOrderRow{
		{Barcode: "A", UnitKind: "serial", Quantity: one, MovementID: pgtype.Int8{Int64: 9, Valid: true}},
		{Barcode: "B", UnitKind: "fixed", Quantity: three},
	}, false)
	if sum != "4" || len(units) != 2 || !units[0].Shipped || units[1].Shipped || *units[1].Quantity != 3 {
		t.Fatalf("pieces = %s %+v", sum, units)
	}
	m, _ := numeric("12.50")
	units, sum = assignedUnits([]db.ListOrderItemUnitsByOrderRow{{Barcode: "R", UnitKind: "serial", Meters: m}}, true)
	if sum != "12.50" || units[0].Meters == nil || *units[0].Meters != "12.50" || units[0].Quantity != nil {
		t.Fatalf("roll = %s %+v", sum, units)
	}
	if _, sum = assignedUnits(nil, true); sum != "0.00" {
		t.Fatalf("empty = %s", sum)
	}
}

func TestStatusLabels(t *testing.T) {
	for _, st := range Statuses {
		key := StatusLabelKey(st)
		for _, loc := range []i18n.Locale{i18n.LocaleTR, i18n.LocaleEN, i18n.LocaleAR} {
			if i18n.Translate(loc, key) == key {
				t.Errorf("%s missing in %s", key, loc)
			}
		}
	}
}
