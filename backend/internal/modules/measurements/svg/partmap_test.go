package svg

import (
	"errors"
	"strings"
	"testing"
)

func TestPartMapResolvesAndCarriesRawAssets(t *testing.T) {
	byID, err := PartMap("MUvi2UUvtDeNoPrWss2qpL")
	if err != nil {
		t.Fatal(err)
	}
	byName, err := PartMap("SEDAN")
	if err != nil {
		t.Fatal(err)
	}
	if byID.ID != "MUvi2UUvtDeNoPrWss2qpL" || byName.ID != byID.ID {
		t.Fatalf("ids = %q / %q", byID.ID, byName.ID)
	}
	if byID.Name != "Sedan" || byID.Bodywork != "SEDAN" || byID.PointRadius != 22 {
		t.Fatalf("header = %+v", byID)
	}
	if strings.Join(byID.Places, ",") != "left,right,top,back" {
		t.Fatalf("places = %v", byID.Places)
	}
	d, err := BodyTypeDetail(byID.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(byID.Assets) != len(d.Assets) || byID.Assets[0].Part != d.Assets[0].Part {
		t.Fatalf("assets not in manifest order")
	}
	var hood *PartMapAsset
	for i := range byID.Assets {
		if byID.Assets[i].Part == "HOOD" {
			hood = &byID.Assets[i]
		}
	}
	if hood == nil {
		t.Fatal("no HOOD asset")
	}
	if hood.SVG == nil || !strings.Contains(*hood.SVG, "<svg") {
		t.Fatal("HOOD svg missing")
	}
	if len(hood.Points) == 0 || hood.Points[0].X == 0 || hood.Points[0].Y == 0 {
		t.Fatalf("HOOD points = %+v", hood.Points)
	}
	if hood.Kind != "element" {
		t.Fatalf("HOOD kind = %q", hood.Kind)
	}
}

func TestPartMapUnknownBodyType(t *testing.T) {
	for _, bt := range []string{"", "NOPE"} {
		if _, err := PartMap(bt); !errors.Is(err, ErrBodyTypeUnknown) {
			t.Fatalf("%q err = %v", bt, err)
		}
	}
}
