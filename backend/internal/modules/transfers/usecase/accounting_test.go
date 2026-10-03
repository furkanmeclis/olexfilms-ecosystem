package usecase

import (
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-200: the booked amount is the sum of the frozen line totals; an
// unpriced request books nothing.
func TestFrozenTotal(t *testing.T) {
	line := func(s string) db.ListTransferRequestItemsRow {
		var r db.ListTransferRequestItemsRow
		if s != "" {
			n, err := numeric(s)
			if err != nil {
				t.Fatal(err)
			}
			r.LineTotal = n
		}
		return r
	}
	if got, ok := frozenTotal([]db.ListTransferRequestItemsRow{line("70.00"), line("12.345"), line("")}); !ok || got != "82.35" {
		t.Fatalf("total = %q %v", got, ok)
	}
	if _, ok := frozenTotal([]db.ListTransferRequestItemsRow{line("")}); ok {
		t.Fatal("an unpriced request must book nothing")
	}
}

// TEC-200: who hears about each transfer event.
func TestNotifyParties(t *testing.T) {
	cases := map[string][]Party{
		events.TransfersRequested: {PartyReceiver, PartyParent},
		events.TransfersApproved:  {PartySender, PartyReceiver},
		events.TransfersRejected:  {PartySender, PartyReceiver},
		events.TransfersShipped:   {PartyReceiver},
		events.TransfersReceived:  {PartySender},
		events.TransfersCancelled: {PartySender, PartyReceiver, PartyParent},
	}
	for ev, want := range cases {
		if got := notifyParties(ev); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", ev, got, want)
		}
	}
	for _, to := range Statuses {
		if eventFor(to) != "" && notifyParties(eventFor(to)) == nil {
			t.Errorf("%s has no recipients", to)
		}
	}
}
