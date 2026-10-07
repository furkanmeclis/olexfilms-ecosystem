// Package usecase implements the dealer-side product sale and external
// purchase rules of F3-07d (TEC-344).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	OrgDealer = "dealer"

	SourceProductSale = "product_sale"
	SourcePurchase    = "purchase_external"

	CategoryProductSale = "product_sale"
	CategoryPurchase    = "purchase_external"

	PaymentCash         = "cash"
	PaymentCard         = "card"
	PaymentBankTransfer = "bank_transfer"
	PaymentCari         = "cari"
)

var (
	ErrForbidden           = errors.New("dealer accounting: forbidden")
	ErrProductNotFound     = errors.New("dealer accounting: product not found")
	ErrCustomerNotFound    = errors.New("dealer accounting: customer not found")
	ErrSupplierNotFound    = errors.New("dealer accounting: supplier not found")
	ErrSaleNotFound        = errors.New("dealer accounting: sale not found")
	ErrPurchaseNotFound    = errors.New("dealer accounting: purchase not found")
	ErrStockUnavailable    = errors.New("dealer accounting: stock unavailable")
	ErrSaleAlreadyVoided   = errors.New("dealer accounting: sale already voided")
	ErrVoidWindowExpired   = errors.New("dealer accounting: void window expired")
	ErrIdempotencyConflict = errors.New("dealer accounting: idempotency conflict")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

type Caller struct {
	UserID int64
	Org    orgctx.Scope
	Filter scopefilter.Filter
}

func (c Caller) actor() *int64 {
	if c.UserID == 0 {
		return nil
	}
	id := c.UserID
	return &id
}

type Service struct {
	pool     TxBeginner
	q        *db.Queries
	stock    *ledger.Ledger
	poster   *posting.Poster
	features FeatureChecker
	now      func() time.Time
}

func New(pool TxBeginner, q *db.Queries, stock *ledger.Ledger, poster *posting.Poster, checker FeatureChecker) *Service {
	return &Service{pool: pool, q: q, stock: stock, poster: poster, features: checker, now: time.Now}
}

func (s *Service) activeDealer(ctx context.Context, c Caller) (db.Organization, error) {
	if c.Org.InternalID == 0 || !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) {
		return db.Organization{}, ErrForbidden
	}
	o, err := s.q.GetOrganizationByID(ctx, c.Org.InternalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrForbidden
	}
	if err != nil {
		return db.Organization{}, fmt.Errorf("dealer accounting: organization: %w", err)
	}
	if o.Type != OrgDealer {
		return db.Organization{}, ErrForbidden
	}
	if s.features != nil {
		on, err := s.features.Enabled(ctx, o.ID, features.ModuleDealerAccounting)
		if err != nil {
			return db.Organization{}, fmt.Errorf("dealer accounting: feature: %w", err)
		}
		if !on {
			return db.Organization{}, ErrForbidden
		}
	}
	return o, nil
}

type DealerPriceInput struct {
	ProductUUID uuid.UUID
	SalePrice   string
}

type DealerPriceView struct {
	ProductUUID          uuid.UUID `json:"product_uuid"`
	SalePrice            string    `json:"sale_price"`
	Currency             string    `json:"currency"`
	RecommendedSalePrice *string   `json:"recommended_sale_price,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (s *Service) ListDealerPrices(ctx context.Context, c Caller) ([]DealerPriceView, error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListDealerProductPrices(ctx, db.ListDealerProductPricesParams{OrganizationID: o.ID})
	if err != nil {
		return nil, fmt.Errorf("dealer accounting: prices: %w", err)
	}
	out := make([]DealerPriceView, 0, len(rows))
	for _, r := range rows {
		p, err := s.q.GetProduct(ctx, db.GetProductParams{ID: r.ProductID, BrandID: o.BrandID})
		if err != nil {
			return nil, fmt.Errorf("dealer accounting: product: %w", err)
		}
		out = append(out, s.priceView(ctx, o, p.Uuid, r.ProductID, r.SalePrice, r.Currency, r.UpdatedAt.Time))
	}
	return out, nil
}

func (s *Service) SetDealerPrice(ctx context.Context, c Caller, in DealerPriceInput) (DealerPriceView, error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return DealerPriceView{}, err
	}
	product, err := s.q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: in.ProductUUID, BrandID: o.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DealerPriceView{}, ErrProductNotFound
	}
	if err != nil {
		return DealerPriceView{}, fmt.Errorf("dealer accounting: product: %w", err)
	}
	price, err := normalizeMoney("sale_price", in.SalePrice)
	if err != nil {
		return DealerPriceView{}, err
	}
	n, err := numeric(price)
	if err != nil {
		return DealerPriceView{}, err
	}
	row, err := s.q.UpsertDealerProductPrice(ctx, db.UpsertDealerProductPriceParams{
		OrganizationID: o.ID, BrandID: o.BrandID, ProductID: product.ID,
		SalePrice: n, Currency: o.Currency, UpdatedByUserID: i8p(c.actor()),
	})
	if err != nil {
		return DealerPriceView{}, fmt.Errorf("dealer accounting: set price: %w", err)
	}
	return s.priceView(ctx, o, product.Uuid, product.ID, row.SalePrice, row.Currency, row.UpdatedAt.Time), nil
}

func (s *Service) priceView(ctx context.Context, o db.Organization, productUUID uuid.UUID, productID int64, price pgtype.Numeric, currency string, updated time.Time) DealerPriceView {
	v := DealerPriceView{ProductUUID: productUUID, SalePrice: format(price, 2), Currency: currency, UpdatedAt: updated}
	rec, err := s.q.GetRecommendedProductPrice(ctx, db.GetRecommendedProductPriceParams{BrandID: o.BrandID, ProductID: productID, Currency: o.Currency})
	if err == nil {
		x := format(rec, 2)
		v.RecommendedSalePrice = &x
	}
	return v
}

type SupplierInput struct {
	Name      string
	TaxNo     *string
	PhoneE164 *string
	Email     *string
	Note      string
	Active    *bool
}

type SupplierView struct {
	UUID      uuid.UUID `json:"uuid"`
	Name      string    `json:"name"`
	TaxNo     *string   `json:"tax_no,omitempty"`
	PhoneE164 *string   `json:"phone_e164,omitempty"`
	Email     *string   `json:"email,omitempty"`
	Note      string    `json:"note"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Service) CreateSupplier(ctx context.Context, c Caller, in SupplierInput) (SupplierView, error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return SupplierView{}, err
	}
	name, err := requiredText("name", in.Name, 200)
	if err != nil {
		return SupplierView{}, err
	}
	if err := validateSupplierContacts(in); err != nil {
		return SupplierView{}, err
	}
	row, err := s.q.CreateSupplier(ctx, db.CreateSupplierParams{
		OrganizationID: o.ID, BrandID: o.BrandID, Name: name,
		TaxNo: textPtr(in.TaxNo), PhoneE164: textPtr(in.PhoneE164), Email: textPtr(in.Email), Note: strings.TrimSpace(in.Note),
	})
	if err != nil {
		return SupplierView{}, fmt.Errorf("dealer accounting: create supplier: %w", err)
	}
	return supplierView(row), nil
}

func (s *Service) UpdateSupplier(ctx context.Context, c Caller, id uuid.UUID, in SupplierInput) (SupplierView, error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return SupplierView{}, err
	}
	cur, err := s.q.GetSupplierByUUID(ctx, db.GetSupplierByUUIDParams{Uuid: id, OrganizationID: o.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierView{}, ErrSupplierNotFound
	}
	if err != nil {
		return SupplierView{}, fmt.Errorf("dealer accounting: supplier: %w", err)
	}
	name, err := requiredText("name", in.Name, 200)
	if err != nil {
		return SupplierView{}, err
	}
	if err := validateSupplierContacts(in); err != nil {
		return SupplierView{}, err
	}
	active := cur.Active
	if in.Active != nil {
		active = *in.Active
	}
	row, err := s.q.UpdateSupplier(ctx, db.UpdateSupplierParams{
		ID: cur.ID, OrganizationID: o.ID, Name: name, TaxNo: textPtr(in.TaxNo),
		PhoneE164: textPtr(in.PhoneE164), Email: textPtr(in.Email), Note: strings.TrimSpace(in.Note), Active: active,
	})
	if err != nil {
		return SupplierView{}, fmt.Errorf("dealer accounting: update supplier: %w", err)
	}
	return supplierView(row), nil
}

func supplierView(r db.Supplier) SupplierView {
	return SupplierView{
		UUID: r.Uuid, Name: r.Name, TaxNo: strp(r.TaxNo), PhoneE164: strp(r.PhoneE164),
		Email: strp(r.Email), Note: r.Note, Active: r.Active, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

type SaleLineInput struct {
	ProductUUID *uuid.UUID
	Barcode     string
	Quantity    int32
	UnitPrice   string
}

type ProductSaleInput struct {
	CustomerUUID  *uuid.UUID
	PaymentMethod string
	Note          string
	Lines         []SaleLineInput
}

type SaleLineView struct {
	ProductUUID      uuid.UUID  `json:"product_uuid"`
	UnitUUID         *uuid.UUID `json:"unit_uuid,omitempty"`
	Barcode          *string    `json:"barcode,omitempty"`
	Quantity         string     `json:"quantity"`
	UnitPrice        string     `json:"unit_price"`
	LineTotal        string     `json:"line_total"`
	PurchaseUnitCost *string    `json:"purchase_unit_cost,omitempty"`
	Profit           *string    `json:"profit,omitempty"`
}

type ProductSaleView struct {
	UUID          uuid.UUID      `json:"uuid"`
	CustomerUUID  *uuid.UUID     `json:"customer_uuid,omitempty"`
	PaymentMethod string         `json:"payment_method"`
	Currency      string         `json:"currency"`
	Total         string         `json:"total"`
	Profit        string         `json:"profit"`
	SoldAt        time.Time      `json:"sold_at"`
	Voided        bool           `json:"voided"`
	Lines         []SaleLineView `json:"lines"`
}

func (s *Service) CreateProductSale(ctx context.Context, c Caller, in ProductSaleInput) (ProductSaleView, error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return ProductSaleView{}, err
	}
	if len(in.Lines) == 0 {
		return ProductSaleView{}, invalid("lines", "must contain at least one line")
	}
	if len(in.Lines) > 100 {
		return ProductSaleView{}, invalid("lines", "must contain at most 100 lines")
	}
	pm := strings.TrimSpace(in.PaymentMethod)
	if pm != PaymentCash && pm != PaymentCard && pm != PaymentCari {
		return ProductSaleView{}, invalid("payment_method", "must be cash, card or cari")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ProductSaleView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	customerID := pgtype.Int8{}
	cariID := int64(0)
	if in.CustomerUUID != nil {
		u, err := q.GetServedCustomerByUUID(ctx, db.GetServedCustomerByUUIDParams{Uuid: *in.CustomerUUID, OrganizationID: o.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ProductSaleView{}, ErrCustomerNotFound
		}
		if err != nil {
			return ProductSaleView{}, fmt.Errorf("dealer accounting: customer: %w", err)
		}
		customerID = pgtype.Int8{Int64: u.ID, Valid: true}
		if pm == PaymentCari {
			cari, err := ensureCustomerCari(ctx, q, o, u.ID)
			if err != nil {
				return ProductSaleView{}, err
			}
			cariID = cari.ID
		}
	} else if pm == PaymentCari {
		return ProductSaleView{}, invalid("customer_uuid", "is required for cari sales")
	}

	prepared := make([]preparedSaleLine, 0, len(in.Lines))
	totals := []string{}
	for i, line := range in.Lines {
		p, err := s.prepareSaleLine(ctx, q, o, i, line)
		if err != nil {
			return ProductSaleView{}, err
		}
		prepared = append(prepared, p)
		totals = append(totals, p.LineTotal)
	}
	total, err := sum(totals)
	if err != nil {
		return ProductSaleView{}, err
	}
	totalNum, err := numeric(total)
	if err != nil {
		return ProductSaleView{}, err
	}
	sale, err := q.CreateProductSale(ctx, db.CreateProductSaleParams{
		OrganizationID: o.ID, BrandID: o.BrandID, CustomerUserID: customerID, PaymentMethod: pm,
		CariID: i8(cariID), Currency: o.Currency, Total: totalNum,
		SoldAt: pgtype.Timestamptz{Time: s.now(), Valid: true}, Note: trimMax(in.Note, 5000),
		CreatedByUserID: i8p(c.actor()),
	})
	if err != nil {
		return ProductSaleView{}, fmt.Errorf("dealer accounting: create sale: %w", err)
	}
	accountID, err := s.accountForPayment(ctx, q, o, pm)
	if err != nil {
		return ProductSaleView{}, err
	}
	post := posting.Entry{
		OrganizationID: o.ID, Source: posting.Source{Type: SourceProductSale, UUID: sale.Uuid},
		Category: CategoryProductSale, Amount: total, Currency: o.Currency, AccountID: accountID,
		CariID: cariID, Description: "Product sale", ActorUserID: c.actor(), PostedAt: sale.SoldAt.Time,
	}
	res, err := s.poster.PostIncome(ctx, tx, post)
	if err != nil {
		return ProductSaleView{}, mapPostingErr(err)
	}
	if _, err := q.SetProductSaleFinanceEntry(ctx, db.SetProductSaleFinanceEntryParams{
		ID: sale.ID, OrganizationID: o.ID, FinanceEntryID: i8(res.Entry.ID),
	}); err != nil {
		return ProductSaleView{}, fmt.Errorf("dealer accounting: link sale entry: %w", err)
	}

	for i, p := range prepared {
		mv, err := s.postSaleStock(ctx, tx, sale, p, c)
		if err != nil {
			return ProductSaleView{}, err
		}
		if _, err := q.CreateProductSaleLine(ctx, db.CreateProductSaleLineParams{
			SaleID: sale.ID, OrganizationID: o.ID, BrandID: o.BrandID, ProductID: p.ProductID,
			UnitID: i8(p.UnitID), Quantity: mustNumeric(p.Quantity), UnitPrice: mustNumeric(p.UnitPrice),
			LineTotal: mustNumeric(p.LineTotal), PurchaseUnitCost: numericNull(p.PurchaseUnitCost),
			StockMovementID: i8(mv.ID), SortOrder: int32(i),
		}); err != nil {
			return ProductSaleView{}, fmt.Errorf("dealer accounting: create sale line: %w", err)
		}
		if _, err := q.UpsertDealerProductPrice(ctx, db.UpsertDealerProductPriceParams{
			OrganizationID: o.ID, BrandID: o.BrandID, ProductID: p.ProductID,
			SalePrice: mustNumeric(p.UnitPrice), Currency: o.Currency, UpdatedByUserID: i8p(c.actor()),
		}); err != nil {
			return ProductSaleView{}, fmt.Errorf("dealer accounting: remember sale price: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ProductSaleView{}, err
	}
	return s.GetProductSale(ctx, c, sale.Uuid)
}

func (s *Service) VoidProductSale(ctx context.Context, c Caller, id uuid.UUID, reason string) (ProductSaleView, error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return ProductSaleView{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ProductSaleView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	sale, err := q.GetProductSaleByUUID(ctx, db.GetProductSaleByUUIDParams{Uuid: id, OrganizationID: o.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ProductSaleView{}, ErrSaleNotFound
	}
	if err != nil {
		return ProductSaleView{}, fmt.Errorf("dealer accounting: sale: %w", err)
	}
	if !sameLocalDay(sale.SoldAt.Time, s.now(), o.Timezone) {
		return ProductSaleView{}, ErrVoidWindowExpired
	}
	lines, err := q.ListProductSaleLines(ctx, db.ListProductSaleLinesParams{SaleID: sale.ID, OrganizationID: o.ID})
	if err != nil {
		return ProductSaleView{}, fmt.Errorf("dealer accounting: sale lines: %w", err)
	}
	if n, err := q.CountOpenFinanceEntriesBySource(ctx, db.CountOpenFinanceEntriesBySourceParams{
		SourceType: SourceProductSale, SourceUuid: pgUUID(sale.Uuid),
	}); err != nil {
		return ProductSaleView{}, err
	} else if n == 0 {
		return ProductSaleView{}, ErrSaleAlreadyVoided
	}
	if _, err := s.poster.VoidBySourceTx(ctx, tx, posting.Source{Type: SourceProductSale, UUID: sale.Uuid}, trimMax(reason, 500), c.actor()); err != nil {
		return ProductSaleView{}, mapPostingErr(err)
	}
	for _, line := range lines {
		if !line.StockMovementID.Valid || !line.UnitID.Valid {
			continue
		}
		unit, err := q.GetUnit(ctx, line.UnitID.Int64)
		if err != nil {
			return ProductSaleView{}, fmt.Errorf("dealer accounting: unit: %w", err)
		}
		m := ledger.Movement{
			Type: ledger.TypeReturn, UnitID: unit.ID, Source: "product_sale", RefType: "product_sale_line", RefID: sale.ID*1000 + int64(line.SortOrder),
			To:     &ledger.Owner{Type: ledger.OwnerOrganization, ID: o.ID, OrgID: o.ID},
			Reason: trimMax(reason, 500), ActorUserID: c.actor(),
			Metadata: map[string]any{"sale_uuid": sale.Uuid.String(), "line_uuid": line.Uuid.String()},
		}
		if unit.UnitKind == ledger.KindFixed {
			m.Quantity = int32FromNumeric(line.Quantity)
		} else {
			m.From = &ledger.Owner{Type: ledger.OwnerTrash, ID: o.ID, OrgID: o.ID}
		}
		if _, err := s.stock.Post(ctx, tx, m); err != nil {
			return ProductSaleView{}, fmt.Errorf("%w: %v", ErrStockUnavailable, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ProductSaleView{}, err
	}
	return s.GetProductSale(ctx, c, id)
}

func (s *Service) GetProductSale(ctx context.Context, c Caller, id uuid.UUID) (ProductSaleView, error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return ProductSaleView{}, err
	}
	sale, err := s.q.GetProductSaleByUUID(ctx, db.GetProductSaleByUUIDParams{Uuid: id, OrganizationID: o.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ProductSaleView{}, ErrSaleNotFound
	}
	if err != nil {
		return ProductSaleView{}, fmt.Errorf("dealer accounting: sale: %w", err)
	}
	lines, err := s.q.ListProductSaleLines(ctx, db.ListProductSaleLinesParams{SaleID: sale.ID, OrganizationID: o.ID})
	if err != nil {
		return ProductSaleView{}, fmt.Errorf("dealer accounting: lines: %w", err)
	}
	return s.saleView(ctx, o, sale, lines)
}

type preparedSaleLine struct {
	ProductID        int64
	Index            int
	UnitID           int64
	UnitUUID         *uuid.UUID
	Barcode          *string
	Quantity         string
	QuantityInt      int32
	UnitPrice        string
	LineTotal        string
	PurchaseUnitCost *string
	Owner            ledger.Owner
	Fixed            bool
}

func (s *Service) prepareSaleLine(ctx context.Context, q *db.Queries, o db.Organization, i int, in SaleLineInput) (preparedSaleLine, error) {
	field := fmt.Sprintf("lines.%d", i)
	price, err := normalizeMoney(field+".unit_price", in.UnitPrice)
	if err != nil {
		return preparedSaleLine{}, err
	}
	var productID pgtype.Int8
	if in.ProductUUID != nil {
		p, err := q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: *in.ProductUUID, BrandID: o.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return preparedSaleLine{}, ErrProductNotFound
		}
		if err != nil {
			return preparedSaleLine{}, fmt.Errorf("dealer accounting: product: %w", err)
		}
		productID = pgtype.Int8{Int64: p.ID, Valid: true}
	}
	barcode := strings.TrimSpace(in.Barcode)
	if barcode == "" && !productID.Valid {
		return preparedSaleLine{}, invalid(field+".product_uuid", "or barcode is required")
	}
	rows, err := q.ListProductSaleStockCandidates(ctx, db.ListProductSaleStockCandidatesParams{
		OrganizationID: o.ID, BrandID: o.BrandID, ProductID: productID, Barcode: text(barcode),
	})
	if err != nil {
		return preparedSaleLine{}, fmt.Errorf("dealer accounting: stock candidates: %w", err)
	}
	qty := in.Quantity
	if qty == 0 {
		qty = 1
	}
	if qty < 0 {
		return preparedSaleLine{}, invalid(field+".quantity", "must be positive")
	}
	for _, r := range rows {
		if r.UnitKind != ledger.KindFixed && qty != 1 {
			continue
		}
		if r.QuantityOnHand < qty {
			continue
		}
		lineTotal, err := multiply(price, big.NewRat(int64(qty), 1))
		if err != nil {
			return preparedSaleLine{}, err
		}
		cost := s.purchaseCost(ctx, q, o, r.UnitID, r.ProductID)
		uu := r.UnitUuid
		bc := r.Barcode
		return preparedSaleLine{
			ProductID: r.ProductID, Index: i, UnitID: r.UnitID, UnitUUID: &uu, Barcode: &bc,
			Quantity: new(big.Rat).SetInt64(int64(qty)).FloatString(2), QuantityInt: qty,
			UnitPrice: price, LineTotal: lineTotal, PurchaseUnitCost: cost,
			Owner: ledger.Owner{Type: ledger.OwnerType(r.OwnerType), ID: r.OwnerID, OrgID: r.HolderOrgID},
			Fixed: r.UnitKind == ledger.KindFixed,
		}, nil
	}
	return preparedSaleLine{}, ErrStockUnavailable
}

func (s *Service) postSaleStock(ctx context.Context, tx pgx.Tx, sale db.ProductSale, p preparedSaleLine, c Caller) (db.StockMovement, error) {
	m := ledger.Movement{
		Type: ledger.TypeSale, UnitID: p.UnitID, Source: "product_sale", RefType: "product_sale_line", RefID: sale.ID*1000 + int64(p.Index),
		Reason: "product sale", ActorUserID: c.actor(), Metadata: map[string]any{"sale_uuid": sale.Uuid.String()},
	}
	if p.Fixed {
		m.From, m.Quantity = &p.Owner, p.QuantityInt
	} else {
		m.From = &p.Owner
	}
	res, err := s.stock.Post(ctx, tx, m)
	if err != nil {
		return db.StockMovement{}, fmt.Errorf("%w: %v", ErrStockUnavailable, err)
	}
	return res.Movement, nil
}

type PurchaseInput struct {
	SupplierUUID  uuid.UUID
	Amount        string
	PaymentMethod string
	Note          string
	Description   string
}

type PurchaseView struct {
	UUID          uuid.UUID `json:"uuid"`
	SupplierUUID  uuid.UUID `json:"supplier_uuid"`
	Amount        string    `json:"amount"`
	Currency      string    `json:"currency"`
	PaymentMethod string    `json:"payment_method"`
	PurchasedOn   string    `json:"purchased_on"`
	Note          string    `json:"note"`
}

func (s *Service) CreatePurchase(ctx context.Context, c Caller, in PurchaseInput) (PurchaseView, error) {
	o, err := s.activeDealer(ctx, c)
	if err != nil {
		return PurchaseView{}, err
	}
	pm := strings.TrimSpace(in.PaymentMethod)
	if pm != PaymentCash && pm != PaymentCard && pm != PaymentBankTransfer && pm != PaymentCari {
		return PurchaseView{}, invalid("payment_method", "must be cash, card, bank_transfer or cari")
	}
	amount, err := normalizePositiveMoney("amount", in.Amount)
	if err != nil {
		return PurchaseView{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PurchaseView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	sup, err := q.GetSupplierByUUID(ctx, db.GetSupplierByUUIDParams{Uuid: in.SupplierUUID, OrganizationID: o.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseView{}, ErrSupplierNotFound
	}
	if err != nil {
		return PurchaseView{}, fmt.Errorf("dealer accounting: supplier: %w", err)
	}
	row, err := q.CreatePurchase(ctx, db.CreatePurchaseParams{
		OrganizationID: o.ID, BrandID: o.BrandID, SupplierID: sup.ID,
		PurchasedOn: pgtype.Date{Time: s.now(), Valid: true}, Currency: o.Currency,
		Amount: mustNumeric(amount), PaymentMethod: pm, Note: trimMax(in.Note, 5000), CreatedByUserID: i8p(c.actor()),
	})
	if err != nil {
		return PurchaseView{}, fmt.Errorf("dealer accounting: purchase: %w", err)
	}
	desc := strings.TrimSpace(in.Description)
	if desc == "" {
		desc = "External purchase"
	}
	if _, err := q.CreatePurchaseLine(ctx, db.CreatePurchaseLineParams{
		PurchaseID: row.ID, OrganizationID: o.ID, BrandID: o.BrandID, Description: trimMax(desc, 1000),
		Quantity: mustNumeric("1"), UnitPrice: mustNumeric(amount), LineTotal: mustNumeric(amount),
	}); err != nil {
		return PurchaseView{}, fmt.Errorf("dealer accounting: purchase line: %w", err)
	}
	accountID, err := s.accountForPayment(ctx, q, o, pm)
	if err != nil {
		return PurchaseView{}, err
	}
	res, err := s.poster.PostExpense(ctx, tx, posting.Entry{
		OrganizationID: o.ID, Source: posting.Source{Type: SourcePurchase, UUID: row.Uuid},
		Category: CategoryPurchase, Amount: amount, Currency: o.Currency, AccountID: accountID,
		Description: "External purchase", ActorUserID: c.actor(), PostedAt: row.CreatedAt.Time,
	})
	if err != nil {
		return PurchaseView{}, mapPostingErr(err)
	}
	if _, err := q.SetPurchaseFinanceEntry(ctx, db.SetPurchaseFinanceEntryParams{ID: row.ID, OrganizationID: o.ID, FinanceEntryID: i8(res.Entry.ID)}); err != nil {
		return PurchaseView{}, fmt.Errorf("dealer accounting: link purchase entry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseView{}, err
	}
	return PurchaseView{UUID: row.Uuid, SupplierUUID: sup.Uuid, Amount: format(row.Amount, 2), Currency: row.Currency, PaymentMethod: row.PaymentMethod, PurchasedOn: row.PurchasedOn.Time.Format(time.DateOnly), Note: row.Note}, nil
}

func (s *Service) purchaseCost(ctx context.Context, q *db.Queries, o db.Organization, unitID, productID int64) *string {
	parent, err := q.SupplierOf(ctx, o.ID)
	if err == nil {
		if op, err := q.GetUnitLastOrderPrice(ctx, db.GetUnitLastOrderPriceParams{UnitID: unitID, SellerOrgID: parent.ID, BuyerOrgID: o.ID, Currency: o.Currency}); err == nil {
			v := format(op.UnitPrice, 2)
			return &v
		}
	}
	prices, err := usecase.New(q).BuyerPurchasePrices(ctx, o.ID, o.Type, o.BrandID, []int64{productID}, o.Currency)
	if err == nil {
		if p, ok := prices[productID]; ok {
			return &p.Price
		}
	}
	return nil
}

func (s *Service) accountForPayment(ctx context.Context, q *db.Queries, o db.Organization, method string) (int64, error) {
	if method == PaymentCari {
		return 0, nil
	}
	typ := "cash"
	name := "Product sales cash"
	if method == PaymentCard || method == PaymentBankTransfer {
		typ, name = "bank", "Product sales bank"
	}
	rows, err := q.ListFinanceAccounts(ctx, db.ListFinanceAccountsParams{OrganizationID: o.ID, Active: pgtype.Bool{Bool: true, Valid: true}})
	if err != nil {
		return 0, fmt.Errorf("dealer accounting: accounts: %w", err)
	}
	for _, a := range rows {
		if a.Type == typ && a.Name == name {
			return a.ID, nil
		}
	}
	a, err := q.CreateFinanceAccount(ctx, db.CreateFinanceAccountParams{
		OrganizationID: o.ID, BrandID: o.BrandID, Type: typ, Name: name, Currency: o.Currency, Active: true,
	})
	if err != nil {
		return 0, fmt.Errorf("dealer accounting: create account: %w", err)
	}
	return a.ID, nil
}

func (s *Service) saleView(ctx context.Context, o db.Organization, sale db.ProductSale, rows []db.ProductSaleLine) (ProductSaleView, error) {
	out := ProductSaleView{UUID: sale.Uuid, PaymentMethod: sale.PaymentMethod, Currency: sale.Currency, Total: format(sale.Total, 2), SoldAt: sale.SoldAt.Time, Lines: []SaleLineView{}}
	if sale.CustomerUserID.Valid {
		u, err := s.q.GetUserByID(ctx, sale.CustomerUserID.Int64)
		if err == nil {
			out.CustomerUUID = &u.Uuid
		}
	}
	profit := new(big.Rat)
	for _, r := range rows {
		p, err := s.q.GetProduct(ctx, db.GetProductParams{ID: r.ProductID, BrandID: o.BrandID})
		if err != nil {
			return ProductSaleView{}, err
		}
		qty := numericRat(r.Quantity)
		lineProfit := (*string)(nil)
		cost := (*string)(nil)
		if r.PurchaseUnitCost.Valid {
			c := format(r.PurchaseUnitCost, 2)
			cost = &c
			lp := new(big.Rat).Sub(numericRat(r.LineTotal), new(big.Rat).Mul(numericRat(r.PurchaseUnitCost), qty))
			v := lp.FloatString(2)
			lineProfit = &v
			profit.Add(profit, lp)
		} else {
			profit.Add(profit, numericRat(r.LineTotal))
		}
		var unitUUID *uuid.UUID
		var barcode *string
		if r.UnitID.Valid {
			if u, err := s.q.GetUnit(ctx, r.UnitID.Int64); err == nil {
				uu := u.Uuid
				bc := u.Barcode
				unitUUID, barcode = &uu, &bc
			}
		}
		out.Lines = append(out.Lines, SaleLineView{
			ProductUUID: p.Uuid, UnitUUID: unitUUID, Barcode: barcode, Quantity: format(r.Quantity, 2),
			UnitPrice: format(r.UnitPrice, 2), LineTotal: format(r.LineTotal, 2), PurchaseUnitCost: cost, Profit: lineProfit,
		})
	}
	out.Profit = profit.FloatString(2)
	if n, err := s.q.CountOpenFinanceEntriesBySource(ctx, db.CountOpenFinanceEntriesBySourceParams{SourceType: SourceProductSale, SourceUuid: pgUUID(sale.Uuid)}); err == nil {
		out.Voided = n == 0
	}
	return out, nil
}

func ensureCustomerCari(ctx context.Context, q *db.Queries, o db.Organization, userID int64) (db.CariAccount, error) {
	c, err := q.CreateCariForUserIfMissing(ctx, db.CreateCariForUserIfMissingParams{OrganizationID: o.ID, BrandID: o.BrandID, CounterpartyUserID: userID, Currency: o.Currency})
	if errors.Is(err, pgx.ErrNoRows) {
		return q.GetCariAccountByCounterpartyUser(ctx, db.GetCariAccountByCounterpartyUserParams{OrganizationID: o.ID, CounterpartyUserID: userID})
	}
	return c, err
}

func mapPostingErr(err error) error {
	if errors.Is(err, posting.ErrIdempotencyConflict) {
		return ErrIdempotencyConflict
	}
	return fmt.Errorf("dealer accounting: post accounting: %w", err)
}

var moneyRe = regexp.MustCompile(`^[0-9]{1,16}(\.[0-9]{1,2})?$`)
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)
var phoneE164Re = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

func normalizeMoney(field, raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if !moneyRe.MatchString(s) {
		return "", invalid(field, "must be a non-negative decimal with at most 2 fractional digits")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() < 0 {
		return "", invalid(field, "must be a non-negative decimal")
	}
	return r.FloatString(2), nil
}

func normalizePositiveMoney(field, raw string) (string, error) {
	s, err := normalizeMoney(field, raw)
	if err != nil {
		return "", err
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 {
		return "", invalid(field, "must be greater than zero")
	}
	return s, nil
}

func validateSupplierContacts(in SupplierInput) error {
	if v := strings.TrimSpace(ptrString(in.PhoneE164)); v != "" && !phoneE164Re.MatchString(v) {
		return invalid("phone_e164", "must be E.164")
	}
	if v := strings.TrimSpace(ptrString(in.Email)); v != "" && !emailRe.MatchString(v) {
		return invalid("email", "must be an email address")
	}
	return nil
}

func ptrString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func numeric(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, err
	}
	return n, nil
}

func mustNumeric(s string) pgtype.Numeric {
	n, err := numeric(s)
	if err != nil {
		panic(err)
	}
	return n
}

func numericNull(v *string) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{}
	}
	return mustNumeric(*v)
}

func numericRat(n pgtype.Numeric) *big.Rat {
	r, ok := new(big.Rat).SetString(posting.FormatNumeric(n))
	if !ok {
		return new(big.Rat)
	}
	return r
}

func format(n pgtype.Numeric, decimals int) string { return numericRat(n).FloatString(decimals) }

func multiply(price string, qty *big.Rat) (string, error) {
	p, ok := new(big.Rat).SetString(price)
	if !ok {
		return "", invalid("unit_price", "must be a decimal")
	}
	return new(big.Rat).Mul(p, qty).FloatString(2), nil
}

func sum(values []string) (string, error) {
	total := new(big.Rat)
	for _, v := range values {
		r, ok := new(big.Rat).SetString(v)
		if !ok {
			return "", fmt.Errorf("invalid decimal %q", v)
		}
		total.Add(total, r)
	}
	return total.FloatString(2), nil
}

func requiredText(field, raw string, max int) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", invalid(field, "is required")
	}
	if len([]rune(s)) > max {
		return "", invalid(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return s, nil
}

func trimMax(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

func text(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func textPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return text(*s)
}

func strp(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func i8(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func i8p(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func int32FromNumeric(n pgtype.Numeric) int32 {
	r := numericRat(n)
	if !r.IsInt() {
		return 0
	}
	return int32(r.Num().Int64())
}

func sameLocalDay(a, b time.Time, tz string) bool {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	ay, am, ad := a.In(loc).Date()
	by, bm, bd := b.In(loc).Date()
	return ay == by && am == bm && ad == bd
}
