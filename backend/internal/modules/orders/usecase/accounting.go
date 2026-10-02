package usecase

// Order -> accounting bridge (TEC-169, F1-04e).
//
// When an order is received, the seller books one income row on the
// buyer's cari and the buyer one purchase expense row on the seller's cari
// (posting.PostHierarchicalSaleTx, K9), inside the received transaction.
// The rows are keyed by the order (source "order"/orders.uuid), so a retry
// or a repeated receipt writes nothing. The rate is the one frozen at
// approval (orders.rate_snapshot, order currency -> TRY); when its pair does
// not match an organization's currency the poster resolves the rate of the
// same day (fxrates.ResolveRate).
//
// The reversal (VoidOrder) is ready for the return flow: a received order
// cannot be cancelled (TEC-168), so nothing calls it yet.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/jackc/pgx/v5"
)

// AccountingSourceType is the finance_entries.source_type of order rows.
const AccountingSourceType = LedgerSource

// SalePoster is the part of *posting.Poster the bridge uses.
type SalePoster interface {
	PostHierarchicalSaleTx(ctx context.Context, tx pgx.Tx, s posting.Sale) (posting.SaleResult, error)
	VoidBySourceTx(ctx context.Context, tx pgx.Tx, src posting.Source, reason string, actorUserID *int64) (posting.VoidResult, error)
}

var _ ReceiptHook = (*AccountingBridge)(nil)

// AccountingBridge is the ReceiptHook that books a received order.
type AccountingBridge struct {
	poster SalePoster
}

// NewAccountingBridge returns the bridge over p (*posting.Poster).
func NewAccountingBridge(p SalePoster) *AccountingBridge {
	return &AccountingBridge{poster: p}
}

// AccountingSource is the ledger source of an order.
func AccountingSource(o db.Order) posting.Source {
	return posting.Source{Type: AccountingSourceType, UUID: o.Uuid}
}

// SaleFor builds the hierarchical sale of an order: its total in the order
// currency at the rate frozen at approval. ok is false for a zero total
// (nothing to book).
func SaleFor(o db.Order, actor *int64) (posting.Sale, bool, error) {
	total := numericRat(o.Total)
	if total == nil || total.Sign() <= 0 {
		return posting.Sale{}, false, nil
	}
	s := posting.Sale{
		Source:      AccountingSource(o),
		SellerOrgID: o.SellerOrgID,
		BuyerOrgID:  o.BuyerOrgID,
		Amount:      total.FloatString(2),
		Currency:    o.Currency,
		Description: "Order " + o.OrderNo,
		ActorUserID: actor,
	}
	if len(o.RateSnapshot) > 0 && string(o.RateSnapshot) != "null" {
		var snap fxrates.Snapshot
		if err := json.Unmarshal(o.RateSnapshot, &snap); err != nil {
			return posting.Sale{}, false, fmt.Errorf("orders: rate snapshot of %s: %w", o.OrderNo, err)
		}
		if snap.RateDate != "" {
			s.RateSnapshot = &snap
		}
	}
	if s.RateSnapshot == nil {
		// No frozen snapshot: the approval day, else the receipt day.
		switch {
		case o.ApprovedAt.Valid:
			s.RateDate = o.ApprovedAt.Time.UTC()
		case o.ReceivedAt.Valid:
			s.RateDate = o.ReceivedAt.Time.UTC()
		default:
			s.RateDate = time.Now().UTC()
		}
	}
	return s, true, nil
}

// OrderReceived books the sale on both ledgers in tx; an error rolls the
// receipt back.
func (b *AccountingBridge) OrderReceived(ctx context.Context, tx pgx.Tx, _ *db.Queries, o db.Order, actor *int64) error {
	s, ok, err := SaleFor(o, actor)
	if err != nil || !ok {
		return err
	}
	if _, err := b.poster.PostHierarchicalSaleTx(ctx, tx, s); err != nil {
		return fmt.Errorf("orders: accounting of %s: %w", o.OrderNo, err)
	}
	return nil
}

// VoidOrder reverses every open accounting row of the order (seller and
// buyer side) in tx with reversal rows; a repeated call writes nothing.
func (b *AccountingBridge) VoidOrder(ctx context.Context, tx pgx.Tx, o db.Order, reason string, actor *int64) (posting.VoidResult, error) {
	r, err := b.poster.VoidBySourceTx(ctx, tx, AccountingSource(o), reason, actor)
	if err != nil {
		return posting.VoidResult{}, fmt.Errorf("orders: accounting reversal of %s: %w", o.OrderNo, err)
	}
	return r, nil
}
