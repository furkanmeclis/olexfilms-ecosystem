package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/wuzapi"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	itWuzapiAdmin  = "it-admin-token"
	itWuzapiUser   = "it-user-token"
	itWebhookKey   = "it-webhook-secret-0123456789abcdef0123"
	itWebhookURL   = "http://backend.olexfilms-app:8080/hooks/wuzapi"
	itInstanceName = "olexfilms"
)

// fakeWuzapi imitates the wuzapi 1.0.9 HTTP API (admin, session, send).
type fakeWuzapi struct {
	mu        sync.Mutex
	users     []map[string]any
	connected bool
	loggedIn  bool
	sends     []map[string]any
	fail      bool
}

func (f *fakeWuzapi) lastSend() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sends) == 0 {
		return nil
	}
	return f.sends[len(f.sends)-1]
}

func (f *fakeWuzapi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(code int, data any) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "data": data, "success": code < 300})
	}
	failWith := func(code int, msg string) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "error": msg, "success": false})
	}
	if strings.HasPrefix(r.URL.Path, "/admin/") {
		if r.Header.Get("Authorization") != itWuzapiAdmin {
			failWith(401, "unauthorized")
			return
		}
		if r.Method == http.MethodGet {
			reply(200, f.users)
			return
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		// Always hand out the fixed token so a DB row from an earlier run
		// keeps working.
		u := map[string]any{"id": "it1", "name": in["name"], "token": itWuzapiUser, "webhook": in["webhook"]}
		f.users = append(f.users, u)
		reply(201, u)
		return
	}
	if r.Header.Get("token") != itWuzapiUser {
		failWith(401, "unauthorized")
		return
	}
	switch r.URL.Path {
	case "/session/connect":
		f.connected = true
		reply(200, map[string]any{"details": "Connected!"})
	case "/session/qr":
		if !f.connected {
			failWith(500, "not connected")
			return
		}
		reply(200, map[string]any{"QRCode": "data:image/png;base64,SVQ="})
	case "/session/status":
		jid := ""
		if f.loggedIn {
			jid = "905320000001.0:1@s.whatsapp.net"
		}
		reply(200, map[string]any{"connected": f.connected, "loggedIn": f.loggedIn, "jid": jid})
	case "/session/logout":
		f.loggedIn = false
		reply(200, map[string]any{"Details": "Logged out"})
	case "/webhook":
		reply(200, map[string]any{"webhook": itWebhookURL})
	case "/chat/send/text":
		if f.fail {
			failWith(500, "error sending message")
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var in map[string]any
		_ = json.Unmarshal(raw, &in)
		f.sends = append(f.sends, in)
		id, _ := in["Id"].(string)
		if id == "" {
			id = fmt.Sprintf("SRV%d", len(f.sends))
		}
		reply(200, map[string]any{"Details": "Sent", "Id": id, "Timestamp": "2026-10-01T12:00:00+03:00"})
	default:
		failWith(404, "not found")
	}
}

func newWhatsAppIntegration(t *testing.T) (*itest, *fakeWuzapi) {
	t.Helper()
	fw := &fakeWuzapi{}
	srv := httptest.NewServer(fw)
	t.Cleanup(srv.Close)
	it := newIntegrationWith(t, func(c *config.Config) {
		c.Wuzapi = config.WuzapiConfig{URL: srv.URL, AdminToken: itWuzapiAdmin, WebhookSecret: itWebhookKey, WebhookURL: itWebhookURL}
	})
	// whatsapp_settings is a singleton shared with other packages' tests:
	// serialize on a session advisory lock (released on cleanup).
	lockConn, err := it.pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockConn.Exec(context.Background(), `SELECT pg_advisory_lock(920092)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock(920092)`)
		lockConn.Release()
	})
	// Fresh gateway singleton for this run.
	if _, err := it.pool.Exec(context.Background(), `UPDATE whatsapp_settings
		SET user_token_enc = NULL, instance_id = NULL, status = 'unknown', jid = NULL, phone_e164 = NULL,
		    last_error_reason = NULL, sms_fallback_enabled = FALSE, instance_name = $1 WHERE id = 1`, itInstanceName); err != nil {
		t.Fatal(err)
	}
	return it, fw
}

func itPhone() string { return fmt.Sprintf("+90533%07d", rand.IntN(10_000_000)) }

var itCodeRe = regexp.MustCompile(`\b(\d{6})\b`)

func sentCode(t *testing.T, fw *fakeWuzapi, e164 string) (string, string) {
	t.Helper()
	m := fw.lastSend()
	if m == nil {
		t.Fatal("nothing sent to wuzapi")
	}
	if m["Phone"] != strings.TrimPrefix(e164, "+") {
		t.Fatalf("sent to %v, want %s", m["Phone"], e164)
	}
	body, _ := m["Body"].(string)
	c := itCodeRe.FindStringSubmatch(body)
	if c == nil {
		t.Fatalf("no code in %q", body)
	}
	return c[1], body
}

func (it *itest) signedHook(body string, key string) (int, envelope) {
	it.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/hooks/wuzapi", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(wuzapi.SignatureHeader, wuzapi.Sign(key, []byte(body)))
	}
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

func (it *itest) waAdminToken() string {
	it.t.Helper()
	admin, pw := it.user("wa-admin", rbac.RoleSuperAdmin)
	return it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": pw,
	})).AccessToken
}

// Acceptance 1 + 2: QR connection flow and a test message (mock wuzapi).
func TestIntegrationWhatsAppAdmin(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	tok := it.waAdminToken()

	code, env := it.do("GET", "/v1/platform/whatsapp", hostOlex, tok, nil)
	if code != http.StatusOK {
		t.Fatalf("overview: %d %s", code, errCode(env))
	}
	var ov struct {
		Configured bool   `json:"configured"`
		Connected  bool   `json:"connected"`
		Status     string `json:"status"`
		KVKK       []struct {
			Locale  string `json:"locale"`
			Version int32  `json:"version"`
		} `json:"kvkk_notices"`
	}
	_ = json.Unmarshal(env.Data, &ov)
	if !ov.Configured || ov.Connected || len(ov.KVKK) < 2 {
		t.Fatalf("overview = %s", env.Data)
	}
	if code, env := it.do("POST", "/v1/platform/whatsapp/connect", hostOlex, tok, nil); code != http.StatusOK {
		t.Fatalf("connect: %d %s", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/platform/whatsapp/qr", hostOlex, tok, nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), "data:image/png;base64") {
		t.Fatalf("qr: %d %s", code, env.Data)
	}
	// The phone scans the QR: wuzapi reports LoggedIn and the QR call says so.
	fw.mu.Lock()
	fw.loggedIn = true
	fw.mu.Unlock()
	code, env = it.do("GET", "/v1/platform/whatsapp/qr", hostOlex, tok, nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), `"connected":true`) {
		t.Fatalf("qr after login: %d %s", code, env.Data)
	}
	_, env = it.do("GET", "/v1/platform/whatsapp", hostOlex, tok, nil)
	_ = json.Unmarshal(env.Data, &ov)
	if !ov.Connected || ov.Status != "connected" {
		t.Fatalf("connected overview = %s", env.Data)
	}

	to := itPhone()
	code, env = it.do("POST", "/v1/platform/whatsapp/test-message", hostOlex, tok, map[string]string{
		"phone": strings.Replace(to, "+90", "0", 1), "body": "Olexfilms test mesajı",
	})
	if code != http.StatusOK {
		t.Fatalf("test message: %d %s", code, errCode(env))
	}
	sent := fw.lastSend()
	if sent["Phone"] != strings.TrimPrefix(to, "+") || sent["Body"] != "Olexfilms test mesajı" {
		t.Fatalf("wuzapi body = %v", sent)
	}
	var outCount int64
	_ = it.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM messages m JOIN conversations c ON c.id = m.conversation_id
		WHERE c.contact_e164 = $1 AND m.direction = 'out' AND m.sender_type = 'staff'`, to).Scan(&outCount)
	if outCount != 1 {
		t.Fatalf("outbound message rows = %d", outCount)
	}

	// Settings: new KVKK text → version bump; SMS fallback switch.
	code, env = it.do("PUT", "/v1/platform/whatsapp/settings", hostOlex, tok, map[string]any{
		"sms_fallback_enabled": true,
		"kvkk_notices":         map[string]string{"tr": "KVKK: güncel aydınlatma metni " + it.suffix},
	})
	if code != http.StatusOK || !strings.Contains(string(env.Data), `"sms_fallback_enabled":true`) {
		t.Fatalf("settings: %d %s", code, env.Data)
	}
	n, err := it.q.GetLatestKVKKNotice(context.Background(), "tr")
	if err != nil || n.Version < 2 || !strings.Contains(n.Body, it.suffix) {
		t.Fatalf("kvkk tr = %+v %v", n, err)
	}
	if code, _ := it.do("PUT", "/v1/platform/whatsapp/settings", hostOlex, tok, map[string]any{
		"kvkk_notices": map[string]string{"xx": "x"},
	}); code != http.StatusBadRequest {
		t.Fatalf("unknown locale: %d", code)
	}

	// Without whatsapp.manage: 403.
	plain, ppw := it.user("wa-plain")
	ptok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{"email": plain.Email.String, "password": ppw})).AccessToken
	if code, _ := it.do("GET", "/v1/platform/whatsapp", hostOlex, ptok, nil); code != http.StatusForbidden {
		t.Fatalf("non-admin overview: %d", code)
	}
	if code, _ := it.do("POST", "/v1/platform/whatsapp/logout", hostOlex, tok, nil); code != http.StatusOK {
		t.Fatalf("logout: %d", code)
	}
}

// Acceptance 3: a customer receives an OTP over WhatsApp and signs in;
// tokens carry aud=portal and refresh keeps the realm.
func TestIntegrationOTPLogin(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	ctx := context.Background()
	ph := itPhone()

	code, env := it.do("POST", "/v1/auth/otp/request", hostOlex, "", map[string]string{"phone": ph, "purpose": "customer_login"})
	if code != http.StatusAccepted {
		t.Fatalf("otp request: %d %s", code, errCode(env))
	}
	var res struct {
		Channel   string `json:"channel"`
		ResendAt  string `json:"resend_at"`
		ExpiresAt string `json:"expires_at"`
	}
	_ = json.Unmarshal(env.Data, &res)
	if res.Channel != "whatsapp" || res.ResendAt == "" || res.ExpiresAt == "" {
		t.Fatalf("otp request body = %s", env.Data)
	}
	otpCode, body := sentCode(t, fw, ph)
	if !strings.Contains(body, "KVKK") {
		t.Fatalf("OTP text lacks the KVKK notice: %q", body)
	}

	// Second request inside 60 s → 429 with resend_at.
	code, env = it.do("POST", "/v1/auth/otp/request", hostOlex, "", map[string]string{"phone": ph})
	if code != http.StatusTooManyRequests || errCode(env) != "RATE_LIMITED" {
		t.Fatalf("cooldown: %d %s", code, errCode(env))
	}

	if code, env := it.do("POST", "/v1/auth/otp/verify", hostOlex, "", map[string]string{"phone": ph, "code": "999999x"}); code != http.StatusUnauthorized || errCode(env) != "INVALID_OTP_CODE" {
		t.Fatalf("wrong code: %d %s", code, errCode(env))
	}
	tp := it.tokensFrom(it.do("POST", "/v1/auth/otp/verify", hostOlex, "", map[string]string{
		"phone": ph, "code": otpCode, "purpose": "customer_login",
	}))
	claims, err := it.tokens.ParseAccess(tp.AccessToken)
	if err != nil || claims.Realm() != jwt.AudiencePortal {
		t.Fatalf("aud = %v %v", claims.Audience, err)
	}
	// Unknown phone → new customer user (email NULL, phone verified).
	u, err := it.q.GetUserByPhone(ctx, pgtype.Text{String: ph, Valid: true})
	if err != nil || u.Email.Valid || !u.PhoneVerifiedAt.Valid {
		t.Fatalf("customer user = %+v %v", u, err)
	}
	slugs, _ := it.q.ListUserRoleSlugs(ctx, u.ID)
	if len(slugs) != 1 || slugs[0] != rbac.RoleCustomer {
		t.Fatalf("customer roles = %v", slugs)
	}
	var realm string
	_ = it.pool.QueryRow(ctx, `SELECT realm FROM refresh_tokens WHERE user_id = $1 ORDER BY id DESC LIMIT 1`, u.ID).Scan(&realm)
	if realm != "portal" {
		t.Fatalf("refresh realm = %q", realm)
	}
	refreshed := it.tokensFrom(it.do("POST", "/v1/auth/refresh", hostOlex, "", map[string]string{"refresh_token": tp.RefreshToken}))
	if c, _ := it.tokens.ParseAccess(refreshed.AccessToken); c.Realm() != jwt.AudiencePortal {
		t.Fatalf("refresh lost the realm: %v", c.Audience)
	}

	// Panel login keeps aud=panel.
	staff, pw := it.user("wa-panel")
	panel := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{"email": staff.Email.String, "password": pw}))
	if c, _ := it.tokens.ParseAccess(panel.AccessToken); c.Realm() != jwt.AudiencePanel {
		t.Fatalf("panel aud = %v", c.Audience)
	}

	// Gateway down and no SMS fallback → 503 (no silent success).
	fw.mu.Lock()
	fw.fail = true
	fw.mu.Unlock()
	if code, env := it.do("POST", "/v1/auth/otp/request", hostOlex, "", map[string]string{"phone": itPhone()}); code != http.StatusServiceUnavailable || errCode(env) != "OTP_DELIVERY_FAILED" {
		t.Fatalf("gateway down: %d %s", code, errCode(env))
	}
	// SMS fallback on → channel sms.
	if _, err := it.q.SetWhatsAppSMSFallback(ctx, true); err != nil {
		t.Fatal(err)
	}
	code, env = it.do("POST", "/v1/auth/otp/request", hostOlex, "", map[string]string{"phone": itPhone()})
	if code != http.StatusAccepted || !strings.Contains(string(env.Data), `"channel":"sms"`) {
		t.Fatalf("sms fallback: %d %s", code, env.Data)
	}
	if code, _ := it.do("POST", "/v1/auth/otp/request", hostOlex, "", map[string]string{"phone": "12"}); code != http.StatusBadRequest {
		t.Fatalf("invalid phone: %d", code)
	}
}

// K26: an unverified user with the phone is claimed; a disabled one gets 403.
func TestIntegrationOTPClaimsAndDisabled(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	ctx := context.Background()

	ph := itPhone()
	existing, err := it.q.CreateUser(ctx, db.CreateUserParams{
		PhoneE164: pgtype.Text{String: ph, Valid: true}, PasswordHash: "x", Name: "Migrated", Surname: "Customer", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, env := it.do("POST", "/v1/auth/otp/request", hostOlex, "", map[string]string{"phone": ph}); code != http.StatusAccepted {
		t.Fatalf("request: %d %s", code, errCode(env))
	}
	c, _ := sentCode(t, fw, ph)
	tp := it.tokensFrom(it.do("POST", "/v1/auth/otp/verify", hostOlex, "", map[string]string{"phone": ph, "code": c}))
	claims, _ := it.tokens.ParseAccess(tp.AccessToken)
	if claims.Subject != existing.Uuid.String() {
		t.Fatalf("verify must claim the existing user, got %s", claims.Subject)
	}
	claimed, _ := it.q.GetUserByID(ctx, existing.ID)
	if !claimed.PhoneVerifiedAt.Valid {
		t.Fatal("claimed user must be phone-verified")
	}

	ph2 := itPhone()
	if _, err := it.q.CreateUser(ctx, db.CreateUserParams{
		PhoneE164: pgtype.Text{String: ph2, Valid: true}, PasswordHash: "x", Status: "disabled",
	}); err != nil {
		t.Fatal(err)
	}
	if code, _ := it.do("POST", "/v1/auth/otp/request", hostOlex, "", map[string]string{"phone": ph2}); code != http.StatusAccepted {
		t.Fatalf("request for disabled user must look the same: %d", code)
	}
	c2, _ := sentCode(t, fw, ph2)
	if code, env := it.do("POST", "/v1/auth/otp/verify", hostOlex, "", map[string]string{"phone": ph2, "code": c2}); code != http.StatusForbidden || errCode(env) != "FORBIDDEN" {
		t.Fatalf("disabled user verify: %d %s", code, errCode(env))
	}
}

const itMessage = `{"type":"Message","token":"it-user-token","event":{"Info":{"ID":"%s","Chat":"%s@s.whatsapp.net","Sender":"%s@s.whatsapp.net","IsFromMe":false,"IsGroup":false,"PushName":"Ayşe","Timestamp":"2026-10-01T12:00:00+03:00","Type":"text"},"Message":{"conversation":"Merhaba"}}}`

// Acceptance 4 + 5: inbound message lands once; bad signatures get 401;
// LoggedOut raises an alarm.
func TestIntegrationWuzapiWebhook(t *testing.T) {
	it, _ := newWhatsAppIntegration(t)
	ctx := context.Background()
	admin, _ := it.user("wa-alarm", rbac.RoleSuperAdmin)

	ph := itPhone()
	digits := strings.TrimPrefix(ph, "+")
	extID := "3EB0" + it.suffix
	body := fmt.Sprintf(itMessage, extID, digits, digits)

	if code, _ := it.signedHook(body, ""); code != http.StatusUnauthorized {
		t.Fatalf("unsigned: %d", code)
	}
	if code, _ := it.signedHook(body, "wrong-secret-0123456789abcdef0123456789"); code != http.StatusUnauthorized {
		t.Fatalf("wrong signature: %d", code)
	}
	code, env := it.signedHook(body, itWebhookKey)
	if code != http.StatusOK || !strings.Contains(string(env.Data), `"messages":1`) {
		t.Fatalf("message hook: %d %s", code, env.Data)
	}
	code, env = it.signedHook(body, itWebhookKey)
	if code != http.StatusOK || !strings.Contains(string(env.Data), `"duplicates":1`) {
		t.Fatalf("duplicate hook: %d %s", code, env.Data)
	}
	n, err := it.q.CountMessagesByExternalID(ctx, db.CountMessagesByExternalIDParams{Channel: "whatsapp", ExternalID: extID})
	if err != nil || n != 1 {
		t.Fatalf("message rows = %d %v", n, err)
	}
	var text, contact string
	_ = it.pool.QueryRow(ctx, `SELECT m.body, c.contact_e164 FROM messages m JOIN conversations c ON c.id = m.conversation_id WHERE m.external_id = $1`, extID).Scan(&text, &contact)
	if text != "Merhaba" || contact != ph {
		t.Fatalf("stored %q from %q", text, contact)
	}
	var outboxRows int
	_ = it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = 'whatsapp.message.received' AND payload->'data'->>'external_id' = $1`, extID).Scan(&outboxRows)
	if outboxRows != 1 {
		t.Fatalf("outbox rows = %d", outboxRows)
	}

	logout := `{"type":"LoggedOut","token":"it-user-token","event":{"OnConnect":false,"Reason":401}}`
	code, env = it.signedHook(logout, itWebhookKey)
	if code != http.StatusOK || !strings.Contains(string(env.Data), `"alarms":1`) {
		t.Fatalf("logout hook: %d %s", code, env.Data)
	}
	st, _ := it.q.GetWhatsAppSettings(ctx)
	if st.Status != "logged_out" || !strings.Contains(st.LastErrorReason.String, "logged_out") {
		t.Fatalf("settings after logout = %+v", st)
	}
	var alarms int
	_ = it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM whatsapp_connection_events WHERE type = 'loggedout' AND alarm`).Scan(&alarms)
	if alarms < 1 {
		t.Fatal("no alarm row")
	}
	var notes int
	_ = it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND source_event = 'whatsapp.connection.alarm'`, admin.ID).Scan(&notes)
	if notes < 1 {
		t.Fatal("admin was not notified")
	}
	ban := `{"type":"TemporaryBan","event":{"Code":101,"Expire":3600}}`
	if code, env := it.signedHook(ban, itWebhookKey); code != http.StatusOK || !strings.Contains(string(env.Data), `"alarms":1`) {
		t.Fatalf("ban hook: %d %s", code, env.Data)
	}
	if st, _ := it.q.GetWhatsAppSettings(ctx); st.Status != "banned" {
		t.Fatalf("status after ban = %s", st.Status)
	}
}
