package otp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type mobileStore struct {
	Store
	row      Code
	consumed bool
}

func (s *mobileStore) ActiveCode(_ context.Context, phone, purpose string, now time.Time) (Code, error) {
	if s.consumed || s.row.Phone != phone || s.row.Purpose != purpose || !now.Before(s.row.ExpiresAt) {
		return Code{}, ErrNotFound
	}
	return s.row, nil
}
func (s *mobileStore) IncrementAttempts(context.Context, int64) (int32, int32, error) {
	s.row.AttemptCount++
	return s.row.AttemptCount, s.row.MaxAttempts, nil
}
func (s *mobileStore) Consume(_ context.Context, _ int64, now time.Time) error {
	if s.consumed || !now.Before(s.row.ExpiresAt) {
		return ErrInvalidCode
	}
	s.consumed = true
	return nil
}

type mobileLimiter struct{ calls int }

func (l *mobileLimiter) AllowStrict(context.Context, string, string, int, time.Duration) (bool, time.Duration, error) {
	l.calls++
	return true, 0, nil
}

func TestMobileOTPVerificationLifecycle(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store := &mobileStore{row: Code{ID: 1, UUID: uuid.New(), Phone: "+905321234567", Purpose: PurposeMobileLogin, ExpiresAt: now.Add(5 * time.Minute), MaxAttempts: 5}}
	limiter := &mobileLimiter{}
	svc := New(store, nil, limiter, Config{Key: []byte("test-key")}, nil)
	svc.SetClock(func() time.Time { return now })
	store.row.CodeHash = svc.hash(store.row.UUID, "123456")
	in := VerifyInput{Phone: store.row.Phone, Code: "123456", IP: "127.0.0.1"}
	mfaRequired := errors.New("mfa required")
	for range 2 {
		_, err := svc.VerifyMobile(ctx, in, func(Verified) error { return mfaRequired })
		if !errors.Is(err, mfaRequired) || store.consumed {
			t.Fatalf("factor retry: %v consumed=%v", err, store.consumed)
		}
	}
	v, err := svc.VerifyMobile(ctx, in, func(v Verified) error {
		if v.Phone != in.Phone {
			t.Fatal("wrong phone")
		}
		return nil
	})
	if err != nil || v.ID != store.row.UUID || !store.consumed {
		t.Fatalf("verify: %+v %v", v, err)
	}
	_, err = svc.VerifyMobile(ctx, in, func(Verified) error { t.Fatal("replay reached MFA"); return nil })
	if !errors.Is(err, ErrInvalidCode) || limiter.calls != 4 {
		t.Fatalf("replay=%v limiter calls=%d", err, limiter.calls)
	}
	// Portal verification cannot use a mobile code.
	store.consumed = false
	_, err = svc.Verify(ctx, VerifyInput{Phone: in.Phone, Code: in.Code, Purpose: PurposeCustomerLogin})
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("cross-purpose accepted: %v", err)
	}
	// Wrong codes count, lock out, and never invoke the MFA check.
	in.Code = "000000"
	for i := 1; i <= 5; i++ {
		_, err = svc.VerifyMobile(ctx, in, func(Verified) error { t.Fatal("wrong code reached MFA"); return nil })
		want := ErrInvalidCode
		if i == 5 {
			want = ErrTooManyAttempts
		}
		if !errors.Is(err, want) || store.row.AttemptCount != int32(i) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if !store.consumed {
		t.Fatal("locked code was not consumed")
	}
	// A pending second-factor check does not extend the original expiry.
	store.consumed = false
	store.row.AttemptCount = 0
	in.Code = "123456"
	now = store.row.ExpiresAt
	_, err = svc.VerifyMobile(ctx, in, func(Verified) error { t.Fatal("expired code reached MFA"); return nil })
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expired: %v", err)
	}
}
