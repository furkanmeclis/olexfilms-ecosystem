package posting

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Cari transfer on re-parenting (TEC-198, F1-07h, K25). When an
// organization moves under another parent, the open cari between it and the
// old parent is carried over to the new parent, on both books:
//
//   - parent side: the old parent's cari with the organization is closed
//     (role close_parent) and the new parent's cari with the organization
//     opens with the same amount as an opening balance (role open_parent;
//     converted to the new parent's currency at today's rate when it differs);
//   - child side: the organization's cari with the old parent is closed
//     (role close_child) and its cari with the new parent opens with the
//     same amount (role open_child).
//
// A closing row is a collection (receivable) or payment (payable) without a
// cash/bank account, category cari_transfer; an opening row is a charge
// (receivable) or an account-less collection (payable), category
// opening_balance. None of them is income or expense and none moves cash, so
// P&L and cash balances are untouched. A zero balance writes nothing.
//
// Every row is sourced (cari_transfer, change uuid): the change is one row
// of organization_parent_changes (organization + old/new parent), so the
// idempotency key is organization + old/new parent + change id. A repeated
// call with the same change writes nothing.
const (
	SourceCariTransfer   = "cari_transfer"
	CategoryCariTransfer = "cari_transfer"

	RoleTransferCloseParent = "close_parent"
	RoleTransferOpenParent  = "open_parent"
	RoleTransferCloseChild  = "close_child"
	RoleTransferOpenChild   = "open_child"
)

// CariTransfer is one re-parenting of OrganizationID from OldParentID to
// NewParentID; Change is the uuid of its organization_parent_changes row.
type CariTransfer struct {
	Change         uuid.UUID
	OrganizationID int64
	OldParentID    int64
	NewParentID    int64
	Description    string
	ActorUserID    *int64
}

// CariTransferResult lists the rows of a transfer. Written holds the rows
// this call inserted (empty on a replay or when every balance was zero).
type CariTransferResult struct {
	Written []db.FinanceEntry
}

// transferLeg is one book's half of a transfer: close owner's cari with
// from, open newOwner's cari with to.
type transferLeg struct {
	owner, from         int64
	newOwner, to        int64
	closeRole, openRole string
}

// TransferCariTx carries the open cari between the organization and its old
// parent over to the new parent in tx. See the notes above.
func (p *Poster) TransferCariTx(ctx context.Context, tx pgx.Tx, t CariTransfer) (CariTransferResult, error) {
	if tx == nil {
		return CariTransferResult{}, errors.New("posting: transaction required")
	}
	if t.Change == uuid.Nil || t.OrganizationID == 0 || t.OldParentID == 0 || t.NewParentID == 0 ||
		t.OldParentID == t.NewParentID {
		return CariTransferResult{}, fmt.Errorf("%w: cari transfer needs a change, an organization and two parents", ErrInvalid)
	}
	legs := []transferLeg{
		{owner: t.OldParentID, from: t.OrganizationID, newOwner: t.NewParentID, to: t.OrganizationID,
			closeRole: RoleTransferCloseParent, openRole: RoleTransferOpenParent},
		{owner: t.OrganizationID, from: t.OldParentID, newOwner: t.OrganizationID, to: t.NewParentID,
			closeRole: RoleTransferCloseChild, openRole: RoleTransferOpenChild},
	}
	out := CariTransferResult{Written: []db.FinanceEntry{}}
	for _, leg := range legs {
		rows, err := p.transferLeg(ctx, tx, t, leg)
		if err != nil {
			return CariTransferResult{}, err
		}
		out.Written = append(out.Written, rows...)
	}
	return out, nil
}

func (p *Poster) transferLeg(ctx context.Context, tx pgx.Tx, t CariTransfer, leg transferLeg) ([]db.FinanceEntry, error) {
	q := p.q.WithTx(tx)
	src := Source{Type: SourceCariTransfer, UUID: t.Change}
	// Replay: the closing row of this change is already written.
	if _, err := q.GetFinanceEntryBySource(ctx, db.GetFinanceEntryBySourceParams{
		OrganizationID: leg.owner, SourceType: src.Type, SourceUuid: src.UUID,
		Role: leg.closeRole, Revision: 1,
	}); err == nil {
		return nil, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("posting: transfer lookup: %w", err)
	}
	owner, err := org(ctx, q, leg.owner)
	if err != nil {
		return nil, err
	}
	cari, err := q.GetCariAccountByCounterpartyOrg(ctx, db.GetCariAccountByCounterpartyOrgParams{
		OrganizationID: owner.ID, CounterpartyOrgID: pgtype.Int8{Int64: leg.from, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // no cari, nothing open
	}
	if err != nil {
		return nil, fmt.Errorf("posting: transfer cari: %w", err)
	}
	bal, err := q.GetCariBalance(ctx, db.GetCariBalanceParams{CariID: cari.ID, OrganizationID: owner.ID})
	if err != nil {
		return nil, fmt.Errorf("posting: transfer balance: %w", err)
	}
	balance, ok := new(big.Rat).SetString(FormatNumeric(bal.Balance))
	if !ok {
		return nil, fmt.Errorf("posting: transfer balance %q", FormatNumeric(bal.Balance))
	}
	if balance.Sign() == 0 {
		return nil, nil
	}
	receivable := balance.Sign() > 0
	amount := new(big.Rat).Abs(balance).FloatString(2)
	closeDir, openDir := DirectionPayment, DirectionCollection
	if receivable {
		closeDir, openDir = DirectionCollection, DirectionCharge
	}
	closing, err := p.post(ctx, tx, closeDir, Entry{
		OrganizationID: owner.ID, Source: src, Role: leg.closeRole, Category: CategoryCariTransfer,
		Amount: amount, Currency: owner.Currency, CounterpartyOrgID: leg.from,
		Description: t.Description, ActorUserID: t.ActorUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("posting: transfer close: %w", err)
	}
	opening, err := p.post(ctx, tx, openDir, Entry{
		OrganizationID: leg.newOwner, Source: src, Role: leg.openRole, Category: CategoryOpeningBalance,
		Amount: amount, Currency: owner.Currency, CounterpartyOrgID: leg.to,
		Description: t.Description, ActorUserID: t.ActorUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("posting: transfer open: %w", err)
	}
	var out []db.FinanceEntry
	for _, r := range []Result{closing, opening} {
		if !r.Replayed {
			out = append(out, r.Entry)
		}
	}
	return out, nil
}
