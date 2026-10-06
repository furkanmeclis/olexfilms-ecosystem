package svg

import (
	"errors"
	"strconv"
)

// PartMapPoint is a measurement point of an element part on the canvas.
type PartMapPoint struct {
	Count *int    `json:"count"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

// PartMapAsset is one part of a body type with its raw (unfilled) SVG; SVG
// is nil when the body type ships no file for the part.
type PartMapAsset struct {
	Part   string         `json:"part"`
	Place  string         `json:"place"`
	Kind   string         `json:"kind"`
	Points []PartMapPoint `json:"points"`
	SVG    *string        `json:"svg"`
}

// PartMapView is the raw NexPTG part map of a body type
// (GET /v1/measurement-part-maps/{body_type}) so a client can render and
// color the map itself.
type PartMapView struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Bodywork    string         `json:"bodywork"`
	PointRadius float64        `json:"point_radius"`
	Places      []string       `json:"places"`
	Assets      []PartMapAsset `json:"assets"`
}

// PartMap resolves a body type (ResolveID) and returns its manifest assets
// in manifest order with the raw SVG contents. The point radius follows
// ComposePlaceView: 22 when the manifest has none, 0 when unparsable.
func PartMap(bodyType string) (*PartMapView, error) {
	id, ok := ResolveID(bodyType)
	if !ok {
		return nil, ErrBodyTypeUnknown
	}
	d, err := BodyTypeDetail(id)
	if err != nil {
		return nil, err
	}
	radius := 22.0
	if d.PointRadius != "" {
		if r, err := strconv.ParseFloat(string(d.PointRadius), 64); err == nil {
			radius = r
		} else {
			radius = 0
		}
	}
	out := &PartMapView{
		ID: d.ID, Name: d.Name, Bodywork: d.Bodywork, PointRadius: radius,
		Places: append([]string(nil), Places...), Assets: make([]PartMapAsset, 0, len(d.Assets)),
	}
	for _, a := range d.Assets {
		points := make([]PartMapPoint, 0, len(a.Points))
		for _, p := range a.Points {
			x, _ := p.X.Float64()
			y, _ := p.Y.Float64()
			points = append(points, PartMapPoint{Count: p.Count, X: x, Y: y})
		}
		asset := PartMapAsset{Part: a.Part, Place: a.Place, Kind: a.Kind, Points: points}
		content, err := svgContents(d.ID, a.Part)
		switch {
		case err == nil:
			asset.SVG = &content
		case !errors.Is(err, ErrPartUnknown):
			return nil, err
		}
		out.Assets = append(out.Assets, asset)
	}
	return out, nil
}
