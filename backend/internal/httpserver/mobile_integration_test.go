package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
)

// fakePublisher records Centrifugo publishes (TEC-91 QR channel).
type fakePublisher struct {
	mu   sync.Mutex
	msgs []fakePublish
}

type fakePublish struct {
	Channel string
	Data    json.RawMessage
}

func (f *fakePublisher) Publish(_ context.Context, channel string, data any) error {
	b, _ := json.Marshal(data)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, fakePublish{Channel: channel, Data: b})
	return nil
}

func (f *fakePublisher) on(channel string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, m := range f.msgs {
		if m.Channel == channel {
			out = append(out, string(m.Data))
		}
	}
	return out
}

// doMobile calls a /v1/mobile/* route with the mobile API version header.
func (it *itest) doMobile(method, path, bearer, version string, body any) (int, envelope, http.Header) {
	it.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "OlexMobile/1.0")
	if version != "" {
		req.Header.Set("X-Mobile-Api-Version", version)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env, rec.Header()
}

func (it *itest) mobileLogin(email, pw, orgSlug, deviceID string) tokenPair {
	it.t.Helper()
	body := map[string]any{
		"email": email, "password": pw,
		"device": map[string]string{"id": deviceID, "name": "Pixel 9", "platform": "android", "app_version": "2.0.0"},
	}
	if orgSlug != "" {
		body["organization_slug"] = orgSlug
	}
	code, env, _ := it.doMobile("POST", "/v1/mobile/auth/login", "", "1", body)
	return it.tokensFrom(code, env)
}

// Acceptance 1, 2, 6: login, me, refresh rotation, reuse detection, org
// context, realm separation and the version gate.
func TestIntegrationMobileAuth(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	orgA := it.org("m91a", "dealer", center)
	orgB := it.org("m91b", "dealer", center)
	u, pw := it.user("mobile91")
	it.member(orgA, u, "owner")
	it.member(orgB, u, "staff")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM refresh_tokens WHERE user_id = $1", u.ID)
	})

	// 6. Version gate: missing and unsupported versions answer 426.
	for _, v := range []string{"", "0", "2", "abc"} {
		code, env, hdr := it.doMobile("POST", "/v1/mobile/auth/login", "", v, map[string]any{"email": "x", "password": "y"})
		if code != http.StatusUpgradeRequired || errCode(env) != "MOBILE_API_VERSION_UNSUPPORTED" || hdr.Get("X-Mobile-Api-Version") != "1" {
			t.Fatalf("version %q = %d %s (%q)", v, code, errCode(env), hdr.Get("X-Mobile-Api-Version"))
		}
	}
	if code, env, _ := it.doMobile("GET", "/v1/mobile/auth/me", "", "1", nil); code != http.StatusUnauthorized {
		t.Fatalf("me without bearer = %d %s", code, errCode(env))
	}
	if code, env, _ := it.doMobile("POST", "/v1/mobile/auth/login", "", "1", map[string]any{
		"email": u.Email.String, "password": pw, "device": map[string]string{"id": "", "platform": "android"},
	}); code != http.StatusBadRequest {
		t.Fatalf("login without device id = %d %s", code, errCode(env))
	}

	// 1. Login -> me with Bearer.
	tp := it.mobileLogin(u.Email.String, pw, orgA.Slug, "dev-1")
	claims, err := it.tokens.ParseAccess(tp.AccessToken)
	if err != nil || claims.Realm() != jwt.AudienceMobile {
		t.Fatalf("mobile token realm = %q (%v)", claims.Realm(), err)
	}
	if got := it.oid(tp.AccessToken); got != orgA.Uuid.String() {
		t.Fatalf("login oid = %s, want A", got)
	}
	code, env, _ := it.doMobile("GET", "/v1/mobile/auth/me", tp.AccessToken, "1", nil)
	if code != http.StatusOK {
		t.Fatalf("me = %d %s", code, errCode(env))
	}
	var me struct {
		User struct {
			Email string `json:"email"`
		} `json:"user"`
		ActiveOrganization string `json:"active_organization_uuid"`
		Session            *struct {
			Device struct {
				ID, Platform string
			} `json:"device"`
		} `json:"session"`
		CurrentWarehouse *json.RawMessage `json:"current_warehouse"`
	}
	_ = json.Unmarshal(env.Data, &me)
	if me.User.Email != u.Email.String || me.ActiveOrganization != orgA.Uuid.String() || me.Session == nil ||
		me.Session.Device.ID != "dev-1" || me.CurrentWarehouse != nil {
		t.Fatalf("me payload = %s", env.Data)
	}
	var client, platform, deviceName string
	if err := it.pool.QueryRow(ctx, `SELECT client, platform, device_name FROM refresh_tokens
		WHERE user_id = $1 AND revoked_at IS NULL AND device_id = 'dev-1'`, u.ID).Scan(&client, &platform, &deviceName); err != nil ||
		client != "mobile" || platform != "android" || deviceName != "Pixel 9" {
		t.Fatalf("refresh row = %s %s %s (%v)", client, platform, deviceName, err)
	}
	var ttlDays float64
	_ = it.pool.QueryRow(ctx, `SELECT EXTRACT(EPOCH FROM expires_at - created_at) / 86400 FROM refresh_tokens
		WHERE user_id = $1 AND revoked_at IS NULL AND device_id = 'dev-1'`, u.ID).Scan(&ttlDays)
	if ttlDays < 29.9 || ttlDays > 30.1 {
		t.Fatalf("mobile refresh ttl = %.2f days, want 30", ttlDays)
	}

	// Realm separation: mobile token off /v1/mobile/*, panel token on it.
	if code, env := it.do("GET", "/v1/auth/me", hostOlex, tp.AccessToken, nil); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("mobile token on panel me = %d %s", code, errCode(env))
	}
	panel := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{"email": u.Email.String, "password": pw}))
	if code, env, _ := it.doMobile("GET", "/v1/mobile/auth/me", panel.AccessToken, "1", nil); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("panel token on mobile me = %d %s", code, errCode(env))
	}
	// A mobile refresh token never rotates on the web route (and vice versa).
	if code, _ := it.do("POST", "/v1/auth/refresh", hostOlex, "", map[string]string{"refresh_token": tp.RefreshToken}); code != http.StatusUnauthorized {
		t.Fatalf("mobile refresh on web route = %d", code)
	}
	if code, _, _ := it.doMobile("POST", "/v1/mobile/auth/refresh", "", "1", map[string]string{"refresh_token": panel.RefreshToken}); code != http.StatusUnauthorized {
		t.Fatalf("web refresh on mobile route = %d", code)
	}

	// 2. Organization context: member org updates oid, other org is 403.
	code, env, _ = it.doMobile("POST", "/v1/mobile/auth/organization-context", tp.AccessToken, "1",
		map[string]string{"organization_slug": orgB.Slug})
	switched := it.tokensFrom(code, env)
	if got := it.oid(switched.AccessToken); got != orgB.Uuid.String() {
		t.Fatalf("switch oid = %s, want B", got)
	}
	if code, _, _ := it.doMobile("GET", "/v1/mobile/auth/me", tp.AccessToken, "1", nil); code != http.StatusUnauthorized {
		t.Fatalf("old session after switch = %d", code)
	}
	stranger := it.org("m91c", "dealer", center)
	if code, env, _ := it.doMobile("POST", "/v1/mobile/auth/organization-context", switched.AccessToken, "1",
		map[string]string{"organization_slug": stranger.Slug}); code != http.StatusForbidden || errCode(env) != "NO_TENANT_MEMBERSHIP" {
		t.Fatalf("switch to non-member org = %d %s", code, errCode(env))
	}

	// 1. Refresh rotation keeps the org and the device chain.
	code, env, _ = it.doMobile("POST", "/v1/mobile/auth/refresh", "", "1",
		map[string]string{"refresh_token": switched.RefreshToken, "app_version": "2.1.0"})
	rotated := it.tokensFrom(code, env)
	if got := it.oid(rotated.AccessToken); got != orgB.Uuid.String() {
		t.Fatalf("refresh oid = %s, want B", got)
	}
	var families int
	var appVersion string
	_ = it.pool.QueryRow(ctx, `SELECT COUNT(DISTINCT family_id), MAX(app_version) FROM refresh_tokens
		WHERE user_id = $1 AND client = 'mobile' AND device_id = 'dev-1' AND created_at > NOW() - INTERVAL '1 hour'`, u.ID).Scan(&families, &appVersion)
	if families != 1 || appVersion != "2.1.0" {
		t.Fatalf("device chain = %d families, app %s", families, appVersion)
	}

	// 1. Reuse detection: the rotated token comes back -> the chain is revoked.
	if code, env, _ := it.doMobile("POST", "/v1/mobile/auth/refresh", "", "1",
		map[string]string{"refresh_token": switched.RefreshToken}); code != http.StatusUnauthorized || errCode(env) != "REFRESH_TOKEN_REUSED" {
		t.Fatalf("reuse = %d %s", code, errCode(env))
	}
	if code, _, _ := it.doMobile("POST", "/v1/mobile/auth/refresh", "", "1",
		map[string]string{"refresh_token": rotated.RefreshToken}); code != http.StatusUnauthorized {
		t.Fatalf("chain survivor refresh after reuse = %d", code)
	}
	if code, _, _ := it.doMobile("GET", "/v1/mobile/auth/me", rotated.AccessToken, "1", nil); code != http.StatusUnauthorized {
		t.Fatalf("chain access after reuse = %d", code)
	}
	// The panel session of the same user is untouched.
	if code, env := it.do("GET", "/v1/auth/me", hostOlex, panel.AccessToken, nil); code != http.StatusOK {
		t.Fatalf("panel after mobile reuse = %d %s", code, errCode(env))
	}

	// A second sign-in on the same device ends the previous device session.
	first := it.mobileLogin(u.Email.String, pw, "", "dev-2")
	second := it.mobileLogin(u.Email.String, pw, "", "dev-2")
	if code, _, _ := it.doMobile("GET", "/v1/mobile/auth/me", first.AccessToken, "1", nil); code != http.StatusUnauthorized {
		t.Fatalf("older device session = %d", code)
	}

	// Measurement contract: 501 until F3.
	if code, env, _ := it.doMobile("POST", "/v1/mobile/measurements", second.AccessToken, "1", map[string]any{"vin": "X"}); code != http.StatusNotImplemented {
		t.Fatalf("measurements = %d %s", code, errCode(env))
	}

	// Logout-all ends web and mobile sessions.
	if code, env, _ := it.doMobile("POST", "/v1/mobile/auth/logout-all", second.AccessToken, "1", nil); code != http.StatusOK {
		t.Fatalf("logout-all = %d %s", code, errCode(env))
	}
	if code, _ := it.do("GET", "/v1/auth/me", hostOlex, panel.AccessToken, nil); code != http.StatusUnauthorized {
		t.Fatalf("panel after logout-all = %d", code)
	}
	var active int
	_ = it.pool.QueryRow(ctx, "SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL", u.ID).Scan(&active)
	if active != 0 {
		t.Fatalf("active sessions after logout-all = %d", active)
	}
}

// Acceptance 3: push token register / delete land in device_push_tokens;
// logout drops the device's tokens.
func TestIntegrationMobilePushToken(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	u, pw := it.user("push91", "dealer_owner")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM refresh_tokens WHERE user_id = $1", u.ID)
	})
	tp := it.mobileLogin(u.Email.String, pw, "", "dev-push-"+it.suffix)
	token := "ExponentPushToken[t91-" + it.suffix + "]"

	if code, env, _ := it.doMobile("PUT", "/v1/mobile/push-token", tp.AccessToken, "1", map[string]string{"expo_push_token": "nope"}); code != http.StatusBadRequest {
		t.Fatalf("invalid token = %d %s", code, errCode(env))
	}
	for i := 0; i < 2; i++ { // idempotent
		if code, env, _ := it.doMobile("PUT", "/v1/mobile/push-token", tp.AccessToken, "1",
			map[string]string{"expo_push_token": token, "platform": "android", "device_name": "Pixel 9"}); code != http.StatusOK {
			t.Fatalf("put push token = %d %s", code, errCode(env))
		}
	}
	var n int
	var deviceID string
	_ = it.pool.QueryRow(ctx, `SELECT COUNT(*), MAX(device_id) FROM device_push_tokens
		WHERE expo_token = $1 AND user_id = $2 AND revoked_at IS NULL`, token, u.ID).Scan(&n, &deviceID)
	if n != 1 || deviceID != "dev-push-"+it.suffix {
		t.Fatalf("push rows = %d device %q", n, deviceID)
	}
	if code, env, _ := it.doMobile("DELETE", "/v1/mobile/push-token", tp.AccessToken, "1", map[string]string{"expo_push_token": token}); code != http.StatusOK {
		t.Fatalf("delete push token = %d %s", code, errCode(env))
	}
	_ = it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM device_push_tokens WHERE expo_token = $1 AND revoked_at IS NOT NULL`, token).Scan(&n)
	if n != 1 {
		t.Fatalf("push token not revoked")
	}

	// Logout drops the device's push tokens and ends the session.
	if code, _, _ := it.doMobile("PUT", "/v1/mobile/push-token", tp.AccessToken, "1", map[string]string{"expo_push_token": token}); code != http.StatusOK {
		t.Fatalf("re-register = %d", code)
	}
	if code, env, _ := it.doMobile("POST", "/v1/mobile/auth/logout", tp.AccessToken, "1", nil); code != http.StatusOK {
		t.Fatalf("logout = %d %s", code, errCode(env))
	}
	_ = it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM device_push_tokens WHERE expo_token = $1 AND revoked_at IS NULL`, token).Scan(&n)
	if n != 0 {
		t.Fatalf("push token survives logout")
	}
	if code, _, _ := it.doMobile("GET", "/v1/mobile/auth/me", tp.AccessToken, "1", nil); code != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d", code)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM device_push_tokens WHERE expo_token = $1", token)
	})
}

// Acceptance 4: QR web sign-in start -> scan -> approve -> complete; reject
// -> 403; expired -> 410; Centrifugo publishes {status} only.
func TestIntegrationQRLogin(t *testing.T) {
	pub := &fakePublisher{}
	it := newIntegrationWithDeps(t, func(c *config.Config) {
		c.Centrifugo.Enabled = true
		c.Centrifugo.TokenHMAC = "t91-centrifugo-hmac"
		c.Centrifugo.WSURL = "ws://centrifugo/connection/websocket"
		c.Centrifugo.TokenTTL = time.Hour
	}, func(d *Deps) { d.Realtime = pub })
	ctx := context.Background()
	center := it.brandCenter("olex")
	org := it.org("qr91", "dealer", center)
	u, pw := it.user("qr91")
	it.member(org, u, "owner")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM refresh_tokens WHERE user_id = $1", u.ID)
	})
	mobile := it.mobileLogin(u.Email.String, pw, org.Slug, "dev-qr")

	type start struct {
		Code     string `json:"code"`
		Secret   string `json:"secret"`
		Payload  string `json:"payload"`
		Realtime struct {
			Enabled           bool   `json:"enabled"`
			Channel           string `json:"channel"`
			Token             string `json:"token"`
			SubscriptionToken string `json:"subscription_token"`
		} `json:"realtime"`
	}
	begin := func() start {
		t.Helper()
		code, env := it.do("POST", "/v1/auth/qr/start", hostOlex, "", nil)
		if code != http.StatusCreated {
			t.Fatalf("qr start = %d %s", code, errCode(env))
		}
		var s start
		_ = json.Unmarshal(env.Data, &s)
		if s.Code == "" || s.Secret == "" || s.Payload != "olexfilms://qr-login/"+s.Code {
			t.Fatalf("qr start payload = %s", env.Data)
		}
		return s
	}

	s := begin()
	if !s.Realtime.Enabled || s.Realtime.Channel != "qr:"+s.Code || s.Realtime.Token == "" || s.Realtime.SubscriptionToken == "" {
		t.Fatalf("qr realtime = %+v", s.Realtime)
	}
	if code, env := it.do("GET", "/v1/auth/qr/"+s.Code+"/status", hostOlex, "", nil); code != http.StatusOK || !strings.Contains(string(env.Data), `"pending"`) {
		t.Fatalf("status = %d %s", code, env.Data)
	}
	// Not approved yet: complete is refused.
	if code, env := it.do("POST", "/v1/auth/qr/complete", hostOlex, "", map[string]string{"code": s.Code, "secret": s.Secret}); code != http.StatusConflict {
		t.Fatalf("complete before approve = %d %s", code, errCode(env))
	}
	// A panel token cannot approve (mobile realm only).
	panel := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{"email": u.Email.String, "password": pw}))
	if code, _, _ := it.doMobile("POST", "/v1/mobile/auth/qr/"+s.Code+"/approve", panel.AccessToken, "1", nil); code != http.StatusForbidden {
		t.Fatalf("panel approve = %d", code)
	}
	code, env, _ := it.doMobile("GET", "/v1/mobile/auth/qr/"+s.Code, mobile.AccessToken, "1", nil)
	if code != http.StatusOK || !strings.Contains(string(env.Data), `"web_ip"`) || !strings.Contains(string(env.Data), `"scanned"`) {
		t.Fatalf("scan = %d %s", code, env.Data)
	}
	if code, env, _ := it.doMobile("POST", "/v1/mobile/auth/qr/"+s.Code+"/approve", mobile.AccessToken, "1", nil); code != http.StatusOK {
		t.Fatalf("approve = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/auth/qr/complete", hostOlex, "", map[string]string{"code": s.Code, "secret": "wrong"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("complete wrong secret = %d %s", code, errCode(env))
	}
	tp := it.tokensFrom(it.do("POST", "/v1/auth/qr/complete", hostOlex, "", map[string]string{"code": s.Code, "secret": s.Secret}))
	claims, err := it.tokens.ParseAccess(tp.AccessToken)
	if err != nil || claims.Realm() != jwt.AudiencePanel || claims.Subject != u.Uuid.String() {
		t.Fatalf("qr panel token = %+v (%v)", claims, err)
	}
	if got := it.oid(tp.AccessToken); got != org.Uuid.String() {
		t.Fatalf("qr panel oid = %s", got)
	}
	if code, env := it.do("GET", "/v1/auth/me", hostOlex, tp.AccessToken, nil); code != http.StatusOK {
		t.Fatalf("panel me via qr = %d %s", code, errCode(env))
	}
	if code, _ := it.do("POST", "/v1/auth/qr/complete", hostOlex, "", map[string]string{"code": s.Code, "secret": s.Secret}); code != http.StatusConflict {
		t.Fatalf("second complete = %d", code)
	}
	got := pub.on("qr:" + s.Code)
	want := []string{`{"status":"scanned"}`, `{"status":"approved"}`, `{"status":"consumed"}`}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("qr publishes = %v, want %v", got, want)
	}

	// Reject -> complete 403.
	r := begin()
	if code, env, _ := it.doMobile("POST", "/v1/mobile/auth/qr/"+r.Code+"/reject", mobile.AccessToken, "1", nil); code != http.StatusOK {
		t.Fatalf("reject = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/auth/qr/complete", hostOlex, "", map[string]string{"code": r.Code, "secret": r.Secret}); code != http.StatusForbidden || errCode(env) != "QR_LOGIN_REJECTED" {
		t.Fatalf("complete after reject = %d %s", code, errCode(env))
	}
	if got := pub.on("qr:" + r.Code); len(got) != 1 || got[0] != `{"status":"rejected"}` {
		t.Fatalf("reject publishes = %v", got)
	}

	// Expired -> 410 everywhere.
	e := begin()
	if _, err := it.pool.Exec(ctx, "UPDATE qr_login_challenges SET expires_at = NOW() - INTERVAL '1 second' WHERE code = $1", e.Code); err != nil {
		t.Fatal(err)
	}
	if code, _ := it.do("GET", "/v1/auth/qr/"+e.Code+"/status", hostOlex, "", nil); code != http.StatusGone {
		t.Fatalf("expired status = %d", code)
	}
	if code, env, _ := it.doMobile("POST", "/v1/mobile/auth/qr/"+e.Code+"/approve", mobile.AccessToken, "1", nil); code != http.StatusGone {
		t.Fatalf("expired approve = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/auth/qr/complete", hostOlex, "", map[string]string{"code": e.Code, "secret": e.Secret}); code != http.StatusGone || errCode(env) != "QR_LOGIN_EXPIRED" {
		t.Fatalf("expired complete = %d %s", code, errCode(env))
	}
	if code, _ := it.do("GET", "/v1/auth/qr/unknown-code/status", hostOlex, "", nil); code != http.StatusNotFound {
		t.Fatalf("unknown code = %d", code)
	}
	_, _ = it.pool.Exec(context.Background(), "DELETE FROM qr_login_challenges WHERE code = ANY($1)", []string{s.Code, r.Code, e.Code})
}
