// Package usecase holds the short URL service (TEC-249, F2-04d): internal
// links of the brand's frontend shortened to /s/{token} for WhatsApp, SMS
// and notification templates.
package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TokenLength is the length of a new token: 10 base62 characters (~59 bits).
const TokenLength = 10

// MaxTargetLength is the column limit of target_path.
const MaxTargetLength = 2048

// tokenAlphabet is base62, the alphabet of the old hub's Laravel
// Str::random tokens as well, so new and migrated tokens look the same.
const tokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// tokenRe is the shape of a stored token (chk_short_urls_token): new tokens
// are TokenLength characters, the old hub's are 8 (VARCHAR(16)); anything
// else is rejected without a database round trip.
var tokenRe = regexp.MustCompile(`^[A-Za-z0-9]{4,16}$`)

// createAttempts bounds the redraws when a token is already taken.
const createAttempts = 5

// AllowedPrefixes are the frontend areas a short URL may point at. Nothing
// else, and never another origin: the public resolver is not an open
// redirect.
var AllowedPrefixes = []string{"/portal", "/garanti", "/bayi"}

var (
	// ErrNotFound: the token is malformed, unknown or another brand's.
	ErrNotFound = errors.New("shorturls: token not found")
	// ErrExpired: the token exists in the brand but expires_at has passed.
	ErrExpired = errors.New("shorturls: token expired")
	// errTokenTaken: the drawn token already exists (redrawn).
	errTokenTaken = errors.New("shorturls: token taken")
)

// ValidationError is a rejected Create input; the HTTP layer answers 400
// VALIDATION_ERROR with Field / Code / Message as the detail.
type ValidationError struct {
	Field   string
	Code    string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// ValidToken reports whether token has the shape of a stored token.
func ValidToken(token string) bool { return tokenRe.MatchString(token) }

// Store is the subset of the generated queries the service uses.
type Store interface {
	CreateShortURL(ctx context.Context, arg db.CreateShortURLParams) (db.CreateShortURLRow, error)
	HitShortURL(ctx context.Context, arg db.HitShortURLParams) (db.HitShortURLRow, error)
	GetShortURLExpiry(ctx context.Context, arg db.GetShortURLExpiryParams) (pgtype.Timestamptz, error)
}

// CreateInput describes a new short URL. Target is an internal path under
// AllowedPrefixes (query and fragment allowed); TTL 0 never expires.
type CreateInput struct {
	BrandID        int64
	OrganizationID *int64
	CreatedBy      *int64
	Target         string
	TTL            time.Duration
}

// ShortURL is a created short URL.
type ShortURL struct {
	UUID       uuid.UUID  `json:"uuid"`
	Token      string     `json:"token"`
	TargetPath string     `json:"target_path"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Resolved is the answer of the public resolver.
type Resolved struct {
	Token      string     `json:"token"`
	TargetPath string     `json:"target_path"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// Service creates and resolves short URLs.
type Service struct {
	store Store
	now   func() time.Time
	token func() (string, error)
}

// New builds the service over the generated queries.
func New(store Store) *Service {
	return &Service{store: store, now: time.Now, token: RandomToken}
}

// WithClock replaces the clock (tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// WithTokens replaces the token source (tests).
func (s *Service) WithTokens(next func() (string, error)) *Service {
	s.token = next
	return s
}

// RandomToken draws a TokenLength character base62 token from crypto/rand.
func RandomToken() (string, error) {
	var b strings.Builder
	b.Grow(TokenLength)
	limit := big.NewInt(int64(len(tokenAlphabet)))
	for range TokenLength {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("shorturls: token: %w", err)
		}
		b.WriteByte(tokenAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// NormalizeTarget checks a target and returns its stored form. It must be
// a same-origin absolute path ("/garanti/X?lang=tr") under one of
// AllowedPrefixes: no scheme, host, "//", backslash, whitespace, control
// characters or dot segments.
func NormalizeTarget(raw string) (string, error) {
	invalid := func(msg string) error {
		return &ValidationError{Field: "target", Code: "invalid_target", Message: msg}
	}
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", &ValidationError{Field: "target", Code: "required", Message: "target is required"}
	}
	if len(target) > MaxTargetLength {
		return "", invalid("target is too long")
	}
	if !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		return "", invalid("target must be an internal path")
	}
	for _, r := range target {
		if r == '\\' || unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", invalid("target must be an internal path")
		}
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || u.Opaque != "" {
		return "", invalid("target must be an internal path")
	}
	if strings.Contains(u.Path, "\\") || strings.HasPrefix(u.Path, "//") {
		return "", invalid("target must be an internal path")
	}
	for seg := range strings.SplitSeq(u.Path, "/") {
		if seg == "." || seg == ".." {
			return "", invalid("target must be an internal path")
		}
	}
	if !allowedPath(u.Path) {
		return "", invalid("target is not an allowed path")
	}
	return target, nil
}

func allowedPath(p string) bool {
	for _, prefix := range AllowedPrefixes {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

// Create stores a new short URL for an allowed internal target. A taken
// token is redrawn a few times.
func (s *Service) Create(ctx context.Context, in CreateInput) (ShortURL, error) {
	target, err := NormalizeTarget(in.Target)
	if err != nil {
		return ShortURL{}, err
	}
	if in.TTL < 0 {
		return ShortURL{}, &ValidationError{Field: "ttl", Code: "invalid_ttl", Message: "ttl must not be negative"}
	}
	if in.BrandID <= 0 {
		return ShortURL{}, &ValidationError{Field: "brand_id", Code: "required", Message: "brand is required"}
	}
	arg := db.CreateShortURLParams{
		OrganizationID: int8Ptr(in.OrganizationID),
		BrandID:        in.BrandID,
		TargetPath:     target,
		CreatedBy:      int8Ptr(in.CreatedBy),
	}
	if in.TTL > 0 {
		arg.ExpiresAt = pgtype.Timestamptz{Time: s.now().Add(in.TTL), Valid: true}
	}
	for range createAttempts {
		token, err := s.token()
		if err != nil {
			return ShortURL{}, err
		}
		arg.Token = token
		row, err := s.store.CreateShortURL(ctx, arg)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // ON CONFLICT (token) DO NOTHING
		}
		if err != nil {
			return ShortURL{}, fmt.Errorf("shorturls: create: %w", err)
		}
		return ShortURL{
			UUID:       row.Uuid,
			Token:      row.Token,
			TargetPath: row.TargetPath,
			ExpiresAt:  timePtr(row.ExpiresAt),
			CreatedAt:  row.CreatedAt.Time,
		}, nil
	}
	return ShortURL{}, errTokenTaken
}

// Resolve returns the target of a live token of the brand and counts the
// hit. A malformed, unknown or other-brand token is ErrNotFound; a token
// past expires_at is ErrExpired (and is not counted).
func (s *Service) Resolve(ctx context.Context, brandID int64, token string) (Resolved, error) {
	if !ValidToken(token) {
		return Resolved{}, ErrNotFound
	}
	now := pgtype.Timestamptz{Time: s.now(), Valid: true}
	row, err := s.store.HitShortURL(ctx, db.HitShortURLParams{Now: now, Token: token, BrandID: brandID})
	if err == nil {
		return Resolved{Token: token, TargetPath: row.TargetPath, ExpiresAt: timePtr(row.ExpiresAt)}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Resolved{}, fmt.Errorf("shorturls: resolve: %w", err)
	}
	exp, err := s.store.GetShortURLExpiry(ctx, db.GetShortURLExpiryParams{Token: token, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Resolved{}, ErrNotFound
	}
	if err != nil {
		return Resolved{}, fmt.Errorf("shorturls: resolve: %w", err)
	}
	if exp.Valid && !exp.Time.After(now.Time) {
		return Resolved{}, ErrExpired
	}
	// Expired between the two statements is impossible; anything else is a
	// row that appeared in between: treat as unknown for this request.
	return Resolved{}, ErrNotFound
}

// Linker builds absolute short links for message templates (WhatsApp, SMS,
// notifications): Link stores the short URL and returns
// {frontendURL}/s/{token}.
type Linker struct {
	svc     *Service
	baseURL string
}

// NewLinker builds the template helper; frontendURL is the public origin
// of the brand frontend (PUBLIC_FRONTEND_URL).
func NewLinker(svc *Service, frontendURL string) *Linker {
	return &Linker{svc: svc, baseURL: strings.TrimRight(frontendURL, "/")}
}

// Link shortens an internal target and returns the absolute short link.
func (l *Linker) Link(ctx context.Context, in CreateInput) (string, error) {
	out, err := l.svc.Create(ctx, in)
	if err != nil {
		return "", err
	}
	return l.URL(out.Token), nil
}

// LinkWithStore shortens an internal target through a caller-provided store.
// Use this when the short URL must participate in the caller's transaction.
func (l *Linker) LinkWithStore(ctx context.Context, store Store, in CreateInput) (string, error) {
	svc := New(store)
	if l.svc != nil {
		svc.now = l.svc.now
		svc.token = l.svc.token
	}
	out, err := svc.Create(ctx, in)
	if err != nil {
		return "", err
	}
	return l.URL(out.Token), nil
}

// URL is the absolute short link of a token.
func (l *Linker) URL(token string) string { return l.baseURL + "/s/" + token }

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
