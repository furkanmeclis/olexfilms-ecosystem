package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-464: the seed data contains the KVKK notice, AI guidelines and
// marketing consent in every supported locale. This test fails on the old
// tr/en-only seed.
func TestConsentLegalTextLocalesSeeded(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	locales := []string{"tr", "en", "bg", "de", "el", "uk", "ru", "fr", "es", "it", "zh-CN", "az", "ar"}
	for _, locale := range locales {
		t.Run(locale, func(t *testing.T) {
			var kvkk, ai, marketing int
			if err := pool.QueryRow(ctx, `
				SELECT
					(SELECT COUNT(*) FROM kvkk_notices
					 WHERE locale = $1 AND version = 1 AND btrim(body) <> ''),
					(SELECT COUNT(*) FROM legal_texts
					 WHERE kind = 'ai_guidelines' AND locale = $1 AND version = 1 AND btrim(body) <> ''),
					(SELECT COUNT(*) FROM legal_texts
					 WHERE kind = 'marketing_consent' AND locale = $1 AND version = 1 AND btrim(body) <> '')`,
				locale).Scan(&kvkk, &ai, &marketing); err != nil {
				t.Fatal(err)
			}
			if kvkk != 1 || ai != 1 || marketing != 1 {
				t.Fatalf("seed counts kvkk/ai/marketing = %d/%d/%d, want 1/1/1", kvkk, ai, marketing)
			}
		})
	}
}
