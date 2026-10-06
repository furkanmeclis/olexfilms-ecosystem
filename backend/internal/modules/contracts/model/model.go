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
	ID                int64            `json:"id"`
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

const (
	SignerCustomer = "customer"
	SignerStaff    = "staff"

	StatusDraft    = "draft"
	StatusPending  = "pending"
	StatusExecuted = "executed"
	StatusVoided   = "voided"
)

// Contract is an executed or in-flight contract instance with evidence.
type Contract struct {
	UUID              uuid.UUID  `json:"uuid"`
	ContractNo        int64      `json:"contract_no"`
	SubjectType       string     `json:"subject_type"`
	SubjectID         int64      `json:"subject_id"`
	Kind              string     `json:"kind"`
	Locale            string     `json:"locale"`
	TemplateVersion   int32      `json:"template_version"`
	OTPRequired       bool       `json:"otp_required"`
	SignatureRequired bool       `json:"signature_required"`
	Status            string     `json:"status"`
	RenderedHTML      string     `json:"rendered_html,omitempty"`
	ContentSHA256     string     `json:"content_sha256,omitempty"`
	ExecutedAt        *time.Time `json:"executed_at,omitempty"`
	VoidedAt          *time.Time `json:"voided_at,omitempty"`
	VoidReason        string     `json:"void_reason,omitempty"`
	// PDFReady is true once the executed PDF is stored (GET .../pdf answers 200).
	PDFReady  bool      `json:"pdf_ready"`
	Signers   []Signer  `json:"signers"`
	Media     []Media   `json:"media"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Signer is one fixed signing slot.
type Signer struct {
	UUID          uuid.UUID  `json:"uuid"`
	Role          string     `json:"role"`
	UserID        *int64     `json:"user_id,omitempty"`
	Name          string     `json:"name"`
	PhoneE164     string     `json:"phone_e164,omitempty"`
	OTPVerifiedAt *time.Time `json:"otp_verified_at,omitempty"`
	SignedAt      *time.Time `json:"signed_at,omitempty"`
	Signature     *Signature `json:"signature,omitempty"`
}

// Signature is the latest canvas signature evidence of a signer.
type Signature struct {
	UUID       uuid.UUID `json:"uuid"`
	StorageKey string    `json:"storage_key"`
	SHA256     string    `json:"sha256"`
	IPAddress  string    `json:"ip_address,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Media is one image attached to a contract.
type Media struct {
	UUID       uuid.UUID `json:"uuid"`
	StorageKey string    `json:"storage_key"`
	MIMEType   string    `json:"mime_type"`
	SizeBytes  int64     `json:"size_bytes"`
	SHA256     string    `json:"sha256"`
	Title      string    `json:"title,omitempty"`
	SortOrder  int32     `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
}
