package usecase

// TEC-348 (F3-07h): paged reads behind the dealer sales screens (sale price
// catalog, barcode lookup for the quick sale, sale/supplier/purchase lists).
// Sort keys are the handler's whitelisted apiquery keys.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListPage is the paging + sort part every list read shares.
type ListPage struct {
	Limit, Offset int32
	Q             string
	Sort          apiquery.ResolvedSort
}

// PriceCatalogFilter narrows the sale price catalog.
type PriceCatalogFilter struct {
	ListPage
	Priced *bool
	// PurchaseVisible: the caller holds pricing.purchase.read; otherwise the
	// purchase price and the estimated profit stay null.
	PurchaseVisible bool
}

type PriceCatalogItem struct {
	ProductUUID          uuid.UUID  `json:"product_uuid"`
	SKU                  string     `json:"sku"`
	Name                 string     `json:"name"`
	UsesFixedBarcode     bool       `json:"uses_fixed_barcode"`
	Currency             string     `json:"currency"`
	SalePrice            *string    `json:"sale_price"`
	RecommendedSalePrice *string    `json:"recommended_sale_price"`
	PurchasePrice        *string    `json:"purchase_price"`
	EstimatedProfit      *string    `json:"estimated_profit"`
	UpdatedAt            *time.Time `json:"updated_at"`
	// Recommended / DeviationPct: see DealerPriceView (TEC-506).
	Recommended  *usecase.RecommendedRef `json:"recommended,omitempty"`
	DeviationPct *string                 `json:"deviation_pct,omitempty"`
}

func (s *Service) ListPriceCatalog(ctx context.Context, c Caller, f PriceCatalogFilter) (apiquery.Page[PriceCatalogItem], error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return apiquery.Page[PriceCatalogItem]{}, err
	}
	rows, err := s.q.ListDealerPriceCatalog(ctx, db.ListDealerPriceCatalogParams{
		OrganizationID: o.ID, BrandID: o.BrandID, Currency: o.Currency,
		Q: text(f.Q), Priced: boolNarg(f.Priced),
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return apiquery.Page[PriceCatalogItem]{}, fmt.Errorf("dealer accounting: price catalog: %w", err)
	}
	total, err := s.q.CountDealerPriceCatalog(ctx, db.CountDealerPriceCatalogParams{
		OrganizationID: o.ID, BrandID: o.BrandID, Q: text(f.Q), Priced: boolNarg(f.Priced),
	})
	if err != nil {
		return apiquery.Page[PriceCatalogItem]{}, fmt.Errorf("dealer accounting: count price catalog: %w", err)
	}
	purchase := map[int64]usecase.PurchasePrice{}
	if f.PurchaseVisible && len(rows) > 0 {
		ids := make([]int64, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ProductID)
		}
		if purchase, err = usecase.New(s.q).BuyerPurchasePrices(ctx, o.ID, o.Type, o.BrandID, ids, o.Currency); err != nil {
			return apiquery.Page[PriceCatalogItem]{}, fmt.Errorf("dealer accounting: purchase prices: %w", err)
		}
	}
	var refs map[usecase.RecommendedKey]usecase.RecommendedRef
	if c.RecommendedRead && len(rows) > 0 {
		ids := make([]int64, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ProductID)
		}
		if refs, err = usecase.ApplicableRecommended(ctx, s.q, o.BrandID, o.CountryID, ids, nil); err != nil {
			return apiquery.Page[PriceCatalogItem]{}, fmt.Errorf("dealer accounting: %w", err)
		}
	}
	items := make([]PriceCatalogItem, 0, len(rows))
	for _, r := range rows {
		it := PriceCatalogItem{
			ProductUUID: r.ProductUuid, SKU: r.Sku, Name: r.Name, UsesFixedBarcode: r.UsesFixedBarcode,
			Currency: o.Currency, SalePrice: moneyPtr(r.SalePrice), RecommendedSalePrice: moneyPtr(r.RecommendedSalePrice),
		}
		if r.SaleCurrency.Valid {
			it.Currency = r.SaleCurrency.String
		}
		if r.PriceUpdatedAt.Valid {
			t := r.PriceUpdatedAt.Time
			it.UpdatedAt = &t
		}
		if ref, ok := refs[usecase.RecommendedKey{ProductID: r.ProductID, Currency: strings.TrimSpace(it.Currency)}]; ok {
			it.Recommended = &ref
			if it.SalePrice != nil {
				it.DeviationPct = usecase.DeviationPct(*it.SalePrice, ref.Price)
			}
		}
		if p, ok := purchase[r.ProductID]; ok {
			pp, err := normalizeMoneyLoose(p.Price)
			if err == nil {
				it.PurchasePrice = &pp
				ref := it.SalePrice
				if ref == nil {
					ref = it.RecommendedSalePrice
				}
				if ref != nil {
					if d, err := subtract(*ref, pp); err == nil {
						it.EstimatedProfit = &d
					}
				}
			}
		}
		items = append(items, it)
	}
	return apiquery.NewPage(items, total, f.Limit, f.Offset), nil
}

// SaleLookup is the quick-sale preview of one scanned barcode or picked
// product: the unit the sale would consume and its prices.
type SaleLookup struct {
	ProductUUID          uuid.UUID `json:"product_uuid"`
	SKU                  string    `json:"sku"`
	Name                 string    `json:"name"`
	Barcode              string    `json:"barcode"`
	UnitKind             string    `json:"unit_kind"`
	QuantityOnHand       int32     `json:"quantity_on_hand"`
	Currency             string    `json:"currency"`
	SalePrice            *string   `json:"sale_price"`
	RecommendedSalePrice *string   `json:"recommended_sale_price"`
	PurchasePrice        *string   `json:"purchase_price"`
}

// LookupSaleItem resolves a barcode (or a product) to the first unit the
// dealer holds, in the same order CreateProductSale consumes them. Nothing
// held → ErrStockUnavailable.
func (s *Service) LookupSaleItem(ctx context.Context, c Caller, barcode string, productUUID *uuid.UUID, purchaseVisible bool) (SaleLookup, error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return SaleLookup{}, err
	}
	var productID pgtype.Int8
	if productUUID != nil {
		p, err := s.q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: *productUUID, BrandID: o.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return SaleLookup{}, ErrProductNotFound
		}
		if err != nil {
			return SaleLookup{}, fmt.Errorf("dealer accounting: product: %w", err)
		}
		productID = pgtype.Int8{Int64: p.ID, Valid: true}
	}
	barcode = strings.TrimSpace(barcode)
	if barcode == "" && !productID.Valid {
		return SaleLookup{}, invalid("barcode", "or product_uuid is required")
	}
	rows, err := s.q.ListProductSaleStockCandidates(ctx, db.ListProductSaleStockCandidatesParams{
		OrganizationID: o.ID, BrandID: o.BrandID, ProductID: productID, Barcode: text(barcode),
	})
	if err != nil {
		return SaleLookup{}, fmt.Errorf("dealer accounting: stock candidates: %w", err)
	}
	if len(rows) == 0 {
		return SaleLookup{}, ErrStockUnavailable
	}
	first := rows[0]
	held := int32(0)
	for _, r := range rows {
		if r.UnitID == first.UnitID {
			held += r.QuantityOnHand
		}
	}
	p, err := s.q.GetProduct(ctx, db.GetProductParams{ID: first.ProductID, BrandID: o.BrandID})
	if err != nil {
		return SaleLookup{}, fmt.Errorf("dealer accounting: product: %w", err)
	}
	out := SaleLookup{
		ProductUUID: p.Uuid, SKU: p.Sku, Name: p.Name, Barcode: first.Barcode, UnitKind: first.UnitKind,
		QuantityOnHand: held, Currency: o.Currency,
	}
	if first.UnitKind != ledger.KindFixed {
		out.QuantityOnHand = 1
	}
	if dp, err := s.q.GetDealerProductPrice(ctx, db.GetDealerProductPriceParams{OrganizationID: o.ID, ProductID: p.ID}); err == nil {
		out.SalePrice = moneyPtr(dp.SalePrice)
	}
	if rec, err := s.q.GetRecommendedProductPrice(ctx, db.GetRecommendedProductPriceParams{BrandID: o.BrandID, ProductID: p.ID, Currency: o.Currency}); err == nil {
		out.RecommendedSalePrice = moneyPtr(rec)
	}
	if purchaseVisible {
		if cost := s.purchaseCost(ctx, s.q, o, first.UnitID, p.ID); cost != nil {
			if v, err := normalizeMoneyLoose(*cost); err == nil {
				out.PurchasePrice = &v
			}
		}
	}
	return out, nil
}

type SaleListFilter struct {
	ListPage
	PaymentMethods     []string
	Voided             *bool
	Sold               apiquery.TimeRange
	TotalMin, TotalMax *float64
	PurchaseVisible    bool
}

type SaleListItem struct {
	UUID          uuid.UUID  `json:"uuid"`
	SoldAt        time.Time  `json:"sold_at"`
	PaymentMethod string     `json:"payment_method"`
	Currency      string     `json:"currency"`
	Total         string     `json:"total"`
	Profit        *string    `json:"profit"`
	CustomerUUID  *uuid.UUID `json:"customer_uuid"`
	CustomerName  *string    `json:"customer_name"`
	Products      string     `json:"products"`
	LineCount     int64      `json:"line_count"`
	Note          string     `json:"note"`
	Voided        bool       `json:"voided"`
}

func (s *Service) ListProductSales(ctx context.Context, c Caller, f SaleListFilter) (apiquery.Page[SaleListItem], error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return apiquery.Page[SaleListItem]{}, err
	}
	rows, err := s.q.ListProductSalesPage(ctx, db.ListProductSalesPageParams{
		OrganizationID: o.ID, PaymentMethods: f.PaymentMethods, Voided: boolNarg(f.Voided),
		SoldFrom: tsNarg(f.Sold.From), SoldBefore: tsNarg(f.Sold.Before),
		TotalMin: floatNarg(f.TotalMin), TotalMax: floatNarg(f.TotalMax), Q: text(f.Q),
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return apiquery.Page[SaleListItem]{}, fmt.Errorf("dealer accounting: sales: %w", err)
	}
	total, err := s.q.CountProductSalesPage(ctx, db.CountProductSalesPageParams{
		OrganizationID: o.ID, PaymentMethods: f.PaymentMethods, Voided: boolNarg(f.Voided),
		SoldFrom: tsNarg(f.Sold.From), SoldBefore: tsNarg(f.Sold.Before),
		TotalMin: floatNarg(f.TotalMin), TotalMax: floatNarg(f.TotalMax), Q: text(f.Q),
	})
	if err != nil {
		return apiquery.Page[SaleListItem]{}, fmt.Errorf("dealer accounting: count sales: %w", err)
	}
	items := make([]SaleListItem, 0, len(rows))
	for _, r := range rows {
		it := SaleListItem{
			UUID: r.Uuid, SoldAt: r.SoldAt.Time, PaymentMethod: r.PaymentMethod, Currency: r.Currency,
			Total: format(r.Total, 2), Products: r.Products, LineCount: r.LineCount, Note: r.Note, Voided: r.Voided,
		}
		if f.PurchaseVisible {
			it.Profit = moneyPtr(r.Profit)
		}
		if r.CustomerUuid.Valid {
			id := uuid.UUID(r.CustomerUuid.Bytes)
			name := r.CustomerName
			it.CustomerUUID, it.CustomerName = &id, &name
		}
		items = append(items, it)
	}
	return apiquery.NewPage(items, total, f.Limit, f.Offset), nil
}

type SupplierListFilter struct {
	ListPage
	Active  *bool
	Created apiquery.TimeRange
}

func (s *Service) ListSuppliersPage(ctx context.Context, c Caller, f SupplierListFilter) (apiquery.Page[SupplierView], error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return apiquery.Page[SupplierView]{}, err
	}
	rows, err := s.q.ListSuppliersPage(ctx, db.ListSuppliersPageParams{
		OrganizationID: o.ID, Active: boolNarg(f.Active), Q: text(f.Q),
		CreatedFrom: tsNarg(f.Created.From), CreatedBefore: tsNarg(f.Created.Before),
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return apiquery.Page[SupplierView]{}, fmt.Errorf("dealer accounting: suppliers: %w", err)
	}
	total, err := s.q.CountSuppliersPage(ctx, db.CountSuppliersPageParams{
		OrganizationID: o.ID, Active: boolNarg(f.Active), Q: text(f.Q),
		CreatedFrom: tsNarg(f.Created.From), CreatedBefore: tsNarg(f.Created.Before),
	})
	if err != nil {
		return apiquery.Page[SupplierView]{}, fmt.Errorf("dealer accounting: count suppliers: %w", err)
	}
	items := make([]SupplierView, 0, len(rows))
	for _, r := range rows {
		items = append(items, supplierView(r))
	}
	return apiquery.NewPage(items, total, f.Limit, f.Offset), nil
}

type PurchaseListFilter struct {
	ListPage
	SupplierUUIDs        []uuid.UUID
	PaymentMethods       []string
	Purchased            apiquery.TimeRange
	AmountMin, AmountMax *float64
}

type PurchaseListItem struct {
	UUID          uuid.UUID `json:"uuid"`
	SupplierUUID  uuid.UUID `json:"supplier_uuid"`
	SupplierName  string    `json:"supplier_name"`
	Amount        string    `json:"amount"`
	Currency      string    `json:"currency"`
	PaymentMethod string    `json:"payment_method"`
	PurchasedOn   string    `json:"purchased_on"`
	Description   string    `json:"description"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
}

func (s *Service) ListPurchases(ctx context.Context, c Caller, f PurchaseListFilter) (apiquery.Page[PurchaseListItem], error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return apiquery.Page[PurchaseListItem]{}, err
	}
	rows, err := s.q.ListPurchasesPage(ctx, db.ListPurchasesPageParams{
		OrganizationID: o.ID, SupplierUuids: f.SupplierUUIDs, PaymentMethods: f.PaymentMethods,
		DateFrom: dateNarg(f.Purchased.From), DateBefore: dateNarg(f.Purchased.Before),
		AmountMin: floatNarg(f.AmountMin), AmountMax: floatNarg(f.AmountMax), Q: text(f.Q),
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return apiquery.Page[PurchaseListItem]{}, fmt.Errorf("dealer accounting: purchases: %w", err)
	}
	total, err := s.q.CountPurchasesPage(ctx, db.CountPurchasesPageParams{
		OrganizationID: o.ID, SupplierUuids: f.SupplierUUIDs, PaymentMethods: f.PaymentMethods,
		DateFrom: dateNarg(f.Purchased.From), DateBefore: dateNarg(f.Purchased.Before),
		AmountMin: floatNarg(f.AmountMin), AmountMax: floatNarg(f.AmountMax), Q: text(f.Q),
	})
	if err != nil {
		return apiquery.Page[PurchaseListItem]{}, fmt.Errorf("dealer accounting: count purchases: %w", err)
	}
	items := make([]PurchaseListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, PurchaseListItem{
			UUID: r.Uuid, SupplierUUID: r.SupplierUuid, SupplierName: r.SupplierName, Amount: format(r.Amount, 2),
			Currency: r.Currency, PaymentMethod: r.PaymentMethod, PurchasedOn: r.PurchasedOn.Time.Format(time.DateOnly),
			Description: r.Description, Note: r.Note, CreatedAt: r.CreatedAt.Time,
		})
	}
	return apiquery.NewPage(items, total, f.Limit, f.Offset), nil
}

func moneyPtr(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	v := format(n, 2)
	return &v
}

func normalizeMoneyLoose(s string) (string, error) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return "", fmt.Errorf("invalid decimal %q", s)
	}
	return r.FloatString(2), nil
}

func subtract(a, b string) (string, error) {
	x, ok := new(big.Rat).SetString(a)
	if !ok {
		return "", fmt.Errorf("invalid decimal %q", a)
	}
	y, ok := new(big.Rat).SetString(b)
	if !ok {
		return "", fmt.Errorf("invalid decimal %q", b)
	}
	return new(big.Rat).Sub(x, y).FloatString(2), nil
}

func boolNarg(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

func tsNarg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// dateNarg turns a TimeRange bound into a date; a mid-day exclusive bound
// (RFC3339 _to) rounds up so that day stays included.
func dateNarg(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	d := t.UTC().Truncate(24 * time.Hour)
	if !d.Equal(t.UTC()) {
		d = d.AddDate(0, 0, 1)
	}
	return pgtype.Date{Time: d, Valid: true}
}

func floatNarg(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	n, err := numeric(strconv.FormatFloat(*f, 'f', -1, 64))
	if err != nil {
		return pgtype.Numeric{}
	}
	return n
}
