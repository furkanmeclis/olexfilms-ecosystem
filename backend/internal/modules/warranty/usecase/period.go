package usecase

import "time"

// EndAt is the end of a warranty period (decision 4): start, read as a
// calendar date in loc, plus months calendar months. When the target month
// has no such day the date is clamped to the month's last day (31 January +
// 1 month = 28 or 29 February). The result is the last instant of that day
// in loc (23:59:59.999999, PostgreSQL's microsecond precision), so the
// expiry cron only compares end_at with now. A nil loc means UTC.
func EndAt(start time.Time, months int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := start.In(loc).Date()
	// Day 1 of the target month never overflows; time.Date normalizes the
	// month (13 -> January of the next year).
	first := time.Date(y, m+time.Month(months), 1, 0, 0, 0, 0, loc)
	if last := daysIn(first.Year(), first.Month()); d > last {
		d = last
	}
	nextDay := time.Date(first.Year(), first.Month(), d+1, 0, 0, 0, 0, loc)
	return nextDay.Add(-time.Microsecond)
}

// daysIn is the number of days of a month (leap years included).
func daysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// orgLocation loads an organization's IANA time zone; an empty or unknown
// zone falls back to UTC (organizations.timezone is validated on write).
func orgLocation(zone string) *time.Location {
	if zone == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.UTC
	}
	return loc
}
