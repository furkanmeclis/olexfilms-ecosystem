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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// OrgRef names an order party.
type OrgRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// ProductRef names an order line's product.
type ProductRef struct {
	UUID     uuid.UUID `json:"uuid"`
	SKU      string    `json:"sku"`
	Name     string    `json:"name"`
	UnitType string    `json:"unit_type"`
}

// AssignedUnitView is a unit assigned to a line (TEC-167). Shipped is true
// once its order_out movement is written.
type AssignedUnitView struct {
	UnitUUID   uuid.UUID `json:"unit_uuid"`
	Barcode    string    `json:"barcode"`
	UnitKind   string    `json:"unit_kind"`
	Quantity   *int32    `json:"quantity"`
	Meters     *string   `json:"meters"`
	Shipped    bool      `json:"shipped"`
	AssignedAt time.Time `json:"assigned_at"`
}

// ItemView is one order line. unit_price is the buyer's purchase price,
// frozen at approval. Assigned is the amount covered by the assigned units
// (quantity, or meters for roll lines).
type ItemView struct {
	UUID        uuid.UUID          `json:"uuid"`
	Product     ProductRef         `json:"product"`
	Quantity    *int32             `json:"quantity"`
	Meters      *string            `json:"meters"`
	UnitPrice   string             `json:"unit_price"`
	PriceSource string             `json:"price_source"`
	LineTotal   string             `json:"line_total"`
	Note        *string            `json:"note"`
	Assigned    string             `json:"assigned"`
	Units       []AssignedUnitView `json:"units"`
}

// HistoryView is one status change.
type HistoryView struct {
	FromStatus *string   `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Reason     *string   `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}

// OrderView is an order as the API returns it.
type OrderView struct {
	UUID         uuid.UUID       `json:"uuid"`
	OrderNo      string          `json:"order_no"`
	Status       string          `json:"status"`
	StatusLabel  string          `json:"status_label"`
	Role         string          `json:"role"`
	Seller       OrgRef          `json:"seller"`
	Buyer        OrgRef          `json:"buyer"`
	Currency     string          `json:"currency"`
	Subtotal     string          `json:"subtotal"`
	TaxTotal     string          `json:"tax_total"`
	Total        string          `json:"total"`
	RateSnapshot json.RawMessage `json:"rate_snapshot"`
	TryRate      *string         `json:"try_rate"`
	Note         *string         `json:"note"`
	CancelReason *string         `json:"cancel_reason"`
	SubmittedAt  *time.Time      `json:"submitted_at"`
	ApprovedAt   *time.Time      `json:"approved_at"`
	ReadyAt      *time.Time      `json:"ready_at"`
	ShippedAt    *time.Time      `json:"shipped_at"`
	CancelledAt  *time.Time      `json:"cancelled_at"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	// AvailableTransitions are the statuses the caller may move the order to.
	AvailableTransitions []string      `json:"available_transitions"`
	Items                []ItemView    `json:"items,omitempty"`
	History              []HistoryView `json:"history,omitempty"`
	// Split is set on an assignment that cut the meters off a roll as a
	// new unit (TEC-184): the new barcode to label (printing: TEC-95).
	Split *SplitView `json:"split,omitempty"`
}

// SplitView is the roll split an assignment made.
type SplitView struct {
	UUID                  uuid.UUID `json:"uuid"`
	Meters                string    `json:"meters"`
	SourceUnitUUID        uuid.UUID `json:"source_unit_uuid"`
	SourceBarcode         string    `json:"source_barcode"`
	SourceRemainingMeters string    `json:"source_remaining_meters"`
	NewUnitUUID           uuid.UUID `json:"new_unit_uuid"`
	NewBarcode            string    `json:"new_barcode"`
	Replayed              bool      `json:"replayed"`
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

// StatusLabelKey is the i18n key of a status label.
func StatusLabelKey(status string) string { return "orders.status." + status }

type orgCache map[int64]OrgRef

func (m orgCache) get(ctx context.Context, q *db.Queries, id int64) (OrgRef, error) {
	if r, ok := m[id]; ok {
		return r, nil
	}
	o, err := q.GetOrganizationByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		m[id] = OrgRef{}
		return OrgRef{}, nil
	}
	if err != nil {
		return OrgRef{}, fmt.Errorf("orders: organization: %w", err)
	}
	r := OrgRef{UUID: o.Uuid, Name: o.Name, Type: o.Type}
	m[id] = r
	return r, nil
}

func (s *Service) summary(ctx context.Context, q *db.Queries, c Caller, o db.Order, orgs orgCache) (OrderView, error) {
	seller, err := orgs.get(ctx, q, o.SellerOrgID)
	if err != nil {
		return OrderView{}, err
	}
	buyer, err := orgs.get(ctx, q, o.BuyerOrgID)
	if err != nil {
		return OrderView{}, err
	}
	party := partyOf(c, o)
	role := string(party)
	if role == "" {
		role = "observer"
	}
	v := OrderView{
		UUID: o.Uuid, OrderNo: o.OrderNo, Status: o.Status,
		StatusLabel: i18n.Translate(i18n.FromContext(ctx).Locale, StatusLabelKey(o.Status)),
		Role:        role, Seller: seller, Buyer: buyer, Currency: strings.TrimSpace(o.Currency),
		Subtotal: numericText(o.Subtotal, 2), TaxTotal: numericText(o.TaxTotal, 2), Total: numericText(o.Total, 2),
		TryRate: rateText(o.TryRate), Note: textPtr(o.Note), CancelReason: textPtr(o.CancelReason),
		SubmittedAt: tsPtr(o.SubmittedAt), ApprovedAt: tsPtr(o.ApprovedAt),
		ReadyAt: tsPtr(o.ReadyAt), ShippedAt: tsPtr(o.ShippedAt), CancelledAt: tsPtr(o.CancelledAt),
		CreatedAt: o.CreatedAt.Time, UpdatedAt: o.UpdatedAt.Time,
		AvailableTransitions: availableTransitions(o.Status, party, c.can),
	}
	if len(o.RateSnapshot) > 0 {
		v.RateSnapshot = json.RawMessage(o.RateSnapshot)
	} else {
		v.RateSnapshot = json.RawMessage("null")
	}
	return v, nil
}

// view is the full order: summary, lines and history.
func (s *Service) view(ctx context.Context, q *db.Queries, c Caller, o db.Order) (OrderView, error) {
	v, err := s.summary(ctx, q, c, o, orgCache{})
	if err != nil {
		return OrderView{}, err
	}
	items, err := q.ListOrderItems(ctx, o.ID)
	if err != nil {
		return OrderView{}, fmt.Errorf("orders: lines: %w", err)
	}
	assigned, err := q.ListOrderItemUnitsByOrder(ctx, o.ID)
	if err != nil {
		return OrderView{}, fmt.Errorf("orders: assigned units: %w", err)
	}
	byItem := map[int64][]db.ListOrderItemUnitsByOrderRow{}
	for _, a := range assigned {
		byItem[a.OrderItemID] = append(byItem[a.OrderItemID], a)
	}
	v.Items = make([]ItemView, 0, len(items))
	for _, it := range items {
		p, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: o.BrandID})
		if err != nil {
			return OrderView{}, fmt.Errorf("orders: product: %w", err)
		}
		iv := ItemView{
			UUID:      it.Uuid,
			Product:   ProductRef{UUID: p.Uuid, SKU: p.Sku, Name: p.Name, UnitType: p.UnitType},
			Meters:    numericTextPtr(it.Meters, 2),
			UnitPrice: numericText(it.UnitPrice, 4), PriceSource: it.PriceSource,
			LineTotal: numericText(it.LineTotal, 2), Note: textPtr(it.Note),
		}
		if it.Quantity.Valid {
			qv := it.Quantity.Int32
			iv.Quantity = &qv
		}
		iv.Units, iv.Assigned = assignedUnits(byItem[it.ID], it.Meters.Valid)
		v.Items = append(v.Items, iv)
	}
	hist, err := q.ListOrderStatusHistory(ctx, o.ID)
	if err != nil {
		return OrderView{}, fmt.Errorf("orders: history: %w", err)
	}
	v.History = make([]HistoryView, 0, len(hist))
	for _, h := range hist {
		v.History = append(v.History, HistoryView{
			FromStatus: textPtr(h.FromStatus), ToStatus: h.ToStatus, Reason: textPtr(h.Reason), CreatedAt: h.CreatedAt.Time,
		})
	}
	return v, nil
}

// Get returns one order the caller's orders.read scope reaches (else 404).
func (s *Service) Get(ctx context.Context, c Caller, orderUUID uuid.UUID) (OrderView, error) {
	o, err := s.q.GetOrderByUUID(ctx, db.GetOrderByUUIDParams{Uuid: orderUUID, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return OrderView{}, ErrNotFound
	}
	if err != nil {
		return OrderView{}, fmt.Errorf("orders: get: %w", err)
	}
	if !visible(c, o) {
		return OrderView{}, ErrNotFound
	}
	return s.view(ctx, s.q, c, o)
}

// Sides of the order list.
const (
	SideAll    = ""
	SideSeller = "seller"
	SideBuyer  = "buyer"
)

// ListFilter narrows the order list.
// CreatedFrom is inclusive, CreatedTo exclusive (TEC-170). TEC-373:
// Statuses, SellerOrgUUIDs and BuyerOrgUUIDs are any-of (empty = all), the
// total range is inclusive, Sort is the resolved sort (zero: the default
// -created_at); SortExplicit marks a sort the request asked for.
type ListFilter struct {
	Side           string
	Statuses       []string
	CreatedFrom    *time.Time
	CreatedTo      *time.Time
	SellerOrgUUIDs []uuid.UUID
	BuyerOrgUUIDs  []uuid.UUID
	TotalMin       *float64
	TotalMax       *float64
	// Q searches the order number, tracking number, external reference
	// and (index) the parties' names and dealer codes (TEC-210).
	Q            string
	Sort         apiquery.ResolvedSort
	SortExplicit bool
	Limit        int32
	Offset       int32
}

// List returns orders inside the caller's orders.read scope: every order
// where an organization of the scope sells or buys (managed: the active
// organization's own sales and purchases). side=seller|buyer limits the
// list to the active organization's sales or purchases. The seller / buyer
// organization filters only narrow that set, so they never leave the scope.
func (s *Service) List(ctx context.Context, c Caller, f ListFilter) ([]OrderView, int64, error) {
	for _, st := range f.Statuses {
		if !IsStatus(st) {
			return nil, 0, invalid("status", "unknown order status")
		}
	}
	if f.CreatedFrom != nil && f.CreatedTo != nil && !f.CreatedTo.After(*f.CreatedFrom) {
		return nil, 0, invalid("created_to", "must be after created_from")
	}
	if f.TotalMin != nil && f.TotalMax != nil && *f.TotalMin > *f.TotalMax {
		return nil, 0, invalid("total_min", "must not exceed total_max")
	}
	brand := c.Org.BrandID
	q := pgtype.Text{}
	if v := strings.TrimSpace(f.Q); v != "" {
		q = pgtype.Text{String: v, Valid: true}
	}
	if f.Side != SideAll && f.Side != SideSeller && f.Side != SideBuyer {
		return nil, 0, invalid("side", "must be seller or buyer")
	}
	if f.Side != SideAll && !c.Filter.AllowsOrg(c.Org.InternalID, brand) {
		return nil, 0, ErrForbidden
	}
	sc := orderScope{side: f.Side, brand: brand, orgID: c.Org.InternalID, orgIDs: c.Filter.OrgIDsArg(), statuses: f.Statuses}
	var (
		rows    []db.Order
		total   int64
		err     error
		indexed bool
	)
	// TEC-210: a text search goes to the orders index when it is up; the
	// date bounds, the party and total filters and an explicit sort are
	// not indexed, so they stay on SQL (TEC-373).
	if q.Valid && !f.sqlOnly() && s.indexEnabled() {
		rows, total, indexed = s.searchIndexed(ctx, sc, q.String, f.Limit, f.Offset)
	}
	if !indexed {
		key, desc := sortArgs(f)
		p := sc.params()
		p.CreatedFrom, p.CreatedBefore, p.Q = tsArg(f.CreatedFrom), tsArg(f.CreatedTo), q
		p.SellerUuids, p.BuyerUuids = f.SellerOrgUUIDs, f.BuyerOrgUUIDs
		p.TotalMin, p.TotalMax = numArg(f.TotalMin), numArg(f.TotalMax)
		p.SortKey, p.SortDesc, p.RowLimit, p.RowOffset = key, desc, f.Limit, f.Offset
		rows, err = s.q.ListOrdersFiltered(ctx, p)
		if err == nil {
			total, err = s.q.CountOrdersFiltered(ctx, db.CountOrdersFilteredParams{
				BrandID: p.BrandID, SellerOrgID: p.SellerOrgID, BuyerOrgID: p.BuyerOrgID, OrgIds: p.OrgIds,
				Statuses: p.Statuses, CreatedFrom: p.CreatedFrom, CreatedBefore: p.CreatedBefore, Q: p.Q,
				SellerUuids: p.SellerUuids, BuyerUuids: p.BuyerUuids, TotalMin: p.TotalMin, TotalMax: p.TotalMax,
			})
		}
	}
	if err != nil {
		return nil, 0, fmt.Errorf("orders: list: %w", err)
	}
	orgs := orgCache{}
	out := make([]OrderView, 0, len(rows))
	for _, o := range rows {
		v, err := s.summary(ctx, s.q, c, o, orgs)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}

// assignedUnits renders the units of a line and the amount they cover.
func assignedUnits(rows []db.ListOrderItemUnitsByOrderRow, meters bool) ([]AssignedUnitView, string) {
	sum := new(big.Rat)
	out := make([]AssignedUnitView, 0, len(rows))
	for _, a := range rows {
		uv := AssignedUnitView{
			UnitUUID: a.UnitUuid, Barcode: a.Barcode, UnitKind: a.UnitKind,
			Meters: numericTextPtr(a.Meters, 2), Shipped: a.MovementID.Valid, AssignedAt: a.AssignedAt.Time,
		}
		if a.Quantity.Valid {
			qv := a.Quantity.Int32
			uv.Quantity = &qv
			sum.Add(sum, new(big.Rat).SetInt64(int64(qv)))
		} else if r := numericRat(a.Meters); r != nil {
			sum.Add(sum, r)
		}
		out = append(out, uv)
	}
	if meters {
		return out, sum.FloatString(2)
	}
	return out, sum.FloatString(0)
}
