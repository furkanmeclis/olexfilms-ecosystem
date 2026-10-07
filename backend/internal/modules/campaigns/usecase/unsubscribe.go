package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// UnsubscribePath is the frontend page of the e-mail unsubscribe link.
const UnsubscribePath = "/abonelik-iptal/"

// unsubscribeMACLen is the truncated HMAC length in the token (128 bits).
const unsubscribeMACLen = 16

// UnsubscribeToken signs the user uuid: base64url(uuid ‖ HMAC-SHA256[:16]).
// It does not expire: an unsubscribe link must keep working.
func UnsubscribeToken(secret []byte, user uuid.UUID) string {
	buf := make([]byte, 0, 16+unsubscribeMACLen)
	buf = append(buf, user[:]...)
	buf = append(buf, unsubscribeMAC(secret, user)...)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func unsubscribeMAC(secret []byte, user uuid.UUID) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("campaigns.unsubscribe\n"))
	_, _ = mac.Write(user[:])
	return mac.Sum(nil)[:unsubscribeMACLen]
}

// ParseUnsubscribeToken verifies a token and returns the user uuid.
func ParseUnsubscribeToken(secret []byte, token string) (uuid.UUID, bool) {
	if len(secret) == 0 {
		return uuid.Nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil || len(raw) != 16+unsubscribeMACLen {
		return uuid.Nil, false
	}
	user, err := uuid.FromBytes(raw[:16])
	if err != nil || !hmac.Equal(raw[16:], unsubscribeMAC(secret, user)) {
		return uuid.Nil, false
	}
	return user, true
}

// UnsubscribeURL is the absolute unsubscribe link of a user.
func (s *Sender) UnsubscribeURL(user uuid.UUID) string {
	if len(s.d.UnsubscribeSecret) == 0 {
		return ""
	}
	return strings.TrimRight(s.d.FrontendURL, "/") + UnsubscribePath + UnsubscribeToken(s.d.UnsubscribeSecret, user)
}

// Unsubscriber records the marketing opt-out of an unsubscribe link.
type Unsubscriber struct {
	q      *db.Queries
	secret []byte
}

// NewUnsubscriber builds the public unsubscribe use case.
func NewUnsubscriber(q *db.Queries, secret []byte) *Unsubscriber {
	return &Unsubscriber{q: q, secret: secret}
}

// Unsubscribe writes a marketing opt-out (source campaign) for the phone
// number of the token's user, the same opt-out a WhatsApp DUR reply
// writes; later campaigns leave the number out. An invalid token or an
// unknown user is ErrNotFound; repeating it is a no-op.
func (u *Unsubscriber) Unsubscribe(ctx context.Context, token string) error {
	id, ok := ParseUnsubscribeToken(u.secret, token)
	if !ok {
		return ErrNotFound
	}
	user, err := u.q.GetUserByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if user.DeletedAt.Valid || !user.PhoneE164.Valid || !e164Re.MatchString(user.PhoneE164.String) {
		return ErrNotFound
	}
	phone := user.PhoneE164.String
	st, err := u.q.GetContactOptOutState(ctx, db.GetContactOptOutStateParams{ContactE164: phone, Scope: "marketing"})
	if err == nil && st.OptedOut {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = u.q.InsertContactOptOut(ctx, db.InsertContactOptOutParams{
		ContactE164: phone, Scope: "marketing", Action: "out", Source: "campaign",
		Note: pgtype.Text{String: "e-mail unsubscribe link", Valid: true},
	})
	return err
}
