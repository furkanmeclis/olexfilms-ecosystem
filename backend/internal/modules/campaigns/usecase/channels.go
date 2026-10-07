package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	whatsappmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	whatsappusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Channel adapters of the sender over the existing platform paths (TEC-407).

// NotificationPush sends campaign pushes with the notification center's
// web push (VAPID) and Expo providers; nil providers are not used.
type NotificationPush struct {
	Web  providers.Provider
	Expo providers.Provider
}

// SendCampaignPush delivers to every browser subscription and device of the
// user. ErrNoAddress when neither has one; success when one channel sent.
func (p NotificationPush) SendCampaignPush(ctx context.Context, m PushMessage) error {
	n := db.Notification{
		Uuid: uuid.New(), UserID: pgtype.Int8{Int64: m.UserID, Valid: true}, Priority: "normal",
		Title: m.Title, Body: m.Body, Language: pgtype.Text{String: m.Locale, Valid: true},
		BrandID: pgtype.Int8{Int64: m.BrandID, Valid: m.BrandID > 0},
	}
	if m.Deeplink != "" {
		n.ActionUrl = pgtype.Text{String: m.Deeplink, Valid: true}
	}
	if m.ImageURL != "" {
		n.Payload, _ = json.Marshal(map[string]string{"image_url": m.ImageURL})
	}
	var lastErr error
	sent := false
	for _, prov := range []providers.Provider{p.Web, p.Expo} {
		if prov == nil {
			continue
		}
		_, err := prov.Deliver(ctx, n, nil)
		switch {
		case err == nil:
			sent = true
		case errors.Is(err, providers.ErrNoRecipient):
		default:
			lastErr = err
		}
	}
	if sent {
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return ErrNoAddress
}

// MailEmail sends campaign e-mails in the branded notification layout with
// the details and unsubscribe links appended in the recipient's language.
type MailEmail struct {
	Mail  mail.Sender
	Brand func(ctx context.Context, brandID int64) providers.EmailBrand
}

// SendCampaignEmail renders and sends one e-mail.
func (e MailEmail) SendCampaignEmail(ctx context.Context, m EmailMessage) error {
	if e.Mail == nil {
		return &PermanentError{Err: errors.New("campaigns: mail sender is not configured")}
	}
	body := EmailBody(m)
	lang := i18n.Normalize(m.Locale)
	brand := providers.EmailBrand{Name: "Olex Films"}
	if e.Brand != nil {
		if b := e.Brand(ctx, m.BrandID); b.Name != "" {
			brand = b
		}
	}
	html, err := mail.Layout{
		Lang: string(lang), Dir: i18n.Dir(lang), Title: m.Subject,
		BrandName: brand.Name, LogoURL: brand.LogoURL, Color: brand.Color,
		BodyHTML: msgtemplate.MarkdownToHTML(body),
	}.RenderHTML()
	if err != nil {
		return &PermanentError{Err: err}
	}
	return e.Mail.Send(ctx, mail.Message{
		To: []string{m.To}, Subject: m.Subject, Body: msgtemplate.MarkdownToText(body), HTMLBody: html,
	})
}

// EmailBody is the markdown of a campaign e-mail: the content, the
// details link and the unsubscribe paragraph.
func EmailBody(m EmailMessage) string {
	t := emailTextsFor(m.Locale)
	var b strings.Builder
	b.WriteString(strings.TrimSpace(m.Body))
	if m.Deeplink != "" {
		b.WriteString("\n\n[" + t.details + "](" + m.Deeplink + ")")
	}
	if m.UnsubscribeURL != "" {
		b.WriteString("\n\n" + t.unsubscribeIntro + " [" + t.unsubscribe + "](" + m.UnsubscribeURL + ")")
	}
	return b.String()
}

// ConversationWhatsApp hands campaign messages to the WhatsApp outgoing
// queue (F4-02d): the user's conversation is opened (or reused) and each
// part is queued as a system message; the send task applies the per
// number limit and the retries.
type ConversationWhatsApp struct {
	Queries   *db.Queries
	Messaging *whatsappusecase.Messaging
}

// SendCampaignWhatsApp queues the text (as the caption of the first file)
// and every further file.
func (w ConversationWhatsApp) SendCampaignWhatsApp(ctx context.Context, m WhatsAppMessage) error {
	if w.Messaging == nil || w.Queries == nil {
		return &PermanentError{Err: errors.New("campaigns: whatsapp messaging is not configured")}
	}
	var name pgtype.Text
	if strings.TrimSpace(m.Name) != "" {
		name = pgtype.Text{String: strings.TrimSpace(m.Name), Valid: true}
	}
	conv, err := w.Queries.UpsertConversation(ctx, db.UpsertConversationParams{
		Channel: whatsapp.ChannelWhatsApp, ContactE164: m.Phone, ContactName: name,
		UserID: pgtype.Int8{Int64: m.UserID, Valid: true},
	})
	if err != nil {
		return err
	}
	parts := []whatsappusecase.OutgoingMessage{}
	if len(m.Media) == 0 {
		parts = append(parts, whatsappusecase.OutgoingMessage{Body: m.Body})
	}
	for i, md := range m.Media {
		part := whatsappusecase.OutgoingMessage{Media: &whatsappusecase.OutgoingMedia{Data: md.Data, FileName: md.FileName}}
		if i == 0 {
			part.Body = m.Body
		}
		parts = append(parts, part)
	}
	for _, part := range parts {
		part.ConversationID = conv.ID
		part.SenderType = whatsappmodel.SenderSystem
		if _, err := w.Messaging.Queue(ctx, part); err != nil {
			if errors.Is(err, whatsappusecase.ErrInvalidRequest) || errors.Is(err, whatsappusecase.ErrNotConfigured) {
				return &PermanentError{Err: err}
			}
			return err
		}
	}
	return nil
}
