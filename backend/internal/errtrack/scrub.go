package errtrack

import (
	"regexp"
	"strings"

	"github.com/getsentry/sentry-go"
)

// Masks written in place of personal data.
const (
	maskEmail    = "[email]"
	maskPhone    = "[phone]"
	maskFiltered = "[Filtered]"
)

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	// phoneCandidateRe finds digit runs that may be phone numbers (E.164 or
	// local, with spaces/dots/dashes/parentheses). Boundaries are checked in
	// scrubPhones so UUID fragments and identifiers are left alone.
	phoneCandidateRe = regexp.MustCompile(`\+?\(?\d[\d\s().\-]{6,}\d`)
)

// Keys whose values are personal data regardless of content (extra,
// contexts, tags set by mistake).
var sensitiveKeys = map[string]struct{}{
	"email": {}, "e_mail": {}, "phone": {}, "phone_number": {}, "phone_e164": {},
	"phone_national": {}, "mobile": {}, "gsm": {}, "name": {}, "full_name": {},
	"first_name": {}, "last_name": {}, "surname": {}, "username": {}, "address": {},
	"password": {}, "token": {}, "access_token": {}, "refresh_token": {},
	"authorization": {}, "cookie": {}, "otp": {}, "code": {}, "tax_number": {},
	"national_id": {}, "tckn": {}, "ip": {}, "ip_address": {},
}

// sdkContexts are filled by the SDK itself (runtime/os/device/trace) and
// hold no personal data; key filtering would only mangle them.
var sdkContexts = map[string]struct{}{"os": {}, "runtime": {}, "device": {}, "trace": {}}

func isSensitiveKey(k string) bool {
	_, ok := sensitiveKeys[strings.ToLower(strings.TrimSpace(k))]
	return ok
}

func isIdentChar(b byte) bool {
	return b == '-' || b == '_' ||
		(b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func scrubPhones(s string) string {
	locs := phoneCandidateRe.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		// Part of a larger identifier (uuid, hex id) → keep.
		// A ':' right after the run means a clock time (2026-10-01 12:00).
		if (start > 0 && isIdentChar(s[start-1])) || (end < len(s) && (isIdentChar(s[end]) || s[end] == ':')) {
			continue
		}
		digits := 0
		for i := start; i < end; i++ {
			if s[i] >= '0' && s[i] <= '9' {
				digits++
			}
		}
		// Phone numbers have 10–15 digits (E.164 max 15; TR local 10–11).
		if digits < 10 || digits > 15 {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString(maskPhone)
		last = end
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// ScrubString masks e-mail addresses and phone numbers in s.
func ScrubString(s string) string {
	if s == "" {
		return s
	}
	s = emailRe.ReplaceAllString(s, maskEmail)
	return scrubPhones(s)
}

func scrubValue(v any) any {
	switch t := v.(type) {
	case string:
		return ScrubString(t)
	case map[string]any:
		return scrubMap(t)
	case map[string]string:
		out := make(map[string]string, len(t))
		for k, val := range t {
			if isSensitiveKey(k) {
				out[k] = maskFiltered
				continue
			}
			out[k] = ScrubString(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = scrubValue(val)
		}
		return out
	case []string:
		out := make([]string, len(t))
		for i, val := range t {
			out[i] = ScrubString(val)
		}
		return out
	default:
		return v
	}
}

func scrubMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if isSensitiveKey(k) {
			out[k] = maskFiltered
			continue
		}
		out[k] = scrubValue(v)
	}
	return out
}

// Scrub removes personal data from an event before it leaves the process
// (the receiver does no masking): e-mail/phone patterns are masked in every
// free-text field, sensitive keys are filtered, the user keeps only its id
// and the request keeps only method + scrubbed URL.
func Scrub(event *sentry.Event) *sentry.Event {
	if event == nil {
		return nil
	}
	event.Message = ScrubString(event.Message)
	event.ServerName = ""
	event.User = sentry.User{ID: event.User.ID}
	for i := range event.Exception {
		event.Exception[i].Value = ScrubString(event.Exception[i].Value)
	}
	for _, bc := range event.Breadcrumbs {
		if bc == nil {
			continue
		}
		bc.Message = ScrubString(bc.Message)
		bc.Data = scrubMap(bc.Data)
	}
	for k, v := range event.Tags {
		if isSensitiveKey(k) {
			event.Tags[k] = maskFiltered
			continue
		}
		event.Tags[k] = ScrubString(v)
	}
	for name, c := range event.Contexts {
		if _, builtin := sdkContexts[name]; builtin {
			continue
		}
		event.Contexts[name] = sentry.Context(scrubMap(map[string]any(c)))
	}
	if event.Request != nil {
		event.Request = &sentry.Request{
			Method: event.Request.Method,
			URL:    ScrubString(stripQuery(event.Request.URL)),
		}
	}
	return event
}

func stripQuery(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i]
	}
	return u
}

func beforeSend(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	return Scrub(event)
}
