package model

import (
	"time"

	"github.com/google/uuid"
)

// LabelTemplate is a label print layout (TEC-202).
type LabelTemplate struct {
	UUID         uuid.UUID `json:"uuid"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	Symbology    string    `json:"symbology"`
	LogoMode     string    `json:"logo_mode"`
	LogoText     *string   `json:"logo_text"`
	LogoImage    *string   `json:"logo_image"`
	WidthMm      string    `json:"width_mm"`
	HeightMm     string    `json:"height_mm"`
	Columns      int       `json:"columns"`
	ShowName     bool      `json:"show_name"`
	ShowCodeText bool      `json:"show_code_text"`
	IsDefault    bool      `json:"is_default"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// BarcodeBatch is one reservation of N barcodes for a product (TEC-202).
type BarcodeBatch struct {
	UUID          uuid.UUID          `json:"uuid"`
	Product       BarcodeProduct     `json:"product"`
	Quantity      int32              `json:"quantity"`
	Prefix        string             `json:"prefix"`
	FirstBarcode  string             `json:"first_barcode"`
	LastBarcode   string             `json:"last_barcode"`
	Meters        *string            `json:"meters"`
	TemplateUUID  *uuid.UUID         `json:"template_uuid"`
	PrintCount    int32              `json:"print_count"`
	LastPrintedAt *time.Time         `json:"last_printed_at"`
	LabelsURL     string             `json:"labels_url"`
	CreatedAt     time.Time          `json:"created_at"`
	Units         []BarcodeBatchUnit `json:"units,omitempty"`
}

// BarcodeProduct names the product of a batch.
type BarcodeProduct struct {
	UUID             uuid.UUID `json:"uuid"`
	SKU              string    `json:"sku"`
	Name             string    `json:"name"`
	UnitType         string    `json:"unit_type"`
	UsesFixedBarcode bool      `json:"uses_fixed_barcode"`
}

// BarcodeBatchUnit is one unit of a batch.
type BarcodeBatchUnit struct {
	UUID    uuid.UUID `json:"uuid"`
	Barcode string    `json:"barcode"`
	Status  string    `json:"status"`
}
