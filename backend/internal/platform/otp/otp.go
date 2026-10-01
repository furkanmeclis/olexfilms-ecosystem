// Package otp issues and verifies phone one-time codes sent over WhatsApp
// (SMS fallback) for customer login (K11) and contract signing.
//
// Limits: a code lives 5 minutes, a new one can be requested after 60
// seconds, at most 5 per phone per hour (DB counters, fail-closed) plus Redis
// counters per phone and per IP (fail-closed when Redis is down), and a code
// dies after 5 wrong attempts. Every message carries the KVKK notice of the
// recipient's locale; the notice version, message hash, IP, user agent,
// channel and provider reference are stored on the otp_codes row as evidence.
package otp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/google/uuid"
)

// Purposes (otp_codes.type).
const (
	PurposeCustomerLogin = "customer_login"
	PurposeContractSign  = "contract_sign"
)

// Limit reasons.
const (
	ReasonCooldown = "cooldown"
	ReasonHourly   = "hourly_limit"
	ReasonIP       = "ip_limit"
	ReasonVerify   = "verify_limit"
)

var (
	ErrInvalidPhone       = errors.New("otp: invalid phone")
	ErrInvalidPurpose     = errors.New("otp: invalid purpose")
	ErrInvalidCode        = errors.New("otp: invalid or expired code")
	ErrTooManyAttempts    = errors.New("otp: too many attempts")
	ErrRateLimited        = errors.New("otp: rate limited")
	ErrLimiterUnavailable = errors.New("otp: rate limiter unavailable")
	ErrDeliveryFailed     = errors.New("otp: delivery failed")
)

// LimitError is a rate limit with the earliest retry time.
type LimitError struct {
	Reason  string
	RetryAt time.Time
}

func (e *LimitError) Error() string { return "otp: rate limited (" + e.Reason + ")" }

// Is makes errors.Is(err, ErrRateLimited) true.
func (e *LimitError) Is(target error) bool { return target == ErrRateLimited }

// Limiter is the Redis fixed-window limiter (fail-closed variant).
type Limiter interface {
	AllowStrict(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration, error)
}

// TextSender delivers the code (whatsapp.Sender: WhatsApp, then SMS).
type TextSender interface {
	SendText(ctx context.Context, to, body string, opts whatsapp.SendOptions) (whatsapp.Delivery, error)
}

// Config tunes the service. Zero values take the defaults.
type Config struct {
	TTL         time.Duration // 5m
	Cooldown    time.Duration // 60s
	HourlyMax   int           // 5 per phone
	IPHourlyMax int           // 20 per IP
	// PhoneBurstMax bounds request attempts per phone per hour in Redis,
	// including ones the DB cooldown rejects (default 2 x HourlyMax).
	PhoneBurstMax int
	MaxAttempts   int32 // 5
	VerifyMax     int   // 20 verify calls per phone+IP per 15 min
	// Key is the server secret for code hashes (HMAC-SHA256).
	Key     []byte
	AppName string
}

func (c *Config) defaults() {
	if c.TTL <= 0 {
		c.TTL = 5 * time.Minute
	}
	if c.Cooldown <= 0 {
		c.Cooldown = 60 * time.Second
	}
	if c.HourlyMax <= 0 {
		c.HourlyMax = 5
	}
	if c.PhoneBurstMax <= 0 {
		c.PhoneBurstMax = 2 * c.HourlyMax
	}
	if c.IPHourlyMax <= 0 {
		c.IPHourlyMax = 20
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.VerifyMax <= 0 {
		c.VerifyMax = 20
	}
	if c.AppName == "" {
		c.AppName = "Olexfilms"
	}
}

// Service issues and verifies codes.
type Service struct {
	store   Store
	sender  TextSender
	limiter Limiter
	cfg     Config
	now     func() time.Time
	log     *slog.Logger
}

// New creates the OTP service.
func New(store Store, sender TextSender, limiter Limiter, cfg Config, log *slog.Logger) *Service {
	cfg.defaults()
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, sender: sender, limiter: limiter, cfg: cfg, now: time.Now, log: log}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// RequestInput asks for a code.
type RequestInput struct {
	Phone     string
	Region    string // default region for parsing (org country), else TR
	Purpose   string
	Locale    string
	IP        string
	UserAgent string
}

// RequestResult is returned to the client (202).
type RequestResult struct {
	ID        uuid.UUID `json:"-"`
	Phone     string    `json:"-"`
	Channel   string    `json:"channel"`
	ExpiresAt time.Time `json:"expires_at"`
	ResendAt  time.Time `json:"resend_at"`
}

// VerifyInput checks a code.
type VerifyInput struct {
	Phone   string
	Region  string
	Purpose string
	Code    string
	IP      string
}

// Verified is a consumed code.
type Verified struct {
	ID     uuid.UUID
	Phone  string
	UserID *int64
}

func validPurpose(p string) bool {
	return p == PurposeCustomerLogin || p == PurposeContractSign
}

// NormalizeLocale maps a locale to one with a KVKK notice (tr, en), tr default.
func NormalizeLocale(locale string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	if i := strings.IndexAny(l, "-_"); i > 0 {
		l = l[:i]
	}
	switch l {
	case "tr", "en":
		return l
	case "":
		return "tr"
	default:
		return "en"
	}
}

// Request issues a code and delivers it synchronously (critical message:
// not routed through the notification queue).
func (s *Service) Request(ctx context.Context, in RequestInput) (RequestResult, error) {
	purpose := strings.TrimSpace(in.Purpose)
	if purpose == "" {
		purpose = PurposeCustomerLogin
	}
	if !validPurpose(purpose) {
		return RequestResult{}, ErrInvalidPurpose
	}
	num, err := phone.Parse(in.Phone, in.Region)
	if err != nil {
		return RequestResult{}, ErrInvalidPhone
	}
	now := s.now().UTC()

	// Redis counters first (fail-closed): per phone and per IP.
	if err := s.allow(ctx, "otp_send", num.E164, s.cfg.PhoneBurstMax, time.Hour, ReasonHourly, now); err != nil {
		return RequestResult{}, err
	}
	if strings.TrimSpace(in.IP) != "" {
		if err := s.allow(ctx, "otp_send_ip", in.IP, s.cfg.IPHourlyMax, time.Hour, ReasonIP, now); err != nil {
			return RequestResult{}, err
		}
	}

	locale := NormalizeLocale(in.Locale)
	notice, err := s.store.LatestNotice(ctx, locale)
	if errors.Is(err, ErrNotFound) && locale != "tr" {
		notice, err = s.store.LatestNotice(ctx, "tr")
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return RequestResult{}, err
	}

	code, err := generateCode()
	if err != nil {
		return RequestResult{}, err
	}
	id := uuid.New()
	message := RenderMessage(locale, s.cfg.AppName, code, int(s.cfg.TTL/time.Minute), notice.Body)
	reserved, err := s.store.Reserve(ctx, ReserveInput{
		UUID: id, Phone: num.E164, Purpose: purpose, CodeHash: s.hash(id, code),
		Now: now, ExpiresAt: now.Add(s.cfg.TTL), MaxAttempts: s.cfg.MaxAttempts,
		Cooldown: s.cfg.Cooldown, HourlyMax: s.cfg.HourlyMax,
		IP: in.IP, UserAgent: truncate(in.UserAgent, 512), Notice: notice,
		MessageSHA256: MessageHash(message),
	})
	if err != nil {
		return RequestResult{}, err
	}

	delivery, err := s.sender.SendText(ctx, num.E164, message, whatsapp.SendOptions{ID: messageID(id)})
	if err != nil {
		s.log.Warn("otp_delivery_failed", "otp", id, "phone", phone.Mask(num.E164), "error", err)
		_ = s.store.MarkFailed(context.WithoutCancel(ctx), reserved.ID, truncate(err.Error(), 500), s.now().UTC())
		return RequestResult{}, fmt.Errorf("%w: %v", ErrDeliveryFailed, err)
	}
	note := ""
	if delivery.WhatsAppErr != nil {
		note = truncate("whatsapp: "+delivery.WhatsAppErr.Error(), 500)
	}
	if err := s.store.MarkDelivered(ctx, reserved.ID, delivery.Channel, delivery.ProviderRef, s.now().UTC(), note); err != nil {
		return RequestResult{}, err
	}
	return RequestResult{
		ID: id, Phone: num.E164, Channel: delivery.Channel,
		ExpiresAt: reserved.ExpiresAt.UTC(), ResendAt: reserved.CreatedAt.Add(s.cfg.Cooldown).UTC(),
	}, nil
}

// Verify checks and consumes a code. Five wrong attempts kill the code.
func (s *Service) Verify(ctx context.Context, in VerifyInput) (Verified, error) {
	purpose := strings.TrimSpace(in.Purpose)
	if purpose == "" {
		purpose = PurposeCustomerLogin
	}
	if !validPurpose(purpose) {
		return Verified{}, ErrInvalidPurpose
	}
	num, err := phone.Parse(in.Phone, in.Region)
	if err != nil {
		return Verified{}, ErrInvalidPhone
	}
	now := s.now().UTC()
	if err := s.allow(ctx, "otp_verify", num.E164+"|"+in.IP, s.cfg.VerifyMax, 15*time.Minute, ReasonVerify, now); err != nil {
		return Verified{}, err
	}
	code := strings.TrimSpace(in.Code)
	row, err := s.store.ActiveCode(ctx, num.E164, purpose, now)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Verified{}, ErrInvalidCode
		}
		return Verified{}, err
	}
	if row.AttemptCount >= row.MaxAttempts {
		_ = s.store.Consume(ctx, row.ID, now)
		return Verified{}, ErrTooManyAttempts
	}
	if subtle.ConstantTimeCompare([]byte(s.hash(row.UUID, code)), []byte(row.CodeHash)) != 1 {
		attempts, maxAttempts, err := s.store.IncrementAttempts(ctx, row.ID)
		if err != nil {
			return Verified{}, err
		}
		if attempts >= maxAttempts {
			_ = s.store.Consume(ctx, row.ID, now)
			return Verified{}, ErrTooManyAttempts
		}
		return Verified{}, ErrInvalidCode
	}
	if err := s.store.Consume(ctx, row.ID, now); err != nil {
		return Verified{}, err
	}
	return Verified{ID: row.UUID, Phone: num.E164, UserID: row.UserID}, nil
}

func (s *Service) allow(ctx context.Context, action, subject string, limit int, window time.Duration, reason string, now time.Time) error {
	if s.limiter == nil {
		return ErrLimiterUnavailable
	}
	ok, retry, err := s.limiter.AllowStrict(ctx, action, subject, limit, window)
	if err != nil {
		return ErrLimiterUnavailable
	}
	if !ok {
		return &LimitError{Reason: reason, RetryAt: now.Add(retry)}
	}
	return nil
}

// DeriveKey derives the code-hash key from the app encryption key so the
// OTP hashes do not reuse the encryption key directly.
func DeriveKey(appSecret string) []byte {
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write([]byte("otp-code-hash-v1"))
	return mac.Sum(nil)
}

// hash is HMAC-SHA256(server key, "<otp uuid>:<code>") in hex.
func (s *Service) hash(id uuid.UUID, code string) string {
	mac := hmac.New(sha256.New, s.cfg.Key)
	mac.Write([]byte(id.String() + ":" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

// MessageHash is the evidence hash of the exact text sent.
func MessageHash(message string) string {
	sum := sha256.Sum256([]byte(message))
	return hex.EncodeToString(sum[:])
}

var messageTemplates = map[string]string{
	"tr": "{app} doğrulama kodunuz: {code}\nKod {minutes} dakika geçerlidir. Bu kodu kimseyle paylaşmayın.",
	"en": "Your {app} verification code: {code}\nIt is valid for {minutes} minutes. Do not share this code.",
}

// RenderMessage builds the OTP text with the KVKK notice appended (K11).
func RenderMessage(locale, app, code string, minutes int, kvkk string) string {
	tpl, ok := messageTemplates[locale]
	if !ok {
		tpl = messageTemplates["tr"]
	}
	msg := strings.NewReplacer("{app}", app, "{code}", code, "{minutes}", fmt.Sprint(minutes)).Replace(tpl)
	if kvkk = strings.TrimSpace(kvkk); kvkk != "" {
		msg += "\n\n" + kvkk
	}
	return msg
}

func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// messageID derives an idempotent provider message id from the OTP uuid.
func messageID(id uuid.UUID) string {
	return strings.ToUpper(strings.ReplaceAll(id.String(), "-", ""))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
