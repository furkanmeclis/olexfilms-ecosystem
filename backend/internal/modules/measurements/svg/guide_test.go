package svg

import (
	"strings"
	"testing"
)

func TestGuideOrderAndNumbering(t *testing.T) {
	one, two := 1, 2
	g := Guide([]PartMapAsset{
		{Part: "LEFT_SIDE", Place: "left", Kind: "main"},
		{Part: "ROOF", Place: "top", Kind: "element", Points: []PartMapPoint{{Count: &one}}},
		{Part: "LEFT_FRONT_DOOR", Place: "left", Kind: "element", Points: []PartMapPoint{{Count: &two, X: 2}, {Count: nil}, {Count: &one, X: 1}}},
		{Part: "HOOD", Place: "top", Kind: "element", Points: []PartMapPoint{{Count: &one}}},
	})
	var got []string
	for _, p := range g {
		got = append(got, p.Part+"#"+string(rune('0'+p.Position)))
	}
	if strings.Join(got, ",") != "HOOD#1,LEFT_FRONT_DOOR#1,LEFT_FRONT_DOOR#2,ROOF#1" {
		t.Fatalf("guide = %v", got)
	}
	if g[1].X != 1 || g[1].Place != "left" {
		t.Fatalf("point = %+v", g[1])
	}
}

func TestBodyTypesShipped(t *testing.T) {
	list, err := BodyTypes()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 13 || list[0].Code != "SEDAN" || list[0].Key != "sedan" {
		t.Fatalf("body types = %d, first %+v", len(list), list[0].Code)
	}
	for _, b := range list {
		if b.Points == 0 || !strings.HasPrefix(b.Thumbnail, "data:image/png;base64,") {
			t.Fatalf("%s: points %d thumbnail %.30s", b.Code, b.Points, b.Thumbnail)
		}
	}
	view, err := PartMap("SEDAN")
	if err != nil || len(view.Guide) != 71 || view.Guide[0].Part != "HOOD" {
		t.Fatalf("sedan guide = %d %v", len(view.Guide), err)
	}
}
