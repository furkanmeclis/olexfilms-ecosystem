package usecase

import (
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

const dateLayout = "2006-01-02"

// Periods are the preset ranges ending today (legacy 7d/30d/90d/12m;
// TEC-1072 adds today for the center home); "custom" is reported when
// range_from / range_to are given.
var Periods = []string{"today", "7d", "30d", "90d", "12m"}

// DefaultPeriod is the period of a request without one.
const DefaultPeriod = "30d"

// PeriodCustom marks an explicit range.
const PeriodCustom = "custom"

// Granularities of time series.
var Granularities = []string{"day", "week", "month"}

const (
	// MaxRangeDays caps an explicit range.
	MaxRangeDays = 731
	// MaxDayBuckets caps day granularity.
	MaxDayBuckets = 366
	// DefaultLimit / MaxLimit of rankings and tables.
	DefaultLimit = 10
	MaxLimit     = 50
)

// Input is the raw query of a report request.
type Input struct {
	Period      string
	RangeFrom   string
	RangeTo     string
	Granularity string
	Limit       string
	// AllTime is all_time=true|false (orders.inactive_buyers only):
	// true ignores the period and lists dealers that never ordered.
	AllTime string
	// Compare is none | previous | year (TEC-1210; empty = none).
	Compare string
	// Locale is the ?locale= override (empty: Fallback).
	Locale string
	// Fallback is the request locale and timezone (K10 chain).
	Fallback i18n.Resolved
}

// query is the resolved request: an inclusive local date range, its
// half-open UTC instants and the bucket size.
type query struct {
	period      string
	fromDate    time.Time
	toDate      time.Time
	from        time.Time
	to          time.Time
	granularity string
	limit       int32
	allTime     bool
	locale      i18n.Locale
	tz          *time.Location
	// compare is none | previous | year; prevFromDate / prevToDate are the
	// compared range (zero with none).
	compare      string
	prevFromDate time.Time
	prevToDate   time.Time
	// autoGranularity is the bucket size of the KPI sparks (the series
	// granularity of a granular report, the default rule otherwise).
	autoGranularity string
	// previous marks the query of the compared range (no sparks).
	previous bool
}

func (s *Service) resolveQuery(d Definition, in Input) (query, error) {
	q := query{locale: in.Fallback.Locale, limit: DefaultLimit, compare: CompareNone}
	if q.locale == "" {
		q.locale = i18n.DefaultLocale
	}
	if raw := strings.TrimSpace(in.Locale); raw != "" {
		l, ok := i18n.Parse(raw)
		if !ok {
			return query{}, invalid("locale", "invalid", "unsupported locale")
		}
		q.locale = l
	}
	tzName := in.Fallback.Timezone
	if tzName == "" {
		tzName = i18n.DefaultTimezone
	}
	tz, err := time.LoadLocation(tzName)
	if err != nil {
		tz = time.UTC
	}
	q.tz = tz

	if raw := strings.TrimSpace(in.Limit); raw != "" {
		if !d.Limited {
			return query{}, invalid("limit", "unsupported", "this report has no limit")
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > MaxLimit {
			return query{}, invalid("limit", "invalid", "limit must be between 1 and 50")
		}
		q.limit = int32(n)
	}

	compare := strings.TrimSpace(in.Compare)
	switch compare {
	case "", CompareNone, ComparePrevious, CompareYear:
	default:
		return query{}, invalid("compare", "invalid", "compare must be none, previous or year")
	}
	period := strings.TrimSpace(in.Period)
	rawFrom, rawTo := strings.TrimSpace(in.RangeFrom), strings.TrimSpace(in.RangeTo)
	gran := strings.TrimSpace(in.Granularity)
	if !d.Periodic {
		switch {
		case period != "":
			return query{}, invalid("period", "unsupported", "this report has no period")
		case rawFrom != "" || rawTo != "":
			return query{}, invalid("range_from", "unsupported", "this report has no period")
		}
	}
	if !d.Granular && gran != "" {
		return query{}, invalid("granularity", "unsupported", "this report has no granularity")
	}
	switch raw := strings.TrimSpace(in.AllTime); {
	case raw == "":
	case !d.AllTime:
		return query{}, invalid("all_time", "unsupported", "this report has no all_time")
	case raw == "true":
		if period != "" || rawFrom != "" || rawTo != "" {
			return query{}, invalid("all_time", "conflict", "all_time and period/range_from/range_to are exclusive")
		}
		q.allTime = true
		return q, nil
	case raw == "false":
	default:
		return query{}, invalid("all_time", "invalid", "all_time must be true or false")
	}
	if !d.Periodic {
		return q, nil
	}

	today := dateOf(s.now().In(tz))
	switch {
	case rawFrom != "" || rawTo != "":
		if period != "" && period != PeriodCustom {
			return query{}, invalid("period", "conflict", "period and range_from/range_to are exclusive")
		}
		if rawFrom == "" || rawTo == "" {
			return query{}, invalid("range_from", "required", "range_from and range_to go together")
		}
		from, err := time.Parse(dateLayout, rawFrom)
		if err != nil {
			return query{}, invalid("range_from", "invalid", "expected YYYY-MM-DD")
		}
		to, err := time.Parse(dateLayout, rawTo)
		if err != nil {
			return query{}, invalid("range_to", "invalid", "expected YYYY-MM-DD")
		}
		if to.Before(from) {
			return query{}, invalid("range_to", "invalid", "range_to must not be before range_from")
		}
		if days(from, to) > MaxRangeDays {
			return query{}, invalid("range_to", "invalid", "the range is limited to 731 days")
		}
		q.period, q.fromDate, q.toDate = PeriodCustom, from, to
	default:
		if period == "" {
			period = DefaultPeriod
		}
		switch period {
		case "today":
			q.fromDate = today
		case "7d":
			q.fromDate = today.AddDate(0, 0, -6)
		case "30d":
			q.fromDate = today.AddDate(0, 0, -29)
		case "90d":
			q.fromDate = today.AddDate(0, 0, -89)
		case "12m":
			q.fromDate = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)
		default:
			return query{}, invalid("period", "invalid", "period must be one of today, 7d, 30d, 90d, 12m")
		}
		q.period, q.toDate = period, today
	}
	span := days(q.fromDate, q.toDate)
	if gran == "" {
		switch {
		case q.period == "12m" || span > 120:
			gran = "month"
		case q.period == "90d" || span > 31:
			gran = "week"
		default:
			gran = "day"
		}
	}
	switch gran {
	case "day":
		if span > MaxDayBuckets {
			return query{}, invalid("granularity", "invalid", "day granularity is limited to 366 days")
		}
	case "week", "month":
	default:
		return query{}, invalid("granularity", "invalid", "granularity must be day, week or month")
	}
	if d.Granular {
		q.granularity = gran
	}
	q.autoGranularity = gran
	if compare != "" && compare != CompareNone {
		q.compare = compare
		q.prevFromDate, q.prevToDate = previousRange(q.period, q.fromDate, q.toDate, compare)
		// dealers.performance reads one month (the month of range_to): its
		// previous is the month before even when the day range stays
		// inside one month.
		if d.Key == ReportDealersPerformance && compare == ComparePrevious &&
			q.prevToDate.Year() == q.toDate.Year() && q.prevToDate.Month() == q.toDate.Month() {
			q.prevToDate = time.Date(q.toDate.Year(), q.toDate.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
			q.prevFromDate = time.Date(q.prevToDate.Year(), q.prevToDate.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
	}
	q.from = time.Date(q.fromDate.Year(), q.fromDate.Month(), q.fromDate.Day(), 0, 0, 0, 0, tz)
	q.to = time.Date(q.toDate.Year(), q.toDate.Month(), q.toDate.Day(), 0, 0, 0, 0, tz).AddDate(0, 0, 1)
	return q, nil
}

// dateOf is the calendar date of t as a UTC midnight.
func dateOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// days is the inclusive day count of [from, to].
func days(from, to time.Time) int {
	return int(to.Sub(from).Hours()/24) + 1
}

// bucketStart truncates a date like Postgres date_trunc (week = Monday).
func bucketStart(d time.Time, gran string) time.Time {
	switch gran {
	case "week":
		wd := (int(d.Weekday()) + 6) % 7
		return d.AddDate(0, 0, -wd)
	case "month":
		return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return d
	}
}

// buckets lists every bucket start of the range, gaps included.
func (q query) buckets() []string {
	out := []string{}
	for b := bucketStart(q.fromDate, q.granularity); !b.After(q.toDate); {
		out = append(out, b.Format(dateLayout))
		switch q.granularity {
		case "week":
			b = b.AddDate(0, 0, 7)
		case "month":
			b = b.AddDate(0, 1, 0)
		default:
			b = b.AddDate(0, 0, 1)
		}
	}
	return out
}

// Compare values (TEC-1210).
const (
	CompareNone     = "none"
	ComparePrevious = "previous"
	CompareYear     = "year"
)

// Compares lists the compare values of a periodic report.
var Compares = []string{CompareNone, ComparePrevious, CompareYear}

// previousRange is the compared range: previous = the same number of days
// right before (12m: the 12 months before), year = the same dates a year
// earlier.
func previousRange(period string, from, to time.Time, compare string) (time.Time, time.Time) {
	if compare == CompareYear || period == "12m" {
		return from.AddDate(-1, 0, 0), to.AddDate(-1, 0, 0)
	}
	n := days(from, to)
	prevTo := from.AddDate(0, 0, -1)
	return prevTo.AddDate(0, 0, -(n - 1)), prevTo
}

// previousQuery is the query of the compared range: same granularity,
// limit and locale, no compare of its own.
func (q query) previousQuery(_ Definition) query {
	p := q
	p.compare, p.previous = CompareNone, true
	p.prevFromDate, p.prevToDate = time.Time{}, time.Time{}
	p.fromDate, p.toDate = q.prevFromDate, q.prevToDate
	p.from = time.Date(p.fromDate.Year(), p.fromDate.Month(), p.fromDate.Day(), 0, 0, 0, 0, q.tz)
	p.to = time.Date(p.toDate.Year(), p.toDate.Month(), p.toDate.Day(), 0, 0, 0, 0, q.tz).AddDate(0, 0, 1)
	return p
}

// periodInfo is the envelope's period_info of a periodic query.
func (q query) periodInfo() *PeriodInfo {
	gran := q.granularity
	if gran == "" {
		gran = q.autoGranularity
	}
	pi := &PeriodInfo{
		From: q.fromDate.Format(dateLayout), To: q.toDate.Format(dateLayout),
		Granularity: gran, Compare: q.compare,
	}
	if q.compare != CompareNone {
		pf, pt := q.prevFromDate.Format(dateLayout), q.prevToDate.Format(dateLayout)
		pi.PreviousFrom, pi.PreviousTo = &pf, &pt
	}
	return pi
}

// sparkBuckets lists the bucket starts of the KPI sparks.
func (q query) sparkBuckets() []string {
	g := q
	g.granularity = q.autoGranularity
	return g.buckets()
}
