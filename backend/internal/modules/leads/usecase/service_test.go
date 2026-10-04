package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	customeruc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	orguc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	serviceuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	t         *testing.T
	ctx       context.Context
	pool      *pgxpool.Pool
	tx        pgx.Tx
	q         *db.Queries
	svc       *Service
	brand     int64
	brandUUID uuid.UUID
	center    db.Organization
	dist      db.Organization
	dealer    db.Organization
	other     db.Organization
	user      db.User
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
	if err := tx.QueryRow(ctx, `SELECT id, uuid FROM brands WHERE slug = 'olex'`).Scan(&f.brand, &f.brandUUID); err != nil {
		t.Fatalf("brand: %v", err)
	}
	f.ctx = brandctx.WithBrand(f.ctx, brandctx.Brand{ID: f.brand, UUID: f.brandUUID, Slug: "olex", Name: "Olex", Status: "active"})
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
	f.dist = f.org("dist-a", "distributor", f.center)
	f.dealer = f.org("dealer-a", "dealer", f.dist)
	f.other = f.org("dealer-b", "dealer", f.center)
	f.user = f.userRow("lead-user")
	f.svc = New(tx, q, nil)
	f.svc.SetConverters(customeruc.New(tx, q, nil, nil), serviceuc.New(tx, q, outbox.NewMemory()), orguc.New(nil, q))
	f.svc.SetClock(func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) })
	return f
}

func (f *fixture) org(slug, typ string, parent db.Organization) db.Organization {
	f.t.Helper()
	slug = fmt.Sprintf("tec313-%s-%d", slug, time.Now().UnixNano())
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: slug, Name: slug, Status: "active", Type: typ,
		ParentID: pgtype.Int8{Int64: parent.ID, Valid: true}, BrandID: f.brand,
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
	perms := map[string]rbac.Scope{rbac.PermLeadsRead: scope, rbac.PermLeadsWrite: scope}
	if org.Type != "dealer" {
		perms[rbac.PermLeadsConvertOrg] = scope
	}
	return Caller{
		Principal: authctx.Principal{UserInternal: f.user.ID, PermissionScopes: perms, IsSuperAdmin: org.Type == "center"},
		Org:       orgctx.Scope{InternalID: org.ID, UUID: org.Uuid, OrgType: org.Type, BrandID: org.BrandID},
		Filter:    scopefilter.Filter{Permission: rbac.PermLeadsRead, Scope: scope, UserID: f.user.ID, OrgID: org.ID, OrgIDs: orgIDs, BrandID: org.BrandID},
	}
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

func TestConvertDealerCandidateCreatesReadOnlyDealerUnderDistributor(t *testing.T) {
	f := newFixture(t)
	c := f.caller(f.dist, rbac.ScopeSubtree)
	phone := fmt.Sprintf("+90555%07d", time.Now().UnixNano()%10000000)
	lead, err := f.svc.Create(f.ctx, c, CreateInput{
		TargetType: "dealer_candidate", Source: "website", Temperature: "hot",
		CandidateCompanyName: strp("Lead Bayi"), CandidateContactName: strp("Ayse Owner"),
		CandidatePhoneE164: &phone,
	})
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}
	out, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDealerCandidate})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if out.Organization == nil || out.Organization.Type != "dealer" || out.Organization.Status != "read_only" {
		t.Fatalf("organization = %+v, want read_only dealer", out.Organization)
	}
	if out.Organization.Parent == nil || out.Organization.Parent.UUID != f.dist.Uuid {
		t.Fatalf("parent = %+v, want distributor %s", out.Organization.Parent, f.dist.Uuid)
	}
	got, err := f.svc.Get(f.ctx, c, lead.UUID)
	if err != nil {
		t.Fatalf("get converted: %v", err)
	}
	if got.Status != StatusWon || got.WonRefType == nil || *got.WonRefType != WonRefOrganization || got.WonRefID == nil {
		t.Fatalf("lead won ref = %+v", got)
	}
}

func TestConvertDistributorCandidateDealerForbidden(t *testing.T) {
	f := newFixture(t)
	c := f.caller(f.dealer, rbac.ScopeManaged)
	phone := fmt.Sprintf("+90556%07d", time.Now().UnixNano()%10000000)
	lead, err := f.svc.Create(f.ctx, c, CreateInput{
		TargetType: "distributor_candidate", Source: "website",
		CandidateCompanyName: strp("Lead Dist"), CandidateContactName: strp("Deniz Owner"),
		CandidatePhoneE164: &phone,
	})
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}
	if _, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDistributorCandidate, Currency: "EUR"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("convert err = %v, want forbidden", err)
	}
}

func TestConvertCustomerUsesExistingPhoneAndCreatesDraft(t *testing.T) {
	f := newFixture(t)
	c := f.caller(f.dealer, rbac.ScopeManaged)
	existing := f.userWithPhone("lead-customer", "+905551234567")
	vehicle := f.vehicle(existing, f.dealer)
	lead, err := f.svc.Create(f.ctx, c, CreateInput{
		TargetType: "customer", Source: "walk_in", Temperature: "warm",
		CandidateContactName: strp("Different Name"), CandidatePhoneE164: strp("+905551234567"),
		VehicleID: &vehicle.ID,
	})
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}
	out, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindCustomer})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if out.ServiceDraft == nil || out.ServiceDraft.Status != serviceuc.StatusDraft {
		t.Fatalf("draft = %+v", out.ServiceDraft)
	}
	users, err := f.tx.Query(f.ctx, `SELECT id FROM users WHERE phone_e164 = $1 AND deleted_at IS NULL`, "+905551234567")
	if err != nil {
		t.Fatal(err)
	}
	defer users.Close()
	count := 0
	for users.Next() {
		count++
	}
	if count != 1 {
		t.Fatalf("users with phone = %d, want 1", count)
	}
	if out.Lead.Status != StatusWon || out.Lead.WonRefType == nil || *out.Lead.WonRefType != WonRefService {
		t.Fatalf("lead = %+v", out.Lead)
	}
	if _, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindCustomer}); !errors.Is(err, ErrAlreadyConverted) {
		t.Fatalf("second convert err = %v, want already converted", err)
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

func (f *fixture) userWithPhone(prefix, ph string) db.User {
	f.t.Helper()
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		Email:        pgtype.Text{String: fmt.Sprintf("%s-%d@example.test", prefix, time.Now().UnixNano()), Valid: true},
		PasswordHash: "x", Name: prefix, Surname: "User", Status: "active",
		PhoneE164: pgtype.Text{String: ph, Valid: true},
	})
	if err != nil {
		f.t.Fatalf("user with phone: %v", err)
	}
	return u
}

func (f *fixture) vehicle(u db.User, org db.Organization) db.Vehicle {
	f.t.Helper()
	brand, err := f.q.CreateCarBrand(f.ctx, db.CreateCarBrandParams{
		ExternalID: pgtype.Text{String: fmt.Sprintf("tec316-brand-%d", time.Now().UnixNano()), Valid: true},
		Name:       fmt.Sprintf("TEC316 Brand %d", time.Now().UnixNano()),
		ShowName:   true, Active: true,
	})
	if err != nil {
		f.t.Fatalf("car brand: %v", err)
	}
	model, err := f.q.CreateCarModel(f.ctx, db.CreateCarModelParams{
		CarBrandID: brand.ID, ExternalID: pgtype.Text{String: fmt.Sprintf("tec316-model-%d", time.Now().UnixNano()), Valid: true},
		Name: "Model", Active: true,
	})
	if err != nil {
		f.t.Fatalf("car model: %v", err)
	}
	v, err := f.q.CreateVehicle(f.ctx, db.CreateVehicleParams{
		UserID: u.ID, OrganizationID: pgtype.Int8{Int64: org.ID, Valid: true}, BrandID: org.BrandID,
		CarBrandID: pgtype.Int8{Int64: brand.ID, Valid: true}, CarModelID: pgtype.Int8{Int64: model.ID, Valid: true},
		Plate: pgtype.Text{String: "34 TEC 316", Valid: true}, PlateNormalized: pgtype.Text{String: "34TEC316", Valid: true},
		PlateCountry: pgtype.Text{String: "TR", Valid: true},
	})
	if err != nil {
		f.t.Fatalf("vehicle: %v", err)
	}
	return v
}

func strp(v string) *string { return &v }

func uniquePhone(prefix string) string {
	return fmt.Sprintf("+90%s%07d", prefix, time.Now().UnixNano()%10000000)
}

func (f *fixture) candidateLead(c Caller, target, ph string) Lead {
	f.t.Helper()
	lead, err := f.svc.Create(f.ctx, c, CreateInput{
		TargetType: target, Source: "website", Temperature: "hot",
		CandidateCompanyName: strp(fmt.Sprintf("Lead Org %d", time.Now().UnixNano())), CandidateContactName: strp("Ayse Owner"),
		CandidatePhoneE164: &ph,
	})
	if err != nil {
		f.t.Fatalf("create lead: %v", err)
	}
	return lead
}

func (f *fixture) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.tx.QueryRow(f.ctx, sql, args...).Scan(&n); err != nil {
		f.t.Fatalf("count: %v", err)
	}
	return n
}

func TestConvertDealerCandidateByDealerForbidden(t *testing.T) {
	f := newFixture(t)
	c := f.caller(f.dealer, rbac.ScopeManaged)
	ph := uniquePhone("557")
	lead := f.candidateLead(c, "dealer_candidate", ph)
	if _, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDealerCandidate}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("convert err = %v, want forbidden", err)
	}
	if n := f.count(`SELECT count(*) FROM users WHERE phone_e164 = $1`, ph); n != 0 {
		t.Fatalf("users created = %d, want 0", n)
	}
}

func TestConvertCenterDealerCandidateRequiresDistributor(t *testing.T) {
	f := newFixture(t)
	c := f.caller(f.center, rbac.ScopeBrand)
	ph := uniquePhone("558")
	lead := f.candidateLead(c, "dealer_candidate", ph)
	var ve *ValidationError
	if _, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDealerCandidate}); !errors.As(err, &ve) || ve.Field != "distributor_uuid" {
		t.Fatalf("convert without distributor err = %v, want distributor_uuid validation", err)
	}
	// A dealer is not a valid parent either (no dealer directly under center, no dealer under dealer).
	if _, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDealerCandidate, DistributorUUID: &f.dealer.Uuid}); !errors.As(err, &ve) {
		t.Fatalf("convert with dealer parent err = %v, want validation", err)
	}
	if n := f.count(`SELECT count(*) FROM users WHERE phone_e164 = $1`, ph); n != 0 {
		t.Fatalf("users created = %d, want 0", n)
	}
	out, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDealerCandidate, DistributorUUID: &f.dist.Uuid})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if out.Organization == nil || out.Organization.Status != "read_only" || out.Organization.Parent == nil || out.Organization.Parent.UUID != f.dist.Uuid {
		t.Fatalf("organization = %+v, want read_only dealer under distributor", out.Organization)
	}
	var owner int
	if err := f.tx.QueryRow(f.ctx, `SELECT count(*) FROM organization_members m JOIN users u ON u.id = m.user_id
		JOIN organizations o ON o.id = m.organization_id WHERE o.uuid = $1 AND u.phone_e164 = $2 AND m.role = 'owner'`,
		out.Organization.UUID, ph).Scan(&owner); err != nil || owner != 1 {
		t.Fatalf("owner membership = %d (%v), want 1", owner, err)
	}
}

func TestConvertDistributorCandidateByCenter(t *testing.T) {
	f := newFixture(t)
	c := f.caller(f.center, rbac.ScopeBrand)
	lead := f.candidateLead(c, "distributor_candidate", uniquePhone("559"))
	var ve *ValidationError
	if _, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDistributorCandidate}); !errors.As(err, &ve) || ve.Field != "currency" {
		t.Fatalf("convert without currency err = %v, want currency validation", err)
	}
	if _, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDistributorCandidate, Currency: "XX"}); !errors.As(err, &ve) {
		t.Fatalf("convert with bad currency err = %v, want validation", err)
	}
	// center staff (not super admin) holding leads.convert_org is still refused.
	staff := c
	staff.Principal.IsSuperAdmin = false
	if _, err := f.svc.Convert(f.ctx, staff, lead.UUID, ConvertInput{Kind: ConvertKindDistributorCandidate, Currency: "EUR"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("center staff convert err = %v, want forbidden", err)
	}
	out, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDistributorCandidate, Currency: "eur"})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if out.Organization == nil || out.Organization.Type != "distributor" || out.Organization.Currency != "EUR" || out.Organization.Status != "active" {
		t.Fatalf("organization = %+v", out.Organization)
	}
}

func TestConvertOrgCandidatePhoneConflictReturnsMaskedUser(t *testing.T) {
	f := newFixture(t)
	c := f.caller(f.dist, rbac.ScopeSubtree)
	ph := uniquePhone("551")
	existing := f.userWithPhone("lead-conflict", ph)
	lead := f.candidateLead(c, "dealer_candidate", ph)
	var conflict *UserConflictError
	if _, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDealerCandidate}); !errors.As(err, &conflict) {
		t.Fatalf("convert err = %v, want user conflict", err)
	}
	if conflict.User.UUID != existing.Uuid || conflict.User.PhoneMasked == nil || *conflict.User.PhoneMasked == ph ||
		conflict.User.EmailMasked == nil || *conflict.User.EmailMasked == existing.Email.String {
		t.Fatalf("existing user = %+v, want masked contact", conflict.User)
	}
	got, err := f.svc.Get(f.ctx, c, lead.UUID)
	if err != nil || got.Status == StatusWon {
		t.Fatalf("lead after conflict = %+v (%v)", got, err)
	}
}

// A failure after the customer/user was created rolls everything back.
func TestConvertCustomerFailureRollsBack(t *testing.T) {
	f := newFixture(t)
	c := f.caller(f.dealer, rbac.ScopeManaged)
	other := f.userWithPhone("lead-vehicle-owner", uniquePhone("552"))
	vehicle := f.vehicle(other, f.dealer)
	ph := uniquePhone("553")
	lead, err := f.svc.Create(f.ctx, c, CreateInput{
		TargetType: "customer", Source: "walk_in", Temperature: "warm",
		CandidateContactName: strp("New Customer"), CandidatePhoneE164: &ph, VehicleID: &vehicle.ID,
	})
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}
	var ve *ValidationError
	if _, err := f.svc.Convert(f.ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindCustomer}); !errors.As(err, &ve) || ve.Field != "vehicle_id" {
		t.Fatalf("convert err = %v, want vehicle_id validation", err)
	}
	if n := f.count(`SELECT count(*) FROM users WHERE phone_e164 = $1`, ph); n != 0 {
		t.Fatalf("customer user left behind = %d, want 0", n)
	}
	if n := f.count(`SELECT count(*) FROM lead_events WHERE lead_id = (SELECT id FROM leads WHERE uuid = $1) AND event_type = 'converted'`, lead.UUID); n != 0 {
		t.Fatalf("converted events = %d, want 0", n)
	}
}
