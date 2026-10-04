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
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
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
