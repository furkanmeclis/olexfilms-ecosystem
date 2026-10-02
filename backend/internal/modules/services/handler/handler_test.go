package handler

import (
	"testing"
	"time"
)

func TestParseCreatedBound(t *testing.T) {
	day := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		raw     string
		end     bool
		want    *time.Time
		wantErr bool
	}{
		{raw: ""},
		{raw: "  "},
		{raw: "2026-10-02", want: &day},
		{raw: "2026-10-02", end: true, want: ptr(day.Add(24 * time.Hour))},
		{raw: "2026-10-02T03:00:00+03:00", want: &day},
		{raw: "2026-10-02T03:00:00+03:00", end: true, want: &day},
		{raw: "02.10.2026", wantErr: true},
		{raw: "2026-13-01", wantErr: true},
	}
	for _, c := range cases {
		got, err := ParseCreatedBound(c.raw, c.end)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: want error", c.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.raw, err)
			continue
		}
		if (got == nil) != (c.want == nil) || (got != nil && !got.Equal(*c.want)) {
			t.Errorf("%q end=%v: got %v, want %v", c.raw, c.end, got, c.want)
		}
	}
}

func ptr(t time.Time) *time.Time { return &t }
