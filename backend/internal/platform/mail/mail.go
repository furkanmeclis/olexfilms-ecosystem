package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

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
	// Attachments, when set, make the message multipart/mixed: the body
	// part first, then each file base64 encoded (TEC-476 fleet report PDF).
	Attachments []Attachment
}

// Attachment is a file sent with a message.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
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
	// tlsConfig overrides the implicit-TLS client config (tests).
	tlsConfig *tls.Config
}

// implicitTLSPort is SMTPS (RFC 8314): TLS from the first byte, no STARTTLS.
// smtp.SendMail only speaks plain + STARTTLS, so on this port it waits for
// a greeting the server never sends until the TLS handshake.
const implicitTLSPort = 465

const smtpDialTimeout = 30 * time.Second

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
	send := smtp.SendMail
	if s.port == implicitTLSPort {
		send = s.sendImplicitTLS
	}
	if err := send(addr, auth, from, msg.To, []byte(payload.String())); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return nil
}

// sendImplicitTLS is smtp.SendMail over a connection that is TLS from the
// start (port 465).
func (s *SMTPSender) sendImplicitTLS(addr string, auth smtp.Auth, from string, to []string, body []byte) error {
	cfg := s.tlsConfig
	if cfg == nil {
		cfg = &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: smtpDialTimeout}, "tcp", addr, cfg)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// writeBody writes the content headers and body: plain text, or
// multipart/alternative (text + HTML) when HTMLBody is set, wrapped in
// multipart/mixed with the attachments when there are any.
func writeBody(b *strings.Builder, msg Message) {
	if len(msg.Attachments) == 0 {
		writeContent(b, msg)
		return
	}
	boundary := "olex-mixed-" + strconv.FormatInt(int64(len(msg.Body))*7907+int64(len(msg.HTMLBody))+int64(len(msg.Attachments)), 36)
	for strings.Contains(msg.Body, boundary) || strings.Contains(msg.HTMLBody, boundary) {
		boundary += "x"
	}
	b.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n\r\n")
	b.WriteString("--" + boundary + "\r\n")
	writeContent(b, msg)
	for _, a := range msg.Attachments {
		ct := a.ContentType
		if ct == "" || strings.ContainsAny(ct, "\r\n") {
			ct = "application/octet-stream"
		}
		name := mime.QEncoding.Encode("utf-8", stripHeaderBreaks(strings.ReplaceAll(a.Filename, `"`, "")))
		b.WriteString("\r\n--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + ct + "; name=\"" + name + "\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		b.WriteString("Content-Disposition: attachment; filename=\"" + name + "\"\r\n\r\n")
		enc := base64.StdEncoding.EncodeToString(a.Data)
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
}

// writeContent writes the text part: plain text, or multipart/alternative
// (text + HTML) when HTMLBody is set.
func writeContent(b *strings.Builder, msg Message) {
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
