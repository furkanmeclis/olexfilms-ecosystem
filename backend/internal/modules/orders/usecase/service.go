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
	// ErrTransitionUnavailable: delivered, received, cancelling (TEC-168).
	ErrTransitionUnavailable = errors.New("orders: transition not available yet")
	// ErrNotEditable: lines change only while the order is a draft.
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
}

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
	now    func() time.Time
}

// New creates the service.
func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, rates RateResolver) *Service {
	return &Service{pool: pool, q: q, out: out, rates: rates, ledger: ledger.New(q, out), now: time.Now}
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
}

// CreateInput is a new draft order of the active organization.
type CreateInput struct {
	Note  *string
	Items []ItemInput
}

type line struct {
	product db.Product
	qty     pgtype.Int4
	meters  pgtype.Numeric
	amount  *big.Rat
	note    pgtype.Text
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

// resolveLines validates the requested lines against the brand catalog.
func resolveLines(ctx context.Context, q *db.Queries, brandID int64, items []ItemInput) ([]line, error) {
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
		if seen[p.ID] {
			return nil, invalid(field+".product_uuid", "each product may appear once")
		}
		seen[p.ID] = true
		l := line{product: p, note: textOrNull(it.Note)}
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

type pricedLine struct {
	unitPrice pgtype.Numeric
	lineTotal pgtype.Numeric
	source    string
}

// priceLines fetches the buyer's effective purchase price of every product
// in the order currency. fields names the input field of each product for
// the error; a missing price is a 422.
func priceLines(ctx context.Context, q *db.Queries, buyer db.Organization, currency string,
	products []int64, amounts []*big.Rat, fields []string) ([]pricedLine, error) {
	prices, err := pricingusecase.New(q).BuyerPurchasePrices(ctx, buyer.ID, buyer.Type, buyer.BrandID, products, currency)
	if err != nil {
		return nil, fmt.Errorf("orders: prices: %w", err)
	}
	out := make([]pricedLine, len(products))
	for i, pid := range products {
		p, ok := prices[pid]
		if !ok {
			return nil, &ValidationError{Field: fields[i], Code: CodePriceNotFound,
				Message: "no purchase price in " + currency + " for this product"}
		}
		total, err := lineTotal(p.Price, amounts[i])
		if err != nil {
			return nil, err
		}
		up, err := numeric(p.Price)
		if err != nil {
			return nil, err
		}
		lt, err := numeric(total)
		if err != nil {
			return nil, err
		}
		out[i] = pricedLine{unitPrice: up, lineTotal: lt, source: priceSource(p.Source)}
	}
	return out, nil
}

func (s *Service) insertLines(ctx context.Context, q *db.Queries, o db.Order, buyer db.Organization, lines []line) error {
	ids := make([]int64, len(lines))
	amounts := make([]*big.Rat, len(lines))
	fields := make([]string, len(lines))
	for i, l := range lines {
		ids[i], amounts[i], fields[i] = l.product.ID, l.amount, fmt.Sprintf("items[%d].product_uuid", i)
	}
	priced, err := priceLines(ctx, q, buyer, o.Currency, ids, amounts, fields)
	if err != nil {
		return err
	}
	for i, l := range lines {
		if _, err := q.CreateOrderItem(ctx, db.CreateOrderItemParams{
			OrderID: o.ID, ProductID: l.product.ID, Quantity: l.qty, Meters: l.meters,
			UnitPrice: priced[i].unitPrice, PriceSource: priced[i].source, LineTotal: priced[i].lineTotal, Note: l.note,
		}); err != nil {
			return fmt.Errorf("orders: create line: %w", err)
		}
	}
	return nil
}

// --- Create / edit ---------------------------------------------------------------

// supplier returns the seller of the active organization: its parent, when
// the flow is center -> distributor or distributor -> dealer (K6).
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
		(buyer.Type == OrgDealer && seller.Type == OrgDistributor)
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
		lines, err := resolveLines(ctx, q, buyer.BrandID, in.Items)
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

// ReplaceItems swaps the lines of a draft order; prices are fetched again.
func (s *Service) ReplaceItems(ctx context.Context, c Caller, orderUUID uuid.UUID, items []ItemInput) (OrderView, error) {
	var updated db.Order
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		if partyOf(c, o) != PartyBuyer || !c.can(rbac.PermOrdersWrite) {
			return ErrForbidden
		}
		if o.Status != StatusDraft {
			return ErrNotEditable
		}
		buyer, err := q.GetOrganizationByID(ctx, o.BuyerOrgID)
		if err != nil {
			return fmt.Errorf("orders: buyer: %w", err)
		}
		lines, err := resolveLines(ctx, q, o.BrandID, items)
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
		if err := s.insertLines(ctx, q, o, buyer, lines); err != nil {
			return err
		}
		if o, err = q.RecalculateOrderTotals(ctx, o.ID); err != nil {
			return fmt.Errorf("orders: totals: %w", err)
		}
		updated = o
		return s.emit(ctx, tx, events.OrdersUpdated, o, o.Status, c)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, updated)
}

// --- Transitions -------------------------------------------------------------------

// TransitionInput moves an order to Status; Reason is kept in the history
// (and as the cancel reason).
type TransitionInput struct {
	Status string
	Reason *string
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
		if !r.allows(partyOf(c, o), c.can) {
			return ErrForbidden
		}
		from := o.Status
		var meta map[string]any
		switch to {
		case StatusApproved:
			o, meta, err = s.approve(ctx, q, c, o)
		case StatusReady:
			if err = checkFullyAssigned(ctx, q, o); err == nil {
				o, err = q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: to})
			}
		case StatusShipped:
			if meta, err = s.ship(ctx, q, tx, c, o); err == nil {
				o, err = q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: to})
			}
		case StatusCancelled:
			// Before shipping a cancel frees every active reservation
			// (TEC-96 decision 1).
			if _, err = q.ReleaseReservationsByOrder(ctx, o.ID); err == nil {
				o, err = q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: to})
			}
		default:
			o, err = q.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{ID: o.ID, Status: to})
		}
		if err != nil {
			return err
		}
		reason := textOrNull(in.Reason)
		if to == StatusCancelled && reason.Valid {
			if o, err = q.SetOrderCancelReason(ctx, db.SetOrderCancelReasonParams{ID: o.ID, CancelReason: reason}); err != nil {
				return fmt.Errorf("orders: cancel reason: %w", err)
			}
		}
		if err := s.history(ctx, q, o, from, to, c, in.Reason, meta); err != nil {
			return err
		}
		result = o
		return s.emit(ctx, tx, eventFor(to), o, from, c)
	})
	if err != nil {
		return OrderView{}, err
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
	ids := make([]int64, len(items))
	amounts := make([]*big.Rat, len(items))
	fields := make([]string, len(items))
	for i, it := range items {
		ids[i], fields[i] = it.ProductID, fmt.Sprintf("items[%d]", i)
		if it.Meters.Valid {
			amounts[i] = numericRat(it.Meters)
		} else {
			amounts[i] = new(big.Rat).SetInt64(int64(it.Quantity.Int32))
		}
	}
	priced, err := priceLines(ctx, q, buyer, o.Currency, ids, amounts, fields)
	if err != nil {
		return o, nil, err
	}
	for i, it := range items {
		if _, err := q.UpdateOrderItem(ctx, db.UpdateOrderItemParams{
			ID: it.ID, Quantity: it.Quantity, Meters: it.Meters, UnitPrice: priced[i].unitPrice,
			PriceSource: priced[i].source, RecommendedPriceSnapshot: it.RecommendedPriceSnapshot,
			LineTotal: priced[i].lineTotal, Note: it.Note,
		}); err != nil {
			return o, nil, fmt.Errorf("orders: lock line price: %w", err)
		}
	}
	if o, err = q.RecalculateOrderTotals(ctx, o.ID); err != nil {
		return o, nil, fmt.Errorf("orders: totals: %w", err)
	}
	snap, err := s.rates.ResolveRate(ctx, s.now(), o.Currency, TryCurrency)
	if errors.Is(err, fxrates.ErrRateNotFound) {
		return o, nil, ErrRateNotFound
	}
	if err != nil {
		return o, nil, fmt.Errorf("orders: rate: %w", err)
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
	ev := events.New(name).WithTenant(o.OrganizationID).WithEntity("order", &id, &uid).WithPayload(payload)
	if c.Principal.UserInternal != 0 {
		ev = ev.WithActor(c.Principal.UserInternal)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("orders: outbox: %w", err)
	}
	return nil
}
