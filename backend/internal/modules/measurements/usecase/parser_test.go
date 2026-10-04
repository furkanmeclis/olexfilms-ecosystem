package usecase

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fixtureMeasuredAt is the report date of both fixtures: Unix 1702629560 in
// the mobile one, "2023-12-15 11:39:20" (Europe/Istanbul) in the legacy one.
var fixtureMeasuredAt = time.Date(2023, 12, 15, 8, 39, 20, 0, time.UTC)

// TEC-294: the mobile NexPTG fixture parses to 15 readings (13 outside, 2
// inside; the "-" reading is skipped), 4 tires and the report date.
func TestParseRawMobileFixture(t *testing.T) {
	rep, err := ParseRaw(fixture(t, "nexptg_mobile.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !rep.MeasuredAt.Equal(fixtureMeasuredAt) {
		t.Fatalf("measured_at = %v", rep.MeasuredAt)
	}
	inside := 0
	for _, v := range rep.Values {
		if v.IsInside {
			inside++
		}
	}
	if len(rep.Values) != 15 || inside != 2 || len(rep.Tires) != 4 {
		t.Fatalf("values %d (inside %d) tires %d", len(rep.Values), inside, len(rep.Tires))
	}
	if rep.DeviceSerial != "18416 Professional" || rep.DeviceModel != "Professional" || rep.BodyType != "SEDAN" {
		t.Fatalf("device %q model %q body %q", rep.DeviceSerial, rep.DeviceModel, rep.BodyType)
	}
	v := rep.Values[0]
	if v.PlaceID != "left" || v.PartType != "LEFT_FRONT_FENDER" || *v.ValueUM != "375.00" || *v.Interpretation != 3 ||
		*v.SubstrateType != "Al" || *v.Position != 1 || v.MeasuredAt == nil {
		t.Fatalf("first reading = %+v", v)
	}
	tire := rep.Tires[0]
	if *tire.Section != "Left front" || *tire.TreadDepth1MM != "6.50" || *tire.TreadDepth2MM != "6.00" {
		t.Fatalf("first tire = %+v", tire)
	}
	if last := rep.Tires[3]; last.TreadDepth1MM != nil || last.TreadDepth2MM != nil || *last.Maker != "Michelin" {
		t.Fatalf("tire with no depth = %+v", last)
	}
}

// TEC-294: the migrator (TEC-262 legacy_import) fixture of the same report
// gives the same readings, measured_at, device and body type. The migrator
// does not carry tires into raw (open question in the PR).
func TestParseRawLegacyFixtureMatchesMobile(t *testing.T) {
	mobile, err := ParseRaw(fixture(t, "nexptg_mobile.json"))
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := ParseRaw(fixture(t, "nexptg_legacy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.MeasuredAt.Equal(mobile.MeasuredAt) {
		t.Fatalf("legacy measured_at %v, mobile %v", legacy.MeasuredAt, mobile.MeasuredAt)
	}
	if !reflect.DeepEqual(legacy.Values, mobile.Values) {
		t.Fatalf("legacy readings differ:\nlegacy %+v\nmobile %+v", legacy.Values, mobile.Values)
	}
	if legacy.DeviceSerial != mobile.DeviceSerial || legacy.DeviceModel != mobile.DeviceModel || legacy.BodyType != mobile.BodyType {
		t.Fatalf("legacy device %q/%q body %q", legacy.DeviceSerial, legacy.DeviceModel, legacy.BodyType)
	}
	if len(legacy.Tires) != 0 {
		t.Fatalf("legacy tires = %d", len(legacy.Tires))
	}
}

// The old app's flat body (TEC-234 alias) and the NexPTG sync envelope.
func TestParseRawOtherShapes(t *testing.T) {
	alias := []byte(`{"client_measurement_id":"legacy-x","measured_at":"2024-03-01T09:00:00+03:00",
		"device":{"serial":"mobile-ab12cd34"},
		"raw":{"body_type":"sedan_4d","date":"2024-03-01 09:00:00","device_serial_number":"mobile-ab12cd34",
		"measurements":[{"part_type":"HOOD","place_id":"top","position":1,"value":112.5,"interpretation":1},
		{"part_type":"ROOF","place_id":"top","position":2,"value":"98","is_inside":true,"timestamp":"2024-03-01 09:01:00"}]}}`)
	rep, err := ParseRaw(alias)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.MeasuredAt.Equal(time.Date(2024, 3, 1, 6, 0, 0, 0, time.UTC)) || len(rep.Values) != 2 ||
		rep.BodyType != "sedan_4d" || rep.DeviceSerial != "mobile-ab12cd34" || rep.DeviceModel != "" {
		t.Fatalf("alias = %+v", rep)
	}
	if *rep.Values[0].ValueUM != "112.50" || rep.Values[0].MeasuredAt != nil || !rep.Values[1].IsInside ||
		!rep.Values[1].MeasuredAt.Equal(time.Date(2024, 3, 1, 6, 1, 0, 0, time.UTC)) {
		t.Fatalf("alias readings = %+v", rep.Values)
	}

	envelope := []byte(`{"raw":{"data":{"history":[],"reports":[{"date":1702629560,"deviceSerialNumber":"77",
		"data":[{"placeId":"left","data":[{"type":"LEFT_FRONT_DOOR","values":[{"value":110,"timestamp":1702629434,"position":1}]}]}]}]}}}`)
	rep, err = ParseRaw(envelope)
	if err != nil || len(rep.Values) != 1 || rep.DeviceSerial != "77" || !rep.MeasuredAt.Equal(fixtureMeasuredAt) {
		t.Fatalf("envelope = %+v, %v", rep, err)
	}

	// No report date and no measured_at: the latest reading time.
	noDate := []byte(`{"raw":{"data":[{"placeId":"left","data":[{"type":"LEFT_FRONT_DOOR","values":[
		{"value":"110","timestamp":1702629434},{"value":"111","timestamp":1702629440}]}]}]}}`)
	rep, err = ParseRaw(noDate)
	if err != nil || !rep.MeasuredAt.Equal(time.Unix(1702629440, 0)) {
		t.Fatalf("no date = %+v, %v", rep.MeasuredAt, err)
	}
}

// TEC-294: a record without readings or without a time is not parseable;
// out-of-range values are dropped, not failed.
func TestParseRawBroken(t *testing.T) {
	for name, raw := range map[string]string{
		"not json":       `nope`,
		"array":          `[1,2]`,
		"tec-233 sample": `{"raw":{"device":"NexPTG","values":[98,102]}}`,
		"no time":        `{"raw":{"data":[{"placeId":"left","data":[{"type":"HOOD","values":[{"value":"110"}]}]}]}}`,
		"only dashes":    `{"raw":{"date":1702629560,"data":[{"placeId":"left","data":[{"type":"HOOD","values":[{"value":"-","timestamp":-1}]}]}]}}`,
		"no place":       `{"raw":{"date":1702629560,"data":[{"data":[{"type":"HOOD","values":[{"value":"110"}]}]}]}}`,
	} {
		if _, err := ParseRaw([]byte(raw)); !errors.Is(err, ErrUnparseable) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	rep, err := ParseRaw([]byte(`{"raw":{"date":1702629560,"data":[{"placeId":"left","data":[{"type":"HOOD","values":[
		{"value":"99999999","interpretation":9,"timestamp":1702629434,"position":"x"}]}]}]}}`))
	if err != nil || len(rep.Values) != 1 {
		t.Fatalf("out of range = %+v, %v", rep, err)
	}
	if v := rep.Values[0]; v.ValueUM != nil || v.Interpretation != nil || v.Position != nil {
		t.Fatalf("out of range reading = %+v", v)
	}
}
