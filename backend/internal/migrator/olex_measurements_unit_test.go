package migrator

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

func TestDeviceSerial(t *testing.T) {
	if s, fromUser, multi := DeviceSerial("dev-1", map[string]bool{"SN-1": true}); s != "SN-1" || fromUser || multi {
		t.Errorf("one report serial = %q, %v, %v", s, fromUser, multi)
	}
	if s, fromUser, multi := DeviceSerial(" dev-1 ", nil); s != "dev-1" || !fromUser || multi {
		t.Errorf("no report serial = %q, %v, %v", s, fromUser, multi)
	}
	if s, fromUser, multi := DeviceSerial("dev-1", map[string]bool{"A": true, "B": true}); s != "dev-1" || !fromUser || !multi {
		t.Errorf("several report serials = %q, %v, %v", s, fromUser, multi)
	}
	if s, _, _ := DeviceSerial(strings.Repeat("x", 80), nil); len(s) != measurementSerialMax {
		t.Errorf("long username not cut: %d", len(s))
	}
}

func TestMeasurementVIN(t *testing.T) {
	cases := []struct {
		raw, vin string
		ok       bool
	}{
		{"synvin00000000001", "SYNVIN00000000001", true},
		{" WVWZZZ1JZXW000001 ", "WVWZZZ1JZXW000001", true},
		{"", "", true},
		{"   ", "", true},
		{"ABC-123", "", false},
		{"SHORT", "", false},
		{strings.Repeat("A", 18), "", false},
	}
	for _, c := range cases {
		vin, ok := MeasurementVIN(c.raw)
		if vin != c.vin || ok != c.ok {
			t.Errorf("MeasurementVIN(%q) = %q, %v; want %q, %v", c.raw, vin, ok, c.vin, c.ok)
		}
	}
}

func TestMeasurementRaw(t *testing.T) {
	at := sql.NullTime{Time: time.Date(2025, 3, 10, 8, 31, 0, 0, time.UTC), Valid: true}
	rep := &legacyReport{
		ID: 7, Name: "R", Date: at, VIN: sql.NullString{String: "X", Valid: true},
		ExtraFields: []byte(`{"source": "device"}`),
		measurements: []legacyMeasurement{
			{ID: 1, ReportID: 7, PlaceID: "left", PartType: "hood", Value: sql.NullString{String: "112.50", Valid: true}, Timestamp: at},
			{ID: 2, ReportID: 7, PlaceID: "top", PartType: "roof"},
		},
		link: &legacyServiceLink{ID: 3, ServiceID: 9, ReportID: 7, MatchType: "before"},
	}
	raw, err := MeasurementRaw("hub", rep)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Legacy       map[string]any   `json:"legacy"`
		Report       map[string]any   `json:"report"`
		Measurements []map[string]any `json:"measurements"`
		ServiceLink  map[string]any   `json:"service_link"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Measurements) != 2 {
		t.Fatalf("measurements = %d, want 2", len(doc.Measurements))
	}
	if doc.Measurements[0]["value"] != 112.5 || doc.Measurements[1]["value"] != nil {
		t.Errorf("values = %v / %v", doc.Measurements[0]["value"], doc.Measurements[1]["value"])
	}
	if doc.Measurements[0]["timestamp"] != "2025-03-10 08:31:00" {
		t.Errorf("timestamp = %v", doc.Measurements[0]["timestamp"])
	}
	if doc.Report["vin"] != "X" || doc.Report["extra_fields"].(map[string]any)["source"] != "device" {
		t.Errorf("report = %v", doc.Report)
	}
	if doc.ServiceLink["match_type"] != "before" || doc.ServiceLink["service_id"] != float64(9) {
		t.Errorf("service link = %v", doc.ServiceLink)
	}
	if doc.Legacy["table"] != "nexptg_reports" || doc.Legacy["id"] != float64(7) {
		t.Errorf("legacy = %v", doc.Legacy)
	}
	// Same input, same bytes: the checksum of a rerun does not move.
	again, _ := MeasurementRaw("hub", rep)
	if string(again) != string(raw) {
		t.Error("raw is not deterministic")
	}
	// A report without a link carries no service_link; no rows is [].
	rep.link, rep.measurements = nil, nil
	raw, _ = MeasurementRaw("hub", rep)
	if strings.Contains(string(raw), "service_link") || !strings.Contains(string(raw), `"measurements":[]`) {
		t.Errorf("bare report raw = %s", raw)
	}
}

// The step queries must pass the read-only source guard (HANDOFF §5) and
// never read the device passwords.
func TestMeasurementQueriesPassGuard(t *testing.T) {
	for _, q := range []string{nexptgAPIUsersQuery, nexptgReportsQuery, nexptgMeasurementsQuery, nexptgServiceLinksQuery} {
		if err := source.CheckReadOnly(q); err != nil {
			t.Errorf("%q: %v", q, err)
		}
	}
	if strings.Contains(nexptgAPIUsersQuery, "password") {
		t.Error("the api user query reads the password")
	}
}

func TestOlexStepsEndWithMeasurements(t *testing.T) {
	steps := olexSteps()
	if got := steps[len(steps)-1].Name(); got != "measurements" {
		t.Errorf("last olex step = %q, want measurements", got)
	}
}
