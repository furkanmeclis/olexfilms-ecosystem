package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	OAuthProviderGitHub   = "github"
	OAuthProviderGoogle   = "google"
	OAuthProviderFacebook = "facebook"
	OAuthProviderApple    = "apple"
)

// IsKnownOAuthProvider reports whether provider is a supported OAuth slug.
func IsKnownOAuthProvider(provider string) bool {
	switch provider {
	case OAuthProviderGitHub, OAuthProviderGoogle, OAuthProviderFacebook, OAuthProviderApple:
		return true
	default:
		return false
	}
}

// OAuthAccountRecord is the persistence shape for linked OAuth providers.
type OAuthAccountRecord struct {
	ID                int64
	UUID              uuid.UUID
	UserID            int64
	UserUUID          uuid.UUID
	Provider          string
	ProviderAccountID string
	Type              string
	GitHubLogin       *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// LinkedIdentity is a user-facing linked provider summary.
type LinkedIdentity struct {
	Provider    string    `json:"provider"`
	GitHubLogin *string   `json:"github_login,omitempty"`
	LinkedAt    time.Time `json:"linked_at"`
}

// IdentityList is GET /v1/auth/identities payload.
type IdentityList struct {
	Items []LinkedIdentity `json:"items"`
	Total int64            `json:"total"`
}

// LinkOAuthAccountInput is the adapter link payload.
type LinkOAuthAccountInput struct {
	UserID            string     `json:"userId"`
	Provider          string     `json:"provider"`
	ProviderAccountID string     `json:"providerAccountId"`
	Type              string     `json:"type"`
	AccessToken       *string    `json:"access_token,omitempty"`
	RefreshToken      *string    `json:"refresh_token,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	TokenType         *string    `json:"token_type,omitempty"`
	Scope             *string    `json:"scope,omitempty"`
	GitHubLogin       *string    `json:"github_login,omitempty"`
}

// CreateOAuthAccountInput is the repository create payload.
type CreateOAuthAccountInput struct {
	UserID            int64
	Provider          string
	ProviderAccountID string
	Type              string
	AccessTokenEnc    *string
	RefreshTokenEnc   *string
	ExpiresAt         *time.Time
	TokenType         *string
	Scope             *string
	GitHubLogin       *string
}
