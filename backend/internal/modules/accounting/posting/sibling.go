package posting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Sibling stock transfer (TEC-200, K13). A received transfer between two
// sibling organizations (same brand, same parent) is booked like a sale at
// the giver's frozen purchase price: the giver (A) books income on the
// receiver's cari — A alacak — and the receiver (B) books a purchase expense
// on the giver's cari — B borç. One row per side (see the sign convention in
// types.go), categories sale / purchase, roles transfer_out / transfer_in,
// source stock_transfer + the request uuid. Each row is converted to its
// organization's currency at the rate of RateDate (the receipt day). A
// repeated call writes nothing; VoidBySourceTx reverses both sides.
const (
	SourceStockTransfer = "stock_transfer"
	RoleTransferOut     = "transfer_out"
	RoleTransferIn      = "transfer_in"
)

// ErrNotSibling: the two organizations of a sibling transfer do not share
// the same parent and brand (K13).
var ErrNotSibling = errors.New("posting: organizations are not siblings")

// SiblingTransfer is one received transfer from SenderOrgID to
// ReceiverOrgID; Amount is the sum of the frozen line totals in Currency.
type SiblingTransfer struct {
	Source        Source
	SenderOrgID   int64
	ReceiverOrgID int64
	Amount        string
	Currency      string
	// RateDate is the rate day; zero means today (UTC).
	RateDate    time.Time
	Description string
	ActorUserID *int64
}

// PostSiblingTransferTx books a received sibling transfer on both ledgers
// in tx (see the notes above).
func (p *Poster) PostSiblingTransferTx(ctx context.Context, tx pgx.Tx, t SiblingTransfer) (SaleResult, error) {
	if tx == nil {
		return SaleResult{}, errors.New("posting: transaction required")
	}
	q := p.q.WithTx(tx)
	sender, err := org(ctx, q, t.SenderOrgID)
	if err != nil {
		return SaleResult{}, err
	}
	receiver, err := org(ctx, q, t.ReceiverOrgID)
	if err != nil {
		return SaleResult{}, err
	}
	if sender.ID == receiver.ID || !sender.ParentID.Valid || !receiver.ParentID.Valid ||
		sender.ParentID.Int64 != receiver.ParentID.Int64 || sender.BrandID != receiver.BrandID {
		return SaleResult{}, fmt.Errorf("%w: sender %d, receiver %d", ErrNotSibling, sender.ID, receiver.ID)
	}
	day := t.RateDate
	if day.IsZero() {
		day = p.now()
	}
	base := Entry{
		Source: t.Source, Amount: t.Amount, Currency: t.Currency, RateDate: dateOnly(day),
		Description: t.Description, ActorUserID: t.ActorUserID,
	}

	out := base
	out.OrganizationID, out.CounterpartyOrgID = sender.ID, receiver.ID
	out.Role, out.Category = RoleTransferOut, CategorySale
	var res SaleResult
	if res.Seller, err = p.post(ctx, tx, DirectionIncome, out); err != nil {
		return SaleResult{}, fmt.Errorf("posting: sender side: %w", err)
	}

	in := base
	in.OrganizationID, in.CounterpartyOrgID = receiver.ID, sender.ID
	in.Role, in.Category = RoleTransferIn, CategoryPurchase
	if res.Buyer, err = p.post(ctx, tx, DirectionExpense, in); err != nil {
		return SaleResult{}, fmt.Errorf("posting: receiver side: %w", err)
	}
	return res, nil
}
