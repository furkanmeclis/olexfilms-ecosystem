package usecase

import (
	"errors"
	"testing"
	"time"
)

// TEC-769: end-of-day cutoff. Without a cutoff the business day is the
// calendar day; with 18:00 day D covers [D-1 18:00, D 18:00) in the
// organization's zone and closes at D 18:00.

func ptr(m int) *int { return &m }

func TestParseCutoff(t *testing.T) {
	for in, want := range map[string]*int{"": nil, "  ": nil, "00:00": nil, "18:00": ptr(1080), "00:01": ptr(1), "23:59": ptr(1439), " 07:30 ": ptr(450)} {
		got, err := ParseCutoff(in)
		if err != nil || (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("ParseCutoff(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"24:00", "18", "18:60", "6:00", "18:00:00", "abc", "-1:00"} {
		_, err := ParseCutoff(in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != "eod_cutoff" {
			t.Errorf("ParseCutoff(%q) = %v, want an eod_cutoff validation error", in, err)
		}
	}
	if s := formatCutoff(ptr(1080)); s == nil || *s != "18:00" {
		t.Fatalf("formatCutoff = %v", s)
	}
	if s := formatCutoff(ptr(5)); s == nil || *s != "00:05" {
		t.Fatalf("formatCutoff = %v", s)
	}
	if formatCutoff(nil) != nil {
		t.Fatal("nil cutoff formats to nil")
	}
}

func TestEODPeriodWithCutoff(t *testing.T) {
	ist, _ := zoneOf("Europe/Istanbul")
	day := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)

	// No cutoff (nil or midnight): the calendar day.
	for _, c := range []*int{nil, ptr(0)} {
		start, end := EODPeriod(day, ist, c)
		ds, de := EODDay(day, ist)
		if !start.Equal(ds) || !end.Equal(de) {
			t.Fatalf("cutoff %v = %s..%s", c, start, end)
		}
	}
	// 18:00 Istanbul (UTC+3): [Oct 1 15:00 UTC, Oct 2 15:00 UTC).
	start, end := EODPeriod(day, ist, ptr(1080))
	if !start.UTC().Equal(time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)) ||
		!end.UTC().Equal(time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("18:00 = %s .. %s", start.UTC(), end.UTC())
	}
	// Month boundary: day 1 starts on the last day of the previous month.
	start, _ = EODPeriod(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), ist, ptr(1080))
	if !start.UTC().Equal(time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("month boundary start = %s", start.UTC())
	}

	// DST: Berlin leaves summer time on 2026-10-25 (03:00 -> 02:00), the
	// business day around it is 25 hours long; it enters on 2026-03-29
	// (23 hours).
	ber, tz := zoneOf("Europe/Berlin")
	if tz != "Europe/Berlin" {
		t.Skip("tzdata without Europe/Berlin")
	}
	start, end = EODPeriod(time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), ber, ptr(1080))
	if end.Sub(start) != 25*time.Hour {
		t.Fatalf("autumn DST day = %s", end.Sub(start))
	}
	start, end = EODPeriod(time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC), ber, ptr(1080))
	if end.Sub(start) != 23*time.Hour {
		t.Fatalf("spring DST day = %s", end.Sub(start))
	}
}

func TestEODOpenAndClosedDay(t *testing.T) {
	ist, _ := zoneOf("Europe/Istanbul")
	date := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	cases := []struct {
		name   string
		now    time.Time
		cutoff *int
		open   time.Time
		closed time.Time
	}{
		// No cutoff: the open day is the local calendar day, the closed
		// one yesterday. 22:30 UTC is already Oct 3 in Istanbul.
		{"none", time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC), nil, date(2026, 10, 2), date(2026, 10, 1)},
		{"none local midnight", time.Date(2026, 10, 2, 22, 30, 0, 0, time.UTC), nil, date(2026, 10, 3), date(2026, 10, 2)},
		// 18:00 Istanbul = 15:00 UTC.
		{"before cutoff", time.Date(2026, 10, 2, 14, 59, 59, 0, time.UTC), ptr(1080), date(2026, 10, 2), date(2026, 10, 1)},
		{"at cutoff", time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC), ptr(1080), date(2026, 10, 3), date(2026, 10, 2)},
		{"after cutoff", time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC), ptr(1080), date(2026, 10, 3), date(2026, 10, 2)},
		// After local midnight but before the cutoff: yesterday's day is
		// closed (a missed run still writes it).
		{"after midnight", time.Date(2026, 10, 2, 22, 30, 0, 0, time.UTC), ptr(1080), date(2026, 10, 3), date(2026, 10, 2)},
		// Year boundary.
		{"new year", time.Date(2026, 12, 31, 16, 0, 0, 0, time.UTC), ptr(1080), date(2027, 1, 1), date(2026, 12, 31)},
	}
	for _, c := range cases {
		if got := EODOpenDay(c.now, ist, c.cutoff); !got.Equal(c.open) {
			t.Errorf("%s: open = %s, want %s", c.name, got.Format(eodDateLayout), c.open.Format(eodDateLayout))
		}
		if got := EODClosedDay(c.now, ist, c.cutoff); !got.Equal(c.closed) {
			t.Errorf("%s: closed = %s, want %s", c.name, got.Format(eodDateLayout), c.closed.Format(eodDateLayout))
		}
		// The open day's period contains now.
		start, end := EODPeriod(EODOpenDay(c.now, ist, c.cutoff), ist, c.cutoff)
		if c.now.Before(start) || !c.now.Before(end) {
			t.Errorf("%s: now %s outside the open period %s..%s", c.name, c.now, start, end)
		}
	}
}
