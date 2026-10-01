// Package wuzapi is the wuzapi (asternic/wuzapi 1.0.9, whatsmeow) driver
// for the whatsapp.Provider interface.
//
// Endpoints used (verified against a 1.0.9 container):
//   - admin (header "Authorization: <admin token>"): GET/POST /admin/users
//   - session (header "token: <user token>"): POST /session/connect,
//     GET /session/qr, GET /session/status, POST /session/pairphone,
//     POST /session/logout, POST /webhook
//   - send: POST /chat/send/text|document|image ({"Phone": digits, ...})
//
// Webhooks (WEBHOOK_FORMAT=json) carry {"type", "event", "token", ...} and an
// "x-hmac-signature" header: hex(HMAC-SHA256(key, raw body)).
package wuzapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// ProviderName is the provider id stored with deliveries.
const ProviderName = "wuzapi"

// DefaultEvents are the webhook subscriptions of the instance.
var DefaultEvents = []string{
	"Message", "ReadReceipt", "Connected", "Disconnected", "LoggedOut",
	"TemporaryBan", "StreamReplaced", "ConnectFailure", "ClientOutdated", "PairSuccess", "QR",
}

const maxMediaBytes = 16 << 20

// Config configures the driver.
type Config struct {
	BaseURL    string
	AdminToken string
	// HMACKey verifies webhook signatures (WUZAPI_WEBHOOK_SECRET; wuzapi
	// signs with WUZAPI_GLOBAL_HMAC_KEY set to the same value).
	HMACKey string
	// WebhookURL is registered on the instance user (WUZAPI_WEBHOOK_URL).
	WebhookURL string
	HTTPClient *http.Client
}

// Client talks to one wuzapi instance user.
type Client struct {
	cfg  Config
	http *http.Client

	mu    sync.RWMutex
	token string
}

// New creates a client. The user token is set later with SetUserToken (it
// is stored encrypted in whatsapp_settings).
func New(cfg Config) *Client {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{cfg: cfg, http: hc}
}

// Configured reports whether a base URL and admin token are set.
func (c *Client) Configured() bool {
	return c != nil && c.cfg.BaseURL != "" && c.cfg.AdminToken != ""
}

// WebhookURL returns the configured webhook target.
func (c *Client) WebhookURL() string { return c.cfg.WebhookURL }

// SetUserToken sets the instance user token used by session/send calls.
func (c *Client) SetUserToken(token string) {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
}

// UserToken returns the current user token.
func (c *Client) UserToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// Name implements whatsapp.Provider.
func (c *Client) Name() string { return ProviderName }

// APIError is a non-2xx wuzapi response.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("wuzapi: http %d: %s", e.Status, e.Message)
}

type envelope struct {
	Code    int             `json:"code"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

func (c *Client) do(ctx context.Context, method, path string, admin bool, in, out any) error {
	if c.cfg.BaseURL == "" {
		return whatsapp.ErrNotConfigured
	}
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if admin {
		if c.cfg.AdminToken == "" {
			return whatsapp.ErrNotConfigured
		}
		req.Header.Set("Authorization", c.cfg.AdminToken)
	} else {
		token := c.UserToken()
		if token == "" {
			return whatsapp.ErrNotConfigured
		}
		req.Header.Set("token", token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wuzapi: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	var env envelope
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || (env.Code != 0 && !env.Success && env.Error != "") {
		msg := env.Error
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		low := strings.ToLower(msg)
		if strings.Contains(low, "not connected") || strings.Contains(low, "no session") || strings.Contains(low, "device jid") {
			return fmt.Errorf("%w: %s", whatsapp.ErrNotConnected, msg)
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("wuzapi: decode %s: %w", path, err)
		}
	}
	return nil
}

// User is an admin listing row.
type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Token     string `json:"token"`
	Webhook   string `json:"webhook"`
	Events    string `json:"events"`
	JID       string `json:"jid"`
	Connected bool   `json:"connected"`
	LoggedIn  bool   `json:"loggedIn"`
}

// EnsureUser returns the instance user named name, creating it when absent
// (idempotent across restarts). A new user gets a random token.
func (c *Client) EnsureUser(ctx context.Context, name string) (User, bool, error) {
	var users []User
	if err := c.do(ctx, http.MethodGet, "/admin/users", true, nil, &users); err != nil {
		return User{}, false, err
	}
	for _, u := range users {
		if u.Name == name {
			return u, false, nil
		}
	}
	token, err := randomToken()
	if err != nil {
		return User{}, false, err
	}
	in := map[string]any{
		"name":    name,
		"token":   token,
		"webhook": c.cfg.WebhookURL,
		"events":  strings.Join(DefaultEvents, ","),
	}
	var created User
	if err := c.do(ctx, http.MethodPost, "/admin/users", true, in, &created); err != nil {
		return User{}, false, err
	}
	if created.Token == "" {
		created.Token = token
	}
	return created, true, nil
}

// SetWebhook points the instance user at the configured webhook URL.
func (c *Client) SetWebhook(ctx context.Context) error {
	if c.cfg.WebhookURL == "" {
		return nil
	}
	return c.do(ctx, http.MethodPost, "/webhook", false, map[string]any{
		"webhookURL": c.cfg.WebhookURL,
		"events":     DefaultEvents,
	}, nil)
}

// Connect implements whatsapp.SessionManager.
func (c *Client) Connect(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/session/connect", false, map[string]any{
		"Subscribe": DefaultEvents,
		"Immediate": true,
	}, nil)
}

// QR implements whatsapp.SessionManager: a data:image/png;base64 URL.
func (c *Client) QR(ctx context.Context) (string, error) {
	var out struct {
		QRCode string `json:"QRCode"`
	}
	if err := c.do(ctx, http.MethodGet, "/session/qr", false, nil, &out); err != nil {
		return "", err
	}
	return out.QRCode, nil
}

// PairPhone implements whatsapp.SessionManager: returns the linking code.
func (c *Client) PairPhone(ctx context.Context, e164 string) (string, error) {
	n, err := phone.Parse(e164, "")
	if err != nil {
		return "", whatsapp.ErrInvalidRecipient
	}
	var out struct {
		LinkingCode string `json:"LinkingCode"`
	}
	if err := c.do(ctx, http.MethodPost, "/session/pairphone", false, map[string]string{"Phone": n.Digits()}, &out); err != nil {
		return "", err
	}
	return out.LinkingCode, nil
}

// Logout implements whatsapp.SessionManager.
func (c *Client) Logout(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/session/logout", false, nil, nil)
}

// Status implements whatsapp.Provider.
func (c *Client) Status(ctx context.Context) (whatsapp.ConnState, error) {
	// 1.0.9 answers lower camel case ("connected", "loggedIn"); the docs
	// show "Connected"/"LoggedIn". encoding/json matches either.
	var out struct {
		Connected bool   `json:"connected"`
		LoggedIn  bool   `json:"loggedIn"`
		JID       string `json:"jid"`
	}
	if err := c.do(ctx, http.MethodGet, "/session/status", false, nil, &out); err != nil {
		return whatsapp.ConnState{}, err
	}
	st := whatsapp.ConnState{Connected: out.Connected, LoggedIn: out.LoggedIn, JID: out.JID}
	if out.JID != "" {
		if n, err := phone.FromDigits(out.JID); err == nil {
			st.Phone = n.E164
		}
	}
	return st, nil
}

type sendResult struct {
	ID        string `json:"Id"`
	Timestamp string `json:"Timestamp"`
}

func (r sendResult) ref() whatsapp.MsgRef {
	ts, err := time.Parse(time.RFC3339, r.Timestamp)
	if err != nil {
		ts = time.Now().UTC()
	}
	return whatsapp.MsgRef{ID: r.ID, Provider: ProviderName, Timestamp: ts}
}

func recipient(to string) (string, error) {
	n, err := phone.Parse(to, "")
	if err != nil {
		return "", whatsapp.ErrInvalidRecipient
	}
	return n.Digits(), nil
}

// SendText implements whatsapp.Provider.
func (c *Client) SendText(ctx context.Context, to, body string, opts whatsapp.SendOptions) (whatsapp.MsgRef, error) {
	digits, err := recipient(to)
	if err != nil {
		return whatsapp.MsgRef{}, err
	}
	in := map[string]any{"Phone": digits, "Body": body}
	if opts.ID != "" {
		in["Id"] = opts.ID
	}
	var out sendResult
	if err := c.do(ctx, http.MethodPost, "/chat/send/text", false, in, &out); err != nil {
		return whatsapp.MsgRef{}, err
	}
	if out.ID == "" {
		out.ID = opts.ID
	}
	return out.ref(), nil
}

// SendDocument implements whatsapp.Provider.
func (c *Client) SendDocument(ctx context.Context, to string, doc whatsapp.Media, opts whatsapp.SendOptions) (whatsapp.MsgRef, error) {
	digits, err := recipient(to)
	if err != nil {
		return whatsapp.MsgRef{}, err
	}
	data, mime, err := c.mediaData(ctx, doc, "application/octet-stream")
	if err != nil {
		return whatsapp.MsgRef{}, err
	}
	name := doc.FileName
	if name == "" {
		name = "document"
	}
	in := map[string]any{
		"Phone":    digits,
		"FileName": name,
		"Document": "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(data),
		"Mimetype": mime,
	}
	if doc.Caption != "" {
		in["Caption"] = doc.Caption
	}
	if opts.ID != "" {
		in["Id"] = opts.ID
	}
	var out sendResult
	if err := c.do(ctx, http.MethodPost, "/chat/send/document", false, in, &out); err != nil {
		return whatsapp.MsgRef{}, err
	}
	return out.ref(), nil
}

// SendImage implements whatsapp.Provider.
func (c *Client) SendImage(ctx context.Context, to string, img whatsapp.Media, opts whatsapp.SendOptions) (whatsapp.MsgRef, error) {
	digits, err := recipient(to)
	if err != nil {
		return whatsapp.MsgRef{}, err
	}
	data, mime, err := c.mediaData(ctx, img, "image/jpeg")
	if err != nil {
		return whatsapp.MsgRef{}, err
	}
	in := map[string]any{
		"Phone": digits,
		"Image": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data),
	}
	if img.Caption != "" {
		in["Caption"] = img.Caption
	}
	if opts.ID != "" {
		in["Id"] = opts.ID
	}
	var out sendResult
	if err := c.do(ctx, http.MethodPost, "/chat/send/image", false, in, &out); err != nil {
		return whatsapp.MsgRef{}, err
	}
	return out.ref(), nil
}

func (c *Client) mediaData(ctx context.Context, m whatsapp.Media, fallbackMime string) ([]byte, string, error) {
	mime := m.MimeType
	if len(m.Data) > 0 {
		if mime == "" {
			mime = http.DetectContentType(m.Data)
		}
		return m.Data, mime, nil
	}
	if m.URL == "" {
		return nil, "", errors.New("wuzapi: media requires data or url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("wuzapi: media fetch: http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMediaBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxMediaBytes {
		return nil, "", errors.New("wuzapi: media too large")
	}
	if mime == "" {
		mime = resp.Header.Get("Content-Type")
	}
	if mime == "" {
		mime = fallbackMime
	}
	return data, mime, nil
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var (
	_ whatsapp.Provider       = (*Client)(nil)
	_ whatsapp.SessionManager = (*Client)(nil)
)
