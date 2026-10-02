package usecase

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
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
	req, appr := holds(rbac.PermTransfersRequest), holds(rbac.PermTransfersApprove)
	cases := []struct {
		from, to string
		party    Party
		can      func(string) bool
		ok       bool
	}{
		{StatusRequested, StatusApproved, PartyReceiver, req, true},
		{StatusRequested, StatusApproved, PartyParent, appr, true},
		{StatusRequested, StatusApproved, PartyParent, req, false},
		{StatusRequested, StatusApproved, PartySender, req, false},
		{StatusRequested, StatusRejected, PartyReceiver, req, true},
		{StatusRequested, StatusRejected, PartySender, req, false},
		{StatusRequested, StatusShipped, PartySender, req, false},
		{StatusRequested, StatusCancelled, PartySender, req, true},
		{StatusRequested, StatusCancelled, PartyReceiver, req, false},
		{StatusApproved, StatusShipped, PartySender, req, true},
		{StatusApproved, StatusShipped, PartyReceiver, req, false},
		{StatusApproved, StatusCancelled, PartyReceiver, req, true},
		{StatusShipped, StatusReceived, PartyReceiver, req, true},
		{StatusShipped, StatusReceived, PartySender, req, false},
		{StatusShipped, StatusCancelled, PartySender, req, true},
		{StatusShipped, StatusCancelled, PartyReceiver, req, false},
		{StatusRejected, StatusApproved, PartyReceiver, req, false},
		{StatusReceived, StatusCancelled, PartySender, req, false},
		{StatusCancelled, StatusRequested, PartySender, req, false},
	}
	for _, c := range cases {
		gs, found := lookupTransition(c.from, c.to)
		got := found && allowed(gs, c.party, c.can)
		if got != c.ok {
			t.Errorf("%s -> %s by %s = %v, want %v", c.from, c.to, c.party, got, c.ok)
		}
	}
}

func TestAvailableTransitions(t *testing.T) {
	req := holds(rbac.PermTransfersRequest)
	if got := availableTransitions(StatusRequested, PartyReceiver, req); !reflect.DeepEqual(got, []string{StatusApproved, StatusRejected}) {
		t.Fatalf("receiver on requested = %v", got)
	}
	if got := availableTransitions(StatusApproved, PartySender, req); !reflect.DeepEqual(got, []string{StatusShipped, StatusCancelled}) {
		t.Fatalf("sender on approved = %v", got)
	}
	if got := availableTransitions(StatusShipped, PartyReceiver, req); !reflect.DeepEqual(got, []string{StatusReceived}) {
		t.Fatalf("receiver on shipped = %v", got)
	}
	if got := availableTransitions(StatusRequested, "", req); len(got) != 0 {
		t.Fatalf("observer = %v", got)
	}
	if got := availableTransitions(StatusReceived, PartyReceiver, req); len(got) != 0 {
		t.Fatalf("final = %v", got)
	}
}

func TestEventForEveryTarget(t *testing.T) {
	known := map[string]bool{}
	for _, e := range events.AllKnownEvents() {
		known[e] = true
	}
	for from, tos := range transitions {
		for to := range tos {
			name := eventFor(to)
			if name == "" || !known[name] {
				t.Errorf("%s -> %s has no catalog event (%q)", from, to, name)
			}
		}
	}
}

func TestSibling(t *testing.T) {
	parent := pgtype.Int8{Int64: 10, Valid: true}
	other := pgtype.Int8{Int64: 11, Valid: true}
	a := db.Organization{ID: 1, Type: OrgDealer, BrandID: 1, ParentID: parent}
	cases := []struct {
		name   string
		target db.Organization
		ok     bool
	}{
		{"sibling dealer", db.Organization{ID: 2, Type: OrgDealer, BrandID: 1, ParentID: parent}, true},
		{"other parent", db.Organization{ID: 3, Type: OrgDealer, BrandID: 1, ParentID: other}, false},
		{"other brand", db.Organization{ID: 4, Type: OrgDealer, BrandID: 2, ParentID: parent}, false},
		{"other type", db.Organization{ID: 5, Type: OrgDistributor, BrandID: 1, ParentID: parent}, false},
		{"self", a, false},
	}
	for _, c := range cases {
		p, ok := sibling(a, c.target)
		if ok != c.ok || (ok && p != 10) {
			t.Errorf("%s: sibling = %d %v, want %v", c.name, p, ok, c.ok)
		}
	}
	center := db.Organization{ID: 9, Type: "center", BrandID: 1}
	if _, ok := sibling(center, db.Organization{ID: 8, Type: "center", BrandID: 1}); ok {
		t.Fatal("centers never transfer")
	}
}

func TestLineAmount(t *testing.T) {
	m, _ := numeric("12.50")
	cases := []struct {
		row  db.ListTransferRequestItemsRow
		want *big.Rat
	}{
		{db.ListTransferRequestItemsRow{Quantity: pgtype.Int4{Int32: 3, Valid: true}}, big.NewRat(3, 1)},
		{db.ListTransferRequestItemsRow{Meters: m, ProductUnitType: "roll_meter"}, big.NewRat(25, 2)},
		{db.ListTransferRequestItemsRow{Meters: m, ProductUnitType: "piece"}, big.NewRat(1, 1)},
		{db.ListTransferRequestItemsRow{}, big.NewRat(1, 1)},
	}
	for i, c := range cases {
		if got := lineAmount(c.row); got.Cmp(c.want) != 0 {
			t.Errorf("case %d: %s, want %s", i, got.RatString(), c.want.RatString())
		}
	}
}
