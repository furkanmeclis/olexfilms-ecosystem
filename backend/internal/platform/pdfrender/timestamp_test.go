package pdfrender

import (
	"testing"
	"time"
)

func TestZone(t *testing.T) {
	for _, tc := range []struct {
		names []string
		want  string
	}{
		{nil, "Europe/Istanbul"},
		{[]string{""}, "Europe/Istanbul"},
		{[]string{"Not/AZone", "Local", " Europe/Berlin"}, "Europe/Istanbul"},
		{[]string{"", "America/New_York"}, "America/New_York"},
		{[]string{"Asia/Baku", "America/New_York"}, "Asia/Baku"},
	} {
		if got := Zone(tc.names...).String(); got != tc.want {
			t.Errorf("Zone(%q) = %s, want %s", tc.names, got, tc.want)
		}
	}
}

func TestIssuedAt(t *testing.T) {
	at := time.Date(2026, 10, 9, 11, 32, 0, 0, time.UTC)
	if got := IssuedAt(at, Zone("Europe/Istanbul")); got != "2026-10-09 14:32 (Europe/Istanbul)" {
		t.Fatalf("istanbul: %q", got)
	}
	if got := IssuedAt(at, nil); got != "2026-10-09 14:32 (Europe/Istanbul)" {
		t.Fatalf("nil zone: %q", got)
	}
	// 23:30 UTC is already the next day in Baku (UTC+4).
	if got := IssuedAt(time.Date(2026, 10, 9, 23, 30, 0, 0, time.UTC), Zone("Asia/Baku")); got != "2026-10-10 03:30 (Asia/Baku)" {
		t.Fatalf("baku: %q", got)
	}
}
