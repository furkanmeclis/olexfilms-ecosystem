package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service; the handler maps them to HTTP codes.
var (
	ErrForbidden           = errors.New("pricing: not allowed for this organization")
	ErrProductNotFound     = errors.New("pricing: product not found")
	ErrPriceNotFound       = errors.New("pricing: price not found")
	ErrDistributorNotFound = errors.New("pricing: distributor not found")
)

// ValidationError is a field-level input error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// Querier is the subset of db.Queries the service uses.
type Querier interface {
	GetProductByUUID(ctx context.Context, arg db.GetProductByUUIDParams) (db.Product, error)
	GetOrganizationByUUID(ctx context.Context, uuid uuid.UUID) (db.Organization, error)
	SupplierOf(ctx context.Context, id int64) (db.Organization, error)

	ListPricedProducts(ctx context.Context, arg db.ListPricedProductsParams) ([]db.ListPricedProductsRow, error)
	ListProductsByUUIDs(ctx context.Context, arg db.ListProductsByUUIDsParams) ([]db.ListProductsByUUIDsRow, error)
	CountPricedProducts(ctx context.Context, arg db.CountPricedProductsParams) (int64, error)
	ListProductPricesForProducts(ctx context.Context, arg db.ListProductPricesForProductsParams) ([]db.ListProductPricesForProductsRow, error)
	ListDistributorOverridesForProducts(ctx context.Context, arg db.ListDistributorOverridesForProductsParams) ([]db.ListDistributorOverridesForProductsRow, error)
	ListDealerPricesForProducts(ctx context.Context, arg db.ListDealerPricesForProductsParams) ([]db.ListDealerPricesForProductsRow, error)

	GetProductPrice(ctx context.Context, arg db.GetProductPriceParams) (db.ProductPrice, error)
	UpsertProductPrice(ctx context.Context, arg db.UpsertProductPriceParams) (db.ProductPrice, error)
	DeleteProductPrice(ctx context.Context, arg db.DeleteProductPriceParams) (int64, error)

	UpsertDistributorPriceOverride(ctx context.Context, arg db.UpsertDistributorPriceOverrideParams) (db.UpsertDistributorPriceOverrideRow, error)
	DeleteDistributorPriceOverride(ctx context.Context, arg db.DeleteDistributorPriceOverrideParams) (int64, error)
	ListDistributorOverrideDetails(ctx context.Context, arg db.ListDistributorOverrideDetailsParams) ([]db.ListDistributorOverrideDetailsRow, error)
	CountDistributorPriceOverrides(ctx context.Context, arg db.CountDistributorPriceOverridesParams) (int64, error)
	CountDistributorOverrideDetails(ctx context.Context, arg db.CountDistributorOverrideDetailsParams) (int64, error)

	UpsertDistributorDealerPrice(ctx context.Context, arg db.UpsertDistributorDealerPriceParams) (db.UpsertDistributorDealerPriceRow, error)
	DeleteDistributorDealerPrice(ctx context.Context, arg db.DeleteDistributorDealerPriceParams) (int64, error)
}

// Service implements the price list rules.
type Service struct {
	q Querier
}

// New creates the service.
func New(q Querier) *Service { return &Service{q: q} }

var (
	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
	// NUMERIC(14,4): at most 10 integer and 4 fractional digits, never negative.
	priceRe = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,4})?$`)
)

// NormalizeCurrency validates an ISO-4217 code (upper-cased).
func NormalizeCurrency(raw string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(raw))
	if !currencyRe.MatchString(c) {
		return "", invalid("currency", "must be a three-letter ISO-4217 code")
	}
	return c, nil
}

// NormalizePrice validates a non-negative decimal string with at most four
// fractional digits.
func NormalizePrice(field, raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if !priceRe.MatchString(p) {
		return "", invalid(field, "must be a non-negative decimal with at most 10 integer and 4 fractional digits")
	}
	return p, nil
}

// --- Effective price view -------------------------------------------------

// Price sources of the effective purchase price.
const (
	SourceList        = "list"        // center list price (sale to distributors)
	SourceOverride    = "override"    // distributor-specific price set by the center
	SourceDistributor = "distributor" // the parent distributor's dealer price
	SourceOwn         = "own"         // the center's own purchase price
)

// EffectivePrice is one currency of a product as the viewer may see it.
// Fields the viewer may not see are nil and never serialised.
type EffectivePrice struct {
	Currency string `json:"currency"`
	// PurchasePrice is what the viewer's organization pays.
	PurchasePrice       *string `json:"purchase_price,omitempty"`
	PurchasePriceSource string  `json:"purchase_price_source,omitempty"`
	// SalePrice is what the viewer's organization sells to the level below
	// (center: to distributors; distributor: to its dealers).
	SalePrice            *string `json:"sale_price,omitempty"`
	RecommendedSalePrice *string `json:"recommended_sale_price,omitempty"`
}

func (e EffectivePrice) empty() bool {
	return e.PurchasePrice == nil && e.SalePrice == nil && e.RecommendedSalePrice == nil
}

// ProductPriceView is the effective price view of one product.
type ProductPriceView struct {
	ProductUUID uuid.UUID        `json:"product_uuid"`
	SKU         string           `json:"sku"`
	Name        string           `json:"name"`
	Viewer      string           `json:"viewer"`
	Prices      []EffectivePrice `json:"prices"`
}

type productRef struct {
	ID   int64
	UUID uuid.UUID
	SKU  string
	Name string
}

func numericText(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	v, err := n.Value()
	if err != nil || v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return nil
	}
	return &s
}

func strPtr(s string) *string { return &s }

// currencyMap keeps per-product, per-currency prices in insertion order.
type currencyMap struct {
	order []string
	items map[string]*EffectivePrice
}

func (m *currencyMap) get(cur string) *EffectivePrice {
	if m.items == nil {
		m.items = map[string]*EffectivePrice{}
	}
	if e, ok := m.items[cur]; ok {
		return e
	}
	e := &EffectivePrice{Currency: cur}
	m.items[cur] = e
	m.order = append(m.order, cur)
	return e
}

func (m *currencyMap) list() []EffectivePrice {
	out := []EffectivePrice{}
	for _, c := range m.order {
		if e := m.items[c]; !e.empty() {
			out = append(out, *e)
		}
	}
	return out
}

func checkViewerType(v Viewer) error {
	switch v.OrgType {
	case OrgCenter, OrgDistributor, OrgDealer:
		return nil
	default:
		return ErrForbidden
	}
}

func (s *Service) views(ctx context.Context, v Viewer, products []productRef) ([]ProductPriceView, error) {
	if err := checkViewerType(v); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(products))
	for _, p := range products {
		ids = append(ids, p.ID)
	}
	byProduct := map[int64]*currencyMap{}
	at := func(id int64, cur string) *EffectivePrice {
		m, ok := byProduct[id]
		if !ok {
			m = &currencyMap{}
			byProduct[id] = m
		}
		return m.get(strings.TrimSpace(cur))
	}
	if len(ids) > 0 {
		var err error
		switch v.OrgType {
		case OrgCenter:
			err = s.centerPrices(ctx, v, ids, at)
		case OrgDistributor:
			err = s.distributorPrices(ctx, v, ids, at)
		case OrgDealer:
			err = s.dealerPrices(ctx, v, ids, at)
		}
		if err != nil {
			return nil, err
		}
	}
	out := make([]ProductPriceView, 0, len(products))
	for _, p := range products {
		view := ProductPriceView{ProductUUID: p.UUID, SKU: p.SKU, Name: p.Name, Viewer: v.OrgType, Prices: []EffectivePrice{}}
		if m, ok := byProduct[p.ID]; ok {
			view.Prices = m.list()
		}
		out = append(out, view)
	}
	return out, nil
}

// Center: its own purchase price, its sale price to distributors and the
// recommended retail price, each behind its permission.
func (s *Service) centerPrices(ctx context.Context, v Viewer, ids []int64, at func(int64, string) *EffectivePrice) error {
	if !v.PurchaseRead && !v.SaleRead && !v.RecommendedRead {
		return nil
	}
	rows, err := s.q.ListProductPricesForProducts(ctx, db.ListProductPricesForProductsParams{BrandID: v.BrandID, ProductIds: ids})
	if err != nil {
		return fmt.Errorf("list prices: %w", err)
	}
	for _, r := range rows {
		e := at(r.ProductID, r.Currency)
		if v.PurchaseRead {
			if e.PurchasePrice = numericText(r.PurchasePrice); e.PurchasePrice != nil {
				e.PurchasePriceSource = SourceOwn
			}
		}
		if v.SaleRead {
			e.SalePrice = numericText(r.SaleToDistributorPrice)
		}
		if v.RecommendedRead {
			e.RecommendedSalePrice = numericText(r.RecommendedSalePrice)
		}
	}
	return nil
}

// Distributor: purchase = its override, else the center's sale price to
// distributors; sale = its own dealer price. The center's own purchase price
// and the recommended price are never part of this view.
func (s *Service) distributorPrices(ctx context.Context, v Viewer, ids []int64, at func(int64, string) *EffectivePrice) error {
	if v.PurchaseRead {
		rows, err := s.q.ListProductPricesForProducts(ctx, db.ListProductPricesForProductsParams{BrandID: v.BrandID, ProductIds: ids})
		if err != nil {
			return fmt.Errorf("list prices: %w", err)
		}
		for _, r := range rows {
			if p := numericText(r.SaleToDistributorPrice); p != nil {
				e := at(r.ProductID, r.Currency)
				e.PurchasePrice, e.PurchasePriceSource = p, SourceList
			}
		}
		overrides, err := s.q.ListDistributorOverridesForProducts(ctx, db.ListDistributorOverridesForProductsParams{
			BrandID: v.BrandID, DistributorOrgID: v.OrgID, ProductIds: ids,
		})
		if err != nil {
			return fmt.Errorf("list overrides: %w", err)
		}
		for _, o := range overrides {
			e := at(o.ProductID, o.Currency)
			e.PurchasePrice, e.PurchasePriceSource = strPtr(o.Price), SourceOverride
		}
	}
	if v.SaleRead {
		rows, err := s.q.ListDealerPricesForProducts(ctx, db.ListDealerPricesForProductsParams{
			BrandID: v.BrandID, DistributorOrgID: v.OrgID, ProductIds: ids,
		})
		if err != nil {
			return fmt.Errorf("list dealer prices: %w", err)
		}
		for _, r := range rows {
			at(r.ProductID, r.Currency).SalePrice = strPtr(r.Price)
		}
	}
	return nil
}

// Dealer: only its purchase price, i.e. the dealer price of its parent
// distributor (K8). A dealer without a distributor parent sees no price.
func (s *Service) dealerPrices(ctx context.Context, v Viewer, ids []int64, at func(int64, string) *EffectivePrice) error {
	if !v.PurchaseRead {
		return nil
	}
	parent, err := s.q.SupplierOf(ctx, v.OrgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("supplier: %w", err)
	}
	if parent.Type != OrgDistributor || parent.BrandID != v.BrandID {
		return nil
	}
	rows, err := s.q.ListDealerPricesForProducts(ctx, db.ListDealerPricesForProductsParams{
		BrandID: v.BrandID, DistributorOrgID: parent.ID, ProductIds: ids,
	})
	if err != nil {
		return fmt.Errorf("list dealer prices: %w", err)
	}
	for _, r := range rows {
		e := at(r.ProductID, r.Currency)
		e.PurchasePrice, e.PurchasePriceSource = strPtr(r.Price), SourceDistributor
	}
	return nil
}

func (s *Service) product(ctx context.Context, v Viewer, productUUID uuid.UUID) (db.Product, error) {
	p, err := s.q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: productUUID, BrandID: v.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Product{}, ErrProductNotFound
	}
	if err != nil {
		return db.Product{}, fmt.Errorf("product: %w", err)
	}
	return p, nil
}

// ProductView returns the effective price view of one product of the brand.
func (s *Service) ProductView(ctx context.Context, v Viewer, productUUID uuid.UUID) (ProductPriceView, error) {
	if err := checkViewerType(v); err != nil {
		return ProductPriceView{}, err
	}
	p, err := s.product(ctx, v, productUUID)
	if err != nil {
		return ProductPriceView{}, err
	}
	views, err := s.views(ctx, v, []productRef{{ID: p.ID, UUID: p.Uuid, SKU: p.Sku, Name: p.Name}})
	if err != nil {
		return ProductPriceView{}, err
	}
	return views[0], nil
}

// ListFilter narrows the product price list.
type ListFilter struct {
	Q      string
	Active *bool
	Limit  int32
	Offset int32
}

// ViewsByUUIDs returns the effective price views of the given products of
// the viewer's brand (TEC-211: catalog export price columns). Unknown or
// foreign products are left out.
func (s *Service) ViewsByUUIDs(ctx context.Context, v Viewer, uuids []uuid.UUID) ([]ProductPriceView, error) {
	if err := checkViewerType(v); err != nil {
		return nil, err
	}
	if len(uuids) == 0 {
		return []ProductPriceView{}, nil
	}
	rows, err := s.q.ListProductsByUUIDs(ctx, db.ListProductsByUUIDsParams{BrandID: v.BrandID, Uuids: uuids})
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	refs := make([]productRef, 0, len(rows))
	for _, r := range rows {
		refs = append(refs, productRef{ID: r.ID, UUID: r.Uuid, SKU: r.Sku, Name: r.Name})
	}
	return s.views(ctx, v, refs)
}

// ListViews returns a page of effective price views of the brand's products.
func (s *Service) ListViews(ctx context.Context, v Viewer, f ListFilter) ([]ProductPriceView, int64, error) {
	if err := checkViewerType(v); err != nil {
		return nil, 0, err
	}
	q := pgtype.Text{String: f.Q, Valid: f.Q != ""}
	active := pgtype.Bool{}
	if f.Active != nil {
		active = pgtype.Bool{Bool: *f.Active, Valid: true}
	}
	rows, err := s.q.ListPricedProducts(ctx, db.ListPricedProductsParams{
		BrandID: v.BrandID, Active: active, Q: q, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	total, err := s.q.CountPricedProducts(ctx, db.CountPricedProductsParams{BrandID: v.BrandID, Active: active, Q: q})
	if err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}
	refs := make([]productRef, 0, len(rows))
	for _, r := range rows {
		refs = append(refs, productRef{ID: r.ID, UUID: r.Uuid, SKU: r.Sku, Name: r.Name})
	}
	views, err := s.views(ctx, v, refs)
	if err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

// --- Center list price ----------------------------------------------------

// OptionalPrice is a PATCH-style field: Set=false keeps the stored value,
// Set=true with Value=nil clears it.
type OptionalPrice struct {
	Set   bool
	Value *string
}

// ListPriceInput is a center list price change for one currency.
type ListPriceInput struct {
	Purchase          OptionalPrice
	SaleToDistributor OptionalPrice
	Recommended       OptionalPrice
}

func mergePrice(field string, in OptionalPrice, current pgtype.Numeric) (pgtype.Text, error) {
	if !in.Set {
		if p := numericText(current); p != nil {
			return pgtype.Text{String: *p, Valid: true}, nil
		}
		return pgtype.Text{}, nil
	}
	if in.Value == nil {
		return pgtype.Text{}, nil
	}
	p, err := NormalizePrice(field, *in.Value)
	if err != nil {
		return pgtype.Text{}, err
	}
	return pgtype.Text{String: p, Valid: true}, nil
}

func requireCenter(v Viewer) error {
	if v.OrgType != OrgCenter || !v.SaleWrite {
		return ErrForbidden
	}
	return nil
}

func requireDistributor(v Viewer) error {
	if v.OrgType != OrgDistributor || !v.SaleWrite {
		return ErrForbidden
	}
	return nil
}

// SetListPrice writes the center list price of a product in one currency.
// Purchase and distributor sale prices need pricing.sale.write; the
// recommended price also needs pricing.recommended.write.
func (s *Service) SetListPrice(ctx context.Context, v Viewer, productUUID uuid.UUID, currency string, in ListPriceInput) (ProductPriceView, error) {
	if err := requireCenter(v); err != nil {
		return ProductPriceView{}, err
	}
	if in.Recommended.Set && !v.RecommendedWrite {
		return ProductPriceView{}, ErrForbidden
	}
	cur, err := NormalizeCurrency(currency)
	if err != nil {
		return ProductPriceView{}, err
	}
	p, err := s.product(ctx, v, productUUID)
	if err != nil {
		return ProductPriceView{}, err
	}
	current, err := s.q.GetProductPrice(ctx, db.GetProductPriceParams{ProductID: p.ID, BrandID: v.BrandID, Currency: cur})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ProductPriceView{}, fmt.Errorf("current price: %w", err)
	}
	purchase, err := mergePrice("purchase_price", in.Purchase, current.PurchasePrice)
	if err != nil {
		return ProductPriceView{}, err
	}
	sale, err := mergePrice("sale_to_distributor_price", in.SaleToDistributor, current.SaleToDistributorPrice)
	if err != nil {
		return ProductPriceView{}, err
	}
	rec, err := mergePrice("recommended_sale_price", in.Recommended, current.RecommendedSalePrice)
	if err != nil {
		return ProductPriceView{}, err
	}
	if _, err := s.q.UpsertProductPrice(ctx, db.UpsertProductPriceParams{
		ProductID: p.ID, BrandID: v.BrandID, Currency: cur,
		PurchasePrice: purchase, SaleToDistributorPrice: sale, RecommendedSalePrice: rec,
	}); err != nil {
		return ProductPriceView{}, fmt.Errorf("upsert price: %w", err)
	}
	return s.ProductView(ctx, v, productUUID)
}

// DeleteListPrice removes the center list price of a product in one currency.
func (s *Service) DeleteListPrice(ctx context.Context, v Viewer, productUUID uuid.UUID, currency string) error {
	if err := requireCenter(v); err != nil {
		return err
	}
	cur, err := NormalizeCurrency(currency)
	if err != nil {
		return err
	}
	p, err := s.product(ctx, v, productUUID)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteProductPrice(ctx, db.DeleteProductPriceParams{ProductID: p.ID, BrandID: v.BrandID, Currency: cur})
	if err != nil {
		return fmt.Errorf("delete price: %w", err)
	}
	if n == 0 {
		return ErrPriceNotFound
	}
	return nil
}

// --- Distributor-specific prices (center writes) --------------------------

// DistributorOverride is a distributor-specific price as the center sees it.
type DistributorOverride struct {
	ProductUUID     uuid.UUID `json:"product_uuid"`
	ProductSKU      string    `json:"product_sku"`
	ProductName     string    `json:"product_name"`
	DistributorUUID uuid.UUID `json:"distributor_uuid"`
	DistributorName string    `json:"distributor_name"`
	Currency        string    `json:"currency"`
	Price           string    `json:"price"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (s *Service) distributor(ctx context.Context, v Viewer, distributorUUID uuid.UUID) (db.Organization, error) {
	o, err := s.q.GetOrganizationByUUID(ctx, distributorUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrDistributorNotFound
	}
	if err != nil {
		return db.Organization{}, fmt.Errorf("distributor: %w", err)
	}
	if o.Type != OrgDistributor || o.BrandID != v.BrandID {
		return db.Organization{}, ErrDistributorNotFound
	}
	return o, nil
}

// SetDistributorOverride writes a distributor-specific price (center only).
func (s *Service) SetDistributorOverride(ctx context.Context, v Viewer, productUUID, distributorUUID uuid.UUID, currency, price string) (DistributorOverride, error) {
	if err := requireCenter(v); err != nil {
		return DistributorOverride{}, err
	}
	cur, err := NormalizeCurrency(currency)
	if err != nil {
		return DistributorOverride{}, err
	}
	pr, err := NormalizePrice("price", price)
	if err != nil {
		return DistributorOverride{}, err
	}
	p, err := s.product(ctx, v, productUUID)
	if err != nil {
		return DistributorOverride{}, err
	}
	d, err := s.distributor(ctx, v, distributorUUID)
	if err != nil {
		return DistributorOverride{}, err
	}
	row, err := s.q.UpsertDistributorPriceOverride(ctx, db.UpsertDistributorPriceOverrideParams{
		ProductID: p.ID, BrandID: v.BrandID, DistributorOrgID: d.ID, Currency: cur, Price: pr,
	})
	if err != nil {
		return DistributorOverride{}, fmt.Errorf("upsert override: %w", err)
	}
	return DistributorOverride{
		ProductUUID: p.Uuid, ProductSKU: p.Sku, ProductName: p.Name,
		DistributorUUID: d.Uuid, DistributorName: d.Name,
		Currency: strings.TrimSpace(row.Currency), Price: row.Price, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

// DeleteDistributorOverride removes a distributor-specific price.
func (s *Service) DeleteDistributorOverride(ctx context.Context, v Viewer, productUUID, distributorUUID uuid.UUID, currency string) error {
	if err := requireCenter(v); err != nil {
		return err
	}
	cur, err := NormalizeCurrency(currency)
	if err != nil {
		return err
	}
	p, err := s.product(ctx, v, productUUID)
	if err != nil {
		return err
	}
	d, err := s.distributor(ctx, v, distributorUUID)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteDistributorPriceOverride(ctx, db.DeleteDistributorPriceOverrideParams{
		ProductID: p.ID, BrandID: v.BrandID, DistributorOrgID: d.ID, Currency: cur,
	})
	if err != nil {
		return fmt.Errorf("delete override: %w", err)
	}
	if n == 0 {
		return ErrPriceNotFound
	}
	return nil
}

// DistributorPriceSort is the sort contract of GET
// /v1/tenant/pricing/distributor-prices (TEC-369, docs/list-contract.md).
// Default: product (sku), then distributor name and currency.
var DistributorPriceSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"product": "product", "distributor": "distributor", "currency": "currency",
		"price": "price", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "product"},
}

// OverrideFilter narrows the distributor-specific price list. Q matches the
// product sku / name and the distributor name; Currencies is any of.
type OverrideFilter struct {
	ProductUUID     *uuid.UUID
	DistributorUUID *uuid.UUID
	Q               string
	Currencies      []string
	Sort            apiquery.ResolvedSort
	Limit           int32
	Offset          int32
}

// ListDistributorOverrides lists distributor-specific prices of the brand
// (center with pricing.sale.read).
func (s *Service) ListDistributorOverrides(ctx context.Context, v Viewer, f OverrideFilter) ([]DistributorOverride, int64, error) {
	if v.OrgType != OrgCenter || !v.SaleRead {
		return nil, 0, ErrForbidden
	}
	var productID, distributorID pgtype.Int8
	if f.ProductUUID != nil {
		p, err := s.product(ctx, v, *f.ProductUUID)
		if err != nil {
			return nil, 0, err
		}
		productID = pgtype.Int8{Int64: p.ID, Valid: true}
	}
	if f.DistributorUUID != nil {
		d, err := s.distributor(ctx, v, *f.DistributorUUID)
		if err != nil {
			return nil, 0, err
		}
		distributorID = pgtype.Int8{Int64: d.ID, Valid: true}
	}
	if f.Sort.Key == "" {
		f.Sort = apiquery.ResolvedSort{Key: DistributorPriceSort.Default.Field}
	}
	var q pgtype.Text
	if t := strings.TrimSpace(f.Q); t != "" {
		q = pgtype.Text{String: t, Valid: true}
	}
	rows, err := s.q.ListDistributorOverrideDetails(ctx, db.ListDistributorOverrideDetailsParams{
		BrandID: v.BrandID, ProductID: productID, DistributorOrgID: distributorID,
		Currencies: f.Currencies, Q: q, SortKey: f.Sort.Key, SortDesc: f.Sort.Desc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list overrides: %w", err)
	}
	total, err := s.q.CountDistributorOverrideDetails(ctx, db.CountDistributorOverrideDetailsParams{
		BrandID: v.BrandID, ProductID: productID, DistributorOrgID: distributorID,
		Currencies: f.Currencies, Q: q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count overrides: %w", err)
	}
	out := make([]DistributorOverride, 0, len(rows))
	for _, r := range rows {
		out = append(out, DistributorOverride{
			ProductUUID: r.ProductUuid, ProductSKU: r.ProductSku, ProductName: r.ProductName,
			DistributorUUID: r.DistributorUuid, DistributorName: r.DistributorName,
			Currency: strings.TrimSpace(r.Currency), Price: r.Price, UpdatedAt: r.UpdatedAt.Time,
		})
	}
	return out, total, nil
}

// --- Distributor's dealer price (distributor writes) ----------------------

// SetDealerPrice writes the price the caller distributor sells a product to
// its dealers (the dealers' purchase price, K8).
func (s *Service) SetDealerPrice(ctx context.Context, v Viewer, productUUID uuid.UUID, currency, price string) (ProductPriceView, error) {
	if err := requireDistributor(v); err != nil {
		return ProductPriceView{}, err
	}
	cur, err := NormalizeCurrency(currency)
	if err != nil {
		return ProductPriceView{}, err
	}
	pr, err := NormalizePrice("price", price)
	if err != nil {
		return ProductPriceView{}, err
	}
	p, err := s.product(ctx, v, productUUID)
	if err != nil {
		return ProductPriceView{}, err
	}
	if _, err := s.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: p.ID, BrandID: v.BrandID, DistributorOrgID: v.OrgID, Currency: cur, Price: pr,
	}); err != nil {
		return ProductPriceView{}, fmt.Errorf("upsert dealer price: %w", err)
	}
	return s.ProductView(ctx, v, productUUID)
}

// DeleteDealerPrice removes the caller distributor's dealer price.
func (s *Service) DeleteDealerPrice(ctx context.Context, v Viewer, productUUID uuid.UUID, currency string) error {
	if err := requireDistributor(v); err != nil {
		return err
	}
	cur, err := NormalizeCurrency(currency)
	if err != nil {
		return err
	}
	p, err := s.product(ctx, v, productUUID)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteDistributorDealerPrice(ctx, db.DeleteDistributorDealerPriceParams{
		ProductID: p.ID, BrandID: v.BrandID, DistributorOrgID: v.OrgID, Currency: cur,
	})
	if err != nil {
		return fmt.Errorf("delete dealer price: %w", err)
	}
	if n == 0 {
		return ErrPriceNotFound
	}
	return nil
}
