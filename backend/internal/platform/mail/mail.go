package mail

import (
	"context"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
)

// Message is an outbound email.
type Message struct {
	To      []string
	Subject string
	Body    string
	// HTMLBody, when set, is sent as the text/html part of a
	// multipart/alternative message next to the plain-text Body.
	HTMLBody string
}

// Sender delivers email messages.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// SMTPSender sends mail via SMTP (MailHog in development).
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
	from     string
	fromName string
}

// NewSMTPSender builds a sender from config.
func NewSMTPSender(cfg config.SMTPConfig) *SMTPSender {
	return &SMTPSender{
		host:     cfg.Host,
		port:     cfg.Port,
		username: cfg.Username,
		password: cfg.Password,
		from:     cfg.From,
		fromName: cfg.FromName,
	}
}

// Send delivers a plain-text email.
func (s *SMTPSender) Send(_ context.Context, msg Message) error {
	if s == nil {
		return fmt.Errorf("mail: sender is nil")
	}
	if len(msg.To) == 0 {
		return fmt.Errorf("mail: recipient required")
	}
	from := s.from
	if from == "" {
		from = "noreply@localhost"
	}
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}
	fromHeader := from
	if s.fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", s.fromName, from)
	}
	// Header values must not carry CR/LF: subjects interpolate user-chosen
	// names and a newline would inject extra headers (Bcc, etc.).
	for _, to := range msg.To {
		if strings.ContainsAny(to, "\r\n") {
			return fmt.Errorf("mail: invalid recipient")
		}
	}
	subject := mime.QEncoding.Encode("utf-8", stripHeaderBreaks(msg.Subject))
	payload := strings.Builder{}
	payload.WriteString("From: ")
	payload.WriteString(fromHeader)
	payload.WriteString("\r\n")
	payload.WriteString("To: ")
	payload.WriteString(strings.Join(msg.To, ", "))
	payload.WriteString("\r\n")
	payload.WriteString("Subject: ")
	payload.WriteString(subject)
	payload.WriteString("\r\n")
	payload.WriteString("MIME-Version: 1.0\r\n")
	writeBody(&payload, msg)
	if err := smtp.SendMail(addr, auth, from, msg.To, []byte(payload.String())); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return nil
}

// writeBody writes the content headers and body: plain text, or
// multipart/alternative (text + HTML) when HTMLBody is set.
func writeBody(b *strings.Builder, msg Message) {
	if msg.HTMLBody == "" {
		b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		b.WriteString(msg.Body)
		return
	}
	boundary := "olex-alt-" + strconv.FormatInt(int64(len(msg.Body))*7919+int64(len(msg.HTMLBody)), 36)
	for strings.Contains(msg.Body, boundary) || strings.Contains(msg.HTMLBody, boundary) {
		boundary += "x"
	}
	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(msg.Body)
	b.WriteString("\r\n--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	b.WriteString(msg.HTMLBody)
	b.WriteString("\r\n--" + boundary + "--\r\n")
}

func stripHeaderBreaks(v string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(v)
}

// NoopSender discards mail (tests).
type NoopSender struct{}

// Send implements Sender.
func (NoopSender) Send(context.Context, Message) error { return nil }

var (
	_ Sender = (*SMTPSender)(nil)
	_ Sender = NoopSender{}
)
