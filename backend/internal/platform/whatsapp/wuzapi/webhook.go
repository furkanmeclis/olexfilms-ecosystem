package wuzapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// SignatureHeader carries hex(HMAC-SHA256(key, raw body)).
const SignatureHeader = "x-hmac-signature"

// Sign returns the signature wuzapi would send for body (tests, tooling).
func Sign(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks the x-hmac-signature header against the raw body.
// An empty key refuses every request (fail closed).
func VerifySignature(key string, header http.Header, body []byte) error {
	if key == "" {
		return whatsapp.ErrInvalidSignature
	}
	got, err := hex.DecodeString(strings.TrimSpace(header.Get(SignatureHeader)))
	if err != nil || len(got) == 0 {
		return whatsapp.ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return whatsapp.ErrInvalidSignature
	}
	return nil
}

type webhookPayload struct {
	Type         string          `json:"type"`
	Event        json.RawMessage `json:"event"`
	Token        string          `json:"token"`
	State        string          `json:"state"`
	UserID       string          `json:"userID"`
	InstanceName string          `json:"instanceName"`
	MimeType     string          `json:"mimeType"`
	FileName     string          `json:"fileName"`
	S3           *struct {
		URL      string `json:"url"`
		MimeType string `json:"mimeType"`
		FileName string `json:"fileName"`
	} `json:"s3"`
}

type messageInfo struct {
	ID           string    `json:"ID"`
	Chat         string    `json:"Chat"`
	Sender       string    `json:"Sender"`
	SenderAlt    string    `json:"SenderAlt"`
	RecipientAlt string    `json:"RecipientAlt"`
	IsFromMe     bool      `json:"IsFromMe"`
	IsGroup      bool      `json:"IsGroup"`
	PushName     string    `json:"PushName"`
	Timestamp    time.Time `json:"Timestamp"`
	Type         string    `json:"Type"`
	MediaType    string    `json:"MediaType"`
}

type mediaMessage struct {
	Caption  string `json:"caption"`
	Mimetype string `json:"mimetype"`
	FileName string `json:"fileName"`
	Title    string `json:"title"`
	URL      string `json:"URL"`
}

type messageBody struct {
	Conversation        string `json:"conversation"`
	ExtendedTextMessage *struct {
		Text string `json:"text"`
	} `json:"extendedTextMessage"`
	ImageMessage    *mediaMessage `json:"imageMessage"`
	DocumentMessage *mediaMessage `json:"documentMessage"`
	AudioMessage    *mediaMessage `json:"audioMessage"`
	VideoMessage    *mediaMessage `json:"videoMessage"`
	StickerMessage  *mediaMessage `json:"stickerMessage"`
}

type messageEvent struct {
	Info    messageInfo `json:"Info"`
	Message messageBody `json:"Message"`
}

type receiptEvent struct {
	MessageIDs []string  `json:"MessageIDs"`
	Chat       string    `json:"Chat"`
	Sender     string    `json:"Sender"`
	Timestamp  time.Time `json:"Timestamp"`
	Type       string    `json:"Type"`
}

// ParseWebhook implements whatsapp.Provider: verify the signature over the
// raw body, then normalize. When the payload carries the instance token it
// must match the configured user token as well.
func (c *Client) ParseWebhook(header http.Header, body []byte) ([]whatsapp.InboundEvent, error) {
	if err := VerifySignature(c.cfg.HMACKey, header, body); err != nil {
		return nil, err
	}
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("wuzapi: webhook body: %w", err)
	}
	if want := c.UserToken(); want != "" && p.Token != "" &&
		subtle.ConstantTimeCompare([]byte(want), []byte(p.Token)) != 1 {
		return nil, whatsapp.ErrInvalidSignature
	}
	ev, ok, err := normalize(p)
	if err != nil || !ok {
		return nil, err
	}
	ev.Raw = append(json.RawMessage(nil), body...)
	return []whatsapp.InboundEvent{ev}, nil
}

func normalize(p webhookPayload) (whatsapp.InboundEvent, bool, error) {
	base := whatsapp.InboundEvent{Type: p.Type}
	switch p.Type {
	case "Message":
		var m messageEvent
		if err := json.Unmarshal(p.Event, &m); err != nil {
			return base, false, fmt.Errorf("wuzapi: message event: %w", err)
		}
		if m.Info.IsGroup || strings.HasSuffix(m.Info.Chat, "@broadcast") || m.Info.ID == "" {
			return base, false, nil
		}
		ev := base
		ev.Kind = whatsapp.KindMessage
		ev.ExternalID = m.Info.ID
		ev.FromMe = m.Info.IsFromMe
		ev.PushName = m.Info.PushName
		ev.Timestamp = m.Info.Timestamp
		contact := firstPhone(m.Info.SenderAlt, m.Info.Sender, m.Info.Chat, m.Info.RecipientAlt)
		if m.Info.IsFromMe {
			ev.To = firstPhone(m.Info.RecipientAlt, m.Info.Chat)
		} else {
			ev.From = contact
		}
		ev.Text, ev.Media = messageContent(m.Message)
		if ev.Media != nil {
			if p.S3 != nil && p.S3.URL != "" {
				ev.Media.URL = p.S3.URL
			}
			if ev.Media.MimeType == "" {
				ev.Media.MimeType = p.MimeType
			}
			if ev.Media.FileName == "" {
				ev.Media.FileName = p.FileName
			}
		}
		return ev, true, nil
	case "ReadReceipt":
		var r receiptEvent
		if err := json.Unmarshal(p.Event, &r); err != nil {
			return base, false, fmt.Errorf("wuzapi: receipt event: %w", err)
		}
		ev := base
		ev.Kind = whatsapp.KindStatus
		ev.MessageIDs = r.MessageIDs
		ev.Timestamp = r.Timestamp
		ev.From = firstPhone(r.Sender, r.Chat)
		state := strings.ToLower(p.State)
		switch {
		case state == "read" || r.Type == "read" || r.Type == "read-self":
			ev.Status = "read"
		default:
			ev.Status = "delivered"
		}
		return ev, len(r.MessageIDs) > 0, nil
	case "Connected", "PairSuccess":
		ev := base
		ev.Kind = whatsapp.KindConnection
		ev.Connection = whatsapp.StateConnected
		var pe struct {
			ID string `json:"ID"`
		}
		_ = json.Unmarshal(p.Event, &pe)
		ev.JID = pe.ID
		return ev, true, nil
	case "QR":
		ev := base
		ev.Kind = whatsapp.KindConnection
		ev.Connection = whatsapp.StateQR
		return ev, true, nil
	case "LoggedOut":
		var lo struct {
			OnConnect bool `json:"OnConnect"`
			Reason    any  `json:"Reason"`
		}
		_ = json.Unmarshal(p.Event, &lo)
		ev := base
		ev.Kind = whatsapp.KindConnection
		ev.Connection = whatsapp.StateLoggedOut
		ev.Reason = reasonString("logged_out", lo.Reason)
		return ev, true, nil
	case "TemporaryBan":
		var tb struct {
			Code   any `json:"Code"`
			Expire any `json:"Expire"`
		}
		_ = json.Unmarshal(p.Event, &tb)
		ev := base
		ev.Kind = whatsapp.KindConnection
		ev.Connection = whatsapp.StateBanned
		ev.Reason = reasonString("temporary_ban", tb.Code)
		if tb.Expire != nil {
			ev.Reason += fmt.Sprintf(" expire=%v", tb.Expire)
		}
		return ev, true, nil
	case "Disconnected", "StreamReplaced", "ConnectFailure", "ClientOutdated", "StreamError":
		var cf struct {
			Reason  any    `json:"Reason"`
			Message string `json:"Message"`
		}
		_ = json.Unmarshal(p.Event, &cf)
		ev := base
		ev.Kind = whatsapp.KindConnection
		ev.Connection = whatsapp.StateDisconnected
		ev.Reason = reasonString(strings.ToLower(p.Type), cf.Reason)
		if cf.Message != "" {
			ev.Reason += ": " + cf.Message
		}
		return ev, true, nil
	default:
		return base, false, nil
	}
}

func messageContent(m messageBody) (string, *whatsapp.InboundMedia) {
	if m.Conversation != "" {
		return m.Conversation, nil
	}
	if m.ExtendedTextMessage != nil && m.ExtendedTextMessage.Text != "" {
		return m.ExtendedTextMessage.Text, nil
	}
	pick := func(kind string, mm *mediaMessage) (string, *whatsapp.InboundMedia) {
		name := mm.FileName
		if name == "" {
			name = mm.Title
		}
		return mm.Caption, &whatsapp.InboundMedia{
			Type: kind, MimeType: mm.Mimetype, FileName: name, Caption: mm.Caption,
		}
	}
	switch {
	case m.ImageMessage != nil:
		return pick("image", m.ImageMessage)
	case m.DocumentMessage != nil:
		return pick("document", m.DocumentMessage)
	case m.AudioMessage != nil:
		return pick("audio", m.AudioMessage)
	case m.VideoMessage != nil:
		return pick("video", m.VideoMessage)
	case m.StickerMessage != nil:
		return pick("sticker", m.StickerMessage)
	}
	return "", nil
}

// firstPhone returns the first JID that resolves to a phone number. LID
// addresses (@lid) never do; whatsmeow then puts the phone JID in *Alt.
func firstPhone(jids ...string) string {
	for _, j := range jids {
		if j == "" || strings.HasSuffix(j, "@lid") || strings.HasSuffix(j, "@g.us") {
			continue
		}
		if n, err := phone.FromDigits(j); err == nil {
			return n.E164
		}
	}
	return ""
}

func reasonString(prefix string, v any) string {
	if v == nil {
		return prefix
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "0" {
		return prefix
	}
	return prefix + ":" + s
}
