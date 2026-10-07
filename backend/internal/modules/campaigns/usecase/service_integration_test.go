package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	ctx                   context.Context
	tx                    pgx.Tx
	q                     *db.Queries
	svc                   *Service
	store                 *platstorage.Memory
	brandID               int64
	seq                   int
	center                db.Organization
	dist, otherDist       db.Organization
	dealer, otherDealer   db.Organization
	marketingTexts        map[string]db.LegalText
	centerC, distC        Caller
	dealerC, otherDealerC Caller
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	tx, err := testPool(t).Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	store := platstorage.NewMemory()
	f := &fixture{ctx: ctx, tx: tx, q: q, svc: New(tx, q, store), store: store, brandID: brand.ID, center: center,
		marketingTexts: map[string]db.LegalText{}}
	f.dist = f.org(t, "dist", rbac.OrgTypeDistributor, center.ID)
	f.otherDist = f.org(t, "dist2", rbac.OrgTypeDistributor, center.ID)
	f.dealer = f.org(t, "dealer", rbac.OrgTypeDealer, f.dist.ID)
	f.otherDealer = f.org(t, "dealer2", rbac.OrgTypeDealer, f.otherDist.ID)
	author := f.user(t, "author", "")
	f.centerC = Caller{UserID: author.ID, OrganizationID: center.ID, OrgType: rbac.OrgTypeCenter, BrandID: brand.ID,
		Filter: scopefilter.Filter{Scope: rbac.ScopeBrand, BrandID: brand.ID, OrgID: center.ID}}
	f.distC = f.managed(author, f.dist)
	f.dealerC = f.managed(author, f.dealer)
	f.otherDealerC = f.managed(author, f.otherDealer)
	for _, l := range []string{"tr", "en"} {
		lt, err := q.GetLatestLegalText(ctx, db.GetLatestLegalTextParams{Kind: "marketing_consent", Locale: l})
		if err != nil {
			t.Fatalf("marketing text %s: %v", l, err)
		}
		f.marketingTexts[l] = lt
	}
	return f
}

func (f *fixture) managed(u db.User, org db.Organization) Caller {
	return Caller{UserID: u.ID, OrganizationID: org.ID, OrgType: org.Type, BrandID: org.BrandID,
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgID: org.ID, OrgIDs: []int64{org.ID}}}
}

func (f *fixture) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("t405-%s-%d", name, time.Now().UnixNano()), Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: f.brandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

// user creates an active user with a unique e-mail and a random phone;
// locale "" inherits.
func (f *fixture) user(t *testing.T, name, locale string) db.User {
	t.Helper()
	f.seq++
	arg := db.CreateUserParams{
		Email:        pgtype.Text{String: fmt.Sprintf("t405-%s-%d-%d@example.test", name, time.Now().UnixNano(), f.seq), Valid: true},
		PhoneE164:    pgtype.Text{String: fmt.Sprintf("+4915%09d", rand.IntN(1_000_000_000)), Valid: true},
		PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
	}
	u, err := f.q.CreateUser(f.ctx, arg)
	if err != nil {
		t.Fatalf("user %s: %v", name, err)
	}
	if locale != "" {
		f.exec(t, `UPDATE users SET locale = $2 WHERE id = $1`, u.ID, locale)
	}
	return u
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.tx.Exec(f.ctx, sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// customer creates a customer of org; consent true/false records an
// accepted/declined marketing consent, nil none.
func (f *fixture) customer(t *testing.T, org db.Organization, name, locale string, consent *bool) db.User {
	t.Helper()
	u := f.user(t, name, locale)
	f.exec(t, `INSERT INTO customer_organizations (user_id, organization_id, brand_id) VALUES ($1, $2, $3)`,
		u.ID, org.ID, org.BrandID)
	if consent != nil {
		lt := f.marketingTexts["tr"]
		if _, err := f.q.InsertConsent(f.ctx, db.InsertConsentParams{
			UserID: u.ID, LegalTextID: lt.ID, Kind: "marketing_consent", Locale: lt.Locale,
			TextVersion: lt.Version, Accepted: *consent,
		}); err != nil {
			t.Fatalf("consent: %v", err)
		}
	}
	return u
}

func (f *fixture) member(t *testing.T, org db.Organization, name string) db.User {
	t.Helper()
	u := f.user(t, name, "")
	if _, err := f.q.CreateOrganizationMember(f.ctx, db.CreateOrganizationMemberParams{
		OrganizationID: org.ID, UserID: u.ID, Role: "staff",
	}); err != nil {
		t.Fatalf("member: %v", err)
	}
	return u
}

func (f *fixture) keys(t *testing.T) []string {
	t.Helper()
	res, err := f.store.List(f.ctx, platstorage.ListObjectsInput{Prefix: "campaigns/"})
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, o := range res.Objects {
		out = append(out, o.Key)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

func (f *fixture) create(t *testing.T, c Caller, filter AudienceFilter, channels ...string) Campaign {
	t.Helper()
	if len(channels) == 0 {
		channels = []string{ChannelWhatsApp}
	}
	out, err := f.svc.Create(f.ctx, c, Input{Name: "Kampanya", Channels: channels, AudienceFilter: &filter})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return out
}

func (f *fixture) preview(t *testing.T, c Caller, id uuid.UUID) Preview {
	t.Helper()
	p, err := f.svc.Preview(f.ctx, c, id)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	return p
}

func wantValidation(t *testing.T, err error, field string) {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) || (field != "" && ve.Field != field) {
		t.Fatalf("err = %v, want validation error on %s", err, field)
	}
}

// Acceptance: a dealer's audience never counts another dealer's customers;
// a distributor reaches its subtree, the center the whole brand.
func TestCampaignAudienceScope(t *testing.T) {
	f := newFixture(t)
	f.customer(t, f.dealer, "own", "tr", ptr(true))
	f.customer(t, f.otherDealer, "foreign", "tr", ptr(true))
	f.customer(t, f.dist, "distown", "tr", ptr(true))
	customers := AudienceFilter{AudienceType: AudienceCustomers}

	dealerCampaign := f.create(t, f.dealerC, customers)
	if p := f.preview(t, f.dealerC, dealerCampaign.UUID); p.Total != 1 || p.Excluded.Total != 0 {
		t.Fatalf("dealer preview = %+v, want 1 customer", p)
	}
	distCampaign := f.create(t, f.distC, customers)
	if p := f.preview(t, f.distC, distCampaign.UUID); p.Total != 2 {
		t.Fatalf("distributor preview total = %d, want 2 (own + sub dealer)", p.Total)
	}
	centerCampaign := f.create(t, f.centerC, AudienceFilter{
		AudienceType: AudienceCustomers, OrganizationUUIDs: []uuid.UUID{f.dealer.Uuid, f.otherDealer.Uuid},
	})
	if p := f.preview(t, f.centerC, centerCampaign.UUID); p.Total != 2 {
		t.Fatalf("center preview (two dealers) total = %d, want 2", p.Total)
	}
	// Another dealer neither sees the campaign nor may filter on a foreign
	// organization.
	if _, err := f.svc.Preview(f.ctx, f.otherDealerC, dealerCampaign.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign preview err = %v, want not found", err)
	}
	_, err := f.svc.Create(f.ctx, f.dealerC, Input{Name: "x", Channels: []string{ChannelPush},
		AudienceFilter: &AudienceFilter{AudienceType: AudienceCustomers, OrganizationUUIDs: []uuid.UUID{f.otherDealer.Uuid}}})
	wantValidation(t, err, "audience_filter.organization_uuids")
}

// Acceptance: a customer without marketing consent is not counted and is
// reported under excluded; an opted-out phone is excluded as well.
func TestCampaignAudienceConsent(t *testing.T) {
	f := newFixture(t)
	ok := f.customer(t, f.dealer, "ok", "tr", ptr(true))
	f.customer(t, f.dealer, "silent", "tr", nil)
	f.customer(t, f.dealer, "declined", "tr", ptr(false))
	opted := f.customer(t, f.dealer, "opted", "tr", ptr(true))
	if _, err := f.q.InsertContactOptOut(f.ctx, db.InsertContactOptOutParams{
		ContactE164: opted.PhoneE164.String, Scope: "marketing", Action: "out", Source: "whatsapp",
	}); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO device_push_tokens (user_id, device_id, platform, expo_token) VALUES ($1, 'd1', 'ios', $2)`,
		ok.ID, "ExponentPushToken["+uuid.NewString()+"]")

	c := f.create(t, f.dealerC, AudienceFilter{AudienceType: AudienceCustomers}, ChannelPush, ChannelWhatsApp, ChannelEmail)
	p := f.preview(t, f.dealerC, c.UUID)
	if p.Total != 1 {
		t.Fatalf("total = %d, want 1 consenting customer", p.Total)
	}
	if p.Excluded != (ExcludedInfo{Total: 3, NoConsent: 2, OptedOut: 1}) {
		t.Fatalf("excluded = %+v", p.Excluded)
	}
	want := []ChannelCount{{ChannelPush, 1}, {ChannelWhatsApp, 1}, {ChannelEmail, 1}}
	if !reflect.DeepEqual(p.Channels, want) {
		t.Fatalf("channels = %+v, want %+v", p.Channels, want)
	}
	if len(p.Sample) != 1 || p.Sample[0].Name != "o*** T***" || p.Sample[0].Phone == nil ||
		strings.Contains(*p.Sample[0].Phone, ok.PhoneE164.String[5:]) || !strings.HasPrefix(*p.Sample[0].Email, "t***@e***") {
		t.Fatalf("sample = %+v", p.Sample)
	}
}

// Panel user audiences: business notifications (no consent); dealer_users is
// closed to dealers, distributor_users open to the center only.
func TestCampaignAudienceTypes(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Create(f.ctx, f.dealerC, Input{Name: "x", Channels: []string{ChannelEmail},
		AudienceFilter: &AudienceFilter{AudienceType: AudienceDealerUsers}})
	wantValidation(t, err, "audience_filter.audience_type")
	_, err = f.svc.Create(f.ctx, f.distC, Input{Name: "x", Channels: []string{ChannelEmail},
		AudienceFilter: &AudienceFilter{AudienceType: AudienceDistributorUsers}})
	wantValidation(t, err, "audience_filter.audience_type")
	_, err = f.svc.Create(f.ctx, f.distC, Input{Name: "x", Channels: []string{ChannelEmail},
		AudienceFilter: &AudienceFilter{AudienceType: AudienceDealerUsers, WarrantyStatuses: []string{WarrantyActive}}})
	wantValidation(t, err, "audience_filter.warranty_statuses")

	f.member(t, f.dealer, "staff1")
	f.member(t, f.dealer, "staff2")
	f.member(t, f.otherDealer, "foreign")
	f.member(t, f.dist, "diststaff")
	c := f.create(t, f.distC, AudienceFilter{AudienceType: AudienceDealerUsers}, ChannelEmail)
	if p := f.preview(t, f.distC, c.UUID); p.Total != 2 || p.Excluded.Total != 0 {
		t.Fatalf("distributor dealer_users preview = %+v, want 2 without consent", p)
	}
	cc := f.create(t, f.centerC, AudienceFilter{
		AudienceType: AudienceDistributorUsers, OrganizationUUIDs: []uuid.UUID{f.dist.Uuid},
	}, ChannelEmail)
	if p := f.preview(t, f.centerC, cc.UUID); p.Total != 1 {
		t.Fatalf("center distributor_users preview total = %d, want 1", p.Total)
	}
}

// Geography (customer address ids) and the resolved-locale filter.
func TestCampaignAudienceFilters(t *testing.T) {
	f := newFixture(t)
	ist := f.customer(t, f.dealer, "ist", "tr", ptr(true))
	f.exec(t, `INSERT INTO customer_profiles (user_id, address) VALUES ($1, '{"country_id": 1, "province_id": 34}')`, ist.ID)
	bad := f.customer(t, f.dealer, "bad", "tr", ptr(true))
	f.exec(t, `INSERT INTO customer_profiles (user_id, address) VALUES ($1, '{"province_id": "unknown"}')`, bad.ID)
	f.customer(t, f.dealer, "de", "de", ptr(true))
	inherit := f.customer(t, f.dealer, "inherit", "", ptr(true))
	_ = inherit

	c := f.create(t, f.dealerC, AudienceFilter{AudienceType: AudienceCustomers, ProvinceIDs: []int64{34}})
	if p := f.preview(t, f.dealerC, c.UUID); p.Total != 1 {
		t.Fatalf("province filter total = %d, want 1", p.Total)
	}
	c = f.create(t, f.dealerC, AudienceFilter{AudienceType: AudienceCustomers, Locales: []string{"de"}})
	if p := f.preview(t, f.dealerC, c.UUID); p.Total != 1 || p.Locales[0] != (LocaleCount{"de", 1}) {
		t.Fatalf("locale filter preview = %+v", p)
	}
	// Without a user locale the serving organization's locale (tr) applies.
	c = f.create(t, f.dealerC, AudienceFilter{AudienceType: AudienceCustomers})
	p := f.preview(t, f.dealerC, c.UUID)
	want := []LocaleCount{{"tr", 3}, {"de", 1}}
	if !reflect.DeepEqual(p.Locales, want) {
		t.Fatalf("locales = %+v, want %+v", p.Locales, want)
	}
	_, err := f.svc.Create(f.ctx, f.dealerC, Input{Name: "x", Channels: []string{ChannelPush},
		AudienceFilter: &AudienceFilter{AudienceType: AudienceCustomers, LastServiceFrom: ptr("2026-05-01"), LastServiceTo: ptr("2026-01-01")}})
	wantValidation(t, err, "audience_filter.last_service_to")
	_, err = f.svc.Create(f.ctx, f.dealerC, Input{Name: "x", Channels: []string{ChannelPush},
		AudienceFilter: &AudienceFilter{AudienceType: AudienceCustomers, Locales: []string{"xx"}}})
	wantValidation(t, err, "audience_filter.locales")
}

// Acceptance: with de + tr in the audience and only tr content, the
// submission gate answers CAMPAIGN_LOCALE_MISSING listing de.
func TestCampaignLocalizationGate(t *testing.T) {
	f := newFixture(t)
	f.customer(t, f.dealer, "tr", "tr", ptr(true))
	f.customer(t, f.dealer, "de", "de", ptr(true))
	c := f.create(t, f.dealerC, AudienceFilter{AudienceType: AudienceCustomers}, ChannelPush, ChannelWhatsApp)
	if _, err := f.svc.PutContent(f.ctx, f.dealerC, c.UUID, "tr", ContentInput{Title: "Kampanya", Body: "İndirim"}); err != nil {
		t.Fatal(err)
	}
	err := f.svc.CheckLocalized(f.ctx, f.dealerC, c.UUID)
	var lm *LocaleMissingError
	if !errors.As(err, &lm) || !reflect.DeepEqual(lm.Locales, []string{"de"}) {
		t.Fatalf("gate err = %v, want locale missing [de]", err)
	}
	if p := f.preview(t, f.dealerC, c.UUID); !reflect.DeepEqual(p.MissingLocales, []string{"de"}) {
		t.Fatalf("preview missing = %v", p.MissingLocales)
	}
	// A de content without the push title is still incomplete.
	if _, err := f.svc.PutContent(f.ctx, f.dealerC, c.UUID, "de", ContentInput{Body: "Rabatt"}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.CheckLocalized(f.ctx, f.dealerC, c.UUID); !errors.As(err, &lm) {
		t.Fatalf("incomplete de err = %v", err)
	}
	if _, err := f.svc.PutContent(f.ctx, f.dealerC, c.UUID, "de", ContentInput{Title: "Aktion", Body: "Rabatt"}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.CheckLocalized(f.ctx, f.dealerC, c.UUID); err != nil {
		t.Fatalf("complete gate err = %v", err)
	}
	// Channel limits: push title <= 65, push body <= 240.
	_, err = f.svc.PutContent(f.ctx, f.dealerC, c.UUID, "tr", ContentInput{Title: strings.Repeat("a", 66), Body: "x"})
	wantValidation(t, err, "title")
	_, err = f.svc.PutContent(f.ctx, f.dealerC, c.UUID, "tr", ContentInput{Title: "a", Body: strings.Repeat("b", 241)})
	wantValidation(t, err, "body")
	// Switching to e-mail only accepts the longer body; a long WhatsApp-only
	// body above 4096 is refused.
	if _, err := f.svc.Update(f.ctx, f.dealerC, c.UUID, PatchInput{Channels: []string{ChannelEmail}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.PutContent(f.ctx, f.dealerC, c.UUID, "tr", ContentInput{Title: "Konu", Body: strings.Repeat("b", 1000)}); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.Update(f.ctx, f.dealerC, c.UUID, PatchInput{Channels: []string{ChannelPush}})
	wantValidation(t, err, "body")
	_, err = f.svc.PutContent(f.ctx, f.dealerC, c.UUID, "xx", ContentInput{Title: "a", Body: "b"})
	wantValidation(t, err, "locale")
}

func pngBytes(size int) []byte {
	b := make([]byte, size)
	copy(b, "\x89PNG\x0D\x0A\x1A\x0A")
	return b
}

// Acceptance: a 6 MB image is refused (400); types are sniffed from bytes.
func TestCampaignMedia(t *testing.T) {
	f := newFixture(t)
	c := f.create(t, f.dealerC, AudienceFilter{AudienceType: AudienceCustomers})

	_, err := f.svc.AddMedia(f.ctx, f.dealerC, c.UUID, "tr", MediaInput{Body: bytes.NewReader(pngBytes(6 << 20)), Filename: "big.png"})
	wantValidation(t, err, "file")
	_, err = f.svc.AddMedia(f.ctx, f.dealerC, c.UUID, "tr", MediaInput{Body: strings.NewReader("hello, not an image"), Filename: "fake.png"})
	wantValidation(t, err, "file")
	if len(f.keys(t)) != 0 {
		t.Fatalf("refused uploads stored objects: %v", f.keys(t))
	}

	img, err := f.svc.AddMedia(f.ctx, f.dealerC, c.UUID, "tr", MediaInput{Body: bytes.NewReader(pngBytes(1024)), Filename: `C:\x\banner.jpg`})
	if err != nil {
		t.Fatal(err)
	}
	if img.Kind != MediaImage || img.MimeType != "image/png" || img.FileName == nil || *img.FileName != "banner.jpg" {
		t.Fatalf("image = %+v", img)
	}
	pdf := append([]byte("%PDF-1.7\n"), make([]byte, 6<<20)...)
	doc, err := f.svc.AddMedia(f.ctx, f.dealerC, c.UUID, "de", MediaInput{Body: bytes.NewReader(pdf), Filename: "katalog.pdf"})
	if err != nil {
		t.Fatalf("6 MB pdf: %v", err)
	}
	if doc.Kind != MediaDocument || doc.MimeType != "application/pdf" {
		t.Fatalf("doc = %+v", doc)
	}
	got, err := f.svc.Get(f.ctx, f.dealerC, c.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Contents) != 2 || len(got.Contents[0].Media) != 1 || len(got.Contents[1].Media) != 1 {
		t.Fatalf("contents = %+v", got.Contents)
	}
	if len(f.keys(t)) != 2 {
		t.Fatalf("stored = %v", f.keys(t))
	}
	if err := f.svc.DeleteMedia(f.ctx, f.dealerC, c.UUID, "tr", doc.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete with wrong locale = %v", err)
	}
	if err := f.svc.DeleteMedia(f.ctx, f.dealerC, c.UUID, "tr", img.UUID); err != nil {
		t.Fatal(err)
	}
	if len(f.keys(t)) != 1 {
		t.Fatalf("after delete stored = %v", f.keys(t))
	}
}

// Acceptance: outside draft PATCH (and every other edit) is a 409 conflict.
func TestCampaignDraftOnly(t *testing.T) {
	f := newFixture(t)
	c := f.create(t, f.dealerC, AudienceFilter{AudienceType: AudienceCustomers})
	if _, err := f.svc.Update(f.ctx, f.dealerC, c.UUID, PatchInput{Name: ptr("Yeni ad")}); err != nil {
		t.Fatalf("draft patch: %v", err)
	}
	f.exec(t, `UPDATE campaigns SET status = 'pending_approval', approver_org_id = $2 WHERE uuid = $1`, c.UUID, f.dist.ID)
	if _, err := f.svc.Update(f.ctx, f.dealerC, c.UUID, PatchInput{Name: ptr("x")}); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("patch err = %v, want not draft", err)
	}
	if _, err := f.svc.PutContent(f.ctx, f.dealerC, c.UUID, "tr", ContentInput{Body: "x"}); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("content err = %v, want not draft", err)
	}
	if _, err := f.svc.AddMedia(f.ctx, f.dealerC, c.UUID, "tr", MediaInput{Body: bytes.NewReader(pngBytes(10))}); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("media err = %v, want not draft", err)
	}
	if err := f.svc.Delete(f.ctx, f.dealerC, c.UUID); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("delete err = %v, want not draft", err)
	}
	if _, err := f.svc.Update(f.ctx, f.otherDealerC, c.UUID, PatchInput{Name: ptr("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign patch err = %v, want not found", err)
	}

	d := f.create(t, f.dealerC, AudienceFilter{AudienceType: AudienceCustomers})
	if err := f.svc.Delete(f.ctx, f.dealerC, d.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Get(f.ctx, f.dealerC, d.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted get err = %v", err)
	}
}

// servedCustomer gives a consenting customer of the dealer a vehicle of
// carBrand, a completed service with one piece of product and a warranty
// on it valid until warrantyEnd.
func (f *fixture) servedCustomer(t *testing.T, name string, carBrand, carModel int64, product db.Product, warrantyEnd time.Time) db.User {
	t.Helper()
	u := f.customer(t, f.dealer, name, "tr", ptr(true))
	v, err := f.q.CreateVehicle(f.ctx, db.CreateVehicleParams{
		UserID: u.ID, BrandID: f.brandID,
		CarBrandID: pgtype.Int8{Int64: carBrand, Valid: true}, CarModelID: pgtype.Int8{Int64: carModel, Valid: true},
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	f.seq++
	svc, err := f.q.CreateService(f.ctx, db.CreateServiceParams{
		ServiceNo: fmt.Sprintf("T405-%d-%d", time.Now().UnixNano(), f.seq), OrganizationID: f.dealer.ID, BrandID: f.brandID,
		CustomerUserID: u.ID, VehicleID: v.ID, CarBrandID: carBrand, CarModelID: carModel, Status: "draft",
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	unit, err := f.q.CreateUnit(f.ctx, db.CreateUnitParams{
		OrganizationID: f.center.ID, BrandID: f.brandID, ProductID: product.ID,
		Barcode: fmt.Sprintf("T405-%d-%d", time.Now().UnixNano(), f.seq), UnitKind: "serial", Source: "generated", Status: "available",
	})
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	item, err := f.q.CreateServiceItem(f.ctx, db.CreateServiceItemParams{
		ServiceID: svc.ID, ProductID: product.ID, UnitID: unit.ID, Kind: "full", AppliedParts: []byte(`[]`),
	})
	if err != nil {
		t.Fatalf("service item: %v", err)
	}
	if _, err := f.q.CompleteService(f.ctx, db.CompleteServiceParams{ID: svc.ID}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := f.q.CreateWarrantyForServiceItem(f.ctx, db.CreateWarrantyForServiceItemParams{
		ServiceItemID: item.ID, HolderUserID: u.ID,
		StartAt: pgtype.Timestamptz{Time: warrantyEnd.AddDate(-1, 0, 0), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: warrantyEnd, Valid: true},
	}); err != nil {
		t.Fatalf("warranty: %v", err)
	}
	return u
}

// Service based filters: last service date, car brand, product, category
// and warranty state (active / ending within 30 days / expired).
func TestCampaignAudienceServiceFilters(t *testing.T) {
	f := newFixture(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var carBrand, otherBrand, carModel, otherModel int64
	var carBrandUUID uuid.UUID
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id, uuid`, "T405 "+suffix).Scan(&carBrand, &carBrandUUID); err != nil {
		t.Fatal(err)
	}
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, "T405b "+suffix).Scan(&otherBrand); err != nil {
		t.Fatal(err)
	}
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, 'M') RETURNING id`, carBrand).Scan(&carModel); err != nil {
		t.Fatal(err)
	}
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, 'M') RETURNING id`, otherBrand).Scan(&otherModel); err != nil {
		t.Fatal(err)
	}
	cat, err := f.q.CreateProductCategory(f.ctx, db.CreateProductCategoryParams{
		OrganizationID: f.center.ID, BrandID: f.brandID, Name: "t405-cat-" + suffix, AvailableParts: []byte(`[]`), Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	product := func(sku string) db.Product {
		p, err := f.q.CreateProduct(f.ctx, db.CreateProductParams{
			OrganizationID: f.center.ID, BrandID: f.brandID, CategoryID: cat.ID,
			Sku: sku + "-" + suffix, Name: sku, Images: []byte("[]"), UnitType: "piece", Active: true,
		})
		if err != nil {
			t.Fatalf("product: %v", err)
		}
		return p
	}
	ppf, glass := product("t405-ppf"), product("t405-glass")
	now := time.Now().UTC()
	f.svc.SetClock(func() time.Time { return now })
	f.servedCustomer(t, "expiring", carBrand, carModel, ppf, now.AddDate(0, 0, 10))
	f.servedCustomer(t, "active", otherBrand, otherModel, glass, now.AddDate(0, 6, 0))
	f.servedCustomer(t, "expired", otherBrand, otherModel, ppf, now.AddDate(0, 0, -3))
	f.customer(t, f.dealer, "never", "tr", ptr(true))

	count := func(filter AudienceFilter) int {
		t.Helper()
		filter.AudienceType = AudienceCustomers
		c := f.create(t, f.dealerC, filter)
		return f.preview(t, f.dealerC, c.UUID).Total
	}
	today, yesterday := now.Format(dateLayout), now.AddDate(0, 0, -1).Format(dateLayout)
	cases := []struct {
		name   string
		filter AudienceFilter
		want   int
	}{
		{"all", AudienceFilter{}, 4},
		{"car brand", AudienceFilter{CarBrandUUIDs: []uuid.UUID{carBrandUUID}}, 1},
		{"product", AudienceFilter{ProductUUIDs: []uuid.UUID{ppf.Uuid}}, 2},
		{"category", AudienceFilter{CategoryUUIDs: []uuid.UUID{cat.Uuid}}, 3},
		{"warranty active", AudienceFilter{WarrantyStatuses: []string{WarrantyActive}}, 2},
		{"warranty expiring", AudienceFilter{WarrantyStatuses: []string{WarrantyExpiring}}, 1},
		{"warranty expired", AudienceFilter{WarrantyStatuses: []string{WarrantyExpired}}, 1},
		{"warranty expiring or expired", AudienceFilter{WarrantyStatuses: []string{WarrantyExpiring, WarrantyExpired}}, 2},
		{"served today", AudienceFilter{LastServiceFrom: &today, LastServiceTo: &today}, 3},
		{"served until yesterday", AudienceFilter{LastServiceTo: &yesterday}, 0},
	}
	for _, tc := range cases {
		if got := count(tc.filter); got != tc.want {
			t.Errorf("%s: total = %d, want %d", tc.name, got, tc.want)
		}
	}
	// Another dealer's campaign sees none of these customers.
	c := f.create(t, f.otherDealerC, AudienceFilter{AudienceType: AudienceCustomers, ProductUUIDs: []uuid.UUID{ppf.Uuid}})
	if p := f.preview(t, f.otherDealerC, c.UUID); p.Total != 0 {
		t.Fatalf("foreign dealer total = %d", p.Total)
	}
}
