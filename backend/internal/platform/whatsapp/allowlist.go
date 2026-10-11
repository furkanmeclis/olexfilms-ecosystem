package whatsapp

import (
	"errors"
	"strings"
)

// ErrRecipientNotAllowed is returned when a recipient allowlist is active and
// the number is not on it (dev/demo guard: a real phone is linked to a test
// stack whose seed data carries other people's numbers).
var ErrRecipientNotAllowed = errors.New("whatsapp: recipient not in allowlist")

// RecipientAllowlist is the set of numbers outbound messages may go to. An
// empty list allows everyone.
type RecipientAllowlist struct {
	digits map[string]struct{}
}

// NewRecipientAllowlist builds the set from E.164-ish strings ("+90 555 ...",
// "90555...", "whatsapp:+90555..." all compare by their digits).
func NewRecipientAllowlist(numbers []string) RecipientAllowlist {
	out := RecipientAllowlist{}
	for _, n := range numbers {
		if d := recipientDigits(n); d != "" {
			if out.digits == nil {
				out.digits = map[string]struct{}{}
			}
			out.digits[d] = struct{}{}
		}
	}
	return out
}

// Active reports whether the list restricts anything.
func (a RecipientAllowlist) Active() bool { return len(a.digits) > 0 }

// Len is the number of allowed recipients.
func (a RecipientAllowlist) Len() int { return len(a.digits) }

// Allows reports whether to may receive a message. An inactive list allows
// every recipient.
func (a RecipientAllowlist) Allows(to string) bool {
	if !a.Active() {
		return true
	}
	_, ok := a.digits[recipientDigits(to)]
	return ok
}

// Check returns ErrRecipientNotAllowed when to is blocked.
func (a RecipientAllowlist) Check(to string) error {
	if a.Allows(to) {
		return nil
	}
	return ErrRecipientNotAllowed
}

func recipientDigits(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, ":"); i >= 0 {
		raw = raw[i+1:]
	}
	if i := strings.Index(raw, "@"); i >= 0 {
		raw = raw[:i]
	}
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
