// Package appversion parses and compares mobile app release versions
// (TEC-236): "2.4.1", "v2.4", "2.4.1+57", "2.4.1-beta.2". Only the numeric
// core (up to four dot-separated parts) is compared; missing parts are 0, so
// "2.4" equals "2.4.0". A pre-release or build suffix is ignored, which keeps
// a "2.4.0-rc1" test build on the same side of the threshold as "2.4.0".
package appversion

import (
	"regexp"
	"strconv"
	"strings"
)

// maxParts bounds the numeric core ("1.2.3.4").
const maxParts = 4

// Version is the numeric core of a release version.
type Version [maxParts]int

// Parse reads a version string. ok is false for an empty or malformed
// value (no digits, a non-numeric part, more than four parts, a part above
// 99999).
func Parse(raw string) (Version, bool) {
	var v Version
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if i := strings.IndexAny(s, "-+ "); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return v, false
	}
	parts := strings.Split(s, ".")
	if len(parts) > maxParts {
		return v, false
	}
	for i, p := range parts {
		if p == "" || len(p) > 5 {
			return v, false
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// Valid reports whether raw parses.
func Valid(raw string) bool {
	_, ok := Parse(raw)
	return ok
}

// Compare returns -1, 0 or 1 as a is below, equal to or above b.
func Compare(a, b Version) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}

// String prints the version with at least three parts ("2.4.0").
func (v Version) String() string {
	n := 3
	if v[3] != 0 {
		n = 4
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = strconv.Itoa(v[i])
	}
	return strings.Join(out, ".")
}

var uaToken = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_.\-]*)/v?(\d+(?:\.\d+){0,3})`)

// FromUserAgent finds the version of one of products in a User-Agent
// ("OlexFilms/2.4.1 (iPhone; iOS 17.4) CFNetwork/1494"). Product names are
// matched case-insensitively and exactly, so a generic HTTP client token
// ("okhttp/4.12.0", "CFNetwork/1494") is never read as the app version.
func FromUserAgent(ua string, products []string) (Version, bool) {
	if ua == "" || len(products) == 0 {
		return Version{}, false
	}
	for _, m := range uaToken.FindAllStringSubmatch(ua, -1) {
		for _, p := range products {
			if strings.EqualFold(m[1], p) {
				if v, ok := Parse(m[2]); ok {
					return v, true
				}
			}
		}
	}
	return Version{}, false
}
