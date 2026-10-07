// Package usecase manages the WhatsApp gateway (K16: one wuzapi instance,
// one number): instance bootstrap, QR / pairing, status, test messages, SMS
// fallback and KVKK texts, and inbound webhooks (messages, receipts,
// connection alarms).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/wuzapi"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidRequest = errors.New("whatsapp: invalid request")
	ErrNotConfigured  = errors.New("whatsapp: gateway not configured")
)

// KVKKLocales are the locales whose KVKK notice the admin edits (seeded tr,
// en; other UI languages fall back to en on OTP messages).
var KVKKLocales = []string{"tr", "en"}

// Gateway is the wuzapi client surface the service needs.
type Gateway interface {
	whatsapp.Provider
	whatsapp.SessionManager
	Configured() bool
	EnsureUser(ctx context.Context, name string) (wuzapi.User, bool, error)
	SetUserToken(token string)
	UserToken() string
	SetWebhook(ctx context.Context) error
	WebhookURL() string
}

// Store is the sqlc query surface used by the service.
type Store interface {
	GetWhatsAppSettings(ctx context.Context) (db.WhatsappSetting, error)
	SetWhatsAppInstance(ctx context.Context, arg db.SetWhatsAppInstanceParams) (db.WhatsappSetting, error)
	UpdateWhatsAppStatus(ctx context.Context, arg db.UpdateWhatsAppStatusParams) (db.WhatsappSetting, error)
	SetWhatsAppSMSFallback(ctx context.Context, enabled bool) (db.WhatsappSetting, error)
	InsertWhatsAppConnectionEvent(ctx context.Context, arg db.InsertWhatsAppConnectionEventParams) (db.WhatsappConnectionEvent, error)
	ListWhatsAppConnectionEvents(ctx context.Context, limit int32) ([]db.WhatsappConnectionEvent, error)
	ListLatestKVKKNotices(ctx context.Context) ([]db.KvkkNotice, error)
	InsertKVKKNotice(ctx context.Context, arg db.InsertKVKKNoticeParams) (db.KvkkNotice, error)
	ApplyMessageReceipt(ctx context.Context, arg db.ApplyMessageReceiptParams) ([]db.Message, error)
	ListWhatsAppAlarmRecipients(ctx context.Context) ([]db.ListWhatsAppAlarmRecipientsRow, error)
	GetUserByPhone(ctx context.Context, phoneE164 pgtype.Text) (db.User, error)
	WithTx(tx pgx.Tx) *db.Queries
}

// TxBeginner opens a transaction (pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Outbox writes an event in the caller's transaction.
type Outbox interface {
	Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error
}

// Notifier enqueues admin notifications.
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// SecretBox encrypts the instance user token at rest.
type SecretBox interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(encoded string) (string, error)
}

// Service is the WhatsApp gateway use case.
type Service struct {
	q        Store
	tx       TxBeginner
	gw       Gateway
	box      SecretBox
	outbox   Outbox
	notifier Notifier
	log      *slog.Logger
	now      func() time.Time
	// msgs publishes webhook messages/receipts and stores inbound media
	// (TEC-395); nil leaves both off.
	msgs *Messaging

	mu    sync.Mutex
	ready bool
}

// New creates the service. outbox and notifier may be nil.
func New(q Store, tx TxBeginner, gw Gateway, box SecretBox, outbox Outbox, notifier Notifier, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{q: q, tx: tx, gw: gw, box: box, outbox: outbox, notifier: notifier, log: log, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// AttachMessaging connects the conversation messaging use case to the
// webhook (realtime publish, inbound media storage, receipts).
func (s *Service) AttachMessaging(m *Messaging) { s.msgs = m }

// Provider exposes the gateway as a send provider (OTP sender, adapters).
func (s *Service) Provider() whatsapp.Provider { return readyProvider{s} }

// MediaDownloader exposes the gateway's inbound media download (nil when
// the gateway cannot download).
func (s *Service) MediaDownloader() whatsapp.MediaDownloader {
	if _, ok := s.gw.(whatsapp.MediaDownloader); !ok || s.gw == nil {
		return nil
	}
	return readyProvider{s}
}

// EnsureReady loads (or creates) the wuzapi instance user and its token.
// Idempotent: the instance user is looked up by name before creating.
func (s *Service) EnsureReady(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ready {
		return nil
	}
	if s.gw == nil {
		return ErrNotConfigured
	}
	st, err := s.q.GetWhatsAppSettings(ctx)
	if err != nil {
		return err
	}
	if st.UserTokenEnc.Valid && st.UserTokenEnc.String != "" {
		token, err := s.box.Decrypt(st.UserTokenEnc.String)
		if err != nil {
			return fmt.Errorf("whatsapp: decrypt token: %w", err)
		}
		s.gw.SetUserToken(token)
		s.ready = true
		return nil
	}
	if !s.gw.Configured() {
		return ErrNotConfigured
	}
	u, created, err := s.gw.EnsureUser(ctx, st.InstanceName)
	if err != nil {
		return err
	}
	enc, err := s.box.Encrypt(u.Token)
	if err != nil {
		return err
	}
	if _, err := s.q.SetWhatsAppInstance(ctx, db.SetWhatsAppInstanceParams{
		InstanceName: st.InstanceName, InstanceID: text(u.ID), UserTokenEnc: text(enc),
	}); err != nil {
		return err
	}
	s.gw.SetUserToken(u.Token)
	if !created && s.gw.WebhookURL() != "" && u.Webhook != s.gw.WebhookURL() {
		if err := s.gw.SetWebhook(ctx); err != nil {
			s.log.Warn("whatsapp_set_webhook_failed", "error", err)
		}
	}
	s.ready = true
	s.log.Info("whatsapp_instance_ready", "instance", st.InstanceName, "created", created)
	return nil
}

// Overview is the admin status payload.
type Overview struct {
	Provider           string          `json:"provider"`
	Configured         bool            `json:"configured"`
	Status             string          `json:"status"`
	Connected          bool            `json:"connected"`
	LoggedIn           bool            `json:"logged_in"`
	JID                string          `json:"jid,omitempty"`
	Phone              string          `json:"phone,omitempty"`
	LastSeenAt         *time.Time      `json:"last_seen_at,omitempty"`
	LastEventAt        *time.Time      `json:"last_event_at,omitempty"`
	LastErrorReason    string          `json:"last_error_reason,omitempty"`
	SMSFallbackEnabled bool            `json:"sms_fallback_enabled"`
	WebhookConfigured  bool            `json:"webhook_configured"`
	KVKK               []KVKKNotice    `json:"kvkk_notices"`
	Events             []ConnectionLog `json:"events"`
	GatewayError       string          `json:"gateway_error,omitempty"`
}

// KVKKNotice is the latest notice version of a locale.
type KVKKNotice struct {
	Locale    string    `json:"locale"`
	Version   int32     `json:"version"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConnectionLog is one connection history row.
type ConnectionLog struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason,omitempty"`
	Alarm     bool      `json:"alarm"`
	CreatedAt time.Time `json:"created_at"`
}

// Overview returns stored state refreshed with a live status call.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	out := Overview{Provider: wuzapi.ProviderName}
	if s.gw != nil {
		out.Configured = s.gw.Configured()
		out.WebhookConfigured = s.gw.WebhookURL() != ""
	}
	if err := s.EnsureReady(ctx); err == nil {
		if live, err := s.gw.Status(ctx); err == nil {
			out.Connected, out.LoggedIn, out.JID, out.Phone = live.Connected, live.LoggedIn, live.JID, live.Phone
			s.syncLiveStatus(ctx, live)
		} else {
			out.GatewayError = err.Error()
		}
	} else if !errors.Is(err, ErrNotConfigured) {
		out.GatewayError = err.Error()
	}
	st, err := s.q.GetWhatsAppSettings(ctx)
	if err != nil {
		return Overview{}, err
	}
	out.Status = st.Status
	out.SMSFallbackEnabled = st.SmsFallbackEnabled
	out.LastErrorReason = st.LastErrorReason.String
	if out.Phone == "" {
		out.Phone = st.PhoneE164.String
	}
	if out.JID == "" {
		out.JID = st.Jid.String
	}
	out.LastSeenAt = tsPtr(st.LastSeenAt)
	out.LastEventAt = tsPtr(st.LastEventAt)
	out.KVKK, err = s.KVKKNotices(ctx)
	if err != nil {
		return Overview{}, err
	}
	rows, err := s.q.ListWhatsAppConnectionEvents(ctx, 20)
	if err != nil {
		return Overview{}, err
	}
	out.Events = make([]ConnectionLog, 0, len(rows))
	for _, r := range rows {
		out.Events = append(out.Events, ConnectionLog{Type: r.Type, Reason: r.Reason.String, Alarm: r.Alarm, CreatedAt: r.CreatedAt.Time})
	}
	return out, nil
}

// syncLiveStatus stores a polled state when it differs from the DB.
func (s *Service) syncLiveStatus(ctx context.Context, live whatsapp.ConnState) {
	st, err := s.q.GetWhatsAppSettings(ctx)
	if err != nil {
		return
	}
	state := live.State()
	if state == st.Status || (state == whatsapp.StateQR && st.Status == whatsapp.StateConnecting) {
		return
	}
	// Do not let a poll hide a ban/logout alarm with a plain "disconnected".
	if state == whatsapp.StateDisconnected && (st.Status == whatsapp.StateBanned || st.Status == whatsapp.StateLoggedOut) {
		return
	}
	_, _ = s.q.UpdateWhatsAppStatus(ctx, db.UpdateWhatsAppStatusParams{
		Status: state, Jid: text(live.JID), PhoneE164: text(live.Phone), At: ts(s.now()),
		LastErrorReason: st.LastErrorReason,
	})
}

// Connect starts the WhatsApp session (QR is then available).
func (s *Service) Connect(ctx context.Context) error {
	if err := s.EnsureReady(ctx); err != nil {
		return err
	}
	if err := s.gw.Connect(ctx); err != nil {
		return err
	}
	_, err := s.q.UpdateWhatsAppStatus(ctx, db.UpdateWhatsAppStatusParams{Status: whatsapp.StateConnecting, At: ts(s.now())})
	return err
}

// QRResult is either a QR image or the connected state.
type QRResult struct {
	Connected bool   `json:"connected"`
	QRCode    string `json:"qr_code,omitempty"`
}

// QR returns the current QR code (data URL) while not logged in.
func (s *Service) QR(ctx context.Context) (QRResult, error) {
	if err := s.EnsureReady(ctx); err != nil {
		return QRResult{}, err
	}
	if st, err := s.gw.Status(ctx); err == nil && st.LoggedIn {
		s.syncLiveStatus(ctx, st)
		return QRResult{Connected: true}, nil
	}
	qr, err := s.gw.QR(ctx)
	if err != nil {
		return QRResult{}, err
	}
	return QRResult{QRCode: qr}, nil
}

// PairPhone returns a linking code for phone-number pairing.
func (s *Service) PairPhone(ctx context.Context, rawPhone string) (string, error) {
	n, err := phone.Parse(rawPhone, "")
	if err != nil {
		return "", fmt.Errorf("%w: phone", ErrInvalidRequest)
	}
	if err := s.EnsureReady(ctx); err != nil {
		return "", err
	}
	return s.gw.PairPhone(ctx, n.E164)
}

// Logout ends the WhatsApp session (a new QR scan is required).
func (s *Service) Logout(ctx context.Context) error {
	if err := s.EnsureReady(ctx); err != nil {
		return err
	}
	if err := s.gw.Logout(ctx); err != nil && !errors.Is(err, whatsapp.ErrNotConnected) {
		return err
	}
	now := s.now()
	if _, err := s.q.UpdateWhatsAppStatus(ctx, db.UpdateWhatsAppStatusParams{
		Status: whatsapp.StateLoggedOut, At: ts(now), LastErrorReason: text("admin_logout"),
	}); err != nil {
		return err
	}
	_, err := s.q.InsertWhatsAppConnectionEvent(ctx, db.InsertWhatsAppConnectionEventParams{
		Type: whatsapp.StateLoggedOut, Reason: text("admin_logout"), Alarm: false,
	})
	return err
}

// TestMessageResult is the outcome of a test send.
type TestMessageResult struct {
	MessageID string `json:"message_id"`
	To        string `json:"to"`
}

// SendTestMessage sends a text through WhatsApp and records it as an
// outbound staff message.
func (s *Service) SendTestMessage(ctx context.Context, rawPhone, body string) (TestMessageResult, error) {
	n, err := phone.Parse(rawPhone, "")
	if err != nil {
		return TestMessageResult{}, fmt.Errorf("%w: phone", ErrInvalidRequest)
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 4096 {
		return TestMessageResult{}, fmt.Errorf("%w: body", ErrInvalidRequest)
	}
	if err := s.EnsureReady(ctx); err != nil {
		return TestMessageResult{}, err
	}
	msgID := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	ref, err := s.gw.SendText(ctx, n.E164, body, whatsapp.SendOptions{ID: msgID})
	if err != nil {
		return TestMessageResult{}, err
	}
	if ref.ID != "" {
		ev := whatsapp.InboundEvent{
			Kind: whatsapp.KindMessage, ExternalID: ref.ID, To: n.E164, FromMe: true,
			Timestamp: ref.Timestamp, Text: body,
		}
		if _, err := s.storeMessage(ctx, ev, model.SenderStaff); err != nil {
			s.log.Warn("whatsapp_test_message_store_failed", "error", err)
		}
	}
	return TestMessageResult{MessageID: ref.ID, To: n.E164}, nil
}

// SettingsInput updates the SMS fallback switch and KVKK texts.
type SettingsInput struct {
	SMSFallbackEnabled *bool             `json:"sms_fallback_enabled"`
	KVKK               map[string]string `json:"kvkk_notices"`
}

// UpdateSettings applies admin settings. A changed KVKK text becomes a new
// version; unchanged texts keep their version.
func (s *Service) UpdateSettings(ctx context.Context, in SettingsInput, actorID *int64) error {
	current, err := s.KVKKNotices(ctx)
	if err != nil {
		return err
	}
	latest := map[string]string{}
	for _, n := range current {
		latest[n.Locale] = n.Body
	}
	for locale, body := range in.KVKK {
		if !validKVKKLocale(locale) {
			return fmt.Errorf("%w: kvkk locale %q", ErrInvalidRequest, locale)
		}
		body = strings.TrimSpace(body)
		if body == "" || len(body) > 2000 {
			return fmt.Errorf("%w: kvkk text for %s must be 1-2000 characters", ErrInvalidRequest, locale)
		}
		if body == latest[locale] {
			continue
		}
		var by pgtype.Int8
		if actorID != nil {
			by = pgtype.Int8{Int64: *actorID, Valid: true}
		}
		if _, err := s.q.InsertKVKKNotice(ctx, db.InsertKVKKNoticeParams{Locale: locale, Body: body, CreatedBy: by}); err != nil {
			return err
		}
	}
	if in.SMSFallbackEnabled != nil {
		if _, err := s.q.SetWhatsAppSMSFallback(ctx, *in.SMSFallbackEnabled); err != nil {
			return err
		}
	}
	return nil
}

// KVKKNotices lists the latest notice per locale.
func (s *Service) KVKKNotices(ctx context.Context) ([]KVKKNotice, error) {
	rows, err := s.q.ListLatestKVKKNotices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]KVKKNotice, 0, len(rows))
	for _, r := range rows {
		out = append(out, KVKKNotice{Locale: r.Locale, Version: r.Version, Body: r.Body, UpdatedAt: r.CreatedAt.Time})
	}
	return out, nil
}

// SMSFallbackEnabled reports the admin switch (OTP Sender).
func (s *Service) SMSFallbackEnabled(ctx context.Context) bool {
	st, err := s.q.GetWhatsAppSettings(ctx)
	return err == nil && st.SmsFallbackEnabled
}

func validKVKKLocale(l string) bool {
	for _, k := range KVKKLocales {
		if k == l {
			return true
		}
	}
	return false
}

func text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func rawJSON(b []byte) []byte {
	if len(b) == 0 || !json.Valid(b) {
		return nil
	}
	return b
}

// readyProvider makes sure the instance token is loaded before a send.
type readyProvider struct{ s *Service }

func (p readyProvider) Name() string { return wuzapi.ProviderName }

func (p readyProvider) SendText(ctx context.Context, to, body string, opts whatsapp.SendOptions) (whatsapp.MsgRef, error) {
	if err := p.s.EnsureReady(ctx); err != nil {
		return whatsapp.MsgRef{}, err
	}
	return p.s.gw.SendText(ctx, to, body, opts)
}

func (p readyProvider) SendDocument(ctx context.Context, to string, doc whatsapp.Media, opts whatsapp.SendOptions) (whatsapp.MsgRef, error) {
	if err := p.s.EnsureReady(ctx); err != nil {
		return whatsapp.MsgRef{}, err
	}
	return p.s.gw.SendDocument(ctx, to, doc, opts)
}

func (p readyProvider) SendImage(ctx context.Context, to string, img whatsapp.Media, opts whatsapp.SendOptions) (whatsapp.MsgRef, error) {
	if err := p.s.EnsureReady(ctx); err != nil {
		return whatsapp.MsgRef{}, err
	}
	return p.s.gw.SendImage(ctx, to, img, opts)
}

func (p readyProvider) DownloadMedia(ctx context.Context, m whatsapp.InboundMedia, max int64) ([]byte, error) {
	if err := p.s.EnsureReady(ctx); err != nil {
		return nil, err
	}
	d, ok := p.s.gw.(whatsapp.MediaDownloader)
	if !ok {
		return nil, ErrNotConfigured
	}
	return d.DownloadMedia(ctx, m, max)
}

func (p readyProvider) ParseWebhook(header http.Header, body []byte) ([]whatsapp.InboundEvent, error) {
	_ = p.s.EnsureReady(context.Background())
	return p.s.gw.ParseWebhook(header, body)
}

func (p readyProvider) Status(ctx context.Context) (whatsapp.ConnState, error) {
	if err := p.s.EnsureReady(ctx); err != nil {
		return whatsapp.ConnState{}, err
	}
	return p.s.gw.Status(ctx)
}
