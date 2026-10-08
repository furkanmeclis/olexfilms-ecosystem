package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeModules turns the fleet module off for the listed organizations.
type fakeModules struct{ off map[int64]bool }

func (m *fakeModules) Enabled(_ context.Context, orgID int64, key string) (bool, error) {
	return key == features.ModuleFleet && !m.off[orgID], nil
}

// fakeFiles serves stored report PDFs from memory.
type fakeFiles map[string][]byte

func (f fakeFiles) Download(_ context.Context, path string) (io.ReadCloser, int64, error) {
	b, ok := f[path]
	if !ok {
		return nil, 0, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

// testFleet is an opened fleet with its primary (portal) user.
type testFleet struct {
	uuid uuid.UUID
	org  db.Organization
	user db.User
}

// openPortalFleet opens a fleet at opener and invites its primary user.
func (f *apiFixture) openPortalFleet(t *testing.T, opener db.Organization, tag string) testFleet {
	t.Helper()
	opened, err := f.svc.Open(f.ctx, f.dealer(opener), OpenInput{
		LegalName: "T474 Filo " + tag, TaxNumber: validVKN(t, fmt.Sprintf("%s%d", f.suffix[:len(f.suffix)-1], int(tag[0])%10)),
	})
	if err != nil {
		t.Fatalf("open %s: %v", tag, err)
	}
	invited, err := f.svc.InviteUser(f.ctx, f.dealer(opener), opened.UUID, InviteInput{
		Email: fmt.Sprintf("t474-%s-%s@example.test", tag, f.suffix), Name: "Filo " + tag,
	})
	if err != nil {
		t.Fatalf("invite %s: %v", tag, err)
	}
	u, err := f.q.GetUserByUUID(f.ctx, invited.UserUUID)
	if err != nil {
		t.Fatal(err)
	}
	fl, err := f.q.GetFleetByUUID(f.ctx, opened.UUID)
	if err != nil {
		t.Fatal(err)
	}
	return testFleet{uuid: opened.UUID, org: fl.Organization, user: u}
}

func (f *apiFixture) fleetVehicle(t *testing.T, opener db.Organization, pf testFleet, n int, withBrand bool) VehicleView {
	t.Helper()
	in := AddVehicleInput{Plate: f.plate(n)}
	if withBrand {
		in.CarBrandUUID, in.CarModelUUID = &f.carBrand.Uuid, &f.carModel.Uuid
	}
	v, _, err := f.svc.AddVehicle(f.ctx, f.dealer(opener), pf.uuid, in)
	if err != nil {
		t.Fatalf("vehicle %d: %v", n, err)
	}
	return v
}

// linkSecond lets dealer request a link that the fleet user accepts.
func (f *apiFixture) linkSecond(t *testing.T, dealer db.Organization, pf testFleet) {
	t.Helper()
	req, err := f.svc.RequestLink(f.ctx, f.dealer(dealer), pf.uuid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.DecideLink(f.ctx, pf.user.ID, req.Link.UUID, true); err != nil {
		t.Fatal(err)
	}
}

func (f *apiFixture) caller(u db.User) PortalCaller {
	return PortalCaller{UserID: u.ID, BrandID: f.brand.ID}
}

// warrantedService is a completed service of org on the vehicle with one
// warranty ending at end (holder: the vehicle owner).
func (f *apiFixture) warrantedService(t *testing.T, org db.Organization, vehicleUUID uuid.UUID, end time.Time) db.Service {
	t.Helper()
	ctx := f.ctx
	v, err := f.q.GetVehicleByUUID(ctx, vehicleUUID)
	if err != nil {
		t.Fatal(err)
	}
	center, err := f.q.GetBrandCenter(ctx, f.brand.ID)
	if err != nil {
		t.Fatal(err)
	}
	seq := time.Now().UnixNano()
	cat, err := f.q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: f.brand.ID, Name: fmt.Sprintf("t474-cat-%d", seq),
		AvailableParts: []byte(`[]`), Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.q.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: center.ID, BrandID: f.brand.ID, CategoryID: cat.ID, Sku: fmt.Sprintf("t474-%d", seq),
		Name: "T474 PPF", Images: []byte("[]"), UnitType: "piece", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := f.q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: center.ID, BrandID: f.brand.ID, ProductID: p.ID, Barcode: fmt.Sprintf("T474-%d", seq),
		UnitKind: "serial", Source: "generated", Status: "available",
	})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := f.tx.QueryRow(ctx, `INSERT INTO services (service_no, organization_id, brand_id, customer_user_id,
		vehicle_id, car_brand_id, car_model_id, plate, plate_country, status)
		VALUES ('T474' || right(gen_random_uuid()::text, 8), $1, $2, $3, $4, $5, $6, $7, 'TR', 'pending')
		RETURNING id`, org.ID, org.BrandID, v.UserID, v.ID, f.carBrand.ID, f.carModel.ID, v.Plate.String).Scan(&id); err != nil {
		t.Fatalf("service: %v", err)
	}
	item, err := f.q.CreateServiceItem(ctx, db.CreateServiceItemParams{
		ServiceID: id, ProductID: p.ID, UnitID: unit.ID, Kind: "full", AppliedParts: []byte(`[]`),
	})
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	svc, err := f.q.CompleteService(ctx, db.CompleteServiceParams{ID: id})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := f.q.CreateWarrantyForServiceItem(ctx, db.CreateWarrantyForServiceItemParams{
		ServiceItemID: item.ID, HolderUserID: v.UserID,
		StartAt: pgtype.Timestamptz{Time: end.AddDate(-1, 0, 0), Valid: true}, EndAt: pgtype.Timestamptz{Time: end, Valid: true},
	}); err != nil {
		t.Fatalf("warranty: %v", err)
	}
	return svc
}

func serviceUUIDs(items []PortalServiceView) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, s := range items {
		out[s.UUID] = true
	}
	return out
}

// Acceptance (TEC-474): a fleet A user never reaches fleet B: B's vehicle
// is 404, B's services are neither listed nor reachable through the
// portal service detail / PDF; a user without a fleet is 404.
func TestPortalFleetUserSeesOnlyOwnFleet(t *testing.T) {
	f := newAPIFixture(t)
	ctx := f.ctx
	a := f.openPortalFleet(t, f.d1, "a")
	b := f.openPortalFleet(t, f.d1, "b")
	va := f.fleetVehicle(t, f.d1, a, 1, true)
	vb := f.fleetVehicle(t, f.d1, b, 2, true)
	sa := f.completedService(t, f.d1, va.UUID)
	sb := f.completedService(t, f.d1, vb.UUID)

	items, total, err := f.svc.PortalVehicles(ctx, f.caller(a.user), PortalVehicleFilter{Limit: 20})
	if err != nil || total != 1 || len(items) != 1 || items[0].UUID != va.UUID || items[0].ServiceCount != 1 {
		t.Fatalf("fleet A vehicles = %+v (%d), %v", items, total, err)
	}
	if _, err := f.svc.PortalVehicle(ctx, f.caller(a.user), vb.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fleet B vehicle from fleet A: err = %v, want ErrNotFound", err)
	}
	detail, err := f.svc.PortalVehicle(ctx, f.caller(a.user), va.UUID)
	if err != nil || len(detail.Services) != 1 || detail.Services[0].UUID != sa.Uuid {
		t.Fatalf("fleet A vehicle detail = %+v, %v", detail.Services, err)
	}
	svcs, total, err := f.svc.PortalServices(ctx, f.caller(a.user), PortalServiceFilter{Limit: 20})
	if err != nil || total != 1 || !serviceUUIDs(svcs)[sa.Uuid] {
		t.Fatalf("fleet A services = %+v (%d), %v", svcs, total, err)
	}
	if svcs, total, err := f.svc.PortalServices(ctx, f.caller(a.user), PortalServiceFilter{VehicleUUID: &vb.UUID, Limit: 20}); err != nil || total != 0 || len(svcs) != 0 {
		t.Fatalf("fleet A services of fleet B vehicle = %d, %v", total, err)
	}
	// The existing portal service detail / PDF: own fleet yes, other no.
	if holder, ok, err := f.svc.PortalServiceHolder(ctx, f.brand.ID, a.user.ID, sa); err != nil || !ok || holder != a.user.ID {
		t.Fatalf("own fleet service holder = %d %v %v", holder, ok, err)
	}
	if _, ok, err := f.svc.PortalServiceHolder(ctx, f.brand.ID, a.user.ID, sb); err != nil || ok {
		t.Fatalf("fleet B service reachable from fleet A: %v %v", ok, err)
	}
	// Fleet B sees its own vehicle only.
	if items, _, err := f.svc.PortalVehicles(ctx, f.caller(b.user), PortalVehicleFilter{Limit: 20}); err != nil || len(items) != 1 || items[0].UUID != vb.UUID {
		t.Fatalf("fleet B vehicles = %+v, %v", items, err)
	}
	// Another brand or a user without a fleet reaches nothing.
	if _, err := f.svc.PortalOverview(ctx, PortalCaller{UserID: a.user.ID, BrandID: f.brand.ID + 100000}, apiquery.TimeRange{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other brand overview: err = %v", err)
	}
	other, err := f.q.CreateUser(ctx, db.CreateUserParams{
		Email: pgtype.Text{String: "t474-nofleet-" + f.suffix + "@example.test", Valid: true}, PasswordHash: "x", Name: "X", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.PortalVehicles(ctx, f.caller(other), PortalVehicleFilter{Limit: 20}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no fleet: err = %v", err)
	}
}

// Acceptance (TEC-474): a dealer that turns the fleet module off drops out
// of the fleet portal (services, vehicle history, overview, accounting, the
// service PDF) while its rows stay; with no actively linked dealer left the
// portal answers ErrPortalClosed (403 FEATURE_DISABLED) and the link
// decision stays open.
func TestPortalModuleOffDealerHidden(t *testing.T) {
	f := newAPIFixture(t)
	ctx := f.ctx
	mods := &fakeModules{off: map[int64]bool{}}
	f.svc.SetModules(mods)
	pf := f.openPortalFleet(t, f.d1, "m")
	f.linkSecond(t, f.d2, pf)
	v := f.fleetVehicle(t, f.d1, pf, 1, true)
	s1 := f.completedService(t, f.d1, v.UUID)
	s2 := f.completedService(t, f.d2, v.UUID)
	f.income(t, f.d1, pf.org.ID, s1, "100.00")
	f.income(t, f.d2, pf.org.ID, s2, "200.00")
	c := f.caller(pf.user)

	all, total, err := f.svc.PortalServices(ctx, c, PortalServiceFilter{Limit: 20})
	if err != nil || total != 2 || !serviceUUIDs(all)[s1.Uuid] || !serviceUUIDs(all)[s2.Uuid] {
		t.Fatalf("both dealers on: services = %d, %v", total, err)
	}
	if acc, err := f.svc.PortalAccounting(ctx, c, "", ""); err != nil || len(acc.Dealers) != 2 {
		t.Fatalf("both dealers on: accounting = %+v, %v", acc, err)
	}

	mods.off[f.d2.ID] = true
	svcs, total, err := f.svc.PortalServices(ctx, c, PortalServiceFilter{Limit: 20})
	if err != nil || total != 1 || len(svcs) != 1 || svcs[0].UUID != s1.Uuid {
		t.Fatalf("dealer 2 off: services = %+v (%d), %v", svcs, total, err)
	}
	if svcs, total, err := f.svc.PortalServices(ctx, c, PortalServiceFilter{DealerUUIDs: []uuid.UUID{f.d2.Uuid}, Limit: 20}); err != nil || total != 0 || len(svcs) != 0 {
		t.Fatalf("dealer 2 off, dealer filter: %d, %v", total, err)
	}
	detail, err := f.svc.PortalVehicle(ctx, c, v.UUID)
	if err != nil || detail.ServiceCount != 1 || len(detail.Services) != 1 || detail.Services[0].UUID != s1.Uuid {
		t.Fatalf("dealer 2 off: vehicle detail = %+v, %v", detail, err)
	}
	ov, err := f.svc.PortalOverview(ctx, c, apiquery.TimeRange{})
	if err != nil || ov.ServiceCount != 1 || len(ov.Dealers) != 1 || ov.Dealers[0].UUID != f.d1.Uuid || ov.VehicleCount != 1 {
		t.Fatalf("dealer 2 off: overview = %+v, %v", ov, err)
	}
	acc, err := f.svc.PortalAccounting(ctx, c, "", "")
	if err != nil || len(acc.Dealers) != 1 || acc.Dealers[0].Dealer.UUID != f.d1.Uuid || acc.Dealers[0].ClosingBalance != "100.00" {
		t.Fatalf("dealer 2 off: accounting = %+v, %v", acc, err)
	}
	if _, ok, err := f.svc.PortalServiceHolder(ctx, f.brand.ID, pf.user.ID, s2); err != nil || ok {
		t.Fatalf("dealer 2 off: service PDF reachable: %v %v", ok, err)
	}
	// Nothing is deleted.
	var n int
	if err := f.tx.QueryRow(ctx, `SELECT COUNT(*) FROM services WHERE vehicle_id = (SELECT id FROM vehicles WHERE uuid = $1)`, v.UUID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("services kept = %d, %v", n, err)
	}

	// Every dealer off: the portal closes, the link decision stays.
	mods.off[f.d1.ID] = true
	if _, err := f.svc.PortalOverview(ctx, c, apiquery.TimeRange{}); !errors.Is(err, ErrPortalClosed) {
		t.Fatalf("all off: err = %v, want ErrPortalClosed", err)
	}
	var re *RuleError
	if _, _, err := f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{Limit: 20}); !errors.As(err, &re) || re.Status() != 403 || re.Code != "FEATURE_DISABLED" {
		t.Fatalf("all off: vehicles err = %v", err)
	}
	if links, err := f.svc.PortalLinks(ctx, pf.user.ID); err != nil || len(links) != 2 {
		t.Fatalf("all off: links = %d, %v", len(links), err)
	}

	// An ended link keeps its history while the dealer has the module.
	mods.off = map[int64]bool{}
	if _, err := f.tx.Exec(ctx, `UPDATE fleet_dealer_links SET status = 'ended', ended_at = NOW() WHERE fleet_org_id = $1 AND dealer_org_id = $2`, pf.org.ID, f.d2.ID); err != nil {
		t.Fatal(err)
	}
	if _, total, err := f.svc.PortalServices(ctx, c, PortalServiceFilter{Limit: 20}); err != nil || total != 2 {
		t.Fatalf("ended link history: %d, %v", total, err)
	}
}

// Acceptance (TEC-474): the accounting answer holds only the fleet cari's
// service income and collections (with their balance) per dealer; the
// dealer's other cari movements and other caris never appear.
func TestPortalAccountingOnlyFleetCariMovements(t *testing.T) {
	f := newAPIFixture(t)
	ctx := f.ctx
	pf := f.openPortalFleet(t, f.d1, "c")
	v := f.fleetVehicle(t, f.d1, pf, 1, true)
	s1 := f.completedService(t, f.d1, v.UUID)
	f.income(t, f.d1, pf.org.ID, s1, "1000.00")
	cari, err := ensureCariForTest(ctx, f.q, f.d1, pf.org.ID)
	if err != nil {
		t.Fatal(err)
	}
	account, err := f.q.CreateFinanceAccount(ctx, db.CreateFinanceAccountParams{
		OrganizationID: f.d1.ID, BrandID: f.brand.ID, Type: "cash", Name: "Kasa " + f.suffix, Currency: "TRY", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	poster := posting.New(f.q, nil, nil)
	if _, err := poster.Collect(ctx, f.tx, posting.Entry{
		OrganizationID: f.d1.ID, Source: posting.Source{Type: "cari_collection", UUID: uuid.New()},
		Category: accounting.CategoryCollection, Amount: "400.00", Currency: "TRY", AccountID: account.ID, CariID: cari,
	}); err != nil {
		t.Fatalf("collection: %v", err)
	}
	// Another movement on the fleet cari (a sale charge) and a service
	// income on another cari: never part of the fleet's view.
	if _, err := poster.Charge(ctx, f.tx, posting.Entry{
		OrganizationID: f.d1.ID, Source: posting.Source{Type: "manual_charge", UUID: uuid.New()},
		Category: accounting.CategorySale, Amount: "77.00", Currency: "TRY", CariID: cari,
	}); err != nil {
		t.Fatalf("charge: %v", err)
	}
	if _, err := poster.PostIncome(ctx, f.tx, posting.Entry{
		OrganizationID: f.d1.ID, Source: posting.Source{Type: model.SourceServiceIncome, UUID: uuid.New()},
		Category: accounting.CategoryServiceIncome, Amount: "555.00", Currency: "TRY", CounterpartyOrgID: f.d2.ID,
	}); err != nil {
		t.Fatalf("other income: %v", err)
	}

	acc, err := f.svc.PortalAccounting(ctx, f.caller(pf.user), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(acc.Dealers) != 1 {
		t.Fatalf("dealers = %+v", acc.Dealers)
	}
	d := acc.Dealers[0]
	if len(d.Lines) != 2 || d.Lines[0].Kind != LineServiceIncome || d.Lines[0].Debit != "1000.00" ||
		d.Lines[0].Service == nil || d.Lines[0].Service.UUID != s1.Uuid ||
		d.Lines[1].Kind != LineCollection || d.Lines[1].Credit != "400.00" || d.Lines[1].Balance != "600.00" {
		t.Fatalf("lines = %+v", d.Lines)
	}
	if d.ServiceIncomeTotal != "1000.00" || d.CollectionTotal != "400.00" || d.ClosingBalance != "600.00" || d.OpeningBalance != "0.00" {
		t.Fatalf("totals = %+v", d)
	}
	// A later period opens with the balance of the same rows only.
	next := time.Now().UTC().AddDate(0, 1, 0)
	from := time.Date(next.Year(), next.Month(), 1, 0, 0, 0, 0, time.UTC)
	later, err := f.svc.PortalAccounting(ctx, f.caller(pf.user), from.Format(time.DateOnly), from.AddDate(0, 0, 5).Format(time.DateOnly))
	if err != nil || later.Dealers[0].OpeningBalance != "600.00" || len(later.Dealers[0].Lines) != 0 {
		t.Fatalf("later period = %+v, %v", later, err)
	}
	var ve *ValidationError
	if _, err := f.svc.PortalAccounting(ctx, f.caller(pf.user), "2026-01-10", ""); !errors.As(err, &ve) || ve.Field != "date_to" {
		t.Fatalf("half period: err = %v", err)
	}
}

// List contract (docs/list-contract.md) of the portal vehicles, services
// and warranties: sort whitelist and default, q, multi-value filters,
// boolean and date filters.
func TestPortalListContracts(t *testing.T) {
	f := newAPIFixture(t)
	ctx := f.ctx
	pf := f.openPortalFleet(t, f.d1, "l")
	c := f.caller(pf.user)
	v1 := f.fleetVehicle(t, f.d1, pf, 1, true)
	v2 := f.fleetVehicle(t, f.d1, pf, 2, false)
	v3 := f.fleetVehicle(t, f.d1, pf, 3, true)
	s1 := f.completedService(t, f.d1, v1.UUID)
	ws := f.warrantedService(t, f.d1, v3.UUID, time.Now().AddDate(1, 0, 0))
	plates := func(items []PortalVehicleView) []uuid.UUID {
		out := make([]uuid.UUID, 0, len(items))
		for _, v := range items {
			out = append(out, v.UUID)
		}
		return out
	}
	eq := func(got []uuid.UUID, want ...uuid.UUID) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	// Vehicles: default plate, -plate, warranty_until, last_service_at.
	items, _, err := f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{Limit: 20})
	if err != nil || !eq(plates(items), v1.UUID, v2.UUID, v3.UUID) {
		t.Fatalf("default sort = %v, %v", plates(items), err)
	}
	items, _, _ = f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{Sort: []apiquery.SortField{{Field: "plate", Desc: true}}, Limit: 20})
	if !eq(plates(items), v3.UUID, v2.UUID, v1.UUID) {
		t.Fatalf("-plate = %v", plates(items))
	}
	items, _, _ = f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{Sort: []apiquery.SortField{{Field: "warranty_until", Desc: true}}, Limit: 20})
	if len(items) != 3 || items[0].UUID != v3.UUID || items[0].WarrantyUntil == nil || items[0].ActiveWarrantyCount != 1 {
		t.Fatalf("-warranty_until = %+v", items)
	}
	items, _, _ = f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{Sort: []apiquery.SortField{{Field: "last_service_at"}}, Limit: 20})
	if len(items) != 3 || items[2].UUID != v2.UUID || items[2].LastServiceAt != nil {
		t.Fatalf("last_service_at (nulls last) = %+v", plates(items))
	}
	var qe *apiquery.ValidationError
	if _, _, err := f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{Sort: []apiquery.SortField{{Field: "vin"}}, Limit: 20}); !errors.As(err, &qe) {
		t.Fatalf("sort vin: err = %v, want validation", err)
	}
	// q (plate), brand (multi value), has_active_warranty, paging total.
	items, total, _ := f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{Q: f.plate(2), Limit: 20})
	if total != 1 || !eq(plates(items), v2.UUID) {
		t.Fatalf("q plate = %v (%d)", plates(items), total)
	}
	items, total, _ = f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{CarBrandUUIDs: []uuid.UUID{f.carBrand.Uuid, uuid.New()}, Limit: 20})
	if total != 2 || !eq(plates(items), v1.UUID, v3.UUID) {
		t.Fatalf("brand = %v (%d)", plates(items), total)
	}
	yes, no := true, false
	items, total, _ = f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{HasActiveWarranty: &yes, Limit: 20})
	if total != 1 || !eq(plates(items), v3.UUID) {
		t.Fatalf("has_active_warranty=true = %v (%d)", plates(items), total)
	}
	items, total, _ = f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{HasActiveWarranty: &no, Limit: 1})
	if total != 2 || !eq(plates(items), v1.UUID) {
		t.Fatalf("has_active_warranty=false limit 1 = %v (%d)", plates(items), total)
	}

	// Services: status, dealer, date range, sort.
	svcs, total, _ := f.svc.PortalServices(ctx, c, PortalServiceFilter{Statuses: []string{"completed"}, DealerUUIDs: []uuid.UUID{f.d1.Uuid}, Limit: 20})
	if total != 2 || !serviceUUIDs(svcs)[s1.Uuid] || !serviceUUIDs(svcs)[ws.Uuid] {
		t.Fatalf("services status+dealer = %d", total)
	}
	if _, total, _ := f.svc.PortalServices(ctx, c, PortalServiceFilter{Statuses: []string{"pending"}, Limit: 20}); total != 0 {
		t.Fatalf("status pending = %d", total)
	}
	future := time.Now().AddDate(0, 0, 2)
	if _, total, _ := f.svc.PortalServices(ctx, c, PortalServiceFilter{Created: apiquery.TimeRange{From: &future}, Limit: 20}); total != 0 {
		t.Fatalf("date_from future = %d", total)
	}
	svcs, _, _ = f.svc.PortalServices(ctx, c, PortalServiceFilter{Sort: []apiquery.SortField{{Field: "service_no"}}, Limit: 20})
	if len(svcs) != 2 || svcs[0].ServiceNo > svcs[1].ServiceNo {
		t.Fatalf("sort service_no = %+v", svcs)
	}
	if _, _, err := f.svc.PortalServices(ctx, c, PortalServiceFilter{Sort: []apiquery.SortField{{Field: "plate"}}, Limit: 20}); !errors.As(err, &qe) {
		t.Fatalf("services sort plate: err = %v", err)
	}

	// Warranties: state filter, q, vehicle.
	wl, total, err := f.svc.PortalWarranties(ctx, c, PortalWarrantyFilter{States: []string{"active"}, Limit: 20})
	if err != nil || total != 1 || wl[0].VehicleUUID != v3.UUID || wl[0].ServiceUUID != ws.Uuid || wl[0].State != "active" || wl[0].DaysLeft < 360 {
		t.Fatalf("active warranties = %+v (%d), %v", wl, total, err)
	}
	if _, total, _ := f.svc.PortalWarranties(ctx, c, PortalWarrantyFilter{States: []string{"expired", "void"}, Limit: 20}); total != 0 {
		t.Fatalf("expired warranties = %d", total)
	}
	if _, total, _ := f.svc.PortalWarranties(ctx, c, PortalWarrantyFilter{VehicleUUID: &v1.UUID, Limit: 20}); total != 0 {
		t.Fatalf("v1 warranties = %d", total)
	}
	// A warranty past its end_at (status still active) counts as expired.
	f.warrantedService(t, f.d1, v2.UUID, time.Now().AddDate(0, 0, -1))
	if wl, total, _ := f.svc.PortalWarranties(ctx, c, PortalWarrantyFilter{States: []string{"expired"}, Limit: 20}); total != 1 || wl[0].VehicleUUID != v2.UUID || wl[0].DaysLeft != 0 {
		t.Fatalf("past end_at counts as expired = %d", total)
	}
	if items, total, _ := f.svc.PortalVehicles(ctx, c, PortalVehicleFilter{HasActiveWarranty: &yes, Limit: 20}); total != 1 || !eq(plates(items), v3.UUID) {
		t.Fatalf("expired warranty is not active: %v (%d)", plates(items), total)
	}
	ov, err := f.svc.PortalOverview(ctx, c, apiquery.TimeRange{})
	if err != nil || ov.VehicleCount != 3 || ov.ActiveWarrantyCount != 1 || ov.ServiceCount != 3 {
		t.Fatalf("overview = %+v, %v", ov, err)
	}
}

// The ready reports of the user's fleet are listed and downloaded; a
// pending report or another fleet's report is ErrNotFound.
func TestPortalReports(t *testing.T) {
	f := newAPIFixture(t)
	ctx := f.ctx
	files := fakeFiles{}
	f.svc.SetReportFiles(files)
	a := f.openPortalFleet(t, f.d1, "r")
	b := f.openPortalFleet(t, f.d1, "s")
	report := func(pf testFleet, start time.Time, ready bool) db.FleetReport {
		r, err := f.q.UpsertFleetReport(ctx, db.UpsertFleetReportParams{
			FleetOrgID: pf.org.ID, BrandID: f.brand.ID, PeriodKind: model.ReportMonthly,
			PeriodStart: pgtype.Date{Time: start, Valid: true}, PeriodEnd: pgtype.Date{Time: start.AddDate(0, 1, -1), Valid: true}, Locale: "tr",
		})
		if err != nil {
			t.Fatal(err)
		}
		if ready {
			key := "fleet-reports/" + r.Uuid.String() + ".pdf"
			files[key] = []byte("%PDF-1.4 " + r.Uuid.String())
			if r, err = f.q.MarkFleetReportReady(ctx, db.MarkFleetReportReadyParams{ID: r.ID, StorageKey: pgtype.Text{String: key, Valid: true}}); err != nil {
				t.Fatal(err)
			}
		}
		return r
	}
	jan := report(a, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true)
	feb := report(a, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), true)
	pending := report(a, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), false)
	other := report(b, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true)

	items, total, err := f.svc.PortalReports(ctx, f.caller(a.user), PortalReportFilter{Limit: 20})
	if err != nil || total != 2 || len(items) != 2 || items[0].UUID != feb.Uuid || items[1].UUID != jan.Uuid || items[0].PeriodStart != "2026-02-01" {
		t.Fatalf("reports = %+v (%d), %v", items, total, err)
	}
	items, _, _ = f.svc.PortalReports(ctx, f.caller(a.user), PortalReportFilter{Sort: []apiquery.SortField{{Field: "period_start"}}, Limit: 20})
	if len(items) != 2 || items[0].UUID != jan.Uuid {
		t.Fatalf("reports period_start asc = %+v", items)
	}
	if _, total, _ := f.svc.PortalReports(ctx, f.caller(a.user), PortalReportFilter{PeriodKinds: []string{model.ReportQuarterly}, Limit: 20}); total != 0 {
		t.Fatalf("quarterly = %d", total)
	}
	rc, name, err := f.svc.PortalReportFile(ctx, f.caller(a.user), jan.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Contains(body, []byte(jan.Uuid.String())) || name != "fleet-report-monthly-2026-01-01.pdf" {
		t.Fatalf("file = %q %q", body, name)
	}
	for _, id := range []uuid.UUID{pending.Uuid, other.Uuid, uuid.New()} {
		if _, _, err := f.svc.PortalReportFile(ctx, f.caller(a.user), id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("report %s: err = %v, want ErrNotFound", id, err)
		}
	}
}
