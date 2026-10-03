package fxrates

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxLookback is how far back a missing day falls (weekends, holidays,
// provider outages). Older rates are not used.
const MaxLookback = 10 * 24 * time.Hour

// Pivots used for cross rates when a pair has no direct quote, in order:
// TCMB quotes everything against TRY, ECB against EUR.
var Pivots = []string{"TRY", "EUR"}

var (
	// ErrRateNotFound: no rate for the pair within MaxLookback.
	ErrRateNotFound = errors.New("fxrates: rate not found")
	// ErrInvalid: malformed currency, date or rate.
	ErrInvalid = errors.New("fxrates: invalid request")
)

// Store is the persistence the service needs (*db.Queries).
type Store interface {
	UpsertExchangeRate(ctx context.Context, arg db.UpsertExchangeRateParams) error
	DeleteManualExchangeRate(ctx context.Context, arg db.DeleteManualExchangeRateParams) (int64, error)
	FindPairRate(ctx context.Context, arg db.FindPairRateParams) (db.FindPairRateRow, error)
	ListExchangeRatesByDate(ctx context.Context, arg db.ListExchangeRatesByDateParams) ([]db.ListExchangeRatesByDateRow, error)
	LatestExchangeRateDate(ctx context.Context, onDate pgtype.Date) (pgtype.Date, error)
	ListCurrencies(ctx context.Context, activeOnly bool) ([]db.Currency, error)
}

// Provider fetches one publication per source.
type Provider interface {
	TCMB(ctx context.Context) (Day, error)
	ECB(ctx context.Context) (Day, error)
}

// Service stores and resolves exchange rates.
type Service struct {
	store    Store
	provider Provider
	log      *slog.Logger
}

// New creates the service. provider may be nil (no fetching).
func New(store Store, provider Provider, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, provider: provider, log: log}
}

// Snapshot is the rate an order or journal entry freezes (rate_snapshot).
// 1 Base = Rate Quote, as published on RateDate by Source.
type Snapshot struct {
	Base     string `json:"base"`
	Quote    string `json:"quote"`
	Rate     string `json:"rate"`
	RateDate string `json:"rate_date"`
	Source   string `json:"source"`
	// Via is the pivot currency of a cross rate (TRY or EUR).
	Via string `json:"via,omitempty"`
	// Pairs holds extra rates frozen at the same moment, e.g. from the order
	// currency to each party's ledger currency (K7: the value is frozen, not
	// only the day). Nested pairs never carry Pairs themselves.
	Pairs []Snapshot `json:"pairs,omitempty"`
}

// Find returns the frozen rate for base/quote: the snapshot itself or one of
// its Pairs. ok is false when neither matches.
func (s *Snapshot) Find(base, quote string) (Snapshot, bool) {
	if s == nil {
		return Snapshot{}, false
	}
	if s.Base == base && s.Quote == quote {
		out := *s
		out.Pairs = nil
		return out, true
	}
	for _, p := range s.Pairs {
		if p.Base == base && p.Quote == quote {
			return p, true
		}
	}
	return Snapshot{}, false
}

// NormalizeCode upper-cases and validates an ISO 4217 code.
func NormalizeCode(raw string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(raw))
	if !validCode(c) {
		return "", fmt.Errorf("%w: currency must be an ISO 4217 code", ErrInvalid)
	}
	return c, nil
}

func pgDate(t time.Time) pgtype.Date {
	y, m, d := t.Date()
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

func dateString(d pgtype.Date) string { return d.Time.Format("2006-01-02") }

type pairRate struct {
	rate   *big.Rat
	date   pgtype.Date
	source string
}

func (s *Service) pair(ctx context.Context, on time.Time, base, quote string) (*pairRate, error) {
	row, err := s.store.FindPairRate(ctx, db.FindPairRateParams{
		OnDate: pgDate(on), MinDate: pgDate(on.Add(-MaxLookback)), Base: base, Quote: quote,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v, ok := new(big.Rat).SetString(row.Rate)
	if !ok || v.Sign() <= 0 {
		return nil, fmt.Errorf("fxrates: stored rate %q is invalid", row.Rate)
	}
	if strings.TrimSpace(row.Base) != base {
		v = new(big.Rat).Inv(v)
	}
	return &pairRate{rate: v, date: row.RateDate, source: row.Source}, nil
}

// ResolveRate returns the rate for base→quote on a day: the latest stored
// day not after `on` (within MaxLookback), manual > tcmb > ecb on that day,
// either direction; without a direct quote a cross rate through TRY or EUR.
func (s *Service) ResolveRate(ctx context.Context, on time.Time, base, quote string) (Snapshot, error) {
	base, err := NormalizeCode(base)
	if err != nil {
		return Snapshot{}, err
	}
	quote, err = NormalizeCode(quote)
	if err != nil {
		return Snapshot{}, err
	}
	if base == quote {
		return Snapshot{Base: base, Quote: quote, Rate: "1", RateDate: pgDate(on).Time.Format("2006-01-02"), Source: "identity"}, nil
	}
	direct, err := s.pair(ctx, on, base, quote)
	if err != nil {
		return Snapshot{}, err
	}
	if direct != nil {
		return Snapshot{
			Base: base, Quote: quote, Rate: FormatRat(direct.rate),
			RateDate: dateString(direct.date), Source: direct.source,
		}, nil
	}
	for _, pivot := range Pivots {
		if pivot == base || pivot == quote {
			continue
		}
		a, err := s.pair(ctx, on, base, pivot)
		if err != nil {
			return Snapshot{}, err
		}
		if a == nil {
			continue
		}
		b, err := s.pair(ctx, on, pivot, quote)
		if err != nil {
			return Snapshot{}, err
		}
		if b == nil {
			continue
		}
		date := a.date
		if b.date.Time.Before(date.Time) {
			date = b.date
		}
		source := a.source
		if b.source != a.source {
			source = a.source + "+" + b.source
		}
		return Snapshot{
			Base: base, Quote: quote, Rate: FormatRat(new(big.Rat).Mul(a.rate, b.rate)),
			RateDate: dateString(date), Source: source, Via: pivot,
		}, nil
	}
	return Snapshot{}, fmt.Errorf("%w: %s/%s on %s", ErrRateNotFound, base, quote, on.Format("2006-01-02"))
}

// SourceReport is the outcome of one provider fetch.
type SourceReport struct {
	Source string `json:"source"`
	Date   string `json:"date,omitempty"`
	Count  int    `json:"count"`
	Error  string `json:"error,omitempty"`
}

// FetchReport is the outcome of FetchLatest.
type FetchReport struct {
	Sources []SourceReport `json:"sources"`
}

// Store writes one publication (idempotent upsert per day/pair/source).
func (s *Service) Store(ctx context.Context, day Day) error {
	for _, r := range day.Rates {
		if err := s.store.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgDate(day.Date), Base: r.Base, Quote: r.Quote, Rate: r.Rate, Source: day.Source,
		}); err != nil {
			return fmt.Errorf("fxrates: store %s %s/%s: %w", day.Source, r.Base, r.Quote, err)
		}
	}
	return nil
}

// FetchLatest fetches TCMB and ECB and stores what they publish. One failing
// source does not block the other; the error (for the queue retry) joins the
// failures.
func (s *Service) FetchLatest(ctx context.Context) (FetchReport, error) {
	if s.provider == nil {
		return FetchReport{}, errors.New("fxrates: no provider configured")
	}
	var rep FetchReport
	var errs []error
	for _, src := range []struct {
		name  string
		fetch func(context.Context) (Day, error)
	}{
		{SourceTCMB, s.provider.TCMB},
		{SourceECB, s.provider.ECB},
	} {
		sr := SourceReport{Source: src.name}
		day, err := src.fetch(ctx)
		if err == nil {
			err = s.Store(ctx, day)
		}
		if err != nil {
			sr.Error = err.Error()
			errs = append(errs, err)
			s.log.Warn("fxrates_fetch_failed", "source", src.name, "error", err)
		} else {
			sr.Date, sr.Count = day.Date.Format("2006-01-02"), len(day.Rates)
			s.log.Info("fxrates_fetched", "source", src.name, "date", sr.Date, "count", sr.Count)
		}
		rep.Sources = append(rep.Sources, sr)
	}
	return rep, errors.Join(errs...)
}

// FetchTask is the queue entry point (asynq retries on error).
func (s *Service) FetchTask(ctx context.Context) error {
	_, err := s.FetchLatest(ctx)
	return err
}

// OverrideInput is a manual rate for a day.
type OverrideInput struct {
	Date    time.Time
	Base    string
	Quote   string
	Rate    string
	Note    string
	ActorID int64
}

// SetOverride stores (or replaces) the manual rate of a day and pair. It wins
// over TCMB/ECB for that day; clearing it brings the fetched value back.
func (s *Service) SetOverride(ctx context.Context, in OverrideInput) (Snapshot, error) {
	base, err := NormalizeCode(in.Base)
	if err != nil {
		return Snapshot{}, err
	}
	quote, err := NormalizeCode(in.Quote)
	if err != nil {
		return Snapshot{}, err
	}
	if base == quote {
		return Snapshot{}, fmt.Errorf("%w: base and quote must differ", ErrInvalid)
	}
	if in.Date.IsZero() {
		return Snapshot{}, fmt.Errorf("%w: date is required", ErrInvalid)
	}
	rate, err := ParseRate(in.Rate)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	note := strings.TrimSpace(in.Note)
	if err := s.store.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
		RateDate: pgDate(in.Date), Base: base, Quote: quote, Rate: rate, Source: SourceManual,
		Note:            pgtype.Text{String: note, Valid: note != ""},
		CreatedByUserID: pgtype.Int8{Int64: in.ActorID, Valid: in.ActorID != 0},
	}); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Base: base, Quote: quote, Rate: rate, RateDate: pgDate(in.Date).Time.Format("2006-01-02"), Source: SourceManual}, nil
}

// ClearOverride removes the manual rate of a day and pair.
func (s *Service) ClearOverride(ctx context.Context, on time.Time, base, quote string) error {
	base, err := NormalizeCode(base)
	if err != nil {
		return err
	}
	quote, err = NormalizeCode(quote)
	if err != nil {
		return err
	}
	n, err := s.store.DeleteManualExchangeRate(ctx, db.DeleteManualExchangeRateParams{RateDate: pgDate(on), Base: base, Quote: quote})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: no manual rate for %s/%s on %s", ErrRateNotFound, base, quote, on.Format("2006-01-02"))
	}
	return nil
}

// StoredRate is one stored row of a day.
type StoredRate struct {
	RateDate  string    `json:"rate_date"`
	Base      string    `json:"base"`
	Quote     string    `json:"quote"`
	Rate      string    `json:"rate"`
	Source    string    `json:"source"`
	Note      *string   `json:"note,omitempty"`
	Effective bool      `json:"effective"`
	FetchedAt time.Time `json:"fetched_at"`
}

// DayRates lists the stored rows of a day (zero date = latest stored day
// up to today). Effective marks the row the resolver picks for its pair.
func (s *Service) DayRates(ctx context.Context, on time.Time, base string) (string, []StoredRate, error) {
	if on.IsZero() {
		d, err := s.store.LatestExchangeRateDate(ctx, pgDate(time.Now()))
		if err != nil {
			return "", nil, err
		}
		on = d.Time
	}
	var baseArg pgtype.Text
	if strings.TrimSpace(base) != "" {
		b, err := NormalizeCode(base)
		if err != nil {
			return "", nil, err
		}
		baseArg = pgtype.Text{String: b, Valid: true}
	}
	rows, err := s.store.ListExchangeRatesByDate(ctx, db.ListExchangeRatesByDateParams{RateDate: pgDate(on), Base: baseArg})
	if err != nil {
		return "", nil, err
	}
	out := make([]StoredRate, 0, len(rows))
	seen := map[string]bool{}
	for _, r := range rows {
		key := strings.TrimSpace(r.Base) + "/" + strings.TrimSpace(r.Quote)
		var note *string
		if r.Note.Valid {
			n := r.Note.String
			note = &n
		}
		out = append(out, StoredRate{
			RateDate: dateString(r.RateDate), Base: strings.TrimSpace(r.Base), Quote: strings.TrimSpace(r.Quote),
			Rate: r.Rate, Source: r.Source, Note: note, Effective: !seen[key], FetchedAt: r.FetchedAt.Time,
		})
		seen[key] = true
	}
	return pgDate(on).Time.Format("2006-01-02"), out, nil
}

// Currency is one ISO 4217 currency.
type Currency struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int16  `json:"decimals"`
}

// Currencies lists the active currencies.
func (s *Service) Currencies(ctx context.Context) ([]Currency, error) {
	rows, err := s.store.ListCurrencies(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make([]Currency, 0, len(rows))
	for _, r := range rows {
		out = append(out, Currency{Code: strings.TrimSpace(r.Code), Name: r.Name, Symbol: r.Symbol, Decimals: r.Decimals})
	}
	return out, nil
}
