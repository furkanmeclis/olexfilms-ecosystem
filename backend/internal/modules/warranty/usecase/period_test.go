package usecase

import (
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("time zone %s not available: %v", name, err)
	}
	return loc
}

// TEC-186 / decision 4: start + N calendar months, clamped to the month
// end, then the end of that day in the organization's zone.
func TestEndAt(t *testing.T) {
	ist := mustLoc(t, "Europe/Istanbul")
	ny := mustLoc(t, "America/New_York")
	tokyo := mustLoc(t, "Asia/Tokyo")
	endOf := func(y int, m time.Month, d int, loc *time.Location) time.Time {
		return time.Date(y, m, d, 23, 59, 59, 999999000, loc)
	}
	cases := []struct {
		name   string
		start  time.Time
		months int
		loc    *time.Location
		want   time.Time
	}{
		{"plain 12 months", time.Date(2026, 3, 15, 10, 0, 0, 0, ist), 12, ist, endOf(2027, 3, 15, ist)},
		{"31 Jan + 1 month, common year", time.Date(2027, 1, 31, 9, 0, 0, 0, ist), 1, ist, endOf(2027, 2, 28, ist)},
		{"31 Jan + 1 month, leap year", time.Date(2028, 1, 31, 9, 0, 0, 0, ist), 1, ist, endOf(2028, 2, 29, ist)},
		{"29 Feb + 12 months", time.Date(2028, 2, 29, 12, 0, 0, 0, ist), 12, ist, endOf(2029, 2, 28, ist)},
		{"29 Feb + 48 months", time.Date(2028, 2, 29, 12, 0, 0, 0, ist), 48, ist, endOf(2032, 2, 29, ist)},
		{"31 Mar + 1 month", time.Date(2026, 3, 31, 12, 0, 0, 0, ist), 1, ist, endOf(2026, 4, 30, ist)},
		{"31 Aug + 6 months, year rollover", time.Date(2026, 8, 31, 12, 0, 0, 0, ist), 6, ist, endOf(2027, 2, 28, ist)},
		{"30 Nov + 3 months", time.Date(2026, 11, 30, 12, 0, 0, 0, ist), 3, ist, endOf(2027, 2, 28, ist)},
		{"31 Dec + 1 month", time.Date(2026, 12, 31, 12, 0, 0, 0, ist), 1, ist, endOf(2027, 1, 31, ist)},
		{"120 months", time.Date(2026, 5, 31, 12, 0, 0, 0, ist), 120, ist, endOf(2036, 5, 31, ist)},
		// 21:30 UTC on 31 Jan is already 1 Feb in Istanbul (UTC+3): the
		// calendar date is taken in the organization's zone.
		{"UTC evening is next day in Istanbul", time.Date(2027, 1, 31, 21, 30, 0, 0, time.UTC), 1, ist, endOf(2027, 3, 1, ist)},
		{"same instant in UTC", time.Date(2027, 1, 31, 21, 30, 0, 0, time.UTC), 1, time.UTC, endOf(2027, 2, 28, time.UTC)},
		// 03:00 UTC on 1 Mar is still 28 Feb in New York.
		{"UTC morning is previous day in New York", time.Date(2027, 3, 1, 3, 0, 0, 0, time.UTC), 1, ny, endOf(2027, 3, 28, ny)},
		// Ends after the DST change (EST -> EDT on 14 Mar 2027).
		{"New York across DST", time.Date(2027, 2, 10, 12, 0, 0, 0, ny), 1, ny, endOf(2027, 3, 10, ny)},
		{"Tokyo", time.Date(2026, 10, 31, 23, 0, 0, 0, tokyo), 1, tokyo, endOf(2026, 11, 30, tokyo)},
		{"nil location is UTC", time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), 1, nil, endOf(2026, 2, 28, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EndAt(tc.start, tc.months, tc.loc)
			if !got.Equal(tc.want) {
				t.Fatalf("EndAt(%s, %d) = %s, want %s", tc.start, tc.months, got, tc.want)
			}
			if !got.After(tc.start) {
				t.Fatalf("end %s not after start %s", got, tc.start)
			}
			// The cron's date formatter shows the same last day.
			loc := tc.loc
			if loc == nil {
				loc = time.UTC
			}
			if d := EndDate(got, loc.String()); d != tc.want.Format(DateLayout) {
				t.Fatalf("EndDate = %s, want %s", d, tc.want.Format(DateLayout))
			}
		})
	}
}

func TestOrgLocation(t *testing.T) {
	if orgLocation("") != time.UTC || orgLocation("Not/AZone") != time.UTC {
		t.Fatal("empty or unknown zone must fall back to UTC")
	}
	if got := orgLocation("Europe/Istanbul"); got.String() != "Europe/Istanbul" {
		t.Fatalf("zone = %s", got)
	}
}

func TestSkipReason(t *testing.T) {
	olex := db.GetWarrantyServiceContextRow{BrandSlug: "olex"}
	glorian := db.GetWarrantyServiceContextRow{BrandSlug: "glorian"}
	months := func(n int32) pgtype.Int4 { return pgtype.Int4{Int32: n, Valid: true} }
	ok := db.ListWarrantyCandidatesByServiceRow{
		Kind: KindFull, UnitSource: "generated", UnitBrandSlug: "olex", WarrantyDurationMonths: months(24),
	}
	with := func(f func(*db.ListWarrantyCandidatesByServiceRow)) db.ListWarrantyCandidatesByServiceRow {
		r := ok
		f(&r)
		return r
	}
	cases := []struct {
		name string
		svc  db.GetWarrantyServiceContextRow
		item db.ListWarrantyCandidatesByServiceRow
		want string
	}{
		{"eligible", olex, ok, ""},
		{"eligible partial", olex, with(func(r *db.ListWarrantyCandidatesByServiceRow) { r.Kind = KindPartial }), ""},
		{"imported unit", olex, with(func(r *db.ListWarrantyCandidatesByServiceRow) { r.UnitSource = "imported" }), ""},
		{"no period", olex, with(func(r *db.ListWarrantyCandidatesByServiceRow) { r.WarrantyDurationMonths = pgtype.Int4{} }), SkipNoPeriod},
		{"zero period", olex, with(func(r *db.ListWarrantyCandidatesByServiceRow) { r.WarrantyDurationMonths = months(0) }), SkipNoPeriod},
		{"glorian service", glorian, ok, SkipGlorian},
		{"glorian unit", olex, with(func(r *db.ListWarrantyCandidatesByServiceRow) { r.UnitBrandSlug = "Glorian" }), SkipGlorian},
		{"external outbound movement", olex, with(func(r *db.ListWarrantyCandidatesByServiceRow) { r.ExternalOutbound = true }), SkipExternal},
		{"external source", olex, with(func(r *db.ListWarrantyCandidatesByServiceRow) { r.UnitSource = "external" }), SkipExternal},
		{"external connection", olex, with(func(r *db.ListWarrantyCandidatesByServiceRow) {
			r.UnitConnectionID = pgtype.Int8{Int64: 7, Valid: true}
		}), SkipExternal},
		// Glorian wins over a missing period (logged reason).
		{"glorian without period", glorian, with(func(r *db.ListWarrantyCandidatesByServiceRow) { r.WarrantyDurationMonths = pgtype.Int4{} }), SkipGlorian},
	}
	for _, tc := range cases {
		if got := skipReason(tc.svc, tc.item); got != tc.want {
			t.Errorf("%s: skipReason = %q, want %q", tc.name, got, tc.want)
		}
	}
}

type jsonNumber string

func (n jsonNumber) Int64() (int64, error) {
	var v int64
	for _, c := range n {
		v = v*10 + int64(c-'0')
	}
	return v, nil
}

func TestPayloadInt64(t *testing.T) {
	p := map[string]any{"a": int64(5), "b": float64(6), "c": 7, "d": 1.5, "e": "8", "f": jsonNumber("9")}
	for key, want := range map[string]int64{"a": 5, "b": 6, "c": 7, "f": 9} {
		if got, ok := payloadInt64(p, key); !ok || got != want {
			t.Errorf("%s = %d %v, want %d", key, got, ok, want)
		}
	}
	for _, key := range []string{"d", "e", "missing"} {
		if _, ok := payloadInt64(p, key); ok {
			t.Errorf("%s must not parse", key)
		}
	}
}
