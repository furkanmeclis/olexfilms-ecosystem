// Package model contains the contract template API shapes.
package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	KindVehicleIntake = "vehicle_intake"
	KindServiceSale   = "service_sale"
)

// Variable is one allowed {{key}} in a contract template.
type Variable struct {
	Key     string `json:"key"`
	Group   string `json:"group"`
	LabelTR string `json:"label_tr"`
	LabelEN string `json:"label_en"`
}

// Template is the platform view of a contract template.
type Template struct {
	UUID              uuid.UUID        `json:"uuid"`
	Name              string           `json:"name"`
	Kind              string           `json:"kind"`
	IsDefault         bool             `json:"is_default"`
	OTPRequired       bool             `json:"otp_required"`
	SignatureRequired bool             `json:"signature_required"`
	IsActive          bool             `json:"is_active"`
	Locales           []TemplateLocale `json:"locales,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

// TemplateLocale is one language version of a template.
type TemplateLocale struct {
	UUID        uuid.UUID       `json:"uuid"`
	Locale      string          `json:"locale"`
	LexicalJSON json.RawMessage `json:"lexical_json,omitempty"`
	HTML        string          `json:"html"`
	Version     int32           `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// RenderInput carries values for server-side contract text rendering.
type RenderInput struct {
	TemplateUUID uuid.UUID
	Locale       string
	Values       map[string]string
}

// Rendered is the selected language version after fallback and substitution.
type Rendered struct {
	HTML    string `json:"html"`
	Locale  string `json:"locale"`
	Version int32  `json:"version"`
}
