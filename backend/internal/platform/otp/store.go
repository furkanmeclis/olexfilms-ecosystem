package otp

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Code is a stored OTP row (subset).
type Code struct {
	ID           int64
	UUID         uuid.UUID
	Phone        string
	Purpose      string
	CodeHash     string
	ExpiresAt    time.Time
	CreatedAt    time.Time
	AttemptCount int32
	MaxAttempts  int32
	UserID       *int64
}

// Notice is a KVKK notice version.
type Notice struct {
	Locale  string
	Version int32
	Body    string
}

// ReserveInput creates a code after the DB cooldown and hourly checks.
type ReserveInput struct {
	UUID          uuid.UUID
	Phone         string
	Purpose       string
	CodeHash      string
	Now           time.Time
	ExpiresAt     time.Time
	MaxAttempts   int32
	Cooldown      time.Duration
	HourlyMax     int
	IP            string
	UserAgent     string
	Notice        Notice
	MessageSHA256 string
}

// Store is the persistence port of the OTP service.
type Store interface {
	Reserve(ctx context.Context, in ReserveInput) (Code, error)
	MarkDelivered(ctx context.Context, id int64, channel, ref string, at time.Time, note string) error
	MarkFailed(ctx context.Context, id int64, reason string, at time.Time) error
	ActiveCode(ctx context.Context, phone, purpose string, now time.Time) (Code, error)
	IncrementAttempts(ctx context.Context, id int64) (attempts, maxAttempts int32, err error)
	Consume(ctx context.Context, id int64, now time.Time) error
	LatestNotice(ctx context.Context, locale string) (Notice, error)
}

// ErrNotFound is returned when no active code / notice exists.
var ErrNotFound = errors.New("otp: not found")

// PGStore implements Store on Postgres (otp_codes, kvkk_notices).
type PGStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPGStore creates a Postgres store.
func NewPGStore(pool *pgxpool.Pool, q *db.Queries) *PGStore {
	return &PGStore{pool: pool, q: q}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func fromRow(r db.OtpCode) Code {
	c := Code{
		ID: r.ID, UUID: r.Uuid, Phone: r.PhoneE164.String, Purpose: r.Type, CodeHash: r.CodeHash,
		ExpiresAt: r.ExpiresAt.Time, CreatedAt: r.CreatedAt.Time,
		AttemptCount: r.AttemptCount, MaxAttempts: r.MaxAttempts,
	}
	if r.UserID.Valid {
		id := r.UserID.Int64
		c.UserID = &id
	}
	return c
}

// Reserve implements Store. A per-phone advisory lock serializes concurrent
// requests so the cooldown and hourly counter cannot be raced.
func (s *PGStore) Reserve(ctx context.Context, in ReserveInput) (Code, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Code{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockOTPSubject(ctx, "otp:"+in.Phone); err != nil {
		return Code{}, err
	}
	latest, err := q.GetLatestPhoneOTP(ctx, db.GetLatestPhoneOTPParams{PhoneE164: text(in.Phone), Type: in.Purpose})
	if err == nil {
		if resendAt := latest.CreatedAt.Time.Add(in.Cooldown); in.Now.Before(resendAt) {
			return Code{}, &LimitError{Reason: ReasonCooldown, RetryAt: resendAt}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Code{}, err
	}
	since := in.Now.Add(-time.Hour)
	count, err := q.CountPhoneOTPsSince(ctx, db.CountPhoneOTPsSinceParams{PhoneE164: text(in.Phone), Since: ts(since)})
	if err != nil {
		return Code{}, err
	}
	if in.HourlyMax > 0 && count >= int64(in.HourlyMax) {
		return Code{}, &LimitError{Reason: ReasonHourly, RetryAt: in.Now.Add(time.Hour)}
	}
	if err := q.InvalidateActivePhoneOTPs(ctx, db.InvalidateActivePhoneOTPsParams{
		Now: ts(in.Now), PhoneE164: text(in.Phone), Type: in.Purpose,
	}); err != nil {
		return Code{}, err
	}
	params := db.CreatePhoneOTPParams{
		Uuid: in.UUID, PhoneE164: text(in.Phone), CodeHash: in.CodeHash, Type: in.Purpose,
		ExpiresAt: ts(in.ExpiresAt), MaxAttempts: in.MaxAttempts,
		Ip: text(in.IP), UserAgent: text(in.UserAgent),
		KvkkLocale: text(in.Notice.Locale), MessageSha256: text(in.MessageSHA256),
		CreatedAt: ts(in.Now),
	}
	if in.Notice.Version > 0 {
		params.KvkkVersion = pgtype.Int4{Int32: in.Notice.Version, Valid: true}
	}
	if u, err := q.GetUserByPhone(ctx, text(in.Phone)); err == nil {
		params.UserID = pgtype.Int8{Int64: u.ID, Valid: true}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Code{}, err
	}
	row, err := q.CreatePhoneOTP(ctx, params)
	if err != nil {
		return Code{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Code{}, err
	}
	return fromRow(row), nil
}

// MarkDelivered implements Store.
func (s *PGStore) MarkDelivered(ctx context.Context, id int64, channel, ref string, at time.Time, note string) error {
	return s.q.MarkOTPDelivered(ctx, db.MarkOTPDeliveredParams{
		Channel: text(channel), ProviderRef: text(ref), DeliveredAt: ts(at), DeliveryError: text(note), ID: id,
	})
}

// MarkFailed implements Store: the code is consumed so it cannot be used.
func (s *PGStore) MarkFailed(ctx context.Context, id int64, reason string, at time.Time) error {
	return s.q.MarkOTPDeliveryFailed(ctx, db.MarkOTPDeliveryFailedParams{
		DeliveryError: text(reason), Now: ts(at), ID: id,
	})
}

// ActiveCode implements Store.
func (s *PGStore) ActiveCode(ctx context.Context, phone, purpose string, now time.Time) (Code, error) {
	row, err := s.q.GetActivePhoneOTP(ctx, db.GetActivePhoneOTPParams{PhoneE164: text(phone), Type: purpose, Now: ts(now)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Code{}, ErrNotFound
		}
		return Code{}, err
	}
	return fromRow(row), nil
}

// IncrementAttempts implements Store.
func (s *PGStore) IncrementAttempts(ctx context.Context, id int64) (int32, int32, error) {
	row, err := s.q.IncrementOTPAttempts(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	return row.AttemptCount, row.MaxAttempts, nil
}

// Consume implements Store.
func (s *PGStore) Consume(ctx context.Context, id int64, now time.Time) error {
	return s.q.ConsumeOTPAt(ctx, db.ConsumeOTPAtParams{Now: ts(now), ID: id})
}

// LatestNotice implements Store.
func (s *PGStore) LatestNotice(ctx context.Context, locale string) (Notice, error) {
	row, err := s.q.GetLatestKVKKNotice(ctx, locale)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Notice{}, ErrNotFound
		}
		return Notice{}, err
	}
	return Notice{Locale: row.Locale, Version: row.Version, Body: row.Body}, nil
}
