// Package whatsapp is the messaging provider abstraction (design K21): a
// WhatsApp Cloud API shaped interface with wuzapi as the first driver (K16:
// one instance, one number). Phone numbers are E.164 everywhere; drivers map
// them to their own wire format.
package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// Errors shared by drivers.
var (
	ErrInvalidSignature = errors.New("whatsapp: invalid webhook signature")
	ErrNotConfigured    = errors.New("whatsapp: provider not configured")
	ErrNotConnected     = errors.New("whatsapp: session not connected")
	ErrInvalidRecipient = errors.New("whatsapp: invalid recipient")
)

// Event kinds of an InboundEvent (Cloud API: messages[], statuses[], and the
// gateway-specific connection lifecycle).
const (
	KindMessage    = "message"
	KindStatus     = "status"
	KindConnection = "connection"
)

// Connection states reported in InboundEvent.Connection and ConnState.State.
const (
	StateConnected    = "connected"
	StateDisconnected = "disconnected"
	StateConnecting   = "connecting"
	StateQR           = "qr"
	StateLoggedOut    = "logged_out"
	StateBanned       = "banned"
)

// MsgRef identifies a sent message at the provider.
type MsgRef struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Timestamp time.Time `json:"timestamp"`
}

// Media is an outbound document or image. Data wins over URL.
type Media struct {
	URL      string
	Data     []byte
	MimeType string
	FileName string
	Caption  string
}

// SendOptions tunes an outbound message.
type SendOptions struct {
	// ID is a client-chosen idempotent message id (optional).
	ID string
}

// InboundMedia describes media attached to an inbound message.
type InboundMedia struct {
	Type     string `json:"type"`
	MimeType string `json:"mime_type,omitempty"`
	FileName string `json:"file_name,omitempty"`
	Caption  string `json:"caption,omitempty"`
	URL      string `json:"url,omitempty"`
	// Size is the file length the sender declared (0 = unknown).
	Size int64 `json:"size,omitempty"`
	// Download is the provider's reference to fetch the encrypted media
	// (wuzapi: directPath, mediaKey and hashes); MediaDownloader uses it.
	Download json.RawMessage `json:"download,omitempty"`
}

// InboundEvent is one normalized webhook event.
type InboundEvent struct {
	Kind       string
	ExternalID string
	// From / To are E.164 when they could be resolved, else empty.
	From      string
	To        string
	FromMe    bool
	PushName  string
	Timestamp time.Time
	Text      string
	Media     *InboundMedia
	// Status: delivered | read (KindStatus); MessageIDs lists the targets.
	Status     string
	MessageIDs []string
	// Connection: one of the State* constants (KindConnection).
	Connection string
	Reason     string
	JID        string
	// Type is the provider's raw event type (e.g. wuzapi "LoggedOut").
	Type string
	Raw  json.RawMessage
}

// ConnState is the provider session status.
type ConnState struct {
	Connected bool   `json:"connected"`
	LoggedIn  bool   `json:"logged_in"`
	JID       string `json:"jid,omitempty"`
	// Phone is the E.164 number of the linked device when known.
	Phone string `json:"phone,omitempty"`
}

// State maps Connected/LoggedIn to a State* constant.
func (s ConnState) State() string {
	switch {
	case s.Connected && s.LoggedIn:
		return StateConnected
	case s.Connected:
		return StateQR
	default:
		return StateDisconnected
	}
}

// Provider sends messages and parses webhooks.
type Provider interface {
	Name() string
	SendText(ctx context.Context, to, body string, opts SendOptions) (MsgRef, error)
	SendDocument(ctx context.Context, to string, doc Media, opts SendOptions) (MsgRef, error)
	SendImage(ctx context.Context, to string, img Media, opts SendOptions) (MsgRef, error)
	// ParseWebhook verifies the signature over the raw body and normalizes
	// the payload. It returns ErrInvalidSignature on a missing or wrong
	// signature.
	ParseWebhook(header http.Header, body []byte) ([]InboundEvent, error)
	Status(ctx context.Context) (ConnState, error)
}

// SessionManager is implemented by self-hosted gateways that link a phone
// with a QR code or pairing code (wuzapi). Cloud API has no equivalent.
type SessionManager interface {
	Connect(ctx context.Context) error
	QR(ctx context.Context) (string, error)
	PairPhone(ctx context.Context, e164 string) (string, error)
	Logout(ctx context.Context) error
}
