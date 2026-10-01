package wuzapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// mockWuzapi imitates the 1.0.9 endpoints with fixed answers.
type mockWuzapi struct {
	mu        sync.Mutex
	users     []User
	lastSend  map[string]any
	lastPath  string
	connected bool
	loggedIn  bool
}

func (m *mockWuzapi) handler(t *testing.T) http.Handler {
	ok := func(w http.ResponseWriter, code int, data any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "data": data, "success": true})
	}
	fail := func(w http.ResponseWriter, code int, msg string) {
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "error": msg, "success": false})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.lastPath = r.URL.Path
		if strings.HasPrefix(r.URL.Path, "/admin/") {
			if r.Header.Get("Authorization") != "admin-token" {
				fail(w, 401, "unauthorized")
				return
			}
			switch r.Method {
			case http.MethodGet:
				ok(w, 200, m.users)
			case http.MethodPost:
				var in map[string]any
				_ = json.NewDecoder(r.Body).Decode(&in)
				u := User{ID: "u1", Name: in["name"].(string), Token: in["token"].(string), Webhook: in["webhook"].(string), Events: in["events"].(string)}
				m.users = append(m.users, u)
				ok(w, 201, u)
			}
			return
		}
		if r.Header.Get("token") != "user-token" {
			fail(w, 401, "unauthorized")
			return
		}
		switch r.URL.Path {
		case "/session/connect":
			m.connected = true
			ok(w, 200, map[string]any{"details": "Connected!", "jid": ""})
		case "/session/qr":
			if !m.connected {
				fail(w, 500, "not connected")
				return
			}
			ok(w, 200, map[string]any{"QRCode": "data:image/png;base64,QUJD"})
		case "/session/status":
			jid := ""
			if m.loggedIn {
				jid = "905559998877.0:12@s.whatsapp.net"
			}
			ok(w, 200, map[string]any{"connected": m.connected, "loggedIn": m.loggedIn, "jid": jid})
		case "/session/pairphone":
			ok(w, 200, map[string]any{"LinkingCode": "ABCD-EFGH"})
		case "/session/logout":
			m.loggedIn = false
			ok(w, 200, map[string]any{"Details": "Logged out"})
		case "/webhook":
			ok(w, 200, map[string]any{"webhook": "x"})
		case "/chat/send/text", "/chat/send/document", "/chat/send/image":
			raw, _ := io.ReadAll(r.Body)
			m.lastSend = map[string]any{}
			_ = json.Unmarshal(raw, &m.lastSend)
			id, _ := m.lastSend["Id"].(string)
			if id == "" {
				id = "SRV1"
			}
			ok(w, 200, map[string]any{"Details": "Sent", "Id": id, "Timestamp": "2026-10-01T12:00:00+03:00"})
		default:
			fail(w, 404, "not found")
		}
	})
}

func newTestClient(t *testing.T) (*Client, *mockWuzapi) {
	t.Helper()
	m := &mockWuzapi{}
	srv := httptest.NewServer(m.handler(t))
	t.Cleanup(srv.Close)
	c := New(Config{BaseURL: srv.URL, AdminToken: "admin-token", HMACKey: "hmac-secret-0123456789abcdef0123456789", WebhookURL: "http://backend.app:8080/hooks/wuzapi"})
	return c, m
}

func TestEnsureUserIdempotent(t *testing.T) {
	c, m := newTestClient(t)
	ctx := context.Background()
	u, created, err := c.EnsureUser(ctx, "olexfilms")
	if err != nil || !created || u.Token == "" || u.Webhook != "http://backend.app:8080/hooks/wuzapi" {
		t.Fatalf("first ensure: %+v %v %v", u, created, err)
	}
	again, created, err := c.EnsureUser(ctx, "olexfilms")
	if err != nil || created || again.Token != u.Token || len(m.users) != 1 {
		t.Fatalf("second ensure must reuse: %+v %v %v", again, created, err)
	}
}

// Acceptance 1: connect → QR → status (mocked wuzapi).
func TestSessionFlow(t *testing.T) {
	c, m := newTestClient(t)
	ctx := context.Background()
	if _, err := c.Status(ctx); !errors.Is(err, whatsapp.ErrNotConfigured) {
		t.Fatalf("no token yet: %v", err)
	}
	c.SetUserToken("user-token")
	if _, err := c.QR(ctx); !errors.Is(err, whatsapp.ErrNotConnected) {
		t.Fatalf("qr before connect: %v", err)
	}
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	qr, err := c.QR(ctx)
	if err != nil || !strings.HasPrefix(qr, "data:image/png;base64,") {
		t.Fatalf("qr = %q %v", qr, err)
	}
	st, err := c.Status(ctx)
	if err != nil || st.State() != whatsapp.StateQR {
		t.Fatalf("status = %+v %v", st, err)
	}
	m.loggedIn = true
	st, _ = c.Status(ctx)
	if st.State() != whatsapp.StateConnected || st.Phone != "+905559998877" {
		t.Fatalf("connected status = %+v", st)
	}
	code, err := c.PairPhone(ctx, "+905551234567")
	if err != nil || code != "ABCD-EFGH" {
		t.Fatalf("pair = %q %v", code, err)
	}
	if err := c.Logout(ctx); err != nil {
		t.Fatal(err)
	}
}

// Acceptance 2: the test message reaches /chat/send/text with digits, body, id.
func TestSendText(t *testing.T) {
	c, m := newTestClient(t)
	c.SetUserToken("user-token")
	ref, err := c.SendText(context.Background(), "+905551234567", "Merhaba", whatsapp.SendOptions{ID: "ABC123"})
	if err != nil {
		t.Fatal(err)
	}
	if m.lastPath != "/chat/send/text" || m.lastSend["Phone"] != "905551234567" || m.lastSend["Body"] != "Merhaba" || m.lastSend["Id"] != "ABC123" {
		t.Fatalf("sent %s %v", m.lastPath, m.lastSend)
	}
	if ref.ID != "ABC123" || ref.Provider != ProviderName {
		t.Fatalf("ref = %+v", ref)
	}
	if _, err := c.SendText(context.Background(), "12", "x", whatsapp.SendOptions{}); !errors.Is(err, whatsapp.ErrInvalidRecipient) {
		t.Fatalf("invalid recipient: %v", err)
	}
	if _, err := c.SendImage(context.Background(), "+905551234567", whatsapp.Media{Data: []byte("\x89PNG\r\n\x1a\nxxxx"), Caption: "c"}, whatsapp.SendOptions{}); err != nil {
		t.Fatal(err)
	}
	if img, _ := m.lastSend["Image"].(string); !strings.HasPrefix(img, "data:image/png;base64,") {
		t.Fatalf("image = %v", m.lastSend["Image"])
	}
	if _, err := c.SendDocument(context.Background(), "+905551234567", whatsapp.Media{Data: []byte("%PDF-1.4"), FileName: "a.pdf"}, whatsapp.SendOptions{}); err != nil {
		t.Fatal(err)
	}
	if m.lastSend["FileName"] != "a.pdf" {
		t.Fatalf("document = %v", m.lastSend)
	}
}

// Optional: run against a real wuzapi 1.0.9 container.
//
//	WUZAPI_TEST_URL=http://127.0.0.1:18082 WUZAPI_TEST_ADMIN_TOKEN=... go test ./internal/platform/whatsapp/wuzapi/
func TestRealContainer(t *testing.T) {
	base := os.Getenv("WUZAPI_TEST_URL")
	admin := os.Getenv("WUZAPI_TEST_ADMIN_TOKEN")
	if base == "" || admin == "" {
		t.Skip("WUZAPI_TEST_URL not set")
	}
	ctx := context.Background()
	c := New(Config{BaseURL: base, AdminToken: admin, WebhookURL: "http://host.docker.internal:8080/hooks/wuzapi"})
	u, _, err := c.EnsureUser(ctx, "tec92-it")
	if err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	again, created, err := c.EnsureUser(ctx, "tec92-it")
	if err != nil || created || again.Token != u.Token {
		t.Fatalf("ensure idempotent: %+v %v %v", again, created, err)
	}
	c.SetUserToken(u.Token)
	if err := c.SetWebhook(ctx); err != nil {
		t.Fatalf("set webhook: %v", err)
	}
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	st, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("status: %+v state=%s", st, st.State())
	if qr, err := c.QR(ctx); err != nil {
		t.Logf("qr unavailable (WhatsApp servers unreachable?): %v", err)
	} else {
		t.Logf("qr length %d", len(qr))
	}
}
