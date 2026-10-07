package tools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	stockmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// StockReader is the stock use case surface the tools use.
type StockReader interface {
	OrganizationStock(ctx context.Context, f scopefilter.Filter, orgUUID uuid.UUID, in stockmodel.StockFilter) ([]stockmodel.ProductStock, int64, error)
	OrganizationUnits(ctx context.Context, v stockuc.UnitViewer, orgUUID uuid.UUID, in stockmodel.UnitFilter) ([]stockmodel.StockUnitRow, int64, error)
}

type stockBase struct {
	stock StockReader
	tree  scopefilter.TreeReader
}

func (b stockBase) errs(tool string) errCases {
	return errCases{tool: tool, what: "organization or product", notFound: []error{stockuc.ErrNotFound},
		invalid: asError[*stockuc.ValidationError]}
}

// target returns the organization whose stock is read: the active one, or
// an organization uuid in the caller's stock.read scope (a distributor
// reading a dealer).
func target(p Principal, raw string) (uuid.UUID, bool) {
	if strings.TrimSpace(raw) == "" {
		return p.Org.UUID, true
	}
	return parseID(raw)
}

var organizationProp = str("Optional organization uuid within your access (e.g. one of your dealers); default: your own organization.", 36)

// StockSummary: stok özeti (ürün bazlı eldeki miktar).
type StockSummary struct{ stockBase }

// Spec implements Tool.
func (StockSummary) Spec() Spec {
	return Spec{
		Name: "stock_summary",
		Description: "On-hand stock per product of your organization (or of an organization within your access): " +
			"quantity for pieces, meters for rolls. Filter by product name / SKU.",
		InputSchema: object(map[string]any{
			"query":        str("Optional product name or SKU.", 100),
			"organization": organizationProp,
			"only_in_stock": map[string]any{"type": "boolean",
				"description": "Only products with stock on hand (default true)."},
			"limit": limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleStock,
		Permissions: []string{rbac.PermStockRead},
	}
}

type stockRow struct {
	Product  string `json:"product"`
	SKU      string `json:"sku"`
	UUID     string `json:"product_uuid"`
	UnitType string `json:"unit_type"`
	Quantity int32  `json:"quantity"`
	Meters   string `json:"meters,omitempty"`
}

// Run implements Tool.
func (t StockSummary) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Query        string `json:"query"`
		Organization string `json:"organization"`
		OnlyInStock  *bool  `json:"only_in_stock"`
		Limit        int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	org, ok := target(env.Principal, in.Organization)
	if !ok {
		return invalidArg("organization must be a uuid"), nil
	}
	f, err := resolveScope(ctx, t.tree, env.Principal, rbac.PermStockRead)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	sf := stockmodel.StockFilter{Q: strings.TrimSpace(in.Query), Limit: limitArg(in.Limit), Status: stockmodel.StatusInStock}
	if in.OnlyInStock != nil && !*in.OnlyInStock {
		sf.Status = ""
	}
	rows, total, err := t.stock.OrganizationStock(ctx, f, org, sf)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	out := make([]stockRow, 0, len(rows))
	for _, r := range rows {
		row := stockRow{Product: dataText(r.Product.Name, maxNameChars), SKU: r.Product.SKU, UUID: r.Product.UUID.String(),
			UnitType: r.Product.UnitType, Quantity: r.Quantity}
		if r.Product.UnitType == "roll_meter" {
			row.Meters = r.Meters
		}
		out = append(out, row)
	}
	return JSONResult(NewList(out, total))
}

// StockUnits: ürün bazlı eldeki birimler (barkod, durum, konum; alış fiyatı
// yalnız pricing.purchase.read ile).
type StockUnits struct{ stockBase }

// Spec implements Tool.
func (StockUnits) Spec() Spec {
	return Spec{
		Name: "stock_units",
		Description: "Physical units (barcodes, rolls) on hand in your organization (or one within your access), " +
			"optionally of one product: status, location, remaining meters. The purchase price is included only " +
			"when the user may read purchase prices.",
		InputSchema: object(map[string]any{
			"product_uuid": str("Optional product uuid (see stock_summary or search_products).", 36),
			"query":        str("Optional barcode, product name or SKU.", 100),
			"organization": organizationProp,
			"limit":        limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleStock,
		Permissions: []string{rbac.PermStockRead},
	}
}

type unitPrice struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type unitRow struct {
	Barcode         string     `json:"barcode"`
	Product         string     `json:"product"`
	SKU             string     `json:"sku"`
	Status          string     `json:"status"`
	Quantity        int32      `json:"quantity"`
	RemainingMeters *string    `json:"remaining_meters,omitempty"`
	Location        string     `json:"location,omitempty"`
	PurchasePrice   *unitPrice `json:"purchase_price,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Run implements Tool.
func (t StockUnits) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		ProductUUID  string `json:"product_uuid"`
		Query        string `json:"query"`
		Organization string `json:"organization"`
		Limit        int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	p := env.Principal
	org, ok := target(p, in.Organization)
	if !ok {
		return invalidArg("organization must be a uuid"), nil
	}
	f, err := resolveScope(ctx, t.tree, p, rbac.PermStockRead)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	uf := stockmodel.UnitFilter{Q: strings.TrimSpace(in.Query), Limit: limitArg(in.Limit)}
	if in.ProductUUID != "" {
		id, ok := parseID(in.ProductUUID)
		if !ok {
			return invalidArg("product_uuid must be a uuid"), nil
		}
		uf.ProductUUID = &id
	}
	rows, total, err := t.stock.OrganizationUnits(ctx, stockuc.UnitViewer{Principal: p.Auth, Org: *p.Org, Filter: f}, org, uf)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	// The use case already hides the price without the grant; the tool
	// re-applies the rule: the own organization's purchase price needs
	// pricing.purchase.read, a sub-organization's also the parent's
	// pricing.sale.read (it is the price the parent sells at).
	showPrice := p.Auth.HasPermission(rbac.PermPricingPurchaseRead) ||
		(org != p.Org.UUID && p.Auth.Can(rbac.PermPricingSaleRead, rbac.ScopeManaged))
	out := make([]unitRow, 0, len(rows))
	for _, r := range rows {
		row := unitRow{
			Barcode: r.Barcode, Product: dataText(r.Product.Name, maxNameChars), SKU: r.Product.SKU,
			Status: r.Status, Quantity: r.Quantity, RemainingMeters: r.RemainingMeters, UpdatedAt: r.UpdatedAt,
		}
		if r.Location != nil {
			row.Location = dataText(r.Location.Code+" "+r.Location.Name, maxNameChars)
		}
		if showPrice && r.PurchasePrice != nil {
			row.PurchasePrice = &unitPrice{Amount: r.PurchasePrice.Amount, Currency: r.PurchasePrice.Currency}
		}
		out = append(out, row)
	}
	return JSONResult(NewList(out, total))
}
