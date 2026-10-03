// Package glorian is the outbound Inventory API client the warehouse module
// uses to stay in sync with the Glorian hub (design K2, §7 "Glorian
// senkronu"). The wire contract is the hub's /api/v1/inventory surface
// (olexfilms docs/inventory-api.md, docs/inventory-api-port-blueprint.md)
// as consumed by olexfilms-warehouse InventoryApiClient.php.
//
// Callers depend on the InventoryClient interface; HTTPClient is the real
// implementation and the fake subpackage serves the same contract from an
// httptest server for tests. Clients for a product are obtained through
// ClientResolver, which never falls back silently to another connection.
package glorian

import "time"

// APIVersion is the X-Inventory-Api-Version this client speaks.
const APIVersion = "1"

// Header names of the Inventory API contract.
const (
	HeaderAPIVersion     = "X-Inventory-Api-Version"
	HeaderAPIKey         = "X-Inventory-Api-Key"
	HeaderIdempotencyKey = "Idempotency-Key"
)

// BasePath is appended to a connection's base URL.
const BasePath = "/api/v1/inventory"

// Contract limits enforced client-side before a request leaves the process.
const (
	MaxBulkItems       = 500
	MaxOrderNotesRunes = 2000
	MaxPerPage         = 100
)

// ListParams drives the incremental pull of every list endpoint.
//
// UpdatedSince maps to updated_since (inclusive, ISO-8601); the zero value
// pulls everything. Cursor is the opaque value returned as Page.NextCursor
// (the hub paginates by page number, the cursor wraps it); empty starts at
// the first page. Filters carries endpoint specific query keys
// (is_active, category_id, status, ...).
type ListParams struct {
	UpdatedSince time.Time
	Cursor       string
	PerPage      int
	Filters      map[string]string
}

// Pagination mirrors meta.pagination of a list envelope.
type Pagination struct {
	CurrentPage int `json:"current_page"`
	PerPage     int `json:"per_page"`
	Total       int `json:"total"`
	LastPage    int `json:"last_page"`
}

// Page is one page of a list endpoint. NextCursor is empty on the last page.
type Page[T any] struct {
	Items      []T
	Pagination Pagination
	NextCursor string
}

// Category is a product-categories resource. Soft-deleted rows are returned
// (DeletedAt set) so the pull can mirror deletions.
type Category struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	AvailableParts []string `json:"available_parts"`
	IsActive       bool     `json:"is_active"`
	DeletedAt      *string  `json:"deleted_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// Product is a products resource. The hub never returns a price.
type Product struct {
	ID               string  `json:"id"`
	SKU              string  `json:"sku"`
	Name             string  `json:"name"`
	Description      *string `json:"description"`
	CategoryID       string  `json:"category_id"`
	WarrantyDuration *int    `json:"warranty_duration"`
	MicronThickness  *int    `json:"micron_thickness"`
	IsActive         bool    `json:"is_active"`
	ImageURL         *string `json:"image_url"`
	DeletedAt        *string `json:"deleted_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// DealerSocial is the nested social links block of a dealer.
type DealerSocial struct {
	FacebookURL       *string `json:"facebook_url"`
	InstagramURL      *string `json:"instagram_url"`
	TwitterURL        *string `json:"twitter_url"`
	LinkedinURL       *string `json:"linkedin_url"`
	GoogleBusinessURL *string `json:"google_business_url"`
}

// Dealer is a dealers resource (the warehouse customer source).
type Dealer struct {
	ID         string       `json:"id"`
	DealerCode *string      `json:"dealer_code"`
	Name       string       `json:"name"`
	Email      *string      `json:"email"`
	Phone      *string      `json:"phone"`
	TaxNo      *string      `json:"tax_no"`
	TaxOffice  *string      `json:"tax_office"`
	Address    *string      `json:"address"`
	City       *string      `json:"city"`
	District   *string      `json:"district"`
	Country    *string      `json:"country"`
	Latitude   *float64     `json:"latitude"`
	Longitude  *float64     `json:"longitude"`
	IsActive   bool         `json:"is_active"`
	WebsiteURL *string      `json:"website_url"`
	LogoURL    *string      `json:"logo_url"`
	Social     DealerSocial `json:"social"`
	CreatedAt  string       `json:"created_at"`
	UpdatedAt  string       `json:"updated_at"`
}

// Stock item statuses and locations of the contract.
const (
	StockStatusAvailable        = "available"
	StockStatusReserved         = "reserved"
	StockStatusUsed             = "used"
	StockStatusExternalOutbound = "external_outbound"

	StockLocationCenter = "center"
	StockLocationDealer = "dealer"
)

// StockItem is a stock-items (barcode unit) resource.
type StockItem struct {
	ID        string  `json:"id"`
	Barcode   string  `json:"barcode"`
	ProductID string  `json:"product_id"`
	DealerID  *string `json:"dealer_id"`
	Status    string  `json:"status"`
	Location  string  `json:"location"`
	UpdatedAt string  `json:"updated_at"`
}

// BarcodeUpsert is one row of POST /stock-items/bulk. Empty optional fields
// are omitted so the hub applies its defaults (location=center,
// status=available, dealer_id=null).
type BarcodeUpsert struct {
	Barcode   string  `json:"barcode"`
	ProductID string  `json:"product_id"`
	DealerID  *string `json:"dealer_id,omitempty"`
	Location  string  `json:"location,omitempty"`
	Status    string  `json:"status,omitempty"`
}

// BulkConflict is a soft conflict of the bulk upsert (HTTP 200 envelope).
type BulkConflict struct {
	Barcode string `json:"barcode"`
	Reason  string `json:"reason"`
}

// BulkUpsertResult is the data of POST /stock-items/bulk. Created and
// Updated hold whatever the hub echoes per row (barcodes or resources).
type BulkUpsertResult struct {
	Created   []any          `json:"created"`
	Updated   []any          `json:"updated"`
	Conflicts []BulkConflict `json:"conflicts"`
}

// StockItemPatch is the body of PATCH /stock-items/by-barcode/{barcode}.
type StockItemPatch struct {
	Status   string `json:"status,omitempty"`
	Location string `json:"location,omitempty"`
}

// OrderItemInput is one line of POST /orders.
type OrderItemInput struct {
	ProductID string   `json:"product_id"`
	Quantity  int      `json:"quantity"`
	Barcodes  []string `json:"barcodes,omitempty"`
}

// CreateOrderInput is the body of POST /orders. ExternalReference is the
// idempotency key: a replay returns the existing order, which is why a
// CreateOrder carrying one is the only write the client retries.
type CreateOrderInput struct {
	DealerID          string           `json:"dealer_id"`
	Notes             *string          `json:"notes"`
	ExternalReference string           `json:"external_reference,omitempty"`
	AutoPrepare       *bool            `json:"auto_prepare,omitempty"`
	Items             []OrderItemInput `json:"items"`
}

// OrderItem is one line of an order resource.
type OrderItem struct {
	ID           string   `json:"id"`
	ProductID    string   `json:"product_id"`
	Quantity     int      `json:"quantity"`
	StockItemIDs []string `json:"stock_item_ids"`
	Barcodes     []string `json:"barcodes"`
}

// Order is an orders resource.
type Order struct {
	ID                string      `json:"id"`
	DealerID          string      `json:"dealer_id"`
	Status            string      `json:"status"`
	CargoCompany      *string     `json:"cargo_company"`
	TrackingNumber    *string     `json:"tracking_number"`
	Notes             *string     `json:"notes"`
	ExternalReference *string     `json:"external_reference"`
	Items             []OrderItem `json:"items"`
	CreatedAt         string      `json:"created_at"`
	UpdatedAt         string      `json:"updated_at"`
}

// OrderAction is a status transition of POST /orders/{id}/{action}.
type OrderAction string

// Order actions of the contract.
const (
	OrderActionPrepare OrderAction = "prepare"
	OrderActionShip    OrderAction = "ship"
	OrderActionDeliver OrderAction = "deliver"
	OrderActionReceive OrderAction = "receive"
	OrderActionCancel  OrderAction = "cancel"
)

// Valid reports whether a is one of the contract's order actions.
func (a OrderAction) Valid() bool {
	switch a {
	case OrderActionPrepare, OrderActionShip, OrderActionDeliver, OrderActionReceive, OrderActionCancel:
		return true
	}
	return false
}

// StockAssignment pins stock items to an order line on prepare.
type StockAssignment struct {
	OrderItemID  string   `json:"order_item_id"`
	StockItemIDs []string `json:"stock_item_ids"`
}

// TransitionInput is the optional body of an order action: ship takes
// CargoCompany/TrackingNumber, prepare may take StockAssignments; the
// others send an empty object.
type TransitionInput struct {
	CargoCompany     string            `json:"cargo_company,omitempty"`
	TrackingNumber   string            `json:"tracking_number,omitempty"`
	StockAssignments []StockAssignment `json:"stock_assignments,omitempty"`
}
