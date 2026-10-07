package providers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type captureMail struct{ msgs []mail.Message }

func (c *captureMail) Send(_ context.Context, m mail.Message) error {
	c.msgs = append(c.msgs, m)
	return nil
}

func TestEmailProviderSendsRTLHTML(t *testing.T) {
	m := &captureMail{}
	p := EmailProvider{Mail: m, Brand: func(context.Context, int64) EmailBrand {
		return EmailBrand{Name: "Olex", Color: "#ff0000"}
	}}
	_, err := p.Deliver(context.Background(), db.Notification{
		Title: "طلب وحدة", Body: "طلبت **Tech Oto** <script>x</script>",
		Recipient: pgtype.Text{String: "a@example.com", Valid: true},
		Language:  pgtype.Text{String: "ar", Valid: true},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := m.msgs[0]
	if !strings.Contains(got.HTMLBody, `dir="rtl"`) || !strings.Contains(got.HTMLBody, `lang="ar"`) {
		t.Fatalf("html lacks rtl: %s", got.HTMLBody)
	}
	if !strings.Contains(got.HTMLBody, "<strong>Tech Oto</strong>") || strings.Contains(got.HTMLBody, "<script>") {
		t.Fatalf("html body not rendered safely: %s", got.HTMLBody)
	}
	if got.Body != "طلبت Tech Oto <script>x</script>" {
		t.Fatalf("text part = %q", got.Body)
	}
	html, _ := p.HTML(context.Background(), db.Notification{Title: "x", Body: "y", Language: pgtype.Text{String: "tr", Valid: true}})
	if !strings.Contains(html, `dir="ltr"`) {
		t.Fatal("tr must be ltr")
	}
}

type fakeSMS struct {
	to, body string
}

func (f *fakeSMS) Name() string { return "fake-sms" }
func (f *fakeSMS) Send(_ context.Context, to, body string) (string, error) {
	f.to, f.body = to, body
	return "ref-1", nil
}

func TestSMSProvider(t *testing.T) {
	f := &fakeSMS{}
	p := SMSProvider{SMS: f}
	res, err := p.Deliver(context.Background(), db.Notification{
		Title: "Başlık", Body: "**Gövde**", Recipient: pgtype.Text{String: "+905551234567", Valid: true},
	}, nil)
	if err != nil || res.ProviderReference != "ref-1" || f.to != "+905551234567" || f.body != "Başlık\nGövde" {
		t.Fatalf("res=%+v err=%v sms=%+v", res, err, f)
	}
	if _, err := p.Deliver(context.Background(), db.Notification{}, nil); !errors.Is(err, ErrNoRecipient) {
		t.Fatalf("no recipient: %v", err)
	}
}

type fakeWebPushStore struct {
	subs    []db.PushSubscription
	deleted []string
}

func (f *fakeWebPushStore) ListPushSubscriptionsByUser(context.Context, int64) ([]db.PushSubscription, error) {
	return f.subs, nil
}

func (f *fakeWebPushStore) DeletePushSubscriptionByEndpoint(_ context.Context, endpoint string) error {
	f.deleted = append(f.deleted, endpoint)
	return nil
}

func TestWebPushProviderDropsGoneSubscriptions(t *testing.T) {
	store := &fakeWebPushStore{subs: []db.PushSubscription{{Endpoint: "https://push/ok"}, {Endpoint: "https://push/gone"}}}
	var payloads []string
	p := WebPushProvider{
		Store: store, VAPID: VAPID{PublicKey: "pub", PrivateKey: "priv"},
		Send: func(msg []byte, s *webpush.Subscription, _ *webpush.Options) (*http.Response, error) {
			payloads = append(payloads, string(msg))
			code := http.StatusCreated
			if strings.HasSuffix(s.Endpoint, "gone") {
				code = http.StatusGone
			}
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(""))}, nil
		},
	}
	res, err := p.Deliver(context.Background(), db.Notification{
		Uuid: uuid.New(), Title: "T", Body: "B", UserID: pgtype.Int8{Int64: 1, Valid: true},
	}, nil)
	if err != nil || res.ProviderReference != "1/2" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "https://push/gone" {
		t.Fatalf("deleted = %v", store.deleted)
	}
	if !strings.Contains(payloads[0], `"title":"T"`) {
		t.Fatalf("payload = %s", payloads[0])
	}
	store.subs = nil
	if _, err := p.Deliver(context.Background(), db.Notification{UserID: pgtype.Int8{Int64: 1, Valid: true}}, nil); !errors.Is(err, ErrNoRecipient) {
		t.Fatalf("no subscriptions: %v", err)
	}
}

type fakeExpoStore struct {
	tokens  []db.DevicePushToken
	revoked []string
}

func (f *fakeExpoStore) ListActiveDevicePushTokens(context.Context, int64) ([]db.DevicePushToken, error) {
	return f.tokens, nil
}

func (f *fakeExpoStore) RevokeDevicePushToken(_ context.Context, arg db.RevokeDevicePushTokenParams) (int64, error) {
	f.revoked = append(f.revoked, arg.ExpoToken)
	return 1, nil
}

func TestExpoProviderAgainstHTTPTest(t *testing.T) {
	var got []map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"data":[
			{"status":"ok","id":"ticket-1"},
			{"status":"error","message":"not registered","details":{"error":"DeviceNotRegistered"}}
		]}`)
	}))
	defer srv.Close()
	store := &fakeExpoStore{tokens: []db.DevicePushToken{
		{ExpoToken: "ExponentPushToken[a]"}, {ExpoToken: "ExponentPushToken[b]"},
	}}
	p := ExpoProvider{Store: store, URL: srv.URL, AccessToken: "tok", HTTP: srv.Client()}
	res, err := p.Deliver(context.Background(), db.Notification{
		Uuid: uuid.New(), Title: "Başlık", Body: "Gövde", Priority: "high",
		UserID: pgtype.Int8{Int64: 7, Valid: true},
	}, nil)
	if err != nil || res.Provider != "expo" || res.ProviderReference != "ticket-1" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if auth != "Bearer tok" || len(got) != 2 || got[0]["to"] != "ExponentPushToken[a]" || got[0]["priority"] != "high" {
		t.Fatalf("request auth=%q body=%+v", auth, got)
	}
	if len(store.revoked) != 1 || store.revoked[0] != "ExponentPushToken[b]" {
		t.Fatalf("revoked = %v", store.revoked)
	}
	store.tokens = nil
	if _, err := p.Deliver(context.Background(), db.Notification{UserID: pgtype.Int8{Int64: 7, Valid: true}}, nil); !errors.Is(err, ErrNoRecipient) {
		t.Fatalf("no tokens: %v", err)
	}
}

// TEC-407: a campaign push carries its image (payload image_url, https
// only) to web push ("image") and Expo ("richContent.image").
func TestPushProvidersCarryImage(t *testing.T) {
	n := db.Notification{
		Uuid: uuid.New(), Title: "T", Body: "B", UserID: pgtype.Int8{Int64: 1, Valid: true},
		Payload: []byte(`{"image_url":"https://cdn.example.test/a.png"}`),
	}
	var web map[string]any
	wp := WebPushProvider{
		Store: &fakeWebPushStore{subs: []db.PushSubscription{{Endpoint: "https://push/ok"}}},
		VAPID: VAPID{PublicKey: "pub", PrivateKey: "priv"},
		Send: func(msg []byte, _ *webpush.Subscription, _ *webpush.Options) (*http.Response, error) {
			_ = json.Unmarshal(msg, &web)
			return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(""))}, nil
		},
	}
	if _, err := wp.Deliver(context.Background(), n, nil); err != nil {
		t.Fatal(err)
	}
	if web["image"] != "https://cdn.example.test/a.png" {
		t.Fatalf("web push payload = %v", web)
	}
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"data":[{"status":"ok","id":"t"}]}`)
	}))
	defer srv.Close()
	ep := ExpoProvider{Store: &fakeExpoStore{tokens: []db.DevicePushToken{{ExpoToken: "ExponentPushToken[a]"}}}, URL: srv.URL, HTTP: srv.Client()}
	if _, err := ep.Deliver(context.Background(), n, nil); err != nil {
		t.Fatal(err)
	}
	rich, _ := got[0]["richContent"].(map[string]any)
	if rich["image"] != "https://cdn.example.test/a.png" {
		t.Fatalf("expo message = %v", got[0])
	}
	n.Payload = []byte(`{"image_url":"http://insecure.test/a.png"}`)
	web = nil
	if _, err := wp.Deliver(context.Background(), n, nil); err != nil || web["image"] != nil {
		t.Fatalf("non-https image must be dropped: %v %v", web, err)
	}
}
