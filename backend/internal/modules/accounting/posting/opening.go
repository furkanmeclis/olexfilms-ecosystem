package posting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/jackc/pgx/v5"
)

// Opening balance (TEC-177, F1-07g). The balance a cari carries over from
// the previous system (F2 migrator) or from paper is booked once per cari
// as a non-P&L row:
//
//   - source_type opening_balance, source_uuid = the cari's uuid, so the
//     source key (organization, opening_balance, cari, main, revision) admits
//     one open opening balance per cari;
//   - category opening_balance; direction charge for a debit opening (the
//     counterparty owes the organization) and collection without a cash/bank
//     account for a credit opening (the organization owes the counterparty).
//     Neither is income or expense, and an account-less collection moves no
//     cash, so P&L and cash balances are untouched (000047 needs no new
//     direction);
//   - created_at = the opening date (00:00 UTC), so statements put the row
//     at the start of the cari's history; rate_date is the opening date too.
//
// A repeated call with the same values is a replay (nothing written); other
// values while an opening balance is open are ErrIdempotencyConflict. The
// correction is a reversal (VoidBySourceTx); afterwards a new opening
// balance is written as the next revision.
const (
	SourceOpeningBalance   = "opening_balance"
	CategoryOpeningBalance = "opening_balance"
	// SideDebit: the counterparty owes the organization (receivable).
	SideDebit = "debit"
	// SideCredit: the organization owes the counterparty (payable).
	SideCredit = "credit"
)

// ErrOpeningDate: the opening date is missing, before 2000-01-01 or in the
// future.
var ErrOpeningDate = fmt.Errorf("%w: opening date must be between 2000-01-01 and today", ErrInvalid)

// earliestOpening bounds the opening date from below (typo guard).
var earliestOpening = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// OpeningBalance is the opening balance of the organization's cari with an
// organization counterparty.
type OpeningBalance struct {
	OrganizationID    int64
	CounterpartyOrgID int64
	Side              string // SideDebit or SideCredit
	// Amount is positive, in Currency (converted to the organization's
	// currency at Date, or at Rate when its pair matches).
	Amount   string
	Currency string
	// Date is the opening date: required, not in the future.
	Date        time.Time
	Rate        *fxrates.Snapshot
	Description string
	ActorUserID *int64
}

// OpeningResult is the outcome of PostOpeningBalanceTx.
type OpeningResult struct {
	Result
	Cari db.CariAccount
}

// PostOpeningBalanceTx books the opening balance of a cari in tx (opening
// the cari when missing). See the package notes above for the rules.
func (p *Poster) PostOpeningBalanceTx(ctx context.Context, tx pgx.Tx, o OpeningBalance) (OpeningResult, error) {
	if tx == nil {
		return OpeningResult{}, errors.New("posting: transaction required")
	}
	var direction string
	switch o.Side {
	case SideDebit:
		direction = DirectionCharge
	case SideCredit:
		direction = DirectionCollection
	default:
		return OpeningResult{}, fmt.Errorf("%w: side must be debit or credit", ErrInvalid)
	}
	if o.CounterpartyOrgID == 0 {
		return OpeningResult{}, fmt.Errorf("%w: an opening balance needs a counterparty", ErrInvalid)
	}
	day := dateOnly(o.Date)
	if o.Date.IsZero() || day.Before(earliestOpening) || day.After(dateOnly(p.now())) {
		return OpeningResult{}, ErrOpeningDate
	}
	q := p.q.WithTx(tx)
	owner, err := org(ctx, q, o.OrganizationID)
	if err != nil {
		return OpeningResult{}, err
	}
	cari, err := ensureCari(ctx, q, owner, o.CounterpartyOrgID)
	if err != nil {
		return OpeningResult{}, err
	}
	src := Source{Type: SourceOpeningBalance, UUID: cari.Uuid}
	revision, err := openingRevision(ctx, q, owner.ID, src)
	if err != nil {
		return OpeningResult{}, err
	}
	res, err := p.post(ctx, tx, direction, Entry{
		OrganizationID: owner.ID, Source: src, Revision: revision,
		Category: CategoryOpeningBalance, Amount: o.Amount, Currency: o.Currency,
		RateDate: day, Rate: o.Rate, CounterpartyOrgID: o.CounterpartyOrgID,
		Description: o.Description, ActorUserID: o.ActorUserID, PostedAt: day,
	})
	if err != nil {
		return OpeningResult{}, err
	}
	return OpeningResult{Result: res, Cari: cari}, nil
}

// openingRevision is the revision the next opening balance of src is
// written under: the open one's (a replay or a conflict), else one past the
// last reversed revision, else 1.
func openingRevision(ctx context.Context, q *db.Queries, orgID int64, src Source) (int32, error) {
	open, err := q.ListOpenFinanceEntriesBySource(ctx, db.ListOpenFinanceEntriesBySourceParams{
		SourceType: src.Type, SourceUuid: src.UUID,
	})
	if err != nil {
		return 0, fmt.Errorf("posting: open opening balance: %w", err)
	}
	for _, e := range open {
		if e.OrganizationID == orgID {
			return e.Revision, nil
		}
	}
	last, err := q.GetMaxFinanceEntryRevisionBySource(ctx, db.GetMaxFinanceEntryRevisionBySourceParams{
		OrganizationID: orgID, SourceType: src.Type, SourceUuid: src.UUID,
	})
	if err != nil {
		return 0, fmt.Errorf("posting: opening balance revision: %w", err)
	}
	return last + 1, nil
}
