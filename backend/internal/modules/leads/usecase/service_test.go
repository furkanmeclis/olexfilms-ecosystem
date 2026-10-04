package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	tx     pgx.Tx
	q      *db.Queries
	svc    *Service
	brand  int64
	center db.Organization
	dealer db.Organization
	other  db.Organization
	user   db.User
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	f := &fixture{t: t, ctx: ctx, pool: pool, tx: tx, q: q}
	if err := tx.QueryRow(ctx, `SELECT id FROM brands WHERE slug = 'olex'`).Scan(&f.brand); err != nil {
		t.Fatalf("brand: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT id, uuid, slug, name, city, district, phone, address, logo_object_key, status, plan_code,
		access_starts_at, access_ends_at, created_at, updated_at, deleted_at, email, website, tagline, footer_text,
		paper_size, primary_color, type, parent_id, brand_id, currency, locale, timezone, country_id, contract_pdf_key,
		contract_valid_until, settings, province_id, district_id, phone_raw, google_business_url, latitude, longitude
		FROM organizations WHERE brand_id = $1 AND type = 'center' ORDER BY id LIMIT 1`, f.brand).Scan(
		&f.center.ID, &f.center.Uuid, &f.center.Slug, &f.center.Name, &f.center.City, &f.center.District, &f.center.Phone,
		&f.center.Address, &f.center.LogoObjectKey, &f.center.Status, &f.center.PlanCode, &f.center.AccessStartsAt,
		&f.center.AccessEndsAt, &f.center.CreatedAt, &f.center.UpdatedAt, &f.center.DeletedAt, &f.center.Email,
		&f.center.Website, &f.center.Tagline, &f.center.FooterText, &f.center.PaperSize, &f.center.PrimaryColor,
		&f.center.Type, &f.center.ParentID, &f.center.BrandID, &f.center.Currency, &f.center.Locale, &f.center.Timezone,
		&f.center.CountryID, &f.center.ContractPdfKey, &f.center.ContractValidUntil, &f.center.Settings, &f.center.ProvinceID,
		&f.center.DistrictID, &f.center.PhoneRaw, &f.center.GoogleBusinessUrl, &f.center.Latitude, &f.center.Longitude); err != nil {
		t.Fatalf("center: %v", err)
	}
	f.dealer = f.org("dealer-a")
	f.other = f.org("dealer-b")
	f.user = f.userRow("lead-user")
	f.svc = New(tx, q, nil)
	f.svc.SetClock(func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) })
	return f
}

func (f *fixture) org(slug string) db.Organization {
	return f.orgTyped(slug, "dealer", f.center.ID)
}

func (f *fixture) orgTyped(slug, typ string, parentID int64) db.Organization {
	f.t.Helper()
	slug = fmt.Sprintf("tec313-%s-%d", slug, time.Now().UnixNano())
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: slug, Name: slug, Status: "active", Type: typ,
		ParentID: pgtype.Int8{Int64: parentID, Valid: parentID != 0}, BrandID: f.brand,
		Currency: "TRY", Locale: "tr", Timezone: "UTC", Settings: []byte(`{}`),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		f.t.Fatalf("org: %v", err)
	}
	return o
}

func (f *fixture) userRow(prefix string) db.User {
	f.t.Helper()
	email := fmt.Sprintf("%s-%d@example.test", prefix, time.Now().UnixNano())
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		Email: pgtype.Text{String: email, Valid: true}, PasswordHash: "x", Name: prefix, Surname: "User", Status: "active",
	})
	if err != nil {
		f.t.Fatalf("user: %v", err)
	}
	return u
}

func (f *fixture) caller(org db.Organization, scope rbac.Scope) Caller {
	var orgIDs []int64
	if scope != rbac.ScopeAll && scope != rbac.ScopeBrand {
		orgIDs = []int64{org.ID}
	}
	perms := map[string]rbac.Scope{
		rbac.PermLeadsRead:        scope,
		rbac.PermLeadsWrite:       scope,
		rbac.PermQuotesRead:       scope,
		rbac.PermQuotesWrite:      scope,
		rbac.PermPricingSaleWrite: scope,
	}
	return Caller{
		Principal: authctx.Principal{UserInternal: f.user.ID, PermissionScopes: perms},
		Org:       orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, OrgType: org.Type, BrandID: org.BrandID},
		Filter:    scopefilter.Filter{Permission: rbac.PermLeadsRead, Scope: scope, UserID: f.user.ID, OrgID: org.ID, OrgIDs: orgIDs, BrandID: org.BrandID},
	}
}

func (f *fixture) quoteCaller(org db.Organization, scope rbac.Scope) Caller {
	c := f.caller(org, scope)
	c.Filter.Permission = rbac.PermQuotesRead
	return c
}

func num(s string) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		panic(err)
	}
	return n
}

func strPtr(s string) *string { return &s }

func (f *fixture) pricedProduct(sku, sale string) db.Product {
	f.t.Helper()
	cat, err := f.q.CreateProductCategory(f.ctx, db.CreateProductCategoryParams{
		OrganizationID: f.center.ID, BrandID: f.brand, Name: sku + " cat", AvailableParts: []byte(`[]`), Active: true,
	})
	if err != nil {
		f.t.Fatalf("category: %v", err)
	}
	p, err := f.q.CreateProduct(f.ctx, db.CreateProductParams{
		OrganizationID: f.center.ID, BrandID: f.brand, CategoryID: cat.ID, Sku: sku, Name: sku + " product",
		DescriptionMd: "", Images: []byte(`[]`), UnitType: "piece", Active: true, LockedFields: []string{},
	})
	if err != nil {
		f.t.Fatalf("product: %v", err)
	}
	if _, err := f.q.UpsertProductPrice(f.ctx, db.UpsertProductPriceParams{
		ProductID: p.ID, BrandID: f.brand, Currency: "TRY",
		SaleToDistributorPrice: pgtype.Text{String: sale, Valid: true},
		RecommendedSalePrice:   pgtype.Text{String: sale, Valid: true},
	}); err != nil {
		f.t.Fatalf("product price: %v", err)
	}
	return p
}

func (f *fixture) catalogItem(name, price string) db.ServiceCatalogItem {
	f.t.Helper()
	item, err := f.q.CreateServiceCatalogItem(f.ctx, db.CreateServiceCatalogItemParams{
		OrganizationID: f.center.ID, BrandID: f.brand, Name: name, Description: "desc",
		Category: "other", DefaultPrice: num(price), Currency: "TRY", Recurrence: "one_time",
		CancellationFee: num("0.00"), IsActive: true,
	})
	if err != nil {
		f.t.Fatalf("service item: %v", err)
	}
	return item
}

func (f *fixture) lead(org db.Organization, status string, follow *time.Time) db.Lead {
	f.t.Helper()
	l, err := f.q.CreateLead(f.ctx, db.CreateLeadParams{
		OrganizationID: org.ID, BrandID: org.BrandID, TargetType: "customer", Source: "walk_in",
		Temperature: "warm", Status: status, FollowUpDate: tstz(follow), Notes: "",
	})
	if err != nil {
		f.t.Fatalf("lead: %v", err)
	}
	return l
}

func TestStatusValidationAndEvents(t *testing.T) {
	f := newFixture(t)
	c := f.caller(f.dealer, rbac.ScopeManaged)
	lead, err := f.svc.Create(f.ctx, c, CreateInput{TargetType: "customer", Source: "walk_in", Temperature: "warm"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.svc.SetStatus(f.ctx, c, lead.UUID, StatusInput{Status: StatusWon}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("new -> won err = %v, want ErrInvalidTransition", err)
	}
	if _, err := f.svc.SetStatus(f.ctx, c, lead.UUID, StatusInput{Status: StatusContacted}); err != nil {
		t.Fatalf("new -> contacted: %v", err)
	}
	if _, err := f.svc.SetStatus(f.ctx, c, lead.UUID, StatusInput{Status: StatusQuoted}); err != nil {
		t.Fatalf("contacted -> quoted: %v", err)
	}
	if _, err := f.svc.SetStatus(f.ctx, c, lead.UUID, StatusInput{Status: StatusLost}); err == nil {
		t.Fatal("lost without reason accepted")
	} else {
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != "lost_reason" {
			t.Fatalf("lost reason err = %v", err)
		}
	}
	patch := "patched"
	if _, err := f.svc.Patch(f.ctx, c, lead.UUID, PatchInput{Notes: &patch}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	events, err := f.svc.Events(f.ctx, c, lead.UUID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("event count = %d, want 4", len(events))
	}
}

func TestLeadVisibilityAndFollowUpQueue(t *testing.T) {
	f := newFixture(t)
	past := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	today := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	future := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	overdue := f.lead(f.dealer, StatusNew, &past)
	f.lead(f.dealer, StatusContacted, &today)
	f.lead(f.dealer, StatusNew, &future)
	f.lead(f.dealer, StatusWon, &past)
	f.lead(f.other, StatusNew, &past)

	dealer := f.caller(f.dealer, rbac.ScopeManaged)
	other := f.caller(f.other, rbac.ScopeManaged)
	center := f.caller(f.center, rbac.ScopeAll)
	if _, err := f.svc.Get(f.ctx, other, overdue.Uuid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other dealer get = %v, want not found", err)
	}
	if _, err := f.svc.Get(f.ctx, center, overdue.Uuid); err != nil {
		t.Fatalf("center get: %v", err)
	}
	rows, total, err := f.svc.List(f.ctx, dealer, ListFilter{FollowUp: "overdue", Limit: 20})
	if err != nil {
		t.Fatalf("overdue list: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].UUID != overdue.Uuid {
		t.Fatalf("overdue = total %d rows %+v, want only %s", total, rows, overdue.Uuid)
	}
	count, err := f.svc.FollowUpCount(f.ctx, dealer)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count.Overdue != 1 || count.Today != 1 {
		t.Fatalf("count = %+v, want overdue=1 today=1", count)
	}
}

func TestNonCenterCannotCreateLeadTask(t *testing.T) {
	f := newFixture(t)
	c := f.caller(f.dealer, rbac.ScopeManaged)
	lead := f.lead(f.dealer, StatusNew, nil)
	if _, err := f.svc.CreateTask(f.ctx, c, lead.Uuid, TaskInput{Title: "Call"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("CreateTask err = %v, want ErrForbidden", err)
	}
}

func TestQuoteTotalsProductAndCatalogLines(t *testing.T) {
	f := newFixture(t)
	dist := f.orgTyped("quote-dist", "distributor", f.center.ID)
	c := f.quoteCaller(dist, rbac.ScopeManaged)
	lead := f.lead(dist, StatusNew, nil)
	product := f.pricedProduct("TEC314P", "100.00")
	item := f.catalogItem("Ceramic care", "50.00")

	q, err := f.svc.CreateQuote(f.ctx, c, lead.Uuid, QuoteInput{Lines: []QuoteLineInput{
		{LineType: QuoteLineProduct, ProductUUID: &product.Uuid, Quantity: "2", DiscountAmount: strPtr("10.00")},
		{LineType: QuoteLineCatalogService, ServiceCatalogItemUUID: &item.Uuid, Quantity: "1.5", DiscountAmount: strPtr("5.00")},
	}})
	if err != nil {
		t.Fatalf("CreateQuote: %v", err)
	}
	if q.Subtotal != "275.00" || q.DiscountTotal != "15.00" || q.TaxTotal != "0.00" || q.GrandTotal != "260.00" {
		t.Fatalf("totals = %+v", q)
	}
	if len(q.Lines) != 2 || q.Lines[0].LineTotal != "190.00" || q.Lines[1].LineTotal != "70.00" {
		t.Fatalf("lines = %+v", q.Lines)
	}
}

func TestQuoteLinesLockedAfterSent(t *testing.T) {
	f := newFixture(t)
	dist := f.orgTyped("quote-sent", "distributor", f.center.ID)
	c := f.quoteCaller(dist, rbac.ScopeManaged)
	lead := f.lead(dist, StatusNew, nil)
	item := f.catalogItem("Detailing", "100.00")
	q, err := f.svc.CreateQuote(f.ctx, c, lead.Uuid, QuoteInput{Lines: []QuoteLineInput{
		{LineType: QuoteLineCatalogService, ServiceCatalogItemUUID: &item.Uuid, Quantity: "1"},
	}})
	if err != nil {
		t.Fatalf("CreateQuote: %v", err)
	}
	row, err := f.q.GetQuoteByUUID(f.ctx, db.GetQuoteByUUIDParams{Uuid: q.UUID, BrandID: f.brand})
	if err != nil {
		t.Fatalf("quote row: %v", err)
	}
	if _, err := f.q.SetQuoteStatus(f.ctx, db.SetQuoteStatusParams{ID: row.ID, OrganizationID: row.OrganizationID, Status: QuoteStatusSent}); err != nil {
		t.Fatalf("sent: %v", err)
	}
	_, err = f.svc.ReplaceQuoteLines(f.ctx, c, q.UUID, []QuoteLineInput{{LineType: QuoteLineCatalogService, ServiceCatalogItemUUID: &item.Uuid, Quantity: "2"}})
	if !errors.Is(err, ErrQuoteConflict) {
		t.Fatalf("ReplaceQuoteLines err = %v, want ErrQuoteConflict", err)
	}
}

func TestQuoteCatalogOverrideDefaultsForDistributor(t *testing.T) {
	f := newFixture(t)
	dist := f.orgTyped("quote-nl-dist", "distributor", f.center.ID)
	c := f.quoteCaller(dist, rbac.ScopeManaged)
	lead := f.lead(dist, StatusNew, nil)
	item := f.catalogItem("NL module", "100.00")
	if _, err := f.q.UpsertServicePriceOverride(f.ctx, db.UpsertServicePriceOverrideParams{
		ItemID: item.ID, OrganizationID: dist.ID, BrandID: f.brand, Price: num("77.00"), Currency: "TRY",
	}); err != nil {
		t.Fatalf("override: %v", err)
	}
	q, err := f.svc.CreateQuote(f.ctx, c, lead.Uuid, QuoteInput{Lines: []QuoteLineInput{
		{LineType: QuoteLineCatalogService, ServiceCatalogItemUUID: &item.Uuid, Quantity: "1"},
	}})
	if err != nil {
		t.Fatalf("CreateQuote: %v", err)
	}
	if got := q.Lines[0].UnitPrice; got != "77.00" {
		t.Fatalf("unit price = %s, want override 77.00", got)
	}
}

func TestQuoteExpireTask(t *testing.T) {
	f := newFixture(t)
	dist := f.orgTyped("quote-expire", "distributor", f.center.ID)
	c := f.quoteCaller(dist, rbac.ScopeManaged)
	lead := f.lead(dist, StatusNew, nil)
	item := f.catalogItem("Expiring", "10.00")
	past := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	q, err := f.svc.CreateQuote(f.ctx, c, lead.Uuid, QuoteInput{ValidUntil: &past, Lines: []QuoteLineInput{
		{LineType: QuoteLineCatalogService, ServiceCatalogItemUUID: &item.Uuid, Quantity: "1"},
	}})
	if err != nil {
		t.Fatalf("CreateQuote: %v", err)
	}
	if err := f.svc.ExpireDueQuotesTask(f.ctx); err != nil {
		t.Fatalf("ExpireDueQuotesTask: %v", err)
	}
	got, err := f.svc.GetQuote(f.ctx, c, q.UUID)
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if got.Status != QuoteStatusExpired {
		t.Fatalf("status = %s, want expired", got.Status)
	}
}

func TestQuotePDFRenderHTMLContainsNumberLinesAndTotal(t *testing.T) {
	f := newFixture(t)
	dist := f.orgTyped("quote-pdf", "distributor", f.center.ID)
	c := f.quoteCaller(dist, rbac.ScopeManaged)
	lead := f.lead(dist, StatusNew, nil)
	item := f.catalogItem("PDF coating package", "120.00")
	q, err := f.svc.CreateQuote(f.ctx, c, lead.Uuid, QuoteInput{Lines: []QuoteLineInput{
		{LineType: QuoteLineCatalogService, ServiceCatalogItemUUID: &item.Uuid, Quantity: "2", DiscountAmount: strPtr("15.00")},
	}})
	if err != nil {
		t.Fatalf("CreateQuote: %v", err)
	}
	gotb := documentstest.NewGotenberg(t, 0)
	docs := docusecase.New(nil, f.q, storage.NewMemory(), pdfrender.New(gotb.URL), nil, pdfrender.FontsEmbedded, nil)
	if err := docs.RegisterLoader(docmodel.KindQuote, f.svc); err != nil {
		t.Fatalf("RegisterLoader: %v", err)
	}
	v, ready, err := docs.RequestRender(f.ctx, docmodel.Viewer{
		UserID: f.user.ID, OrganizationID: dist.ID, BrandID: f.brand,
	}, docusecase.RenderInput{Kind: docmodel.KindQuote, SourceID: q.UUID.String(), Locale: "tr"})
	if err != nil {
		t.Fatalf("RequestRender: %v", err)
	}
	if !ready || v.Status != docusecase.RenderReady {
		t.Fatalf("render = ready %v view %+v", ready, v)
	}
	html := gotb.LastHTML()
	for _, want := range []string{q.DisplayNo, "PDF coating package", "225.00 TRY"} {
		if !strings.Contains(html, want) {
			t.Fatalf("quote pdf html missing %q in %s", want, html)
		}
	}
}

func TestHelpersValidateWithoutDatabase(t *testing.T) {
	if allowedTransition(StatusNew, StatusWon) {
		t.Fatal("new -> won accepted")
	}
	if !allowedTransition(StatusLost, StatusContacted) {
		t.Fatal("lost -> contacted rejected")
	}
	if validSource("bad") || !validSource("walk_in") {
		t.Fatal("source validation")
	}
	if e164Re.MatchString("05551234567") || !e164Re.MatchString("+905551234567") {
		t.Fatal("e164 validation")
	}
	_ = uuid.Nil
}
