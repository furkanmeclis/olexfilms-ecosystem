package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Warranty claim accounting (TEC-337, F3-06d). When the re-application
// service of a claim is completed, the claim is booked once through the
// accounting source API (accounting/posting), keyed by source_type
// warranty_claim + the claim uuid:
//
//   - product cost: the consumed units at the center's purchase price
//     (product_prices.purchase_price in the brand currency, F1 price chain)
//     are a center expense, category warranty_cost, role warranty_cost
//     (targetless sourced row, 000098);
//   - product refund: the dealer used units of its own stock, so every hop of
//     the chain (center → distributor → dealer) credits the child what it paid
//     for them, with the existing stock return reversal of the hierarchical
//     sale (posting.PostStockReturnTx, K24 pattern; roles return_in /
//     return_out, no new bridge type). The child's price is its latest
//     received order price of the unit, else its F1 purchase price;
//   - labor (system setting warranty_claims.labor_rule): dealer (default)
//     books nothing; center credits warranty_claims.labor_amount (center
//     currency) down the chain, shared warranty_claims.labor_share_percent
//     percent of it. Each hop: parent expense warranty_labor (role
//     labor_out) on the child's cari, child income warranty_labor_income
//     (role labor_in) on the parent's cari.
//
// Every row is converted at the rate of the posting day and keeps it (K7).
// A claim that already has warranty_claim rows is skipped, and the posting
// API itself is idempotent per (organization, source, role, revision).
//
// Cancelling the completed re-application service (TEC-382, F3-06h)
// reverses every open warranty_claim row of the claim with the accounting
// reversal pattern (posting.VoidBySourceTx: same amounts, opposite sign,
// reversal_of_id to the original; nothing is deleted or updated) and adds a
// note to the claim timeline. Rows already reversed are skipped, so a
// repeated cancellation writes nothing. Stock is not touched here.
const (
	AccountingSourceType = "warranty_claim"
	RoleWarrantyCost     = "warranty_cost"
	RoleLaborOut         = "labor_out"
	RoleLaborIn          = "labor_in"
)

// Poster is the subset of *posting.Poster the claim accounting uses.
type Poster interface {
	PostExpense(ctx context.Context, tx pgx.Tx, e posting.Entry) (posting.Result, error)
	PostIncome(ctx context.Context, tx pgx.Tx, e posting.Entry) (posting.Result, error)
	PostStockReturnTx(ctx context.Context, tx pgx.Tx, r posting.StockReturn) (posting.SaleResult, error)
	VoidBySourceTx(ctx context.Context, tx pgx.Tx, src posting.Source, reason string, actorUserID *int64) (posting.VoidResult, error)
}

// Settings reads the labor rule (*sysconfig.Service).
type Settings interface {
	String(ctx context.Context, key string) string
	Int(ctx context.Context, key string) int64
}

// Accounting books closed claims (see the notes above).
type Accounting struct {
	pool     txBeginner
	poster   Poster
	settings Settings
	log      *slog.Logger
}

// NewAccounting returns the claim accounting over pool.
func NewAccounting(pool txBeginner, poster Poster, settings Settings, log *slog.Logger) *Accounting {
	if log == nil {
		log = slog.Default()
	}
	return &Accounting{pool: pool, poster: poster, settings: settings, log: log}
}

// RegisterAccountingHandlers subscribes the claim accounting to
// service.completed (TEC-337) and service.cancelled (TEC-382).
func RegisterAccountingHandlers(bus events.Bus, pool txBeginner, poster Poster, settings Settings, log *slog.Logger) {
	if bus == nil || pool == nil || poster == nil {
		return
	}
	a := NewAccounting(pool, poster, settings, log)
	bus.Subscribe(events.ServiceCompleted, a.HandleServiceCompleted)
	bus.Subscribe(events.ServiceCancelled, a.HandleServiceCancelled)
}

// HandleServiceCompleted is the bus handler of service.completed.
func (a *Accounting) HandleServiceCompleted(ctx context.Context, ev events.Event) error {
	serviceID, ok := payloadInt64(ev.Payload["service_id"])
	if !ok {
		return nil
	}
	brandID, ok := payloadInt64(ev.Payload["brand_id"])
	if !ok {
		return nil
	}
	_, err := a.PostReapplyService(ctx, serviceID, brandID, ev.ActorUserID)
	if err != nil {
		a.log.Error("warranty_claim_accounting_failed", "service_id", serviceID, "error", err)
	}
	return err
}

// HandleServiceCancelled is the bus handler of service.cancelled (TEC-382).
func (a *Accounting) HandleServiceCancelled(ctx context.Context, ev events.Event) error {
	serviceID, ok := payloadInt64(ev.Payload["service_id"])
	if !ok && ev.EntityID != nil {
		serviceID, ok = *ev.EntityID, *ev.EntityID > 0
	}
	if !ok {
		return nil
	}
	brandID, ok := payloadInt64(ev.Payload["brand_id"])
	if !ok {
		return nil
	}
	reason, _ := ev.Payload["reason"].(string)
	_, err := a.ReverseReapplyService(ctx, serviceID, brandID, reason, ev.ActorUserID)
	if err != nil {
		a.log.Error("warranty_claim_accounting_reversal_failed", "service_id", serviceID, "error", err)
	}
	return err
}

// laborRule reads the settings; anything unreadable is the conservative
// default (dealer: no labor).
func (a *Accounting) laborRule(ctx context.Context) (string, *big.Rat) {
	if a.settings == nil {
		return sysconfig.LaborRuleDealer, nil
	}
	rule := strings.TrimSpace(a.settings.String(ctx, sysconfig.KeyWarrantyClaimsLaborRule))
	if rule != sysconfig.LaborRuleCenter && rule != sysconfig.LaborRuleShared {
		return sysconfig.LaborRuleDealer, nil
	}
	amount, ok := new(big.Rat).SetString(strings.TrimSpace(a.settings.String(ctx, sysconfig.KeyWarrantyClaimsLaborAmount)))
	if !ok || amount.Sign() <= 0 {
		return rule, nil
	}
	if rule == sysconfig.LaborRuleShared {
		pct := a.settings.Int(ctx, sysconfig.KeyWarrantyClaimsLaborSharePercent)
		if pct <= 0 {
			return rule, nil
		}
		if pct > 100 {
			pct = 100
		}
		amount.Mul(amount, big.NewRat(pct, 100))
	}
	return rule, amount
}

// PostedClaim summarizes what PostReapplyService wrote.
type PostedClaim struct {
	ClaimID  int64
	Skipped  bool // nothing to do: not a claim service, or already booked
	Rule     string
	Currency string // center currency
	Rows     int
}

// PostReapplyService books the claim of a completed re-application service
// in one transaction. A service without a claim, a claim whose current
// re-application is another service, or a claim already booked writes
// nothing.
func (a *Accounting) PostReapplyService(ctx context.Context, serviceID, brandID int64, actor *int64) (PostedClaim, error) {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return PostedClaim{}, fmt.Errorf("warranty claims: accounting begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	svc, err := q.GetService(ctx, db.GetServiceParams{ID: serviceID, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!svc.WarrantyClaimID.Valid || svc.Status != "completed")) {
		return PostedClaim{Skipped: true}, nil
	}
	if err != nil {
		return PostedClaim{}, fmt.Errorf("warranty claims: accounting service: %w", err)
	}
	claim, err := q.GetWarrantyClaimByIDForUpdate(ctx, db.GetWarrantyClaimByIDForUpdateParams{
		ID: svc.WarrantyClaimID.Int64, BrandID: brandID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PostedClaim{Skipped: true}, nil
	}
	if err != nil {
		return PostedClaim{}, fmt.Errorf("warranty claims: accounting claim: %w", err)
	}
	out := PostedClaim{ClaimID: claim.ID}
	if !claim.ReapplyServiceID.Valid || claim.ReapplyServiceID.Int64 != svc.ID ||
		(claim.Status != StatusReapplied && claim.Status != StatusClosed) {
		out.Skipped = true
		return out, nil
	}
	n, err := q.CountWarrantyClaimFinanceEntries(ctx, claim.Uuid)
	if err != nil {
		return PostedClaim{}, fmt.Errorf("warranty claims: accounting existing rows: %w", err)
	}
	if n > 0 {
		out.Skipped = true
		return out, nil
	}
	if out, err = a.post(ctx, q, tx, claim, svc, actor); err != nil {
		return PostedClaim{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PostedClaim{}, fmt.Errorf("warranty claims: accounting commit: %w", err)
	}
	return out, nil
}

// EventNote is the claim timeline event type of the accounting reversal; the
// payload carries ReversalEventKind as "kind".
const (
	EventNote         = "note"
	ReversalEventKind = "accounting_reversed"
)

// ReversedClaim summarizes what ReverseReapplyService wrote.
type ReversedClaim struct {
	ClaimID     int64
	Skipped     bool // nothing to do: not a claim service, or nothing open
	ReversalIDs []int64
}

// ReverseReapplyService reverses the open warranty_claim rows of the claim
// whose re-application service was cancelled, in one transaction, and adds
// a timeline note. A service that is not cancelled, has no claim, is not the
// claim's re-application service or whose claim has nothing open writes
// nothing.
func (a *Accounting) ReverseReapplyService(ctx context.Context, serviceID, brandID int64, reason string,
	actor *int64) (ReversedClaim, error) {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return ReversedClaim{}, fmt.Errorf("warranty claims: reversal begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	svc, err := q.GetService(ctx, db.GetServiceParams{ID: serviceID, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!svc.WarrantyClaimID.Valid || svc.Status != "cancelled")) {
		return ReversedClaim{Skipped: true}, nil
	}
	if err != nil {
		return ReversedClaim{}, fmt.Errorf("warranty claims: reversal service: %w", err)
	}
	claim, err := q.GetWarrantyClaimByIDForUpdate(ctx, db.GetWarrantyClaimByIDForUpdateParams{
		ID: svc.WarrantyClaimID.Int64, BrandID: brandID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReversedClaim{Skipped: true}, nil
	}
	if err != nil {
		return ReversedClaim{}, fmt.Errorf("warranty claims: reversal claim: %w", err)
	}
	out := ReversedClaim{ClaimID: claim.ID, Skipped: true}
	if !claim.ReapplyServiceID.Valid || claim.ReapplyServiceID.Int64 != svc.ID {
		return out, nil
	}
	reason = strings.TrimSpace(reason)
	desc := fmt.Sprintf("Garanti talebi #%d iptal (hizmet %s)", claim.ClaimNo, svc.ServiceNo)
	if reason != "" {
		desc += ": " + reason
	}
	vr, err := a.poster.VoidBySourceTx(ctx, tx, posting.Source{Type: AccountingSourceType, UUID: claim.Uuid}, desc, actor)
	if err != nil {
		return ReversedClaim{}, fmt.Errorf("warranty claims: reverse claim rows: %w", err)
	}
	if len(vr.Reversals) == 0 {
		return out, nil
	}
	out.Skipped = false
	for _, r := range vr.Reversals {
		out.ReversalIDs = append(out.ReversalIDs, r.ID)
	}
	payload, err := json.Marshal(map[string]any{
		"kind": ReversalEventKind, "service_uuid": svc.Uuid.String(), "service_no": svc.ServiceNo,
		"reason": reason, "reversal_ids": out.ReversalIDs,
	})
	if err != nil {
		return ReversedClaim{}, fmt.Errorf("warranty claims: reversal event payload: %w", err)
	}
	var actorID pgtype.Int8
	if actor != nil {
		actorID = int8(*actor)
	}
	if _, err := q.AddWarrantyClaimEvent(ctx, db.AddWarrantyClaimEventParams{
		ClaimID: claim.ID, OrganizationID: claim.OrganizationID, BrandID: claim.BrandID,
		EventType: EventNote, Note: text(desc), Payload: payload, ActorUserID: actorID,
	}); err != nil {
		return ReversedClaim{}, fmt.Errorf("warranty claims: reversal event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReversedClaim{}, fmt.Errorf("warranty claims: reversal commit: %w", err)
	}
	return out, nil
}

func (a *Accounting) post(ctx context.Context, q *db.Queries, tx pgx.Tx, claim db.WarrantyClaim, svc db.Service,
	actor *int64) (PostedClaim, error) {
	out := PostedClaim{ClaimID: claim.ID}
	brand, err := q.GetBrandByID(ctx, claim.BrandID)
	if err != nil {
		return out, fmt.Errorf("warranty claims: accounting brand: %w", err)
	}
	chain, err := orgChain(ctx, q, claim.OrganizationID)
	if err != nil {
		return out, err
	}
	center := chain[len(chain)-1]
	out.Currency = center.Currency
	src := posting.Source{Type: AccountingSourceType, UUID: claim.Uuid}
	desc := fmt.Sprintf("Garanti talebi #%d", claim.ClaimNo)

	// Product cost on the center's book.
	items, err := q.ListWarrantyReapplyItemCosts(ctx, db.ListWarrantyReapplyItemCostsParams{
		BuyerOrgID: 0, ServiceID: svc.ID, BrandID: svc.BrandID,
	})
	if err != nil {
		return out, fmt.Errorf("warranty claims: accounting items: %w", err)
	}
	if len(items) == 0 {
		return out, nil
	}
	productIDs := make([]int64, 0, len(items))
	for _, it := range items {
		productIDs = append(productIDs, it.ProductID)
	}
	listPrices, err := q.ListProductPricesForProducts(ctx, db.ListProductPricesForProductsParams{
		BrandID: claim.BrandID, ProductIds: uniquePositive(productIDs),
	})
	if err != nil {
		return out, fmt.Errorf("warranty claims: accounting center prices: %w", err)
	}
	centerCost := new(big.Rat)
	for _, it := range items {
		var price *big.Rat
		for _, p := range listPrices {
			if p.ProductID == it.ProductID && p.Currency == brand.Currency && p.PurchasePrice.Valid {
				price = numRat(p.PurchasePrice)
			}
		}
		if price == nil {
			a.log.Warn("warranty_claim_accounting_no_center_cost", "claim_id", claim.ID, "product_id", it.ProductID)
			continue
		}
		centerCost.Add(centerCost, new(big.Rat).Mul(numRat(it.Consumed), price))
	}
	if amount := money(centerCost); amount != "" {
		if _, err := a.poster.PostExpense(ctx, tx, posting.Entry{
			OrganizationID: center.ID, Source: src, Role: RoleWarrantyCost,
			Category: accounting.CategoryWarrantyCost, Amount: amount, Currency: brand.Currency,
			AllowNoTarget: true, Description: desc, ActorUserID: actor,
		}); err != nil {
			return out, fmt.Errorf("warranty claims: post warranty cost: %w", err)
		}
		out.Rows++
	}

	// Product refund down the chain: each hop credits the child its price.
	pricing := pricingusecase.New(q)
	for i := 0; i+1 < len(chain); i++ {
		child, parent := chain[i], chain[i+1]
		amount, err := a.childCost(ctx, q, pricing, svc, child, productIDs, brand.Currency, claim.ID)
		if err != nil {
			return out, err
		}
		if amount == "" {
			continue
		}
		if _, err := a.poster.PostStockReturnTx(ctx, tx, posting.StockReturn{
			Source: src, ChildOrgID: child.ID, ParentOrgID: parent.ID,
			Amount: amount, Currency: brand.Currency, Description: desc, ActorUserID: actor,
		}); err != nil {
			return out, fmt.Errorf("warranty claims: post product refund %d→%d: %w", parent.ID, child.ID, err)
		}
		out.Rows += 2
	}

	// Labor down the chain.
	rule, labor := a.laborRule(ctx)
	out.Rule = rule
	if amount := money(labor); amount != "" {
		for i := len(chain) - 1; i > 0; i-- {
			parent, child := chain[i], chain[i-1]
			if _, err := a.poster.PostExpense(ctx, tx, posting.Entry{
				OrganizationID: parent.ID, CounterpartyOrgID: child.ID, Source: src, Role: RoleLaborOut,
				Category: accounting.CategoryWarrantyLabor, Amount: amount, Currency: center.Currency,
				Description: desc, ActorUserID: actor,
			}); err != nil {
				return out, fmt.Errorf("warranty claims: post labor %d→%d: %w", parent.ID, child.ID, err)
			}
			if _, err := a.poster.PostIncome(ctx, tx, posting.Entry{
				OrganizationID: child.ID, CounterpartyOrgID: parent.ID, Source: src, Role: RoleLaborIn,
				Category: accounting.CategoryWarrantyLaborIncome, Amount: amount, Currency: center.Currency,
				Description: desc, ActorUserID: actor,
			}); err != nil {
				return out, fmt.Errorf("warranty claims: post labor income %d←%d: %w", child.ID, parent.ID, err)
			}
			out.Rows += 2
		}
	}
	return out, nil
}

// childCost is what child paid for the consumed units: its latest received
// order price of each unit in the brand currency, else its F1 purchase
// price. Items without either are left out (logged).
func (a *Accounting) childCost(ctx context.Context, q *db.Queries, pricing *pricingusecase.Service, svc db.Service,
	child db.Organization, productIDs []int64, currency string, claimID int64) (string, error) {
	items, err := q.ListWarrantyReapplyItemCosts(ctx, db.ListWarrantyReapplyItemCostsParams{
		BuyerOrgID: child.ID, ServiceID: svc.ID, BrandID: svc.BrandID,
	})
	if err != nil {
		return "", fmt.Errorf("warranty claims: accounting items of %d: %w", child.ID, err)
	}
	var fallback map[int64]pricingusecase.PurchasePrice
	total := new(big.Rat)
	for _, it := range items {
		var price *big.Rat
		if it.OrderUnitPrice.Valid && it.OrderCurrency == currency {
			price = numRat(it.OrderUnitPrice)
		} else {
			if fallback == nil {
				fallback, err = pricing.BuyerPurchasePrices(ctx, child.ID, child.Type, child.BrandID, uniquePositive(productIDs), currency)
				if errors.Is(err, pricingusecase.ErrForbidden) {
					fallback, err = map[int64]pricingusecase.PurchasePrice{}, nil
				}
				if err != nil {
					return "", fmt.Errorf("warranty claims: accounting purchase prices of %d: %w", child.ID, err)
				}
			}
			if p, ok := fallback[it.ProductID]; ok {
				price, _ = new(big.Rat).SetString(p.Price)
			}
		}
		if price == nil {
			a.log.Warn("warranty_claim_accounting_no_purchase_price", "claim_id", claimID,
				"organization_id", child.ID, "product_id", it.ProductID)
			continue
		}
		total.Add(total, new(big.Rat).Mul(numRat(it.Consumed), price))
	}
	return money(total), nil
}

// orgChain returns the organization and its ancestors up to the root (the
// brand center), child first.
func orgChain(ctx context.Context, q *db.Queries, orgID int64) ([]db.Organization, error) {
	var chain []db.Organization
	seen := map[int64]bool{}
	for id := orgID; ; {
		if seen[id] || len(chain) > 16 {
			return nil, fmt.Errorf("warranty claims: organization chain of %d loops", orgID)
		}
		seen[id] = true
		o, err := q.GetOrganizationByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("warranty claims: organization %d: %w", id, err)
		}
		chain = append(chain, o)
		if !o.ParentID.Valid {
			return chain, nil
		}
		id = o.ParentID.Int64
	}
}

// money renders a positive amount with two decimals; zero, negative or nil
// is "" (nothing to post).
func money(r *big.Rat) string {
	if r == nil {
		return ""
	}
	s := r.FloatString(2)
	if v, ok := new(big.Rat).SetString(s); !ok || v.Sign() <= 0 {
		return ""
	}
	return s
}

// numRat converts a NUMERIC to a rational (NULL is zero).
func numRat(n pgtype.Numeric) *big.Rat {
	if !n.Valid || n.Int == nil {
		return new(big.Rat)
	}
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp == 0 {
		return r
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(absInt32(n.Exp))), nil)
	if n.Exp > 0 {
		return r.Mul(r, new(big.Rat).SetInt(scale))
	}
	return r.Quo(r, new(big.Rat).SetInt(scale))
}

func absInt32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// Get returns the claim detail; center callers with accounting.read also
// get its cost summary (TEC-337).
func (s *Service) Get(ctx context.Context, c Caller, id uuid.UUID) (model.ClaimView, error) {
	if c.OrgType == "customer" || c.OrganizationID <= 0 || !has(c, rbac.PermWarrantyClaimsRead) {
		return model.ClaimView{}, ErrForbidden
	}
	row, err := s.claim(ctx, c, id, false)
	if err != nil {
		return model.ClaimView{}, err
	}
	v, err := s.view(ctx, row, true)
	if err != nil {
		return model.ClaimView{}, err
	}
	if c.OrgType == "center" && has(c, rbac.PermAccountingRead) {
		if v.CostSummary, err = s.costSummary(ctx, row); err != nil {
			return model.ClaimView{}, err
		}
	}
	return v, nil
}

// costSummary sums the open warranty_claim rows of the center's book; nil
// while the claim is not booked. A booked claim whose rows are all reversed
// (TEC-382) nets to zero.
func (s *Service) costSummary(ctx context.Context, row db.WarrantyClaim) (*model.CostSummary, error) {
	center, err := s.q.GetBrandCenter(ctx, row.BrandID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("warranty claims: cost summary center: %w", err)
	}
	entries, err := s.q.ListWarrantyClaimCostEntries(ctx, db.ListWarrantyClaimCostEntriesParams{
		OrganizationID: center.ID, SourceUuid: row.Uuid,
	})
	if err != nil {
		return nil, fmt.Errorf("warranty claims: cost summary: %w", err)
	}
	if len(entries) == 0 {
		n, err := s.q.CountWarrantyClaimFinanceEntries(ctx, row.Uuid)
		if err != nil {
			return nil, fmt.Errorf("warranty claims: cost summary rows: %w", err)
		}
		if n == 0 {
			return nil, nil
		}
	}
	product, labor := new(big.Rat), new(big.Rat)
	for _, e := range entries {
		switch e.Role {
		case RoleWarrantyCost:
			product.Add(product, numRat(e.Amount))
		case RoleLaborOut:
			labor.Add(labor, numRat(e.Amount))
		}
	}
	return &model.CostSummary{
		ProductCost: product.FloatString(2), Labor: labor.FloatString(2), Currency: center.Currency,
	}, nil
}
