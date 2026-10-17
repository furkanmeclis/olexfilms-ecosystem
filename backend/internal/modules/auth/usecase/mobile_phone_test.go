package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	platformtotp "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/totp"
	"github.com/google/uuid"
)

type mobilePhoneRepo struct {
	*memRepo
	user              model.User
	missing           bool
	created, verified int
	meta              model.SessionMeta
	endedDevice       string
	totp              model.UserTOTP
	allowCreate       bool
}

func (r *mobilePhoneRepo) FindUserByPhone(context.Context, string) (model.User, error) {
	if r.missing {
		return model.User{}, repository.ErrNotFound
	}
	return r.user, nil
}
func (r *mobilePhoneRepo) CreatePhoneUser(ctx context.Context, e164, _, locale, role string) (model.User, error) {
	r.created++
	if !r.allowCreate {
		return model.User{}, errors.New("unexpected creation")
	}
	u, err := r.memRepo.CreateUser(ctx, model.User{Phone: e164, Locale: locale, Status: "active"}, false)
	if err != nil {
		return model.User{}, err
	}
	r.memRepo.userRoles[u.ID] = []string{role}
	r.user, r.missing = u, false
	return u, nil
}
func (r *mobilePhoneRepo) MarkPhoneVerified(context.Context, int64) error { r.verified++; return nil }
func (r *mobilePhoneRepo) SaveRefresh(_ context.Context, _ int64, _ string, _ time.Time, meta model.SessionMeta) (uuid.UUID, error) {
	r.meta = meta
	return uuid.New(), nil
}
func (r *mobilePhoneRepo) RevokeMobileDeviceSessions(_ context.Context, _ int64, device string, _ uuid.UUID) ([]uuid.UUID, error) {
	r.endedDevice = device
	return nil, nil
}
func (r *mobilePhoneRepo) GetUserTOTP(context.Context, int64) (model.UserTOTP, error) {
	return r.totp, nil
}

type mobileOTPStub struct {
	request  otp.RequestInput
	err      error
	consumed bool
}

func (s *mobileOTPStub) Request(_ context.Context, in otp.RequestInput) (otp.RequestResult, error) {
	s.request = in
	return otp.RequestResult{ExpiresAt: time.Now().Add(5 * time.Minute), ResendAt: time.Now().Add(time.Minute)}, s.err
}
func (s *mobileOTPStub) VerifyMobile(_ context.Context, in otp.VerifyInput, check func(otp.Verified) error) (otp.Verified, error) {
	if s.err != nil {
		return otp.Verified{}, s.err
	}
	if s.consumed || in.Code != "123456" {
		return otp.Verified{}, otp.ErrInvalidCode
	}
	v := otp.Verified{Phone: "+905321234567", ID: uuid.New()}
	if err := check(v); err != nil {
		return otp.Verified{}, err
	}
	s.consumed = true
	return v, nil
}

func TestMobileIdentify(t *testing.T) {
	uc := &AuthUseCase{}
	for _, tc := range []struct {
		input, region, kind, next, phone string
		want                             error
	}{
		{"Staff@Example.com", "", "email", "password", "", nil},
		{"0532 123 45 67", "", "phone", "otp", "+905321234567", nil},
		{"+905000001234", "", "phone", "otp", "+905000001234", nil},
		{"+49 1512 3456789", "DE", "phone", "otp", "+4915123456789", nil},
		{"", "", "", "", "", ErrInvalidRequest},
		{"invalid@", "", "", "", "", ErrInvalidRequest},
		{"bad-phone", "", "", "", "", otp.ErrInvalidPhone},
	} {
		t.Run(tc.input, func(t *testing.T) {
			svc := &mobileOTPStub{}
			got, err := uc.MobileIdentify(context.Background(), svc, tc.input, tc.region, "en", model.SessionMeta{IP: "127.0.0.1"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v", err)
			}
			if err != nil {
				return
			}
			if got.Kind != tc.kind || got.Next != tc.next || got.PhoneE164 != tc.phone {
				t.Fatalf("result=%+v", got)
			}
			if tc.kind == "email" && svc.request.Phone != "" {
				t.Fatal("email requested OTP")
			}
			if tc.kind == "phone" && (svc.request.Purpose != otp.PurposeMobileLogin || svc.request.Phone != tc.phone || got.ResendAt == nil || got.ExpiresAt == nil) {
				t.Fatalf("request=%+v result=%+v", svc.request, got)
			}
		})
	}
	limit := &otp.LimitError{Reason: otp.ReasonCooldown, RetryAt: time.Now().Add(time.Minute)}
	_, err := uc.MobileIdentify(context.Background(), &mobileOTPStub{err: limit}, "+905321234567", "", "", model.SessionMeta{})
	if err != limit {
		t.Fatalf("limit not preserved: %v", err)
	}
}

func TestMobileVerifyOTP(t *testing.T) {
	for _, tc := range []struct {
		name, role, status, code string
		missing                  bool
		want                     error
	}{
		{"staff", rbac.RoleSuperAdmin, "active", "123456", false, nil},
		{"wrong", rbac.RoleSuperAdmin, "active", "000000", false, otp.ErrInvalidCode},
		{"disabled", rbac.RoleSuperAdmin, "disabled", "123456", false, otp.ErrInvalidCode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := newMemRepo()
			user, err := base.CreateUser(context.Background(), model.User{Email: "staff@example.com", Status: tc.status}, true)
			if err != nil {
				t.Fatal(err)
			}
			base.userRoles[user.ID] = []string{tc.role}
			repo := &mobilePhoneRepo{memRepo: base, user: user, missing: tc.missing}
			manager, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", 15*time.Minute, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			uc := New(repo, manager)
			uc.SetPhoneRepository(repo)
			svc := &mobileOTPStub{}
			got, err := uc.MobileVerifyOTP(context.Background(), svc, "+905321234567", tc.code, "", "", "", model.DeviceInfo{ID: "phone-1", Platform: "android"}, model.SessionMeta{Realm: jwt.AudiencePortal, ImpersonatorUserID: &user.ID})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if repo.created != 0 || len(base.users) != 1 {
				t.Fatal("created an account")
			}
			if err != nil {
				if got.AccessToken != "" || repo.meta.Client != "" {
					t.Fatal("issued session on failure")
				}
				return
			}
			claims, err := manager.ParseAccess(got.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			if claims.Realm() != jwt.AudienceMobile || claims.SessionKind() != jwt.MobileKindStaff || claims.MobileKind != "" || got.RefreshToken == "" || repo.meta.Client != model.ClientMobile || repo.meta.Device.ID != "phone-1" || repo.meta.ImpersonatorUserID != nil || repo.endedDevice != "phone-1" || repo.verified != 1 {
				t.Fatalf("claims=%+v meta=%+v", claims, repo.meta)
			}
			if time.Until(got.RefreshExpiresAt) < 29*24*time.Hour {
				t.Fatal("wrong refresh lifetime")
			}
			_, err = uc.MobileVerifyOTP(context.Background(), svc, "+905321234567", tc.code, "", "", "", model.DeviceInfo{ID: "phone-1", Platform: "android"}, model.SessionMeta{})
			if !errors.Is(err, otp.ErrInvalidCode) {
				t.Fatalf("replayed code: %v", err)
			}
		})
	}
}

func TestMobileVerifyOTPMFARetry(t *testing.T) {
	base := newMemRepo()
	user, err := base.CreateUser(context.Background(), model.User{Email: "staff@example.com", Status: "active"}, true)
	if err != nil {
		t.Fatal(err)
	}
	base.userRoles[user.ID] = []string{rbac.RoleSuperAdmin}
	box := testOAuthBox(t)
	enc, err := box.Encrypt("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	recovery, hashes, err := platformtotp.GenerateRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	repo := &mobilePhoneRepo{memRepo: base, user: user, totp: model.UserTOTP{Enabled: true, SecretEnc: enc, RecoveryHashes: hashes}}
	manager, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", 15*time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uc := New(repo, manager)
	uc.SetPhoneRepository(repo)
	uc.SetSecretBox(box, "Olexfilms")
	svc := &mobileOTPStub{}
	dev := model.DeviceInfo{ID: "phone-1", Platform: "ios"}
	_, err = uc.MobileVerifyOTP(context.Background(), svc, "+905321234567", "123456", "", "", "", dev, model.SessionMeta{})
	if !errors.Is(err, ErrMFARequired) || svc.consumed {
		t.Fatalf("missing MFA: %v consumed=%v", err, svc.consumed)
	}
	_, err = uc.MobileVerifyOTP(context.Background(), svc, "+905321234567", "123456", "invalid", "", "", dev, model.SessionMeta{})
	if !errors.Is(err, ErrInvalidMFACode) || svc.consumed {
		t.Fatalf("invalid MFA: %v", err)
	}
	got, err := uc.MobileVerifyOTP(context.Background(), svc, "+905321234567", "123456", recovery[0], "", "", dev, model.SessionMeta{})
	if err != nil || got.AccessToken == "" || !svc.consumed {
		t.Fatalf("MFA retry: %v", err)
	}
}
