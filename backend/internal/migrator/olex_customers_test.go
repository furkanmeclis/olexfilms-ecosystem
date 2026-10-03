package migrator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	authrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
)

// fixtureCustomerPhones are the E.164 numbers of the live fixture customers.
var fixtureCustomerPhones = []string{"+905551112233", "+905324445566", "+905419876543"}

type migratedCustomer struct {
	UserID                       int64
	UUID                         string
	Email, Phone, PhoneRaw       string
	Status                       string
	PhoneVerified, Legacy, Gone  bool
	ProfileType, Company, TaxNo4 string
	Prefs, Address               []byte
	Links                        int
	CustomerRole                 bool
}

// TEC-255 acceptance over the legacy fixture (8 customers, see
// testdata/legacy_hub.sql): two phone pairs collapse into one user each (6
// users), the phoneless and the unresolvable customer are unverified, the
// multi-dealer customer has two customer_organizations rows, a rerun writes
// nothing and an unverified record is claimed by OTP.
func TestOlexCustomers(t *testing.T) {
	e := newOrgsUsersEnv(t)
	ctx, pool := e.ctx, e.pool

	var maxUserID int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(MAX(id), 0) FROM users`).Scan(&maxUserID); err != nil {
		t.Fatal(err)
	}
	var preexisting int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND phone_e164 = ANY($1)`,
		fixtureCustomerPhones).Scan(&preexisting); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		exec := func(sql string, args ...any) {
			if _, err := pool.Exec(ctx, sql, args...); err != nil {
				t.Logf("cleanup %q: %v", sql, err)
			}
		}
		exec(`DELETE FROM customer_organizations WHERE organization_id IN (
			SELECT o.id FROM organizations o JOIN migration_map m ON m.target_uuid = o.uuid
			WHERE m.source_system = $1 AND m.target_table = 'organizations')`, e.system)
		exec(`DELETE FROM users WHERE id > $2 AND uuid IN (
			SELECT target_uuid FROM migration_map WHERE source_system = $1 AND source_table = 'customers')`, e.system, maxUserID)
	})

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	box, err := crypto.NewPIIBox(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	e.runner.Profiles["olexfx-customers"] = Profile{
		Name: "olexfx-customers", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step {
			return []Step{OrganizationsStep{System: e.system}, CustomersStep{System: e.system, PII: box}}
		},
	}

	first := e.run("olexfx-customers", Options{})
	cc := stepCounts(t, first, "customers")
	if cc[cntRead] != 8 || cc["unverified"] != 2 || cc["deleted"] != 1 {
		t.Errorf("customers counts = %v", cc)
	}
	// Another test may hold a fixture phone in the shared database; the step
	// then links to that account, so the exact split is checked only here.
	if preexisting == 0 && (cc[cntCreated] != 6 || cc["merged"] != 2) {
		t.Errorf("created / merged = %d / %d, want 6 / 2 (%v)", cc[cntCreated], cc["merged"], cc)
	}
	if cc["phone_unresolved:customer:6"] != 1 || cc["phone_missing"] != 1 || cc["tax_nos_set"] != 1 {
		t.Errorf("report keys = %v", cc)
	}

	load := func() map[string]migratedCustomer {
		t.Helper()
		rows, err := pool.Query(ctx, `
			SELECT m.source_id, u.id, u.uuid::text, COALESCE(u.email, ''), COALESCE(u.phone_e164, ''),
			       COALESCE(u.legacy_phone_raw, ''), u.status, u.phone_verified_at IS NOT NULL, u.legacy_unverified,
			       u.deleted_at IS NOT NULL, COALESCE(p.type, ''), COALESCE(p.company_name, ''),
			       COALESCE(p.tax_no_last4, ''), COALESCE(p.notification_prefs, '{}'::jsonb), COALESCE(p.address, '{}'::jsonb),
			       (SELECT COUNT(*) FROM customer_organizations co WHERE co.user_id = u.id),
			       EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
			               WHERE ur.user_id = u.id AND r.slug = 'customer')
			FROM migration_map m
			JOIN users u ON u.uuid = m.target_uuid
			LEFT JOIN customer_profiles p ON p.user_id = u.id
			WHERE m.source_system = $1 AND m.source_table = 'customers'`, e.system)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]migratedCustomer{}
		for rows.Next() {
			var id string
			var c migratedCustomer
			if err := rows.Scan(&id, &c.UserID, &c.UUID, &c.Email, &c.Phone, &c.PhoneRaw, &c.Status, &c.PhoneVerified,
				&c.Legacy, &c.Gone, &c.ProfileType, &c.Company, &c.TaxNo4, &c.Prefs, &c.Address, &c.Links,
				&c.CustomerRole); err != nil {
				t.Fatal(err)
			}
			out[id] = c
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	got := load()
	if len(got) != 8 {
		t.Fatalf("mapped customers = %d, want 8", len(got))
	}
	users := map[string]bool{}
	for id, c := range got {
		users[c.UUID] = true
		if !c.CustomerRole || c.ProfileType == "" || c.Links == 0 || c.PhoneVerified {
			t.Errorf("customer %s = %+v: want customer role, profile, a link and an unverified phone", id, c)
		}
	}
	// 8 customers, 2 pairs merged -> 6 users; several map rows share a uuid.
	if len(users) != 6 {
		t.Errorf("distinct users = %d, want 6", len(users))
	}
	if got["1"].UUID != got["2"].UUID || got["1"].Phone != "+905551112233" {
		t.Errorf("pair 1 not merged: %+v / %+v", got["1"], got["2"])
	}
	if got["3"].UUID != got["4"].UUID || got["3"].Phone != "+905324445566" {
		t.Errorf("pair 2 not merged: %+v / %+v", got["3"], got["4"])
	}
	// The multi-dealer customers: one customer_organizations row per dealer.
	if got["1"].Links != 2 || got["3"].Links != 2 {
		t.Errorf("multi-dealer links = %d / %d, want 2 / 2", got["1"].Links, got["3"].Links)
	}
	for _, id := range []string{"5", "6"} {
		c := got[id]
		if !c.Legacy || c.Phone != "" || c.Email != "" || c.PhoneVerified || c.Status != "active" || c.Links != 1 {
			t.Errorf("customer %s must be unverified and contactless: %+v", id, c)
		}
	}
	if got["5"].PhoneRaw != "" || got["6"].PhoneRaw != "12345" {
		t.Errorf("legacy raw phones = %q / %q", got["5"].PhoneRaw, got["6"].PhoneRaw)
	}
	if c := got["7"]; c.Legacy || c.Phone != "+905419876543" || c.Email != "kurumsal@example.test" ||
		c.ProfileType != "corporate" || c.Company != "Sentetik Kurumsal A.Ş." || c.TaxNo4 != "2222" {
		t.Errorf("corporate customer = %+v", c)
	}
	if c := got["8"]; !c.Gone || c.Status != "disabled" || c.Phone != "+4915123456789" {
		t.Errorf("deleted customer = %+v", c)
	}
	var prefs map[string]bool
	if err := json.Unmarshal(got["1"].Prefs, &prefs); err != nil || !prefs["sms"] || prefs["push"] || !prefs["whatsapp"] {
		t.Errorf("customer 1 prefs = %s (%v)", got["1"].Prefs, err)
	}
	var addr map[string]any
	if err := json.Unmarshal(got["1"].Address, &addr); err != nil || addr["city"] != "İstanbul" || addr["province_id"] == nil {
		t.Errorf("customer 1 address = %s (%v)", got["1"].Address, err)
	}

	// The narrow exception: only a legacy-marked row may lack both contacts.
	var pgErr *pgconn.PgError
	_, err = pool.Exec(ctx, `UPDATE users SET legacy_unverified = FALSE WHERE id = $1`, got["5"].UserID)
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "chk_users_email_or_phone" {
		t.Errorf("clearing the marker of a contactless user = %v, want chk_users_email_or_phone", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO users (password_hash, name, surname, status) VALUES ('x', 'a', 'b', 'active')`)
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "chk_users_email_or_phone" {
		t.Errorf("contactless non-legacy insert = %v, want chk_users_email_or_phone", err)
	}

	before := e.customerCounts()

	// Second run: nothing new.
	second := e.run("olexfx-customers", Options{})
	sc := stepCounts(t, second, "customers")
	for _, k := range []string{cntCreated, cntUpdated, "merged", "links_created", "profiles_created", "profiles_filled",
		"user_roles_created", "users_filled", "tax_nos_set", "national_ids_set"} {
		if sc[k] != 0 {
			t.Errorf("rerun customers.%s = %d, want 0 (%v)", k, sc[k], sc)
		}
	}
	if sc[cntUnchanged] != 8 {
		t.Errorf("rerun unchanged = %d, want 8", sc[cntUnchanged])
	}
	if after := e.customerCounts(); after != before {
		t.Errorf("rerun changed rows: %+v -> %+v", before, after)
	}

	// Claim (K26): an OTP-verified phone binds the migrated record through
	// the regular portal login instead of creating a new account.
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	repo := authrepo.NewPostgres(pool, db.New(pool))
	auth := authusecase.New(repo, tokens)
	auth.SetPhoneRepository(repo)
	claim := func(phone string, userID int64) {
		t.Helper()
		if _, created, err := auth.LoginWithVerifiedPhone(ctx, phone, "tr", model.SessionMeta{}); err != nil || created {
			t.Fatalf("OTP login %s = created %v, %v", phone, created, err)
		}
		var id int64
		var verified, legacy bool
		var raw *string
		if err := pool.QueryRow(ctx, `SELECT id, phone_verified_at IS NOT NULL, legacy_unverified, legacy_phone_raw
			FROM users WHERE phone_e164 = $1 AND deleted_at IS NULL`, phone).Scan(&id, &verified, &legacy, &raw); err != nil {
			t.Fatal(err)
		}
		if id != userID || !verified || legacy || raw != nil {
			t.Errorf("claim %s: user %d (want %d) verified %v legacy %v raw %v", phone, id, userID, verified, legacy, raw)
		}
	}
	if got["3"].UserID > maxUserID {
		claim("+905324445566", got["3"].UserID)
	}
	// The unresolvable record gets a phone (an operator fixes it), then the
	// first OTP on that number claims it and clears the legacy marker.
	phone := fmt.Sprintf("+90555%07d", time.Now().UnixNano()%10_000_000)
	if _, err := pool.Exec(ctx, `UPDATE users SET phone_e164 = $2 WHERE id = $1`, got["6"].UserID, phone); err != nil {
		t.Fatal(err)
	}
	claim(phone, got["6"].UserID)
}

type customerTableCounts struct{ Users, Profiles, Links, Roles, Maps int64 }

func (e *orgsUsersEnv) customerCounts() customerTableCounts {
	e.t.Helper()
	var c customerTableCounts
	err := e.pool.QueryRow(e.ctx, `
		WITH cu AS (SELECT DISTINCT u.id FROM users u JOIN migration_map m ON m.target_uuid = u.uuid
		            WHERE m.source_system = $1 AND m.source_table = 'customers')
		SELECT
		  (SELECT COUNT(*) FROM cu),
		  (SELECT COUNT(*) FROM customer_profiles WHERE user_id IN (SELECT id FROM cu)),
		  (SELECT COUNT(*) FROM customer_organizations WHERE user_id IN (SELECT id FROM cu)),
		  (SELECT COUNT(*) FROM user_roles WHERE user_id IN (SELECT id FROM cu)),
		  (SELECT COUNT(*) FROM migration_map WHERE source_system = $1)`, e.system).
		Scan(&c.Users, &c.Profiles, &c.Links, &c.Roles, &c.Maps)
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}
