package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	catalogmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// CatalogReader is the catalog use case surface the tools use.
type CatalogReader interface {
	ListProducts(ctx context.Context, org orgctx.Scope, f catalogmodel.ProductFilter) ([]catalogmodel.Product, int64, error)
}

// SearchProducts: katalog ürün arama (yalnız alan adının markası, K20).
type SearchProducts struct{ catalog CatalogReader }

// Spec implements Tool.
func (SearchProducts) Spec() Spec {
	return Spec{
		Name: "search_products",
		Description: "Search the product catalog of your brand (films, coatings) by name or SKU: category, unit type, " +
			"warranty months, thickness. Active products only unless include_inactive is true. No prices.",
		InputSchema: object(map[string]any{
			"query":            str("Optional product name or SKU.", 100),
			"include_inactive": map[string]any{"type": "boolean", "description": "Also list inactive products."},
			"limit":            limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleCatalog,
		Permissions: []string{rbac.PermCatalogRead},
	}
}

type productRow struct {
	UUID           uuid.UUID `json:"uuid"`
	SKU            string    `json:"sku"`
	Name           string    `json:"name"`
	Category       string    `json:"category"`
	UnitType       string    `json:"unit_type"`
	WarrantyMonths *int32    `json:"warranty_months,omitempty"`
	MicronThick    *float64  `json:"micron_thickness,omitempty"`
	Active         bool      `json:"active"`
	Description    string    `json:"description,omitempty"`
}

// Run implements Tool.
func (t SearchProducts) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Query           string `json:"query"`
		IncludeInactive bool   `json:"include_inactive"`
		Limit           int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := errCases{tool: t.Spec().Name, what: "product", notFound: []error{catalogusecase.ErrNotFound},
		invalid: asError[*catalogusecase.ValidationError]}
	f := catalogmodel.ProductFilter{Q: strings.TrimSpace(in.Query), Limit: limitArg(in.Limit)}
	if !in.IncludeInactive {
		active := true
		f.Active = &active
	}
	// The catalog is per brand: the use case reads the active
	// organization's brand, which is the request's domain brand (K20).
	rows, total, err := t.catalog.ListProducts(ctx, *env.Principal.Org, f)
	if err != nil {
		return errs.result(err)
	}
	out := make([]productRow, 0, len(rows))
	for _, p := range rows {
		out = append(out, productRow{
			UUID: p.UUID, SKU: p.SKU, Name: dataText(p.Name, maxNameChars), Category: dataText(p.Category.Name, maxNameChars),
			UnitType: p.UnitType, WarrantyMonths: p.WarrantyDurationMonths, MicronThick: p.MicronThickness,
			Active: p.Active, Description: dataText(p.DescriptionMD, maxTextChars),
		})
	}
	return JSONResult(NewList(out, total))
}

// OrganizationsReader is the organizations use case surface the tools use.
type OrganizationsReader interface {
	ListInScope(ctx context.Context, f scopefilter.Filter, in orgusecase.ScopedListInput) ([]orgusecase.Organization, error)
}

// ListSubOrganizations: alt bayiler listesi (distribütör / merkez).
type ListSubOrganizations struct {
	orgs OrganizationsReader
	tree scopefilter.TreeReader
}

// Spec implements Tool.
func (ListSubOrganizations) Spec() Spec {
	return Spec{
		Name: "list_sub_organizations",
		Description: "List the dealers (or, for the center, distributors) below your organization: name, dealer " +
			"code, city, phone, status and parent. Filter by name, dealer code or city.",
		InputSchema: object(map[string]any{
			"query": str("Optional name, dealer code or city.", 100),
			"type":  enum("dealer (default) or distributor.", orgusecase.TypeDealer, orgusecase.TypeDistributor),
			"limit": limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleOrganizations,
		OrgTypes:    []string{OrgCenter, OrgDistributor},
		Permissions: []string{rbac.PermOrganizationsRead},
	}
}

type orgRow struct {
	UUID     uuid.UUID `json:"uuid"`
	Code     string    `json:"dealer_code"`
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	City     string    `json:"city,omitempty"`
	District string    `json:"district,omitempty"`
	Phone    string    `json:"phone,omitempty"`
	Status   string    `json:"status"`
	Parent   string    `json:"parent,omitempty"`
}

// Run implements Tool.
func (t ListSubOrganizations) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Query string `json:"query"`
		Type  string `json:"type"`
		Limit int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := errCases{tool: t.Spec().Name, what: "organization", notFound: []error{orgusecase.ErrNotFound}}
	p := env.Principal
	f, err := resolveScope(ctx, t.tree, p, rbac.PermOrganizationsRead)
	if err != nil {
		return errs.result(err)
	}
	if in.Type == "" {
		in.Type = orgusecase.TypeDealer
	}
	limit := limitArg(in.Limit)
	// One extra row tells whether the list was cut.
	rows, err := t.orgs.ListInScope(ctx, f, orgusecase.ScopedListInput{Type: in.Type, Q: strings.TrimSpace(in.Query), Limit: limit + 1})
	if errors.Is(err, orgusecase.ErrInvalidRequest) {
		return invalidArg(err.Error()), nil
	}
	if err != nil {
		return errs.result(err)
	}
	out := make([]orgRow, 0, len(rows))
	for _, o := range rows {
		if o.UUID == p.Org.UUID {
			continue
		}
		r := orgRow{UUID: o.UUID, Code: o.Slug, Name: dataText(o.Name, maxNameChars), Type: o.Type,
			City: dataText(o.City, maxNameChars), District: dataText(o.District, maxNameChars), Phone: o.Phone, Status: o.Status}
		if o.Parent != nil {
			r.Parent = dataText(o.Parent.Name, maxNameChars)
		}
		out = append(out, r)
	}
	total := int64(len(out))
	if len(out) > int(limit) {
		out = out[:limit]
	}
	return JSONResult(NewList(out, total))
}
