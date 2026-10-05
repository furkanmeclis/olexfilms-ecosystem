package usecase

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	contractrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/repository"
	contractuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type subEnv struct {
	ctx                         context.Context
	pool                        *pgxpool.Pool
	q                           *db.Queries
	svc                         *Service
	actor                       db.User
	features                    *features.Service
	center, dist, other, dealer db.Organization
	suffix                      string
}

func newSubEnv(t *testing.T) *subEnv {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	feat := features.New(pool, q, features.NoCache{}, nil)
	if err := feat.SyncCatalog(ctx); err != nil {
		t.Fatalf("sync modules: %v", err)
	}
	e := &subEnv{ctx: ctx, pool: pool, q: q, center: center, features: feat, suffix: fmt.Sprint(time.Now().UnixNano())}
	e.actor = e.user(t, "staff")
	e.dist = e.org(t, "dist", "distributor", center.ID)
	e.other = e.org(t, "other", "distributor", center.ID)
	e.dealer = e.org(t, "dealer", "dealer", e.dist.ID)
	e.svc = New(q).WithLifecycle(pool, outbox.NewStore(pool, q), fxrates.New(q, nil, nil), feat)
	return e
}

func (e *subEnv) org(t *testing.T, name, typ string, parent int64) db.Organization {
	t.Helper()
	o, err := e.q.CreateOrganization(e.ctx, db.CreateOrganizationParams{
		Slug: "t307-" + name + "-" + e.suffix, Name: name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent, Valid: true},
		BrandID: e.center.BrandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	owner := e.user(t, name+"-owner")
	if _, err := e.q.CreateOrganizationMember(e.ctx, db.CreateOrganizationMemberParams{
		OrganizationID: o.ID, UserID: owner.ID, Role: "owner",
	}); err != nil {
		t.Fatalf("org %s owner: %v", name, err)
	}
	return o
}

func (e *subEnv) item(t *testing.T, category, price string) db.ServiceCatalogItem {
	t.Helper()
	it, err := e.q.CreateServiceCatalogItem(e.ctx, db.CreateServiceCatalogItemParams{
		OrganizationID: e.center.ID, BrandID: e.center.BrandID, Name: "Subscription " + category + e.suffix,
		Category: category, DefaultPrice: num(price), Currency: "TRY", Recurrence: "monthly",
		CancellationFee: num("25.00"), IsActive: true,
	})
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	return it
}

func (e *subEnv) itemWithTemplate(t *testing.T, tplID int64) db.ServiceCatalogItem {
	t.Helper()
	it, err := e.q.CreateServiceCatalogItem(e.ctx, db.CreateServiceCatalogItemParams{
		OrganizationID: e.center.ID, BrandID: e.center.BrandID, Name: "Subscription contract " + e.suffix,
		Category: "software", DefaultPrice: num("100.00"), Currency: "TRY", Recurrence: "monthly",
		CancellationFee: num("25.00"), ContractTemplateID: pgtype.Int8{Int64: tplID, Valid: true}, IsActive: true,
	})
	if err != nil {
		t.Fatalf("item with template: %v", err)
	}
	return it
}

func (e *subEnv) user(t *testing.T, kind string) db.User {
	t.Helper()
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	phoneSuffix := fmt.Sprintf("%09d", time.Now().UnixNano()%1_000_000_000)
	row, err := e.q.CreateUser(e.ctx, db.CreateUserParams{
		Email: pgText(kind + "." + id + "@example.test"), PasswordHash: "x",
		Name: strings.ToUpper(kind[:1]) + kind[1:], Surname: "TEC309", Status: "active",
		PhoneE164: pgText("+90554" + phoneSuffix), PhoneVerifiedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		t.Fatalf("user %s: %v", kind, err)
	}
	return row
}

func (e *subEnv) serviceSaleTemplate(t *testing.T) db.ContractTemplate {
	t.Helper()
	tpl, err := e.q.CreateContractTemplate(e.ctx, db.CreateContractTemplateParams{
		OrganizationID: e.center.ID, BrandID: e.center.BrandID,
		Name: "TEC309 service sale " + uuid.NewString(), Kind: "service_sale",
		IsDefault: false, OtpRequired: true, SignatureRequired: true,
		IsActive: true, CreatedByUserID: pgtype.Int8{Int64: e.actor.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("contract template: %v", err)
	}
	if _, err := e.q.UpsertContractTemplateLocale(e.ctx, db.UpsertContractTemplateLocaleParams{
		TemplateID: tpl.ID, OrganizationID: tpl.OrganizationID, BrandID: tpl.BrandID,
		Locale: "tr", Html: `<p>{{org_name}} {{service_name}} {{start_date}} {{end_date}} {{price}} {{plate}}{{vin}}{{vehicle_label}}</p>`,
		UpdatedByUserID: pgtype.Int8{Int64: e.actor.ID, Valid: true},
	}); err != nil {
		t.Fatalf("contract template locale: %v", err)
	}
	return tpl
}

func (e *subEnv) vehicleIntakeTemplate(t *testing.T) db.ContractTemplate {
	t.Helper()
	tpl, err := e.q.CreateContractTemplate(e.ctx, db.CreateContractTemplateParams{
		OrganizationID: e.center.ID, BrandID: e.center.BrandID,
		Name: "TEC309 vehicle intake " + uuid.NewString(), Kind: "vehicle_intake",
		IsDefault: false, OtpRequired: true, SignatureRequired: true,
		IsActive: true, CreatedByUserID: pgtype.Int8{Int64: e.actor.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("vehicle template: %v", err)
	}
	return tpl
}

func (e *subEnv) caller(org db.Organization, scope rbac.Scope, perm string) Caller {
	return Caller{
		Principal: authctx.Principal{UserInternal: e.actor.ID, PermissionScopes: map[string]rbac.Scope{perm: scope}},
		Org: orgctx.Scope{
			InternalID: org.ID, UUID: org.Uuid, OrgType: org.Type, BrandID: org.BrandID, Name: org.Name,
		},
		Filter: scopefilter.Filter{Permission: perm, Scope: scope, OrgID: org.ID, OrgIDs: []int64{org.ID}, BrandID: org.BrandID},
	}
}

func TestSubscriptionContractCreatedForTemplatedItem(t *testing.T) {
	e := newSubEnv(t)
	start, end := subDates()
	tpl := e.serviceSaleTemplate(t)
	item := e.itemWithTemplate(t, tpl.ID)

	sub, err := e.svc.Assign(e.ctx, e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsAssign), SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: start, EndsOn: end,
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if sub.Contract == nil || sub.Contract.Status != "pending" {
		t.Fatalf("contract summary = %+v, want pending", sub.Contract)
	}
	stored, err := e.q.GetServiceSubscriptionByUUID(e.ctx, db.GetServiceSubscriptionByUUIDParams{Uuid: sub.UUID, BrandID: e.center.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	if !stored.ContractID.Valid {
		t.Fatal("subscription contract_id was not linked")
	}
	inst, err := e.q.GetContractInstanceByID(e.ctx, stored.ContractID.Int64)
	if err != nil {
		t.Fatal(err)
	}
	if inst.SubjectType != "service_subscription" || inst.SubjectID != stored.ID || inst.Kind != "service_sale" {
		t.Fatalf("contract subject/kind = %s/%d/%s", inst.SubjectType, inst.SubjectID, inst.Kind)
	}
	signers, err := e.q.ListContractSigners(e.ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(signers) != 2 || signers[0].Role != "customer" || signers[1].Role != "staff" {
		t.Fatalf("signers = %+v, want customer/staff", signers)
	}
	if !signers[0].PhoneE164.Valid || signers[0].UserID.Int64 == e.actor.ID || signers[1].UserID.Int64 != e.actor.ID {
		t.Fatalf("signer users = %+v", signers)
	}
}

func TestSubscriptionWithoutTemplateDoesNotCreateContract(t *testing.T) {
	e := newSubEnv(t)
	start, end := subDates()
	item := e.item(t, "software", "100.00")

	sub, err := e.svc.Assign(e.ctx, e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsAssign), SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: start, EndsOn: end,
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if sub.Contract != nil {
		t.Fatalf("contract summary = %+v, want nil", sub.Contract)
	}
	stored, err := e.q.GetServiceSubscriptionByUUID(e.ctx, db.GetServiceSubscriptionByUUIDParams{Uuid: sub.UUID, BrandID: e.center.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	if stored.ContractID.Valid {
		t.Fatalf("contract_id = %d, want null", stored.ContractID.Int64)
	}
}

func TestSubscriptionContractSigningExecutesAndEmitsPDFEvent(t *testing.T) {
	e := newSubEnv(t)
	start, end := subDates()
	item := e.itemWithTemplate(t, e.serviceSaleTemplate(t).ID)
	sub, err := e.svc.Assign(e.ctx, e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsAssign), SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: start, EndsOn: end,
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if sub.Contract == nil {
		t.Fatal("contract summary is nil")
	}
	now := time.Now().UTC().Truncate(time.Second)
	memOut := outbox.NewMemory()
	contracts := contractuc.New(contractrepo.New(e.pool, e.q),
		contractuc.WithOTP(&subscriptionFakeOTP{q: e.q, now: now, createdAt: now}),
		contractuc.WithStorage(storage.NewMemory()),
		contractuc.WithOutbox(memOut),
		contractuc.WithClock(func() time.Time { return now }),
	)
	caller := contractuc.Caller{
		UserID: e.actor.ID, OrganizationID: e.dealer.ID, BrandID: e.dealer.BrandID,
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgID: e.dealer.ID, OrgIDs: []int64{e.dealer.ID}, BrandID: e.dealer.BrandID, UserID: e.actor.ID},
	}
	if _, err := contracts.SignCustomer(e.ctx, caller, sub.Contract.UUID, contractuc.SignatureInput{
		Code: "123456", PNGBase64: subscriptionPNGBase64(),
	}); err != nil {
		t.Fatalf("customer sign: %v", err)
	}
	got, err := contracts.SignStaff(e.ctx, caller, sub.Contract.UUID, contractuc.SignatureInput{PNGBase64: subscriptionPNGBase64()})
	if err != nil {
		t.Fatalf("staff sign: %v", err)
	}
	if got.Status != "executed" || got.ExecutedAt == nil {
		t.Fatalf("contract = %+v, want executed", got)
	}
	if rows := memOut.All(); len(rows) != 1 || rows[0].EventName != "contract.executed" {
		t.Fatalf("outbox rows = %+v, want one contract.executed", rows)
	}
}

func TestVehicleIntakeTemplateCannotBeAttachedToCatalogItem(t *testing.T) {
	e := newSubEnv(t)
	tpl := e.vehicleIntakeTemplate(t)
	_, err := e.svc.Create(e.ctx, orgctx.Scope{
		InternalID: e.center.ID, UUID: e.center.Uuid, OrgType: e.center.Type, BrandID: e.center.BrandID, Name: e.center.Name,
	}, ItemInput{
		Name: strPtr("Bad template " + e.suffix), Category: strPtr("software"), DefaultPrice: strPtr("10.00"),
		Currency: strPtr("TRY"), Recurrence: strPtr("monthly"), ContractTemplateID: &tpl.ID,
	})
	if !isValidation(err, "contract_template_id") {
		t.Fatalf("create err = %v, want contract_template_id validation", err)
	}
}

func subDates() (time.Time, time.Time) {
	return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 10, 4, 0, 0, 0, 0, time.UTC)
}

func TestSubscriptionLifecycleAcceptance(t *testing.T) {
	e := newSubEnv(t)
	start, end := subDates()
	item := e.item(t, "software", "100.00")
	if _, err := e.q.UpsertServicePriceOverride(e.ctx, db.UpsertServicePriceOverrideParams{
		ItemID: item.ID, OrganizationID: e.dist.ID, BrandID: item.BrandID, Price: num("80.00"), Currency: "TRY",
	}); err != nil {
		t.Fatalf("override: %v", err)
	}
	dist := e.caller(e.dist, rbac.ScopeSubtree, rbac.PermServiceSubscriptionsAssign)

	if _, err := e.svc.Assign(e.ctx, dist, SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: e.other.Uuid, StartsOn: start, EndsOn: end,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outside subtree = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.Assign(e.ctx, dist, SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: end, EndsOn: start,
	}); !isValidation(err, "ends_on") {
		t.Fatalf("bad dates = %v, want ends_on validation", err)
	}

	sub, err := e.svc.Assign(e.ctx, dist, SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: start, EndsOn: end,
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if sub.Price != "80.00" {
		t.Fatalf("snapshot price = %s, want 80.00", sub.Price)
	}
	if _, err := e.q.UpsertServicePriceOverride(e.ctx, db.UpsertServicePriceOverrideParams{
		ItemID: item.ID, OrganizationID: e.dist.ID, BrandID: item.BrandID, Price: num("120.00"), Currency: "TRY",
	}); err != nil {
		t.Fatalf("override change: %v", err)
	}
	got, err := e.svc.GetSubscription(e.ctx, e.caller(e.dealer, rbac.ScopeManaged, rbac.PermServiceSubscriptionsRead), sub.UUID)
	if err != nil || got.Price != "80.00" {
		t.Fatalf("stored snapshot = %+v, %v", got, err)
	}

	req, err := e.svc.RequestCancel(e.ctx, e.caller(e.dealer, rbac.ScopeManaged, rbac.PermServiceSubscriptionsCancelRequest),
		sub.UUID, CancelRequestInput{Reason: "not needed"})
	if err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	center := e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsCancelApprove)
	decided, err := e.svc.ApproveCancel(e.ctx, center, req.UUID, DecisionInput{})
	if err != nil || decided.Status != "approved" {
		t.Fatalf("approve = %+v, %v", decided, err)
	}
	if _, err := e.svc.ApproveCancel(e.ctx, center, req.UUID, DecisionInput{}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("second approve = %v, want ErrInvalidStatus", err)
	}
	var n int
	if err := e.pool.QueryRow(e.ctx, `
		SELECT count(*) FROM outbox_events
		WHERE event_name = $1 AND payload->'data'->>'subscription_uuid' = $2`,
		"service_subscription.cancelled", sub.UUID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("cancelled outbox events = %d, want 1", n)
	}
}

func TestModuleBundleSubscriptionTogglesFeature(t *testing.T) {
	e := newSubEnv(t)
	start, end := subDates()
	item := e.item(t, CategoryModuleBundle, "50.00")
	if err := e.q.AddServiceCatalogModule(e.ctx, db.AddServiceCatalogModuleParams{
		ItemID: item.ID, ModuleKey: features.ModuleStockForecast,
	}); err != nil {
		t.Fatalf("bundle module: %v", err)
	}
	if _, err := e.features.SetByAdmin(e.ctx, 0, e.dist.ID, features.ModuleStockForecast, true); err != nil {
		t.Fatalf("open parent: %v", err)
	}
	if ok, err := e.features.Enabled(e.ctx, e.dealer.ID, features.ModuleStockForecast); err != nil || ok {
		t.Fatalf("before subscription feature = %v, %v", ok, err)
	}
	sub, err := e.svc.Assign(e.ctx, e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsAssign), SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: start, EndsOn: end,
	})
	if err != nil {
		t.Fatalf("assign bundle: %v", err)
	}
	if ok, err := e.features.Enabled(e.ctx, e.dealer.ID, features.ModuleStockForecast); err != nil || !ok {
		t.Fatalf("after subscription feature = %v, %v", ok, err)
	}
	req, err := e.svc.RequestCancel(e.ctx, e.caller(e.dealer, rbac.ScopeManaged, rbac.PermServiceSubscriptionsCancelRequest),
		sub.UUID, CancelRequestInput{Reason: "done"})
	if err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	if _, err := e.svc.ApproveCancel(e.ctx, e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsCancelApprove), req.UUID, DecisionInput{}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if ok, err := e.features.Enabled(e.ctx, e.dealer.ID, features.ModuleStockForecast); err != nil || ok {
		t.Fatalf("after cancel feature = %v, %v", ok, err)
	}
}

func isValidation(err error, field string) bool {
	var ve *ValidationError
	return errors.As(err, &ve) && ve.Field == field
}

type subscriptionFakeOTP struct {
	q         *db.Queries
	now       time.Time
	createdAt time.Time
}

func (f *subscriptionFakeOTP) Request(context.Context, otp.RequestInput) (otp.RequestResult, error) {
	return otp.RequestResult{Channel: "fake", ExpiresAt: f.now.Add(5 * time.Minute), ResendAt: f.now.Add(time.Minute)}, nil
}

func (f *subscriptionFakeOTP) Verify(ctx context.Context, in otp.VerifyInput) (otp.Verified, error) {
	if strings.TrimSpace(in.Code) != "123456" {
		return otp.Verified{}, otp.ErrInvalidCode
	}
	row, err := f.q.CreatePhoneOTP(ctx, db.CreatePhoneOTPParams{
		Uuid: uuid.New(), PhoneE164: pgText(in.Phone), CodeHash: "fake", Type: otp.PurposeContractSign,
		ExpiresAt: pgtype.Timestamptz{Time: f.createdAt.Add(5 * time.Minute), Valid: true}, MaxAttempts: 5,
		KvkkLocale: pgText("tr"), KvkkVersion: pgtype.Int4{Int32: 1, Valid: true}, MessageSha256: pgText(strings.Repeat("a", 64)),
		CreatedAt: pgtype.Timestamptz{Time: f.createdAt, Valid: true},
	})
	if err != nil {
		return otp.Verified{}, err
	}
	if err := f.q.ConsumeOTPAt(ctx, db.ConsumeOTPAtParams{ID: row.ID, Now: pgtype.Timestamptz{Time: f.now, Valid: true}}); err != nil {
		return otp.Verified{}, err
	}
	return otp.Verified{ID: row.Uuid, Phone: in.Phone}, nil
}

func subscriptionPNGBase64() string {
	raw, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	return base64.StdEncoding.EncodeToString(raw)
}

func TestModuleBundleBlockedByParentRollsBack(t *testing.T) {
	e := newSubEnv(t)
	start, end := subDates()
	item := e.item(t, CategoryModuleBundle, "50.00")
	if err := e.q.AddServiceCatalogModule(e.ctx, db.AddServiceCatalogModuleParams{
		ItemID: item.ID, ModuleKey: features.ModuleStockForecast,
	}); err != nil {
		t.Fatalf("bundle module: %v", err)
	}
	if _, err := e.features.SetByAdmin(e.ctx, 0, e.dist.ID, features.ModuleStockForecast, false); err != nil {
		t.Fatalf("close parent: %v", err)
	}
	_, err := e.svc.Assign(e.ctx, e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsAssign), SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: start, EndsOn: end,
	})
	if !errors.Is(err, ErrModuleBlockedByParent) {
		t.Fatalf("assign under closed parent = %v, want ErrModuleBlockedByParent", err)
	}
	var subs, flags int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM service_subscriptions WHERE organization_id = $1`, e.dealer.ID).Scan(&subs); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM module_flags WHERE organization_id = $1 AND source = 'service'`, e.dealer.ID).Scan(&flags); err != nil {
		t.Fatal(err)
	}
	if subs != 0 || flags != 0 {
		t.Fatalf("after blocked assign: subscriptions=%d service flags=%d, want 0/0", subs, flags)
	}
}

func TestDealerCannotAssignAndRejectRestoresActive(t *testing.T) {
	e := newSubEnv(t)
	start, end := subDates()
	item := e.item(t, "software", "100.00")
	if _, err := e.svc.Assign(e.ctx, e.caller(e.dealer, rbac.ScopeManaged, rbac.PermServiceSubscriptionsAssign), SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: start, EndsOn: end,
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer assign = %v, want ErrForbidden", err)
	}
	sub, err := e.svc.Assign(e.ctx, e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsAssign), SubscriptionInput{
		ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: start, EndsOn: end,
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	dealer := e.caller(e.dealer, rbac.ScopeManaged, rbac.PermServiceSubscriptionsCancelRequest)
	center := e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsCancelApprove)
	req, err := e.svc.RequestCancel(e.ctx, dealer, sub.UUID, CancelRequestInput{Reason: "maybe"})
	if err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	if _, err := e.svc.RequestCancel(e.ctx, dealer, sub.UUID, CancelRequestInput{Reason: "again"}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("second pending request = %v, want ErrInvalidStatus", err)
	}
	if _, err := e.svc.ApproveCancel(e.ctx, e.caller(e.dist, rbac.ScopeSubtree, rbac.PermServiceSubscriptionsCancelApprove), req.UUID, DecisionInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor approve = %v, want ErrForbidden", err)
	}
	if d, err := e.svc.RejectCancel(e.ctx, center, req.UUID, DecisionInput{}); err != nil || d.Status != StatusRejected {
		t.Fatalf("reject = %+v, %v", d, err)
	}
	got, err := e.svc.GetSubscription(e.ctx, e.caller(e.dealer, rbac.ScopeManaged, rbac.PermServiceSubscriptionsRead), sub.UUID)
	if err != nil || got.Status != StatusActive {
		t.Fatalf("after reject = %+v, %v; want active", got, err)
	}
	if _, err := e.svc.RequestCancel(e.ctx, dealer, sub.UUID, CancelRequestInput{Reason: "now really"}); err != nil {
		t.Fatalf("request after reject: %v", err)
	}
}
