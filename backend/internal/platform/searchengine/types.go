package searchengine

import "context"

// Spec describes one searchable resource catalog entry.
type Spec struct {
	ID           string   `json:"id"`
	LabelKey     string   `json:"label_key"`
	Permission   string   `json:"permission,omitempty"`
	Icon         string   `json:"icon,omitempty"`
	Searchable   []string `json:"searchable_fields,omitempty"`
	Filterable   []string `json:"filterable_fields,omitempty"`
	TenantScoped bool     `json:"tenant_scoped,omitempty"`
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
