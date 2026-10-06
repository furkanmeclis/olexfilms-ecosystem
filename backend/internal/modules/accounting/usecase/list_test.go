package usecase

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

func TestParseAccountingListFilters(t *testing.T) {
	e, err := ParseEntryFilter(url.Values{
		"direction": {"income,expense"}, "category": {"rent"}, "source_type": {"manual,order"},
		"date_from": {"2026-10-01"}, "date_to": {"2026-10-02"}, "amount_min": {"10"}, "q": {"50%"}, "sort": {"-amount"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(e.Directions, ",") != "income,expense" || len(e.SourceTypes) != 2 || e.Sort.Key != "amount" || !e.Sort.Desc ||
		!e.CreatedBefore.Equal(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)) || *e.AmountMin != 10 {
		t.Fatalf("entry filter = %+v", e)
	}
	if likeArg(e.Q).String != `50\%` {
		t.Fatalf("q = %q", likeArg(e.Q).String)
	}
	c, err := ParseCariFilter(url.Values{"counterparty_kind": {"dealer,customer"}, "balance_max": {"0"}})
	if err != nil || len(c.Kinds) != 2 || *c.BalanceMax != 0 || c.Sort.Key != "name" || c.Sort.Desc {
		t.Fatalf("cari filter = %+v, %v", c, err)
	}
	a, err := ParseAccountFilter(url.Values{"type": {"cash"}, "active": {"true"}})
	if err != nil || a.Types[0] != "cash" || !*a.Active || a.Sort.Key != "type" {
		t.Fatalf("account filter = %+v, %v", a, err)
	}
	d, err := ParseDisputeFilter(url.Values{"status": {"open,rejected"}, "created_from": {"2026-10-01"}})
	if err != nil || len(d.Statuses) != 2 || d.CreatedFrom == nil || d.Sort.Key != "created_at" || !d.Sort.Desc {
		t.Fatalf("dispute filter = %+v, %v", d, err)
	}
	if got := EntryListValues(map[string]string{"q": "x", "limit": "5", "book_organization_id": "1"}); got.Encode() != "q=x" {
		t.Fatalf("entry list values = %s", got.Encode())
	}
	for name, fn := range map[string]func() error{
		"entry direction": func() error { _, err := ParseEntryFilter(url.Values{"direction": {"refund"}}); return err },
		"entry category":  func() error { _, err := ParseEntryFilter(url.Values{"category": {"Rent"}}); return err },
		"entry source":    func() error { _, err := ParseEntryFilter(url.Values{"source_type": {"x-y"}}); return err },
		"entry sort":      func() error { _, err := ParseEntryFilter(url.Values{"sort": {"description"}}); return err },
		"entry both ranges": func() error {
			_, err := ParseEntryFilter(url.Values{"date_from": {"2026-10-01"}, "created_from": {"2026-10-01"}})
			return err
		},
		"cari kind":     func() error { _, err := ParseCariFilter(url.Values{"counterparty_kind": {"supplier"}}); return err },
		"cari balance":  func() error { _, err := ParseCariFilter(url.Values{"balance_min": {"x"}}); return err },
		"account type":  func() error { _, err := ParseAccountFilter(url.Values{"type": {"card"}}); return err },
		"account uuid":  func() error { _, err := ParseAccountFilter(url.Values{"organization_uuid": {"x"}}); return err },
		"dispute org":   func() error { _, err := ParseDisputeFilter(url.Values{"organization_uuid": {"x"}}); return err },
		"dispute state": func() error { _, err := ParseDisputeFilter(url.Values{"status": {"closed"}}); return err },
	} {
		var ve *apiquery.ValidationError
		if err := fn(); !errors.As(err, &ve) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}
