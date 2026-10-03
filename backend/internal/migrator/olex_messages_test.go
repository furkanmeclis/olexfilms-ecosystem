package migrator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	legacyrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legacymessages/repository"
	shorthandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/handler"
	shorturls "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
)

type allowAll struct{}

func (allowAll) Allow(context.Context, string, string, int, time.Duration) (bool, time.Duration) {
	return true, 0
}

// fixtureShortTokens are the hub short_urls tokens of the fixture.
var fixtureShortTokens = []string{"synA1b2C3", "synD4e5F6", "synG7h8J9", "synK0m1N2"}

// TEC-263 acceptance over the legacy fixture. The whole run happens in one
// transaction that is rolled back: legacy_messages is append-only, so a
// committed run could not be cleaned up from the shared test database.
//
//   - old tokens resolve through GET /v1/public/short-urls/{token};
//   - the external target is not imported and is reported;
//   - legacy_messages per channel: sms 2, whatsapp 1, notification 2;
//   - UPDATE / DELETE are rejected; only the K19 function masks;
//   - a second run creates nothing.
func TestOlexShortURLsAndLegacyMessages(t *testing.T) {
	e := newOrgsUsersEnv(t)
	ctx, pool := e.ctx, e.pool

	var taken int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM short_urls WHERE token = ANY($1)`, fixtureShortTokens).Scan(&taken); err != nil {
		t.Fatal(err)
	}
	if taken != 0 {
		t.Fatalf("%d fixture short url tokens already exist in the test database", taken)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	e.runner.Pool = tx
	e.runner.Profiles["olexfx-messages"] = Profile{
		Name: "olexfx-messages", Enabled: true, Sources: []string{SourceHub, SourceWH},
		Steps: func() []Step {
			return []Step{
				OrganizationsStep{System: e.system},
				UsersStep{System: e.system},
				ShortURLsStep{System: e.system},
				LegacyMessagesStep{System: e.system},
			}
		},
	}

	first := e.run("olexfx-messages", Options{})
	sc := stepCounts(t, first, "short_urls")
	if sc[cntRead] != 4 || sc[cntCreated] != 3 || sc[ShortTargetUnmapped] != 1 || sc[ShortTargetUnmapped+":4"] != 1 {
		t.Errorf("short_urls counts = %v", sc)
	}
	mc := stepCounts(t, first, "legacy_messages")
	if mc["sms_logs_read"] != 3 || mc["notifications_read"] != 2 || mc[cntCreated] != 5 ||
		mc["created_sms"] != 2 || mc["created_whatsapp"] != 1 || mc["created_notification"] != 2 {
		t.Errorf("legacy_messages counts = %v", mc)
	}
	if mc["recipient_unresolved"] != 1 {
		t.Errorf("recipient_unresolved = %d, want 1 (%v)", mc["recipient_unresolved"], mc)
	}

	// Old tokens resolve through the public endpoint of the brand.
	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		t.Fatal(err)
	}
	h := shorthandler.NewPublic(shorturls.New(q), allowAll{}, 1000, time.Minute)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/public/short-urls/{token}", h.Get)
	resolve := func(token string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/public/short-urls/"+token, nil)
		req = req.WithContext(brandctx.WithBrand(req.Context(), brandctx.Brand{ID: brand.ID, Slug: brand.Slug}))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var env struct {
			Data struct {
				TargetPath string `json:"target_path"`
			} `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return rec.Code, env.Data.TargetPath
	}
	for token, want := range map[string]string{
		"synA1b2C3": "/garanti/SYN-S-0001",
		"synD4e5F6": "/bayi/SYN00001",
		"synG7h8J9": "/portal",
	} {
		if code, got := resolve(token); code != http.StatusOK || got != want {
			t.Errorf("resolve %s = %d %q, want 200 %q", token, code, got, want)
		}
	}
	if code, _ := resolve("synK0m1N2"); code != http.StatusNotFound {
		t.Errorf("unmapped external token = %d, want 404", code)
	}
	var legacyURL string
	if err := tx.QueryRow(ctx, `SELECT legacy_target_url FROM short_urls WHERE token = 'synG7h8J9'`).Scan(&legacyURL); err != nil {
		t.Fatal(err)
	}
	if legacyURL != "https://hub.example.test/customer/eyJpdiI6InN5bnRoZXRpYyJ9" {
		t.Errorf("legacy_target_url = %q", legacyURL)
	}

	// Per channel counts and the mapped columns.
	byChannel := map[string]int{}
	rows, err := tx.Query(ctx, `SELECT channel, COUNT(*) FROM legacy_messages WHERE source_system = $1 GROUP BY channel`, e.system)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var ch string
		var n int
		if err := rows.Scan(&ch, &n); err != nil {
			t.Fatal(err)
		}
		byChannel[ch] = n
	}
	rows.Close()
	if byChannel[ChannelSMS] != 2 || byChannel[ChannelWhatsApp] != 1 || byChannel[ChannelNotification] != 2 {
		t.Errorf("legacy_messages per channel = %v", byChannel)
	}
	type msg struct {
		Recipient, Raw, Body string
		UserMapped           bool
		Dealer               bool
	}
	read := func(table, id string) msg {
		t.Helper()
		var m msg
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(l.recipient, ''), COALESCE(l.payload->>'recipient_raw', ''), l.body, l.user_id IS NOT NULL,
			       o.type = 'dealer'
			FROM legacy_messages l JOIN organizations o ON o.id = l.organization_id
			WHERE l.source_system = $1 AND l.source_table = $2 AND l.source_id = $3`, e.system, table, id).
			Scan(&m.Recipient, &m.Raw, &m.Body, &m.UserMapped, &m.Dealer); err != nil {
			t.Fatalf("%s#%s: %v", table, id, err)
		}
		return m
	}
	if m := read("sms_logs", "1"); m.Recipient != "+905551112233" || m.Body != "Sentetik hizmet mesajı" || !m.Dealer {
		t.Errorf("sms 1 = %+v (want E.164 recipient, dealer organization)", m)
	}
	if m := read("sms_logs", "3"); m.Recipient != "" || m.Raw != "12345" {
		t.Errorf("sms 3 = %+v (want unresolved phone in payload)", m)
	}
	if m := read("notifications", "00000000-0000-4000-8000-000000000001"); !m.UserMapped || m.Body != "Sentetik bildirim" || !m.Dealer {
		t.Errorf("notification 1 = %+v (want mapped user, dealer organization)", m)
	}

	// Read-only repository: hub user 3's notification.
	var user3 int64
	if err := tx.QueryRow(ctx, `SELECT u.id FROM users u JOIN migration_map m ON m.target_uuid = u.uuid
		WHERE m.source_system = $1 AND m.source_table = 'users' AND m.source_id = '3'`, e.system).Scan(&user3); err != nil {
		t.Fatal(err)
	}
	list, err := legacyrepo.New(q).ListForPerson(ctx, brand.ID, &user3, "", nil, 10)
	if err != nil || len(list) != 1 || list[0].Channel != ChannelNotification {
		t.Errorf("ListForPerson(user 3) = %+v, %v", list, err)
	}

	// UPDATE and DELETE are rejected, also with the anonymization flag set
	// by hand when the values are not the masked ones.
	reject := func(name, sql string) {
		t.Helper()
		sp, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sp.Rollback(ctx) }()
		_, err = sp.Exec(ctx, sql, e.system)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23001" {
			t.Errorf("%s: err = %v, want restrict_violation", name, err)
		}
	}
	reject("update", `UPDATE legacy_messages SET body = 'x' WHERE source_system = $1`)
	reject("delete", `DELETE FROM legacy_messages WHERE source_system = $1`)
	reject("update with flag", `WITH f AS (SELECT set_config('olex.legacy_messages_anonymize', 'on', true))
		UPDATE legacy_messages SET body = 'x', anonymized_at = NOW() FROM f WHERE source_system = $1`)

	// K19: the anonymization masks the person's rows (by phone here) and
	// nothing else; a second call masks nothing.
	var masked int
	if err := tx.QueryRow(ctx, `SELECT anonymize_legacy_messages(NULL, '+905551112233')`).Scan(&masked); err != nil {
		t.Fatal(err)
	}
	if masked < 2 {
		t.Errorf("masked = %d, want >= 2 (sms 1, whatsapp 2)", masked)
	}
	if m := read("sms_logs", "2"); m.Recipient != "" || m.Body != "[anonymized]" {
		t.Errorf("whatsapp 2 after anonymization = %+v", m)
	}
	if m := read("sms_logs", "3"); m.Body != "Sentetik başarısız mesaj" {
		t.Errorf("sms 3 must stay = %+v", m)
	}
	if err := tx.QueryRow(ctx, `SELECT anonymize_legacy_messages(NULL, '+905551112233')`).Scan(&masked); err != nil {
		t.Fatal(err)
	}
	if masked != 0 {
		t.Errorf("second anonymization masked %d", masked)
	}
	reject("update after anonymization", `UPDATE legacy_messages SET body = 'x' WHERE source_system = $1`)

	// Second run: nothing new.
	second := e.run("olexfx-messages", Options{})
	sc = stepCounts(t, second, "short_urls")
	if sc[cntCreated] != 0 || sc[cntUpdated] != 0 || sc[cntUnchanged] != 3 {
		t.Errorf("short_urls rerun counts = %v", sc)
	}
	mc = stepCounts(t, second, "legacy_messages")
	if mc[cntCreated] != 0 || mc[cntUnchanged] != 5 {
		t.Errorf("legacy_messages rerun counts = %v", mc)
	}
	var total int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM legacy_messages WHERE source_system = $1`, e.system).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("legacy_messages after rerun = %d, want 5", total)
	}
}
