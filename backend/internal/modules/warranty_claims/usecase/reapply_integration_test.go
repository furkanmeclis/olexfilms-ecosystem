package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type reapplyFixture struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	tx     pgx.Tx
	q      *db.Queries
	svc    *Service
	bus    events.Bus
	brand  db.Brand
	center db.Organization
	dealer db.Organization
	user   db.User
}

func newReapplyFixture(t *testing.T) *reapplyFixture {
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
	tx, err := pool.Begin(ctx)
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
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	dealer, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "tec336-dealer-" + suffix, Name: "TEC336 Dealer", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: center.ID, Valid: true},
		BrandID: brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("dealer: %v", err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "TEC336", Surname: "Customer", Status: "active",
		Email: pgtype.Text{String: "tec336-" + suffix + "@example.test", Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	mem := outbox.NewMemory()
	bus := events.NewBus(slog.Default())
	svc := NewWithStore(txStore{Queries: q, pool: tx}, nil, mem)
	RegisterEventHandlers(bus, tx, q, mem, slog.Default())
	return &reapplyFixture{ctx: ctx, pool: pool, tx: tx, q: q, svc: svc, bus: bus, brand: brand, center: center, dealer: dealer, user: user}
}

func (f *reapplyFixture) claim(t *testing.T) db.WarrantyClaim {
	t.Helper()
	claim := f.openClaim(t)
	approved, err := f.q.SetWarrantyClaimStatus(f.ctx, db.SetWarrantyClaimStatusParams{
		ID: claim.ID, BrandID: claim.BrandID, FromStatus: StatusOpen, Status: StatusApproved,
	})
	if err != nil {
		t.Fatalf("approve claim: %v", err)
	}
	return approved
}

// openClaim creates an open claim with one part (body_kaput).
func (f *reapplyFixture) openClaim(t *testing.T) db.WarrantyClaim {
	t.Helper()
	q, ctx := f.q, f.ctx
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var carBrand, carModel int64
	if err := f.tx.QueryRow(ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, "TEC336 "+suffix).Scan(&carBrand); err != nil {
		t.Fatalf("car brand: %v", err)
	}
	if err := f.tx.QueryRow(ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, $2) RETURNING id`, carBrand, "Model").Scan(&carModel); err != nil {
		t.Fatalf("car model: %v", err)
	}
	vehicle, err := q.CreateVehicle(ctx, db.CreateVehicleParams{
		UserID: f.user.ID, BrandID: f.brand.ID,
		CarBrandID: pgtype.Int8{Int64: carBrand, Valid: true},
		CarModelID: pgtype.Int8{Int64: carModel, Valid: true},
		Vin:        pgtype.Text{String: "1HGCM82633A004352", Valid: true},
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: f.center.ID, BrandID: f.brand.ID, Name: "tec336-cat-" + suffix,
		AvailableParts: []byte(`["body_kaput","body_tavan"]`), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product, err := q.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: f.center.ID, BrandID: f.brand.ID, CategoryID: cat.ID,
		Sku: "tec336-" + suffix, Name: "TEC336 Film", Images: []byte("[]"), UnitType: "piece", Active: true,
		WarrantyDurationMonths: pgtype.Int4{Int32: 12, Valid: true},
	})
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	unit, err := q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: f.center.ID, BrandID: f.brand.ID, ProductID: product.ID,
		Barcode: "TEC336-" + suffix, UnitKind: "serial", Source: "generated", Status: "available",
	})
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	service, err := q.CreateService(ctx, db.CreateServiceParams{
		ServiceNo: "T336-" + suffix[len(suffix)-8:], OrganizationID: f.dealer.ID, BrandID: f.brand.ID,
		CustomerUserID: f.user.ID, VehicleID: vehicle.ID, CarBrandID: carBrand, CarModelID: carModel,
		Vin: pgtype.Text{String: "1HGCM82633A004352", Valid: true}, Status: "draft",
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	item, err := q.CreateServiceItem(ctx, db.CreateServiceItemParams{
		ServiceID: service.ID, ProductID: product.ID, UnitID: unit.ID, Kind: "full",
		AppliedParts: []byte(`["body_kaput"]`),
	})
	if err != nil {
		t.Fatalf("service item: %v", err)
	}
	if _, err := q.CompleteService(ctx, db.CompleteServiceParams{ID: service.ID}); err != nil {
		t.Fatalf("complete original service: %v", err)
	}
	warranty, err := q.CreateWarrantyForServiceItem(ctx, db.CreateWarrantyForServiceItemParams{
		ServiceItemID: item.ID, HolderUserID: f.user.ID,
		StartAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		EndAt:   pgtype.Timestamptz{Time: time.Now().AddDate(1, 0, 0), Valid: true},
	})
	if err != nil {
		t.Fatalf("warranty: %v", err)
	}
	claim, err := q.CreateWarrantyClaim(ctx, db.CreateWarrantyClaimParams{
		OrganizationID: f.dealer.ID, BrandID: f.brand.ID, WarrantyID: warranty.ID, ServiceID: service.ID,
		VehicleID: vehicle.ID, CustomerUserID: f.user.ID, Description: "Film kalkti",
		Status: StatusOpen, CoverageCheck: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := q.AddWarrantyClaimPart(ctx, db.AddWarrantyClaimPartParams{
		ClaimID: claim.ID, OrganizationID: claim.OrganizationID, BrandID: claim.BrandID,
		PartKey: "body_kaput", ServiceItemID: int8(item.ID), ProductID: int8(product.ID), UnitID: int8(unit.ID),
	}); err != nil {
		t.Fatalf("claim part: %v", err)
	}
	return claim
}

func (f *reapplyFixture) publishClaimApproved(t *testing.T, claim db.WarrantyClaim) {
	t.Helper()
	ev := events.New(events.WarrantyClaimStatusChanged).
		WithTenant(claim.OrganizationID).
		WithEntity("warranty_claim", &claim.ID, &claim.Uuid).
		WithPayload(map[string]any{"brand_id": claim.BrandID, "from": StatusCenterReview, "to": StatusApproved})
	if err := f.bus.Publish(f.ctx, ev); err != nil {
		t.Fatalf("publish approved: %v", err)
	}
}

func (f *reapplyFixture) reapplyService(t *testing.T, claim db.WarrantyClaim) db.Service {
	t.Helper()
	got, err := f.q.GetWarrantyClaimByID(f.ctx, db.GetWarrantyClaimByIDParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil {
		t.Fatalf("reload claim: %v", err)
	}
	if !got.ReapplyServiceID.Valid {
		t.Fatalf("claim has no reapply service: %+v", got)
	}
	svc, err := f.q.GetService(f.ctx, db.GetServiceParams{ID: got.ReapplyServiceID.Int64, BrandID: got.BrandID})
	if err != nil {
		t.Fatalf("reapply service: %v", err)
	}
	return svc
}

func (f *reapplyFixture) apiCaller(claim db.WarrantyClaim) Caller {
	return Caller{
		OrganizationID: f.dealer.ID, BrandID: f.brand.ID, OrgType: "dealer",
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{claim.OrganizationID}},
		Permissions: map[string]rbac.Scope{
			rbac.PermWarrantyClaimsRead:  rbac.ScopeManaged,
			rbac.PermWarrantyClaimsWrite: rbac.ScopeManaged,
		},
	}
}

func TestReapplyServiceApprovedEventIsIdempotent(t *testing.T) {
	f := newReapplyFixture(t)
	claim := f.claim(t)

	f.publishClaimApproved(t, claim)
	f.publishClaimApproved(t, claim)

	var count int
	if err := f.tx.QueryRow(f.ctx, `SELECT COUNT(*) FROM services WHERE warranty_claim_id = $1`, claim.ID).Scan(&count); err != nil {
		t.Fatalf("count services: %v", err)
	}
	if count != 1 {
		t.Fatalf("reapply service count = %d, want 1", count)
	}
	svc := f.reapplyService(t, claim)
	if svc.Status != "draft" || svc.CustomerUserID != claim.CustomerUserID || svc.VehicleID != claim.VehicleID {
		t.Fatalf("service = %+v; claim = %+v", svc, claim)
	}
	items, err := f.q.ListServiceItems(f.ctx, svc.ID)
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	var parts []string
	if err := json.Unmarshal(items[0].AppliedParts, &parts); err != nil {
		t.Fatalf("parts json: %v", err)
	}
	if len(parts) != 1 || parts[0] != "body_kaput" {
		t.Fatalf("parts = %#v", parts)
	}
	if _, err := f.svc.ReapplyService(f.ctx, f.apiCaller(claim), claim.Uuid); !errors.Is(err, ErrReapplyOpen) {
		t.Fatalf("open reapply call err = %v, want ErrReapplyOpen", err)
	}
}

func TestReapplyServiceCompletionClosesClaim(t *testing.T) {
	f := newReapplyFixture(t)
	claim := f.claim(t)
	f.publishClaimApproved(t, claim)
	svc := f.reapplyService(t, claim)

	done, err := f.q.CompleteService(f.ctx, db.CompleteServiceParams{ID: svc.ID})
	if err != nil {
		t.Fatalf("complete reapply service: %v", err)
	}
	ev := events.New(events.ServiceCompleted).
		WithTenant(done.OrganizationID).
		WithEntity("service", &done.ID, &done.Uuid).
		WithPayload(map[string]any{"service_id": done.ID, "brand_id": done.BrandID})
	if err := f.bus.Publish(f.ctx, ev); err != nil {
		t.Fatalf("publish completed: %v", err)
	}
	got, err := f.q.GetWarrantyClaimByID(f.ctx, db.GetWarrantyClaimByIDParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil {
		t.Fatalf("reload claim: %v", err)
	}
	if got.Status != StatusClosed {
		t.Fatalf("claim status = %s, want closed", got.Status)
	}
	events, err := f.q.ListWarrantyClaimEvents(f.ctx, claim.ID)
	if err != nil {
		t.Fatalf("claim events: %v", err)
	}
	gotSeq := []string{}
	for _, ev := range events {
		if ev.EventType == "status_changed" {
			gotSeq = append(gotSeq, ev.ToStatus.String)
		}
	}
	want := []string{StatusApproved, StatusReapplied, StatusClosed}
	if fmt.Sprint(gotSeq) != fmt.Sprint(want) {
		t.Fatalf("status event sequence = %v, want %v", gotSeq, want)
	}
}

func TestCancelledReapplyServiceAllowsSecondService(t *testing.T) {
	f := newReapplyFixture(t)
	claim := f.claim(t)
	f.publishClaimApproved(t, claim)
	first := f.reapplyService(t, claim)

	cancelled, err := f.q.CancelService(f.ctx, db.CancelServiceParams{ID: first.ID, CancelReason: pgtype.Text{String: "iptal", Valid: true}})
	if err != nil {
		t.Fatalf("cancel reapply service: %v", err)
	}
	ev := events.New(events.ServiceCancelled).
		WithTenant(cancelled.OrganizationID).
		WithEntity("service", &cancelled.ID, &cancelled.Uuid).
		WithPayload(map[string]any{"service_id": cancelled.ID, "brand_id": cancelled.BrandID})
	if err := f.bus.Publish(f.ctx, ev); err != nil {
		t.Fatalf("publish cancelled: %v", err)
	}
	got, err := f.q.GetWarrantyClaimByID(f.ctx, db.GetWarrantyClaimByIDParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil {
		t.Fatalf("reload claim: %v", err)
	}
	if got.Status != StatusApproved || got.ReapplyServiceID.Valid {
		t.Fatalf("claim after cancel = %+v, want approved without reapply link", got)
	}
	second, err := f.svc.ReapplyService(f.ctx, f.apiCaller(got), got.Uuid)
	if err != nil {
		t.Fatalf("second reapply service: %v", err)
	}
	if second.UUID == first.Uuid {
		t.Fatalf("second service reused first uuid %s", second.UUID)
	}
}
