// Package usecase holds the portal legal texts and consent rules (TEC-90,
// K19/K22): admin-edited Markdown per language, versioned; each user is asked
// once per text version and the decision (accept or decline) is recorded.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Kinds of legal texts.
const (
	KindAIGuidelines = "ai_guidelines"
)

// MaxBodyBytes bounds an admin text.
const MaxBodyBytes = 64 << 10

var (
	ErrInvalidRequest = errors.New("invalid request")
	ErrNotFound       = errors.New("not found")
	// ErrStaleVersion: the client answered a text version that is no longer
	// the current one (the admin published a newer version meanwhile).
	ErrStaleVersion = errors.New("legal text version is not current")
)

// Store is the persistence port (generated sqlc queries).
type Store interface {
	GetLatestLegalText(ctx context.Context, arg db.GetLatestLegalTextParams) (db.LegalText, error)
	ListLatestLegalTexts(ctx context.Context, kind string) ([]db.LegalText, error)
	ListLegalTextVersions(ctx context.Context, arg db.ListLegalTextVersionsParams) ([]db.LegalText, error)
	InsertLegalText(ctx context.Context, arg db.InsertLegalTextParams) (db.LegalText, error)
	GetConsentForText(ctx context.Context, arg db.GetConsentForTextParams) (db.Consent, error)
	InsertConsent(ctx context.Context, arg db.InsertConsentParams) (db.Consent, error)
}

// Service implements legal text and consent flows.
type Service struct {
	store Store
}

// New creates the service.
func New(store Store) *Service { return &Service{store: store} }

// IsKind reports whether kind is a known legal text kind.
func IsKind(kind string) bool { return kind == KindAIGuidelines }

// Text is a published legal text version.
type Text struct {
	UUID      uuid.UUID `json:"uuid"`
	Kind      string    `json:"kind"`
	Locale    string    `json:"locale"`
	Version   int32     `json:"version"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Consent is a recorded decision.
type Consent struct {
	UUID        uuid.UUID `json:"uuid"`
	Kind        string    `json:"kind"`
	Locale      string    `json:"locale"`
	TextVersion int32     `json:"text_version"`
	Accepted    bool      `json:"accepted"`
	DecidedAt   time.Time `json:"decided_at"`
}

func toText(t db.LegalText) Text {
	return Text{UUID: t.Uuid, Kind: t.Kind, Locale: t.Locale, Version: t.Version, Body: t.Body, CreatedAt: t.CreatedAt.Time}
}

func toConsent(c db.Consent) Consent {
	return Consent{UUID: c.Uuid, Kind: c.Kind, Locale: c.Locale, TextVersion: c.TextVersion, Accepted: c.Accepted, DecidedAt: c.DecidedAt.Time}
}

// currentText returns the latest text of kind for locale, falling back to
// the default locale (tr) and then English.
func (s *Service) currentText(ctx context.Context, kind string, locale i18n.Locale) (db.LegalText, error) {
	tried := map[string]bool{}
	for _, l := range []string{string(locale), string(i18n.DefaultLocale), "en"} {
		if l == "" || tried[l] {
			continue
		}
		tried[l] = true
		t, err := s.store.GetLatestLegalText(ctx, db.GetLatestLegalTextParams{Kind: kind, Locale: l})
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.LegalText{}, err
		}
	}
	return db.LegalText{}, ErrNotFound
}

// Pending returns the legal texts the user has not answered in their current
// version. A user who declined is not asked again until the version changes.
func (s *Service) Pending(ctx context.Context, userID int64, locale i18n.Locale) ([]Text, error) {
	out := []Text{}
	for _, kind := range []string{KindAIGuidelines} {
		t, err := s.currentText(ctx, kind, locale)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		_, err = s.store.GetConsentForText(ctx, db.GetConsentForTextParams{UserID: userID, LegalTextID: t.ID})
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		out = append(out, toText(t))
	}
	return out, nil
}

// DecideInput is a consent answer.
type DecideInput struct {
	Kind      string
	Locale    string
	Version   int32
	Accepted  bool
	IP        string
	UserAgent string
}

// Decide records the user's answer to the current text version. Answering a
// version twice keeps the first decision (asked once).
func (s *Service) Decide(ctx context.Context, userID int64, in DecideInput) (Consent, error) {
	if !IsKind(in.Kind) || in.Version < 1 {
		return Consent{}, ErrInvalidRequest
	}
	locale, ok := i18n.Parse(in.Locale)
	if !ok {
		return Consent{}, fmt.Errorf("%w: unsupported locale", ErrInvalidRequest)
	}
	// The client answers the text it was shown (pending returns its locale,
	// which may be a fallback locale).
	t, err := s.store.GetLatestLegalText(ctx, db.GetLatestLegalTextParams{Kind: in.Kind, Locale: string(locale)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Consent{}, ErrStaleVersion
	}
	if err != nil {
		return Consent{}, err
	}
	if t.Version != in.Version {
		return Consent{}, ErrStaleVersion
	}
	c, err := s.store.InsertConsent(ctx, db.InsertConsentParams{
		UserID: userID, LegalTextID: t.ID, Kind: t.Kind, Locale: t.Locale, TextVersion: t.Version,
		Accepted: in.Accepted, Ip: optText(in.IP, 64), UserAgent: optText(in.UserAgent, 512),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Already answered: return the stored decision.
		c, err = s.store.GetConsentForText(ctx, db.GetConsentForTextParams{UserID: userID, LegalTextID: t.ID})
	}
	if err != nil {
		return Consent{}, err
	}
	return toConsent(c), nil
}

// AdminView is the editor payload: the latest text per locale plus history.
type AdminView struct {
	Kind     string `json:"kind"`
	Texts    []Text `json:"texts"`
	Versions []Text `json:"versions"`
}

// AdminGet returns the latest texts of kind for the editor.
func (s *Service) AdminGet(ctx context.Context, kind string) (AdminView, error) {
	if !IsKind(kind) {
		return AdminView{}, ErrNotFound
	}
	latest, err := s.store.ListLatestLegalTexts(ctx, kind)
	if err != nil {
		return AdminView{}, err
	}
	history, err := s.store.ListLegalTextVersions(ctx, db.ListLegalTextVersionsParams{Kind: kind, Limit: 50})
	if err != nil {
		return AdminView{}, err
	}
	view := AdminView{Kind: kind, Texts: make([]Text, 0, len(latest)), Versions: make([]Text, 0, len(history))}
	for _, t := range latest {
		view.Texts = append(view.Texts, toText(t))
	}
	for _, t := range history {
		view.Versions = append(view.Versions, toText(t))
	}
	return view, nil
}

// AdminPublish stores body as a new version for (kind, locale). An unchanged
// body is not a new version (no one is asked again for nothing).
func (s *Service) AdminPublish(ctx context.Context, kind, locale, body string, actorID *int64) (Text, bool, error) {
	if !IsKind(kind) {
		return Text{}, false, ErrNotFound
	}
	l, ok := i18n.Parse(locale)
	if !ok {
		return Text{}, false, fmt.Errorf("%w: unsupported locale", ErrInvalidRequest)
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > MaxBodyBytes {
		return Text{}, false, fmt.Errorf("%w: body is required (max 64 KiB)", ErrInvalidRequest)
	}
	cur, err := s.store.GetLatestLegalText(ctx, db.GetLatestLegalTextParams{Kind: kind, Locale: string(l)})
	if err == nil && strings.TrimSpace(cur.Body) == body {
		return toText(cur), false, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Text{}, false, err
	}
	var by pgtype.Int8
	if actorID != nil {
		by = pgtype.Int8{Int64: *actorID, Valid: true}
	}
	t, err := s.store.InsertLegalText(ctx, db.InsertLegalTextParams{Kind: kind, Locale: string(l), Body: body, CreatedBy: by})
	if err != nil {
		return Text{}, false, err
	}
	return toText(t), true, nil
}

func optText(s string, max int) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	if len(s) > max {
		s = s[:max]
	}
	return pgtype.Text{String: s, Valid: true}
}
