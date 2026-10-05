package usecase

import (
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
)

type diffFixture struct {
	*dbFixture
	linker *Linker
	seq    int
}

func newDiffFixture(t *testing.T) *diffFixture {
	f := &diffFixture{dbFixture: newDBFixture(t)}
	f.linker = NewLinker(f.tx, f.q, nil, nil)
	return f
}

func (f *diffFixture) linkCaller() LinkCaller {
	return LinkCaller{
		UserID: f.user.ID,
		Org:    orgctx.Scope{InternalID: f.org.ID, UUID: f.org.Uuid, BrandID: f.org.BrandID, OrgType: f.org.Type},
		Filter: scopefilter.Filter{Permission: rbac.PermMeasurementsLink, Scope: rbac.ScopeManaged, OrgID: f.org.ID, OrgIDs: []int64{f.org.ID}},
	}
}

func (f *diffFixture) service(micron *string, beforeUM, afterUM string) db.Service {
	f.t.Helper()
	f.seq++
	cust, veh := f.customerVehicle()
	svc, err := f.q.CreateService(f.ctx, db.CreateServiceParams{
		ServiceNo:      fmt.Sprintf("T297-%d-%d", time.Now().UnixNano(), f.seq),
		OrganizationID: f.org.ID, BrandID: f.org.BrandID, CustomerUserID: cust.ID, VehicleID: veh.ID,
		CarBrandID: veh.CarBrandID.Int64, CarModelID: veh.CarModelID.Int64, Status: "draft",
		Vin: pgtype.Text{String: "WVWZZZ1JZ3W297001", Valid: true}, HasMeasurement: true,
	})
	if err != nil {
		f.t.Fatalf("service: %v", err)
	}
	productID := f.product(micron)
	unitID := f.unit(productID)
	if _, err := f.q.CreateServiceItem(f.ctx, db.CreateServiceItemParams{
		ServiceID: svc.ID, ProductID: productID, UnitID: unitID, Kind: "full",
		AppliedParts: []byte(`["body_kaput"]`),
	}); err != nil {
		f.t.Fatalf("service item: %v", err)
	}
	before := f.measurement(svc, beforeUM, time.Now().Add(-time.Hour))
	after := f.measurement(svc, afterUM, time.Now())
	if _, err := f.q.LinkServiceMeasurement(f.ctx, db.LinkServiceMeasurementParams{
		OrganizationID: f.org.ID, BrandID: f.org.BrandID, ServiceID: svc.ID,
		MeasurementResultID: before, Phase: PhaseBefore, LinkSource: LinkSourceManual,
	}); err != nil {
		f.t.Fatalf("before link: %v", err)
	}
	if _, err := f.q.LinkServiceMeasurement(f.ctx, db.LinkServiceMeasurementParams{
		OrganizationID: f.org.ID, BrandID: f.org.BrandID, ServiceID: svc.ID,
		MeasurementResultID: after, Phase: PhaseAfter, LinkSource: LinkSourceManual,
	}); err != nil {
		f.t.Fatalf("after link: %v", err)
	}
	return svc
}

func (f *diffFixture) customerVehicle() (db.User, db.Vehicle) {
	f.t.Helper()
	cust, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		Email:        pgtype.Text{String: fmt.Sprintf("tec297-cust-%d@example.test", time.Now().UnixNano()), Valid: true},
		PasswordHash: "x", Name: "tec297", Surname: "Customer", Status: "active",
	})
	if err != nil {
		f.t.Fatalf("customer: %v", err)
	}
	if _, err := f.q.LinkCustomerOrganization(f.ctx, db.LinkCustomerOrganizationParams{
		UserID: cust.ID, OrganizationID: f.org.ID, BrandID: f.org.BrandID,
	}); err != nil {
		f.t.Fatalf("customer link: %v", err)
	}
	carBrand, err := f.q.CreateCarBrand(f.ctx, db.CreateCarBrandParams{Name: fmt.Sprintf("T297 Brand %d", time.Now().UnixNano()), ShowName: true, Active: true})
	if err != nil {
		f.t.Fatalf("car brand: %v", err)
	}
	carModel, err := f.q.CreateCarModel(f.ctx, db.CreateCarModelParams{CarBrandID: carBrand.ID, Name: "T297 Model", Active: true})
	if err != nil {
		f.t.Fatalf("car model: %v", err)
	}
	veh, err := f.q.CreateVehicle(f.ctx, db.CreateVehicleParams{
		UserID: cust.ID, OrganizationID: pgtype.Int8{Int64: f.org.ID, Valid: true}, BrandID: f.org.BrandID,
		CarBrandID: pgtype.Int8{Int64: carBrand.ID, Valid: true}, CarModelID: pgtype.Int8{Int64: carModel.ID, Valid: true},
		ModelYear: pgtype.Int2{Int16: 2024, Valid: true},
		Plate:     pgtype.Text{String: fmt.Sprintf("34T297%d", f.seq), Valid: true}, PlateNormalized: pgtype.Text{String: fmt.Sprintf("34T297%d", f.seq), Valid: true},
		PlateCountry: pgtype.Text{String: "TR", Valid: true},
	})
	if err != nil {
		f.t.Fatalf("vehicle: %v", err)
	}
	return cust, veh
}

func (f *diffFixture) product(micron *string) int64 {
	f.t.Helper()
	centerID := f.centerID()
	cat, err := f.q.CreateProductCategory(f.ctx, db.CreateProductCategoryParams{
		OrganizationID: centerID, BrandID: f.org.BrandID, Name: fmt.Sprintf("T297 Cat %d", time.Now().UnixNano()),
		AvailableParts: []byte(`["body_kaput"]`), Sort: 1, Active: true,
	})
	if err != nil {
		f.t.Fatalf("category: %v", err)
	}
	var n pgtype.Numeric
	if micron != nil {
		if err := n.Scan(*micron); err != nil {
			f.t.Fatalf("micron: %v", err)
		}
	}
	p, err := f.q.CreateProduct(f.ctx, db.CreateProductParams{
		OrganizationID: centerID, BrandID: f.org.BrandID, CategoryID: cat.ID,
		Sku: fmt.Sprintf("T297-%d", time.Now().UnixNano()), Name: "PPF", DescriptionMd: "",
		MicronThickness: n, Images: []byte(`[]`), UnitType: "piece", UsesFixedBarcode: false, Active: true,
	})
	if err != nil {
		f.t.Fatalf("product: %v", err)
	}
	return p.ID
}

func (f *diffFixture) unit(productID int64) int64 {
	f.t.Helper()
	var id int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO units (organization_id, brand_id, product_id, barcode, unit_kind, source, status)
		VALUES ($1, $2, $3, $4, 'serial', 'generated', 'available') RETURNING id`,
		f.centerID(), f.org.BrandID, productID, fmt.Sprintf("T297-%d", time.Now().UnixNano())).Scan(&id); err != nil {
		f.t.Fatalf("unit: %v", err)
	}
	return id
}

func (f *diffFixture) centerID() int64 {
	f.t.Helper()
	var centerID int64
	if err := f.tx.QueryRow(f.ctx, `SELECT id FROM organizations WHERE brand_id = $1 AND type = 'center' ORDER BY id LIMIT 1`,
		f.org.BrandID).Scan(&centerID); err != nil {
		f.t.Fatalf("center org: %v", err)
	}
	return centerID
}

func (f *diffFixture) measurement(svc db.Service, value string, at time.Time) int64 {
	f.t.Helper()
	var id int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO measurement_results (organization_id, brand_id, service_id, vehicle_id, vin, status, raw, source, created_by, measured_at)
		VALUES ($1, $2, $3, $4, $5, 'accepted', '{"raw":true}'::jsonb, 'mobile', $6, $7) RETURNING id`,
		f.org.ID, f.org.BrandID, svc.ID, svc.VehicleID, svc.Vin, f.user.ID, at).Scan(&id); err != nil {
		f.t.Fatalf("measurement: %v", err)
	}
	f.measurementValue(id, value, false)
	return id
}

func (f *diffFixture) measurementValue(resultID int64, value string, inside bool) {
	f.t.Helper()
	n := pgtype.Numeric{}
	if err := n.Scan(value); err != nil {
		f.t.Fatalf("value: %v", err)
	}
	if _, err := f.q.InsertMeasurementValue(f.ctx, db.InsertMeasurementValueParams{
		OrganizationID: f.org.ID, BrandID: f.org.BrandID, ResultID: resultID,
		PlaceID: "top", PartType: "HOOD", IsInside: inside, ValueUm: n,
	}); err != nil {
		f.t.Fatalf("measurement value: %v", err)
	}
}

func (f *diffFixture) checkRequired(id int64) bool {
	f.t.Helper()
	var required bool
	if err := f.tx.QueryRow(f.ctx, `SELECT measurement_check_required FROM services WHERE id = $1`, id).Scan(&required); err != nil {
		f.t.Fatal(err)
	}
	return required
}

func strptr(s string) *string { return &s }

func TestMeasurementDiffFlagsDeviation(t *testing.T) {
	f := newDiffFixture(t)
	svc := f.service(strptr("190"), "100", "220")
	if err := f.linker.RecalculateServiceDiff(f.ctx, svc.ID); err != nil {
		t.Fatal(err)
	}
	if !f.checkRequired(svc.ID) {
		t.Fatal("measurement_check_required = false, want true")
	}
	diff, err := f.linker.ServiceMeasurementDiff(f.ctx, f.linkCaller(), svc.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Parts) != 1 || !diff.Parts[0].Deviation || *diff.Parts[0].DiffUM != "120.00" || *diff.Parts[0].ExpectedUM != "190.00" {
		t.Fatalf("diff = %+v", diff.Parts)
	}
}

func TestMeasurementDiffWithinToleranceDoesNotFlag(t *testing.T) {
	f := newDiffFixture(t)
	svc := f.service(strptr("190"), "100", "285")
	if err := f.linker.RecalculateServiceDiff(f.ctx, svc.ID); err != nil {
		t.Fatal(err)
	}
	if f.checkRequired(svc.ID) {
		t.Fatal("measurement_check_required = true, want false")
	}
}

func TestMeasurementDiffIgnoresInsideReadings(t *testing.T) {
	f := newDiffFixture(t)
	svc := f.service(strptr("190"), "100", "290")
	links, err := f.q.ListServiceMeasurements(f.ctx, db.ListServiceMeasurementsParams{
		ServiceID: svc.ID, OrganizationID: f.org.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range links {
		f.measurementValue(link.MeasurementResultID, "300", true)
	}
	if err := f.linker.RecalculateServiceDiff(f.ctx, svc.ID); err != nil {
		t.Fatal(err)
	}
	if f.checkRequired(svc.ID) {
		t.Fatal("measurement_check_required = true, want false")
	}
	diff, err := f.linker.ServiceMeasurementDiff(f.ctx, f.linkCaller(), svc.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Parts) != 1 || diff.Parts[0].Before.Count != 1 || diff.Parts[0].After.Count != 1 ||
		*diff.Parts[0].DiffUM != "190.00" {
		t.Fatalf("diff = %+v", diff.Parts)
	}
}

func TestMeasurementDiffMissingProductMicronDoesNotFlag(t *testing.T) {
	f := newDiffFixture(t)
	svc := f.service(nil, "100", "220")
	if err := f.linker.RecalculateServiceDiff(f.ctx, svc.ID); err != nil {
		t.Fatal(err)
	}
	if f.checkRequired(svc.ID) {
		t.Fatal("measurement_check_required = true, want false")
	}
	diff, err := f.linker.ServiceMeasurementDiff(f.ctx, f.linkCaller(), svc.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Parts) != 1 || diff.Parts[0].ExpectedUM != nil || diff.Parts[0].ExpectedStatus != "missing" {
		t.Fatalf("diff = %+v", diff.Parts)
	}
}

func TestMeasurementDiffUnlinkRecalculatesFlag(t *testing.T) {
	f := newDiffFixture(t)
	svc := f.service(strptr("190"), "100", "220")
	if err := f.linker.RecalculateServiceDiff(f.ctx, svc.ID); err != nil {
		t.Fatal(err)
	}
	if !f.checkRequired(svc.ID) {
		t.Fatal("measurement_check_required = false, want true before unlink")
	}
	if err := f.linker.UnlinkMeasurement(f.ctx, f.linkCaller(), svc.Uuid, PhaseAfter); err != nil {
		t.Fatal(err)
	}
	if f.checkRequired(svc.ID) {
		t.Fatal("measurement_check_required = true, want false after unlink")
	}
}
