package fxrates

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// memStore mirrors the SQL of FindPairRate: latest day in [min, on], then
// manual > tcmb > ecb, then direct before inverse.
type memStore struct {
	rows []db.UpsertExchangeRateParams
}

func (m *memStore) UpsertExchangeRate(_ context.Context, arg db.UpsertExchangeRateParams) error {
	for i, r := range m.rows {
		if r.RateDate == arg.RateDate && r.Base == arg.Base && r.Quote == arg.Quote && r.Source == arg.Source {
			m.rows[i] = arg
			return nil
		}
	}
	m.rows = append(m.rows, arg)
	return nil
}

func (m *memStore) DeleteManualExchangeRate(_ context.Context, arg db.DeleteManualExchangeRateParams) (int64, error) {
	var n int64
	kept := m.rows[:0]
	for _, r := range m.rows {
		if r.Source == SourceManual && r.RateDate == arg.RateDate && r.Base == arg.Base && r.Quote == arg.Quote {
			n++
			continue
		}
		kept = append(kept, r)
	}
	m.rows = kept
	return n, nil
}

func priority(s string) int {
	switch s {
	case SourceManual:
		return 0
	case SourceTCMB:
		return 1
	default:
		return 2
	}
}

func (m *memStore) FindPairRate(_ context.Context, arg db.FindPairRateParams) (db.FindPairRateRow, error) {
	var cand []db.UpsertExchangeRateParams
	for _, r := range m.rows {
		if r.RateDate.Time.After(arg.OnDate.Time) || r.RateDate.Time.Before(arg.MinDate.Time) {
			continue
		}
		if (r.Base == arg.Base && r.Quote == arg.Quote) || (r.Base == arg.Quote && r.Quote == arg.Base) {
			cand = append(cand, r)
		}
	}
	if len(cand) == 0 {
		return db.FindPairRateRow{}, pgx.ErrNoRows
	}
	sort.SliceStable(cand, func(i, j int) bool {
		a, b := cand[i], cand[j]
		if !a.RateDate.Time.Equal(b.RateDate.Time) {
			return a.RateDate.Time.After(b.RateDate.Time)
		}
		if priority(a.Source) != priority(b.Source) {
			return priority(a.Source) < priority(b.Source)
		}
		return a.Base == arg.Base && b.Base != arg.Base
	})
	r := cand[0]
	return db.FindPairRateRow{RateDate: r.RateDate, Base: r.Base, Quote: r.Quote, Rate: r.Rate, Source: r.Source}, nil
}

func (m *memStore) ListExchangeRatesByDate(context.Context, db.ListExchangeRatesByDateParams) ([]db.ListExchangeRatesByDateRow, error) {
	return nil, nil
}

func (m *memStore) LatestExchangeRateDate(_ context.Context, d pgtype.Date) (pgtype.Date, error) {
	return d, nil
}

func (m *memStore) ListCurrencies(context.Context, bool) ([]db.Currency, error) { return nil, nil }

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func seed(t *testing.T, svc *Service, d Day) {
	t.Helper()
	if err := svc.Store(context.Background(), d); err != nil {
		t.Fatal(err)
	}
}

func TestResolveOverridePriority(t *testing.T) {
	ctx := context.Background()
	svc := New(&memStore{}, nil, nil)
	seed(t, svc, Day{Source: SourceECB, Date: day("2026-09-30"), Rates: []Rate{{Base: "EUR", Quote: "TRY", Rate: "48.70"}}})
	seed(t, svc, Day{Source: SourceTCMB, Date: day("2026-09-30"), Rates: []Rate{{Base: "EUR", Quote: "TRY", Rate: "48.80"}}})

	snap, err := svc.ResolveRate(ctx, day("2026-09-30"), "EUR", "TRY")
	if err != nil || snap.Source != SourceTCMB || snap.Rate != "48.8" {
		t.Fatalf("tcmb before ecb: %+v %v", snap, err)
	}

	if _, err := svc.SetOverride(ctx, OverrideInput{Date: day("2026-09-30"), Base: "eur", Quote: "try", Rate: "50", ActorID: 1}); err != nil {
		t.Fatal(err)
	}
	snap, err = svc.ResolveRate(ctx, day("2026-09-30"), "EUR", "TRY")
	if err != nil || snap.Source != SourceManual || snap.Rate != "50" {
		t.Fatalf("manual wins: %+v %v", snap, err)
	}

	// Clearing the override brings the fetched value back.
	if err := svc.ClearOverride(ctx, day("2026-09-30"), "EUR", "TRY"); err != nil {
		t.Fatal(err)
	}
	snap, _ = svc.ResolveRate(ctx, day("2026-09-30"), "EUR", "TRY")
	if snap.Source != SourceTCMB {
		t.Fatalf("after clear: %+v", snap)
	}
	if err := svc.ClearOverride(ctx, day("2026-09-30"), "EUR", "TRY"); !errors.Is(err, ErrRateNotFound) {
		t.Fatalf("second clear = %v", err)
	}
}

func TestResolvePreviousDayAndLookback(t *testing.T) {
	ctx := context.Background()
	svc := New(&memStore{}, nil, nil)
	// Friday's TCMB rate serves Saturday and Sunday.
	seed(t, svc, Day{Source: SourceTCMB, Date: day("2026-09-25"), Rates: []Rate{{Base: "USD", Quote: "TRY", Rate: "41.2"}}})
	snap, err := svc.ResolveRate(ctx, day("2026-09-27"), "USD", "TRY")
	if err != nil || snap.RateDate != "2026-09-25" {
		t.Fatalf("weekend: %+v %v", snap, err)
	}
	// A rate older than MaxLookback is not used; a later rate is not used for an earlier day.
	if _, err := svc.ResolveRate(ctx, day("2026-10-20"), "USD", "TRY"); !errors.Is(err, ErrRateNotFound) {
		t.Fatalf("stale rate used: %v", err)
	}
	if _, err := svc.ResolveRate(ctx, day("2026-09-24"), "USD", "TRY"); !errors.Is(err, ErrRateNotFound) {
		t.Fatalf("future rate used: %v", err)
	}
	// A newer day wins over an older manual override.
	if _, err := svc.SetOverride(ctx, OverrideInput{Date: day("2026-09-24"), Base: "USD", Quote: "TRY", Rate: "40"}); err != nil {
		t.Fatal(err)
	}
	snap, _ = svc.ResolveRate(ctx, day("2026-09-26"), "USD", "TRY")
	if snap.Source != SourceTCMB || snap.RateDate != "2026-09-25" {
		t.Fatalf("latest day first: %+v", snap)
	}
}

func TestResolveInverseAndCross(t *testing.T) {
	ctx := context.Background()
	svc := New(&memStore{}, nil, nil)
	seed(t, svc, Day{Source: SourceTCMB, Date: day("2026-09-30"), Rates: []Rate{
		{Base: "EUR", Quote: "TRY", Rate: "50"},
		{Base: "AZN", Quote: "TRY", Rate: "25"},
	}})
	seed(t, svc, Day{Source: SourceECB, Date: day("2026-09-29"), Rates: []Rate{{Base: "EUR", Quote: "GBP", Rate: "0.8"}}})

	snap, err := svc.ResolveRate(ctx, day("2026-09-30"), "TRY", "EUR")
	if err != nil || snap.Rate != "0.02" || snap.Via != "" {
		t.Fatalf("inverse: %+v %v", snap, err)
	}
	// EUR→AZN: no direct quote, cross through TRY (50 / 25).
	snap, err = svc.ResolveRate(ctx, day("2026-09-30"), "EUR", "AZN")
	if err != nil || snap.Rate != "2" || snap.Via != "TRY" || snap.Source != SourceTCMB {
		t.Fatalf("cross via TRY: %+v %v", snap, err)
	}
	// GBP→TRY: cross through EUR (1/0.8 * 50), dated by the older leg.
	snap, err = svc.ResolveRate(ctx, day("2026-09-30"), "GBP", "TRY")
	if err != nil || snap.Rate != "62.5" || snap.Via != "EUR" || snap.RateDate != "2026-09-29" || snap.Source != "ecb+tcmb" {
		t.Fatalf("cross via EUR: %+v %v", snap, err)
	}
	snap, err = svc.ResolveRate(ctx, day("2026-09-30"), "TRY", "TRY")
	if err != nil || snap.Rate != "1" {
		t.Fatalf("identity: %+v %v", snap, err)
	}
	if _, err := svc.ResolveRate(ctx, day("2026-09-30"), "USD", "CNY"); !errors.Is(err, ErrRateNotFound) {
		t.Fatalf("unknown pair: %v", err)
	}
	if _, err := svc.ResolveRate(ctx, day("2026-09-30"), "US", "TRY"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad code: %v", err)
	}
}

type fakeProvider struct {
	tcmb, ecb Day
	ecbErr    error
}

func (f fakeProvider) TCMB(context.Context) (Day, error) { return f.tcmb, nil }
func (f fakeProvider) ECB(context.Context) (Day, error)  { return f.ecb, f.ecbErr }

func TestFetchLatestStoresAndReports(t *testing.T) {
	ctx := context.Background()
	store := &memStore{}
	prov := fakeProvider{
		tcmb:   Day{Source: SourceTCMB, Date: day("2026-09-30"), Rates: []Rate{{Base: "USD", Quote: "TRY", Rate: "41.5"}}},
		ecbErr: errors.New("ecb down"),
	}
	svc := New(store, prov, nil)
	rep, err := svc.FetchLatest(ctx)
	if err == nil {
		t.Fatal("a failing source must surface an error (queue retry)")
	}
	if len(rep.Sources) != 2 || rep.Sources[0].Count != 1 || rep.Sources[1].Error == "" {
		t.Fatalf("report = %+v", rep)
	}
	// TCMB was stored although ECB failed; a second run is idempotent.
	_, _ = svc.FetchLatest(ctx)
	if len(store.rows) != 1 {
		t.Fatalf("rows = %d", len(store.rows))
	}
}
