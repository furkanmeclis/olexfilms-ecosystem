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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
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
	// Unresolved counts inbound messages whose sender arrived as a bare
	// linked id the backend has no phone for yet (TEC-623).
	Unresolved int
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
			ev, ok := s.resolveContactLID(ctx, ev)
			if !ok {
				res.Unresolved++
				continue
			}
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
			s.learnLIDFromReceipt(ctx, ev)
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

// resolveContactLID (TEC-623) keeps the LID -> phone map from events that
// carry both and fills From from the map when an inbound message arrives
// with the linked id alone (whatsmeow omits SenderAlt for a privacy-mode
// contact it has not matched yet). It returns false when the sender stays
// unknown; the message is then skipped and logged instead of dropped
// silently.
func (s *Service) resolveContactLID(ctx context.Context, ev whatsapp.InboundEvent) (whatsapp.InboundEvent, bool) {
	lid := strings.TrimSpace(ev.ContactLID)
	if lid == "" {
		return ev, true
	}
	phone := ev.From
	if ev.FromMe {
		phone = ev.To
	}
	if phone != "" {
		if err := s.q.UpsertWhatsAppLID(ctx, db.UpsertWhatsAppLIDParams{Lid: lid, PhoneE164: phone}); err != nil {
			s.log.Warn("whatsapp_lid_map_write_failed", "lid", lid, "error", err)
		}
		return ev, true
	}
	if ev.FromMe {
		return ev, true
	}
	known, err := s.q.GetWhatsAppLIDPhone(ctx, lid)
	if err != nil || known == "" {
		s.log.Warn("whatsapp_inbound_lid_unresolved", "lid", lid, "external_id", ev.ExternalID, "push_name", ev.PushName)
		return ev, false
	}
	ev.From = known
	return ev, true
}

// learnLIDFromReceipt (TEC-623) pairs a receipt's linked-id chat with the
// phone of the message it acknowledges: messages sent through the API get
// no outbound Message event, so the receipt is the first place the LID of
// a privacy-mode contact shows up next to something we know the phone of.
func (s *Service) learnLIDFromReceipt(ctx context.Context, ev whatsapp.InboundEvent) {
	lid := strings.TrimSpace(ev.ContactLID)
	if lid == "" || len(ev.MessageIDs) == 0 {
		return
	}
	// Same transaction scope as the message writes (the receipt may follow
	// the message in one webhook batch).
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	phone := ev.From
	if phone == "" {
		known, err := q.GetWhatsAppContactByExternalIDs(ctx, ev.MessageIDs)
		if err != nil || known == "" {
			return
		}
		phone = known
	}
	if err := q.UpsertWhatsAppLID(ctx, db.UpsertWhatsAppLIDParams{Lid: lid, PhoneE164: phone}); err != nil {
		s.log.Warn("whatsapp_lid_map_write_failed", "lid", lid, "error", err)
		return
	}
	_ = tx.Commit(ctx)
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
		// TEC-398: the assigned user is notified (conversation.inbound).
		if conv.AssignedUserID.Valid {
			e.Payload["assigned_user_id"] = conv.AssignedUserID.Int64
			e.Payload["contact_name"] = contactLabel(conv)
			e.Payload["preview"] = messagePreview(ev.Text, ev.Media != nil)
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

// alarmTexts holds the connection alarm per supported locale (TEC-1233);
// %s is the reason reported by the provider.
var alarmTexts = map[string]map[string][2]string{
	"tr": {
		whatsapp.StateLoggedOut: {"WhatsApp bağlantısı koptu", "WhatsApp numarasının oturumu kapandı (%s). OTP ve bildirimler gönderilemiyor; Entegrasyonlar > WhatsApp ekranından QR ile yeniden bağlayın."},
		whatsapp.StateBanned:    {"WhatsApp numarası geçici olarak engellendi", "WhatsApp numarası geçici olarak engellendi (%s). Gönderimler durdu; SMS yedeğini açmayı değerlendirin."},
	},
	"en": {
		whatsapp.StateLoggedOut: {"WhatsApp disconnected", "The WhatsApp number was logged out (%s). OTPs and notifications cannot be sent; reconnect with a QR code under Integrations > WhatsApp."},
		whatsapp.StateBanned:    {"WhatsApp number temporarily banned", "The WhatsApp number is temporarily banned (%s). Sending stopped; consider enabling the SMS fallback."},
	},
	"bg": {
		whatsapp.StateLoggedOut: {"Връзката с WhatsApp е прекъсната", "Сесията на номера в WhatsApp е прекратена (%s). Не могат да се изпращат OTP кодове и известия; свържете отново с QR код от Интеграции > WhatsApp."},
		whatsapp.StateBanned:    {"Номерът в WhatsApp е временно блокиран", "Номерът в WhatsApp е временно блокиран (%s). Изпращането е спряно; помислете за включване на резервния SMS канал."},
	},
	"de": {
		whatsapp.StateLoggedOut: {"WhatsApp-Verbindung getrennt", "Die WhatsApp-Nummer wurde abgemeldet (%s). OTP-Codes und Benachrichtigungen können nicht gesendet werden; verbinden Sie sie unter Integrationen > WhatsApp per QR-Code erneut."},
		whatsapp.StateBanned:    {"WhatsApp-Nummer vorübergehend gesperrt", "Die WhatsApp-Nummer ist vorübergehend gesperrt (%s). Der Versand wurde gestoppt; erwägen Sie, den SMS-Fallback zu aktivieren."},
	},
	"el": {
		whatsapp.StateLoggedOut: {"Η σύνδεση WhatsApp διακόπηκε", "Έγινε αποσύνδεση του αριθμού WhatsApp (%s). Δεν είναι δυνατή η αποστολή κωδικών OTP και ειδοποιήσεων· συνδεθείτε ξανά με κωδικό QR από Ενσωματώσεις > WhatsApp."},
		whatsapp.StateBanned:    {"Ο αριθμός WhatsApp αποκλείστηκε προσωρινά", "Ο αριθμός WhatsApp έχει αποκλειστεί προσωρινά (%s). Οι αποστολές σταμάτησαν· εξετάστε την ενεργοποίηση της εφεδρικής αποστολής SMS."},
	},
	"uk": {
		whatsapp.StateLoggedOut: {"З’єднання з WhatsApp розірвано", "Сеанс номера WhatsApp завершено (%s). Неможливо надсилати OTP-коди та сповіщення; підключіть номер повторно за QR-кодом у розділі Інтеграції > WhatsApp."},
		whatsapp.StateBanned:    {"Номер WhatsApp тимчасово заблоковано", "Номер WhatsApp тимчасово заблоковано (%s). Надсилання зупинено; розгляньте можливість увімкнути резервне надсилання SMS."},
	},
	"ru": {
		whatsapp.StateLoggedOut: {"Соединение с WhatsApp разорвано", "Сеанс номера WhatsApp завершён (%s). Невозможно отправлять OTP-коды и уведомления; подключите номер заново по QR-коду в разделе Интеграции > WhatsApp."},
		whatsapp.StateBanned:    {"Номер WhatsApp временно заблокирован", "Номер WhatsApp временно заблокирован (%s). Отправка остановлена; рассмотрите возможность включить резервную отправку SMS."},
	},
	"fr": {
		whatsapp.StateLoggedOut: {"WhatsApp déconnecté", "Le numéro WhatsApp a été déconnecté (%s). Les codes OTP et les notifications ne peuvent pas être envoyés ; reconnectez-le avec un code QR dans Intégrations > WhatsApp."},
		whatsapp.StateBanned:    {"Numéro WhatsApp temporairement bloqué", "Le numéro WhatsApp est temporairement bloqué (%s). Les envois sont arrêtés ; envisagez d'activer le secours par SMS."},
	},
	"es": {
		whatsapp.StateLoggedOut: {"WhatsApp desconectado", "Se cerró la sesión del número de WhatsApp (%s). No se pueden enviar códigos OTP ni notificaciones; vuelva a vincularlo con un código QR en Integraciones > WhatsApp."},
		whatsapp.StateBanned:    {"Número de WhatsApp bloqueado temporalmente", "El número de WhatsApp está bloqueado temporalmente (%s). Los envíos se han detenido; considere activar el respaldo por SMS."},
	},
	"it": {
		whatsapp.StateLoggedOut: {"WhatsApp disconnesso", "La sessione del numero WhatsApp è stata chiusa (%s). Non è possibile inviare codici OTP e notifiche; ricolleghi il numero con un codice QR da Integrazioni > WhatsApp."},
		whatsapp.StateBanned:    {"Numero WhatsApp temporaneamente bloccato", "Il numero WhatsApp è temporaneamente bloccato (%s). Gli invii sono stati interrotti; valuti di attivare il fallback via SMS."},
	},
	"zh-CN": {
		whatsapp.StateLoggedOut: {"WhatsApp 连接已断开", "WhatsApp 号码已退出登录（%s）。无法发送 OTP 验证码和通知；请在 集成 > WhatsApp 中扫描二维码重新连接。"},
		whatsapp.StateBanned:    {"WhatsApp 号码已被临时封禁", "WhatsApp 号码已被临时封禁（%s）。发送已停止；请考虑启用短信备用通道。"},
	},
	"az": {
		whatsapp.StateLoggedOut: {"WhatsApp bağlantısı kəsildi", "WhatsApp nömrəsinin sessiyası bağlandı (%s). OTP kodları və bildirişlər göndərilə bilmir; İnteqrasiyalar > WhatsApp bölməsindən QR kodla yenidən qoşun."},
		whatsapp.StateBanned:    {"WhatsApp nömrəsi müvəqqəti bloklandı", "WhatsApp nömrəsi müvəqqəti bloklanıb (%s). Göndərişlər dayandı; SMS ehtiyat kanalını aktivləşdirməyi nəzərdən keçirin."},
	},
	"ar": {
		whatsapp.StateLoggedOut: {"انقطع اتصال WhatsApp", "تم تسجيل خروج رقم WhatsApp (%s). لا يمكن إرسال رموز OTP والإشعارات؛ أعد الربط برمز QR من التكاملات > WhatsApp."},
		whatsapp.StateBanned:    {"تم حظر رقم WhatsApp مؤقتًا", "رقم WhatsApp محظور مؤقتًا (%s). توقف الإرسال؛ فكّر في تفعيل الإرسال الاحتياطي عبر SMS."},
	},
}

// alarmLocale picks the alarm language of a recipient: the stored locale,
// tr when empty, en when there is no text for it.
func alarmLocale(stored string) string {
	if stored == "" {
		return string(i18n.DefaultLocale)
	}
	if _, ok := alarmTexts[stored]; ok {
		return stored
	}
	if l, ok := i18n.Parse(stored); ok {
		if _, ok := alarmTexts[string(l)]; ok {
			return string(l)
		}
	}
	return string(i18n.FallbackLocale)
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
		lang := alarmLocale(r.Locale.String)
		t := alarmTexts[lang][state]
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

// contactLabel is the contact's name, else the number.
func contactLabel(c db.Conversation) string {
	if name := strings.TrimSpace(c.ContactName.String); name != "" {
		return name
	}
	return c.ContactE164
}

// previewRunes bounds the message preview of the inbound notification.
const previewRunes = 120

// messagePreview is the start of an inbound text ("📎" for a media message
// without a caption).
func messagePreview(body string, hasMedia bool) string {
	body = strings.Join(strings.Fields(body), " ")
	if body == "" && hasMedia {
		return "📎"
	}
	if r := []rune(body); len(r) > previewRunes {
		return string(r[:previewRunes-1]) + "…"
	}
	return body
}
