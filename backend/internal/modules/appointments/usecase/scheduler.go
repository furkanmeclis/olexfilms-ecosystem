package usecase

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type WorkWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type DailyRule struct {
	Date     time.Time
	Location *time.Location
	Capacity int32
	Interval time.Duration
	Duration time.Duration
	Windows  []WorkWindow
	Closed   bool
	Occupied int64
	Now      time.Time
}

type Slot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type DayAvailability struct {
	Date              string `json:"date"`
	Capacity          int32  `json:"capacity"`
	Occupied          int64  `json:"occupied"`
	RemainingCapacity int32  `json:"remaining_capacity"`
	Closed            bool   `json:"closed"`
	Slots             []Slot `json:"slots"`
	// Timezone is the organization's IANA zone the day is bounded in
	// (TEC-326: the panel calendar renders in it).
	Timezone string `json:"timezone,omitempty"`
}

func ParseWorkingHours(raw []byte, weekday time.Weekday) ([]WorkWindow, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var byDay map[string][]WorkWindow
	if err := json.Unmarshal(raw, &byDay); err != nil {
		return nil, fmt.Errorf("working_hours: %w", err)
	}
	keys := weekdayKeys(weekday)
	for _, key := range keys {
		if windows, ok := byDay[key]; ok {
			return windows, nil
		}
	}
	return nil, nil
}

// ValidateWorkingHours checks a working_hours object: every key is a weekday
// (monday/mon/1 ... sunday/sun/7) and every window is a valid HH:MM range.
func ValidateWorkingHours(raw []byte) error {
	var byDay map[string][]WorkWindow
	if err := json.Unmarshal(raw, &byDay); err != nil {
		return fmt.Errorf("must map weekdays to [{start, end}] windows")
	}
	valid := map[string]bool{}
	for w := time.Sunday; w <= time.Saturday; w++ {
		for _, k := range weekdayKeys(w) {
			valid[k] = true
		}
	}
	ref := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	for key, windows := range byDay {
		if !valid[key] {
			return fmt.Errorf("unknown weekday %q", key)
		}
		for _, w := range windows {
			if _, _, err := windowBounds(ref, time.UTC, w); err != nil {
				return err
			}
		}
	}
	return nil
}

func BuildDayAvailability(rule DailyRule) (DayAvailability, error) {
	if rule.Location == nil {
		rule.Location = time.UTC
	}
	if rule.Interval <= 0 {
		rule.Interval = 30 * time.Minute
	}
	if rule.Duration <= 0 {
		rule.Duration = rule.Interval
	}
	day := dateOnly(rule.Date, rule.Location)
	out := DayAvailability{
		Date:     day.Format(time.DateOnly),
		Capacity: rule.Capacity,
		Occupied: rule.Occupied,
		Closed:   rule.Closed,
	}
	if remaining := int64(rule.Capacity) - rule.Occupied; remaining > 0 && !rule.Closed {
		out.RemainingCapacity = int32(remaining)
	}
	if rule.Closed || out.RemainingCapacity <= 0 || rule.Capacity <= 0 {
		return out, nil
	}
	now := rule.Now
	if now.IsZero() {
		now = time.Now()
	}
	for _, window := range rule.Windows {
		start, end, err := windowBounds(day, rule.Location, window)
		if err != nil {
			return DayAvailability{}, err
		}
		for at := start; !at.Add(rule.Duration).After(end); at = at.Add(rule.Interval) {
			if at.Before(now) {
				continue
			}
			out.Slots = append(out.Slots, Slot{Start: at.UTC(), End: at.Add(rule.Duration).UTC()})
		}
	}
	sort.Slice(out.Slots, func(i, j int) bool { return out.Slots[i].Start.Before(out.Slots[j].Start) })
	return out, nil
}

func DayBounds(date time.Time, loc *time.Location) (time.Time, time.Time) {
	start := dateOnly(date, loc)
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}

func dateOnly(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func windowBounds(day time.Time, loc *time.Location, w WorkWindow) (time.Time, time.Time, error) {
	sh, sm, err := parseClock(w.Start)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	eh, em, err := parseClock(w.End)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), sh, sm, 0, 0, loc)
	end := time.Date(day.Year(), day.Month(), day.Day(), eh, em, 0, 0, loc)
	if !end.After(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("working_hours: end must be after start")
	}
	return start, end, nil
}

func parseClock(v string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("working_hours: invalid clock %q", v)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("working_hours: invalid hour %q", v)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("working_hours: invalid minute %q", v)
	}
	return h, m, nil
}

func weekdayKeys(w time.Weekday) []string {
	full := []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}[w]
	short := full[:3]
	iso := int(w)
	if iso == 0 {
		iso = 7
	}
	return []string{full, short, strconv.Itoa(iso)}
}
