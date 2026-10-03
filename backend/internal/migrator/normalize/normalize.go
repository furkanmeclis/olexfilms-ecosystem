// Package normalize holds the value clean-up every migrator step shares
// (TEC-252): legacy phones to E.164 (K29) and e-mail addresses.
package normalize

import (
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
)

// Phone is a legacy phone after normalization.
type Phone struct {
	// Raw is the trimmed source value, kept for unresolved numbers.
	Raw string
	// E164, Country and National are set only when Verified.
	E164     string
	Country  string
	National string
	// Verified is false for empty or unparseable input: the record is
	// migrated as "unverified" and claimed on the first OTP login (K26, K29).
	Verified bool
}

// NormalizePhone parses a legacy phone with defaultRegion (empty = TR).
// Legacy forms such as "0(555) 123 45 67", "0555...", "555...", "90555..."
// and "+90 555..." resolve to +905551234567; anything libphonenumber does
// not accept as a valid number stays unverified.
func NormalizePhone(raw, defaultRegion string) Phone {
	raw = strings.TrimSpace(raw)
	out := Phone{Raw: raw}
	if raw == "" {
		return out
	}
	// libphonenumber strips the default region's calling code itself, so
	// "905551234567" resolves without a "+".
	n, err := phone.Parse(raw, defaultRegion)
	if err != nil {
		return out
	}
	out.E164, out.Country, out.National, out.Verified = n.E164, n.Country, n.National, true
	return out
}

// NormalizeEmail lower-cases and trims an address; blank input returns "".
// It does not validate: a malformed legacy address is kept as is (lowered)
// for an operator to fix.
func NormalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
