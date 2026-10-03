package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// memStore is an in-memory Store with the semantics of the SQL queries.
type memStore struct {
	rows map[string]*memRow
}

type memRow struct {
	brandID   int64
	target    string
	expiresAt pgtype.Timestamptz
	hits      int64
	lastHit   pgtype.Timestamptz
}

func newMemStore() *memStore { return &memStore{rows: map[string]*memRow{}} }

func (m *memStore) CreateShortURL(_ context.Context, arg db.CreateShortURLParams) (db.CreateShortURLRow, error) {
	if _, ok := m.rows[arg.Token]; ok {
		return db.CreateShortURLRow{}, pgx.ErrNoRows
	}
	m.rows[arg.Token] = &memRow{brandID: arg.BrandID, target: arg.TargetPath, expiresAt: arg.ExpiresAt}
	return db.CreateShortURLRow{
		Uuid: uuid.New(), Token: arg.Token, TargetPath: arg.TargetPath, ExpiresAt: arg.ExpiresAt,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}, nil
}

func (m *memStore) HitShortURL(_ context.Context, arg db.HitShortURLParams) (db.HitShortURLRow, error) {
	r, ok := m.rows[arg.Token]
	if !ok || r.brandID != arg.BrandID || (r.expiresAt.Valid && !r.expiresAt.Time.After(arg.Now.Time)) {
		return db.HitShortURLRow{}, pgx.ErrNoRows
	}
	r.hits++
	r.lastHit = arg.Now
	return db.HitShortURLRow{TargetPath: r.target, ExpiresAt: r.expiresAt}, nil
}

func (m *memStore) GetShortURLExpiry(_ context.Context, arg db.GetShortURLExpiryParams) (pgtype.Timestamptz, error) {
	r, ok := m.rows[arg.Token]
	if !ok || r.brandID != arg.BrandID {
		return pgtype.Timestamptz{}, pgx.ErrNoRows
	}
	return r.expiresAt, nil
}

func TestNormalizeTarget(t *testing.T) {
	ok := []string{
		"/portal", "/portal/services/abc", "/garanti/AbCdEfGh?lang=tr", "/bayi/olex-kadikoy#map",
		"  /garanti/X  ",
	}
	for _, in := range ok {
		if _, err := NormalizeTarget(in); err != nil {
			t.Errorf("%q rejected: %v", in, err)
		}
	}
	bad := []string{
		"", "https://evil.example/portal", "//evil.example/portal", "/\\evil.example",
		"javascript:alert(1)", "portal", "/admin", "/portalx", "/portal/../platform",
		"/portal/%2e%2e/platform", "/garanti/a b", "/garanti/\x00", "http:/portal",
		"/" + strings.Repeat("a", MaxTargetLength),
	}
	for _, in := range bad {
		_, err := NormalizeTarget(in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != "target" {
			t.Errorf("%q accepted or wrong error: %v", in, err)
		}
	}
}

func TestRandomToken(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		tok, err := RandomToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != TokenLength || !ValidToken(tok) || seen[tok] {
			t.Fatalf("token %q", tok)
		}
		seen[tok] = true
	}
}

func TestValidTokenAcceptsLegacyHubTokens(t *testing.T) {
	// Laravel Str::random(8): case-sensitive base62 (old hub /_/{token}).
	for _, tok := range []string{"aZ3kP9qX", "AbCdEfGhIj", "abcd", "A1b2C3d4E5f6G7h8"} {
		if !ValidToken(tok) {
			t.Errorf("%q rejected", tok)
		}
	}
	for _, tok := range []string{"", "abc", "A1b2C3d4E5f6G7h8X", "ab-cd", "ab_cd", "ab cd", "ab/cd"} {
		if ValidToken(tok) {
			t.Errorf("%q accepted", tok)
		}
	}
}

func TestCreateAndResolve(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	store := newMemStore()
	svc := New(store).WithClock(func() time.Time { return now })

	out, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Target: "/garanti/AbCdEfGh?lang=tr"})
	if err != nil {
		t.Fatal(err)
	}
	if out.ExpiresAt != nil || len(out.Token) != TokenLength {
		t.Fatalf("created %+v", out)
	}
	for i := 1; i <= 2; i++ {
		res, err := svc.Resolve(context.Background(), 1, out.Token)
		if err != nil {
			t.Fatal(err)
		}
		if res.TargetPath != "/garanti/AbCdEfGh?lang=tr" {
			t.Fatalf("target %q", res.TargetPath)
		}
		if got := store.rows[out.Token].hits; got != int64(i) {
			t.Fatalf("hit_count = %d, want %d", got, i)
		}
	}
	// Another brand's domain does not see the token.
	if _, err := svc.Resolve(context.Background(), 2, out.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other brand: %v", err)
	}
}

func TestResolveExpiredAndUnknown(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	store := newMemStore()
	svc := New(store).WithClock(func() time.Time { return now })
	out, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Target: "/portal", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if out.ExpiresAt == nil || !out.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("expires_at %v", out.ExpiresAt)
	}
	now = now.Add(time.Hour)
	if _, err := svc.Resolve(context.Background(), 1, out.Token); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: %v", err)
	}
	if store.rows[out.Token].hits != 0 {
		t.Fatal("expired hit counted")
	}
	for _, tok := range []string{"Unknown123", "bad/token", ""} {
		if _, err := svc.Resolve(context.Background(), 1, tok); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%q: %v", tok, err)
		}
	}
}

func TestCreateRejectsExternalTarget(t *testing.T) {
	svc := New(newMemStore())
	_, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Target: "https://evil.example/portal"})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Code != "invalid_target" {
		t.Fatalf("err = %v", err)
	}
	_, err = svc.Create(context.Background(), CreateInput{BrandID: 1, Target: "/portal", TTL: -time.Second})
	if !errors.As(err, &ve) || ve.Field != "ttl" {
		t.Fatalf("ttl err = %v", err)
	}
}

func TestCreateRedrawsTakenToken(t *testing.T) {
	store := newMemStore()
	store.rows["TakenToken"] = &memRow{brandID: 1, target: "/portal"}
	tokens := []string{"TakenToken", "FreshToken"}
	svc := New(store).WithTokens(func() (string, error) {
		tok := tokens[0]
		tokens = tokens[1:]
		return tok, nil
	})
	out, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Target: "/bayi/x"})
	if err != nil || out.Token != "FreshToken" {
		t.Fatalf("out %+v err %v", out, err)
	}
	svc.WithTokens(func() (string, error) { return "TakenToken", nil })
	if _, err := svc.Create(context.Background(), CreateInput{BrandID: 1, Target: "/bayi/x"}); err == nil {
		t.Fatal("endless conflict accepted")
	}
}

func TestLinker(t *testing.T) {
	svc := New(newMemStore()).WithTokens(func() (string, error) { return "AbCdEfGhIj", nil })
	link, err := NewLinker(svc, "https://olexfilms.app/").Link(context.Background(), CreateInput{BrandID: 1, Target: "/portal"})
	if err != nil || link != "https://olexfilms.app/s/AbCdEfGhIj" {
		t.Fatalf("link %q err %v", link, err)
	}
}
