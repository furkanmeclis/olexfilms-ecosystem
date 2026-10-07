package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type memStore struct {
	texts    []db.LegalText
	consents []db.Consent
}

func (m *memStore) latest(kind, locale string) (db.LegalText, bool) {
	var best db.LegalText
	found := false
	for _, t := range m.texts {
		if t.Kind == kind && t.Locale == locale && (!found || t.Version > best.Version) {
			best, found = t, true
		}
	}
	return best, found
}

func (m *memStore) GetLatestLegalText(_ context.Context, a db.GetLatestLegalTextParams) (db.LegalText, error) {
	if t, ok := m.latest(a.Kind, a.Locale); ok {
		return t, nil
	}
	return db.LegalText{}, pgx.ErrNoRows
}

func (m *memStore) ListLatestLegalTexts(_ context.Context, kind string) ([]db.LegalText, error) {
	out := []db.LegalText{}
	for _, l := range []string{"en", "tr"} {
		if t, ok := m.latest(kind, l); ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *memStore) ListLegalTextVersions(_ context.Context, _ db.ListLegalTextVersionsParams) ([]db.LegalText, error) {
	return m.texts, nil
}

func (m *memStore) InsertLegalText(_ context.Context, a db.InsertLegalTextParams) (db.LegalText, error) {
	v := int32(1)
	if t, ok := m.latest(a.Kind, a.Locale); ok {
		v = t.Version + 1
	}
	t := db.LegalText{ID: int64(len(m.texts) + 1), Uuid: uuid.New(), Kind: a.Kind, Locale: a.Locale, Version: v, Body: a.Body}
	m.texts = append(m.texts, t)
	return t, nil
}

func (m *memStore) GetConsentForText(_ context.Context, a db.GetConsentForTextParams) (db.Consent, error) {
	for _, c := range m.consents {
		if c.UserID == a.UserID && c.LegalTextID == a.LegalTextID {
			return c, nil
		}
	}
	return db.Consent{}, pgx.ErrNoRows
}

func (m *memStore) InsertConsent(ctx context.Context, a db.InsertConsentParams) (db.Consent, error) {
	if _, err := m.GetConsentForText(ctx, db.GetConsentForTextParams{UserID: a.UserID, LegalTextID: a.LegalTextID}); err == nil {
		return db.Consent{}, pgx.ErrNoRows
	}
	c := db.Consent{ID: int64(len(m.consents) + 1), Uuid: uuid.New(), UserID: a.UserID, LegalTextID: a.LegalTextID,
		Kind: a.Kind, Locale: a.Locale, TextVersion: a.TextVersion, Accepted: a.Accepted, Ip: a.Ip}
	m.consents = append(m.consents, c)
	return c, nil
}

func TestConsentAskedOncePerVersion(t *testing.T) {
	ctx := context.Background()
	store := &memStore{}
	svc := New(store)
	if _, _, err := svc.AdminPublish(ctx, KindAIGuidelines, "tr", "metin v1", nil); err != nil {
		t.Fatal(err)
	}
	pending, err := svc.Pending(ctx, 7, i18n.Locale("tr"))
	if err != nil || len(pending) != 1 || pending[0].Version != 1 {
		t.Fatalf("pending = %+v %v", pending, err)
	}
	// A decline is a decision: not asked again.
	c, err := svc.Decide(ctx, 7, DecideInput{Kind: KindAIGuidelines, Locale: "tr", Version: 1, Accepted: false, IP: "10.0.0.1"})
	if err != nil || c.Accepted {
		t.Fatalf("decide = %+v %v", c, err)
	}
	if p, _ := svc.Pending(ctx, 7, i18n.Locale("tr")); len(p) != 0 {
		t.Fatalf("asked twice: %+v", p)
	}
	// Answering again keeps the first decision.
	c2, err := svc.Decide(ctx, 7, DecideInput{Kind: KindAIGuidelines, Locale: "tr", Version: 1, Accepted: true})
	if err != nil || c2.Accepted || c2.UUID != c.UUID {
		t.Fatalf("second answer = %+v %v", c2, err)
	}
	// Same body: no new version. New body: asked again.
	if _, created, _ := svc.AdminPublish(ctx, KindAIGuidelines, "tr", " metin v1 ", nil); created {
		t.Fatal("unchanged body must not create a version")
	}
	if _, created, _ := svc.AdminPublish(ctx, KindAIGuidelines, "tr", "metin v2", nil); !created {
		t.Fatal("new body must create a version")
	}
	pending, _ = svc.Pending(ctx, 7, i18n.Locale("tr"))
	if len(pending) != 1 || pending[0].Version != 2 {
		t.Fatalf("pending after bump = %+v", pending)
	}
	if _, err := svc.Decide(ctx, 7, DecideInput{Kind: KindAIGuidelines, Locale: "tr", Version: 1, Accepted: true}); !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("stale version err = %v", err)
	}
}

func TestPendingFallsBackToDefaultLocale(t *testing.T) {
	ctx := context.Background()
	store := &memStore{}
	svc := New(store)
	_, _, _ = svc.AdminPublish(ctx, KindAIGuidelines, "tr", "metin", nil)
	pending, err := svc.Pending(ctx, 1, i18n.Locale("de"))
	if err != nil || len(pending) != 1 || pending[0].Locale != "tr" {
		t.Fatalf("fallback pending = %+v %v", pending, err)
	}
	if _, err := svc.Decide(ctx, 1, DecideInput{Kind: KindAIGuidelines, Locale: "tr", Version: 1, Accepted: true}); err != nil {
		t.Fatal(err)
	}
	if p, _ := svc.Pending(ctx, 1, i18n.Locale("de")); len(p) != 0 {
		t.Fatalf("fallback answered but pending: %+v", p)
	}
}

func TestAdminPublishValidation(t *testing.T) {
	svc := New(&memStore{})
	ctx := context.Background()
	if _, _, err := svc.AdminPublish(ctx, "unknown", "tr", "x", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown kind: %v", err)
	}
	if _, _, err := svc.AdminPublish(ctx, KindAIGuidelines, "xx", "x", nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("bad locale: %v", err)
	}
	if _, _, err := svc.AdminPublish(ctx, KindAIGuidelines, "tr", "   ", nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty body: %v", err)
	}
}

// TEC-404: marketing_consent is an editable, answerable kind but never part
// of the blocking pending prompt (explicit opt-in, unchecked by default).
func TestMarketingConsentKind(t *testing.T) {
	if !IsKind(KindMarketingConsent) || IsKind("newsletter") {
		t.Fatal("IsKind")
	}
	ctx := context.Background()
	svc := New(&memStore{})
	if _, _, err := svc.AdminPublish(ctx, KindMarketingConsent, "tr", "izin metni", nil); err != nil {
		t.Fatal(err)
	}
	pending, err := svc.Pending(ctx, 7, i18n.Locale("tr"))
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending = %+v %v", pending, err)
	}
	c, err := svc.Decide(ctx, 7, DecideInput{Kind: KindMarketingConsent, Locale: "tr", Version: 1, Accepted: true})
	if err != nil || c.Kind != KindMarketingConsent || !c.Accepted {
		t.Fatalf("decide = %+v %v", c, err)
	}
}
