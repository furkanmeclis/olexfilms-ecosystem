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
	if !got.Closed || len(got.Slots) != 0 {
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
