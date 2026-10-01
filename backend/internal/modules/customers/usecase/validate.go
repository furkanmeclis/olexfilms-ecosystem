package usecase

import (
	"bytes"
	"encoding/json"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Customer types (customer_profiles.type).
const (
	TypeIndividual = "individual"
	TypeCorporate  = "corporate"
)

// Column limits.
const (
	maxNameLen        = 100
	maxEmailLen       = 255
	maxCompanyNameLen = 200
	maxTaxOfficeLen   = 150
	maxJSONBytes      = 8 << 10
)

var (
	vinRe      = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)
	identityRe = regexp.MustCompile(`^[0-9A-Z]{5,20}$`)
)

// notificationChannels are the keys of customer_profiles.notification_prefs.
var notificationChannels = []string{"whatsapp", "email", "sms", "push"}

// normalizeName trims a name or surname and checks its length.
func normalizeName(raw, field string) (string, error) {
	v := strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(v) > maxNameLen {
		return "", invalid(field, "must be at most 100 characters")
	}
	return v, nil
}

// normalizeEmail lower-cases and checks an e-mail; empty means none.
func normalizeEmail(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return "", nil
	}
	if len(v) > maxEmailLen {
		return "", invalid("email", "must be at most 255 characters")
	}
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v || !strings.Contains(v[strings.LastIndex(v, "@")+1:], ".") {
		return "", invalid("email", "must be a valid e-mail address")
	}
	return v, nil
}

// normalizeText trims an optional text and checks its length; "" means none.
func normalizeText(raw, field string, maxLen int, msg string) (string, error) {
	v := strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(v) > maxLen {
		return "", invalid(field, msg)
	}
	return v, nil
}

// normalizeIdentityNumber cleans a national id (TC kimlik no) or tax number:
// separators are dropped, letters upper-cased; 5-20 letters or digits. ""
// means none.
func normalizeIdentityNumber(raw, field string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		switch r {
		case ' ', '-', '.', '/', '\t':
			continue
		}
		b.WriteRune(r)
	}
	v := b.String()
	if v == "" {
		return "", nil
	}
	if !identityRe.MatchString(v) {
		return "", invalid(field, "must be 5-20 letters or digits")
	}
	return v, nil
}

// normalizeCustomerType checks the customer type; "" keeps the default.
func normalizeCustomerType(raw string) (string, error) {
	switch v := strings.TrimSpace(raw); v {
	case "", TypeIndividual, TypeCorporate:
		return v, nil
	default:
		return "", invalid("type", "must be individual or corporate")
	}
}

// NormalizeVIN cleans a VIN (spaces and dashes dropped, upper-cased) and
// checks it: 17 letters or digits without I, O and Q. "" means none.
func NormalizeVIN(raw string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		if r == ' ' || r == '-' || r == '\t' {
			continue
		}
		b.WriteRune(r)
	}
	v := b.String()
	if v == "" {
		return "", nil
	}
	if !vinRe.MatchString(v) {
		return "", invalid("vin", "must be 17 letters or digits (no I, O or Q)")
	}
	return v, nil
}

// validateModelYear accepts 1900 up to next year.
func validateModelYear(year, currentYear int) error {
	if year < 1900 || year > currentYear+1 {
		return invalid("model_year", "must be between 1900 and next year")
	}
	return nil
}

// normalizeAddress checks a JSON object (free-form address, K29 country/
// province/district ids live inside); nil means none ("{}").
func normalizeAddress(raw json.RawMessage) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return []byte("{}"), nil
	}
	if len(raw) > maxJSONBytes {
		return nil, invalid("address", "is too large")
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, invalid("address", "must be a JSON object")
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, invalid("address", "must be a JSON object")
	}
	return out, nil
}

// mergeNotificationPrefs applies a {channel: bool} patch over current
// preferences (or the defaults); unknown channels are refused.
func mergeNotificationPrefs(current []byte, patch map[string]bool) ([]byte, error) {
	prefs := map[string]bool{"whatsapp": true, "email": true, "sms": false, "push": true}
	if len(current) > 0 {
		var cur map[string]bool
		if err := json.Unmarshal(current, &cur); err == nil {
			for k, v := range cur {
				prefs[k] = v
			}
		}
	}
	for k, v := range patch {
		known := false
		for _, c := range notificationChannels {
			if c == k {
				known = true
				break
			}
		}
		if !known {
			return nil, invalid("notification_prefs", "unknown channel "+k)
		}
		prefs[k] = v
	}
	return json.Marshal(prefs)
}

// isEmptyAddress reports whether a stored address holds nothing.
func isEmptyAddress(b []byte) bool {
	t := bytes.TrimSpace(b)
	return len(t) == 0 || string(t) == "{}" || string(t) == "null"
}
