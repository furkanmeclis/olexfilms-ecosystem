package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	appointmentsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	authmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	catalogmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	portaluc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/portalvehicles/usecase"
	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	shorturlsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/usecase"
	warrantyuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	claimsmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

// Customer fakes: the portal of user 7 (ownerUser) holds one service; any
// other user owns nothing.

const ownerUser = 7

// ownServiceUUID is the uuid validValue uses for "uuid" properties.
var ownServiceUUID = serviceUUID

type fakeLinks struct{ targets []string }

func (f *fakeLinks) Link(_ context.Context, in shorturlsuc.CreateInput) (string, error) {
	if _, err := shorturlsuc.NormalizeTarget(in.Target); err != nil {
		return "", err
	}
	f.targets = append(f.targets, in.Target)
	return fmt.Sprintf("https://olex.test/s/tok%d", len(f.targets)), nil
}

type fakePortal struct{}

func (fakePortal) ListVehicles(_ context.Context, c portaluc.Caller, _ portaluc.Page) ([]portaluc.VehicleView, int64, error) {
	if c.UserID != ownerUser {
		return []portaluc.VehicleView{}, 0, nil
	}
	plate := "34OWN1"
	return []portaluc.VehicleView{{Plate: &plate, CarBrand: &portaluc.NamedRef{Name: "BMW"}}}, 1, nil
}

func (fakePortal) ListServices(_ context.Context, c portaluc.Caller, f portaluc.ServiceListFilter) ([]portaluc.ServiceView, int64, error) {
	if c.UserID != ownerUser || (f.Q != "" && !strings.EqualFold(f.Q, "DSOWN00001")) {
		return []portaluc.ServiceView{}, 0, nil
	}
	return []portaluc.ServiceView{{UUID: ownServiceUUID, ServiceNo: "DSOWN00001", Status: "completed",
		Organization: portaluc.OrganizationRef{Name: "Dealer"}}}, 1, nil
}

type fakePortalServices struct{}

func (fakePortalServices) PortalGet(_ context.Context, _, userID int64, id uuid.UUID) (svcuc.PortalServiceView, error) {
	if userID != ownerUser || id != ownServiceUUID {
		return svcuc.PortalServiceView{}, svcuc.ErrNotFound
	}
	return svcuc.PortalServiceView{UUID: id, ServiceNo: "DSOWN00001", Status: "completed",
		Warranties: []svcuc.PortalWarrantyView{
			{PublicCode: "WOK1", ProductName: "Film", Status: "active", EndAt: time.Now().AddDate(1, 0, 0)},
			{PublicCode: "WVOID", ProductName: "Film", Status: "void"},
		}}, nil
}

func (fakePortalServices) PortalGetReview(_ context.Context, _, userID int64, id uuid.UUID) (svcuc.PortalServiceReview, error) {
	if userID != ownerUser || id != ownServiceUUID {
		return svcuc.PortalServiceReview{}, svcuc.ErrNotFound
	}
	u := "https://g.page/r/dealer/review"
	return svcuc.PortalServiceReview{GoogleBusinessURL: &u, CanReview: true}, nil
}

type fakePortalWarranties struct{}

func (fakePortalWarranties) PortalList(context.Context, int64, int64, warrantyuc.ListFilter) ([]warrantyuc.WarrantyListView, int64, error) {
	return []warrantyuc.WarrantyListView{{PublicCode: "WOK1", Status: "active",
		StartAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)}}, 1, nil
}

type fakePortalAppointments struct{}

func (fakePortalAppointments) PortalList(context.Context, appointmentsuc.PortalCaller, appointmentsuc.PortalListFilter) ([]appointmentsuc.PortalAppointment, int64, error) {
	return []appointmentsuc.PortalAppointment{{Appointment: appointmentsuc.Appointment{Status: "scheduled", CustomerUserID: ownerUser,
		CustomerName: "Ada Lovelace"}, DealerName: "Dealer"}}, 1, nil
}

type fakePortalClaims struct{}

func (fakePortalClaims) PortalList(context.Context, int64, int64) ([]claimsmodel.PortalClaimView, error) {
	w := uuid.New()
	return []claimsmodel.PortalClaimView{{UUID: uuid.New(), WarrantyUUID: &w, Status: "in_review"}}, nil
}

type fakeProfile struct{ got *string }

func (f *fakeProfile) UpdateProfile(_ context.Context, _ uuid.UUID, _, _ *uuid.UUID, in authmodel.ProfilePatch) (authmodel.Me, error) {
	f.got = in.Locale
	return authmodel.Me{EffectiveLocale: *in.Locale}, nil
}

func testCustomerDeps() CustomerDeps {
	return CustomerDeps{Portal: fakePortal{}, Services: fakePortalServices{}, Warranties: fakePortalWarranties{},
		Appointments: fakePortalAppointments{}, Claims: fakePortalClaims{}, Profile: &fakeProfile{}, Links: &fakeLinks{}}
}

// Visitor fakes return rows carrying data a visitor must never see (an
// external id, the dealer's address and tax number are not even fields of
// the tool DTOs; the warranty is the masked public projection).

type fakeVisitorCatalog struct{}

func (fakeVisitorCatalog) ListProducts(_ context.Context, org orgctx.Scope, _ catalogmodel.ProductFilter) ([]catalogmodel.Product, int64, error) {
	ext := "ERP-SECRET-1"
	months := int32(60)
	return []catalogmodel.Product{{UUID: productUUID, SKU: "SKU-1", Name: "Matte PPF", ExternalID: &ext,
		WarrantyDurationMonths: &months, Category: catalogmodel.CategoryRef{Name: "PPF"}, DescriptionMD: "Self healing."}}, 1, nil
}

func (fakeVisitorCatalog) ListCategories(context.Context, orgctx.Scope, catalogmodel.CategoryFilter) ([]catalogmodel.Category, int64, error) {
	return []catalogmodel.Category{{UUID: uuid.New(), Name: "PPF"}, {UUID: uuid.New(), Name: "Seramik Kaplama"}}, 2, nil
}

type fakeDealers struct{}

func (fakeDealers) NearbyDealers(context.Context, int64, orgusecase.NearbyInput) ([]orgusecase.NearbyDealer, error) {
	wa := "+905550000000"
	return []orgusecase.NearbyDealer{{UUID: uuid.New(), Slug: "d1", Name: "Dealer", City: "Istanbul", DistanceKm: 2.5, WhatsApp: &wa,
		Latitude: 41, Longitude: 29}}, nil
}

func (fakeDealers) AreaDealers(context.Context, int64, orgusecase.AreaInput) ([]orgusecase.NearbyDealer, error) {
	return []orgusecase.NearbyDealer{{UUID: uuid.New(), Slug: "d1", Name: "Dealer", City: "Istanbul"}}, nil
}

type fakeSettings struct{ text string }

func (f fakeSettings) GetAISettings(context.Context) (db.AiSetting, error) {
	return db.AiSetting{KnowledgeText: f.text}, nil
}

type fakePublicWarranty struct{}

func (fakePublicWarranty) Lookup(_ context.Context, brand warrantyuc.PublicWarrantyBrand, _ int64, code string) (warrantyuc.PublicWarranty, error) {
	if code != "DSAB12CD34" {
		return warrantyuc.PublicWarranty{}, warrantyuc.ErrPublicNotFound
	}
	masked, vin := "34 *** 12", "1234"
	logo := uuid.New()
	return warrantyuc.PublicWarranty{PublicCode: code, Status: "active", Product: warrantyuc.PublicProduct{Name: "Film"},
		Brand: brand, Dealer: warrantyuc.PublicDealer{Name: "Dealer", City: "Istanbul"},
		Vehicle: warrantyuc.PublicWarrantyCar{BrandName: "BMW", BrandLogoUUID: &logo, PlateMasked: &masked, VINLast4: &vin}}, nil
}

func testVisitorDeps() VisitorDeps {
	return VisitorDeps{Catalog: fakeVisitorCatalog{}, Dealers: fakeDealers{},
		Settings:   fakeSettings{text: "# Garanti\nGaranti süresi ürüne göre 5-10 yıldır.\n\n# Bakım\nYıkama 7 gün sonra."},
		Warranties: fakePublicWarranty{}, FrontendURL: "https://olex.test/"}
}

var testBrand = &brandctx.Brand{ID: 1, Slug: "olex", Name: "Olex", Status: "active"}

// realmPrincipal is a principal that passes the gate of a realm.
func realmPrincipal(realm Realm) Principal {
	switch realm {
	case RealmCustomer:
		return Principal{Auth: authctx.Principal{UserID: uuid.New(), UserInternal: ownerUser}, Realm: RealmCustomer, Brand: testBrand}
	case RealmVisitor:
		return Principal{Realm: RealmVisitor, Brand: testBrand}
	}
	return superAdmin()
}

// TEC-386 acceptance: customer A asking for another customer's service by
// number (or uuid) gets NOT_FOUND; the own service is found.
func TestCustomerServiceToolsOnlyOwnRecords(t *testing.T) {
	r := testRegistry(nil)
	owner := realmPrincipal(RealmCustomer)
	other := owner
	other.Auth.UserInternal = 99
	for _, name := range []string{"my_service_detail", "my_service_pdf_link", "dealer_review_link"} {
		for _, ref := range []string{"DSOWN00001", ownServiceUUID.String()} {
			if res := call(t, r, owner, name, `{"service":"`+ref+`"}`); res.IsError {
				t.Fatalf("owner %s(%s): %+v", name, ref, res)
			}
			if res := call(t, r, other, name, `{"service":"`+ref+`"}`); res.Code != CodeNotFound || strings.Contains(res.Content, "DSOWN") {
				t.Fatalf("other customer %s(%s): %+v", name, ref, res)
			}
		}
	}
	if res := call(t, r, other, "my_services", `{}`); res.IsError || !strings.Contains(res.Content, `"total":0`) {
		t.Fatalf("other customer services: %+v", res)
	}
}

func TestCustomerLinksAndDays(t *testing.T) {
	links := &fakeLinks{}
	r := NewRegistry(nil).WithClock(func() time.Time { return time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC) }, nil)
	d := testCustomerDeps()
	d.Links = links
	RegisterCustomer(r, d)
	p := realmPrincipal(RealmCustomer)

	res := call(t, r, p, "my_service_pdf_link", `{"service":"DSOWN00001"}`)
	if res.IsError || !strings.Contains(res.Content, `"service_page_url":"https://olex.test/s/`) || strings.Contains(res.Content, "WVOID") {
		t.Fatalf("pdf link: %+v", res)
	}
	want := []string{"/portal/services/" + ownServiceUUID.String(), "/garanti/WOK1/pdf"}
	if fmt.Sprint(links.targets) != fmt.Sprint(want) {
		t.Fatalf("link targets = %v, want %v (void warranty skipped)", links.targets, want)
	}
	if res := call(t, r, p, "my_profile_link", `{}`); res.IsError || links.targets[len(links.targets)-1] != "/portal" {
		t.Fatalf("profile link: %+v %v", res, links.targets)
	}
	if res := call(t, r, p, "dealer_review_link", `{}`); !strings.Contains(res.Content, "g.page/r/dealer/review") ||
		!strings.Contains(res.Content, "review_form_url") {
		t.Fatalf("review link: %+v", res)
	}
	// Ten days left on 2026-10-07 09:00 to 2026-10-17 09:00.
	if res := call(t, r, p, "my_warranties", `{}`); !strings.Contains(res.Content, `"days_left":10`) {
		t.Fatalf("warranty days: %s", res.Content)
	}
	// Claims: status and dates only.
	res = call(t, r, p, "my_warranty_claims", `{}`)
	var claims List[map[string]any]
	if err := json.Unmarshal([]byte(res.Content), &claims); err != nil || len(claims.Items) != 1 {
		t.Fatalf("claims: %s", res.Content)
	}
	if got := keysOf(claims.Items[0]); fmt.Sprint(got) != "[opened_at status updated_at]" {
		t.Fatalf("claim fields = %v", got)
	}
	// Appointments drop the customer identifiers.
	if res := call(t, r, p, "my_appointments", `{}`); strings.Contains(res.Content, "Ada") || strings.Contains(res.Content, "customer") {
		t.Fatalf("appointments leak: %s", res.Content)
	}
}

func TestChangeLanguageRunsWithoutConfirmation(t *testing.T) {
	prof := &fakeProfile{}
	r := NewRegistry(nil)
	RegisterCustomer(r, CustomerDeps{Profile: prof})
	tool, _ := r.Get("change_language")
	if tool.Spec().Kind != KindSelf {
		t.Fatalf("kind = %s", tool.Spec().Kind)
	}
	res := call(t, r, realmPrincipal(RealmCustomer), "change_language", `{"locale":"zh-CN"}`)
	if res.IsError || prof.got == nil || *prof.got != "zh-CN" {
		t.Fatalf("change_language: %+v %v", res, prof.got)
	}
	if res := call(t, r, realmPrincipal(RealmCustomer), "change_language", `{"locale":"xx"}`); res.Code != CodeInvalidInput {
		t.Fatalf("unsupported locale: %+v", res)
	}
}

// TEC-386 acceptance: the visitor tools return no personal data field.
// Every key of every visitor result is on the whitelist, none is a
// personal data key of the public warranty page check (FindPIIKeys) and no
// price / id / internal value of the fakes leaks.
func TestVisitorToolsReturnNoPersonalData(t *testing.T) {
	r := testRegistry(nil)
	p := realmPrincipal(RealmVisitor)
	inputs := map[string][]string{
		"recommend_products":   {`{}`, `{"category":"ppf","query":"matte"}`},
		"find_nearest_dealers": {`{"city":"Istanbul"}`, `{"latitude":41,"longitude":29}`},
		"search_knowledge":     {`{"query":"garanti süresi"}`},
		"lookup_warranty":      {`{"code":"DSAB12CD34"}`},
	}
	avail, err := r.Available(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(avail); fmt.Sprint(got) != "[find_nearest_dealers lookup_warranty recommend_products search_knowledge]" {
		t.Fatalf("visitor tools = %v", got)
	}
	for _, tool := range avail {
		name := tool.Spec().Name
		for _, in := range inputs[name] {
			res := call(t, r, p, name, in)
			if res.IsError {
				t.Fatalf("%s(%s): %+v", name, in, res)
			}
			var doc any
			if err := json.Unmarshal([]byte(res.Content), &doc); err != nil {
				t.Fatal(err)
			}
			for _, k := range allKeys(doc) {
				if !VisitorOutputKeys[k] {
					t.Errorf("%s(%s): field %q is not on the visitor whitelist", name, in, k)
				}
			}
			if pii := warrantyuc.FindPIIKeys([]byte(res.Content)); len(pii) > 0 {
				t.Errorf("%s: personal data keys %v", name, pii)
			}
			for _, leak := range []string{"ERP-SECRET", "SKU-1", "uuid", `"price`, "latitude", "brand_logo", productUUID.String()} {
				if strings.Contains(res.Content, leak) {
					t.Errorf("%s(%s) leaks %q: %s", name, in, leak, res.Content)
				}
			}
		}
	}
	// Visitors get no customer or panel tool.
	for _, name := range []string{"my_services", "search_services", "get_service"} {
		if res := call(t, r, p, name, `{}`); res.Code != CodeToolNotAllowed {
			t.Fatalf("visitor %s: %+v", name, res)
		}
	}
}

func TestRealmGateNeedsBrandAndUser(t *testing.T) {
	r := testRegistry(nil)
	ctx := context.Background()
	noBrand := realmPrincipal(RealmVisitor)
	noBrand.Brand = nil
	if got, _ := r.Available(ctx, noBrand); len(got) != 0 {
		t.Fatalf("visitor without brand: %v", names(got))
	}
	anon := realmPrincipal(RealmCustomer)
	anon.Auth.UserInternal = 0
	if res := call(t, r, anon, "my_services", `{}`); res.Code != CodeToolNotAllowed {
		t.Fatalf("customer without user: %+v", res)
	}
	// K20: another brand's domain reaches no customer tool.
	glorian := brandctx.WithBrand(ctx, brandctx.Brand{ID: 2, Slug: "glorian"})
	if got, _ := r.Available(glorian, realmPrincipal(RealmCustomer)); len(got) != 0 {
		t.Fatalf("customer tools across brands: %v", names(got))
	}
	// Panel tools never reach the customer realm and vice versa.
	if res := call(t, r, superAdmin(), "my_services", `{}`); res.Code != CodeToolNotAllowed {
		t.Fatalf("panel principal called a customer tool: %+v", res)
	}
}

func TestFindNearestDealersNeedsAPlace(t *testing.T) {
	r := testRegistry(nil)
	p := realmPrincipal(RealmVisitor)
	for _, in := range []string{`{}`, `{"latitude":41}`} {
		if res := call(t, r, p, "find_nearest_dealers", in); res.Code != CodeInvalidInput {
			t.Fatalf("%s: %+v", in, res)
		}
	}
	res := call(t, r, p, "find_nearest_dealers", `{"latitude":41,"longitude":29}`)
	if !strings.Contains(res.Content, `"distance_km":2.5`) || !strings.Contains(res.Content, `"page_url":"https://olex.test/bayi/d1"`) {
		t.Fatalf("by location: %s", res.Content)
	}
}

func TestKnowledgeSearch(t *testing.T) {
	text := "# Garanti\nGaranti süresi 10 yıl.\n\n## Bakım\nİlk yıkama 7 gün sonra.\n# İletişim\nBayiye yazın."
	if got := KnowledgeSearch(text, "GARANTİ SÜRESİ ne kadar", 3); len(got) != 1 || !strings.HasPrefix(got[0], "# Garanti") {
		t.Fatalf("headed search = %q", got)
	}
	if got := KnowledgeSearch(text, "ilk yikama", 3); len(got) != 1 || !strings.Contains(got[0], "7 gün") {
		t.Fatalf("folded search = %q", got)
	}
	if got := KnowledgeSearch("Bir.\n\nİki yıkama.\n\nÜç yıkama bakım.", "yıkama bakım", 1); len(got) != 1 || got[0] != "Üç yıkama bakım." {
		t.Fatalf("paragraph ranking = %q", got)
	}
	if got := KnowledgeSearch("", "garanti", 3); got != nil {
		t.Fatalf("empty text = %q", got)
	}
	r := testRegistry(nil)
	if res := call(t, r, realmPrincipal(RealmVisitor), "search_knowledge", `{"query":"fiyat listesi"}`); res.IsError ||
		!strings.Contains(res.Content, `"passages":[]`) || !strings.Contains(res.Content, "Do not guess") {
		t.Fatalf("no match: %+v", res)
	}
}

func allKeys(v any) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		for k, c := range t {
			out = append(out, k)
			out = append(out, allKeys(c)...)
		}
	case []any:
		for _, c := range t {
			out = append(out, allKeys(c)...)
		}
	}
	return out
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
