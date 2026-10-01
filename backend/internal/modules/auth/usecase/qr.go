package usecase

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-91: QR web sign-in. The web (not signed in) starts a challenge and shows
// its code as a QR; the signed-in mobile app scans it, sees the browser's IP
// and user agent, and approves or rejects; the web exchanges code + its own
// secret for a panel session. Status changes are published to the Centrifugo
// channel qr:{code} as {status} only; GET status is the polling fallback.

// QR challenge statuses (qr_login_challenges.status; "expired" is derived).
const (
	QRStatusPending  = "pending"
	QRStatusScanned  = "scanned"
	QRStatusApproved = "approved"
	QRStatusRejected = "rejected"
	QRStatusConsumed = "consumed"
	QRStatusExpired  = "expired"
)

// DefaultQRLoginTTL is the lifetime of a QR challenge.
const DefaultQRLoginTTL = 120 * time.Second

var (
	ErrQRNotFound = errors.New("qr login challenge not found")
	ErrQRExpired  = errors.New("qr login challenge expired")
	ErrQRRejected = errors.New("qr login was rejected")
	ErrQRPending  = errors.New("qr login is not approved yet")
	ErrQRClosed   = errors.New("qr login challenge is already decided or used")
	ErrQRSecret   = errors.New("qr login secret does not match")
	ErrQRDisabled = errors.New("qr login is not configured")
)

// QRStore is the persistence of QR challenges (*db.Queries).
type QRStore interface {
	CreateQRLoginChallenge(ctx context.Context, arg db.CreateQRLoginChallengeParams) (db.QrLoginChallenge, error)
	GetQRLoginChallengeByCode(ctx context.Context, code string) (db.QrLoginChallenge, error)
	MarkQRLoginChallengeScanned(ctx context.Context, code string) (db.QrLoginChallenge, error)
	DecideQRLoginChallenge(ctx context.Context, arg db.DecideQRLoginChallengeParams) (db.QrLoginChallenge, error)
	ConsumeQRLoginChallenge(ctx context.Context, code string) (db.QrLoginChallenge, error)
	DeleteStaleQRLoginChallenges(ctx context.Context) (int64, error)
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
}

// QRPublisher publishes status changes (realtime.Publisher).
type QRPublisher interface {
	Publish(ctx context.Context, channel string, data any) error
}

// QRGuestTokens mints the anonymous, single channel, subscribe-only
// Centrifugo tokens of a waiting browser (realtime.TokenIssuer).
type QRGuestTokens interface {
	Enabled() bool
	WSURL() string
	GuestChannelTokens(channel string, ttl time.Duration) (connection, subscription string, expiresAt time.Time, err error)
}

type qrDeps struct {
	store     QRStore
	publisher QRPublisher
	guest     QRGuestTokens
	ttl       time.Duration
}

// SetQRLogin wires QR web sign-in. publisher and guest may be nil
// (Centrifugo disabled: clients poll GET status).
func (u *AuthUseCase) SetQRLogin(store QRStore, publisher QRPublisher, guest QRGuestTokens, ttl time.Duration) {
	if ttl <= 0 {
		ttl = DefaultQRLoginTTL
	}
	u.qr = qrDeps{store: store, publisher: publisher, guest: guest, ttl: ttl}
}

// QRChannel is the Centrifugo channel of a challenge.
func QRChannel(code string) string { return "qr:" + code }

// QRRealtime lets a waiting browser subscribe to qr:{code}.
type QRRealtime struct {
	Enabled           bool      `json:"enabled"`
	WSURL             string    `json:"ws_url,omitempty"`
	Channel           string    `json:"channel"`
	Token             string    `json:"token,omitempty"`
	SubscriptionToken string    `json:"subscription_token,omitempty"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// QRStartResult is returned to the web that starts a challenge. Secret is
// shown once and never stored in clear.
type QRStartResult struct {
	Code      string     `json:"code"`
	Secret    string     `json:"secret"`
	Payload   string     `json:"payload"`
	Status    string     `json:"status"`
	ExpiresAt time.Time  `json:"expires_at"`
	Realtime  QRRealtime `json:"realtime"`
}

// QRStatusResult is the polling view of a challenge.
type QRStatusResult struct {
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// QRScanResult is what the mobile app shows before approve / reject.
type QRScanResult struct {
	Code         string    `json:"code"`
	Status       string    `json:"status"`
	WebIP        string    `json:"web_ip,omitempty"`
	WebUserAgent string    `json:"web_user_agent,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// QRCompleteResult carries the panel token pair for the web.
type QRCompleteResult struct {
	model.Tokens
	User model.PublicUser `json:"user"`
}

// QRPayloadPrefix is the deep link the mobile app recognizes.
const QRPayloadPrefix = "olexfilms://qr-login/"

func randomURLToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("qr login: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (u *AuthUseCase) qrReady() error {
	if u.qr.store == nil {
		return ErrQRDisabled
	}
	return nil
}

func qrEffectiveStatus(row db.QrLoginChallenge, now time.Time) string {
	if row.Status == QRStatusRejected || row.Status == QRStatusConsumed {
		return row.Status
	}
	if !row.ExpiresAt.Time.After(now) {
		return QRStatusExpired
	}
	return row.Status
}

func (u *AuthUseCase) publishQR(ctx context.Context, code, status string) {
	if u.qr.publisher == nil {
		return
	}
	// Only the status: no user, device or token data on a guest channel.
	if err := u.qr.publisher.Publish(ctx, QRChannel(code), map[string]string{"status": status}); err != nil && u.log != nil {
		u.log.Warn("qr_login_publish_failed", "error", err)
	}
}

func (u *AuthUseCase) loadQR(ctx context.Context, code string) (db.QrLoginChallenge, error) {
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 64 {
		return db.QrLoginChallenge{}, ErrQRNotFound
	}
	row, err := u.qr.store.GetQRLoginChallengeByCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.QrLoginChallenge{}, ErrQRNotFound
		}
		return db.QrLoginChallenge{}, err
	}
	return row, nil
}

// qrStateError maps the effective status to the error of an operation that
// needs another status.
func qrStateError(status string) error {
	switch status {
	case QRStatusExpired:
		return ErrQRExpired
	case QRStatusRejected:
		return ErrQRRejected
	case QRStatusPending, QRStatusScanned:
		return ErrQRPending
	default:
		return ErrQRClosed
	}
}

// QRStart opens a challenge for a browser that is not signed in.
func (u *AuthUseCase) QRStart(ctx context.Context, meta model.SessionMeta) (QRStartResult, error) {
	if err := u.qrReady(); err != nil {
		return QRStartResult{}, err
	}
	if _, err := u.qr.store.DeleteStaleQRLoginChallenges(ctx); err != nil && u.log != nil {
		u.log.Warn("qr_login_cleanup_failed", "error", err)
	}
	code, err := randomURLToken(16)
	if err != nil {
		return QRStartResult{}, err
	}
	secret, err := randomURLToken(32)
	if err != nil {
		return QRStartResult{}, err
	}
	expires := u.now().UTC().Add(u.qr.ttl)
	params := db.CreateQRLoginChallengeParams{
		Code: code, SecretHash: tokenHash(secret),
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	}
	if b, ok := brandctx.From(ctx); ok && b.ID > 0 {
		params.BrandID = pgtype.Int8{Int64: b.ID, Valid: true}
	}
	if ip := strings.TrimSpace(meta.IP); ip != "" && len(ip) <= 64 {
		params.WebIp = pgtype.Text{String: ip, Valid: true}
	}
	if ua := strings.TrimSpace(meta.UserAgent); ua != "" {
		if len(ua) > 512 {
			ua = ua[:512]
		}
		params.WebUserAgent = pgtype.Text{String: ua, Valid: true}
	}
	row, err := u.qr.store.CreateQRLoginChallenge(ctx, params)
	if err != nil {
		return QRStartResult{}, err
	}
	out := QRStartResult{
		Code: row.Code, Secret: secret, Payload: QRPayloadPrefix + row.Code,
		Status: QRStatusPending, ExpiresAt: row.ExpiresAt.Time,
		Realtime: QRRealtime{Channel: QRChannel(row.Code), ExpiresAt: row.ExpiresAt.Time},
	}
	if u.qr.guest != nil && u.qr.guest.Enabled() {
		conn, sub, exp, err := u.qr.guest.GuestChannelTokens(QRChannel(row.Code), u.qr.ttl)
		if err == nil {
			out.Realtime = QRRealtime{
				Enabled: true, WSURL: u.qr.guest.WSURL(), Channel: QRChannel(row.Code),
				Token: conn, SubscriptionToken: sub, ExpiresAt: exp,
			}
		} else if u.log != nil {
			u.log.Warn("qr_login_guest_token_failed", "error", err)
		}
	}
	return out, nil
}

// QRStatus is the polling fallback; an expired challenge is ErrQRExpired.
func (u *AuthUseCase) QRStatus(ctx context.Context, code string) (QRStatusResult, error) {
	if err := u.qrReady(); err != nil {
		return QRStatusResult{}, err
	}
	row, err := u.loadQR(ctx, code)
	if err != nil {
		return QRStatusResult{}, err
	}
	status := qrEffectiveStatus(row, u.now())
	if status == QRStatusExpired {
		return QRStatusResult{}, ErrQRExpired
	}
	return QRStatusResult{Status: status, ExpiresAt: row.ExpiresAt.Time}, nil
}

// QRScan shows the challenge to the mobile app (browser IP and user agent)
// and marks it scanned.
func (u *AuthUseCase) QRScan(ctx context.Context, code string) (QRScanResult, error) {
	if err := u.qrReady(); err != nil {
		return QRScanResult{}, err
	}
	row, err := u.loadQR(ctx, code)
	if err != nil {
		return QRScanResult{}, err
	}
	switch status := qrEffectiveStatus(row, u.now()); status {
	case QRStatusPending:
		if scanned, err := u.qr.store.MarkQRLoginChallengeScanned(ctx, row.Code); err == nil {
			row = scanned
			u.publishQR(ctx, row.Code, QRStatusScanned)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return QRScanResult{}, err
		}
	case QRStatusScanned:
	default:
		return QRScanResult{}, qrStateError(status)
	}
	return QRScanResult{
		Code: row.Code, Status: qrEffectiveStatus(row, u.now()),
		WebIP: row.WebIp.String, WebUserAgent: row.WebUserAgent.String,
		CreatedAt: row.CreatedAt.Time, ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

// QRDecide approves or rejects a challenge from a mobile session (sid). An
// approval signs the web in as the mobile user, in the session's active
// organization; the panel realm rule applies.
func (u *AuthUseCase) QRDecide(ctx context.Context, code string, approve bool, userID int64, sid uuid.UUID) (QRStatusResult, error) {
	if err := u.qrReady(); err != nil {
		return QRStatusResult{}, err
	}
	session, err := u.MobileSession(ctx, userID, sid)
	if err != nil {
		return QRStatusResult{}, err
	}
	row, err := u.loadQR(ctx, code)
	if err != nil {
		return QRStatusResult{}, err
	}
	if status := qrEffectiveStatus(row, u.now()); status != QRStatusPending && status != QRStatusScanned {
		return QRStatusResult{}, qrStateError(status)
	}
	params := db.DecideQRLoginChallengeParams{
		Code: row.Code, Status: QRStatusRejected,
		ApproverSessionID: pgtype.Int8{Int64: session.ID, Valid: session.ID > 0},
		ApproverDevice:    textOrNull(deviceSummary(session.Device)),
	}
	if approve {
		user, err := u.repo.FindUserByID(ctx, userID)
		if err != nil {
			return QRStatusResult{}, err
		}
		if user.Status != "active" {
			return QRStatusResult{}, ErrUserDisabled
		}
		if err := u.checkRealmAccess(ctx, user, jwt.AudiencePanel); err != nil {
			return QRStatusResult{}, err
		}
		params.Status = QRStatusApproved
		params.UserID = pgtype.Int8{Int64: userID, Valid: true}
		if session.OrganizationUUID != nil {
			if internalID, err := u.repo.ResolveOrganizationInternalID(ctx, *session.OrganizationUUID); err == nil {
				params.OrganizationID = pgtype.Int8{Int64: internalID, Valid: true}
			}
		}
	}
	decided, err := u.qr.store.DecideQRLoginChallenge(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Lost a race with another decision or the expiry.
			if again, lerr := u.loadQR(ctx, row.Code); lerr == nil {
				return QRStatusResult{}, qrStateError(qrEffectiveStatus(again, u.now()))
			}
			return QRStatusResult{}, ErrQRClosed
		}
		return QRStatusResult{}, err
	}
	u.publishQR(ctx, decided.Code, decided.Status)
	return QRStatusResult{Status: decided.Status, ExpiresAt: decided.ExpiresAt.Time}, nil
}

// QRComplete exchanges an approved challenge and the browser's secret for a
// panel token pair (single use).
func (u *AuthUseCase) QRComplete(ctx context.Context, code, secret string, meta model.SessionMeta) (QRCompleteResult, error) {
	if err := u.qrReady(); err != nil {
		return QRCompleteResult{}, err
	}
	row, err := u.loadQR(ctx, code)
	if err != nil {
		return QRCompleteResult{}, err
	}
	if subtle.ConstantTimeCompare([]byte(tokenHash(strings.TrimSpace(secret))), []byte(row.SecretHash)) != 1 {
		return QRCompleteResult{}, ErrQRSecret
	}
	if status := qrEffectiveStatus(row, u.now()); status != QRStatusApproved {
		return QRCompleteResult{}, qrStateError(status)
	}
	if row.BrandID.Valid {
		if b, ok := brandctx.From(ctx); ok && b.ID > 0 && b.ID != row.BrandID.Int64 {
			return QRCompleteResult{}, ErrBrandMismatch
		}
	}
	consumed, err := u.qr.store.ConsumeQRLoginChallenge(ctx, row.Code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return QRCompleteResult{}, ErrQRClosed
		}
		return QRCompleteResult{}, err
	}
	u.publishQR(ctx, consumed.Code, QRStatusConsumed)
	user, err := u.repo.FindUserByID(ctx, consumed.UserID.Int64)
	if err != nil {
		return QRCompleteResult{}, ErrInvalidCredentials
	}
	if user.Status != "active" {
		return QRCompleteResult{}, ErrUserDisabled
	}
	if err := u.checkRealmAccess(ctx, user, jwt.AudiencePanel); err != nil {
		return QRCompleteResult{}, err
	}
	var orgUUID *uuid.UUID
	if consumed.OrganizationID.Valid && u.orgResolver != nil {
		if org, err := u.qr.store.GetOrganizationByID(ctx, consumed.OrganizationID.Int64); err == nil {
			// Re-validate membership and the domain brand; drop oid quietly.
			if resolved, err := u.orgResolver.ResolveOrganizationUUID(ctx, user.ID, org.Uuid); err == nil {
				orgUUID = &resolved
			}
		}
	}
	if err := u.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		return QRCompleteResult{}, err
	}
	meta.Realm = jwt.AudiencePanel
	meta.Client, meta.Device, meta.FamilyID, meta.ImpersonatorUserID = "", nil, uuid.Nil, nil
	tokens, err := u.issueTokensForUser(ctx, user, meta, orgUUID)
	if err != nil {
		return QRCompleteResult{}, err
	}
	isSA, err := u.repo.UserHasRoleSlug(ctx, user.ID, rbac.RoleSuperAdmin)
	if err != nil {
		return QRCompleteResult{}, err
	}
	return QRCompleteResult{Tokens: tokens, User: model.ToPublicUser(user, isSA)}, nil
}

func deviceSummary(d *model.DeviceInfo) string {
	if d == nil {
		return ""
	}
	s := strings.TrimSpace(strings.Join([]string{d.Name, d.Platform, d.AppVersion}, " "))
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}

func textOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
