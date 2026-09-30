package model

import (
	"time"

	"github.com/google/uuid"
)

// PasskeyRecord is the persistence shape for webauthn credentials.
type PasskeyRecord struct {
	ID                int64
	UUID              uuid.UUID
	UserID            int64
	CredentialID      string
	PublicKey         string
	Counter           int64
	DeviceType        string
	BackedUp          bool
	Transports        *string
	ProviderAccountID string
	Name              *string
	LastUsedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Passkey is a user-facing passkey summary.
type Passkey struct {
	UUID       uuid.UUID  `json:"uuid"`
	Name       *string    `json:"name,omitempty"`
	DeviceType string     `json:"device_type"`
	BackedUp   bool       `json:"backed_up"`
	Transports *string    `json:"transports,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// PasskeyList is a list payload for GET /v1/auth/passkeys.
type PasskeyList struct {
	Items []Passkey `json:"items"`
	Total int64     `json:"total"`
}

// AdapterUser is the NextAuth adapter user shape.
type AdapterUser struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	EmailVerified *time.Time `json:"emailVerified,omitempty"`
	Name          *string    `json:"name,omitempty"`
}

// AdapterAuthenticator is the NextAuth adapter authenticator shape.
type AdapterAuthenticator struct {
	CredentialID         string  `json:"credentialID"`
	ProviderAccountID    string  `json:"providerAccountId"`
	UserID               string  `json:"userId"`
	CredentialPublicKey  string  `json:"credentialPublicKey"`
	Counter              int64   `json:"counter"`
	CredentialDeviceType string  `json:"credentialDeviceType"`
	CredentialBackedUp   bool    `json:"credentialBackedUp"`
	Transports           *string `json:"transports,omitempty"`
}

// CreateAdapterAuthenticatorInput is the internal adapter create payload.
type CreateAdapterAuthenticatorInput struct {
	UserID               string  `json:"userId"`
	CredentialID         string  `json:"credentialID"`
	ProviderAccountID    string  `json:"providerAccountId"`
	CredentialPublicKey  string  `json:"credentialPublicKey"`
	Counter              int64   `json:"counter"`
	CredentialDeviceType string  `json:"credentialDeviceType"`
	CredentialBackedUp   bool    `json:"credentialBackedUp"`
	Transports           *string `json:"transports,omitempty"`
}
