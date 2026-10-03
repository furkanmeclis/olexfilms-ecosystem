package posting

import (
	"context"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
)

type countingResolver struct {
	calls int
	rate  string
}

func (r *countingResolver) ResolveRate(_ context.Context, on time.Time, base, quote string) (fxrates.Snapshot, error) {
	r.calls++
	return fxrates.Snapshot{Base: base, Quote: quote, Rate: r.rate, RateDate: on.Format(time.DateOnly), Source: "tcmb"}, nil
}

// TEC-226 (K7): a pair frozen in the snapshot is used as is; the day's rate
// is only looked up for snapshots without that pair (orders approved earlier).
func TestConversionPrefersFrozenPair(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	res := &countingResolver{rate: "99"}
	p := New(nil, nil, res)
	frozen := &fxrates.Snapshot{
		Base: "EUR", Quote: "TRY", Rate: "35", RateDate: "2026-10-03", Source: "tcmb",
		Pairs: []fxrates.Snapshot{{Base: "EUR", Quote: "UAH", Rate: "45.5", RateDate: "2026-10-03", Source: "tcmb"}},
	}

	conv, err := p.conversion(context.Background(), "EUR", "UAH", Entry{RateDate: day, Rate: frozen})
	if err != nil {
		t.Fatal(err)
	}
	if got := conv.rate.FloatString(1); got != "45.5" || res.calls != 0 {
		t.Fatalf("frozen pair: rate %s, resolver calls %d", got, res.calls)
	}

	conv, err = p.conversion(context.Background(), "EUR", "TRY", Entry{RateDate: day, Rate: frozen})
	if err != nil {
		t.Fatal(err)
	}
	if got := conv.rate.FloatString(0); got != "35" || res.calls != 0 {
		t.Fatalf("top-level pair: rate %s, resolver calls %d", got, res.calls)
	}

	legacy := &fxrates.Snapshot{Base: "EUR", Quote: "TRY", Rate: "35", RateDate: "2026-10-03", Source: "tcmb"}
	conv, err = p.conversion(context.Background(), "EUR", "UAH", Entry{RateDate: day, Rate: legacy})
	if err != nil {
		t.Fatal(err)
	}
	if got := conv.rate.FloatString(0); got != "99" || res.calls != 1 {
		t.Fatalf("legacy snapshot: rate %s, resolver calls %d", got, res.calls)
	}
}
