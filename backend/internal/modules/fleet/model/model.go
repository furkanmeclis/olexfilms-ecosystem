// Package model holds the fleet (F5-02, TEC-127) constants and the
// VKN/TCKN checksum rules.
package model

// Link statuses of fleet_dealer_links.
const (
	LinkPending = "pending"
	LinkActive  = "active"
	LinkEnded   = "ended"
)

// Report frequencies of fleet_profiles.report_frequency (F5 Q12: monthly
// by default).
const (
	ReportMonthly   = "monthly"
	ReportQuarterly = "quarterly"
	ReportOff       = "off"
)

// Report statuses of fleet_reports.
const (
	ReportPending = "pending"
	ReportReady   = "ready"
	ReportFailed  = "failed"
)

// Named business rule codes (422 / 409) of the fleet endpoints.
const (
	CodeInvalidTaxNumber = "FLEET_INVALID_TAX_NUMBER"
	CodeFleetExists      = "FLEET_ALREADY_EXISTS"
	CodeLinkExists       = "FLEET_LINK_EXISTS"
	CodeLinkStale        = "FLEET_LINK_STATE_CHANGED"
)

// ValidTaxNumber reports whether s is a valid VKN (10 digits) or TCKN (11
// digits) including its checksum.
func ValidTaxNumber(s string) bool {
	switch len(s) {
	case 10:
		return ValidVKN(s)
	case 11:
		return ValidTCKN(s)
	default:
		return false
	}
}

// ValidVKN checks a 10-digit Turkish tax identification number (vergi
// kimlik numarası) with the Gelir İdaresi checksum.
func ValidVKN(s string) bool {
	d, ok := digits(s, 10)
	if !ok {
		return false
	}
	sum := 0
	for i := 0; i < 9; i++ {
		t := (d[i] + 9 - i) % 10
		if t == 9 {
			sum += 9
			continue
		}
		sum += (t << (9 - i)) % 9
	}
	return (10-sum%10)%10 == d[9]
}

// ValidTCKN checks an 11-digit Turkish citizen identity number (sole
// proprietors invoice with it).
func ValidTCKN(s string) bool {
	d, ok := digits(s, 11)
	if !ok || d[0] == 0 {
		return false
	}
	odd := d[0] + d[2] + d[4] + d[6] + d[8]
	even := d[1] + d[3] + d[5] + d[7]
	if ((odd*7-even)%10+10)%10 != d[9] {
		return false
	}
	sum := 0
	for _, v := range d[:10] {
		sum += v
	}
	return sum%10 == d[10]
}

func digits(s string, n int) ([]int, bool) {
	if len(s) != n {
		return nil, false
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return nil, false
		}
		out[i] = int(c - '0')
	}
	return out, true
}
