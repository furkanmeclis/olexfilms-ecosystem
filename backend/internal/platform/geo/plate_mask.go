package geo

import (
	"regexp"
	"strings"
)

// trPlateParts splits a compact Turkish plate (seed regex of migration
// 000035: province code, 1-3 letters, 2-5 digits) into its groups.
var trPlateParts = regexp.MustCompile(`^(0[1-9]|[1-7][0-9]|8[01])([A-Z]{1,3})([0-9]{2,5})$`)

// plateMask is the fixed middle of a masked plate. Its length never depends
// on the hidden characters, so the masked form does not leak the plate length.
const plateMask = "***"

// MaskPlate returns a plate safe for public pages (TEC-189): a Turkish plate
// keeps its province code and the last two digits ("34 ABC 123" ->
// "34 *** 23"); other countries keep the first and the last two characters
// of the compact form. A plate of four or fewer characters is fully masked.
// An empty plate returns "".
func MaskPlate(country, plate string) string {
	n := []rune(NormalizePlate(plate))
	if len(n) == 0 {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(country), "TR") {
		if m := trPlateParts.FindStringSubmatch(string(n)); m != nil {
			digits := m[3]
			return m[1] + " " + plateMask + " " + digits[len(digits)-2:]
		}
	}
	if len(n) <= 4 {
		return plateMask
	}
	return string(n[:2]) + " " + plateMask + " " + string(n[len(n)-2:])
}

// VINLast4 returns the last four characters of a VIN ("" when shorter).
func VINLast4(vin string) string {
	v := []rune(strings.ToUpper(strings.TrimSpace(vin)))
	if len(v) < 4 {
		return ""
	}
	return string(v[len(v)-4:])
}
