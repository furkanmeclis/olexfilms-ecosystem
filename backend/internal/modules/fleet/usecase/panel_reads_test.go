package usecase

import (
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// TEC-477: the panel lists its plans of a fleet and opens one with the
// vehicle plates and the draft services of the intake; another dealer's
// plan and an unlinked dealer are 404.
func TestFleetServicePlanListAndDetail(t *testing.T) {
	f := newAPIFixture(t)
	f.settings(t, f.d1, 3)
	opened, vehicles := f.fleetWithVehicles(t, 2)
	plan, err := f.svc.CreateServicePlan(f.ctx, f.dealer(f.d1), opened.UUID,
		ServicePlanInput{VehicleUUIDs: uuidsOf(vehicles), ServiceType: "PPF", StartDate: "2026-10-05"}, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.CreateServicePlan(f.ctx, f.dealer(f.d1), opened.UUID,
		ServicePlanInput{VehicleUUIDs: uuidsOf(vehicles[:1]), ServiceType: "Seramik", StartDate: "2026-10-12"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CancelServicePlan(f.ctx, f.dealer(f.d1), opened.UUID, second.UUID, ""); err != nil {
		t.Fatal(err)
	}

	items, total, err := f.svc.ListServicePlans(f.ctx, f.dealer(f.d1), opened.UUID, PlanFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 2 || items[0].UUID != second.UUID {
		t.Fatalf("plans = %d %+v", total, items)
	}
	if items[1].AppointmentCount != 2 || items[1].StartDate != "2026-10-05" || items[1].Status != "scheduled" {
		t.Fatalf("plan row = %+v", items[1])
	}
	items, total, err = f.svc.ListServicePlans(f.ctx, f.dealer(f.d1), opened.UUID, PlanFilter{
		Statuses: []string{"scheduled"}, Sort: []apiquery.SortField{{Field: "start_date"}}, Limit: 20,
	})
	if err != nil || total != 1 || items[0].UUID != plan.UUID {
		t.Fatalf("scheduled plans = %d %+v %v", total, items, err)
	}
	if _, _, err := f.svc.ListServicePlans(f.ctx, f.dealer(f.d1), opened.UUID, PlanFilter{
		Sort: []apiquery.SortField{{Field: "nope"}}, Limit: 20,
	}); err == nil {
		t.Fatal("unknown sort accepted")
	}

	// A row-wise intake opens a draft service; the detail names it.
	svc := f.completedService(t, f.d1, vehicles[0].UUID)
	if _, err := f.tx.Exec(f.ctx, `UPDATE appointments SET service_id = $2 WHERE uuid = $1`,
		plan.Appointments[0].UUID, svc.ID); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.GetServicePlan(f.ctx, f.dealer(f.d1), opened.UUID, plan.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Appointments) != 2 {
		t.Fatalf("appointments = %+v", got.Appointments)
	}
	plates := map[string]bool{}
	for i, ap := range got.Appointments {
		if (ap.ServiceUUID != nil) != (i == 0) || (i == 0 && *ap.ServiceUUID != svc.Uuid) {
			t.Fatalf("appointment %d service = %v", i, ap.ServiceUUID)
		}
		if ap.VehicleUUID == nil || ap.VehiclePlate == nil || ap.VehicleLabel == nil {
			t.Fatalf("appointment refs = %+v", ap)
		}
		plates[*ap.VehiclePlate] = true
	}
	for _, v := range vehicles {
		if v.Plate == nil || !plates[*v.Plate] {
			t.Fatalf("plates %v miss %+v", plates, v.Plate)
		}
	}

	if _, err := f.svc.GetServicePlan(f.ctx, f.dealer(f.d1), opened.UUID, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown plan err = %v", err)
	}
	if _, err := f.svc.GetServicePlan(f.ctx, f.dealer(f.d2), opened.UUID, plan.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked dealer err = %v", err)
	}
	if _, _, err := f.svc.ListServicePlans(f.ctx, f.dealer(f.d2), opened.UUID, PlanFilter{Limit: 20}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked dealer list err = %v", err)
	}
}

// TEC-477: the panel report list shows every status of the fleet, filters
// by status and period kind, and a not-ready report has no file.
func TestFleetReportPanelList(t *testing.T) {
	f := newAPIFixture(t)
	opened, _ := f.fleetWithVehicles(t, 0)
	fleet, err := f.q.GetFleetByUUID(f.ctx, opened.UUID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct {
		kind       string
		start, end time.Time
	}{
		{"monthly", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)},
		{"monthly", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)},
		{"quarterly", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)},
	} {
		if _, err := f.q.UpsertFleetReport(f.ctx, db.UpsertFleetReportParams{
			FleetOrgID: fleet.Organization.ID, BrandID: f.brand.ID, PeriodKind: p.kind,
			PeriodStart: pgDate(p.start), PeriodEnd: pgDate(p.end), Locale: "tr",
		}); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := f.svc.ListReports(f.ctx, f.dealer(f.d1), opened.UUID, ReportFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || items[0].PeriodStart != "2026-09-01" || items[0].Status != "pending" {
		t.Fatalf("reports = %d %+v", total, items)
	}
	items, total, err = f.svc.ListReports(f.ctx, f.dealer(f.d1), opened.UUID, ReportFilter{
		PeriodKinds: []string{"monthly"}, Sort: []apiquery.SortField{{Field: "period_start"}}, Limit: 20,
	})
	if err != nil || total != 2 || items[0].PeriodStart != "2026-08-01" {
		t.Fatalf("monthly reports = %d %+v %v", total, items, err)
	}
	if _, total, _ := f.svc.ListReports(f.ctx, f.dealer(f.d1), opened.UUID, ReportFilter{Statuses: []string{"ready"}, Limit: 20}); total != 0 {
		t.Fatalf("ready reports = %d", total)
	}
	if _, _, err := f.svc.ReportFile(f.ctx, f.dealer(f.d1), opened.UUID, items[0].UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pending report file err = %v", err)
	}
	if _, _, err := f.svc.ListReports(f.ctx, f.dealer(f.d2), opened.UUID, ReportFilter{Limit: 20}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked dealer err = %v", err)
	}
}

// TEC-477: the "new fleet" dialog looks the VKN up first: a fleet of the
// brand is found with the caller's link status (empty for a second dealer),
// an unknown valid number is ErrNotFound, a bad checksum is rejected.
func TestFleetLookupByTaxNumber(t *testing.T) {
	f := newAPIFixture(t)
	tax := validVKN(t, f.suffix)
	opened, err := f.svc.Open(f.ctx, f.dealer(f.d1), OpenInput{LegalName: "T477 Filo A.Ş.", TaxNumber: tax})
	if err != nil {
		t.Fatal(err)
	}
	own, err := f.svc.Lookup(f.ctx, f.dealer(f.d1), " "+tax+" ")
	if err != nil || own.FleetUUID != opened.UUID || own.LegalName != "T477 Filo A.Ş." || own.LinkStatus != "active" {
		t.Fatalf("own lookup = %+v %v", own, err)
	}
	other, err := f.svc.Lookup(f.ctx, f.dealer(f.d2), tax)
	if err != nil || other.FleetUUID != opened.UUID || other.LinkStatus != "" {
		t.Fatalf("second dealer lookup = %+v %v", other, err)
	}
	first := byte('1')
	if tax[0] == first {
		first = '2'
	}
	unknown := validVKN(t, string(first)+tax[1:9])
	if _, err := f.svc.Lookup(f.ctx, f.dealer(f.d2), unknown); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown lookup err = %v", err)
	}
	bad := tax[:9] + string('0'+(tax[9]-'0'+1)%10)
	if _, err := f.svc.Lookup(f.ctx, f.dealer(f.d2), bad); !errors.Is(err, ErrInvalidTaxNumber) {
		t.Fatalf("bad checksum err = %v", err)
	}
	if _, err := f.svc.Lookup(f.ctx, f.dealer(f.dist), ""); err == nil {
		t.Fatal("empty tax number accepted")
	}
}
