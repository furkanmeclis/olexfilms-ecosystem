package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	brandID       = int64(1)
	centerID      = int64(10)
	distributorID = int64(20)
	otherDistID   = int64(21)
	dealerID      = int64(30)
	productID     = int64(100)
)

var productUUID = uuid.MustParse("00000000-0000-0000-0000-000000000100")

type key struct {
	product, org int64
	cur          string
}

type fakeQ struct {
	Querier   // unused methods panic
	prices    map[key]db.ProductPrice
	overrides map[key]string
	dealer    map[key]string
	orgs      map[int64]db.Organization
}

func newFake() *fakeQ {
	return &fakeQ{
		prices: map[key]db.ProductPrice{}, overrides: map[key]string{}, dealer: map[key]string{},
		orgs: map[int64]db.Organization{
			centerID:      {ID: centerID, Type: OrgCenter, BrandID: brandID},
			distributorID: {ID: distributorID, Uuid: uuid.New(), Type: OrgDistributor, BrandID: brandID, ParentID: pgtype.Int8{Int64: centerID, Valid: true}},
			otherDistID:   {ID: otherDistID, Uuid: uuid.New(), Type: OrgDistributor, BrandID: brandID, ParentID: pgtype.Int8{Int64: centerID, Valid: true}},
			dealerID:      {ID: dealerID, Type: OrgDealer, BrandID: brandID, ParentID: pgtype.Int8{Int64: distributorID, Valid: true}},
		},
	}
}

func num(s string) pgtype.Numeric {
	if s == "" {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		panic(err)
	}
	return n
}

func textNum(t pgtype.Text) pgtype.Numeric {
	if !t.Valid {
		return pgtype.Numeric{}
	}
	return num(t.String)
}

func (f *fakeQ) GetProductByUUID(_ context.Context, arg db.GetProductByUUIDParams) (db.Product, error) {
	if arg.Uuid != productUUID || arg.BrandID != brandID {
		return db.Product{}, pgx.ErrNoRows
	}
	return db.Product{ID: productID, Uuid: productUUID, BrandID: brandID, Sku: "PPF-1", Name: "Film"}, nil
}

// TEC-506: the recommended block reads the viewer organization and finds
// no recommended price in the fake.
func (f *fakeQ) GetOrganizationByID(_ context.Context, id int64) (db.Organization, error) {
	if o, ok := f.orgs[id]; ok {
		return o, nil
	}
	return db.Organization{ID: id}, nil
}

func (f *fakeQ) ListApplicableRecommendedPrices(context.Context, db.ListApplicableRecommendedPricesParams) ([]db.ListApplicableRecommendedPricesRow, error) {
	return nil, nil
}

func (f *fakeQ) ListDealerProductPrices(context.Context, db.ListDealerProductPricesParams) ([]db.DealerProductPrice, error) {
	return nil, nil
}

func (f *fakeQ) GetOrganizationByUUID(_ context.Context, id uuid.UUID) (db.Organization, error) {
	for _, o := range f.orgs {
		if o.Uuid == id && id != uuid.Nil {
			return o, nil
		}
	}
	return db.Organization{}, pgx.ErrNoRows
}

func (f *fakeQ) SupplierOf(_ context.Context, id int64) (db.Organization, error) {
	o := f.orgs[id]
	if !o.ParentID.Valid {
		return db.Organization{}, pgx.ErrNoRows
	}
	return f.orgs[o.ParentID.Int64], nil
}

func (f *fakeQ) ListProductPricesForProducts(_ context.Context, arg db.ListProductPricesForProductsParams) ([]db.ListProductPricesForProductsRow, error) {
	out := []db.ListProductPricesForProductsRow{}
	for k, p := range f.prices {
		out = append(out, db.ListProductPricesForProductsRow{
			ProductID: k.product, Currency: k.cur, PurchasePrice: p.PurchasePrice,
			SaleToDistributorPrice: p.SaleToDistributorPrice, RecommendedSalePrice: p.RecommendedSalePrice,
		})
	}
	return out, nil
}

func (f *fakeQ) ListDistributorOverridesForProducts(_ context.Context, arg db.ListDistributorOverridesForProductsParams) ([]db.ListDistributorOverridesForProductsRow, error) {
	out := []db.ListDistributorOverridesForProductsRow{}
	for k, p := range f.overrides {
		if k.org == arg.DistributorOrgID {
			out = append(out, db.ListDistributorOverridesForProductsRow{ProductID: k.product, Currency: k.cur, Price: p})
		}
	}
	return out, nil
}

func (f *fakeQ) ListDealerPricesForProducts(_ context.Context, arg db.ListDealerPricesForProductsParams) ([]db.ListDealerPricesForProductsRow, error) {
	out := []db.ListDealerPricesForProductsRow{}
	for k, p := range f.dealer {
		if k.org == arg.DistributorOrgID {
			out = append(out, db.ListDealerPricesForProductsRow{ProductID: k.product, Currency: k.cur, Price: p})
		}
	}
	return out, nil
}

func (f *fakeQ) GetProductPrice(_ context.Context, arg db.GetProductPriceParams) (db.ProductPrice, error) {
	p, ok := f.prices[key{arg.ProductID, 0, arg.Currency}]
	if !ok {
		return db.ProductPrice{}, pgx.ErrNoRows
	}
	return p, nil
}

func (f *fakeQ) UpsertProductPrice(_ context.Context, arg db.UpsertProductPriceParams) (db.ProductPrice, error) {
	p := db.ProductPrice{
		ProductID: arg.ProductID, BrandID: arg.BrandID, Currency: arg.Currency,
		PurchasePrice: textNum(arg.PurchasePrice), SaleToDistributorPrice: textNum(arg.SaleToDistributorPrice),
		RecommendedSalePrice: textNum(arg.RecommendedSalePrice),
	}
	f.prices[key{arg.ProductID, 0, arg.Currency}] = p
	return p, nil
}

func (f *fakeQ) UpsertDistributorPriceOverride(_ context.Context, arg db.UpsertDistributorPriceOverrideParams) (db.UpsertDistributorPriceOverrideRow, error) {
	f.overrides[key{arg.ProductID, arg.DistributorOrgID, arg.Currency}] = arg.Price
	return db.UpsertDistributorPriceOverrideRow{Currency: arg.Currency, Price: arg.Price}, nil
}

func (f *fakeQ) UpsertDistributorDealerPrice(_ context.Context, arg db.UpsertDistributorDealerPriceParams) (db.UpsertDistributorDealerPriceRow, error) {
	f.dealer[key{arg.ProductID, arg.DistributorOrgID, arg.Currency}] = arg.Price
	return db.UpsertDistributorDealerPriceRow{Currency: arg.Currency, Price: arg.Price}, nil
}

func viewerFor(orgID int64, orgType, role string) Viewer {
	r, ok := rbac.RoleBySlug(role)
	if !ok {
		panic("role " + role)
	}
	p := authctx.Principal{PermissionScopes: map[string]rbac.Scope{}}
	for slug, scope := range rbac.RoleGrants(r) {
		p.PermissionScopes[slug] = scope
		p.Permissions = append(p.Permissions, slug)
	}
	return ViewerFrom(p, orgctx.Scope{InternalID: orgID, OrgType: orgType, BrandID: brandID})
}

func s(v string) *string { return &v }

func seed(t *testing.T, svc *Service, f *fakeQ) {
	t.Helper()
	ctx := context.Background()
	center := viewerFor(centerID, OrgCenter, rbac.RoleCenterAccounting)
	if _, err := svc.SetListPrice(ctx, center, productUUID, "try", ListPriceInput{
		Purchase:          OptionalPrice{Set: true, Value: s("50")},
		SaleToDistributor: OptionalPrice{Set: true, Value: s("80.5")},
		Recommended:       OptionalPrice{Set: true, Value: s("150")},
	}); err != nil {
		t.Fatalf("list price: %v", err)
	}
	if _, err := svc.SetDistributorOverride(ctx, center, productUUID, f.orgs[distributorID].Uuid, "TRY", "70"); err != nil {
		t.Fatalf("override: %v", err)
	}
	dist := viewerFor(distributorID, OrgDistributor, rbac.RoleDistributorOwner)
	if _, err := svc.SetDealerPrice(ctx, dist, productUUID, "TRY", "95"); err != nil {
		t.Fatalf("dealer price: %v", err)
	}
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func eq(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s missing, want %s", name, want)
	}
	g, _ := new(big.Rat).SetString(*got)
	w, _ := new(big.Rat).SetString(want)
	if g == nil || g.Cmp(w) != 0 {
		t.Fatalf("%s = %s, want %s", name, *got, want)
	}
}

func TestEffectiveViews(t *testing.T) {
	f := newFake()
	svc := New(f)
	seed(t, svc, f)
	ctx := context.Background()

	center, err := svc.ProductView(ctx, viewerFor(centerID, OrgCenter, rbac.RoleCenterAccounting), productUUID)
	if err != nil || len(center.Prices) != 1 {
		t.Fatalf("center view: %+v %v", center, err)
	}
	eq(t, "center purchase", center.Prices[0].PurchasePrice, "50")
	eq(t, "center sale", center.Prices[0].SalePrice, "80.5")
	eq(t, "center recommended", center.Prices[0].RecommendedSalePrice, "150")

	// center_staff only holds pricing.recommended.read.
	staff, err := svc.ProductView(ctx, viewerFor(centerID, OrgCenter, rbac.RoleCenterStaff), productUUID)
	if err != nil {
		t.Fatal(err)
	}
	if js := jsonOf(t, staff); strings.Contains(js, "purchase_price") || strings.Contains(js, "\"sale_price\"") {
		t.Fatalf("center staff view leaks: %s", js)
	}

	dist, err := svc.ProductView(ctx, viewerFor(distributorID, OrgDistributor, rbac.RoleDistributorOwner), productUUID)
	if err != nil || len(dist.Prices) != 1 {
		t.Fatalf("distributor view: %+v %v", dist, err)
	}
	eq(t, "distributor purchase", dist.Prices[0].PurchasePrice, "70")
	if dist.Prices[0].PurchasePriceSource != SourceOverride {
		t.Fatalf("source = %s", dist.Prices[0].PurchasePriceSource)
	}
	eq(t, "distributor sale", dist.Prices[0].SalePrice, "95")
	if js := jsonOf(t, dist); strings.Contains(js, "recommended") || strings.Contains(js, "\"50") {
		t.Fatalf("distributor view leaks: %s", js)
	}

	other, err := svc.ProductView(ctx, viewerFor(otherDistID, OrgDistributor, rbac.RoleDistributorOwner), productUUID)
	if err != nil || len(other.Prices) != 1 {
		t.Fatalf("other distributor view: %+v %v", other, err)
	}
	eq(t, "other distributor purchase", other.Prices[0].PurchasePrice, "80.5")
	if other.Prices[0].PurchasePriceSource != SourceList || other.Prices[0].SalePrice != nil {
		t.Fatalf("other distributor view: %s", jsonOf(t, other))
	}

	dealer, err := svc.ProductView(ctx, viewerFor(dealerID, OrgDealer, rbac.RoleDealerOwner), productUUID)
	if err != nil || len(dealer.Prices) != 1 {
		t.Fatalf("dealer view: %+v %v", dealer, err)
	}
	eq(t, "dealer purchase", dealer.Prices[0].PurchasePrice, "95")
	js := jsonOf(t, dealer.Prices[0])
	for _, field := range []string{"\"sale_price\"", "\"recommended_sale_price\""} {
		if strings.Contains(js, field) {
			t.Fatalf("dealer view has %s: %s", field, js)
		}
	}

	// Dealer staff has no pricing grant: nothing is visible.
	ds, err := svc.ProductView(ctx, viewerFor(dealerID, OrgDealer, rbac.RoleDealerStaff), productUUID)
	if err != nil || len(ds.Prices) != 0 {
		t.Fatalf("dealer staff view: %s %v", jsonOf(t, ds), err)
	}
}

func TestWriteRules(t *testing.T) {
	f := newFake()
	svc := New(f)
	ctx := context.Background()
	dist := viewerFor(distributorID, OrgDistributor, rbac.RoleDistributorOwner)
	dealer := viewerFor(dealerID, OrgDealer, rbac.RoleDealerOwner)
	center := viewerFor(centerID, OrgCenter, rbac.RoleCenterAccounting)

	if _, err := svc.SetListPrice(ctx, dist, productUUID, "TRY", ListPriceInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor list price: %v", err)
	}
	if _, err := svc.SetDistributorOverride(ctx, dist, productUUID, f.orgs[distributorID].Uuid, "TRY", "1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor override: %v", err)
	}
	if _, err := svc.SetDealerPrice(ctx, dealer, productUUID, "TRY", "1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer dealer price: %v", err)
	}
	if _, err := svc.SetDealerPrice(ctx, center, productUUID, "TRY", "1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("center dealer price: %v", err)
	}
	// A dealer is not a distributor target.
	if _, err := svc.SetDistributorOverride(ctx, center, productUUID, uuid.New(), "TRY", "1"); !errors.Is(err, ErrDistributorNotFound) {
		t.Fatalf("unknown distributor: %v", err)
	}
	var ve *ValidationError
	if _, err := svc.SetDealerPrice(ctx, dist, productUUID, "TRY", "-1"); !errors.As(err, &ve) {
		t.Fatalf("negative price: %v", err)
	}
	if _, err := svc.SetDealerPrice(ctx, dist, productUUID, "TL", "1"); !errors.As(err, &ve) || ve.Field != "currency" {
		t.Fatalf("bad currency: %v", err)
	}
	if _, err := svc.SetDealerPrice(ctx, dist, uuid.New(), "TRY", "1"); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("unknown product: %v", err)
	}

	// Partial update keeps the other fields; recommended needs its own grant.
	if _, err := svc.SetListPrice(ctx, center, productUUID, "TRY", ListPriceInput{
		Purchase: OptionalPrice{Set: true, Value: s("10")}, Recommended: OptionalPrice{Set: true, Value: s("30")},
	}); err != nil {
		t.Fatal(err)
	}
	v, err := svc.SetListPrice(ctx, center, productUUID, "TRY", ListPriceInput{
		SaleToDistributor: OptionalPrice{Set: true, Value: s("20")},
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "kept purchase", v.Prices[0].PurchasePrice, "10")
	eq(t, "new sale", v.Prices[0].SalePrice, "20")
	eq(t, "kept recommended", v.Prices[0].RecommendedSalePrice, "30")
	noRec := center
	noRec.RecommendedWrite = false
	if _, err := svc.SetListPrice(ctx, noRec, productUUID, "TRY", ListPriceInput{
		Recommended: OptionalPrice{Set: true},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("recommended without grant: %v", err)
	}
}

func TestNormalizePrice(t *testing.T) {
	for _, ok := range []string{"0", "1", "12.5", "9999999999.9999"} {
		if _, err := NormalizePrice("p", ok); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-1", "1.23456", "1e3", "12345678901", "abc"} {
		if _, err := NormalizePrice("p", bad); err == nil {
			t.Fatalf("%s accepted", bad)
		}
	}
}
