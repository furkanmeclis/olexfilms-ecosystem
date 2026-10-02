package usecase

import (
	"math/big"
	"reflect"
	"testing"

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
		{StatusPreparing, StatusCancelled, PartySeller, holds(rbac.PermOrdersCancel), true},
		{StatusPreparing, StatusCancelled, PartySeller, holds(rbac.PermOrdersWrite), false},
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
		{StatusDraft, StatusApproved}, {StatusProcessing, StatusCancelled}, {StatusApproved, StatusShipped},
		{StatusCancelled, StatusDraft}, {StatusPreparing, StatusReady}, {StatusSubmitted, StatusDraft},
	} {
		if _, ok := lookupTransition(m[0], m[1]); ok {
			t.Errorf("%s -> %s must not be allowed", m[0], m[1])
		}
	}
	for _, to := range []string{StatusShipped, StatusReceived, StatusCancelling, StatusDelivered, StatusReady} {
		if supportedTargets[to] {
			t.Errorf("%s belongs to TEC-167/168", to)
		}
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
