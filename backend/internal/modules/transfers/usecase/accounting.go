package usecase

// Transfer -> accounting bridge (TEC-200, F1-04f3, K13).
//
// When a request is received the giver (A) books income on the receiver's
// cari and the receiver (B) a purchase expense on the giver's cari
// (posting.PostSiblingTransferTx) for the sum of the line totals frozen at
// approval (stock_transfer_request_items.line_total, A's purchase price),
// inside the received transaction next to the transfer_in movements. The
// rows are keyed by the request (source stock_transfer + request uuid, role
// transfer_out / transfer_in), so a retry writes nothing. Lines without a
// frozen price add nothing; a zero total books nothing. Each side is
// converted to its organization's currency at the receipt day's rate; a
// missing rate is ErrRateNotFound and rolls the receipt back.
//
// A cancel after shipping calls VoidBySourceTx (nothing is booked before
// receipt, so it writes nothing today); VoidAccountingTx is the reversal
// for a later return flow (received is final in the state machine).

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/jackc/pgx/v5"
)

// ErrRateNotFound: no exchange rate between the transfer currency and an
// organization's currency on the receipt day; nothing is written.
var ErrRateNotFound = errors.New("transfers: exchange rate not found")

// AccountingPoster is the part of *posting.Poster the bridge uses.
type AccountingPoster interface {
	PostSiblingTransferTx(ctx context.Context, tx pgx.Tx, t posting.SiblingTransfer) (posting.SaleResult, error)
	VoidBySourceTx(ctx context.Context, tx pgx.Tx, src posting.Source, reason string, actorUserID *int64) (posting.VoidResult, error)
}

// WithAccounting sets the poster that books received transfers (nil: none).
func (s *Service) WithAccounting(p AccountingPoster) *Service {
	s.accounting = p
	return s
}

// AccountingSource is the ledger source of a transfer request.
func AccountingSource(r db.StockTransferRequest) posting.Source {
	return posting.Source{Type: posting.SourceStockTransfer, UUID: r.Uuid}
}

// frozenTotal sums the line totals frozen at approval; ok is false when
// nothing is priced.
func frozenTotal(rows []db.ListTransferRequestItemsRow) (string, bool) {
	total := new(big.Rat)
	for _, it := range rows {
		if lt := numericRat(it.LineTotal); lt != nil {
			total.Add(total, lt)
		}
	}
	if total.Sign() <= 0 {
		return "", false
	}
	return total.FloatString(2), true
}

// book writes the K13 rows of a received request in tx.
func (s *Service) book(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, r db.StockTransferRequest) error {
	if s.accounting == nil {
		return nil
	}
	rows, err := s.items(ctx, q, r)
	if err != nil {
		return err
	}
	amount, ok := frozenTotal(rows)
	if !ok {
		return nil
	}
	_, err = s.accounting.PostSiblingTransferTx(ctx, tx, posting.SiblingTransfer{
		Source: AccountingSource(r), SenderOrgID: r.FromOrgID, ReceiverOrgID: r.ToOrgID,
		Amount: amount, Currency: r.Currency, RateDate: s.now().UTC(),
		Description: "Transfer " + r.TransferNo, ActorUserID: c.actorPtr(),
	})
	if errors.Is(err, fxrates.ErrRateNotFound) {
		return fmt.Errorf("%w: %v", ErrRateNotFound, err)
	}
	if err != nil {
		return fmt.Errorf("transfers: accounting of %s: %w", r.TransferNo, err)
	}
	return nil
}

// VoidAccountingTx reverses every open accounting row of the request (both
// sides) in tx; a repeated call writes nothing.
func (s *Service) VoidAccountingTx(ctx context.Context, tx pgx.Tx, r db.StockTransferRequest, reason string, actor *int64) (posting.VoidResult, error) {
	if s.accounting == nil {
		return posting.VoidResult{}, nil
	}
	res, err := s.accounting.VoidBySourceTx(ctx, tx, AccountingSource(r), reason, actor)
	if err != nil {
		return posting.VoidResult{}, fmt.Errorf("transfers: accounting reversal of %s: %w", r.TransferNo, err)
	}
	return res, nil
}
