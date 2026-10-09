package usecase

import (
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

const dateLayout = "2006-01-02"

// Periods are the preset ranges ending today (legacy 7d/30d/90d/12m);
// "custom" is reported when range_from / range_to are given.
var Periods = []string{"7d", "30d", "90d", "12m"}

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
	locale      i18n.Locale
	tz          *time.Location
}

func (s *Service) resolveQuery(d Definition, in Input) (query, error) {
	q := query{locale: in.Fallback.Locale, limit: DefaultLimit}
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
		case "7d":
			q.fromDate = today.AddDate(0, 0, -6)
		case "30d":
			q.fromDate = today.AddDate(0, 0, -29)
		case "90d":
			q.fromDate = today.AddDate(0, 0, -89)
		case "12m":
			q.fromDate = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)
		default:
			return query{}, invalid("period", "invalid", "period must be one of 7d, 30d, 90d, 12m")
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
