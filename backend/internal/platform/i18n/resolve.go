package i18n

import "context"

// Sources are the stored preferences the effective locale and timezone are
// resolved from. Empty fields are skipped; invalid stored values are skipped
// too, so a bad row never breaks a response.
type Sources struct {
	// UserLocale / UserTimezone: users.locale / users.timezone (NULL = inherit).
	UserLocale   string
	UserTimezone string
	// OrgLocale / OrgTimezone: the active organization.
	OrgLocale   string
	OrgTimezone string
	// CenterLocale / CenterTimezone: the center organization of the brand
	// (the active organization's brand, or the request brand).
	CenterLocale   string
	CenterTimezone string
	// AcceptLanguage is the raw request header.
	AcceptLanguage string
}

// Resolved is the effective locale and timezone of a request or recipient.
type Resolved struct {
	Locale   Locale
	Timezone string
}

// Resolve applies the K10 chains:
//
//	locale:   user -> active org -> brand center -> Accept-Language -> tr
//	timezone: user -> active org -> brand center -> Europe/Istanbul
//
// Requests, jobs and notification recipients all resolve through here.
func Resolve(s Sources) Resolved {
	out := Resolved{Locale: DefaultLocale, Timezone: DefaultTimezone}
	if l, ok := firstLocale(s.UserLocale, s.OrgLocale, s.CenterLocale); ok {
		out.Locale = l
	} else if l, ok := ParseAcceptLanguage(s.AcceptLanguage); ok {
		out.Locale = l
	}
	for _, tz := range []string{s.UserTimezone, s.OrgTimezone, s.CenterTimezone} {
		if ValidTimezone(tz) {
			out.Timezone = tz
			break
		}
	}
	return out
}

func firstLocale(values ...string) (Locale, bool) {
	for _, v := range values {
		if l, ok := Parse(v); ok {
			return l, true
		}
	}
	return "", false
}

type ctxKey int

const keyAcceptLanguage ctxKey = 1

// WithAcceptLanguage stores the request Accept-Language header on ctx (set by
// the locale middleware for every request).
func WithAcceptLanguage(ctx context.Context, header string) context.Context {
	return context.WithValue(ctx, keyAcceptLanguage, header)
}

// AcceptLanguage returns the request Accept-Language header stored on ctx.
func AcceptLanguage(ctx context.Context) string {
	v, _ := ctx.Value(keyAcceptLanguage).(string)
	return v
}

// FromContext resolves the request-level locale (Accept-Language -> tr) for
// code that has no user or organization at hand, such as public endpoints.
func FromContext(ctx context.Context) Resolved {
	return Resolve(Sources{AcceptLanguage: AcceptLanguage(ctx)})
}
