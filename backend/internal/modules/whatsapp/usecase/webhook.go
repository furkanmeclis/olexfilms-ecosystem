package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WebhookResult summarizes one webhook delivery.
type WebhookResult struct {
	Messages    int
	Duplicates  int
	Statuses    int
	Connections int
	Alarms      int
}

// HandleWebhook verifies and applies a raw wuzapi webhook body. It returns
// whatsapp.ErrInvalidSignature for unsigned or forged requests.
func (s *Service) HandleWebhook(ctx context.Context, header http.Header, body []byte) (WebhookResult, error) {
	if s.gw == nil {
		return WebhookResult{}, ErrNotConfigured
	}
	// Load the instance token (token cross-check) without failing the hook.
	if err := s.EnsureReady(ctx); err != nil && !errors.Is(err, ErrNotConfigured) {
		s.log.Warn("whatsapp_webhook_not_ready", "error", err)
	}
	evs, err := s.gw.ParseWebhook(header, body)
	if err != nil {
		return WebhookResult{}, err
	}
	var res WebhookResult
	for _, ev := range evs {
		switch ev.Kind {
		case whatsapp.KindMessage:
			stored, err := s.storeMessage(ctx, ev, "contact")
			if err != nil {
				return res, err
			}
			if stored != nil {
				res.Messages++
				if s.msgs != nil {
					s.msgs.AfterStored(ctx, stored.conv, stored.msg)
				}
			} else {
				res.Duplicates++
			}
		case whatsapp.KindStatus:
			// TEC-395: forward-only delivered/read + delivery_status_at.
			if s.msgs != nil {
				if _, err := s.msgs.ApplyReceipt(ctx, ev); err != nil {
					return res, err
				}
			} else if _, err := applyReceipt(ctx, s.q, ev, s.now()); err != nil {
				return res, err
			}
			res.Statuses++
		case whatsapp.KindConnection:
			alarm, err := s.applyConnection(ctx, ev.Connection, ev.Type, ev.Reason, ev.JID, ev.Raw)
			if err != nil {
				return res, err
			}
			res.Connections++
			if alarm {
				res.Alarms++
			}
		}
	}
	return res, nil
}

// storedMessage is a message newly stored by storeMessage.
type storedMessage struct {
	conv db.Conversation
	msg  db.Message
}

// storeMessage upserts the conversation and inserts the message once
// (UNIQUE(channel, external_id) + ON CONFLICT DO NOTHING). An inbound
// message also writes whatsapp.message.received to the outbox in the same
// transaction. It returns nil for a duplicate or an unusable event.
func (s *Service) storeMessage(ctx context.Context, ev whatsapp.InboundEvent, senderType string) (*storedMessage, error) {
	contact := ev.From
	direction := "in"
	status := "received"
	if ev.FromMe {
		contact, direction, status = ev.To, "out", "sent"
		if senderType == "contact" {
			senderType = "staff"
		}
	}
	if contact == "" || ev.ExternalID == "" {
		return nil, nil
	}
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	var userID pgtype.Int8
	if u, err := q.GetUserByPhone(ctx, text(contact)); err == nil {
		userID = pgtype.Int8{Int64: u.ID, Valid: true}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	at := ev.Timestamp
	if at.IsZero() {
		at = s.now()
	}
	conv, err := q.UpsertConversation(ctx, db.UpsertConversationParams{
		Channel: whatsapp.ChannelWhatsApp, ContactE164: contact, ContactName: text(ev.PushName),
		UserID: userID, LastMessageAt: ts(at),
	})
	if err != nil {
		return nil, err
	}
	var media []byte
	if ev.Media != nil {
		media, _ = json.Marshal(ev.Media)
	}
	msg, err := q.InsertMessage(ctx, db.InsertMessageParams{
		ConversationID: conv.ID, OrganizationID: conv.OrganizationID, BrandID: conv.BrandID,
		Channel: whatsapp.ChannelWhatsApp, Direction: direction, SenderType: senderType,
		ExternalID: ev.ExternalID, Body: text(ev.Text), Media: media, Status: status,
		Raw: rawJSON(ev.Raw), SentAt: ts(at),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // duplicate delivery
	}
	if err != nil {
		return nil, err
	}
	// TEC-393: inbox counters only for a newly stored message.
	if direction == "in" {
		conv, err = q.TouchConversationInbound(ctx, db.TouchConversationInboundParams{ID: conv.ID, At: ts(at)})
	} else {
		conv, err = q.TouchConversationOutbound(ctx, db.TouchConversationOutboundParams{ID: conv.ID, At: ts(at)})
	}
	if err != nil {
		return nil, err
	}
	if direction == "in" && s.outbox != nil {
		e := events.New(events.WhatsAppMessageReceived)
		e.EntityType = "message"
		e.EntityID = &msg.ID
		e.EntityUUID = &msg.Uuid
		e.Payload = map[string]any{
			"conversation_uuid": conv.Uuid.String(),
			"message_uuid":      msg.Uuid.String(),
			"external_id":       ev.ExternalID,
			"from":              contact,
			"has_media":         ev.Media != nil,
		}
		if userID.Valid {
			e.Payload["user_id"] = userID.Int64
		}
		if err := s.outbox.Enqueue(ctx, tx, e); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &storedMessage{conv: conv, msg: msg}, nil
}

func isAlarm(state string) bool {
	return state == whatsapp.StateLoggedOut || state == whatsapp.StateBanned
}

// applyConnection stores a connection state change, its history row and,
// for logout/ban, an alarm to the WhatsApp admins.
func (s *Service) applyConnection(ctx context.Context, state, rawType, reason, jid string, raw []byte) (bool, error) {
	if state == "" {
		return false, nil
	}
	params := db.UpdateWhatsAppStatusParams{Status: state, At: ts(s.now()), LastErrorReason: text(reason)}
	if state == whatsapp.StateConnected {
		params.LastErrorReason = pgtype.Text{}
		if jid != "" {
			params.Jid = text(jid)
			if n, err := phone.FromDigits(jid); err == nil {
				params.PhoneE164 = text(n.E164)
			}
		}
	}
	if _, err := s.q.UpdateWhatsAppStatus(ctx, params); err != nil {
		return false, err
	}
	alarm := isAlarm(state)
	evType := state
	if rawType != "" {
		evType = strings.ToLower(rawType)
	}
	if _, err := s.q.InsertWhatsAppConnectionEvent(ctx, db.InsertWhatsAppConnectionEventParams{
		Type: evType, Reason: text(reason), Alarm: alarm, Raw: rawJSON(raw),
	}); err != nil {
		return false, err
	}
	if alarm {
		s.notifyAlarm(ctx, state, reason)
	}
	return alarm, nil
}

var alarmTexts = map[string]map[string][2]string{
	"tr": {
		whatsapp.StateLoggedOut: {"WhatsApp bağlantısı koptu", "WhatsApp numarasının oturumu kapandı (%s). OTP ve bildirimler gönderilemiyor; Entegrasyonlar > WhatsApp ekranından QR ile yeniden bağlayın."},
		whatsapp.StateBanned:    {"WhatsApp numarası geçici olarak engellendi", "WhatsApp numarası geçici olarak engellendi (%s). Gönderimler durdu; SMS yedeğini açmayı değerlendirin."},
	},
	"en": {
		whatsapp.StateLoggedOut: {"WhatsApp disconnected", "The WhatsApp number was logged out (%s). OTPs and notifications cannot be sent; reconnect with a QR code under Integrations > WhatsApp."},
		whatsapp.StateBanned:    {"WhatsApp number temporarily banned", "The WhatsApp number is temporarily banned (%s). Sending stopped; consider enabling the SMS fallback."},
	},
}

func (s *Service) notifyAlarm(ctx context.Context, state, reason string) {
	if s.notifier == nil {
		return
	}
	recipients, err := s.q.ListWhatsAppAlarmRecipients(ctx)
	if err != nil {
		s.log.Warn("whatsapp_alarm_recipients_failed", "error", err)
		return
	}
	if reason == "" {
		reason = state
	}
	actionURL := "/platform/integrations/whatsapp"
	for _, r := range recipients {
		lang := r.Locale.String
		if lang == "" {
			lang = "tr"
		}
		texts, ok := alarmTexts[lang]
		if !ok {
			lang, texts = "en", alarmTexts["en"]
		}
		t := texts[state]
		uid := r.ID
		if _, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			UserID:      &uid,
			Channels:    []string{notifmodel.ChannelInapp, notifmodel.ChannelEmail},
			Priority:    notifmodel.PriorityCritical,
			Title:       t[0],
			Body:        fmt.Sprintf(t[1], reason),
			ActionURL:   &actionURL,
			SourceEvent: events.WhatsAppConnectionAlarm,
			Language:    lang,
			Payload:     map[string]any{"state": state, "reason": reason},
		}); err != nil {
			s.log.Warn("whatsapp_alarm_notify_failed", "user", r.ID, "error", err)
		}
	}
}

// PollStatus is the 1-minute worker job: refresh the status, reconnect a
// dropped websocket, and raise an alarm when a linked session is gone.
func (s *Service) PollStatus(ctx context.Context) error {
	if err := s.EnsureReady(ctx); err != nil {
		if errors.Is(err, ErrNotConfigured) {
			return nil
		}
		return err
	}
	prev, err := s.q.GetWhatsAppSettings(ctx)
	if err != nil {
		return err
	}
	live, err := s.gw.Status(ctx)
	if err != nil {
		return err
	}
	switch {
	case live.Connected && live.LoggedIn:
		if prev.Status != whatsapp.StateConnected {
			_, err = s.applyConnection(ctx, whatsapp.StateConnected, "poll_connected", "", live.JID, nil)
		} else {
			_, err = s.q.UpdateWhatsAppStatus(ctx, db.UpdateWhatsAppStatusParams{
				Status: whatsapp.StateConnected, At: ts(s.now()),
			})
		}
		return err
	case prev.Status == whatsapp.StateConnected && live.Connected && !live.LoggedIn:
		// The session was unlinked on the phone.
		_, err = s.applyConnection(ctx, whatsapp.StateLoggedOut, "poll_logged_out", "session_not_logged_in", "", nil)
		return err
	case !live.Connected && prev.Jid.Valid && prev.Status != whatsapp.StateLoggedOut && prev.Status != whatsapp.StateBanned:
		// Websocket dropped on a linked device: reconnect.
		if prev.Status == whatsapp.StateConnected {
			if _, err := s.applyConnection(ctx, whatsapp.StateDisconnected, "poll_disconnected", "websocket_down", "", nil); err != nil {
				return err
			}
		}
		if err := s.gw.Connect(ctx); err != nil {
			s.log.Warn("whatsapp_reconnect_failed", "error", err)
		}
	}
	return nil
}
