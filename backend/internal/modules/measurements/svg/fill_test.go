package svg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	Name         string   `json:"name"`
	BodyType     string   `json:"body_type"`
	Parts        []string `json:"parts"`
	Measurements []struct {
		PartType       string `json:"part_type"`
		Position       *int   `json:"position"`
		Interpretation *int   `json:"interpretation"`
	} `json:"measurements"`
}

func loadFixture(t *testing.T, name string) (fixture, Report) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var fx fixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	r := Report{BodyType: fx.BodyType}
	for _, m := range fx.Measurements {
		r.Readings = append(r.Readings, Reading{PartType: m.PartType, Position: m.Position, Interpretation: m.Interpretation})
	}
	return fx, r
}

func golden(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func assertSame(t *testing.T, name, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	lo := max(i-80, 0)
	t.Fatalf("%s differs from the PHP reference at byte %d (got len %d, want len %d)\n got: %q\nwant: %q",
		name, i, len(got), len(want), got[lo:min(i+80, len(got))], want[lo:min(i+80, len(want))])
}

// TestGoldenMatchesLegacyPHP: the SVGs produced from the fixture report are
// byte-identical to the legacy NexptgSvgFillService output
// (testdata/gen_golden.php).
func TestGoldenMatchesLegacyPHP(t *testing.T) {
	fx, report := loadFixture(t, "fixture_sedan.json")
	f, err := NewFiller(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, place := range Places {
		got, ok, err := f.ComposePlaceView(place)
		if err != nil || !ok {
			t.Fatalf("compose %s: ok=%v err=%v", place, ok, err)
		}
		name := fx.Name + "_composite_" + place + ".svg"
		assertSame(t, name, got, golden(t, name))
	}
	for _, part := range fx.Parts {
		got, err := f.FillPart(part)
		if err != nil {
			t.Fatalf("fill %s: %v", part, err)
		}
		name := fx.Name + "_part_" + part + ".svg"
		assertSame(t, name, got, golden(t, name))
	}
}

func TestCardsOrderAndContent(t *testing.T) {
	_, report := loadFixture(t, "fixture_sedan.json")
	cards, err := Cards(report)
	if err != nil {
		t.Fatal(err)
	}
	// Sedan: 4 composite views + 13 element parts.
	if len(cards) != 17 {
		t.Fatalf("cards = %d, want 17", len(cards))
	}
	for i, place := range Places {
		if cards[i].Type != "main" || cards[i].Place != place {
			t.Fatalf("card %d = %s/%s, want main/%s", i, cards[i].Type, cards[i].Place, place)
		}
	}
	if cards[4].Type != "part" || cards[4].Part != "LEFT_FRONT_FENDER" {
		t.Fatalf("first part card = %+v", cards[4].Part)
	}
	right := cards[1].SVG
	for _, want := range []string{`id="nexptg_part_body_RIGHT_REAR_DOOR"`, `id="RIGHT_FRONT_DOOR_point_1"`, `fill="#e9df28"`} {
		if !strings.Contains(right, want) {
			t.Errorf("right composite misses %s", want)
		}
	}
}

func TestAverageInterpretationExcludesNullAndDisabled(t *testing.T) {
	_, report := loadFixture(t, "fixture_sedan.json")
	f, err := NewFiller(report)
	if err != nil {
		t.Fatal(err)
	}
	// HOOD: 0, -1, null, 2, 4 -> avg(0, 2, 4) = 2.
	if got, ok := f.AveragePartInterpretation("HOOD"); !ok || got != SecondLayer {
		t.Fatalf("HOOD = %d %v", got, ok)
	}
	// RIGHT_REAR_DOOR: 3, 4, 5 -> 4.
	if got, ok := f.AveragePartInterpretation("RIGHT_REAR_DOOR"); !ok || got != ThickPutty {
		t.Fatalf("RIGHT_REAR_DOOR = %d %v", got, ok)
	}
	if _, ok := f.AveragePartInterpretation("RIGHT_FRONT_FENDER"); ok {
		t.Fatal("only null readings must not have an average")
	}
}

func TestResolveID(t *testing.T) {
	for in, want := range map[string]string{
		"MUvi2UUvtDeNoPrWss2qpL": "MUvi2UUvtDeNoPrWss2qpL",
		"SEDAN":                  "MUvi2UUvtDeNoPrWss2qpL",
		"sedan":                  "MUvi2UUvtDeNoPrWss2qpL",
		"Hatchback 5d":           "CexVhymW3JTPErUj51DnTE",
		"ESTATE":                 "8ay5gjdU7zaBH2McBgnFFu",
	} {
		if got, ok := ResolveID(in); !ok || got != want {
			t.Errorf("ResolveID(%q) = %q %v", in, got, ok)
		}
	}
	if _, ok := ResolveID("SPACESHIP"); ok {
		t.Error("unknown body type resolved")
	}
	cards, err := Cards(Report{BodyType: "SPACESHIP"})
	if err != nil || cards != nil {
		t.Fatalf("unknown body type: %v %v", cards, err)
	}
}

// Every shipped model loads and every manifest part has its SVG.
func TestCatalogComplete(t *testing.T) {
	list, err := Models()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 13 {
		t.Fatalf("models = %d", len(list))
	}
	for _, m := range list {
		d, err := BodyTypeDetail(m.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range d.Assets {
			if _, err := svgContents(m.ID, a.Part); err != nil {
				t.Errorf("%s: %v", m.Bodywork, err)
			}
		}
	}
}

func TestPHPFloat(t *testing.T) {
	for in, want := range map[float64]string{
		22: "22", 518.8125: "518.8125", 1133.5999755859375: "1133.5999755859", 0.5: "0.5", -3: "-3",
	} {
		if got := phpFloat(in); got != want {
			t.Errorf("phpFloat(%v) = %q, want %q", in, got, want)
		}
	}
}
