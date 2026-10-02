package usecase

import (
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Eligibility is what the write-free warranty rules read for one service
// item. The service.completed listener (TEC-186) and the repair scan
// (TEC-194) both build it from their own query rows and ask SkipReason, so
// the two paths cannot drift apart.
type Eligibility struct {
	ServiceBrandSlug       string
	UnitBrandSlug          string
	UnitSource             string
	UnitConnectionID       pgtype.Int8
	ExternalOutbound       bool
	WarrantyDurationMonths pgtype.Int4
}

// SkipReason applies the rules that need no write: Glorian (service or unit
// brand, K2), units that left the system or came from outside, and the
// warranty period of the product. "" means the item should get a warranty,
// subject to the write-time checks (an existing warranty, an active full
// warranty of the vehicle on the unit).
func SkipReason(e Eligibility) string {
	switch {
	case strings.EqualFold(e.ServiceBrandSlug, GlorianBrandSlug) || strings.EqualFold(e.UnitBrandSlug, GlorianBrandSlug):
		return SkipGlorian
	case e.ExternalOutbound || e.UnitSource == "external" || e.UnitConnectionID.Valid:
		return SkipExternal
	case !e.WarrantyDurationMonths.Valid || e.WarrantyDurationMonths.Int32 <= 0:
		return SkipNoPeriod
	}
	return ""
}
