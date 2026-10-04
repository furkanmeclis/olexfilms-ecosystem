package usecase

// TEC-294 (F3-02b): the NexPTG raw record parser.
//
// measurement_results.raw arrives in three shapes; ParseRaw reads all of
// them into one Report:
//
//  1. Mobile upload (POST /v1/mobile/measurements, TEC-233): the request
//     body; the device report sits under raw.raw. The report is the NexPTG
//     report object (docs/nexptg/all.md §3.3: id, date (Unix seconds),
//     deviceSerialNumber, typeOfBody, data / dataInside grouped by placeId,
//     tires), optionally still wrapped in the sync envelope
//     {"data": {"reports": [...]}} or {"reports": [...]}.
//  2. Old mobile app alias (POST /api/v1/nexptg-reports, TEC-234): the same
//     body with raw.raw holding the hub's StoreNexptgReportRequest fields
//     (date as text, device_serial_number, body_type and a flat
//     measurements list of place_id / part_type / value / ...).
//  3. Migrator import (source legacy_import, TEC-262): {legacy, report,
//     measurements, service_link?}; report holds the hub nexptg_reports
//     columns, measurements the flat nexptg_report_measurements rows. The
//     hub stored zone-less times in its app timezone (Europe/Istanbul).
//
// Unknown fields are ignored. A record without a measurement time or
// without a single reading is not parseable (ErrUnparseable): the caller
// keeps the raw record and leaves parsed_at NULL.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Europe/Istanbul without a system zoneinfo
	"unicode/utf8"
)

// ErrUnparseable wraps the reason a raw record cannot be normalized.
var ErrUnparseable = errors.New("measurements: raw record is not parseable")

// Column limits of measurement_values / measurement_tires (000085) and
// measurement_results.body_type / measurement_devices.model.
const (
	maxPlaceID   = 16
	maxPartType  = 32
	maxSubstrate = 32
	maxBodyType  = 64
	maxTireSize  = 16
	maxTireMaker = 64
	maxTireText  = 32
	// NUMERIC(8,2) and NUMERIC(5,2).
	maxValueUM    = 999999.99
	maxTreadDepth = 999.99
)

// legacyZone is the hub's app timezone: its zone-less times are local.
var legacyZone = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*3600)
	}
	return loc
}()

// Report is a parsed raw record.
type Report struct {
	MeasuredAt   time.Time
	DeviceSerial string
	DeviceModel  string
	BodyType     string
	Values       []Reading
	Tires        []Tire
}

// Reading is one paint thickness reading (a measurement_values row).
type Reading struct {
	PlaceID        string
	PartType       string
	IsInside       bool
	Position       *int32
	ValueUM        *string // decimal text, 2 places
	Interpretation *int16
	SubstrateType  *string
	MeasuredAt     *time.Time
}

// Tire is one tire of the report (a measurement_tires row).
type Tire struct {
	Section, Width, Profile, Diameter, Maker, Season *string
	TreadDepth1MM, TreadDepth2MM                     *string // decimal text, 2 places
}

type obj = map[string]any

func unparseable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnparseable, fmt.Sprintf(format, args...))
}

// ParseRaw parses one measurement_results.raw document.
func ParseRaw(raw []byte) (Report, error) {
	var out Report
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc obj
	if err := dec.Decode(&doc); err != nil || doc == nil {
		return out, unparseable("raw is not a JSON object")
	}

	// Shape 1/2: the mobile request body with the report under raw.
	var outer obj
	inner := doc
	if r, ok := doc["raw"].(obj); ok {
		outer, inner = doc, r
	}

	report := inner
	var flat []any
	hasFlat := false
	if rep, ok := inner["report"].(obj); ok {
		// Shape 3: migrator legacy_import.
		report = rep
		flat, hasFlat = inner["measurements"].([]any)
	} else if rep := unwrapSync(inner); rep != nil {
		report = rep
	}
	if !hasFlat {
		flat, hasFlat = report["measurements"].([]any)
	}

	if hasFlat {
		out.Values = flatReadings(flat)
	} else {
		out.Values = append(nestedReadings(report["data"], false), nestedReadings(report["dataInside"], true)...)
	}
	tires, _ := report["tires"].([]any)
	if tires == nil {
		tires, _ = inner["tires"].([]any)
	}
	out.Tires = parseTires(tires)

	// Measurement time: the report date, then the upload's measured_at, then
	// the latest reading.
	if t := parseTime(first(report, "date")); t != nil {
		out.MeasuredAt = *t
	} else if t := parseTime(first(outer, "measured_at")); t != nil {
		out.MeasuredAt = *t
	} else {
		for _, v := range out.Values {
			if v.MeasuredAt != nil && v.MeasuredAt.After(out.MeasuredAt) {
				out.MeasuredAt = *v.MeasuredAt
			}
		}
	}

	device, _ := outer["device"].(obj)
	out.DeviceSerial = clip(firstText(device, "serial"), maxSerial)
	if out.DeviceSerial == "" {
		out.DeviceSerial = clip(firstText(report, "deviceSerialNumber", "device_serial_number"), maxSerial)
	}
	out.DeviceModel = clip(firstText(device, "model"), maxModel)
	if out.DeviceModel == "" {
		out.DeviceModel = clip(serialModel(out.DeviceSerial), maxModel)
	}
	out.BodyType = firstText(outer, "body_type")
	if out.BodyType == "" {
		out.BodyType = firstText(report, "body_type", "typeOfBody", "type_of_body")
	}
	out.BodyType = clip(out.BodyType, maxBodyType)

	if out.MeasuredAt.IsZero() {
		return out, unparseable("measurement time missing")
	}
	if len(out.Values) == 0 {
		return out, unparseable("no readings")
	}
	return out, nil
}

// unwrapSync returns the single report of a NexPTG sync envelope
// ({"data": {"reports": [...]}} or {"reports": [...]}), nil otherwise.
func unwrapSync(m obj) obj {
	reports, ok := m["reports"].([]any)
	if !ok {
		if d, isObj := m["data"].(obj); isObj {
			reports, ok = d["reports"].([]any)
		}
	}
	if !ok || len(reports) == 0 {
		return nil
	}
	rep, _ := reports[0].(obj)
	return rep
}

// nestedReadings reads the NexPTG data / dataInside list:
// [{placeId, data: [{type, values: [{value, interpretation, type, timestamp, position}]}]}].
func nestedReadings(v any, inside bool) []Reading {
	places, _ := v.([]any)
	var out []Reading
	for _, p := range places {
		place, _ := p.(obj)
		placeID := firstText(place, "placeId", "place_id")
		if placeID == "" {
			continue
		}
		parts, _ := place["data"].([]any)
		for _, pt := range parts {
			part, _ := pt.(obj)
			partType := firstText(part, "type", "part_type")
			values, _ := part["values"].([]any)
			for _, vv := range values {
				val, _ := vv.(obj)
				if r, ok := reading(placeID, partType, inside, val, "type"); ok {
					out = append(out, r)
				}
			}
		}
	}
	return out
}

// flatReadings reads the hub's flat rows: [{place_id, part_type, is_inside,
// value, interpretation, substrate_type, timestamp, position}].
func flatReadings(rows []any) []Reading {
	var out []Reading
	for _, rv := range rows {
		row, _ := rv.(obj)
		if r, ok := reading(firstText(row, "place_id", "placeId"), firstText(row, "part_type", "type"),
			parseBool(row["is_inside"]), row, "substrate_type"); ok {
			out = append(out, r)
		}
	}
	return out
}

// reading builds one row; like the hub's sync, a reading with neither a
// value nor a time ("-") is skipped, as is one without a place or part.
func reading(placeID, partType string, inside bool, v obj, substrateKey string) (Reading, bool) {
	if v == nil || placeID == "" || partType == "" {
		return Reading{}, false
	}
	r := Reading{
		PlaceID: clip(placeID, maxPlaceID), PartType: clip(partType, maxPartType), IsInside: inside,
		ValueUM: decimal(v["value"], maxValueUM), MeasuredAt: parseTime(v["timestamp"]),
	}
	if r.ValueUM == nil && r.MeasuredAt == nil {
		return Reading{}, false
	}
	if n, ok := parseInt(v["interpretation"]); ok && n >= -1 && n <= 5 {
		i := int16(n)
		r.Interpretation = &i
	}
	if n, ok := parseInt(v["position"]); ok && n >= math.MinInt32 && n <= math.MaxInt32 {
		p := int32(n)
		r.Position = &p
	}
	if s := clip(firstText(v, substrateKey), maxSubstrate); s != "" {
		r.SubstrateType = &s
	}
	return r, true
}

func parseTires(rows []any) []Tire {
	var out []Tire
	for _, rv := range rows {
		row, ok := rv.(obj)
		if !ok {
			continue
		}
		t := Tire{
			Section: optText(row, "section", maxTireText), Width: optText(row, "width", maxTireSize),
			Profile: optText(row, "profile", maxTireSize), Diameter: optText(row, "diameter", maxTireSize),
			Maker: optText(row, "maker", maxTireMaker), Season: optText(row, "season", maxTireText),
			TreadDepth1MM: decimal(first(row, "value1", "tread_depth_1_mm"), maxTreadDepth),
			TreadDepth2MM: decimal(first(row, "value2", "tread_depth_2_mm"), maxTreadDepth),
		}
		if t == (Tire{}) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// serialModel is the model part of a NexPTG serial ("18416 Professional"
// -> "Professional"); empty when the serial carries none.
func serialModel(serial string) string {
	head, tail, ok := strings.Cut(serial, " ")
	if !ok || head == "" || strings.Trim(head, "0123456789") != "" {
		return ""
	}
	return strings.TrimSpace(tail)
}

func first(m obj, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

// firstText is the first non-empty string (or number) value of keys.
func firstText(m obj, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case json.Number:
			return v.String()
		}
	}
	return ""
}

func optText(m obj, key string, n int) *string {
	s := clip(firstText(m, key), n)
	if s == "" {
		return nil
	}
	return &s
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n]))
}

func number(v any) (float64, bool) {
	var s string
	switch x := v.(type) {
	case json.Number:
		s = x.String()
	case string:
		s = strings.TrimSpace(x)
	case float64:
		return x, !math.IsNaN(x) && !math.IsInf(x, 0)
	default:
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// decimal is a non-negative number rounded to 2 places as text; nil for
// "-", empty, non-numeric or out of the column's range.
func decimal(v any, limit float64) *string {
	f, ok := number(v)
	if !ok {
		return nil
	}
	f = math.Round(f*100) / 100
	if f < 0 || f > limit {
		return nil
	}
	s := strconv.FormatFloat(f, 'f', 2, 64)
	return &s
}

func parseInt(v any) (int64, bool) {
	f, ok := number(v)
	if !ok || f != math.Trunc(f) {
		return 0, false
	}
	return int64(f), true
}

func parseBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case json.Number:
		return x.String() != "0"
	case string:
		b, _ := strconv.ParseBool(strings.TrimSpace(x))
		return b
	}
	return false
}

var timeLayouts = []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"}

// parseTime reads a NexPTG Unix time (seconds, or milliseconds; 0 and -1
// mean none), an RFC 3339 time or a hub zone-less time (Europe/Istanbul).
func parseTime(v any) *time.Time {
	if v == nil {
		return nil
	}
	if f, ok := number(v); ok {
		if f <= 0 {
			return nil
		}
		if f > 1e11 { // milliseconds
			f /= 1000
		}
		t := time.Unix(int64(f), 0).UTC()
		return &t
	}
	s, ok := v.(string)
	if !ok {
		return nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		t = t.UTC()
		return &t
	}
	for _, layout := range timeLayouts {
		if t, err := time.ParseInLocation(layout, s, legacyZone); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}
