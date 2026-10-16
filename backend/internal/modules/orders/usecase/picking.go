package usecase

// Picking help for a preparing order (TEC-768, Depo F4.3).
//
// Suggest proposes the seller's units for the rest of a line: placed units
// (a warehouse location owns them) before units the organization holds
// without a location, the oldest entry first (FIFO on units.created_at).
// Units without a location are only proposed with include_unlocated. Rolls
// prefer the first roll that covers the missing meters (a longer roll is
// split on assignment); without one, whole rolls are taken in the same order
// and the last one is cut. Fixed barcodes propose a quantity from the
// unreserved holding that one owner can ship (located holdings only,
// unless include_unlocated).
//
// ApplySuggestions assigns the units the user confirmed through the same
// assignment rules as a scan (assignLocked), all in one transaction.
//
// PickLocations lists where the seller keeps a line's product
// (bin_product_stocks, plus the stock without a location), and PickList is
// the whole order in location order for printing.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// suggestScanLimit bounds the serial candidates read for one line.
const suggestScanLimit = 1000

// MaxApplySuggestions bounds the units of one apply request.
const MaxApplySuggestions = 500

// SuggestionView is one proposed unit and the amount to assign from it.
type SuggestionView struct {
	UnitUUID uuid.UUID `json:"unit_uuid"`
	Barcode  string    `json:"barcode"`
	UnitKind string    `json:"unit_kind"`
	// Quantity is set for pieces (1) and fixed barcodes.
	Quantity *int64 `json:"quantity"`
	// Meters is set for rolls; below RemainingMeters the roll is split.
	Meters          *string    `json:"meters"`
	RemainingMeters *string    `json:"remaining_meters"`
	Located         bool       `json:"located"`
	LocationUUID    *uuid.UUID `json:"location_uuid"`
	LocationCode    *string    `json:"location_code"`
	EnteredAt       time.Time  `json:"entered_at"`
}

// SuggestionsView answers a suggestion request for one line.
type SuggestionsView struct {
	ItemUUID uuid.UUID `json:"item_uuid"`
	// Missing is the amount still to assign; Covered what the suggestions
	// cover (both quantity, or meters for roll lines).
	Missing          string           `json:"missing"`
	Covered          string           `json:"covered"`
	Complete         bool             `json:"complete"`
	IncludeUnlocated bool             `json:"include_unlocated"`
	Suggestions      []SuggestionView `json:"suggestions"`
}

// SuggestInput selects the candidate pool.
type SuggestInput struct {
	IncludeUnlocated bool
}

// Suggest proposes units for the rest of a preparing line (seller,
// orders.ship). Nothing is reserved.
func (s *Service) Suggest(ctx context.Context, c Caller, orderUUID, itemUUID uuid.UUID, in SuggestInput) (SuggestionsView, error) {
	o, it, err := s.sellerLine(ctx, c, orderUUID, itemUUID)
	if err != nil {
		return SuggestionsView{}, err
	}
	if o.Status != StatusPreparing {
		return SuggestionsView{}, ErrNotAssignable
	}
	return s.suggest(ctx, s.q, o, it, in)
}

func (s *Service) suggest(ctx context.Context, q *db.Queries, o db.Order, it db.OrderItem, in SuggestInput) (SuggestionsView, error) {
	done, err := reservedOnLine(ctx, q, it.ID)
	if err != nil {
		return SuggestionsView{}, err
	}
	missing := new(big.Rat).Sub(lineAmount(it), done)
	if missing.Sign() < 0 {
		missing.SetInt64(0)
	}
	out := SuggestionsView{ItemUUID: it.Uuid, IncludeUnlocated: in.IncludeUnlocated, Suggestions: []SuggestionView{}}
	covered := new(big.Rat)
	if missing.Sign() > 0 {
		p, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: o.BrandID})
		if err != nil {
			return SuggestionsView{}, fmt.Errorf("orders: product: %w", err)
		}
		switch {
		case p.UsesFixedBarcode:
			out.Suggestions, err = s.suggestFixed(ctx, q, o, it, missing, in, covered)
		case it.Meters.Valid:
			out.Suggestions, err = suggestRolls(ctx, q, o, it, missing, in, covered)
		default:
			out.Suggestions, err = suggestPieces(ctx, q, o, it, missing, in, covered)
		}
		if err != nil {
			return SuggestionsView{}, err
		}
	}
	prec := 0
	if it.Meters.Valid {
		prec = 2
	}
	out.Missing, out.Covered = missing.FloatString(prec), covered.FloatString(prec)
	out.Complete = covered.Cmp(missing) >= 0
	return out, nil
}

func serialCandidates(ctx context.Context, q *db.Queries, o db.Order, it db.OrderItem, in SuggestInput) ([]db.ListOrderSuggestSerialCandidatesRow, error) {
	rows, err := q.ListOrderSuggestSerialCandidates(ctx, db.ListOrderSuggestSerialCandidatesParams{
		ProductID: it.ProductID, SellerOrgID: o.SellerOrgID, IncludeUnlocated: in.IncludeUnlocated,
		RowLimit: suggestScanLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("orders: suggestion candidates: %w", err)
	}
	return rows, nil
}

func serialSuggestion(r db.ListOrderSuggestSerialCandidatesRow) SuggestionView {
	v := SuggestionView{
		UnitUUID: r.Uuid, Barcode: r.Barcode, UnitKind: r.UnitKind,
		Located:   r.OwnerType == string(ledger.OwnerWarehouseLocation),
		EnteredAt: r.CreatedAt.Time, RemainingMeters: numericTextPtr(r.RemainingMeters, 2),
	}
	if r.LocationUuid.Valid {
		id := uuid.UUID(r.LocationUuid.Bytes)
		v.LocationUUID = &id
	}
	if r.LocationCode != "" {
		code := r.LocationCode
		v.LocationCode = &code
	}
	return v
}

// suggestPieces: one serial piece per missing quantity.
func suggestPieces(ctx context.Context, q *db.Queries, o db.Order, it db.OrderItem, missing *big.Rat, in SuggestInput, covered *big.Rat) ([]SuggestionView, error) {
	rows, err := serialCandidates(ctx, q, o, it, in)
	if err != nil {
		return nil, err
	}
	out := []SuggestionView{}
	one := int64(1)
	for _, r := range rows {
		if covered.Cmp(missing) >= 0 {
			break
		}
		if r.RemainingMeters.Valid {
			continue
		}
		v := serialSuggestion(r)
		v.Quantity = &one
		out = append(out, v)
		covered.Add(covered, big.NewRat(1, 1))
	}
	return out, nil
}

// suggestRolls: the first roll that covers the missing meters; else whole
// rolls in order, the last one cut to the rest.
func suggestRolls(ctx context.Context, q *db.Queries, o db.Order, it db.OrderItem, missing *big.Rat, in SuggestInput, covered *big.Rat) ([]SuggestionView, error) {
	rows, err := serialCandidates(ctx, q, o, it, in)
	if err != nil {
		return nil, err
	}
	meters := func(r db.ListOrderSuggestSerialCandidatesRow) *big.Rat {
		if !r.RemainingMeters.Valid {
			return nil
		}
		return numericRat(r.RemainingMeters)
	}
	take := func(r db.ListOrderSuggestSerialCandidatesRow, amount *big.Rat) SuggestionView {
		v := serialSuggestion(r)
		m := amount.FloatString(2)
		v.Meters = &m
		covered.Add(covered, amount)
		return v
	}
	for _, r := range rows {
		if m := meters(r); m != nil && m.Cmp(missing) >= 0 {
			return []SuggestionView{take(r, missing)}, nil
		}
	}
	out := []SuggestionView{}
	for _, r := range rows {
		m := meters(r)
		if m == nil || m.Sign() <= 0 {
			continue
		}
		rest := new(big.Rat).Sub(missing, covered)
		if rest.Sign() <= 0 {
			break
		}
		if m.Cmp(rest) > 0 {
			m = rest
		}
		out = append(out, take(r, m))
	}
	return out, nil
}

// suggestFixed: quantities from the seller's unreserved fixed holdings.
func (s *Service) suggestFixed(ctx context.Context, q *db.Queries, o db.Order, it db.OrderItem, missing *big.Rat, in SuggestInput, covered *big.Rat) ([]SuggestionView, error) {
	rows, err := q.ListOrderSuggestFixedCandidates(ctx, db.ListOrderSuggestFixedCandidatesParams{
		ProductID: it.ProductID, SellerOrgID: o.SellerOrgID,
	})
	if err != nil {
		return nil, fmt.Errorf("orders: fixed candidates: %w", err)
	}
	out := []SuggestionView{}
	for _, r := range rows {
		rest := new(big.Rat).Sub(missing, covered)
		if rest.Sign() <= 0 {
			break
		}
		// The unit is already on the line: a scan of it is refused too.
		if _, err := q.GetOrderItemUnit(ctx, db.GetOrderItemUnitParams{OrderItemID: it.ID, UnitID: r.ID}); err == nil {
			continue
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("orders: assignment: %w", err)
		}
		reserved, err := q.SumActiveReservedQuantityByUnit(ctx, db.SumActiveReservedQuantityByUnitParams{
			UnitID: r.ID, OrganizationID: o.SellerOrgID,
		})
		if err != nil {
			return nil, fmt.Errorf("orders: reserved quantity: %w", err)
		}
		// Reservations are not tied to an owner: they come off the whole
		// holding. One assignment ships from one owner (fixedSource), so
		// the quantity is bounded by the largest holding of the pool.
		free := r.LocatedQuantity + r.UnlocatedQuantity - reserved
		pool := r.LocatedMax
		if in.IncludeUnlocated {
			pool = max(pool, r.UnlocatedMax)
		}
		qty := min(free, pool, rest.Num().Int64()/max(rest.Denom().Int64(), 1))
		if qty <= 0 {
			continue
		}
		v := SuggestionView{
			UnitUUID: r.Uuid, Barcode: r.Barcode, UnitKind: ledger.KindFixed,
			Quantity: &qty, Located: r.LocatedQuantity > 0, EnteredAt: r.CreatedAt.Time,
		}
		locs, err := q.ListFixedUnitLocations(ctx, db.ListFixedUnitLocationsParams{UnitID: r.ID, SellerOrgID: o.SellerOrgID})
		if err != nil {
			return nil, fmt.Errorf("orders: fixed locations: %w", err)
		}
		if len(locs) > 0 {
			id, code := locs[0].LocationUuid, locs[0].LocationCode
			v.LocationUUID, v.LocationCode = &id, &code
		}
		out = append(out, v)
		covered.Add(covered, new(big.Rat).SetInt64(qty))
	}
	return out, nil
}

// ApplyInput is one confirmed suggestion.
type ApplyInput struct {
	UnitUUID string
	Quantity *int64
	Meters   *string
}

// ApplySuggestions assigns the confirmed units to the line in one
// transaction (seller, orders.ship, preparing). A refused unit rolls the
// whole request back.
func (s *Service) ApplySuggestions(ctx context.Context, c Caller, orderUUID, itemUUID uuid.UUID, units []ApplyInput) (OrderView, error) {
	if len(units) == 0 {
		return OrderView{}, invalid("units", "at least one unit is required")
	}
	if len(units) > MaxApplySuggestions {
		return OrderView{}, invalid("units", fmt.Sprintf("at most %d units", MaxApplySuggestions))
	}
	var result db.Order
	var splits []SplitView
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, it, err := s.lockAssignable(ctx, q, c, orderUUID, itemUUID)
		if err != nil {
			return err
		}
		for i, u := range units {
			split, err := s.assignLocked(ctx, q, tx, c, o, it, AssignInput{
				UnitUUID: u.UnitUUID, Quantity: u.Quantity, Meters: u.Meters,
			})
			if err != nil {
				var ve *ValidationError
				if errors.As(err, &ve) {
					ve.Field = fmt.Sprintf("units[%d].%s", i, ve.Field)
				}
				return err
			}
			if split != nil {
				splits = append(splits, *split)
			}
		}
		result = o
		return s.emit(ctx, tx, events.OrdersUpdated, o, o.Status, c)
	})
	if err != nil {
		return OrderView{}, err
	}
	v, err := s.view(ctx, s.q, c, result)
	if err != nil {
		return OrderView{}, err
	}
	v.Splits = splits
	return v, nil
}

// PickLocationView is one place the seller keeps a product. LocationUUID is
// nil for the stock without a location.
type PickLocationView struct {
	LocationUUID *uuid.UUID `json:"location_uuid"`
	LocationCode *string    `json:"location_code"`
	LocationName *string    `json:"location_name"`
	Quantity     int64      `json:"quantity"`
	Meters       *string    `json:"meters"`
}

// productLocations lists the seller's locations of a product by location
// code, the stock without a location last.
func productLocations(ctx context.Context, q *db.Queries, seller, product int64, roll bool) ([]PickLocationView, error) {
	rows, err := q.ListProductPickLocations(ctx, db.ListProductPickLocationsParams{SellerOrgID: seller, ProductID: product})
	if err != nil {
		return nil, fmt.Errorf("orders: pick locations: %w", err)
	}
	out := make([]PickLocationView, 0, len(rows)+1)
	for _, r := range rows {
		id, code, name := r.LocationUuid, r.LocationCode, r.LocationName
		v := PickLocationView{LocationUUID: &id, LocationCode: &code, LocationName: &name, Quantity: int64(r.Quantity)}
		if roll {
			v.Meters = numericTextPtr(r.Meters, 2)
		}
		out = append(out, v)
	}
	un, err := q.SumProductUnlocatedStock(ctx, db.SumProductUnlocatedStockParams{ProductID: product, SellerOrgID: seller})
	if err != nil {
		return nil, fmt.Errorf("orders: unlocated stock: %w", err)
	}
	if un.Quantity > 0 {
		v := PickLocationView{Quantity: un.Quantity}
		if roll {
			v.Meters = numericTextPtr(un.Meters, 2)
		}
		out = append(out, v)
	}
	return out, nil
}

// PickLocations lists where the seller keeps the product of a line (the
// seller side with orders.ship; any status before shipping).
func (s *Service) PickLocations(ctx context.Context, c Caller, orderUUID, itemUUID uuid.UUID) ([]PickLocationView, error) {
	o, it, err := s.sellerLine(ctx, c, orderUUID, itemUUID)
	if err != nil {
		return nil, err
	}
	return productLocations(ctx, s.q, o.SellerOrgID, it.ProductID, it.Meters.Valid)
}

// sellerLine loads an order of the caller's selling side and one line.
func (s *Service) sellerLine(ctx context.Context, c Caller, orderUUID, itemUUID uuid.UUID) (db.Order, db.OrderItem, error) {
	o, err := s.sellerOrder(ctx, c, orderUUID)
	if err != nil {
		return o, db.OrderItem{}, err
	}
	it, err := s.q.GetOrderItemByUUID(ctx, db.GetOrderItemByUUIDParams{Uuid: itemUUID, BrandID: o.BrandID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && it.OrderID != o.ID) {
		return o, db.OrderItem{}, ErrItemNotFound
	}
	if err != nil {
		return o, db.OrderItem{}, fmt.Errorf("orders: line: %w", err)
	}
	return o, it, nil
}

// sellerOrder loads a visible order the caller sells with orders.ship.
func (s *Service) sellerOrder(ctx context.Context, c Caller, orderUUID uuid.UUID) (db.Order, error) {
	o, err := s.q.GetOrderByUUID(ctx, db.GetOrderByUUIDParams{Uuid: orderUUID, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Order{}, ErrNotFound
	}
	if err != nil {
		return db.Order{}, fmt.Errorf("orders: get: %w", err)
	}
	if !visible(c, o) {
		return db.Order{}, ErrNotFound
	}
	if partyOf(c, o) != PartySeller || !c.can(rbac.PermOrdersShip) {
		return db.Order{}, ErrForbidden
	}
	return o, nil
}

// Pick list row kinds.
const (
	PickAssigned = "assigned"
	PickStock    = "stock"
)

// PickRow is one stop of the pick list: an assigned unit at its location,
// or (for a line still missing units) a location holding the product.
type PickRow struct {
	Kind         string     `json:"kind"`
	ItemUUID     uuid.UUID  `json:"item_uuid"`
	Product      ProductRef `json:"product"`
	LocationCode *string    `json:"location_code"`
	LocationName *string    `json:"location_name"`
	Barcode      *string    `json:"barcode"`
	Quantity     *int64     `json:"quantity"`
	Meters       *string    `json:"meters"`
}

// PickLineView is a line of the pick list with its amounts.
type PickLineView struct {
	ItemUUID uuid.UUID  `json:"item_uuid"`
	Product  ProductRef `json:"product"`
	Quantity *int32     `json:"quantity"`
	Meters   *string    `json:"meters"`
	Assigned string     `json:"assigned"`
}

// PickListView is the printable pick list of an order: rows in location
// order (no location last), then product and barcode.
type PickListView struct {
	OrderUUID   uuid.UUID      `json:"order_uuid"`
	OrderNo     string         `json:"order_no"`
	Status      string         `json:"status"`
	Seller      OrgRef         `json:"seller"`
	Buyer       OrgRef         `json:"buyer"`
	GeneratedAt time.Time      `json:"generated_at"`
	Lines       []PickLineView `json:"lines"`
	Rows        []PickRow      `json:"rows"`
}

// PickList builds the pick list of an order the caller sells (orders.ship).
func (s *Service) PickList(ctx context.Context, c Caller, orderUUID uuid.UUID) (PickListView, error) {
	o, err := s.sellerOrder(ctx, c, orderUUID)
	if err != nil {
		return PickListView{}, err
	}
	v, err := s.view(ctx, s.q, c, o)
	if err != nil {
		return PickListView{}, err
	}
	out := PickListView{
		OrderUUID: o.Uuid, OrderNo: o.OrderNo, Status: o.Status, Seller: v.Seller, Buyer: v.Buyer,
		GeneratedAt: s.now().UTC(), Lines: []PickLineView{}, Rows: []PickRow{},
	}
	items, err := s.q.ListOrderItems(ctx, o.ID)
	if err != nil {
		return PickListView{}, fmt.Errorf("orders: lines: %w", err)
	}
	assigned, err := s.q.ListOrderAssignedUnitLocations(ctx, o.ID)
	if err != nil {
		return PickListView{}, fmt.Errorf("orders: assigned locations: %w", err)
	}
	byItem := map[int64][]db.ListOrderAssignedUnitLocationsRow{}
	for _, a := range assigned {
		byItem[a.OrderItemID] = append(byItem[a.OrderItemID], a)
	}
	for i, it := range items {
		iv := v.Items[i]
		out.Lines = append(out.Lines, PickLineView{
			ItemUUID: it.Uuid, Product: iv.Product, Quantity: iv.Quantity, Meters: iv.Meters, Assigned: iv.Assigned,
		})
		for _, a := range byItem[it.ID] {
			row := PickRow{Kind: PickAssigned, ItemUUID: it.Uuid, Product: iv.Product, Meters: numericTextPtr(a.Meters, 2)}
			bc := a.Barcode
			row.Barcode = &bc
			if a.Quantity.Valid {
				qv := int64(a.Quantity.Int32)
				row.Quantity = &qv
			}
			if a.UnitKind == ledger.KindFixed {
				locs, err := s.q.ListFixedUnitLocations(ctx, db.ListFixedUnitLocationsParams{UnitID: a.UnitID, SellerOrgID: o.SellerOrgID})
				if err != nil {
					return PickListView{}, fmt.Errorf("orders: fixed locations: %w", err)
				}
				if len(locs) > 0 {
					code := locs[0].LocationCode
					row.LocationCode = &code
				}
			} else if a.LocationCode != "" {
				code := a.LocationCode
				row.LocationCode = &code
			}
			out.Rows = append(out.Rows, row)
		}
		done, err := reservedOnLine(ctx, s.q, it.ID)
		if err != nil {
			return PickListView{}, err
		}
		// Stock rows only while the line still waits for units.
		if done.Cmp(lineAmount(it)) >= 0 || !pickingStatus(o.Status) {
			continue
		}
		locs, err := productLocations(ctx, s.q, o.SellerOrgID, it.ProductID, it.Meters.Valid)
		if err != nil {
			return PickListView{}, err
		}
		for _, l := range locs {
			qv := l.Quantity
			out.Rows = append(out.Rows, PickRow{
				Kind: PickStock, ItemUUID: it.Uuid, Product: iv.Product,
				LocationCode: l.LocationCode, LocationName: l.LocationName, Quantity: &qv, Meters: l.Meters,
			})
		}
	}
	sort.SliceStable(out.Rows, func(i, j int) bool {
		a, b := out.Rows[i], out.Rows[j]
		if (a.LocationCode == nil) != (b.LocationCode == nil) {
			return a.LocationCode != nil
		}
		if a.LocationCode != nil && *a.LocationCode != *b.LocationCode {
			return *a.LocationCode < *b.LocationCode
		}
		if a.Product.SKU != b.Product.SKU {
			return a.Product.SKU < b.Product.SKU
		}
		return strPtr(a.Barcode) < strPtr(b.Barcode)
	})
	return out, nil
}

// pickingStatus: the order is accepted and not yet ready.
func pickingStatus(status string) bool {
	return status == StatusApproved || status == StatusProcessing || status == StatusPreparing
}

func strPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
