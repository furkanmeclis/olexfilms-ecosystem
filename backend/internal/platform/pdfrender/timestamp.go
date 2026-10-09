package pdfrender

import (
	"time"
	_ "time/tzdata" // printed times on images without a system zoneinfo

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// TEC-521: the issued-at line of every Gotenberg document is printed in a
// local zone, never in UTC.

// Zone is the zone the times of a document are printed in: the first valid
// IANA name of names (e.g. user, organization), else i18n.DefaultTimezone
// (K10). Empty, "Local" and unknown names are skipped.
func Zone(names ...string) *time.Location {
	for _, name := range names {
		if !i18n.ValidTimezone(name) {
			continue
		}
		if z, err := time.LoadLocation(name); err == nil {
			return z
		}
	}
	z, err := time.LoadLocation(i18n.DefaultTimezone)
	if err != nil {
		return time.UTC
	}
	return z
}

// IssuedAt prints the moment a document was issued as wall-clock time in
// zone, followed by the zone name: "2026-10-09 14:32 (Europe/Istanbul)". A
// nil zone is i18n.DefaultTimezone.
func IssuedAt(t time.Time, zone *time.Location) string {
	if zone == nil {
		zone = Zone()
	}
	return t.In(zone).Format("2006-01-02 15:04") + " (" + zone.String() + ")"
}
