package otp_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sms"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/fake"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type env struct {
	svc      *otp.Service
	q        *db.Queries
	wa       *fake.Provider
	fallback bool
	mu       sync.Mutex
	now      time.Time
}

func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	e.now = e.now.Add(d)
	e.mu.Unlock()
}

func newEnv(t *testing.T, withRedis bool) *env {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	e := &env{q: q, wa: &fake.Provider{}, now: time.Now().UTC()}
	var lim *ratelimit.Limiter
	if withRedis {
		mr := miniredis.RunT(t)
		lim = ratelimit.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test")
	} else {
		lim = ratelimit.New(nil, "test")
	}
	sender := &whatsapp.Sender{WhatsApp: e.wa, SMS: sms.Noop{}, SMSFallback: func(context.Context) bool { return e.fallback }}
	e.svc = otp.New(otp.NewPGStore(pool, q), sender, lim, otp.Config{Key: []byte("test-otp-key")}, nil)
	e.svc.SetClock(func() time.Time {
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.now
	})
	return e
}

func randomPhone() string {
	return fmt.Sprintf("+90532%07d", rand.IntN(10_000_000))
}

var codeRe = regexp.MustCompile(`\b(\d{6})\b`)

func (e *env) lastCode(t *testing.T) (string, string) {
	t.Helper()
	m, ok := e.wa.Last()
	if !ok {
		t.Fatal("no message sent")
	}
	c := codeRe.FindStringSubmatch(m.Body)
	if c == nil {
		t.Fatalf("no code in %q", m.Body)
	}
	return c[1], m.Body
}

func (e *env) request(t *testing.T, ph string) (otp.RequestResult, error) {
	t.Helper()
	return e.svc.Request(context.Background(), otp.RequestInput{
		Phone: ph, Purpose: otp.PurposeCustomerLogin, Locale: "tr-TR", IP: "203.0.113.7", UserAgent: "go-test",
	})
}

func (e *env) verify(ph, code string) (otp.Verified, error) {
	return e.svc.Verify(context.Background(), otp.VerifyInput{Phone: ph, Code: code, IP: "203.0.113.7"})
}

func TestRequestAndVerify(t *testing.T) {
	e := newEnv(t, true)
	ph := randomPhone()
	res, err := e.request(t, strings.Replace(ph, "+90", "0", 1)) // national input
	if err != nil {
		t.Fatal(err)
	}
	ttl := res.ExpiresAt.Sub(e.now)
	if res.Channel != "whatsapp" || res.Phone != ph || ttl < 5*time.Minute-time.Second || ttl > 5*time.Minute+time.Second {
		t.Fatalf("result = %+v", res)
	}
	if got := res.ResendAt.Sub(e.now); got < 59*time.Second || got > 61*time.Second {
		t.Fatalf("resend_at offset = %v", got)
	}
	code, body := e.lastCode(t)
	sent, _ := e.wa.Last()
	if sent.To != ph {
		t.Fatalf("sent to %s", sent.To)
	}
	// KVKK notice (tr, seeded version 1) is part of the message; the stored
	// hash matches the exact text and the evidence columns are filled.
	if !strings.Contains(body, "KVKK") {
		t.Fatalf("message lacks KVKK notice: %q", body)
	}
	row, err := e.q.GetOTPByUUID(context.Background(), res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.MessageSha256.String != otp.MessageHash(body) || row.KvkkVersion.Int32 < 1 || row.KvkkLocale.String != "tr" ||
		row.Channel.String != "whatsapp" || row.ProviderRef.String == "" || row.Ip.String != "203.0.113.7" ||
		row.UserAgent.String != "go-test" || !row.DeliveredAt.Valid {
		t.Fatalf("evidence row = %+v", row)
	}
	if strings.Contains(row.CodeHash, code) {
		t.Fatal("code stored in clear")
	}
	v, err := e.verify(ph, code)
	if err != nil || v.Phone != ph || v.ID != res.ID {
		t.Fatalf("verify = %+v %v", v, err)
	}
	if _, err := e.verify(ph, code); !errors.Is(err, otp.ErrInvalidCode) {
		t.Fatalf("code is single use: %v", err)
	}
}

func TestWrongCodeLocksAfterFive(t *testing.T) {
	e := newEnv(t, true)
	ph := randomPhone()
	if _, err := e.request(t, ph); err != nil {
		t.Fatal(err)
	}
	code, _ := e.lastCode(t)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 1; i <= 4; i++ {
		if _, err := e.verify(ph, wrong); !errors.Is(err, otp.ErrInvalidCode) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := e.verify(ph, wrong); !errors.Is(err, otp.ErrTooManyAttempts) {
		t.Fatalf("5th attempt must lock: %v", err)
	}
	if _, err := e.verify(ph, code); !errors.Is(err, otp.ErrInvalidCode) {
		t.Fatalf("locked code must not verify: %v", err)
	}
}

func TestCooldownAndHourlyLimit(t *testing.T) {
	e := newEnv(t, true)
	ph := randomPhone()
	first, err := e.request(t, ph)
	if err != nil {
		t.Fatal(err)
	}
	e.advance(30 * time.Second)
	_, err = e.request(t, ph)
	var le *otp.LimitError
	if !errors.As(err, &le) || le.Reason != otp.ReasonCooldown || !le.RetryAt.Equal(first.ResendAt) || !errors.Is(err, otp.ErrRateLimited) {
		t.Fatalf("second request within 60s: %v (%+v) want resend %v", err, le, first.ResendAt)
	}
	// Requests 2..5 after each cooldown pass; the 6th within the hour fails.
	for i := 2; i <= 5; i++ {
		e.advance(31 * time.Second)
		if _, err := e.request(t, ph); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		e.advance(30 * time.Second)
	}
	e.advance(31 * time.Second)
	_, err = e.request(t, ph)
	if !errors.As(err, &le) || le.Reason != otp.ReasonHourly {
		t.Fatalf("6th request in the hour: %v", err)
	}
	// The newest code still works.
	code, _ := e.lastCode(t)
	if _, err := e.verify(ph, code); err != nil {
		t.Fatalf("latest code: %v", err)
	}
}

func TestExpiredCode(t *testing.T) {
	e := newEnv(t, true)
	ph := randomPhone()
	if _, err := e.request(t, ph); err != nil {
		t.Fatal(err)
	}
	code, _ := e.lastCode(t)
	e.advance(5*time.Minute + time.Second)
	if _, err := e.verify(ph, code); !errors.Is(err, otp.ErrInvalidCode) {
		t.Fatalf("expired code: %v", err)
	}
}

func TestSMSFallbackAndDeliveryFailure(t *testing.T) {
	e := newEnv(t, true)
	e.wa.Err = errors.New("wuzapi: http 500")
	ph := randomPhone()
	if _, err := e.request(t, ph); !errors.Is(err, otp.ErrDeliveryFailed) {
		t.Fatalf("no channel: %v", err)
	}
	e.fallback = true
	e.advance(61 * time.Second)
	res, err := e.request(t, ph)
	if err != nil || res.Channel != "sms" {
		t.Fatalf("sms fallback: %+v %v", res, err)
	}
	row, _ := e.q.GetOTPByUUID(context.Background(), res.ID)
	if row.Channel.String != "sms" || !strings.HasPrefix(row.ProviderRef.String, "noop-") || !strings.Contains(row.DeliveryError.String, "500") {
		t.Fatalf("sms evidence = %+v", row)
	}
}

func TestRedisUnavailableFailsClosed(t *testing.T) {
	e := newEnv(t, false)
	if _, err := e.request(t, randomPhone()); !errors.Is(err, otp.ErrLimiterUnavailable) {
		t.Fatalf("no redis: %v", err)
	}
	if len(e.wa.Sent()) != 0 {
		t.Fatal("nothing may be sent without the limiter")
	}
}

func TestEnglishNoticeAndUserLink(t *testing.T) {
	e := newEnv(t, true)
	ph := randomPhone()
	ctx := context.Background()
	u, err := e.q.CreateUser(ctx, db.CreateUserParams{
		PhoneE164: pgtype.Text{String: ph, Valid: true}, PasswordHash: "x", Name: "", Surname: "", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Request(ctx, otp.RequestInput{Phone: ph, Locale: "en", IP: "198.51.100.1"})
	if err != nil {
		t.Fatal(err)
	}
	_, body := e.lastCode(t)
	if !strings.Contains(body, "verification code") || !strings.Contains(body, "Privacy") {
		t.Fatalf("en message = %q", body)
	}
	code, _ := e.lastCode(t)
	v, err := e.verify(ph, code)
	if err != nil || v.UserID == nil || *v.UserID != u.ID || v.ID != res.ID {
		t.Fatalf("verify linked user: %+v %v", v, err)
	}
}

func TestInvalidInput(t *testing.T) {
	e := newEnv(t, true)
	if _, err := e.request(t, "12"); !errors.Is(err, otp.ErrInvalidPhone) {
		t.Fatalf("phone: %v", err)
	}
	if _, err := e.svc.Request(context.Background(), otp.RequestInput{Phone: randomPhone(), Purpose: "nope"}); !errors.Is(err, otp.ErrInvalidPurpose) {
		t.Fatalf("purpose: %v", err)
	}
	if otp.NormalizeLocale("de-DE") != "en" || otp.NormalizeLocale("") != "tr" || otp.NormalizeLocale("TR_tr") != "tr" {
		t.Fatal("locale")
	}
}
