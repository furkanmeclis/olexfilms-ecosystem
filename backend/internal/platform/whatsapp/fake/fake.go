// Package fake is an in-memory whatsapp.Provider for tests and local runs
// without a gateway.
package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// Sent is one recorded outbound message.
type Sent struct {
	To   string
	Body string
	ID   string
	Kind string // text | document | image
}

// Provider records sends. Set Err to make every send fail.
type Provider struct {
	mu    sync.Mutex
	sent  []Sent
	Err   error
	State whatsapp.ConnState
	// Events is returned by ParseWebhook (signature always accepted).
	Events []whatsapp.InboundEvent
	seq    int
}

// Name implements whatsapp.Provider.
func (*Provider) Name() string { return "fake" }

func (p *Provider) record(kind, to, body, id string) (whatsapp.MsgRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Err != nil {
		return whatsapp.MsgRef{}, p.Err
	}
	p.seq++
	if id == "" {
		id = fmt.Sprintf("FAKE%06d", p.seq)
	}
	p.sent = append(p.sent, Sent{To: to, Body: body, ID: id, Kind: kind})
	return whatsapp.MsgRef{ID: id, Provider: "fake", Timestamp: time.Now().UTC()}, nil
}

// SendText implements whatsapp.Provider.
func (p *Provider) SendText(_ context.Context, to, body string, opts whatsapp.SendOptions) (whatsapp.MsgRef, error) {
	return p.record("text", to, body, opts.ID)
}

// SendDocument implements whatsapp.Provider.
func (p *Provider) SendDocument(_ context.Context, to string, doc whatsapp.Media, opts whatsapp.SendOptions) (whatsapp.MsgRef, error) {
	return p.record("document", to, doc.Caption, opts.ID)
}

// SendImage implements whatsapp.Provider.
func (p *Provider) SendImage(_ context.Context, to string, img whatsapp.Media, opts whatsapp.SendOptions) (whatsapp.MsgRef, error) {
	return p.record("image", to, img.Caption, opts.ID)
}

// ParseWebhook implements whatsapp.Provider.
func (p *Provider) ParseWebhook(_ http.Header, body []byte) ([]whatsapp.InboundEvent, error) {
	out := make([]whatsapp.InboundEvent, len(p.Events))
	copy(out, p.Events)
	for i := range out {
		out[i].Raw = json.RawMessage(body)
	}
	return out, nil
}

// Status implements whatsapp.Provider.
func (p *Provider) Status(context.Context) (whatsapp.ConnState, error) { return p.State, nil }

// Sent returns a copy of the recorded messages.
func (p *Provider) Sent() []Sent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Sent(nil), p.sent...)
}

// Last returns the last recorded message.
func (p *Provider) Last() (Sent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.sent) == 0 {
		return Sent{}, false
	}
	return p.sent[len(p.sent)-1], true
}

var _ whatsapp.Provider = (*Provider)(nil)
