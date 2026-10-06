// Package model holds the API views and inputs of the TEC-149 vehicle
// catalog (car brands and models).
package model

import (
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
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

// BrandSort is the sort contract of GET /v1/vehicle-catalog/brands
// (TEC-369, docs/list-contract.md). Default: name.
var BrandSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"name": "name", "model_count": "model_count", "active": "active",
		"created_at": "created_at", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "name"},
}

// ModelSort is the sort contract of GET /v1/vehicle-catalog/models
// (TEC-369). Default: brand name, then model name.
var ModelSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"brand": "brand", "name": "name", "year_start": "year_start", "year_stop": "year_stop",
		"body_type": "body_type", "powertrain": "powertrain", "active": "active",
		"created_at": "created_at", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "brand"},
}

// BrandFilter filters the brand list.
type BrandFilter struct {
	Q       string
	Active  *bool
	HasLogo *bool
	Sort    apiquery.ResolvedSort
	Limit   int32
	Offset  int32
}

// ModelFilter filters the model list / search.
type ModelFilter struct {
	Q         string
	BrandUUID *uuid.UUID
	Active    *bool
	// OnlyActiveBrands hides models of inactive brands (pickers).
	OnlyActiveBrands bool
	// BodyTypes / Powertrains: any of (exact values, CSV).
	BodyTypes   []string
	Powertrains []string
	// Year: the production span overlaps [Min, Max] (year_min / year_max).
	Year   apiquery.NumberRange
	Sort   apiquery.ResolvedSort
	Limit  int32
	Offset int32
}

// FacetValue is one distinct value of a free-text model column.
type FacetValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// ModelFacets are the faceted filter options of the model list (TEC-369).
type ModelFacets struct {
	BodyType   []FacetValue `json:"body_type"`
	Powertrain []FacetValue `json:"powertrain"`
}
