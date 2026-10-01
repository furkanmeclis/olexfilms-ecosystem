package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type meLocale struct {
	User struct {
		Locale   *string `json:"locale"`
		Timezone *string `json:"timezone"`
	} `json:"user"`
	EffectiveLocale   string `json:"effective_locale"`
	EffectiveTimezone string `json:"effective_timezone"`
}

// meLocale calls an endpoint that returns Me and decodes its locale fields.
func (it *itest) meLocale(method, path, host, bearer string, body any) meLocale {
	it.t.Helper()
	code, env := it.do(method, path, host, bearer, body)
	if code != http.StatusOK {
		it.t.Fatalf("%s %s: status %d (%+v)", method, path, code, env.Error)
	}
	var me meLocale
	if err := json.Unmarshal(env.Data, &me); err != nil {
		it.t.Fatalf("me payload: %v", err)
	}
	return me
}

// TEC-136: users.locale NULL inherits the org; PATCH normalizes zh_CN and
// rejects unknown codes / timezones with 422; DB CHECKs hold the 13 codes.
func TestIntegrationLocaleTimezone(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	if center.Locale != "tr" {
		t.Fatalf("center locale = %q, want tr (migrated from tr-TR)", center.Locale)
	}
	org, err := it.q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t136-de-" + it.suffix, Name: "DE " + it.suffix, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "dealer", ParentID: pgtype.Int8{Int64: center.ID, Valid: true},
		BrandID: center.BrandID, Currency: "EUR", Locale: "de", Timezone: "Europe/Berlin",
		Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM refresh_tokens WHERE organization_id = $1", org.ID)
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
	})
	u, pw := it.user("locale")
	it.member(org, u, "owner")
	if u.Locale.Valid {
		t.Fatalf("new user locale = %q, want NULL", u.Locale.String)
	}

	// Without an organization: the brand center (tr / Istanbul).
	plain := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": u.Email, "password": pw,
	}))
	me := it.meLocale("GET", "/v1/auth/me", hostOlex, plain.AccessToken, nil)
	if me.EffectiveLocale != "tr" || me.EffectiveTimezone != "Europe/Istanbul" {
		t.Fatalf("no-org effective = %q / %q", me.EffectiveLocale, me.EffectiveTimezone)
	}

	// Acceptance 2: NULL user locale inherits the active organization.
	tp := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": u.Email, "password": pw, "organization_slug": org.Slug,
	}))
	me = it.meLocale("GET", "/v1/auth/me", hostOlex, tp.AccessToken, nil)
	if me.User.Locale != nil || me.User.Timezone != nil {
		t.Fatalf("user locale/timezone = %v / %v, want null", me.User.Locale, me.User.Timezone)
	}
	if me.EffectiveLocale != "de" || me.EffectiveTimezone != "Europe/Berlin" {
		t.Fatalf("org effective = %q / %q, want de / Europe/Berlin", me.EffectiveLocale, me.EffectiveTimezone)
	}

	// Acceptance 1 + 3: zh_CN stored as zh-CN, Asia/Dubai effective.
	me = it.meLocale("PATCH", "/v1/auth/profile", hostOlex, tp.AccessToken, map[string]string{
		"locale": "zh_CN", "timezone": "Asia/Dubai",
	})
	if me.EffectiveLocale != "zh-CN" || me.EffectiveTimezone != "Asia/Dubai" {
		t.Fatalf("patched effective = %q / %q", me.EffectiveLocale, me.EffectiveTimezone)
	}
	var storedLocale, storedTZ string
	if err := it.pool.QueryRow(ctx, "SELECT locale, timezone FROM users WHERE id = $1", u.ID).Scan(&storedLocale, &storedTZ); err != nil {
		t.Fatal(err)
	}
	if storedLocale != "zh-CN" || storedTZ != "Asia/Dubai" {
		t.Fatalf("stored = %q / %q", storedLocale, storedTZ)
	}

	for _, body := range []map[string]string{{"locale": "xx"}, {"locale": "klingon"}, {"timezone": "Mars/Olympus"}} {
		code, env := it.do("PATCH", "/v1/auth/profile", hostOlex, tp.AccessToken, body)
		if code != http.StatusUnprocessableEntity || errCode(env) != "VALIDATION_ERROR" {
			t.Fatalf("PATCH %v = %d %s, want 422 VALIDATION_ERROR", body, code, errCode(env))
		}
	}

	// Empty clears back to inherit.
	me = it.meLocale("PATCH", "/v1/auth/profile", hostOlex, tp.AccessToken, map[string]string{"locale": "", "timezone": ""})
	if me.User.Locale != nil || me.EffectiveLocale != "de" || me.EffectiveTimezone != "Europe/Berlin" {
		t.Fatalf("cleared = %v %q / %q", me.User.Locale, me.EffectiveLocale, me.EffectiveTimezone)
	}

	// DB CHECKs: only canonical codes.
	for _, stmt := range []struct{ sql, constraint string }{
		{"UPDATE users SET locale = 'zh_CN' WHERE id = $1", "chk_users_locale"},
		{"UPDATE users SET locale = 'tr-TR' WHERE id = $1", "chk_users_locale"},
		{"UPDATE organizations SET locale = 'tr-TR' WHERE id = $1", "chk_organizations_locale"},
	} {
		id := u.ID
		if strings.HasPrefix(stmt.sql, "UPDATE organizations") {
			id = org.ID
		}
		_, err := it.pool.Exec(ctx, stmt.sql, id)
		if err == nil || !strings.Contains(err.Error(), stmt.constraint) {
			t.Fatalf("%s: err = %v, want %s violation", stmt.sql, err, stmt.constraint)
		}
	}
	if _, err := it.pool.Exec(ctx, "UPDATE users SET locale = 'ar' WHERE id = $1", u.ID); err != nil {
		t.Fatalf("ar must pass the CHECK: %v", err)
	}
}
