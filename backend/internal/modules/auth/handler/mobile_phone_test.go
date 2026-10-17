package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/redis/go-redis/v9"
)

type mobilePhoneOTPStub struct {
	err          error
	request      otp.RequestInput
	verifyCalled bool
}

func (s *mobilePhoneOTPStub) Request(_ context.Context, in otp.RequestInput) (otp.RequestResult, error) {
	s.request = in
	return otp.RequestResult{Phone: in.Phone, ExpiresAt: time.Now().Add(5 * time.Minute), ResendAt: time.Now().Add(time.Minute)}, s.err
}
func (s *mobilePhoneOTPStub) VerifyMobile(context.Context, otp.VerifyInput, func(otp.Verified) error) (otp.Verified, error) {
	s.verifyCalled = true
	return otp.Verified{}, s.err
}

func TestMobilePhoneIdentifyHandler(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantCode string
		status               int
		err                  error
	}{
		{"email", `{"identifier":"staff@example.com"}`, "", 200, nil},
		{"phone", `{"identifier":"0532 123 45 67"}`, "", 200, nil},
		{"empty", `{"identifier":""}`, "VALIDATION_ERROR", 400, nil},
		{"bad email", `{"identifier":"bad@"}`, "VALIDATION_ERROR", 400, nil},
		{"bad phone", `{"identifier":"abc"}`, "INVALID_PHONE", 400, nil},
		{"cooldown", `{"identifier":"+905321234567"}`, "RATE_LIMITED", 429, &otp.LimitError{Reason: otp.ReasonCooldown, RetryAt: time.Now().Add(time.Minute)}},
		{"delivery", `{"identifier":"+905321234567"}`, "OTP_DELIVERY_FAILED", 503, otp.ErrDeliveryFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mobilePhoneOTPStub{err: tc.err}
			h := NewMobile(&usecase.AuthUseCase{}, nil, nil)
			h.SetOTP(svc)
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/v1/mobile/auth/identify", strings.NewReader(tc.body))
			h.Identify(w, r)
			if w.Code != tc.status || (tc.wantCode != "" && !strings.Contains(w.Body.String(), tc.wantCode)) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if tc.name == "email" {
				if svc.request.Phone != "" || !strings.Contains(w.Body.String(), `"next":"password"`) {
					t.Fatal(w.Body.String())
				}
			}
			if tc.name == "phone" {
				if svc.request.Phone != "+905321234567" || svc.request.Purpose != otp.PurposeMobileLogin || !strings.Contains(w.Body.String(), `"next":"otp"`) {
					t.Fatalf("%s %+v", w.Body.String(), svc.request)
				}
			}
			if errors.Is(tc.err, otp.ErrRateLimited) {
				if w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), "resend_at") {
					t.Fatal(w.Body.String())
				}
			}
		})
	}
}

func TestMobilePhoneIdentifyLoginRateLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	h := NewMobile(&usecase.AuthUseCase{}, nil, ratelimit.New(client, "test"))
	for i := 0; i < 11; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/mobile/auth/identify", strings.NewReader(`{"identifier":"staff@example.com"}`))
		h.Identify(w, r)
		want := 200
		if i == 10 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d: %d %s", i, w.Code, w.Body.String())
		}
	}
}

func TestMobilePhoneVersionHeader(t *testing.T) {
	h := NewMobile(&usecase.AuthUseCase{}, nil, nil)
	for _, fn := range []http.HandlerFunc{h.Identify, h.VerifyOTP} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
		middleware.MobileAPIVersion(1, 1)(fn).ServeHTTP(w, r)
		if w.Code != 426 {
			t.Fatalf("missing version: %d", w.Code)
		}
	}
}

func TestMobilePhoneVerifyHandlerErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{otp.ErrInvalidCode, 401, "INVALID_OTP_CODE"},
		{otp.ErrTooManyAttempts, 429, "OTP_LOCKED"},
		{usecase.ErrNoPanelAccess, 403, "NO_PANEL_ACCESS"},
		{usecase.ErrMFARequired, 403, "MFA_REQUIRED"},
		{usecase.ErrInvalidMFACode, 401, "INVALID_MFA_CODE"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", nil)
		writeMobilePhoneError(w, r, tc.err)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}
