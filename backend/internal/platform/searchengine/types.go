package searchengine

import (
	"context"
	"errors"
)

// ErrSkipDocument is returned by Adapter.Document when the entity must not
// be in the index (e.g. an anonymized customer); the indexer then removes
// any stale document instead of upserting.
var ErrSkipDocument = errors.New("searchengine: document is not indexable")

// Spec describes one searchable resource catalog entry.
type Spec struct {
	ID           string   `json:"id"`
	LabelKey     string   `json:"label_key"`
	Permission   string   `json:"permission,omitempty"`
	Icon         string   `json:"icon,omitempty"`
	Searchable   []string `json:"searchable_fields,omitempty"`
	Filterable   []string `json:"filterable_fields,omitempty"`
	TenantScoped bool     `json:"tenant_scoped,omitempty"`
	// BrandScoped documents carry brand_id; search filters on the brand of
	// the active organization (K1/K20) and hides the spec without one.
	BrandScoped bool `json:"brand_scoped,omitempty"`
	// ListScoped specs are searched only by their module's list endpoint,
	// which applies the permission scope (e.g. customers through
	// customer_organizations, TEC-164). The command palette never lists or
	// queries them.
	ListScoped bool `json:"list_scoped,omitempty"`
}

// Document is the Meilisearch index payload for one entity.
type Document struct {
	ID               string   `json:"id"`
	Spec             string   `json:"spec"`
	Title            string   `json:"title"`
	Subtitle         string   `json:"subtitle,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
	Href             string   `json:"href"`
	Icon             string   `json:"icon,omitempty"`
	OrganizationSlug string   `json:"organization_slug,omitempty"`
	// BrandID is set on brand scoped specs (filterable).
	BrandID int64 `json:"brand_id,omitempty"`
	// OrganizationIDs / BrandIDs are the organizations and brands a record
	// is linked to (list scoped specs, filterable with IN / =).
	OrganizationIDs []int64 `json:"organization_ids,omitempty"`
	BrandIDs        []int64 `json:"brand_ids,omitempty"`
	// Status is the filterable record status (list scoped specs).
	Status string `json:"status,omitempty"`
}

// Hit is a normalized search result returned to clients.
type Hit struct {
	Spec     string  `json:"spec"`
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle,omitempty"`
	Href     string  `json:"href"`
	Icon     string  `json:"icon,omitempty"`
	Score    float64 `json:"score,omitempty"`
}

// Adapter connects searchengine to a list resource.
type Adapter interface {
	Spec() Spec
	ListAll(ctx context.Context) ([]Document, error)
	Document(ctx context.Context, id string) (Document, error)
}
