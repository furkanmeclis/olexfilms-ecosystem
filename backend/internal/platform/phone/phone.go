// Package phone normalizes phone numbers to E.164 (design K29).
//
// Every phone stored in the database, used as an OTP key or sent to a
// messaging provider goes through Parse. The default region is the
// organization's country when known, otherwise TR.
package phone

import (
	"errors"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// DefaultRegion is used when neither the input nor the caller names a country.
const DefaultRegion = "TR"

// ErrInvalid is returned for input that is not a valid phone number.
var ErrInvalid = errors.New("phone: invalid number")

// Number is a parsed, validated phone number.
type Number struct {
	// E164 is the canonical form, e.g. +905551234567.
	E164 string
	// Country is the ISO 3166-1 alpha-2 region, e.g. TR.
	Country string
	// National is the national significant number, e.g. 5551234567.
	National string
}

// Digits returns the E.164 number without the leading "+", the form
// WhatsApp gateways (wuzapi) expect.
func (n Number) Digits() string { return strings.TrimPrefix(n.E164, "+") }

// Region picks the default region for parsing: the first non-empty ISO
// alpha-2 code in candidates (for example the organization country), else TR.
func Region(candidates ...string) string {
	for _, c := range candidates {
		c = strings.ToUpper(strings.TrimSpace(c))
		if len(c) == 2 {
			return c
		}
	}
	return DefaultRegion
}

// Parse validates raw input and returns its E.164 form. Input with a leading
// "+" or "00" is parsed as international; anything else uses defaultRegion
// (empty means TR).
func Parse(raw, defaultRegion string) (Number, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Number{}, ErrInvalid
	}
	if strings.HasPrefix(raw, "00") {
		raw = "+" + strings.TrimPrefix(raw, "00")
	}
	region := Region(defaultRegion)
	num, err := phonenumbers.Parse(raw, region)
	if err != nil {
		return Number{}, ErrInvalid
	}
	if !phonenumbers.IsValidNumber(num) {
		return Number{}, ErrInvalid
	}
	return Number{
		E164:     phonenumbers.Format(num, phonenumbers.E164),
		Country:  phonenumbers.GetRegionCodeForNumber(num),
		National: phonenumbers.GetNationalSignificantNumber(num),
	}, nil
}

// NormalizeE164 is Parse returning only the E.164 string.
func NormalizeE164(raw, defaultRegion string) (string, error) {
	n, err := Parse(raw, defaultRegion)
	if err != nil {
		return "", err
	}
	return n.E164, nil
}

// FromDigits parses a gateway identifier such as "905551234567" or a JID
// local part ("905551234567@s.whatsapp.net", "905551234567:12@...") as an
// international number.
func FromDigits(id string) (Number, error) {
	id = strings.TrimSpace(id)
	if at := strings.IndexByte(id, '@'); at >= 0 {
		id = id[:at]
	}
	if colon := strings.IndexByte(id, ':'); colon >= 0 {
		id = id[:colon]
	}
	if dot := strings.IndexByte(id, '.'); dot >= 0 {
		id = id[:dot]
	}
	id = strings.TrimPrefix(id, "+")
	if id == "" {
		return Number{}, ErrInvalid
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return Number{}, ErrInvalid
		}
	}
	return Parse("+"+id, "")
}

// Mask hides all but the last four digits (logs, API echoes).
func Mask(e164 string) string {
	if len(e164) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(e164)-4) + e164[len(e164)-4:]
}
