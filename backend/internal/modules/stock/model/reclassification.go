package model

import (
	"time"

	"github.com/google/uuid"
)

// Reclassification statuses (stock_reclassifications.status, TEC-157).
const (
	ReclassPending   = "pending"
	ReclassApproved  = "approved"
	ReclassRejected  = "rejected"
	ReclassCancelled = "cancelled"
)

// UserRef names a user.
type UserRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// UnitRef names a unit by its barcode.
type UnitRef struct {
	UUID    uuid.UUID `json:"uuid"`
	Barcode string    `json:"barcode"`
}

// Reclassification is a request to move a unit to another product while
// its barcode stays the same (TEC-157).
type Reclassification struct {
	UUID         uuid.UUID  `json:"uuid"`
	Status       string     `json:"status"`
	Unit         UnitRef    `json:"unit"`
	Organization OrgRef     `json:"organization"`
	FromProduct  ProductRef `json:"from_product"`
	ToProduct    ProductRef `json:"to_product"`
	Reason       string     `json:"reason"`
	RequestedBy  *UserRef   `json:"requested_by"`
	DecidedBy    *UserRef   `json:"decided_by"`
	DecidedAt    *time.Time `json:"decided_at"`
	DecisionNote *string    `json:"decision_note"`
	MovementUUID *uuid.UUID `json:"movement_uuid"`
	CreatedAt    time.Time  `json:"created_at"`
}

// SplitUnit is one side of a roll split.
type SplitUnit struct {
	UUID            uuid.UUID `json:"uuid"`
	Barcode         string    `json:"barcode"`
	Status          string    `json:"status"`
	RemainingMeters string    `json:"remaining_meters"`
	// LabelURL is the label print endpoint of the unit (TEC-202; set for
	// the new unit of a split).
	LabelURL string `json:"label_url,omitempty"`
}

// Split is a roll split (TEC-184): meters cut off a roll as a new unit with
// its own barcode. Replayed is true when the idempotency key was already
// used (nothing new was written).
type Split struct {
	UUID      uuid.UUID `json:"uuid"`
	Meters    string    `json:"meters"`
	Source    SplitUnit `json:"source"`
	NewUnit   SplitUnit `json:"new_unit"`
	Replayed  bool      `json:"replayed"`
	CreatedAt time.Time `json:"created_at"`
}
