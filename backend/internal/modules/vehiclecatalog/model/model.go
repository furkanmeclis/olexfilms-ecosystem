// Package model holds the API views and inputs of the TEC-149 vehicle
// catalog (car brands and models).
package model

import (
	"time"

	"github.com/google/uuid"
)

// Brand is a car brand.
type Brand struct {
	UUID       uuid.UUID `json:"uuid"`
	ExternalID *string   `json:"external_id"`
	Name       string    `json:"name"`
	ShowName   bool      `json:"show_name"`
	LogoHeight *int16    `json:"logo_height"`
	Active     bool      `json:"active"`
	HasLogo    bool      `json:"has_logo"`
	// LogoURL is the stable public logo URL (/brand-logos/{uuid}); it
	// serves a placeholder while the brand has no logo. ?v= changes with
	// the logo so caches pick up a new upload at once.
	LogoURL string `json:"logo_url"`
	HasHero bool   `json:"has_hero"`
	// HeroURL resolves brand hero → default image.
	HeroURL    string    `json:"hero_url"`
	ModelCount int64     `json:"model_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// BrandRef is the brand summary embedded in a model.
type BrandRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Model is a car model of a brand.
type Model struct {
	UUID       uuid.UUID `json:"uuid"`
	Brand      BrandRef  `json:"brand"`
	ExternalID *string   `json:"external_id"`
	Name       string    `json:"name"`
	BodyType   *string   `json:"body_type"`
	Powertrain *string   `json:"powertrain"`
	YearStart  *int16    `json:"year_start"`
	YearStop   *int16    `json:"year_stop"`
	Active     bool      `json:"active"`
	HasHero    bool      `json:"has_hero"`
	// HeroURL resolves model hero → brand hero → default image.
	HeroURL   string    `json:"hero_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BrandInput creates (POST) or patches (PATCH) a brand. Nullable fields
// named in a PATCH body with null are cleared (Present).
type BrandInput struct {
	ExternalID *string `json:"external_id"`
	Name       *string `json:"name"`
	ShowName   *bool   `json:"show_name"`
	LogoHeight *int16  `json:"logo_height"`
	Active     *bool   `json:"active"`

	Present map[string]bool `json:"-"`
}

// ModelInput creates or patches a model.
type ModelInput struct {
	BrandUUID  *uuid.UUID `json:"brand_uuid"`
	ExternalID *string    `json:"external_id"`
	Name       *string    `json:"name"`
	BodyType   *string    `json:"body_type"`
	Powertrain *string    `json:"powertrain"`
	YearStart  *int16     `json:"year_start"`
	YearStop   *int16     `json:"year_stop"`
	Active     *bool      `json:"active"`

	Present map[string]bool `json:"-"`
}

// BrandFilter filters the brand list.
type BrandFilter struct {
	Q      string
	Active *bool
	Limit  int32
	Offset int32
}

// ModelFilter filters the model list / search.
type ModelFilter struct {
	Q         string
	BrandUUID *uuid.UUID
	Active    *bool
	// OnlyActiveBrands hides models of inactive brands (pickers).
	OnlyActiveBrands bool
	Limit            int32
	Offset           int32
}
