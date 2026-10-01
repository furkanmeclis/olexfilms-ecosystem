// Package model holds the stock read API views (TEC-155).
package model

import (
	"time"

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
	Reason             *string   `json:"reason"`
	CreatedAt          time.Time `json:"created_at"`
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
}

// Stock status filter values.
const (
	StatusInStock    = "in_stock"
	StatusOutOfStock = "out_of_stock"
)

// StockFilter filters the product stock lists.
type StockFilter struct {
	ProductUUID  *uuid.UUID
	CategoryUUID *uuid.UUID
	// Status is "", in_stock or out_of_stock.
	Status string
	Q      string
	Limit  int32
	Offset int32
}
