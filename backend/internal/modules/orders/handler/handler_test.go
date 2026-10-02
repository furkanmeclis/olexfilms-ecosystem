package handler

import (
	"testing"
	"time"
)

func TestParseCreatedBound(t *testing.T) {
	if v, err := parseCreatedBound("  ", false); v != nil || err != nil {
		t.Fatalf("empty: %v %v", v, err)
	}
	start, err := parseCreatedBound("2026-10-02", false)
	if err != nil || !start.Equal(time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("day start: %v %v", start, err)
	}
	end, err := parseCreatedBound("2026-10-02", true)
	if err != nil || !end.Equal(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("day end is the next midnight: %v %v", end, err)
	}
	ts, err := parseCreatedBound("2026-10-01T21:00:00+03:00", true)
	if err != nil || !ts.Equal(time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)) || ts.Location() != time.UTC {
		t.Fatalf("rfc3339 kept as UTC: %v %v", ts, err)
	}
	if _, err := parseCreatedBound("yesterday", false); err == nil {
		t.Fatal("invalid date accepted")
	}
}
