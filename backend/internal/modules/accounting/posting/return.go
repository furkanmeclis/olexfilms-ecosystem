package posting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Stock return to the parent (TEC-223). A received return from a child to
// its direct parent is booked as the reverse of the hierarchical sale (K9,
// PostHierarchicalSaleTx), on the pattern of the sibling transfer (TEC-200):
// the child books income on the parent's cari — its debt to the parent goes
// down — and the parent books a purchase expense on the child's cari — the
// child's debt to it goes down. One row per side (sign convention in
// types.go), the system categories sale (child) / purchase (parent) so the
// F1 category catalog needs no new entry, roles return_out (child) /
// return_in (parent), source stock_return + the request uuid. Each row is
// converted to its organization's currency at the rate of RateDate (the
// receipt day). A repeated call writes nothing;
// VoidBySourceTx reverses both sides.
const (
	SourceStockReturn = "stock_return"
	RoleReturnOut     = "return_out"
	RoleReturnIn      = "return_in"
)

// StockReturn is one received return from ChildOrgID to ParentOrgID;
// Amount is the sum of the frozen line totals in Currency.
type StockReturn struct {
	Source      Source
	ChildOrgID  int64
	ParentOrgID int64
	Amount      string
	Currency    string
	// RateDate is the rate day; zero means today (UTC).
	RateDate    time.Time
	Description string
	ActorUserID *int64
}

// PostStockReturnTx books a received return on both ledgers in tx (see the
// notes above). Seller is the parent side, Buyer the child side.
func (p *Poster) PostStockReturnTx(ctx context.Context, tx pgx.Tx, r StockReturn) (SaleResult, error) {
	if tx == nil {
		return SaleResult{}, errors.New("posting: transaction required")
	}
	q := p.q.WithTx(tx)
	parent, err := org(ctx, q, r.ParentOrgID)
	if err != nil {
		return SaleResult{}, err
	}
	child, err := org(ctx, q, r.ChildOrgID)
	if err != nil {
		return SaleResult{}, err
	}
	if !child.ParentID.Valid || child.ParentID.Int64 != parent.ID || child.BrandID != parent.BrandID {
		return SaleResult{}, fmt.Errorf("%w: parent %d, child %d", ErrNotParent, parent.ID, child.ID)
	}
	day := r.RateDate
	if day.IsZero() {
		day = p.now()
	}
	base := Entry{
		Source: r.Source, Amount: r.Amount, Currency: r.Currency, RateDate: dateOnly(day),
		Description: r.Description, ActorUserID: r.ActorUserID,
	}

	in := base
	in.OrganizationID, in.CounterpartyOrgID = parent.ID, child.ID
	in.Role, in.Category = RoleReturnIn, CategoryPurchase
	var res SaleResult
	if res.Seller, err = p.post(ctx, tx, DirectionExpense, in); err != nil {
		return SaleResult{}, fmt.Errorf("posting: parent side: %w", err)
	}

	out := base
	out.OrganizationID, out.CounterpartyOrgID = child.ID, parent.ID
	out.Role, out.Category = RoleReturnOut, CategorySale
	if res.Buyer, err = p.post(ctx, tx, DirectionIncome, out); err != nil {
		return SaleResult{}, fmt.Errorf("posting: child side: %w", err)
	}
	return res, nil
}
