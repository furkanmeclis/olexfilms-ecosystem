// Package model holds the catalog API views and inputs (TEC-145).
package model

import (
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// Unit types of a product (products.unit_type).
const (
	UnitPiece     = "piece"
	UnitRollMeter = "roll_meter"
)

// Locked fields (TEC-268): products.locked_fields names the fields a remote
// hub owns (Glorian catalog pull, K2). The panel may not change them; the
// names are the API input fields.
const (
	FieldCategoryUUID           = "category_uuid"
	FieldSKU                    = "sku"
	FieldName                   = "name"
	FieldDescriptionMD          = "description_md"
	FieldWarrantyDurationMonths = "warranty_duration_months"
	FieldMicronThickness        = "micron_thickness"
	FieldActive                 = "active"
	FieldAvailableParts         = "available_parts"
)

// SyncedProductLockedFields are the fields the Glorian pull writes on a
// product; images, unit_type and uses_fixed_barcode stay local.
var SyncedProductLockedFields = []string{
	FieldCategoryUUID, FieldSKU, FieldName, FieldDescriptionMD,
	FieldWarrantyDurationMonths, FieldMicronThickness, FieldActive,
}

// SyncedCategoryLockedFields are the category fields a brand with an
// integration connection takes from the hub; sort stays local.
var SyncedCategoryLockedFields = []string{FieldName, FieldAvailableParts, FieldActive}

// Category is the API view of a product category.
type Category struct {
	UUID           uuid.UUID `json:"uuid"`
	Name           string    `json:"name"`
	AvailableParts []string  `json:"available_parts"`
	Sort           int32     `json:"sort"`
	Active         bool      `json:"active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CategoryRef is the short category view embedded in a product.
type CategoryRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Image is one product image (a storage object key).
type Image struct {
	Key  string `json:"key"`
	Sort int    `json:"sort"`
}

// Product is the API view of a product. The F2 Glorian sync columns are
// exposed read-only (external_id, locked_fields).
type Product struct {
	UUID                   uuid.UUID   `json:"uuid"`
	Category               CategoryRef `json:"category"`
	SKU                    string      `json:"sku"`
	Name                   string      `json:"name"`
	DescriptionMD          string      `json:"description_md"`
	WarrantyDurationMonths *int32      `json:"warranty_duration_months"`
	MicronThickness        *float64    `json:"micron_thickness"`
	Images                 []Image     `json:"images"`
	UnitType               string      `json:"unit_type"`
	UsesFixedBarcode       bool        `json:"uses_fixed_barcode"`
	Active                 bool        `json:"active"`
	ExternalID             *string     `json:"external_id"`
	LockedFields           []string    `json:"locked_fields"`
	CreatedAt              time.Time   `json:"created_at"`
	UpdatedAt              time.Time   `json:"updated_at"`
}

// CategoryInput creates a category (POST) or patches it (PATCH: nil keeps).
type CategoryInput struct {
	Name           *string   `json:"name"`
	AvailableParts *[]string `json:"available_parts"`
	Sort           *int32    `json:"sort"`
	Active         *bool     `json:"active"`
}

// ProductInput creates a product (POST) or patches it (PATCH: nil keeps).
type ProductInput struct {
	CategoryUUID           *uuid.UUID `json:"category_uuid"`
	SKU                    *string    `json:"sku"`
	Name                   *string    `json:"name"`
	DescriptionMD          *string    `json:"description_md"`
	WarrantyDurationMonths *int32     `json:"warranty_duration_months"`
	MicronThickness        *float64   `json:"micron_thickness"`
	Images                 *[]Image   `json:"images"`
	UnitType               *string    `json:"unit_type"`
	UsesFixedBarcode       *bool      `json:"uses_fixed_barcode"`
	Active                 *bool      `json:"active"`

	// WarrantySet / MicronSet mark that the PATCH body named the field, so an
	// explicit null clears it. Filled by the handler, never from JSON.
	WarrantySet bool `json:"-"`
	MicronSet   bool `json:"-"`
}

// CategorySort is the sort contract of GET /v1/catalog/categories
// (TEC-369, docs/list-contract.md). Default: the display order (sort).
var CategorySort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"name": "name", "sort": "sort", "active": "active",
		"created_at": "created_at", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "sort"},
}

// ProductSort is the sort contract of GET /v1/catalog/products (TEC-369).
var ProductSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"sku": "sku", "name": "name", "category": "category", "unit_type": "unit_type",
		"warranty_duration_months": "warranty_duration_months", "micron_thickness": "micron_thickness",
		"active": "active", "created_at": "created_at", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "name"},
}

// CategoryFilter narrows the category list.
type CategoryFilter struct {
	Q      string
	Active *bool
	Sort   apiquery.ResolvedSort
	Limit  int32
	Offset int32
}

// ProductFilter narrows the product list. CategoryUUIDs and UnitTypes are
// multi-value (any of); the ranges are inclusive (created: [From, Before)).
type ProductFilter struct {
	Q                string
	CategoryUUIDs    []uuid.UUID
	Active           *bool
	UnitTypes        []string
	UsesFixedBarcode *bool
	Warranty         apiquery.NumberRange
	Micron           apiquery.NumberRange
	Created          apiquery.TimeRange
	Sort             apiquery.ResolvedSort
	Limit            int32
	Offset           int32
}

// CategoryOrderInput is the body of PUT /v1/catalog/categories/order: the
// category uuids in the wanted display order (a subset is allowed).
type CategoryOrderInput struct {
	UUIDs []uuid.UUID `json:"uuids"`
}

// BulkActiveResult is the outcome of a bulk activate/deactivate.
type BulkActiveResult struct {
	Requested int         `json:"requested"`
	Updated   int         `json:"updated"`
	UUIDs     []uuid.UUID `json:"uuids"`
}
