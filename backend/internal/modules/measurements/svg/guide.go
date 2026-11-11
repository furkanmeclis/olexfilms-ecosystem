package svg

import (
	"embed"
	"encoding/base64"
	"errors"
	"io/fs"
	"slices"
	"sort"
	"strings"
)

// thumbnails holds the left side view of every body type as a 360 px PNG
// (rsvg-convert -w 360 assets/<id>/LEFT_SIDE.svg), for the body type picker.
//
//go:embed thumbnails
var thumbnails embed.FS

// GuideOrder is the walking order of guided measuring (mobile, 11.11): hood,
// left side front to back, trunk, right side back to front, roof. Parts not
// listed follow in manifest order.
var GuideOrder = []string{
	"HOOD",
	"LEFT_FRONT_FENDER", "LEFT_FRONT_DOOR", "LEFT_PILLAR", "LEFT_REAR_DOOR", "LEFT_REAR_FENDER",
	"TRUNK",
	"RIGHT_REAR_FENDER", "RIGHT_REAR_DOOR", "RIGHT_PILLAR", "RIGHT_FRONT_DOOR", "RIGHT_FRONT_FENDER",
	"ROOF",
}

// GuidePoint is one numbered point of the guided walk.
type GuidePoint struct {
	Part     string  `json:"part"`
	Place    string  `json:"place"`
	Position int     `json:"position"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
}

// Guide lists the numbered points of a part map in GuideOrder, each part's
// points by number; points without a number are left out.
func Guide(assets []PartMapAsset) []GuidePoint {
	type el struct {
		a    PartMapAsset
		i    int
		rank int
	}
	var els []el
	for i, a := range assets {
		if a.Kind != "element" || a.Part == "" {
			continue
		}
		rank := slices.Index(GuideOrder, a.Part)
		if rank < 0 {
			rank = len(GuideOrder)
		}
		els = append(els, el{a, i, rank})
	}
	sort.SliceStable(els, func(x, y int) bool {
		if els[x].rank != els[y].rank {
			return els[x].rank < els[y].rank
		}
		return els[x].i < els[y].i
	})
	out := []GuidePoint{}
	for _, e := range els {
		pts := make([]PartMapPoint, 0, len(e.a.Points))
		for _, p := range e.a.Points {
			if p.Count != nil && *p.Count >= 1 {
				pts = append(pts, p)
			}
		}
		sort.SliceStable(pts, func(x, y int) bool { return *pts[x].Count < *pts[y].Count })
		for _, p := range pts {
			out = append(out, GuidePoint{Part: e.a.Part, Place: e.a.Place, Position: *p.Count, X: p.X, Y: p.Y})
		}
	}
	return out
}

// BodyType is one entry of the body type picker.
type BodyType struct {
	// Code is the catalog bodywork ("SEDAN"): the measurement's body_type.
	Code string `json:"code"`
	// Key is the code in lower snake case, for client translations ("hatchback_5d").
	Key string `json:"key"`
	// Name is the catalog name (NexPTG, not translated).
	Name string `json:"name"`
	// Thumbnail is the left side view as a PNG data URI.
	Thumbnail string `json:"thumbnail"`
	// Points is the number of guided points.
	Points int `json:"points"`
}

// pickerOrder is the order of the body type picker: the common bodies first.
var pickerOrder = []string{
	"SEDAN", "HATCHBACK 5D", "HATCHBACK 3D", "SUV", "ESTATE", "LIFTBACK", "COUPE", "CABRIOLET",
	"MINIVAN", "VAN", "PICKUP 2D", "PICKUP 4D", "TRUCK",
}

// BodyTypes lists the shipped body types that have a manifest, in picker
// order, with their thumbnail and guided point count.
func BodyTypes() ([]BodyType, error) {
	list, err := Models()
	if err != nil {
		return nil, err
	}
	out := make([]BodyType, 0, len(list))
	for _, m := range list {
		view, err := PartMap(m.ID)
		if err != nil {
			if errors.Is(err, ErrBodyTypeUnknown) || errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		png, err := thumbnails.ReadFile("thumbnails/" + m.ID + ".png")
		if err != nil {
			continue
		}
		out = append(out, BodyType{
			Code:      m.Bodywork,
			Key:       strings.ToLower(strings.ReplaceAll(m.Bodywork, " ", "_")),
			Name:      m.Name,
			Thumbnail: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
			Points:    len(view.Guide),
		})
	}
	rank := func(code string) int {
		if i := slices.Index(pickerOrder, code); i >= 0 {
			return i
		}
		return len(pickerOrder)
	}
	sort.SliceStable(out, func(x, y int) bool { return rank(out[x].Code) < rank(out[y].Code) })
	return out, nil
}
