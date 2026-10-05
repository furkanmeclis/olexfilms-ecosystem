package posting

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Poster writes sourced ledger rows. It holds no state of its own.
type Poster struct {
	q     *db.Queries
	out   outbox.Enqueuer
	rates RateResolver
	now   func() time.Time
}

// New returns a Poster over q; events go to out (nil: none), rates resolve
// currency conversions (*fxrates.Service).
func New(q *db.Queries, out outbox.Enqueuer, rates RateResolver) *Poster {
	return &Poster{q: q, out: out, rates: rates, now: time.Now}
}

// Result is the outcome of one sourced write.
type Result struct {
	Entry db.FinanceEntry
	// Replayed is true when the source key already existed: nothing was
	// written and Entry is the earlier row.
	Replayed bool
}

// SaleResult is the outcome of PostHierarchicalSaleTx.
type SaleResult struct {
	Seller Result
	Buyer  Result
}

// Replayed reports whether both sides already existed.
func (r SaleResult) Replayed() bool { return r.Seller.Replayed && r.Buyer.Replayed }

// VoidResult lists the reversal rows written by VoidBySourceTx (empty when
// the source had nothing open).
type VoidResult struct {
	Reversals []db.FinanceEntry
}

// PostIncome books income (cash/bank account and/or the counterparty's cari).
func (p *Poster) PostIncome(ctx context.Context, tx pgx.Tx, e Entry) (Result, error) {
	return p.post(ctx, tx, DirectionIncome, e)
}

// PostExpense books an expense (cash/bank account and/or a cari debt).
func (p *Poster) PostExpense(ctx context.Context, tx pgx.Tx, e Entry) (Result, error) {
	return p.post(ctx, tx, DirectionExpense, e)
}

// Charge books a non-P&L debit on the counterparty's cari (no account).
func (p *Poster) Charge(ctx context.Context, tx pgx.Tx, e Entry) (Result, error) {
	if e.AccountID != 0 || !e.hasCari() {
		return Result{}, fmt.Errorf("%w: a charge books a cari only", ErrInvalid)
	}
	return p.post(ctx, tx, DirectionCharge, e)
}

// Collect books a collection: the counterparty paid the organization into a
// cash/bank account; the cari receivable goes down, no income is written.
func (p *Poster) Collect(ctx context.Context, tx pgx.Tx, e Entry) (Result, error) {
	if e.AccountID == 0 || !e.hasCari() {
		return Result{}, fmt.Errorf("%w: a collection needs an account and a cari", ErrInvalid)
	}
	return p.post(ctx, tx, DirectionCollection, e)
}

// Pay books a payment: the organization paid the counterparty from a
// cash/bank account; the cari debt goes down, no expense is written.
func (p *Poster) Pay(ctx context.Context, tx pgx.Tx, e Entry) (Result, error) {
	if e.AccountID == 0 || !e.hasCari() {
		return Result{}, fmt.Errorf("%w: a payment needs an account and a cari", ErrInvalid)
	}
	return p.post(ctx, tx, DirectionPayment, e)
}

// PostHierarchicalSaleTx books a parent→child sale on both ledgers (K9):
// seller income on the buyer's cari in the seller's currency, buyer
// "purchase" expense on the seller's cari in the buyer's currency, both
// converted at the frozen rate day. Rows keep the original currency, amount
// and rate. A repeated call writes nothing and returns the earlier rows.
func (p *Poster) PostHierarchicalSaleTx(ctx context.Context, tx pgx.Tx, s Sale) (SaleResult, error) {
	if tx == nil {
		return SaleResult{}, errors.New("posting: transaction required")
	}
	q := p.q.WithTx(tx)
	seller, err := org(ctx, q, s.SellerOrgID)
	if err != nil {
		return SaleResult{}, err
	}
	buyer, err := org(ctx, q, s.BuyerOrgID)
	if err != nil {
		return SaleResult{}, err
	}
	if !buyer.ParentID.Valid || buyer.ParentID.Int64 != seller.ID || buyer.BrandID != seller.BrandID {
		return SaleResult{}, fmt.Errorf("%w: seller %d, buyer %d", ErrNotParent, seller.ID, buyer.ID)
	}
	day := s.RateDate
	if day.IsZero() && s.RateSnapshot != nil {
		if day, err = time.Parse(time.DateOnly, s.RateSnapshot.RateDate); err != nil {
			return SaleResult{}, fmt.Errorf("%w: rate snapshot date %q", ErrInvalid, s.RateSnapshot.RateDate)
		}
	}
	if day.IsZero() {
		return SaleResult{}, fmt.Errorf("%w: a sale needs its frozen rate date", ErrInvalid)
	}
	base := Entry{
		Source: s.Source, Amount: s.Amount, Currency: s.Currency, RateDate: day,
		Rate: s.RateSnapshot, Description: s.Description, ActorUserID: s.ActorUserID,
	}

	sellerSide := base
	sellerSide.OrganizationID, sellerSide.CounterpartyOrgID = seller.ID, buyer.ID
	sellerSide.Role, sellerSide.Category = RoleSale, CategorySale
	var res SaleResult
	if res.Seller, err = p.post(ctx, tx, DirectionIncome, sellerSide); err != nil {
		return SaleResult{}, fmt.Errorf("posting: seller side: %w", err)
	}

	buyerSide := base
	buyerSide.OrganizationID, buyerSide.CounterpartyOrgID = buyer.ID, seller.ID
	buyerSide.Role, buyerSide.Category = RolePurchase, CategoryPurchase
	if res.Buyer, err = p.post(ctx, tx, DirectionExpense, buyerSide); err != nil {
		return SaleResult{}, fmt.Errorf("posting: buyer side: %w", err)
	}
	return res, nil
}

// VoidBySourceTx reverses every open row of the source in every
// organization (seller and buyer side) in tx. Rows already reversed are
// skipped, so a repeated void writes nothing.
func (p *Poster) VoidBySourceTx(ctx context.Context, tx pgx.Tx, src Source, reason string, actorUserID *int64) (VoidResult, error) {
	if tx == nil {
		return VoidResult{}, errors.New("posting: transaction required")
	}
	if err := src.validate(); err != nil {
		return VoidResult{}, err
	}
	q := p.q.WithTx(tx)
	open, err := q.ListOpenFinanceEntriesBySource(ctx, db.ListOpenFinanceEntriesBySourceParams{
		SourceType: src.Type, SourceUuid: src.UUID,
	})
	if err != nil {
		return VoidResult{}, fmt.Errorf("posting: open entries: %w", err)
	}
	out := VoidResult{Reversals: []db.FinanceEntry{}}
	for _, e := range open {
		rev, err := q.InsertFinanceReversal(ctx, db.InsertFinanceReversalParams{
			ReversalOfID: e.ID, OrganizationID: e.OrganizationID,
			Description: text(reason), ActorUserID: i8p(actorUserID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // reversed concurrently
		}
		if err != nil {
			return VoidResult{}, fmt.Errorf("posting: reverse entry %d: %w", e.ID, err)
		}
		if err := p.publish(ctx, tx, rev, actorUserID); err != nil {
			return VoidResult{}, err
		}
		out.Reversals = append(out.Reversals, rev)
	}
	return out, nil
}

// ReviseResult lists the rows written by ReviseBySourceTx: the reversals of
// the open rows and their reposts (revision + 1) with the corrected amount.
type ReviseResult struct {
	Reversals []db.FinanceEntry
	Reposts   []db.FinanceEntry
}

// ReviseBySourceTx corrects the amount of a source (TEC-174, K24): every
// open row of the source is reversed (VoidBySourceTx) and reposted in the
// same organization, direction, category, role and targets as revision + 1
// with amount, in the original currency, at the row's frozen rate. All open
// rows must share the original currency and amount (a hierarchical sale's
// two sides do); anything else is ErrInvalid. Nothing open is ErrNothingOpen.
func (p *Poster) ReviseBySourceTx(ctx context.Context, tx pgx.Tx, src Source, amount, reason string, actorUserID *int64) (ReviseResult, error) {
	if tx == nil {
		return ReviseResult{}, errors.New("posting: transaction required")
	}
	if err := src.validate(); err != nil {
		return ReviseResult{}, err
	}
	corrected, err := parseAmount(amount)
	if err != nil {
		return ReviseResult{}, err
	}
	q := p.q.WithTx(tx)
	open, err := q.ListOpenFinanceEntriesBySource(ctx, db.ListOpenFinanceEntriesBySourceParams{
		SourceType: src.Type, SourceUuid: src.UUID,
	})
	if err != nil {
		return ReviseResult{}, fmt.Errorf("posting: open entries: %w", err)
	}
	if len(open) == 0 {
		return ReviseResult{}, fmt.Errorf("%w: %s/%s", ErrNothingOpen, src.Type, src.UUID)
	}
	first := open[0]
	for _, e := range open[1:] {
		if e.OrigCurrency != first.OrigCurrency || FormatNumeric(e.OrigAmount) != FormatNumeric(first.OrigAmount) {
			return ReviseResult{}, fmt.Errorf("%w: rows of %s/%s differ in original amount", ErrInvalid, src.Type, src.UUID)
		}
	}
	// Targets of the reposts, read before the reversals are written.
	counterparties := make([]int64, len(open))
	for i, e := range open {
		if !e.CariID.Valid {
			continue
		}
		c, err := q.GetCariAccount(ctx, db.GetCariAccountParams{ID: e.CariID.Int64, OrganizationID: e.OrganizationID})
		if err != nil {
			return ReviseResult{}, fmt.Errorf("posting: cari of entry %d: %w", e.ID, err)
		}
		if !c.CounterpartyOrgID.Valid {
			return ReviseResult{}, fmt.Errorf("%w: entry %d is on a user cari", ErrInvalid, e.ID)
		}
		counterparties[i] = c.CounterpartyOrgID.Int64
	}

	voided, err := p.VoidBySourceTx(ctx, tx, src, reason, actorUserID)
	if err != nil {
		return ReviseResult{}, err
	}
	out := ReviseResult{Reversals: voided.Reversals, Reposts: make([]db.FinanceEntry, 0, len(open))}
	value := corrected.FloatString(2)
	for i, e := range open {
		day := dateOnly(e.RateDate.Time)
		res, err := p.post(ctx, tx, e.Direction, Entry{
			OrganizationID: e.OrganizationID, Source: src,
			Role: e.Role, Revision: e.Revision + 1, Category: e.Category,
			Amount: value, Currency: e.OrigCurrency, RateDate: day,
			Rate: &fxrates.Snapshot{
				Base: e.OrigCurrency, Quote: e.Currency, Rate: FormatRate(e.Rate),
				RateDate: day.Format(time.DateOnly), Source: "frozen",
			},
			AccountID: e.AccountID.Int64, CounterpartyOrgID: counterparties[i],
			Description: reason, ActorUserID: actorUserID,
		})
		if err != nil {
			return ReviseResult{}, fmt.Errorf("posting: repost entry %d: %w", e.ID, err)
		}
		out.Reposts = append(out.Reposts, res.Entry)
	}
	return out, nil
}

// hasCari reports whether the entry books a cari (by counterparty or id).
func (e Entry) hasCari() bool { return e.CounterpartyOrgID != 0 || e.CariID != 0 }

var codeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var categoryRe = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,63}$`)
var roleRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func (s Source) validate() error {
	if !codeRe.MatchString(s.Type) || s.UUID == uuid.Nil {
		return fmt.Errorf("%w: source %q/%s", ErrInvalid, s.Type, s.UUID)
	}
	return nil
}

func (p *Poster) post(ctx context.Context, tx pgx.Tx, direction string, e Entry) (Result, error) {
	if tx == nil {
		return Result{}, errors.New("posting: transaction required")
	}
	if err := e.Source.validate(); err != nil {
		return Result{}, err
	}
	if e.Role == "" {
		e.Role = DefaultRole
	}
	if e.Revision == 0 {
		e.Revision = 1
	}
	if !roleRe.MatchString(e.Role) || e.Revision < 1 || !categoryRe.MatchString(e.Category) {
		return Result{}, fmt.Errorf("%w: role %q, revision %d, category %q", ErrInvalid, e.Role, e.Revision, e.Category)
	}
	if e.CounterpartyOrgID != 0 && e.CariID != 0 {
		return Result{}, fmt.Errorf("%w: give a counterparty or a cari, not both", ErrInvalid)
	}
	amount, err := parseAmount(e.Amount)
	if err != nil {
		return Result{}, err
	}
	cur, err := fxrates.NormalizeCode(e.Currency)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	q := p.q.WithTx(tx)

	owner, err := org(ctx, q, e.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	key := db.GetFinanceEntryBySourceParams{
		OrganizationID: owner.ID, SourceType: e.Source.Type, SourceUuid: e.Source.UUID,
		Role: e.Role, Revision: e.Revision,
	}
	// A replay returns the earlier row before resolving any rate, so it
	// does not depend on rates still being available.
	if res, done, err := p.replay(ctx, q, key, direction, cur, amount.FloatString(2), e); done || err != nil {
		return res, err
	}

	var cari pgtype.Int8
	switch {
	case e.CounterpartyOrgID != 0:
		c, err := ensureCari(ctx, q, owner, e.CounterpartyOrgID)
		if err != nil {
			return Result{}, err
		}
		cari = pgtype.Int8{Int64: c.ID, Valid: true}
	case e.CariID != 0:
		c, err := q.GetCariAccount(ctx, db.GetCariAccountParams{ID: e.CariID, OrganizationID: owner.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, fmt.Errorf("%w: cari %d is not a cari of organization %d", ErrInvalid, e.CariID, owner.ID)
		}
		if err != nil {
			return Result{}, fmt.Errorf("posting: cari %d: %w", e.CariID, err)
		}
		cari = pgtype.Int8{Int64: c.ID, Valid: true}
	}

	conv, err := p.conversion(ctx, cur, owner.Currency, e)
	if err != nil {
		return Result{}, err
	}
	origAmount, err := numeric(amount.FloatString(2))
	if err != nil {
		return Result{}, err
	}
	converted, err := numeric(convert(amount, conv.rate))
	if err != nil {
		return Result{}, err
	}
	rate, err := numeric(conv.rate.FloatString(rateScale))
	if err != nil {
		return Result{}, err
	}
	arg := db.InsertFinanceEntryParams{
		OrganizationID: owner.ID, BrandID: owner.BrandID, CariID: cari,
		Direction: direction, Category: e.Category,
		OrigCurrency: cur, OrigAmount: origAmount,
		Currency: owner.Currency, Amount: converted,
		Rate: rate, RateDate: pgtype.Date{Time: conv.day, Valid: true},
		SourceType: text(e.Source.Type), SourceUuid: pgtype.UUID{Bytes: e.Source.UUID, Valid: true},
		Role: e.Role, Revision: e.Revision,
		Description: text(e.Description), ActorUserID: i8p(e.ActorUserID),
	}
	if e.AccountID != 0 {
		arg.AccountID = pgtype.Int8{Int64: e.AccountID, Valid: true}
	}
	if !e.PostedAt.IsZero() {
		arg.PostedAt = pgtype.Timestamptz{Time: e.PostedAt, Valid: true}
	}
	row, err := q.InsertFinanceEntry(ctx, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		// A concurrent writer of the same key won.
		res, _, err := p.replay(ctx, q, key, direction, cur, amount.FloatString(2), e)
		return res, err
	}
	if err != nil {
		return Result{}, fmt.Errorf("posting: insert entry: %w", err)
	}
	if err := p.publish(ctx, tx, row, e.ActorUserID); err != nil {
		return Result{}, err
	}
	return Result{Entry: row}, nil
}

// replay returns the row already stored under key; it must be the same
// write (direction, original amount and currency, category, account).
func (p *Poster) replay(ctx context.Context, q *db.Queries, key db.GetFinanceEntryBySourceParams,
	direction, cur, amount string, e Entry) (Result, bool, error) {
	prev, err := q.GetFinanceEntryBySource(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, true, fmt.Errorf("posting: source lookup: %w", err)
	}
	same := prev.Direction == direction && prev.OrigCurrency == cur &&
		FormatNumeric(prev.OrigAmount) == amount && prev.Category == e.Category &&
		prev.AccountID.Int64 == e.AccountID && prev.CariID.Valid == e.hasCari() &&
		(e.CariID == 0 || prev.CariID.Int64 == e.CariID)
	if !same {
		return Result{}, true, fmt.Errorf("%w: %s/%s role %s revision %d in organization %d",
			ErrIdempotencyConflict, key.SourceType, key.SourceUuid, key.Role, key.Revision, key.OrganizationID)
	}
	return Result{Entry: prev, Replayed: true}, true, nil
}

type conversion struct {
	rate *big.Rat
	day  time.Time
}

// conversion returns the rate from the original currency to the ledger
// currency: identity, the caller's frozen snapshot (or one of its frozen
// Pairs) when the pair matches, otherwise ResolveRate on the frozen day.
// Orders approved before TEC-226 have no Pairs and keep the day lookup.
func (p *Poster) conversion(ctx context.Context, from, to string, e Entry) (conversion, error) {
	day := e.RateDate
	if day.IsZero() {
		day = p.now()
	}
	day = dateOnly(day)
	if from == to {
		return conversion{rate: big.NewRat(1, 1), day: day}, nil
	}
	var snap *fxrates.Snapshot
	if frozen, ok := e.Rate.Find(from, to); ok {
		snap = &frozen
	} else {
		if p.rates == nil {
			return conversion{}, fmt.Errorf("posting: no rate resolver for %s/%s", from, to)
		}
		s, err := p.rates.ResolveRate(ctx, day, from, to)
		if err != nil {
			return conversion{}, fmt.Errorf("posting: rate %s/%s on %s: %w", from, to, day.Format(time.DateOnly), err)
		}
		snap = &s
	}
	rate, err := parseRate(snap.Rate)
	if err != nil {
		return conversion{}, err
	}
	rateDay, err := time.Parse(time.DateOnly, snap.RateDate)
	if err != nil {
		return conversion{}, fmt.Errorf("%w: rate date %q", ErrInvalid, snap.RateDate)
	}
	return conversion{rate: rate, day: rateDay}, nil
}

// ensureCari returns owner's cari with the counterparty organization,
// opening it in the owner's brand and currency when missing.
func ensureCari(ctx context.Context, q *db.Queries, owner db.Organization, counterparty int64) (db.CariAccount, error) {
	cp, err := org(ctx, q, counterparty)
	if err != nil {
		return db.CariAccount{}, err
	}
	if cp.ID == owner.ID {
		return db.CariAccount{}, fmt.Errorf("%w: an organization has no cari with itself", ErrInvalid)
	}
	if cp.BrandID != owner.BrandID {
		return db.CariAccount{}, fmt.Errorf("%w: organization %d, counterparty %d", ErrCrossBrand, owner.ID, cp.ID)
	}
	c, err := q.CreateCariForOrgIfMissing(ctx, db.CreateCariForOrgIfMissingParams{
		OrganizationID: owner.ID, BrandID: owner.BrandID, CounterpartyOrgID: pgtype.Int8{Int64: cp.ID, Valid: true}, Currency: owner.Currency,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		c, err = q.GetCariAccountByCounterpartyOrg(ctx, db.GetCariAccountByCounterpartyOrgParams{
			OrganizationID: owner.ID, CounterpartyOrgID: pgtype.Int8{Int64: cp.ID, Valid: true},
		})
	}
	if err != nil {
		return db.CariAccount{}, fmt.Errorf("posting: ensure cari: %w", err)
	}
	return c, nil
}

func org(ctx context.Context, q *db.Queries, id int64) (db.Organization, error) {
	o, err := q.GetOrganizationByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, fmt.Errorf("%w: %d", ErrOrganizationNotFound, id)
	}
	if err != nil {
		return db.Organization{}, fmt.Errorf("posting: organization %d: %w", id, err)
	}
	return o, nil
}

// publish writes the row's event in tx: cari.* for a cari row, finance.*
// for an account-only row.
func (p *Poster) publish(ctx context.Context, tx pgx.Tx, row db.FinanceEntry, actor *int64) error {
	if p.out == nil {
		return nil
	}
	reversal := row.ReversalOfID.Valid
	var name string
	switch {
	case row.CariID.Valid && reversal:
		name = events.CariEntryVoided
	case row.CariID.Valid && row.SourceType.String == SourceOpeningBalance:
		name = events.CariOpeningBalancePosted
	case row.CariID.Valid && (row.Direction == DirectionCollection || row.Direction == DirectionPayment):
		name = events.CariPaymentPosted
	case row.CariID.Valid:
		name = events.CariChargePosted
	case reversal:
		name = events.FinanceEntryVoided
	default:
		name = events.FinanceEntryPosted
	}
	id, uid := row.ID, row.Uuid
	payload := map[string]any{
		"entry_id":        row.ID,
		"entry_uuid":      row.Uuid.String(),
		"organization_id": row.OrganizationID,
		"brand_id":        row.BrandID,
		"direction":       row.Direction,
		"category":        row.Category,
		"orig_currency":   row.OrigCurrency,
		"orig_amount":     FormatNumeric(row.OrigAmount),
		"currency":        row.Currency,
		"amount":          FormatNumeric(row.Amount),
		"rate":            FormatRate(row.Rate),
		"rate_date":       row.RateDate.Time.Format(time.DateOnly),
		"source_type":     row.SourceType.String,
		"source_uuid":     uuid.UUID(row.SourceUuid.Bytes).String(),
		"role":            row.Role,
		"revision":        row.Revision,
	}
	if row.CariID.Valid {
		payload["cari_id"] = row.CariID.Int64
	}
	if row.AccountID.Valid {
		payload["account_id"] = row.AccountID.Int64
	}
	if reversal {
		payload["reversal_of_id"] = row.ReversalOfID.Int64
	}
	ev := events.New(name).WithTenant(row.OrganizationID).
		WithEntity("finance_entry", &id, &uid).WithPayload(payload)
	if actor != nil {
		ev = ev.WithActor(*actor)
	}
	if err := p.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("posting: outbox: %w", err)
	}
	return nil
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func i8p(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
