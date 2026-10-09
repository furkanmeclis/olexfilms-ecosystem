// Package provider is the boundary to a GİB e-invoice integrator (özel
// entegratör). Sending invoices is out of F5 scope (design §9): only the
// interface and a noop implementation exist, and nothing calls them yet.
package provider

import (
	"context"
	"errors"
)

// ErrNotConfigured: no integrator is configured.
var ErrNotConfigured = errors.New("einvoice provider: not configured")

// Envelope is an archived invoice handed to the integrator.
type Envelope struct {
	UUID    string
	Number  string
	Profile string
	// ReceiverAlias is the buyer's GİB mailbox (organizations.einvoice_alias)
	// for e-Fatura; empty for e-Arşiv.
	ReceiverAlias string
	XML           []byte
}

// Status is the integrator's view of a sent invoice.
type Status struct {
	State   string
	Message string
}

// Provider sends, tracks and cancels invoices at an integrator.
type Provider interface {
	Send(ctx context.Context, env Envelope) (Status, error)
	Status(ctx context.Context, uuid string) (Status, error)
	Cancel(ctx context.Context, uuid, reason string) (Status, error)
}

// Noop is the F5 provider: every call is ErrNotConfigured.
type Noop struct{}

var _ Provider = Noop{}

func (Noop) Send(context.Context, Envelope) (Status, error) { return Status{}, ErrNotConfigured }

func (Noop) Status(context.Context, string) (Status, error) { return Status{}, ErrNotConfigured }

func (Noop) Cancel(context.Context, string, string) (Status, error) {
	return Status{}, ErrNotConfigured
}
