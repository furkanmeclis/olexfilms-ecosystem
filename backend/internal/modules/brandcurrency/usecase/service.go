// Package usecase manages the brand currency (TEC-1214): brands.currency is
// the price and order currency of a brand (organizations.currency stays the
// ledger currency, K7). A platform admin previews a change (missing prices,
// open orders, rates), then applies it; existing approved orders, caris and
// finance entries keep their own currency and frozen rates. The price
// conversion tool writes the price list in a new currency from the current
// one at today's rate, as prices that stay unused until the brand switches.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	// ErrNotFound: unknown brand.
	ErrNotFound = errors.New("brand not found")
	// ErrInvalid: malformed or inactive currency, unknown option.
	ErrInvalid = errors.New("invalid request")
	// ErrRateNotFound: no rate for the conversion.
	ErrRateNotFound = errors.New("rate not found")
)

// BlockedError lists why a change cannot be applied (409).
type BlockedError struct {
	Blockers []string
	Preview  ChangePreview
}

func (e *BlockedError) Error() string {
	return "brand currency change blocked: " + strings.Join(e.Blockers, ", ")
}

// Open order handling of a change.
const (
	OpenOrdersBlock   = "block"
	OpenOrdersReprice = "reprice"
)

// Blockers of a change.
const (
	BlockerSameCurrency          = "same_currency"
	BlockerOpenOrders            = "open_orders"
	BlockerMissingPrices         = "missing_prices"
	BlockerRateToLocalMissing    = "rate_to_local_missing"
	BlockerConversionRateMissing = "conversion_rate_missing"
)

// PreviewLimit caps the listed products and orders of a preview.
const PreviewLimit = 200

// TxBeginner starts a transaction (*pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// RateResolver resolves a rate on a day (*fxrates.Service).
type RateResolver interface {
	ResolveRate(ctx context.Context, on time.Time, base, quote string) (fxrates.Snapshot, error)
}

// LocalCurrencySource is the local display currency (*sysconfig.Service).
type LocalCurrencySource interface {
	LocalCurrency(ctx context.Context) string
}

// Service is the brand currency use case.
type Service struct {
	pool  TxBeginner
	q     *db.Queries
	rates RateResolver
	local LocalCurrencySource
	now   func() time.Time
}

// New creates the service.
func New(pool TxBeginner, q *db.Queries, rates RateResolver, local LocalCurrencySource) *Service {
	return &Service{pool: pool, q: q, rates: rates, local: local, now: time.Now}
}

// Brand is a brand with its currency.
type Brand struct {
	UUID     uuid.UUID `json:"uuid"`
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	Currency string    `json:"currency"`
}

// List returns the brands with their currency and the local currency.
func (s *Service) List(ctx context.Context) ([]Brand, string, error) {
	rows, err := s.q.ListBrandsWithCurrency(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("brand currency: list: %w", err)
	}
	out := make([]Brand, 0, len(rows))
	for _, r := range rows {
		out = append(out, Brand{UUID: r.Uuid, Slug: r.Slug, Name: r.Name, Status: r.Status, Currency: strings.TrimSpace(r.Currency)})
	}
	return out, s.localCurrency(ctx), nil
}

func (s *Service) localCurrency(ctx context.Context) string {
	if s.local != nil {
		if c := s.local.LocalCurrency(ctx); c != "" {
			return c
		}
	}
	return "TRY"
}

// Rate is a resolved rate of a preview.
type Rate struct {
	Base     string `json:"base"`
	Quote    string `json:"quote"`
	Rate     string `json:"rate"`
	RateDate string `json:"rate_date"`
	Source   string `json:"source"`
}

// MissingPrice is an active product without a center sale price in the
// new currency.
type MissingPrice struct {
	ProductUUID uuid.UUID `json:"product_uuid"`
	SKU         string    `json:"sku"`
	Name        string    `json:"name"`
}

// OpenOrder is a draft or submitted order (prices not frozen yet).
type OpenOrder struct {
	UUID      uuid.UUID `json:"uuid"`
	OrderNo   string    `json:"order_no"`
	Status    string    `json:"status"`
	Currency  string    `json:"currency"`
	Total     string    `json:"total"`
	Seller    string    `json:"seller_name"`
	Buyer     string    `json:"buyer_name"`
	CreatedAt time.Time `json:"created_at"`
}

// ChangeInput is a PUT /v1/platform/brands/{uuid}/currency body.
type ChangeInput struct {
	Currency           string
	DryRun             bool
	OpenOrders         string
	AllowMissingPrices bool
}

// ChangePreview is the result of a change (dry run or applied).
type ChangePreview struct {
	Brand             Brand          `json:"brand"`
	From              string         `json:"from"`
	To                string         `json:"to"`
	LocalCurrency     string         `json:"local_currency"`
	OpenOrdersOption  string         `json:"open_orders_option"`
	ActiveProducts    int64          `json:"active_products"`
	MissingPriceCount int64          `json:"missing_price_count"`
	MissingPrices     []MissingPrice `json:"missing_prices"`
	OpenOrderCount    int64          `json:"open_order_count"`
	OpenOrders        []OpenOrder    `json:"open_orders"`
	// RateToLocal is new currency -> local currency (order approvals need
	// it); null when missing.
	RateToLocal *Rate `json:"rate_to_local"`
	// ConversionRate is old -> new currency (reprice); null when missing.
	ConversionRate *Rate    `json:"conversion_rate"`
	Blockers       []string `json:"blockers"`
	DryRun         bool     `json:"dry_run"`
	Applied        bool     `json:"applied"`
	RepricedOrders int64    `json:"repriced_orders"`
}

func (s *Service) normalizeCurrency(ctx context.Context, raw string) (string, error) {
	c, err := fxrates.NormalizeCode(raw)
	if err != nil {
		return "", fmt.Errorf("%w: currency must be an ISO 4217 code", ErrInvalid)
	}
	active, err := s.q.ListCurrencies(ctx, true)
	if err != nil {
		return "", fmt.Errorf("brand currency: currencies: %w", err)
	}
	for _, a := range active {
		if strings.TrimSpace(a.Code) == c {
			return c, nil
		}
	}
	return "", fmt.Errorf("%w: currency %s is not an active currency", ErrInvalid, c)
}

func (s *Service) rate(ctx context.Context, base, quote string) (*Rate, error) {
	if s.rates == nil {
		return nil, nil
	}
	snap, err := s.rates.ResolveRate(ctx, s.now().UTC(), base, quote)
	if errors.Is(err, fxrates.ErrRateNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("brand currency: rate %s/%s: %w", base, quote, err)
	}
	return &Rate{Base: snap.Base, Quote: snap.Quote, Rate: snap.Rate, RateDate: snap.RateDate, Source: snap.Source}, nil
}

// ChangeCurrency previews (DryRun) or applies a brand currency change. An
// applied change with blockers answers *BlockedError and changes nothing.
func (s *Service) ChangeCurrency(ctx context.Context, brandUUID uuid.UUID, in ChangeInput) (ChangePreview, error) {
	to, err := s.normalizeCurrency(ctx, in.Currency)
	if err != nil {
		return ChangePreview{}, err
	}
	option := strings.TrimSpace(in.OpenOrders)
	if option == "" {
		option = OpenOrdersBlock
	}
	if option != OpenOrdersBlock && option != OpenOrdersReprice {
		return ChangePreview{}, fmt.Errorf("%w: open_orders must be block or reprice", ErrInvalid)
	}
	if in.DryRun {
		b, err := s.q.GetBrandByUUID(ctx, brandUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ChangePreview{}, ErrNotFound
		}
		if err != nil {
			return ChangePreview{}, fmt.Errorf("brand currency: brand: %w", err)
		}
		p, err := s.preview(ctx, s.q, b, to, option, in.AllowMissingPrices)
		if err != nil {
			return ChangePreview{}, err
		}
		p.DryRun = true
		return p, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ChangePreview{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	b, err := q.LockBrandByUUID(ctx, brandUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangePreview{}, ErrNotFound
	}
	if err != nil {
		return ChangePreview{}, fmt.Errorf("brand currency: lock brand: %w", err)
	}
	orderIDs, err := q.LockBrandOpenOrders(ctx, b.ID)
	if err != nil {
		return ChangePreview{}, fmt.Errorf("brand currency: lock orders: %w", err)
	}
	p, err := s.preview(ctx, q, b, to, option, in.AllowMissingPrices)
	if err != nil {
		return ChangePreview{}, err
	}
	if len(p.Blockers) > 0 {
		return ChangePreview{}, &BlockedError{Blockers: p.Blockers, Preview: p}
	}
	updated, err := q.SetBrandCurrency(ctx, db.SetBrandCurrencyParams{ID: b.ID, Currency: to})
	if err != nil {
		return ChangePreview{}, fmt.Errorf("brand currency: update: %w", err)
	}
	if len(orderIDs) > 0 {
		var rate pgtype.Numeric
		if err := rate.Scan(p.ConversionRate.Rate); err != nil {
			return ChangePreview{}, fmt.Errorf("brand currency: rate %q: %w", p.ConversionRate.Rate, err)
		}
		for _, id := range orderIDs {
			if _, err := q.RepriceOrderItems(ctx, db.RepriceOrderItemsParams{OrderID: id, Rate: rate}); err != nil {
				return ChangePreview{}, fmt.Errorf("brand currency: reprice lines: %w", err)
			}
			if err := q.RepriceOrderHeader(ctx, db.RepriceOrderHeaderParams{ID: id, Currency: to, Rate: rate}); err != nil {
				return ChangePreview{}, fmt.Errorf("brand currency: reprice order: %w", err)
			}
		}
		p.RepricedOrders = int64(len(orderIDs))
	}
	if err := tx.Commit(ctx); err != nil {
		return ChangePreview{}, err
	}
	p.Brand.Currency = strings.TrimSpace(updated.Currency)
	p.Applied = true
	return p, nil
}

func (s *Service) preview(ctx context.Context, q *db.Queries, b db.Brand, to, option string, allowMissing bool) (ChangePreview, error) {
	from := strings.TrimSpace(b.Currency)
	p := ChangePreview{
		Brand: Brand{UUID: b.Uuid, Slug: b.Slug, Name: b.Name, Status: b.Status, Currency: from},
		From:  from, To: to, LocalCurrency: s.localCurrency(ctx), OpenOrdersOption: option,
		MissingPrices: []MissingPrice{}, OpenOrders: []OpenOrder{}, Blockers: []string{},
	}
	if from == to {
		p.Blockers = append(p.Blockers, BlockerSameCurrency)
	}
	var err error
	if p.ActiveProducts, err = q.CountBrandActiveProducts(ctx, b.ID); err != nil {
		return p, fmt.Errorf("brand currency: products: %w", err)
	}
	missing, err := q.ListBrandProductsMissingPrice(ctx, db.ListBrandProductsMissingPriceParams{BrandID: b.ID, Currency: to, RowLimit: PreviewLimit})
	if err != nil {
		return p, fmt.Errorf("brand currency: missing prices: %w", err)
	}
	for _, m := range missing {
		p.MissingPriceCount = m.TotalCount
		p.MissingPrices = append(p.MissingPrices, MissingPrice{ProductUUID: m.Uuid, SKU: m.Sku, Name: m.Name})
	}
	orders, err := q.ListBrandOpenOrders(ctx, db.ListBrandOpenOrdersParams{BrandID: b.ID, RowLimit: PreviewLimit})
	if err != nil {
		return p, fmt.Errorf("brand currency: open orders: %w", err)
	}
	for _, o := range orders {
		p.OpenOrderCount = o.TotalCount
		p.OpenOrders = append(p.OpenOrders, OpenOrder{
			UUID: o.Uuid, OrderNo: o.OrderNo, Status: o.Status, Currency: strings.TrimSpace(o.Currency), Total: o.Total,
			Seller: o.SellerName, Buyer: o.BuyerName, CreatedAt: o.CreatedAt.Time.UTC(),
		})
	}
	if p.RateToLocal, err = s.rate(ctx, to, p.LocalCurrency); err != nil {
		return p, err
	}
	if from != to {
		if p.ConversionRate, err = s.rate(ctx, from, to); err != nil {
			return p, err
		}
	}
	if p.RateToLocal == nil {
		p.Blockers = append(p.Blockers, BlockerRateToLocalMissing)
	}
	if p.OpenOrderCount > 0 {
		switch {
		case option == OpenOrdersBlock:
			p.Blockers = append(p.Blockers, BlockerOpenOrders)
		case p.ConversionRate == nil && from != to:
			p.Blockers = append(p.Blockers, BlockerConversionRateMissing)
		}
	}
	if p.MissingPriceCount > 0 && !allowMissing {
		p.Blockers = append(p.Blockers, BlockerMissingPrices)
	}
	return p, nil
}

// PriceSet is the three center price list prices of a product.
type PriceSet struct {
	PurchasePrice          *string `json:"purchase_price"`
	SaleToDistributorPrice *string `json:"sale_to_distributor_price"`
	RecommendedSalePrice   *string `json:"recommended_sale_price"`
}

// Conversion row actions.
const (
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionSkip   = "skip"
)

// ConversionRow is one product of a price conversion.
type ConversionRow struct {
	ProductUUID uuid.UUID `json:"product_uuid"`
	SKU         string    `json:"sku"`
	Name        string    `json:"name"`
	Action      string    `json:"action"`
	Source      PriceSet  `json:"source"`
	Converted   PriceSet  `json:"converted"`
	// Existing is the current row in the target currency (null: none).
	Existing *PriceSet `json:"existing"`
}

// ConvertInput is a POST /v1/platform/brands/{uuid}/price-conversion body.
type ConvertInput struct {
	SourceCurrency string
	TargetCurrency string
	DryRun         bool
	Overwrite      bool
}

// ConversionPreview is the result of a price conversion.
type ConversionPreview struct {
	Brand     Brand           `json:"brand"`
	Source    string          `json:"source_currency"`
	Target    string          `json:"target_currency"`
	Rate      Rate            `json:"rate"`
	Overwrite bool            `json:"overwrite"`
	Total     int             `json:"total"`
	Create    int             `json:"create"`
	Update    int             `json:"update"`
	Skip      int             `json:"skip"`
	Rows      []ConversionRow `json:"rows"`
	DryRun    bool            `json:"dry_run"`
	Applied   bool            `json:"applied"`
}

// ConvertPrices previews (DryRun) or writes the price list of the source
// currency (default: the brand currency) in the target currency at today's
// rate, rounded to cents. A product that already has a target row is
// skipped unless Overwrite.
func (s *Service) ConvertPrices(ctx context.Context, brandUUID uuid.UUID, in ConvertInput) (ConversionPreview, error) {
	target, err := s.normalizeCurrency(ctx, in.TargetCurrency)
	if err != nil {
		return ConversionPreview{}, err
	}
	b, err := s.q.GetBrandByUUID(ctx, brandUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConversionPreview{}, ErrNotFound
	}
	if err != nil {
		return ConversionPreview{}, fmt.Errorf("brand currency: brand: %w", err)
	}
	source := strings.TrimSpace(b.Currency)
	if strings.TrimSpace(in.SourceCurrency) != "" {
		if source, err = fxrates.NormalizeCode(in.SourceCurrency); err != nil {
			return ConversionPreview{}, fmt.Errorf("%w: source_currency must be an ISO 4217 code", ErrInvalid)
		}
	}
	if source == target {
		return ConversionPreview{}, fmt.Errorf("%w: target_currency must differ from the source currency", ErrInvalid)
	}
	rate, err := s.rate(ctx, source, target)
	if err != nil {
		return ConversionPreview{}, err
	}
	if rate == nil {
		return ConversionPreview{}, ErrRateNotFound
	}
	r, ok := new(big.Rat).SetString(rate.Rate)
	if !ok || r.Sign() <= 0 {
		return ConversionPreview{}, fmt.Errorf("brand currency: bad rate %q", rate.Rate)
	}
	out := ConversionPreview{
		Brand:  Brand{UUID: b.Uuid, Slug: b.Slug, Name: b.Name, Status: b.Status, Currency: strings.TrimSpace(b.Currency)},
		Source: source, Target: target, Rate: *rate, Overwrite: in.Overwrite, Rows: []ConversionRow{}, DryRun: in.DryRun,
	}
	rows, err := s.q.ListBrandPricesInCurrency(ctx, db.ListBrandPricesInCurrencyParams{BrandID: b.ID, Source: source, Target: target})
	if err != nil {
		return ConversionPreview{}, fmt.Errorf("brand currency: prices: %w", err)
	}
	type write struct {
		productID int64
		set       PriceSet
	}
	writes := make([]write, 0, len(rows))
	for _, row := range rows {
		src := PriceSet{PurchasePrice: textPtr(row.PurchasePrice), SaleToDistributorPrice: textPtr(row.SaleToDistributorPrice), RecommendedSalePrice: textPtr(row.RecommendedSalePrice)}
		conv := PriceSet{PurchasePrice: convert(src.PurchasePrice, r), SaleToDistributorPrice: convert(src.SaleToDistributorPrice, r), RecommendedSalePrice: convert(src.RecommendedSalePrice, r)}
		cr := ConversionRow{ProductUUID: row.ProductUuid, SKU: row.Sku, Name: row.Name, Source: src, Converted: conv}
		switch {
		case row.TargetExists && !in.Overwrite:
			cr.Action = ActionSkip
			out.Skip++
		case row.TargetExists:
			cr.Action = ActionUpdate
			out.Update++
		default:
			cr.Action = ActionCreate
			out.Create++
		}
		if row.TargetExists {
			cr.Existing = &PriceSet{PurchasePrice: textPtr(row.TargetPurchasePrice), SaleToDistributorPrice: textPtr(row.TargetSaleToDistributorPrice), RecommendedSalePrice: textPtr(row.TargetRecommendedSalePrice)}
		}
		if cr.Action != ActionSkip {
			writes = append(writes, write{productID: row.ProductID, set: conv})
		}
		out.Rows = append(out.Rows, cr)
	}
	out.Total = len(rows)
	if in.DryRun || len(writes) == 0 {
		out.Applied = !in.DryRun
		return out, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ConversionPreview{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	for _, w := range writes {
		if err := q.UpsertConvertedPrice(ctx, db.UpsertConvertedPriceParams{
			ProductID: w.productID, BrandID: b.ID, Currency: target,
			PurchasePrice: numeric(w.set.PurchasePrice), SaleToDistributorPrice: numeric(w.set.SaleToDistributorPrice),
			RecommendedSalePrice: numeric(w.set.RecommendedSalePrice),
		}); err != nil {
			return ConversionPreview{}, fmt.Errorf("brand currency: write price: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ConversionPreview{}, err
	}
	out.Applied = true
	return out, nil
}

func textPtr(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	v, err := n.Value()
	if err != nil {
		return nil
	}
	str, ok := v.(string)
	if !ok {
		return nil
	}
	return &str
}

// convert is amount × rate rounded to cents (nil stays nil).
func convert(amount *string, rate *big.Rat) *string {
	if amount == nil {
		return nil
	}
	a, ok := new(big.Rat).SetString(*amount)
	if !ok {
		return nil
	}
	v := new(big.Rat).Mul(a, rate).FloatString(2)
	return &v
}

func numeric(v *string) pgtype.Numeric {
	var n pgtype.Numeric
	if v == nil {
		return n
	}
	_ = n.Scan(*v)
	return n
}

// SessionCurrencies is the currency block of /auth/me.
type SessionCurrencies struct {
	Brand        string
	Organization string
	Display      string
}

// SessionCurrencies resolves the brand currency (active organization's
// brand, else brandID), the organization ledger currency and the local
// display currency (TEC-1214).
func (s *Service) SessionCurrencies(ctx context.Context, orgUUID *uuid.UUID, brandID *int64) (SessionCurrencies, error) {
	p := db.GetSessionCurrenciesParams{}
	if orgUUID != nil {
		p.OrgUuid = pgtype.UUID{Bytes: *orgUUID, Valid: true}
	}
	if brandID != nil {
		p.BrandID = pgtype.Int8{Int64: *brandID, Valid: true}
	}
	row, err := s.q.GetSessionCurrencies(ctx, p)
	if err != nil {
		return SessionCurrencies{}, fmt.Errorf("brand currency: session: %w", err)
	}
	return SessionCurrencies{
		Brand: strings.TrimSpace(row.BrandCurrency), Organization: strings.TrimSpace(row.OrganizationCurrency),
		Display: s.localCurrency(ctx),
	}, nil
}
