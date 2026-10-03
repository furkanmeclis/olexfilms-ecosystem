package usecase

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCoordinatePatch(t *testing.T) {
	decode := func(body string) LetterheadPatch {
		t.Helper()
		var p LetterheadPatch
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatalf("decode %s: %v", body, err)
		}
		return p
	}
	cases := []struct {
		body    string
		set     bool
		invalid bool
	}{
		{`{}`, false, false},
		{`{"latitude": 41.0082, "longitude": 28.9784}`, true, false},
		{`{"latitude": null, "longitude": null}`, true, false},
		{`{"latitude": 41.0}`, false, true},
		{`{"longitude": 28.0}`, false, true},
		{`{"latitude": 41.0, "longitude": null}`, false, true},
		{`{"latitude": 90.1, "longitude": 28.0}`, false, true},
		{`{"latitude": 41.0, "longitude": -180.5}`, false, true},
	}
	for _, c := range cases {
		p := decode(c.body)
		set, lat, lng, err := coordinatePatch(p.Latitude, p.Longitude)
		if c.invalid {
			if !errors.Is(err, ErrInvalidCoordinates) || !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("%s: err %v, want ErrInvalidCoordinates", c.body, err)
			}
			continue
		}
		if err != nil || set != c.set {
			t.Fatalf("%s: set=%v err=%v", c.body, set, err)
		}
		if c.body == `{"latitude": 41.0082, "longitude": 28.9784}` {
			if v := numericFloat(lat); v == nil || *v != 41.0082 {
				t.Fatalf("lat numeric: %v", v)
			}
			if v := numericFloat(lng); v == nil || *v != 28.9784 {
				t.Fatalf("lng numeric: %v", v)
			}
		}
		if c.body == `{"latitude": null, "longitude": null}` && (lat.Valid || lng.Valid) {
			t.Fatalf("clear must be NULL")
		}
	}
}
