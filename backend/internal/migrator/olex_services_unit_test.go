package migrator

import (
	"database/sql"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// TEC-259: the step queries pass the read-only guard, with and without the
// delta filter (no REPLACE / SET / INTO, a single SELECT).
func TestServicesQueriesAreReadOnly(t *testing.T) {
	for _, q := range []string{
		servicesQuery + " ORDER BY id",
		servicesQuery + " WHERE COALESCE(updated_at, created_at) > ? ORDER BY id",
		serviceItemsQuery + " WHERE COALESCE(updated_at, created_at) > ? ORDER BY id",
		serviceImagesQuery + " WHERE COALESCE(i.updated_at, i.created_at) > ? ORDER BY i.id",
		serviceStatusLogsQuery + " WHERE COALESCE(updated_at, created_at) > ? ORDER BY id",
	} {
		if err := source.CheckReadOnly(q); err != nil {
			t.Errorf("%v: %s", err, q)
		}
	}
}

// The status map covers the legacy ServiceStatusEnum and lands only on the
// statuses chk_services_status accepts.
func TestServiceStatusMap(t *testing.T) {
	legacy := []string{"draft", "pending", "processing", "ready", "completed", "cancelled"}
	allowed := map[string]bool{"draft": true, "pending": true, "processing": true, "ready": true,
		"completed": true, "cancelled": true}
	for _, s := range legacy {
		got, ok := serviceStatusMap[s]
		if !ok || !allowed[got] {
			t.Errorf("legacy status %q -> %q, %v", s, got, ok)
		}
	}
	if len(serviceStatusMap) != len(legacy) {
		t.Errorf("status map has %d entries, want %d", len(serviceStatusMap), len(legacy))
	}
	if _, ok := serviceStatusMap["archived"]; ok {
		t.Error("unknown status mapped")
	}
}

func TestNormalizeServiceVehicle(t *testing.T) {
	v, notes := normalizeServiceVehicle(legacyService{
		VIN: " wvwzzz1jzxw000001 ", Plate: "34 SYN 001", PlateCountry: "tr",
		Year: sql.NullInt64{Int64: 2020, Valid: true},
	})
	if v.VIN.String != "WVWZZZ1JZXW000001" || v.Plate.String != "34 SYN 001" || v.PlateNorm.String != "34SYN001" ||
		v.Country.String != "TR" || v.Year.Int16 != 2020 || len(notes) != 0 {
		t.Errorf("normalized = %+v, notes %v", v, notes)
	}

	// I/O/Q is not a VIN (the fixture VINs contain an I), an empty plate is no plate, a year out of range is
	// dropped, a bad country falls back to TR.
	v, notes = normalizeServiceVehicle(legacyService{VIN: "SYNVIN00000000001", Year: sql.NullInt64{Int64: 1800, Valid: true}})
	if v.VIN.Valid || v.Plate.Valid || v.Country.Valid || v.Year.Valid {
		t.Errorf("normalized = %+v", v)
	}
	if want := []string{"vin_invalid", "plate_missing", "year_invalid"}; len(notes) != 3 ||
		notes[0] != want[0] || notes[1] != want[1] || notes[2] != want[2] {
		t.Errorf("notes = %v, want %v", notes, want)
	}
	v, notes = normalizeServiceVehicle(legacyService{Plate: "B-AB 123 und viel zu lang", PlateCountry: "Deutschland"})
	if v.Country.String != TRCountry || len([]rune(v.Plate.String)) != servicePlateMax || len(notes) != 1 {
		t.Errorf("normalized = %+v, notes %v", v, notes)
	}
}

func TestServiceStatusTimes(t *testing.T) {
	at := func(s string) sql.NullTime {
		ts, err := time.Parse(time.DateTime, s)
		if err != nil {
			t.Fatal(err)
		}
		return sql.NullTime{Time: ts, Valid: true}
	}
	ls := legacyService{CompletedAt: at("2025-03-10 17:00:00"), UpdatedAt: at("2025-03-11 09:00:00")}
	done, cancelled, fallback := serviceStatusTimes(ls, serviceCompleted)
	if !done.Valid || !done.Time.Equal(ls.CompletedAt.Time) || cancelled.Valid || fallback {
		t.Errorf("completed = %+v %+v %v", done, cancelled, fallback)
	}
	ls.CompletedAt = sql.NullTime{}
	if done, _, fallback = serviceStatusTimes(ls, serviceCompleted); !done.Time.Equal(ls.UpdatedAt.Time) || !fallback {
		t.Errorf("completed without time = %+v %v", done, fallback)
	}
	done, cancelled, _ = serviceStatusTimes(ls, serviceCancelled)
	if done.Valid || !cancelled.Valid {
		t.Errorf("cancelled = %+v %+v", done, cancelled)
	}
	ls.CompletedAt = at("2025-03-10 17:00:00")
	if done, cancelled, _ = serviceStatusTimes(ls, "processing"); done.Valid || cancelled.Valid {
		t.Errorf("processing = %+v %+v", done, cancelled)
	}
}

func TestItemParts(t *testing.T) {
	got, dropped := itemParts([]string{"hood", "roof", "hood", "door"}, []byte(`["hood","roof","front_bumper"]`))
	if string(got) != `["hood","roof"]` || dropped != 1 {
		t.Errorf("itemParts = %s, %d", got, dropped)
	}
	if got, dropped = itemParts(nil, []byte(`not json`)); string(got) != `[]` || dropped != 0 {
		t.Errorf("itemParts(nil) = %s, %d", got, dropped)
	}
	if parts := legacyServiceParts([]byte(`{"a":1}`)); parts != nil {
		t.Errorf("legacyServiceParts(object) = %v", parts)
	}
}
