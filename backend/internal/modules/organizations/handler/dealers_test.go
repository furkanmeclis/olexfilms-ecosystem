package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TEC-240: query validation answers 400 VALIDATION_ERROR before any DB or
// brand lookup.
func TestPublicNearbyDealersValidation(t *testing.T) {
	h := &Handler{}
	cases := []struct {
		query string
		field string
	}{
		{"?lng=29", "lat"},
		{"?lat=abc&lng=29", "lat"},
		{"?lat=91&lng=29", "lat"},
		{"?lat=-90.5&lng=29", "lat"},
		{"?lat=NaN&lng=29", "lat"},
		{"?lat=41&lng=181", "lng"},
		{"?lat=41&lng=29&radius_km=0", "radius_km"},
		{"?lat=41&lng=29&radius_km=1001", "radius_km"},
		{"?lat=41&lng=29&radius_km=x", "radius_km"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.PublicNearbyDealers(rec, httptest.NewRequest(http.MethodGet, "/v1/public/dealers/nearby"+c.query, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400", c.query, rec.Code)
		}
		var env struct {
			Error struct {
				Code    string `json:"code"`
				Details []struct {
					Field string `json:"field"`
				} `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if env.Error.Code != "VALIDATION_ERROR" {
			t.Fatalf("%s: code %q", c.query, env.Error.Code)
		}
		found := false
		for _, d := range env.Error.Details {
			found = found || d.Field == c.field
		}
		if !found {
			t.Fatalf("%s: details %+v miss field %s", c.query, env.Error.Details, c.field)
		}
	}
}

func TestParseNearbyQueryDefaults(t *testing.T) {
	in, details := parseNearbyQuery(httptest.NewRequest(http.MethodGet, "/x?lat=41.0082&lng=28.9784", nil))
	if len(details) != 0 {
		t.Fatalf("details: %+v", details)
	}
	if in.Lat != 41.0082 || in.Lng != 28.9784 || in.RadiusKm != 100 {
		t.Fatalf("input: %+v", in)
	}
	in, details = parseNearbyQuery(httptest.NewRequest(http.MethodGet, "/x?lat=-90&lng=180&radius_km=1000", nil))
	if len(details) != 0 || in.RadiusKm != 1000 {
		t.Fatalf("bounds: %+v %+v", in, details)
	}
}
