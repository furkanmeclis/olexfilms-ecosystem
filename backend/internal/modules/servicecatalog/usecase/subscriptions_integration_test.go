package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type subEnv struct {
	ctx                         context.Context
	pool                        *pgxpool.Pool
	q                           *db.Queries
	svc                         *Service
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

func (e *subEnv) caller(org db.Organization, scope rbac.Scope, perm string) Caller {
	return Caller{
		Principal: authctx.Principal{UserInternal: 0, PermissionScopes: map[string]rbac.Scope{perm: scope}},
		Org: orgctx.Scope{
			InternalID: org.ID, UUID: org.Uuid, OrgType: org.Type, BrandID: org.BrandID, Name: org.Name,
		},
		Filter: scopefilter.Filter{Permission: perm, Scope: scope, OrgID: org.ID, OrgIDs: []int64{org.ID}, BrandID: org.BrandID},
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
