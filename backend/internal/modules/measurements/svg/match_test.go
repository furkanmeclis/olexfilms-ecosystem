package svg

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the TEC-1010 golden files")

// legacyReports is testdata/legacy_reports.json: every hub NexPTG report
// with its readings as [part_type, position, interpretation, value_um,
// is_inside].
type legacyReports struct {
	Reports []struct {
		ID         int               `json:"id"`
		BodyType   string            `json:"body_type"`
		TypeOfBody string            `json:"type_of_body"`
		Readings   []json.RawMessage `json:"readings"`
	} `json:"reports"`
}

func loadLegacyReports(t *testing.T) map[int]Report {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "legacy_reports.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f legacyReports
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	out := map[int]Report{}
	for _, rep := range f.Reports {
		// The parser's order: body_type, then type_of_body.
		r := Report{BodyType: rep.BodyType}
		if r.BodyType == "" {
			r.BodyType = rep.TypeOfBody
		}
		for _, rd := range rep.Readings {
			var cols []any
			if err := json.Unmarshal(rd, &cols); err != nil {
				t.Fatal(err)
			}
			pos := int(cols[1].(float64))
			reading := Reading{PartType: cols[0].(string), Position: &pos}
			if v, ok := cols[2].(float64); ok {
				level := int(v)
				reading.Interpretation = &level
			}
			r.Readings = append(r.Readings, reading)
		}
		out[rep.ID] = r
	}
	return out
}

// TestLegacyReportsHaveNoUnmatchedReadings is the TEC-1010 "eşleşmeyen
// değer = 0" report: every reading of every legacy report lands on a
// numbered point of its body type's part map.
func TestLegacyReportsHaveNoUnmatchedReadings(t *testing.T) {
	reports := loadLegacyReports(t)
	if len(reports) != 122 {
		t.Fatalf("legacy reports = %d, want 122", len(reports))
	}
	total, unmatched := 0, 0
	for id, r := range reports {
		total += len(r.Readings)
		for _, u := range UnmatchedReadings(r) {
			unmatched++
			t.Errorf("report %d (%s): %s", id, r.BodyType, u)
		}
	}
	t.Logf("legacy readings: %d, unmatched: %d", total, unmatched)
	if total != 7082 {
		t.Errorf("legacy readings = %d, want 7082", total)
	}
}

func TestResolveIDAliases(t *testing.T) {
	for in, want := range map[string]string{
		"ESTATE (STATION WAGON)": "8ay5gjdU7zaBH2McBgnFFu",
		"estate (station wagon)": "8ay5gjdU7zaBH2McBgnFFu",
		"STATION WAGON":          "8ay5gjdU7zaBH2McBgnFFu",
		"ESTATE":                 "8ay5gjdU7zaBH2McBgnFFu",
		"SUV":                    "9fxJEsSTS3zQHeBmjyxGXC",
	} {
		if got, ok := ResolveID(in); !ok || got != want {
			t.Errorf("ResolveID(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := ResolveID("SPACESHIP"); ok {
		t.Error("unknown body type resolved")
	}
}

func TestUnmatchedReadingsReasons(t *testing.T) {
	pos := func(n int) *int { return &n }
	got := UnmatchedReadings(Report{BodyType: "COUPE", Readings: []Reading{
		{PartType: "HOOD", Position: pos(5)},
		{PartType: "HOOD", Position: pos(9)},
		{PartType: "LEFT_REAR_DOOR", Position: pos(1)},
		{PartType: "HOOD"},
	}})
	if len(got) != 2 || got[0].String() != "position:HOOD#9" || got[1].String() != "part:LEFT_REAR_DOOR#1" {
		t.Fatalf("unmatched = %v", got)
	}
	got = UnmatchedReadings(Report{BodyType: "SPACESHIP", Readings: []Reading{{PartType: "HOOD", Position: pos(1)}}})
	if len(got) != 1 || got[0].Reason != UnmatchedBodyType {
		t.Fatalf("unmatched = %v", got)
	}
}

// TestLegacyGoldenViews renders sample legacy reports (an estate, which
// resolves only through the alias, and an SUV) and compares them with the
// golden files; go test -run TestLegacyGoldenViews -update rewrites them.
func TestLegacyGoldenViews(t *testing.T) {
	reports := loadLegacyReports(t)
	for _, tc := range []struct {
		name string
		id   int
	}{{"estate", estateReportID(t, reports)}, {"suv", firstReportOf(t, reports, "SUV")}} {
		f, err := NewFiller(reports[tc.id])
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		for _, place := range Places {
			got, ok, err := f.ComposePlaceView(place)
			if err != nil || !ok {
				t.Fatalf("%s %s: ok=%v err=%v", tc.name, place, ok, err)
			}
			name := "legacy_" + tc.name + "_" + place + ".svg"
			path := filepath.Join("testdata", name)
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			assertSame(t, name, got, golden(t, name))
		}
	}
}

func estateReportID(t *testing.T, reports map[int]Report) int {
	t.Helper()
	return firstReportOf(t, reports, "ESTATE (STATION WAGON)")
}

// firstReportOf is the lowest report id of a body type.
func firstReportOf(t *testing.T, reports map[int]Report, bodyType string) int {
	t.Helper()
	best := 0
	for id, r := range reports {
		if r.BodyType == bodyType && (best == 0 || id < best) {
			best = id
		}
	}
	if best == 0 {
		t.Fatalf("no legacy %s report", bodyType)
	}
	return best
}
