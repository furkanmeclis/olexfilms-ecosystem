// Package sms is the SMS fallback channel (design K21). The first release
// ships a logging no-op driver; a real gateway plugs in behind Provider.
package sms

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// Provider sends a text SMS to an E.164 number and returns a provider ref.
type Provider interface {
	Name() string
	Send(ctx context.Context, to, body string) (string, error)
}

// Noop logs and pretends delivery (no SMS gateway configured yet).
type Noop struct {
	Log *slog.Logger
}

// Name implements Provider.
func (Noop) Name() string { return "noop" }

// Send implements Provider. The body is not logged (it may carry an OTP).
func (n Noop) Send(_ context.Context, to string, body string) (string, error) {
	ref := "noop-" + uuid.NewString()
	if n.Log != nil {
		n.Log.Info("sms_noop_send", "to_suffix", suffix(to), "len", len(body), "ref", ref)
	}
	return ref, nil
}

func suffix(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}
