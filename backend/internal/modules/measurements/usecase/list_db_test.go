package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// listRow inserts one measurement result of f.org; plate adds a vehicle.
func (f *dbFixture) listRow(vin, serial, plate string, measuredAt *time.Time, createdAt time.Time) uuid.UUID {
	f.t.Helper()
	var vehicleID *int64
	if plate != "" {
		var id int64
		norm := strings.ToUpper(strings.ReplaceAll(plate, " ", ""))
		if err := f.tx.QueryRow(f.ctx, `INSERT INTO vehicles (user_id, organization_id, brand_id, plate, plate_normalized, plate_country)
			VALUES ($1, $2, $3, $4, $5, 'TR') RETURNING id`, f.user.ID, f.org.ID, f.org.BrandID, plate, norm).Scan(&id); err != nil {
			f.t.Fatalf("vehicle: %v", err)
		}
		vehicleID = &id
	}
	var vinArg *string
	status := StatusVINPending
	if vin != "" {
		vinArg, status = &vin, StatusAccepted
	}
	var id uuid.UUID
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO measurement_results
		(organization_id, brand_id, vehicle_id, vin, status, raw, device_serial, created_by, measured_at, created_at)
		VALUES ($1, $2, $3, $4, $5, '{}', $6, $7, $8, $9) RETURNING uuid`,
		f.org.ID, f.org.BrandID, vehicleID, vinArg, status, serial, f.user.ID, measuredAt, createdAt).Scan(&id); err != nil {
		f.t.Fatalf("result: %v", err)
	}
	return id
}

// TEC-299: GET /v1/measurements sort / q / multi-value status / plate.
func TestDBListMeasurementsContract(t *testing.T) {
	f := newDBFixture(t)
	s := f.svc()
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	a := f.listRow("WVWZZZ1JZ3W000001", "NX-100", "34 ABC 12", &t0, t0)
	b := f.listRow("", "NX_200", "", &t1, t0)
	c := f.listRow("WVWZZZ1JZ3W000003", "NX-300", "06 XYZ 99", nil, t0.Add(-time.Hour))

	list := func(mf MeasurementFilter) []uuid.UUID {
		t.Helper()
		mf.Limit = 20
		items, total, err := s.ListMeasurements(f.ctx, f.panel(f.org), mf)
		if err != nil {
			t.Fatal(err)
		}
		if int(total) != len(items) {
			t.Fatalf("total %d != %d", total, len(items))
		}
		out := make([]uuid.UUID, 0, len(items))
		for _, it := range items {
			out = append(out, it.UUID)
		}
		return out
	}
	eq := func(name string, got []uuid.UUID, want ...uuid.UUID) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: got %d rows, want %d", name, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: row %d = %s, want %s (a=%s b=%s c=%s)", name, i, got[i], want[i], a, b, c)
			}
		}
	}
	sorted := func(key string, desc bool) []uuid.UUID {
		return list(MeasurementFilter{Sort: apiquery.ResolvedSort{Key: key, Desc: desc}})
	}

	eq("default", list(MeasurementFilter{}), b, a, c)
	eq("measured_at asc", sorted("measured_at", false), c, a, b)
	eq("created_at asc", sorted("created_at", false), c, a, b)
	eq("plate asc", sorted("plate", false), c, a, b)
	eq("plate desc", sorted("plate", true), a, c, b)
	eq("vin asc", sorted("vin", false), a, c, b)
	eq("vin desc", sorted("vin", true), c, a, b)
	eq("status asc", sorted("status", false), a, c, b)
	eq("status desc", sorted("status", true), b, c, a)

	eq("q plate", list(MeasurementFilter{Q: "abc"}), a)
	eq("q vin", list(MeasurementFilter{Q: "w000003"}), c)
	eq("q serial escaped", list(MeasurementFilter{Q: "nx_"}), b)
	eq("q percent", list(MeasurementFilter{Q: "%"}))

	eq("status single", list(MeasurementFilter{Statuses: []string{StatusVINPending}}), b)
	eq("status csv", list(MeasurementFilter{Statuses: []string{StatusAccepted, StatusVINPending}}), b, a, c)

	items, _, err := s.ListMeasurements(f.ctx, f.panel(f.org), MeasurementFilter{Q: "abc", Limit: 20})
	if err != nil || len(items) != 1 || items[0].Plate == nil || *items[0].Plate != "34 ABC 12" {
		t.Fatalf("list plate = %+v, %v", items, err)
	}
	det, err := s.GetMeasurement(f.ctx, f.panel(f.org), a)
	if err != nil || det.Plate == nil || *det.Plate != "34 ABC 12" {
		t.Fatalf("detail plate = %+v, %v", det.Plate, err)
	}
	det, err = s.GetMeasurement(f.ctx, f.panel(f.org), b)
	if err != nil || det.Plate != nil {
		t.Fatalf("detail without vehicle = %+v, %v", det.Plate, err)
	}
}
