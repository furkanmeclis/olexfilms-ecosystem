package usecase

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// TEC-170: a reversed or empty created range is refused before any query runs.
func TestListRejectsReversedCreatedRange(t *testing.T) {
	from := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour)
	_, _, err := (&Service{}).List(context.Background(), Caller{}, ListFilter{
		Side: SideBuyer, CreatedFrom: &from, CreatedTo: &to,
	})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "created_to" {
		t.Fatalf("want created_to validation error, got %v", err)
	}
	same := from
	_, _, err = (&Service{}).List(context.Background(), Caller{}, ListFilter{
		Side: SideBuyer, CreatedFrom: &from, CreatedTo: &same,
	})
	if !errors.As(err, &ve) {
		t.Fatalf("an empty range must be refused, got %v", err)
	}
}

// TEC-373: the list parameters parse into the filter; bad values are 400s.
func TestParseListFilter(t *testing.T) {
	f, err := ParseListFilter(url.Values{
		"status": {"draft,shipped"}, "sort": {"-total"}, "total_min": {"10"}, "created_to": {"2026-10-02"},
		"buyer_org_uuid": {"6f1f9c2e-6b0e-4d0f-9d55-6d2a1d7b0c11"},
	})
	if err != nil || len(f.Statuses) != 2 || f.Sort.Key != "total" || !f.Sort.Desc || !f.SortExplicit ||
		f.TotalMin == nil || *f.TotalMin != 10 || len(f.BuyerOrgUUIDs) != 1 ||
		!f.CreatedTo.Equal(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)) || !f.sqlOnly() {
		t.Fatalf("filter = %+v, %v", f, err)
	}
	if f, _ := ParseListFilter(url.Values{"q": {"x"}}); f.sqlOnly() || f.Sort.Key != "created_at" || !f.Sort.Desc {
		t.Fatalf("default filter = %+v", f)
	}
	for _, bad := range []url.Values{
		{"status": {"bogus"}}, {"sort": {"buyer"}}, {"total_max": {"x"}}, {"seller_org_uuid": {"nope"}},
	} {
		var ve *apiquery.ValidationError
		if _, err := ParseListFilter(bad); !errors.As(err, &ve) {
			t.Errorf("%v: want validation error, got %v", bad, err)
		}
	}
	if got := ListValues(map[string]string{"q": " a ", "limit": "5", "_scope": "brand"}); got.Encode() != "q=a" {
		t.Fatalf("list values = %v", got)
	}
}
