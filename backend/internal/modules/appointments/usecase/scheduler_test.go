package usecase

import (
	"testing"
	"time"
)

func TestBuildDayAvailabilityCapacityFull(t *testing.T) {
	loc := time.UTC
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, loc)
	got, err := BuildDayAvailability(DailyRule{
		Date: day, Location: loc, Capacity: 3, Occupied: 3,
		Interval: 30 * time.Minute, Duration: time.Hour,
		Windows: []WorkWindow{{Start: "09:00", End: "17:00"}},
		Now:     day,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RemainingCapacity != 0 || len(got.Slots) != 0 {
		t.Fatalf("availability = %+v, want full day with no slots", got)
	}
}

func TestBuildDayAvailabilityClosedDay(t *testing.T) {
	loc := time.UTC
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, loc)
	got, err := BuildDayAvailability(DailyRule{
		Date: day, Location: loc, Capacity: 3, Closed: true,
		Interval: 30 * time.Minute, Duration: time.Hour,
		Windows: []WorkWindow{{Start: "09:00", End: "17:00"}},
		Now:     day,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Closed || len(got.Slots) != 0 || got.RemainingCapacity != 0 {
		t.Fatalf("availability = %+v, want closed day with no slots", got)
	}
}

func TestBuildDayAvailabilityBerlinSlotUTC(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, loc)
	got, err := BuildDayAvailability(DailyRule{
		Date: day, Location: loc, Capacity: 3,
		Interval: time.Hour, Duration: time.Hour,
		Windows: []WorkWindow{{Start: "09:00", End: "10:00"}},
		Now:     day,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Slots) != 1 {
		t.Fatalf("slots = %d, want 1", len(got.Slots))
	}
	want := time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC)
	if !got.Slots[0].Start.Equal(want) {
		t.Fatalf("slot start = %s, want %s", got.Slots[0].Start, want)
	}
}

func TestParseWorkingHoursWeekdayKeys(t *testing.T) {
	raw := []byte(`{"monday":[{"start":"09:00","end":"12:00"}],"2":[{"start":"10:00","end":"11:00"}]}`)
	windows, err := ParseWorkingHours(raw, time.Monday)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 1 || windows[0].Start != "09:00" {
		t.Fatalf("monday windows = %+v", windows)
	}
	windows, err = ParseWorkingHours(raw, time.Tuesday)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 1 || windows[0].Start != "10:00" {
		t.Fatalf("tuesday windows = %+v", windows)
	}
}

func TestValidateWorkingHours(t *testing.T) {
	if err := ValidateWorkingHours([]byte(`{"monday":[{"start":"09:00","end":"17:00"}],"sun":[],"6":[{"start":"10:00","end":"12:00"}]}`)); err != nil {
		t.Fatalf("valid hours rejected: %v", err)
	}
	for _, raw := range []string{
		`{"funday":[{"start":"09:00","end":"17:00"}]}`,
		`{"monday":[{"start":"17:00","end":"09:00"}]}`,
		`{"monday":[{"start":"9","end":"17:00"}]}`,
		`{"monday":"09-17"}`,
	} {
		if err := ValidateWorkingHours([]byte(raw)); err == nil {
			t.Fatalf("invalid hours accepted: %s", raw)
		}
	}
}

func TestBuildDayAvailabilityAcrossDSTChange(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	// 2026-10-25: CEST -> CET; 09:00 local is 08:00 UTC (not 07:00).
	day := time.Date(2026, 10, 25, 0, 0, 0, 0, loc)
	got, err := BuildDayAvailability(DailyRule{
		Date: day, Location: loc, Capacity: 1, Interval: time.Hour, Duration: time.Hour,
		Windows: []WorkWindow{{Start: "09:00", End: "10:00"}}, Now: day,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Slots) != 1 || !got.Slots[0].Start.Equal(time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("slots = %+v", got.Slots)
	}
	start, end := DayBounds(day, loc)
	if end.Sub(start) != 25*time.Hour {
		t.Fatalf("DST day length = %s, want 25h", end.Sub(start))
	}
}

func TestCalendarDayKeepsDateWestOfUTC(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := time.Parse(time.DateOnly, "2026-10-05")
	if got := calendarDay(parsed, loc).Format(time.DateOnly); got != "2026-10-05" {
		t.Fatalf("calendarDay = %s, want 2026-10-05", got)
	}
}
