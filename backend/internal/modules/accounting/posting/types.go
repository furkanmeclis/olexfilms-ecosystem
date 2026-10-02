// Package posting is the source-keyed write API of the accounting ledger
// (TEC-173, F1-07c). Other modules (orders TEC-169, transfers TEC-97) call it
// inside their own transaction; it has no HTTP surface.
//
// Every write is keyed by (organization, source_type, source_uuid, role,
// revision) — uq_finance_entries_source — and inserted with ON CONFLICT DO
// NOTHING, so a retried call writes nothing and returns the earlier row.
// Corrections are reversal rows (finance_entries is append-only);
// VoidBySourceTx reverses every open row of a source in every organization.
//
// Sign convention (000047): original rows are positive. An income or expense
// row that carries a cari_id is at the same time the P&L row and the cari
// movement (income: the counterparty owes us; expense: we owe the
// counterparty), so a sale on cari is ONE row per side — writing a separate
// charge next to it would count the receivable twice.
package posting

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/google/uuid"
)

// Directions written by this package (finance_entries.direction).
const (
	DirectionIncome  = "income"
	DirectionExpense = "expense"
	DirectionCharge  = "charge"
	// Settlements (TEC-172): cash/bank movement that closes a cari; never
	// income or expense (TEC-99 decision 2).
	DirectionCollection = "collection"
	DirectionPayment    = "payment"
)

// Categories and roles of the hierarchical sale bridge.
const (
	CategorySale     = "sale"
	CategoryPurchase = "purchase"
	RoleSale         = "sale"
	RolePurchase     = "purchase"
	DefaultRole      = "main"
)

var (
	// ErrInvalid: malformed request (amount, currency, source, targets).
	ErrInvalid = errors.New("posting: invalid request")
	// ErrOrganizationNotFound: an organization of the request does not exist.
	ErrOrganizationNotFound = errors.New("posting: organization not found")
	// ErrNotParent: the seller of a hierarchical sale is not the buyer's
	// parent in the same brand (K9; sibling transfers are TEC-170).
	ErrNotParent = errors.New("posting: seller is not the buyer's parent")
	// ErrCrossBrand: the counterparty belongs to another brand (K1).
	ErrCrossBrand = errors.New("posting: counterparty belongs to another brand")
	// ErrIdempotencyConflict: the source key already holds a different row.
	ErrIdempotencyConflict = errors.New("posting: source already posted with different values")
	// ErrNothingOpen: the source has no open (unreversed) row to revise.
	ErrNothingOpen = errors.New("posting: source has no open entry")
)

// Source identifies the business record behind a ledger row.
type Source struct {
	Type string // e.g. "order"
	UUID uuid.UUID
}

// Entry is one sourced ledger row.
type Entry struct {
	OrganizationID int64
	Source         Source
	// Role separates the rows one source writes in one organization;
	// empty means DefaultRole. Revision defaults to 1 (TEC-99d bumps it).
	Role     string
	Revision int32
	Category string
	// Amount is a positive decimal ("1250.00") in Currency, the original
	// currency. It is converted to the organization's currency (K7).
	Amount   string
	Currency string
	// RateDate is the frozen rate day; zero means today (UTC). Rate, when
	// its pair is Currency→organization currency, is used as is.
	RateDate time.Time
	Rate     *fxrates.Snapshot
	// AccountID is a cash/bank account of the organization (0: none).
	AccountID int64
	// CounterpartyOrgID opens (ensures) the organization's cari with that
	// organization and books the row on it (0: none).
	CounterpartyOrgID int64
	Description       string
	ActorUserID       *int64
	// PostedAt places the row in the ledger order (created_at); zero means
	// now. Only an opening balance (TEC-177) sets it, to its opening date.
	PostedAt time.Time
}

// Sale is a sale from an organization to its child (K9): the seller books
// income on the buyer's cari, the buyer books a purchase expense on the
// seller's cari, each in its own currency at the frozen rate day.
type Sale struct {
	Source      Source
	SellerOrgID int64
	BuyerOrgID  int64
	Amount      string // sale total in Currency (brands.currency, TEC-96)
	Currency    string
	// RateDate is the day the order froze its rate; when zero the date of
	// RateSnapshot is used. One of them is required.
	RateDate     time.Time
	RateSnapshot *fxrates.Snapshot
	Description  string
	ActorUserID  *int64
}

// RateResolver resolves a rate on a day (*fxrates.Service).
type RateResolver interface {
	ResolveRate(ctx context.Context, on time.Time, base, quote string) (fxrates.Snapshot, error)
}
