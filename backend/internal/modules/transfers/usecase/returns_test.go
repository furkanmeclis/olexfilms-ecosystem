package usecase

import (
	"reflect"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// TEC-223: on a return the parent decides and receives; the giver ships or
// cancels. The receiver role (= parent) never appears.
func TestReturnTransitions(t *testing.T) {
	req, appr := holds(rbac.PermTransfersRequest), holds(rbac.PermTransfersApprove)
	cases := []struct {
		from  string
		party Party
		can   func(string) bool
		want  []string
	}{
		{StatusRequested, PartyParent, appr, []string{StatusApproved, StatusRejected, StatusCancelled}},
		{StatusRequested, PartyParent, req, []string{}},
		{StatusRequested, PartySender, req, []string{StatusCancelled}},
		{StatusApproved, PartySender, req, []string{StatusShipped, StatusCancelled}},
		{StatusApproved, PartyParent, appr, []string{StatusCancelled}},
		{StatusShipped, PartyParent, req, []string{StatusReceived}},
		{StatusShipped, PartyParent, appr, []string{StatusReceived}},
		{StatusShipped, PartySender, req, []string{StatusCancelled}},
		{StatusShipped, PartyReceiver, req, []string{}},
		{StatusReceived, PartyParent, appr, []string{}},
	}
	for _, c := range cases {
		if got := availableKindTransitions(KindReturn, c.from, c.party, c.can); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s as %s = %v, want %v", c.from, c.party, got, c.want)
		}
	}
	for from, tos := range returnTransitions {
		for to := range tos {
			if eventFor(to) == "" {
				t.Errorf("%s -> %s has no event", from, to)
			}
		}
	}
	// The sibling machine is unchanged by the kind.
	if got := availableKindTransitions(KindSibling, StatusRequested, PartyReceiver, req); !reflect.DeepEqual(got, []string{StatusApproved, StatusRejected}) {
		t.Fatalf("sibling receiver = %v", got)
	}
}

// TEC-223: the parent of a return is its parent side even though it is
// also to_org_id.
func TestReturnPartyOf(t *testing.T) {
	r := db.StockTransferRequest{Kind: KindReturn, BrandID: 1, FromOrgID: 5, ToOrgID: 9, ApproverOrgID: 9}
	at := func(org int64) Caller {
		return Caller{Principal: authctx.Principal{}, Org: orgctx.Scope{InternalID: org, BrandID: 1}}
	}
	if p := partyOf(at(5), r); p != PartySender {
		t.Fatalf("child = %q", p)
	}
	if p := partyOf(at(9), r); p != PartyParent {
		t.Fatalf("parent = %q", p)
	}
	if visible(at(7), r) {
		t.Fatal("outsider sees the return")
	}
}

func TestReturnNotifyParties(t *testing.T) {
	cases := map[string][]Party{
		events.TransfersRequested: {PartyParent},
		events.TransfersApproved:  {PartySender},
		events.TransfersRejected:  {PartySender},
		events.TransfersShipped:   {PartyParent},
		events.TransfersReceived:  {PartySender},
		events.TransfersCancelled: {PartySender, PartyParent},
	}
	for ev, want := range cases {
		if got := notifyParties(KindReturn, ev); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", ev, got, want)
		}
	}
}

func TestAccountingSourceByKind(t *testing.T) {
	id := uuid.New()
	if s := AccountingSource(db.StockTransferRequest{Uuid: id, Kind: KindReturn}); s.Type != "stock_return" || s.UUID != id {
		t.Fatalf("return source = %+v", s)
	}
	if s := AccountingSource(db.StockTransferRequest{Uuid: id, Kind: KindSibling}); s.Type != "stock_transfer" {
		t.Fatalf("sibling source = %+v", s)
	}
}
