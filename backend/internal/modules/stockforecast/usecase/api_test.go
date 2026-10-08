package usecase

import (
	"net/url"
	"testing"
)

func TestParseListFilterContract(t *testing.T) {
	t.Run("sort status and days range", func(t *testing.T) {
		f, err := ParseListFilter(url.Values{
			"sort": {"-days_left"}, "status": {"critical,warning"}, "days_left_min": {"1"}, "days_left_max": {"14"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if f.SortKey != "days_left" || !f.SortDesc || len(f.Statuses) != 2 || *f.DaysLeftMin != 1 || *f.DaysLeftMax != 14 {
			t.Fatalf("filter = %+v", f)
		}
	})
	t.Run("unknown sort is validation error", func(t *testing.T) {
		if _, err := ParseListFilter(url.Values{"sort": {"bogus"}}); err == nil {
			t.Fatal("expected validation error")
		}
	})
	t.Run("bad status is validation error", func(t *testing.T) {
		if _, err := ParseListFilter(url.Values{"status": {"critical,bogus"}}); err == nil {
			t.Fatal("expected validation error")
		}
	})
}
