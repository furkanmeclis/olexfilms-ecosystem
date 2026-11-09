// Package usecase holds the order rules behind /v1/orders (TEC-166,
// F1-04b): draft orders up the tree (K6), server-side prices from the
// pricing module (K8), the rate frozen at seller approval (K7, TEC-96
// decision 2) and the stock-free part of the state machine. Every
// transition writes order_status_history and an orders.* outbox event in
// the same transaction.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service; the handler maps them to HTTP codes.
var (
	ErrNotFound  = errors.New("orders: order not found")
	ErrForbidden = errors.New("orders: not allowed for this organization")
	// ErrNoSupplier: the active organization has no seller above it (the
	// center) or its parent may not sell it products (K6).
	ErrNoSupplier = errors.New("orders: organization cannot place orders")
	// ErrInvalidTransition: the order's status does not allow the move.
	ErrInvalidTransition = errors.New("orders: invalid status transition")
	// ErrTransitionUnavailable: a status this API does not move to.
	ErrTransitionUnavailable = errors.New("orders: transition not available yet")
	// ErrNotEditable: the lines change only in a draft (buyer) or before
	// the order is ready (seller, TEC-1056).
	ErrNotEditable = errors.New("orders: order is not a draft")
	// ErrRateNotFound: no rate to freeze at approval.
	ErrRateNotFound = errors.New("orders: exchange rate not found")
)

// ValidationError is a field-level input error.
type ValidationError struct {
	Field   string
	Code    string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// CodePriceNotFound marks a line without an effective purchase price.
const CodePriceNotFound = "PRICE_NOT_FOUND"

// CodeProductNotOrderable marks a line whose product the center closed to
// dealer / distributor orders (TEC-1061, products.dealer_orderable).
const CodeProductNotOrderable = "PRODUCT_NOT_ORDERABLE"

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// Organization types (organizations.type).
const (
	OrgCenter      = "center"
	OrgDistributor = "distributor"
	OrgDealer      = "dealer"
)

// TryCurrency is the quote currency of the frozen rate (try_rate).
const TryCurrency = "TRY"

// MaxItems bounds the lines of one order.
const MaxItems = 200

// TxBeginner starts a transaction (*pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// RateResolver resolves a rate on a day (*fxrates.Service).
type RateResolver interface {
	ResolveRate(ctx context.Context, on time.Time, base, quote string) (fxrates.Snapshot, error)
}

// Caller is the request principal in its active organization; Filter is
// the resolved orders.read scope.
type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
	// PriceGrant is set by a background job (list export) whose request
	// snapshot already granted the price columns; request callers rely on
	// the principal (TEC-1050).
	PriceGrant bool
}

// SeesPrices reports whether the caller may see prices, totals and rates
// (TEC-1050): a principal holding a rbac.PriceViewPermissions grant, or a
// job that snapshotted one. Otherwise the views carry null money fields.
func (c Caller) SeesPrices() bool { return c.PriceGrant || c.Principal.SeesPrices() }

func (c Caller) actor() pgtype.Int8 {
	if c.Principal.UserInternal == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: c.Principal.UserInternal, Valid: true}
}

// can reports whether the caller holds slug for its own organization.
func (c Caller) can(slug string) bool { return c.Principal.Can(slug, rbac.ScopeManaged) }

// Service implements the order use cases.
type Service struct {
	pool  TxBeginner
	q     *db.Queries
	out   outbox.Enqueuer
	rates RateResolver
	// ledger writes the order_out movements at shipping (TEC-167).
	ledger *ledger.Ledger
	// receipts is called in the received transaction (accounting, TEC-169).
	receipts ReceiptHook
	now      func() time.Time
	// finder answers q from the orders index (TEC-210; nil: SQL only).
	finder searchengine.ListFinder
	// store and pdf keep the shipping documents (TEC-768; nil: unavailable).
	store storage.Driver
	pdf   PDFRenderer
	log   *slog.Logger
}

// New creates the service.
func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, rates RateResolver) *Service {
	return &Service{pool: pool, q: q, out: out, rates: rates, ledger: ledger.New(q, out),
		receipts: NoopReceiptHook{}, now: time.Now, log: slog.Default()}
}

// WithReceiptHook sets the hook called when an order is received.
func (s *Service) WithReceiptHook(h ReceiptHook) *Service {
	if h == nil {
		h = NoopReceiptHook{}
	}
	s.receipts = h
	return s
}

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("orders: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("orders: commit: %w", err)
	}
	return nil
}

// --- Input -------------------------------------------------------------------

// ItemInput is one requested line. Prices are never taken from the client:
// the unit price is the buyer's effective purchase price (K8).
type ItemInput struct {
	ProductUUID string
	Quantity    *int64
	Meters      *string
	Note        *string
	// UnitPrice is read only when the center orders for a dealer
	// (TEC-1024); on the buyer's own order it is ignored (K8).
	UnitPrice *string
	// DiscountPct and IsGift are read only when the center orders for a
	// dealer (TEC-1064): a typed discount replaces the dealer's default
	// one; a gift line is charged 0. The buyer's own order always gets
	// its default discount and no gift.
	DiscountPct *string
	IsGift      bool
	// PlotterCut marks the line "sent plotter cut" (TEC-1058, information
	// only); nil = off on a new line, unchanged on a kept one.
	PlotterCut *bool
}

// CreateInput is a new draft order of the active organization.
type CreateInput struct {
	Note  *string
	Items []ItemInput
	// BuyerOrgUUID: the center opens the order for this dealer directly
	// under it (TEC-1024); empty = the active organization is the buyer.
	BuyerOrgUUID *string
	// SellerWarehouseUUID: the center's warehouse the order falls into
	// (TEC-1028, with BuyerOrgUUID); empty = the primary warehouse.
	SellerWarehouseUUID *string
	// SelfPrepare: the center user prepares the order himself (TEC-1052,
	// with BuyerOrgUUID, needs orders.ship): no warehouse staff is told.
	SelfPrepare bool
}

// DraftResult reports whether a stock-forecast draft reused an existing open
// order or created a fresh one.
type DraftResult struct {
	Order   OrderView `json:"order"`
	Created bool      `json:"created"`
}

type line struct {
	product db.Product
	qty     pgtype.Int4
	meters  pgtype.Numeric
	amount  *big.Rat
	note    pgtype.Text
	// plotterCut: the line's plotter mark (TEC-1058).
	plotterCut bool
}

func textOrNull(p *string) pgtype.Text {
	if p == nil {
		return pgtype.Text{}
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

// lineRules tells resolveLines who places the order (TEC-1061).
type lineRules struct {
	// dealerOrder: the buyer orders for itself (dealer / distributor), so a
	// product closed to dealer orders is refused. The center's order for a
	// dealer (createForBuyer) leaves it false.
	dealerOrder bool
	// keep: product ids already on the order; a draft may keep a line whose
	// product was closed after it was added.
	keep map[int64]bool
}

// resolveLines validates the requested lines against the brand catalog.
func resolveLines(ctx context.Context, q *db.Queries, brandID int64, items []ItemInput, rules lineRules) ([]line, error) {
	if len(items) == 0 {
		return nil, invalid("items", "at least one line is required")
	}
	if len(items) > MaxItems {
		return nil, invalid("items", fmt.Sprintf("at most %d lines", MaxItems))
	}
	seen := map[int64]bool{}
	out := make([]line, 0, len(items))
	for i, it := range items {
		field := fmt.Sprintf("items[%d]", i)
		id, err := uuid.Parse(strings.TrimSpace(it.ProductUUID))
		if err != nil {
			return nil, invalid(field+".product_uuid", "must be a UUID")
		}
		p, err := q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: id, BrandID: brandID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !p.Active) {
			return nil, invalid(field+".product_uuid", "product not found")
		}
		if err != nil {
			return nil, fmt.Errorf("orders: product: %w", err)
		}
		if rules.dealerOrder && !p.DealerOrderable && !rules.keep[p.ID] {
			return nil, &ValidationError{
				Field: field + ".product_uuid", Code: CodeProductNotOrderable,
				Message: "product is not open to dealer orders",
			}
		}
		if seen[p.ID] {
			return nil, invalid(field+".product_uuid", "each product may appear once")
		}
		seen[p.ID] = true
		l := line{product: p, note: textOrNull(it.Note), plotterCut: it.PlotterCut != nil && *it.PlotterCut}
		if p.UnitType == "roll_meter" {
			if it.Quantity != nil || it.Meters == nil {
				return nil, invalid(field+".meters", "roll products are ordered in meters")
			}
			m, ok := normalizeMeters(*it.Meters)
			if !ok {
				return nil, invalid(field+".meters", "must be a positive decimal with at most 2 fractional digits")
			}
			if l.meters, err = numeric(m); err != nil {
				return nil, err
			}
			l.amount, _ = parseRat(m)
		} else {
			if it.Meters != nil || it.Quantity == nil {
				return nil, invalid(field+".quantity", "this product is ordered by quantity")
			}
			if *it.Quantity <= 0 || *it.Quantity > MaxQuantity {
				return nil, invalid(field+".quantity", fmt.Sprintf("must be between 1 and %d", MaxQuantity))
			}
			l.qty = pgtype.Int4{Int32: int32(*it.Quantity), Valid: true}
			l.amount = new(big.Rat).SetInt64(*it.Quantity)
		}
		out = append(out, l)
	}
	return out, nil
}

// priceSource maps a pricing source onto order_items.price_source.
func priceSource(src string) string {
	switch src {
	case pricingusecase.SourceOverride:
		return "override"
	case pricingusecase.SourceDistributor:
		return "distributor_dealer"
	default:
		return "list"
	}
}

// priceLines fetches the buyer's effective purchase price of every product
// in the order currency and applies each line's discount (TEC-1064; nil:
// none). fields names the input field of each product for the error; a
// missing price is a 422.
func priceLines(ctx context.Context, q *db.Queries, buyer db.Organization, currency string,
	products []int64, amounts, discounts []*big.Rat, fields []string) ([]lineCharge, error) {
	prices, err := pricingusecase.New(q).BuyerPurchasePrices(ctx, buyer.ID, buyer.Type, buyer.BrandID, products, currency)
	if err != nil {
		return nil, fmt.Errorf("orders: prices: %w", err)
	}
	out := make([]lineCharge, len(products))
	for i, pid := range products {
		p, ok := prices[pid]
		if !ok {
			return nil, &ValidationError{Field: fields[i], Code: CodePriceNotFound,
				Message: "no purchase price in " + currency + " for this product"}
		}
		list := p.Price
		if out[i], err = charge(chargeInput{list: &list, source: priceSource(p.Source), amount: amounts[i],
			discount: discounts[i]}); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// insertLines writes the buyer's own lines with its default discount to the
// seller (TEC-1064).
func (s *Service) insertLines(ctx context.Context, q *db.Queries, o db.Order, buyer db.Organization, lines []line) error {
	d, err := buyerDiscount(ctx, q, buyer, o.SellerOrgID, o.ID)
	if err != nil {
		return err
	}
	ids := make([]int64, len(lines))
	amounts := make([]*big.Rat, len(lines))
	discounts := make([]*big.Rat, len(lines))
	fields := make([]string, len(lines))
	for i, l := range lines {
		ids[i], amounts[i], discounts[i], fields[i] = l.product.ID, l.amount, d.Pct, fmt.Sprintf("items[%d].product_uuid", i)
	}
	priced, err := priceLines(ctx, q, buyer, o.Currency, ids, amounts, discounts, fields)
	if err != nil {
		return err
	}
	for i, l := range lines {
		if err := createLine(ctx, q, o, l, priced[i]); err != nil {
			return err
		}
	}
	return nil
}

func createLine(ctx context.Context, q *db.Queries, o db.Order, l line, c lineCharge) error {
	if _, err := q.CreateOrderItem(ctx, db.CreateOrderItemParams{
		OrderID: o.ID, ProductID: l.product.ID, Quantity: l.qty, Meters: l.meters,
		UnitPrice: c.unitPrice, PriceSource: c.source, LineTotal: c.lineTotal, Note: l.note,
		ListUnitPrice: c.listPrice, DiscountPct: c.discount, IsGift: pgtype.Bool{Bool: c.gift, Valid: true},
		PlotterCut: l.plotterCut,
	}); err != nil {
		return fmt.Errorf("orders: create line: %w", err)
	}
	return nil
}

// --- Create / edit ---------------------------------------------------------------

// supplier returns the seller of the active organization: its parent, when
// the flow is center -> distributor, distributor -> dealer or, for a dealer
// directly under the center (Türkiye, TEC-1003), center -> dealer (K6).
func (s *Service) supplier(ctx context.Context, q *db.Queries, buyer db.Organization) (db.Organization, error) {
	if buyer.Type != OrgDistributor && buyer.Type != OrgDealer {
		return db.Organization{}, ErrNoSupplier
	}
	seller, err := q.SupplierOf(ctx, buyer.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrNoSupplier
	}
	if err != nil {
		return db.Organization{}, fmt.Errorf("orders: supplier: %w", err)
	}
	flowOK := (buyer.Type == OrgDistributor && seller.Type == OrgCenter) ||
		(buyer.Type == OrgDealer && (seller.Type == OrgDistributor || seller.Type == OrgCenter))
	if seller.BrandID != buyer.BrandID || !flowOK {
		return db.Organization{}, ErrNoSupplier
	}
	return seller, nil
}

// Create opens a draft order of the active organization (the buyer) to its
// parent (the seller).
func (s *Service) Create(ctx context.Context, c Caller, in CreateInput) (OrderView, error) {
	if c.Org.InternalID == 0 || !c.can(rbac.PermOrdersWrite) {
		return OrderView{}, ErrForbidden
	}
	if in.BuyerOrgUUID != nil && strings.TrimSpace(*in.BuyerOrgUUID) != "" {
		return s.createForBuyer(ctx, c, in)
	}
	if in.SellerWarehouseUUID != nil && strings.TrimSpace(*in.SellerWarehouseUUID) != "" {
		return OrderView{}, invalid("seller_warehouse_uuid", "is only accepted with buyer_org_uuid")
	}
	if in.SelfPrepare {
		return OrderView{}, invalid("self_prepare", "is only accepted with buyer_org_uuid")
	}
	var created db.Order
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		buyer, err := q.GetOrganizationByID(ctx, c.Org.InternalID)
		if err != nil {
			return fmt.Errorf("orders: buyer: %w", err)
		}
		seller, err := s.supplier(ctx, q, buyer)
		if err != nil {
			return err
		}
		brand, err := q.GetBrandByID(ctx, buyer.BrandID)
		if err != nil {
			return fmt.Errorf("orders: brand: %w", err)
		}
		lines, err := resolveLines(ctx, q, buyer.BrandID, in.Items, lineRules{dealerOrder: true})
		if err != nil {
			return err
		}
		o, err := q.CreateOrder(ctx, db.CreateOrderParams{
			SellerOrgID: seller.ID, BrandID: buyer.BrandID, BuyerOrgID: buyer.ID,
			Currency: strings.TrimSpace(brand.Currency), Note: textOrNull(in.Note), CreatedByUserID: c.actor(),
		})
		if err != nil {
			return fmt.Errorf("orders: create: %w", err)
		}
		if err := s.insertLines(ctx, q, o, buyer, lines); err != nil {
			return err
		}
		if o, err = q.RecalculateOrderTotals(ctx, o.ID); err != nil {
			return fmt.Errorf("orders: totals: %w", err)
		}
		if err := s.history(ctx, q, o, "", StatusDraft, c, nil, nil); err != nil {
			return err
		}
		created = o
		return s.emit(ctx, tx, events.OrdersCreated, o, "", c)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, created)
}

// CreateOrAppendDraft creates a buyer draft to its tree supplier, or merges
// the requested lines into the oldest open draft for the same buyer/seller.
// It is used by stock forecast recommendations so the order rules (K6/K8)
// stay in the orders module.
func (s *Service) CreateOrAppendDraft(ctx context.Context, c Caller, in CreateInput) (DraftResult, error) {
	if c.Org.InternalID == 0 || !c.can(rbac.PermOrdersWrite) {
		return DraftResult{}, ErrForbidden
	}
	var order db.Order
	created := false
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		buyer, err := q.GetOrganizationByID(ctx, c.Org.InternalID)
		if err != nil {
			return fmt.Errorf("orders: buyer: %w", err)
		}
		seller, err := s.supplier(ctx, q, buyer)
		if err != nil {
			return err
		}
		brand, err := q.GetBrandByID(ctx, buyer.BrandID)
		if err != nil {
			return fmt.Errorf("orders: brand: %w", err)
		}
		lines, err := resolveLines(ctx, q, buyer.BrandID, in.Items, lineRules{dealerOrder: true})
		if err != nil {
			return err
		}
		o, err := q.FindOpenDraftOrderForBuyerSeller(ctx, db.FindOpenDraftOrderForBuyerSellerParams{
			BrandID: buyer.BrandID, BuyerOrgID: buyer.ID, SellerOrgID: seller.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			o, err = q.CreateOrder(ctx, db.CreateOrderParams{
				SellerOrgID: seller.ID, BrandID: buyer.BrandID, BuyerOrgID: buyer.ID,
				Currency: strings.TrimSpace(brand.Currency), Note: textOrNull(in.Note), CreatedByUserID: c.actor(),
			})
			if err != nil {
				return fmt.Errorf("orders: create: %w", err)
			}
			if err := s.insertLines(ctx, q, o, buyer, lines); err != nil {
				return err
			}
			if o, err = q.RecalculateOrderTotals(ctx, o.ID); err != nil {
				return fmt.Errorf("orders: totals: %w", err)
			}
			if err := s.history(ctx, q, o, "", StatusDraft, c, nil, nil); err != nil {
				return err
			}
			created = true
			order = o
			return s.emit(ctx, tx, events.OrdersCreated, o, "", c)
		}
		if err != nil {
			return fmt.Errorf("orders: find draft: %w", err)
		}
		before, err := snapshotLines(ctx, q, o)
		if err != nil {
			return err
		}
		merged, err := s.mergeDraftLines(ctx, q, o, lines)
		if err != nil {
			return err
		}
		old, err := q.LockOrderItems(ctx, o.ID)
		if err != nil {
			return fmt.Errorf("orders: lines: %w", err)
		}
		for _, it := range old {
			if _, err := q.DeleteOrderItem(ctx, it.ID); err != nil {
				return fmt.Errorf("orders: delete line: %w", err)
			}
		}
		if err := s.insertLines(ctx, q, o, buyer, merged); err != nil {
			return err
		}
		if o, err = q.RecalculateOrderTotals(ctx, o.ID); err != nil {
			return fmt.Errorf("orders: totals: %w", err)
		}
		after, err := snapshotLines(ctx, q, o)
		if err != nil {
			return err
		}
		if err := s.recordUpdated(ctx, q, c, o, len(merged), diffLines(before, after)); err != nil {
			return err
		}
		order = o
		return s.emit(ctx, tx, events.OrdersUpdated, o, o.Status, c)
	})
	if err != nil {
		return DraftResult{}, err
	}
	view, err := s.view(ctx, s.q, c, order)
	if err != nil {
		return DraftResult{}, err
	}
	return DraftResult{Order: view, Created: created}, nil
}

func (s *Service) mergeDraftLines(ctx context.Context, q *db.Queries, o db.Order, add []line) ([]line, error) {
	items, err := q.LockOrderItems(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("orders: lines: %w", err)
	}
	out := make([]line, 0, len(items)+len(add))
	byProduct := map[int64]int{}
	for _, it := range items {
		p, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: o.BrandID})
		if err != nil {
			return nil, fmt.Errorf("orders: product: %w", err)
		}
		l := line{product: p, qty: it.Quantity, meters: it.Meters, note: it.Note, plotterCut: it.PlotterCut}
		if p.UnitType == "roll_meter" {
			m, err := parseRat(numericText(it.Meters, 2))
			if err != nil {
				return nil, fmt.Errorf("orders: existing meters: %w", err)
			}
			l.amount = m
		} else {
			if !it.Quantity.Valid {
				return nil, fmt.Errorf("orders: invalid existing quantity")
			}
			l.amount = new(big.Rat).SetInt64(int64(it.Quantity.Int32))
		}
		byProduct[p.ID] = len(out)
		out = append(out, l)
	}
	for _, l := range add {
		i, ok := byProduct[l.product.ID]
		if !ok {
			byProduct[l.product.ID] = len(out)
			out = append(out, l)
			continue
		}
		out[i].plotterCut = out[i].plotterCut || l.plotterCut
		if l.product.UnitType == "roll_meter" {
			total := new(big.Rat).Add(out[i].amount, l.amount)
			m, err := numeric(total.FloatString(2))
			if err != nil {
				return nil, err
			}
			out[i].meters, out[i].amount = m, total
		} else {
			total := int64(out[i].qty.Int32) + int64(l.qty.Int32)
			if total > MaxQuantity {
				return nil, invalid("items", fmt.Sprintf("quantity must be at most %d per product", MaxQuantity))
			}
			out[i].qty = pgtype.Int4{Int32: int32(total), Valid: true}
			out[i].amount = new(big.Rat).SetInt64(total)
		}
	}
	return out, nil
}

// ReplaceItems edits the lines of an order. The buyer swaps the lines of
// its draft (orders.write; prices are fetched again). The seller edits an
// accepted order before it is ready (orders.write or orders.ship, TEC-1056):
// lines matched by product, see editSellerLines. A buyer's order past the
// draft and a ready or shipped order are not editable (409).
func (s *Service) ReplaceItems(ctx context.Context, c Caller, orderUUID uuid.UUID, items []ItemInput) (OrderView, error) {
	var updated db.Order
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		switch partyOf(c, o) {
		case PartyBuyer:
			if !c.can(rbac.PermOrdersWrite) {
				return ErrForbidden
			}
		case PartySeller:
			if !c.can(rbac.PermOrdersWrite) && !c.can(rbac.PermOrdersShip) {
				return ErrForbidden
			}
		default:
			return ErrForbidden
		}
		if !canEditItems(c, o) {
			return ErrNotEditable
		}
		before, err := snapshotLines(ctx, q, o)
		if err != nil {
			return err
		}
		updated = o
		if partyOf(c, o) == PartySeller {
			changed, err := s.editSellerLines(ctx, q, c, o, items)
			if err != nil || !changed {
				return err
			}
		} else if err := s.replaceDraftLines(ctx, q, o, items); err != nil {
			return err
		}
		if o, err = q.RecalculateOrderTotals(ctx, o.ID); err != nil {
			return fmt.Errorf("orders: totals: %w", err)
		}
		after, err := snapshotLines(ctx, q, o)
		if err != nil {
			return err
		}
		if err := s.recordUpdated(ctx, q, c, o, len(after), diffLines(before, after)); err != nil {
			return err
		}
		updated = o
		return s.emit(ctx, tx, events.OrdersUpdated, o, o.Status, c)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, updated)
}

// replaceDraftLines swaps every line of a draft; prices are fetched again.
func (s *Service) replaceDraftLines(ctx context.Context, q *db.Queries, o db.Order, items []ItemInput) error {
	buyer, err := q.GetOrganizationByID(ctx, o.BuyerOrgID)
	if err != nil {
		return fmt.Errorf("orders: buyer: %w", err)
	}
	old, err := q.LockOrderItems(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("orders: lines: %w", err)
	}
	// TEC-1061: the buyer orders for itself; a line already on the draft may
	// stay after its product was closed.
	keep := make(map[int64]bool, len(old))
	for _, it := range old {
		keep[it.ProductID] = true
	}
	lines, err := resolveLines(ctx, q, o.BrandID, items, lineRules{dealerOrder: true, keep: keep})
	if err != nil {
		return err
	}
	for _, it := range old {
		if _, err := q.DeleteOrderItem(ctx, it.ID); err != nil {
			return fmt.Errorf("orders: delete line: %w", err)
		}
	}
	return s.insertLines(ctx, q, o, buyer, lines)
}

// --- Transitions -------------------------------------------------------------------

// TransitionInput moves an order to Status; Reason is kept in the history
// (and as the cancel reason). DeliveryMethodUUID (one of the seller's active
// delivery methods) and TrackingNo are read when the order ships (TEC-768).
type TransitionInput struct {
	Status             string
	Reason             *string
	DeliveryMethodUUID *string
	TrackingNo         *string
}

func partyOf(c Caller, o db.Order) Party {
	switch c.Org.InternalID {
	case o.SellerOrgID:
		return PartySeller
	case o.BuyerOrgID:
		return PartyBuyer
	}
	return ""
}

func visible(c Caller, o db.Order) bool {
	return o.BrandID == c.Org.BrandID &&
		(c.Filter.AllowsOrg(o.SellerOrgID, o.BrandID) || c.Filter.AllowsOrg(o.BuyerOrgID, o.BrandID))
}

func (s *Service) lockVisible(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.Order, error) {
	o, err := q.LockOrderByUUID(ctx, db.LockOrderByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Order{}, ErrNotFound
	}
	if err != nil {
		return db.Order{}, fmt.Errorf("orders: lock: %w", err)
	}
	if !visible(c, o) {
		return db.Order{}, ErrNotFound
	}
	return o, nil
}

// Transition moves an order along the state machine. A repeated request
// for the current status is a no-op (a second ship writes no movement).
func (s *Service) Transition(ctx context.Context, c Caller, orderUUID uuid.UUID, in TransitionInput) (OrderView, error) {
	to := strings.TrimSpace(in.Status)
	if !IsStatus(to) {
		return OrderView{}, invalid("status", "unknown order status")
	}
	var result db.Order
	shipped := false
	var warehouse *db.Warehouse
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		result = o
		if o.Status == to {
			return nil
		}
		if !supportedTargets[to] {
			return ErrTransitionUnavailable
		}
		r, ok := lookupTransition(o.Status, to)
		if !ok {
			return ErrInvalidTransition
		}
		onBehalf := false
		if !r.allows(partyOf(c, o), c.can) {
			// TEC-1062: the center receives for a memberless pool recipient.
			if onBehalf, err = s.receivesOnBehalf(ctx, q, c, o, to); err != nil {
				return err
			}
			if !onBehalf {
				return ErrForbidden
			}
		}
		// TEC-1070: an order with an invoice mark (possible from shipped
		// on) is not cancelled; the seller deletes the record first.
		if (to == StatusCancelling || to == StatusCancelled) && o.InvoiceStatus != InvoiceStatusPending {
			return ErrOrderInvoiced
		}
		from := o.Status
		var meta map[string]any
		switch to {
		case StatusApproved:
			o, meta, err = s.approve(ctx, q, c, o)
			if err == nil {
				o, warehouse, err = assignWarehouse(ctx, q, o, nil)
			}
			if err == nil && warehouse != nil {
				meta["seller_warehouse"] = warehouse.Code
			}
		case StatusReady:
			if err = checkFullyAssigned(ctx, q, o); err == nil {
				o, err = q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: to})
			}
		case StatusShipped:
			var info shippingInput
			if info, err = in.shipping(ctx, q, o); err != nil {
				return err
			}
			if meta, err = s.ship(ctx, q, tx, c, o); err == nil {
				o, err = q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: to})
			}
			if err == nil {
				o, err = setShipping(ctx, q, o, info)
			}
			if err == nil && info.method != nil {
				meta["delivery_method"] = info.method.Code
			}
			shipped = true
		case StatusReceived:
			if meta, err = s.receive(ctx, q, tx, c, o); err == nil {
				o, err = q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: to})
			}
			if err == nil && onBehalf {
				meta[MetaOnBehalfOfBuyer] = true
			}
			if err == nil {
				// Accounting hook point (TEC-96 decision 3, TEC-169).
				err = s.receipts.OrderReceived(ctx, tx, q, o, actorOf(c))
			}
		case StatusCancelled:
			if from == StatusCancelling {
				// After shipping the goods come back: one restore per
				// shipped unit (TEC-96 decision 1).
				meta, err = s.restore(ctx, q, tx, c, o)
			} else {
				// Before shipping a cancel frees every active reservation.
				_, err = q.ReleaseReservationsByOrder(ctx, o.ID)
			}
			if err == nil {
				o, err = q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: to})
			}
		default:
			o, err = q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: to})
		}
		if err != nil {
			return err
		}
		reason := textOrNull(in.Reason)
		if (to == StatusCancelled || to == StatusCancelling) && reason.Valid {
			if o, err = q.SetOrderCancelReason(ctx, db.SetOrderCancelReasonParams{ID: o.ID, CancelReason: reason}); err != nil {
				return fmt.Errorf("orders: cancel reason: %w", err)
			}
		}
		if err := s.history(ctx, q, o, from, to, c, in.Reason, meta); err != nil {
			return err
		}
		result = o
		notice, err := s.statusNotice(ctx, q, o, to, c)
		if err != nil {
			return err
		}
		if err := s.emitWith(ctx, tx, eventFor(to), o, from, c, notice); err != nil {
			return err
		}
		if to == StatusApproved {
			return s.emitWarehouseNew(ctx, q, tx, o, warehouse, c)
		}
		return nil
	})
	if err != nil {
		return OrderView{}, err
	}
	if shipped {
		result = s.afterShip(ctx, result)
	}
	return s.view(ctx, s.q, c, result)
}

// approve re-reads every line's price, locks it into the line, recomputes
// the totals and freezes the rate to TRY (TEC-96 decisions 2 and 4).
func (s *Service) approve(ctx context.Context, q *db.Queries, c Caller, o db.Order) (db.Order, map[string]any, error) {
	items, err := q.LockOrderItems(ctx, o.ID)
	if err != nil {
		return o, nil, fmt.Errorf("orders: lines: %w", err)
	}
	buyer, err := q.GetOrganizationByID(ctx, o.BuyerOrgID)
	if err != nil {
		return o, nil, fmt.Errorf("orders: buyer: %w", err)
	}
	// A gift line stays 0; every other line keeps the discount it was
	// written with (TEC-1064) on the price read again.
	priceable := make([]db.OrderItem, 0, len(items))
	for _, it := range items {
		if !it.IsGift {
			priceable = append(priceable, it)
		}
	}
	ids := make([]int64, len(priceable))
	amounts := make([]*big.Rat, len(priceable))
	discounts := make([]*big.Rat, len(priceable))
	fields := make([]string, len(priceable))
	for i, it := range priceable {
		ids[i], discounts[i], fields[i] = it.ProductID, numericRat(it.DiscountPct), fmt.Sprintf("items[%d]", i)
		if it.Meters.Valid {
			amounts[i] = numericRat(it.Meters)
		} else {
			amounts[i] = new(big.Rat).SetInt64(int64(it.Quantity.Int32))
		}
	}
	priced, err := priceLines(ctx, q, buyer, o.Currency, ids, amounts, discounts, fields)
	if err != nil {
		return o, nil, err
	}
	for i, it := range priceable {
		if _, err := q.UpdateOrderItem(ctx, db.UpdateOrderItemParams{
			ID: it.ID, Quantity: it.Quantity, Meters: it.Meters, UnitPrice: priced[i].unitPrice,
			PriceSource: priced[i].source, RecommendedPriceSnapshot: it.RecommendedPriceSnapshot,
			LineTotal: priced[i].lineTotal, Note: it.Note,
			ListUnitPrice: priced[i].listPrice, DiscountPct: priced[i].discount,
		}); err != nil {
			return o, nil, fmt.Errorf("orders: lock line price: %w", err)
		}
	}
	if o, err = q.RecalculateOrderTotals(ctx, o.ID); err != nil {
		return o, nil, fmt.Errorf("orders: totals: %w", err)
	}
	return s.freezeApproval(ctx, q, c, o, buyer)
}

// freezeApproval freezes the rate to TRY (and to each party's ledger
// currency) and marks the order approved; the line prices are already
// locked. It is the approval step shared by the seller's approval and an
// order the center opens for a dealer (TEC-1024).
func (s *Service) freezeApproval(ctx context.Context, q *db.Queries, c Caller, o db.Order,
	buyer db.Organization) (db.Order, map[string]any, error) {
	// The rate day is the UTC day, like the ledger's fallback day
	// (accounting.go) and the stored rate days; the process zone must not
	// move it (TEC-775).
	rateDay := s.now().UTC()
	snap, err := s.rates.ResolveRate(ctx, rateDay, o.Currency, TryCurrency)
	if errors.Is(err, fxrates.ErrRateNotFound) {
		return o, nil, ErrRateNotFound
	}
	if err != nil {
		return o, nil, fmt.Errorf("orders: rate: %w", err)
	}
	// K7 (TEC-226): freeze the rate value, not only the day. Next to the
	// order currency -> TRY rate, the rates to each party's ledger currency
	// are resolved now and kept in the snapshot, so a later same-day manual
	// override cannot change what the books get on receipt.
	seller, err := q.GetOrganizationByID(ctx, o.SellerOrgID)
	if err != nil {
		return o, nil, fmt.Errorf("orders: seller: %w", err)
	}
	for _, cur := range []string{seller.Currency, buyer.Currency} {
		if _, ok := snap.Find(o.Currency, cur); ok || cur == "" || cur == o.Currency {
			continue
		}
		pair, err := s.rates.ResolveRate(ctx, rateDay, o.Currency, cur)
		if errors.Is(err, fxrates.ErrRateNotFound) {
			return o, nil, ErrRateNotFound
		}
		if err != nil {
			return o, nil, fmt.Errorf("orders: rate %s/%s: %w", o.Currency, cur, err)
		}
		pair.Pairs = nil
		snap.Pairs = append(snap.Pairs, pair)
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return o, nil, fmt.Errorf("orders: snapshot: %w", err)
	}
	tryRate, err := numeric(snap.Rate)
	if err != nil {
		return o, nil, err
	}
	o, err = q.ApproveOrder(ctx, db.ApproveOrderParams{
		ID: o.ID, ApprovedByUserID: c.actor(), RateSnapshot: raw, TryRate: tryRate,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return o, nil, ErrInvalidTransition
	}
	if err != nil {
		return o, nil, fmt.Errorf("orders: approve: %w", err)
	}
	meta := map[string]any{"rate_snapshot": snap, "total": numericText(o.Total, 2), "currency": o.Currency}
	return o, meta, nil
}

func (s *Service) history(ctx context.Context, q *db.Queries, o db.Order, from, to string, c Caller,
	reason *string, meta map[string]any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("orders: history metadata: %w", err)
	}
	fromText := pgtype.Text{}
	if from != "" {
		fromText = pgtype.Text{String: from, Valid: true}
	}
	if _, err := q.InsertOrderStatusHistory(ctx, db.InsertOrderStatusHistoryParams{
		OrderID: o.ID, FromStatus: fromText, ToStatus: to, ActorUserID: c.actor(),
		Reason: textOrNull(reason), Metadata: raw,
	}); err != nil {
		return fmt.Errorf("orders: history: %w", err)
	}
	return nil
}

func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, o db.Order, from string, c Caller) error {
	return s.emitWith(ctx, tx, name, o, from, c, nil)
}

// emitWith writes the order event with extra payload fields (e.g. the
// notification recipients of shipped, delivered and received).
func (s *Service) emitWith(ctx context.Context, tx pgx.Tx, name string, o db.Order, from string, c Caller,
	extra map[string]any) error {
	id, uid := o.ID, o.Uuid
	payload := map[string]any{
		"order_uuid":    o.Uuid.String(),
		"order_no":      o.OrderNo,
		"status":        o.Status,
		"seller_org_id": o.SellerOrgID,
		"buyer_org_id":  o.BuyerOrgID,
		"brand_id":      o.BrandID,
		"currency":      o.Currency,
		"total":         numericText(o.Total, 2),
	}
	if from != "" && from != o.Status {
		payload["from_status"] = from
	}
	for k, v := range extra {
		payload[k] = v
	}
	ev := events.New(name).WithTenant(o.OrganizationID).WithEntity("order", &id, &uid).WithPayload(payload)
	if c.Principal.UserInternal != 0 {
		ev = ev.WithActor(c.Principal.UserInternal)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("orders: outbox: %w", err)
	}
	return nil
}
