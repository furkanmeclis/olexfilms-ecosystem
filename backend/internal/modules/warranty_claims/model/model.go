// Package model contains warranty claim API models.
package model

import (
	"time"

	"github.com/google/uuid"
)

type PartInput struct {
	PartKey         string     `json:"part_key"`
	ServiceItemUUID *uuid.UUID `json:"service_item_uuid"`
	Note            string     `json:"note"`
}

type CreateInput struct {
	WarrantyUUID uuid.UUID   `json:"warranty_uuid"`
	Description  string      `json:"description"`
	Parts        []PartInput `json:"parts"`
}

type TransitionInput struct {
	Status          string `json:"status"`
	RejectionReason string `json:"rejection_reason"`
}

type ListFilter struct {
	Status      string
	WarrantyID  uuid.UUID
	ServiceID   uuid.UUID
	VehicleID   uuid.UUID
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Limit       int32
	Offset      int32
}

type CoverageCheck struct {
	OK        bool     `json:"ok"`
	Reasons   []string `json:"reasons"`
	CheckedAt string   `json:"checked_at"`
}

type PartView struct {
	UUID    uuid.UUID `json:"uuid"`
	PartKey string    `json:"part_key"`
	Note    string    `json:"note,omitempty"`
}

type PhotoView struct {
	UUID      uuid.UUID `json:"uuid"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

type ClaimView struct {
	UUID            uuid.UUID     `json:"uuid"`
	ClaimNo         int64         `json:"claim_no"`
	WarrantyUUID    uuid.UUID     `json:"warranty_uuid"`
	Status          string        `json:"status"`
	Description     string        `json:"description,omitempty"`
	RejectionReason *string       `json:"rejection_reason,omitempty"`
	CoverageCheck   CoverageCheck `json:"coverage_check"`
	AIDamageType    *string       `json:"ai_damage_type,omitempty"`
	AISummary       *string       `json:"ai_summary,omitempty"`
	AIConfidence    *string       `json:"ai_confidence,omitempty"`
	AITriagedAt     *time.Time    `json:"ai_triaged_at,omitempty"`
	Parts           []PartView    `json:"parts,omitempty"`
	Photos          []PhotoView   `json:"photos,omitempty"`
	CostSummary     *CostSummary  `json:"cost_summary,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// CostSummary is the center's warranty cost of a claim (TEC-337): the
// product cost (warranty_cost) and the labor the center credited, in the
// center's currency. Only on the detail, for center callers with
// accounting.read, once the claim is booked.
type CostSummary struct {
	ProductCost string `json:"product_cost"`
	Labor       string `json:"labor"`
	Currency    string `json:"currency"`
}

type ListView struct {
	Items  []ClaimView `json:"items"`
	Total  int64       `json:"total"`
	Limit  int32       `json:"limit"`
	Offset int32       `json:"offset"`
}

type PortalClaimView struct {
	UUID      uuid.UUID `json:"uuid"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ReportFilter struct {
	From  *time.Time
	To    *time.Time
	Group string
}

type FailureRateRow struct {
	Group              string     `json:"group"`
	ProductUUID        uuid.UUID  `json:"product_uuid"`
	ProductSKU         string     `json:"product_sku"`
	ProductName        string     `json:"product_name"`
	LotUUID            *uuid.UUID `json:"lot_uuid,omitempty"`
	LotCode            *string    `json:"lot_code,omitempty"`
	WarrantyCount      int64      `json:"warranty_count"`
	ClaimCount         int64      `json:"claim_count"`
	ApprovedClaimCount int64      `json:"approved_claim_count"`
	ClaimRate          float64    `json:"claim_rate"`
	ApprovedRate       float64    `json:"approved_rate"`
}

type FailureRateReport struct {
	Group string           `json:"group"`
	Items []FailureRateRow `json:"items"`
}

type DealerReportRow struct {
	OrganizationUUID   uuid.UUID `json:"organization_uuid"`
	OrganizationName   string    `json:"organization_name"`
	OrganizationType   string    `json:"organization_type"`
	ClaimCount         int64     `json:"claim_count"`
	ApprovedClaimCount int64     `json:"approved_claim_count"`
	RejectedClaimCount int64     `json:"rejected_claim_count"`
	ApprovalRate       float64   `json:"approval_rate"`
}

type DealerReport struct {
	Items []DealerReportRow `json:"items"`
}

type PartsReportRow struct {
	PartKey            string     `json:"part_key"`
	ProductUUID        *uuid.UUID `json:"product_uuid,omitempty"`
	ProductSKU         string     `json:"product_sku,omitempty"`
	ProductName        string     `json:"product_name,omitempty"`
	PartCount          int64      `json:"part_count"`
	ClaimCount         int64      `json:"claim_count"`
	ApprovedClaimCount int64      `json:"approved_claim_count"`
}

type PartsReport struct {
	Items []PartsReportRow `json:"items"`
}
