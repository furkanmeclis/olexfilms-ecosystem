// Package model holds the stock read API views (TEC-155).
package model

import (
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// OrgRef names an organization.
type OrgRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// LocationRef names a warehouse location.
type LocationRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// Owner is a polymorphic owner. Masked owners belong to an organization the
// viewer may not see: their organization and location are left out.
type Owner struct {
	Type         string       `json:"type"`
	Organization *OrgRef      `json:"organization"`
	Location     *LocationRef `json:"location"`
	Masked       bool         `json:"masked"`
}

// ProductRef is the product of a unit.
type ProductRef struct {
	UUID             uuid.UUID `json:"uuid"`
	SKU              string    `json:"sku"`
	Name             string    `json:"name"`
	UnitType         string    `json:"unit_type"`
	UsesFixedBarcode bool      `json:"uses_fixed_barcode"`
}

// Unit is one physical unit (serial/roll) or one fixed barcode.
type Unit struct {
	UUID            uuid.UUID  `json:"uuid"`
	Barcode         string     `json:"barcode"`
	UnitKind        string     `json:"unit_kind"`
	Source          string     `json:"source"`
	Status          string     `json:"status"`
	InitialMeters   *string    `json:"initial_meters"`
	RemainingMeters *string    `json:"remaining_meters"`
	Product         ProductRef `json:"product"`
	CreatedAt       time.Time  `json:"created_at"`
}

// CurrentState is the single active owner of a serial unit.
type CurrentState struct {
	Owner     Owner     `json:"owner"`
	Holder    OrgRef    `json:"holder"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Holding is the quantity of a fixed barcode at one owner.
type Holding struct {
	Owner    Owner  `json:"owner"`
	Holder   OrgRef `json:"holder"`
	Quantity int32  `json:"quantity"`
}

// Movement is one ledger row of the barcode history.
type Movement struct {
	UUID               uuid.UUID `json:"uuid"`
	Type               string    `json:"type"`
	QuantityDelta      int32     `json:"quantity_delta"`
	MetersDelta        string    `json:"meters_delta"`
	FromStatus         *string   `json:"from_status"`
	ToStatus           *string   `json:"to_status"`
	FromOwner          *Owner    `json:"from_owner"`
	ToOwner            *Owner    `json:"to_owner"`
	Organization       *OrgRef   `json:"organization"`
	OrganizationMasked bool      `json:"organization_masked"`
	ReferenceType      *string   `json:"reference_type"`
	// Document behind the movement and the acting user (TEC-763); null
	// on masked rows.
	Document  *DocumentRef `json:"document"`
	Actor     *UserRef     `json:"actor"`
	Reason    *string      `json:"reason"`
	CreatedAt time.Time    `json:"created_at"`
}

// DocumentRef points at the document of a movement (stock_entry,
// warehouse_transfer, stock_count, service, order, transfer_request,
// stock_reclassification).
type DocumentRef struct {
	Type string    `json:"type"`
	UUID uuid.UUID `json:"uuid"`
}

// MovementPage is a page of the barcode history.
type MovementPage struct {
	Items  []Movement `json:"items"`
	Total  int64      `json:"total"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}

// UnitHistory is the barcode lookup response.
type UnitHistory struct {
	Unit      Unit          `json:"unit"`
	Current   *CurrentState `json:"current"`
	Holdings  []Holding     `json:"holdings"`
	Movements MovementPage  `json:"movements"`
	// Trace (TEC-1006): where the unit came from and went.
	Trace *UnitTrace `json:"trace"`
	// Splits (TEC-1078): the roll this unit was cut from and the parts
	// cut from it (recursively), limited to the viewer's reach.
	Splits UnitSplits `json:"splits"`
	// Credits (TEC-1221): the usage credits of a serial piece unit; nil
	// for a roll or a fixed barcode.
	Credits *UnitCredits `json:"credits"`
}

// UnitCredits is the credit card of a unit (TEC-1221): the balance, the
// services the unit was selected in and the credit movements.
type UnitCredits struct {
	UnitUUID  uuid.UUID        `json:"unit_uuid"`
	Total     int32            `json:"credits_total"`
	Remaining int32            `json:"credits_remaining"`
	UsedCount int32            `json:"used_count"`
	Services  []CreditService  `json:"services"`
	Movements []CreditMovement `json:"movements"`
}

// CreditService is a service the unit was selected in.
type CreditService struct {
	UUID            uuid.UUID  `json:"uuid"`
	ServiceNo       string     `json:"service_no"`
	Status          string     `json:"status"`
	FinishedProduct bool       `json:"finished_product"`
	CreditsTaken    int32      `json:"credits_taken"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at"`
}

// CreditMovement is one credit ledger row.
type CreditMovement struct {
	UUID           uuid.UUID  `json:"uuid"`
	Type           string     `json:"type"`
	TotalDelta     int32      `json:"total_delta"`
	RemainingDelta int32      `json:"remaining_delta"`
	Reason         *string    `json:"reason"`
	ServiceUUID    *uuid.UUID `json:"service_uuid"`
	ServiceNo      *string    `json:"service_no"`
	ActorName      *string    `json:"actor_name"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Split family states of a part (TEC-1078), relative to the organization
// the family is read for.
const (
	FamilyOnHand    = "on_hand"    // available/placed at that organization
	FamilySold      = "sold"       // available/placed at another organization
	FamilyUsed      = "used"       // consumed (service) or used up
	FamilyInTransit = "in_transit" // on the way
	FamilyOther     = "other"      // label not in stock yet, void
)

// UnitSplits is the split family around one unit (TEC-1078).
type UnitSplits struct {
	// Source: the roll the unit was cut from; null for a roll that was
	// never cut off another or one outside the viewer's reach.
	Source *SplitSource `json:"source"`
	// Parts cut from the unit, then from those parts (depth 1, 2, ...).
	Parts []SplitPart `json:"parts"`
}

// SplitSource is the roll a unit was cut from.
type SplitSource struct {
	UUID    uuid.UUID `json:"uuid"`
	Barcode string    `json:"barcode"`
	// Meters cut off it as this unit.
	Meters string `json:"meters"`
}

// SplitPart is one unit cut from the roll.
type SplitPart struct {
	UUID    uuid.UUID `json:"uuid"`
	Barcode string    `json:"barcode"`
	// Depth 1: cut from the unit itself; 2: from one of its parts, ...
	Depth         int32  `json:"depth"`
	SourceBarcode string `json:"source_barcode"`
	// Meters cut (the part's initial meters) and what is left of it.
	Meters          string    `json:"meters"`
	RemainingMeters *string   `json:"remaining_meters"`
	Status          string    `json:"status"`
	FamilyStatus    string    `json:"family_status"`
	OwnerType       *string   `json:"owner_type"`
	Holder          *OrgRef   `json:"holder"`
	Order           *OrderRef `json:"order"`
	SplitAt         time.Time `json:"split_at"`
}

// OrderRef names an order.
type OrderRef struct {
	UUID    uuid.UUID `json:"uuid"`
	OrderNo string    `json:"order_no"`
}

// Unit trace sources (TEC-1006).
const (
	TraceSourceOrder   = "order"   // it went out with an order
	TraceSourceStock   = "stock"   // it never left its issuer
	TraceSourceUnknown = "unknown" // at / used by another organization without an order
)

// UnitTrace is the warehouse-layer trace of a unit (TEC-1006): its origin
// warehouse, the orders that carried it and the services that used it.
// Orders and services outside the viewer's scope are left out.
type UnitTrace struct {
	Source          string         `json:"source"`
	OriginWarehouse *WarehouseRef  `json:"origin_warehouse"`
	Orders          []TraceOrder   `json:"orders"`
	Services        []TraceService `json:"services"`
}

// WarehouseRef names a warehouse.
type WarehouseRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// TraceOrder is an order that carried the unit.
type TraceOrder struct {
	UUID              uuid.UUID  `json:"uuid"`
	OrderNo           string     `json:"order_no"`
	Status            string     `json:"status"`
	ExternalReference *string    `json:"external_reference"`
	Buyer             OrgRef     `json:"buyer"`
	Seller            OrgRef     `json:"seller"`
	CreatedAt         time.Time  `json:"created_at"`
	SubmittedAt       *time.Time `json:"submitted_at"`
	ShippedAt         *time.Time `json:"shipped_at"`
	ReceivedAt        *time.Time `json:"received_at"`
}

// TraceService is a service that used the unit.
type TraceService struct {
	UUID         uuid.UUID     `json:"uuid"`
	ServiceNo    string        `json:"service_no"`
	Status       string        `json:"status"`
	CompletedAt  *time.Time    `json:"completed_at"`
	Organization OrgRef        `json:"organization"`
	Customer     TraceCustomer `json:"customer"`
}

// WarehouseUnitRow is one unit of the warehouse unit list (TEC-1006):
// every unit that originates from the warehouse, in it or gone.
type WarehouseUnitRow struct {
	UUID            uuid.UUID        `json:"uuid"`
	Barcode         string           `json:"barcode"`
	UnitKind        string           `json:"unit_kind"`
	Status          string           `json:"status"`
	InitialMeters   *string          `json:"initial_meters"`
	RemainingMeters *string          `json:"remaining_meters"`
	Product         ProductRef       `json:"product"`
	Source          string           `json:"source"`
	InWarehouse     bool             `json:"in_warehouse"`
	OwnerType       *string          `json:"owner_type"`
	LocationCode    *string          `json:"location_code"`
	Holder          *OrgRef          `json:"holder"`
	Order           *TraceOrderRef   `json:"order"`
	Service         *TraceServiceRef `json:"service"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// TraceOrderRef is the latest order of a list row.
type TraceOrderRef struct {
	UUID      uuid.UUID `json:"uuid"`
	OrderNo   string    `json:"order_no"`
	BuyerName string    `json:"buyer_name"`
}

// TraceServiceRef is the latest service of a list row.
type TraceServiceRef struct {
	UUID         uuid.UUID  `json:"uuid"`
	ServiceNo    string     `json:"service_no"`
	CompletedAt  *time.Time `json:"completed_at"`
	CustomerName string     `json:"customer_name"`
}

// TraceCustomer is the customer of a trace service.
type TraceCustomer struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// CategoryRef names a product category.
type CategoryRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// StockProduct is the product of a stock row.
type StockProduct struct {
	UUID             uuid.UUID   `json:"uuid"`
	SKU              string      `json:"sku"`
	Name             string      `json:"name"`
	UnitType         string      `json:"unit_type"`
	UsesFixedBarcode bool        `json:"uses_fixed_barcode"`
	Active           bool        `json:"active"`
	Category         CategoryRef `json:"category"`
}

// FixedBarcodeQuantity is the quantity on hand of one fixed barcode.
type FixedBarcodeQuantity struct {
	UnitUUID uuid.UUID `json:"unit_uuid"`
	Barcode  string    `json:"barcode"`
	Quantity int32     `json:"quantity"`
}

// ProductStock is one row of the organization or bin product stock.
type ProductStock struct {
	Product       StockProduct           `json:"product"`
	Quantity      int32                  `json:"quantity"`
	Meters        string                 `json:"meters"`
	FixedBarcodes []FixedBarcodeQuantity `json:"fixed_barcodes"`
	UpdatedAt     time.Time              `json:"updated_at"`
	// Balance splits the on-hand stock (TEC-1229): reserved, available,
	// incoming and quarantine. Filled by the organization, warehouse and
	// location product lists.
	Balance *StockBalance `json:"balance,omitempty"`
}

// StockBalance is the TEC-1229 stock view of one product in a scope
// (organization, warehouse or location). Quantities count pieces, rolls
// and fixed barcode pieces; meters are roll meters ("12.50"), kept apart.
// Available = on hand - reserved (never below 0). Incoming is stock on its
// way (in-transit transfers and shipped purchase orders) and quarantine the
// returned units waiting for inspection (TEC-1226); neither is part of on
// hand. No prices.
type StockBalance struct {
	OnHandQuantity     int64  `json:"on_hand_quantity"`
	OnHandMeters       string `json:"on_hand_meters"`
	ReservedQuantity   int64  `json:"reserved_quantity"`
	ReservedMeters     string `json:"reserved_meters"`
	AvailableQuantity  int64  `json:"available_quantity"`
	AvailableMeters    string `json:"available_meters"`
	IncomingQuantity   int64  `json:"incoming_quantity"`
	IncomingMeters     string `json:"incoming_meters"`
	QuarantineQuantity int64  `json:"quarantine_quantity"`
	QuarantineMeters   string `json:"quarantine_meters"`
	// Measures splits the on-hand stock into closed boxes, open rolls, cut
	// pieces and remnants (TEC-1243, K14).
	Measures StockMeasures `json:"measures"`
}

// StockMeasures is the TEC-1243 (K14) split of the on-hand stock of one
// product in a scope:
//   - Boxes: closed boxes, the serial units without meters plus fixed
//     barcode pieces (on-hand quantity minus the metered units).
//   - Remnants: metered units whose rest is below the product's
//     minimum cut (the unit list's remnant filter, TEC-1230); they are not
//     offered and not counted in the meter threshold.
//   - Cut pieces: other metered units cut off another unit (for an order, a
//     buyer or by hand) or reserved for an order line.
//   - Open rolls: the other metered units (a box opened or entered as a
//     roll).
//
// AvailableBoxes is boxes minus their reservations; AvailableMeters is the
// open roll and cut piece meters minus their reserved meters (the meter
// threshold reads it). No prices.
type StockMeasures struct {
	Boxes           int64  `json:"boxes"`
	AvailableBoxes  int64  `json:"available_boxes"`
	OpenRolls       int64  `json:"open_rolls"`
	OpenMeters      string `json:"open_meters"`
	CutPieces       int64  `json:"cut_pieces"`
	CutMeters       string `json:"cut_meters"`
	Remnants        int64  `json:"remnants"`
	RemnantMeters   string `json:"remnant_meters"`
	AvailableMeters string `json:"available_meters"`
}

// WarehouseProductStock is one product of a warehouse (TEC-1090,
// TEC-1069): the product stock row summed over the warehouse's locations
// plus the roll units in stock there. No prices.
type WarehouseProductStock struct {
	ProductStock
	RollCount int32 `json:"roll_count"`
}

// Stock status filter values.
const (
	StatusInStock    = "in_stock"
	StatusOutOfStock = "out_of_stock"
)

// UnitStatuses are the units.status values the unit list filters on.
var UnitStatuses = []string{"reserved", "printed", "available", "placed", "in_transit", "used", "void", "quarantine"}

// UnitFilter filters the organization unit list (TEC-216). TEC-373:
// Statuses is any-of (empty: available and placed); Barcode is an exact
// match, or a prefix with BarcodePrefix; LocationUUIDs is any-of (units
// on those locations; fixed barcodes have no single location);
// UpdatedFrom inclusive, UpdatedBefore exclusive; Sort zero means the
// default (product); SortExplicit marks a sort the request asked for.
type UnitFilter struct {
	ProductUUID   *uuid.UUID
	Statuses      []string
	Barcode       string
	BarcodePrefix bool
	LocationUUIDs []uuid.UUID
	UpdatedFrom   *time.Time
	UpdatedBefore *time.Time
	// Remnant (TEC-1230): only rolls under the product's minimum cut
	// (true) or only the others (false); nil lists both.
	Remnant *bool
	Q       string
	// Family (TEC-1078): the split family of this barcode instead of the
	// on-hand list (the other filters are ignored).
	Family       string
	Sort         apiquery.ResolvedSort
	SortExplicit bool
	Limit        int32
	Offset       int32
}

// UnitPurchasePrice is the purchase price the holding organization pays
// for the unit's product (K8: the sale price of the level above) in the
// organization's currency.
type UnitPurchasePrice struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Source   string `json:"source"`
}

// StockUnitRow is one unit (or one fixed barcode with its quantity on
// hand) held by an organization. PurchasePrice is null when the viewer
// may not read it (pricing.purchase.read) or no price is set.
type StockUnitRow struct {
	UUID            uuid.UUID `json:"uuid"`
	Barcode         string    `json:"barcode"`
	UnitKind        string    `json:"unit_kind"`
	Status          string    `json:"status"`
	Quantity        int32     `json:"quantity"`
	InitialMeters   *string   `json:"initial_meters"`
	RemainingMeters *string   `json:"remaining_meters"`
	// Remnant (TEC-1230): a roll with meters left but fewer than the
	// product's minimum cut (artık parça).
	Remnant bool       `json:"remnant"`
	Product ProductRef `json:"product"`
	// Category of the product (TEC-870: the transfer form's stock picker).
	Category      CategoryRef        `json:"category"`
	Location      *LocationRef       `json:"location"`
	PurchasePrice *UnitPurchasePrice `json:"purchase_price"`
	UpdatedAt     time.Time          `json:"updated_at"`
	// Credits (TEC-1221): usage credits of a serial piece unit (nil for a
	// roll or fixed barcode) and the services it was used in.
	CreditsTotal     *int32 `json:"credits_total"`
	CreditsRemaining *int32 `json:"credits_remaining"`
	CreditsUsed      *int32 `json:"credits_used"`
	// Split family list only (family=<barcode>, TEC-1078): the holder, the
	// state relative to the listed organization, the depth in the family
	// (0: the roll), the source barcode, the meters cut and the order that
	// carried the part.
	Holder        *OrgRef   `json:"holder,omitempty"`
	FamilyStatus  string    `json:"family_status,omitempty"`
	FamilyDepth   *int32    `json:"family_depth,omitempty"`
	SourceBarcode *string   `json:"source_barcode,omitempty"`
	SplitMeters   *string   `json:"split_meters,omitempty"`
	Order         *OrderRef `json:"order,omitempty"`
}

// StockFilter filters the product stock lists.
type StockFilter struct {
	ProductUUID  *uuid.UUID
	CategoryUUID *uuid.UUID
	// Status is "", in_stock or out_of_stock.
	Status string
	Q      string
	// Sort is the resolved sort (TEC-373; zero: by product name).
	Sort   apiquery.ResolvedSort
	Limit  int32
	Offset int32
}
