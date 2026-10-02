package posting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

// Cash/bank opening balance (TEC-198, F1-07h). The balance a cash or bank
// account carries over from before the system is booked once per account
// without a cari:
//
//   - direction opening (000056): account only, positive, never income or
//     expense, so it raises the account balance and stays out of P&L;
//   - source_type account_opening, source_uuid = the account's uuid, so one
//     opening balance per account is open; category opening_balance;
//   - kept in the account's currency (= the organization's currency);
//     created_at and rate_date = the opening date (00:00 UTC).
//
// A repeated call with the same values is a replay; other values while one
// is open are ErrIdempotencyConflict. The correction is a reversal
// (VoidBySourceTx); afterwards a new one is written as the next revision.
const (
	DirectionOpening     = "opening"
	SourceAccountOpening = "account_opening"
)

// AccountOpening is the opening balance of a cash/bank account.
type AccountOpening struct {
	OrganizationID int64
	AccountID      int64
	// Amount is positive, in the account's currency.
	Amount string
	// Date is the opening date: required, not in the future.
	Date        time.Time
	Description string
	ActorUserID *int64
}

// AccountOpeningResult is the outcome of PostAccountOpeningTx.
type AccountOpeningResult struct {
	Result
	Account db.FinanceAccount
}

// PostAccountOpeningTx books the opening balance of a cash/bank account in
// tx. See the notes above.
func (p *Poster) PostAccountOpeningTx(ctx context.Context, tx pgx.Tx, o AccountOpening) (AccountOpeningResult, error) {
	if tx == nil {
		return AccountOpeningResult{}, errors.New("posting: transaction required")
	}
	if o.AccountID == 0 {
		return AccountOpeningResult{}, fmt.Errorf("%w: an account opening balance needs an account", ErrInvalid)
	}
	day := dateOnly(o.Date)
	if o.Date.IsZero() || day.Before(earliestOpening) || day.After(dateOnly(p.now())) {
		return AccountOpeningResult{}, ErrOpeningDate
	}
	q := p.q.WithTx(tx)
	acc, err := q.GetFinanceAccount(ctx, db.GetFinanceAccountParams{ID: o.AccountID, OrganizationID: o.OrganizationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountOpeningResult{}, fmt.Errorf("%w: account %d", ErrInvalid, o.AccountID)
	}
	if err != nil {
		return AccountOpeningResult{}, fmt.Errorf("posting: account: %w", err)
	}
	src := Source{Type: SourceAccountOpening, UUID: acc.Uuid}
	revision, err := openingRevision(ctx, q, acc.OrganizationID, src)
	if err != nil {
		return AccountOpeningResult{}, err
	}
	res, err := p.post(ctx, tx, DirectionOpening, Entry{
		OrganizationID: acc.OrganizationID, Source: src, Revision: revision,
		Category: CategoryOpeningBalance, Amount: o.Amount, Currency: acc.Currency,
		RateDate: day, AccountID: acc.ID, Description: o.Description,
		ActorUserID: o.ActorUserID, PostedAt: day,
	})
	if err != nil {
		return AccountOpeningResult{}, err
	}
	return AccountOpeningResult{Result: res, Account: acc}, nil
}
