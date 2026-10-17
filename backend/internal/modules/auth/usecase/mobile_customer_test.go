package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// mobileCustomerRepo serves one stored mobile refresh session (refresh test).
type mobileCustomerRepo struct {
	*mobilePhoneRepo
	session *model.RefreshSession
}

func (r *mobileCustomerRepo) GetRefreshSession(context.Context, string) (model.RefreshSession, error) {
	if r.session == nil {
		return model.RefreshSession{}, errors.New("no session")
	}
	return *r.session, nil
}

func newMobileCustomerUC(t *testing.T, roles []string, missing bool) (*AuthUseCase, *mobileCustomerRepo, *jwt.Manager, model.User) {
	t.Helper()
	base := newMemRepo()
	user, err := base.CreateUser(context.Background(), model.User{Email: "c@example.com", Phone: "+905321234567", Status: "active"}, true)
	if err != nil {
		t.Fatal(err)
	}
	base.userRoles[user.ID] = roles
	phoneRepo := &mobilePhoneRepo{memRepo: base, user: user, missing: missing, allowCreate: missing}
	repo := &mobileCustomerRepo{mobilePhoneRepo: phoneRepo}
	manager, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", 15*time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uc := New(repo, manager)
	uc.SetPhoneRepository(phoneRepo)
	return uc, repo, manager, user
}

func TestMobileVerifyOTPCustomerSessions(t *testing.T) {
	dev := model.DeviceInfo{ID: "phone-1", Platform: "ios"}
	for _, tc := range []struct {
		name    string
		roles   []string
		missing bool
		kind    string
	}{
		{"customer", []string{rbac.RoleCustomer}, false, jwt.MobileKindCustomer},
		{"fleet", []string{rbac.RoleFleet}, false, jwt.MobileKindCustomer},
		{"staff", []string{rbac.RoleSuperAdmin}, false, jwt.MobileKindStaff},
		{"staff_and_customer", []string{rbac.RoleCustomer, rbac.RoleSuperAdmin}, false, jwt.MobileKindStaff},
		{"unknown_phone", nil, true, jwt.MobileKindCustomer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uc, repo, manager, _ := newMobileCustomerUC(t, tc.roles, tc.missing)
			svc := &mobileOTPStub{}
			got, err := uc.MobileVerifyOTP(context.Background(), svc, "+905321234567", "123456", "", "ignored-org", "de", dev, model.SessionMeta{})
			if tc.kind == jwt.MobileKindStaff {
				// Staff sign-in is unchanged: the organization slug is resolved
				// (no resolver configured here, so it fails as before).
				if !errors.Is(err, ErrNoTenantMembership) {
					t.Fatalf("staff with organization slug: %v", err)
				}
				got, err = uc.MobileVerifyOTP(context.Background(), &mobileOTPStub{}, "+905321234567", "123456", "", "", "", dev, model.SessionMeta{})
			}
			if err != nil {
				t.Fatal(err)
			}
			claims, err := manager.ParseAccess(got.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			if claims.Realm() != jwt.AudienceMobile || claims.SessionKind() != tc.kind || claims.OrganizationID != nil {
				t.Fatalf("claims=%+v", claims)
			}
			if repo.meta.Client != model.ClientMobile || repo.meta.Realm != jwt.AudienceMobile || repo.endedDevice != "phone-1" {
				t.Fatalf("session meta=%+v", repo.meta)
			}
			if tc.missing {
				if repo.created != 1 || len(repo.memRepo.users) != 2 {
					t.Fatalf("account not created: created=%d users=%d", repo.created, len(repo.memRepo.users))
				}
				roles := repo.memRepo.userRoles[repo.user.ID]
				if len(roles) != 1 || roles[0] != rbac.RoleCustomer || repo.user.Locale != "de" {
					t.Fatalf("new account roles=%v locale=%q", roles, repo.user.Locale)
				}
			} else if repo.created != 0 {
				t.Fatal("created an account for a known phone")
			}
			p, err := uc.PrincipalFromAccess(context.Background(), got.AccessToken)
			if err != nil || p.MobileKind != tc.kind {
				t.Fatalf("principal kind=%q err=%v", p.MobileKind, err)
			}
		})
	}
}

func TestMobileVerifyOTPUnknownPhoneWrongCode(t *testing.T) {
	uc, repo, _, _ := newMobileCustomerUC(t, nil, true)
	_, err := uc.MobileVerifyOTP(context.Background(), &mobileOTPStub{}, "+905321234567", "000000", "", "", "", model.DeviceInfo{ID: "d", Platform: "android"}, model.SessionMeta{})
	if !errors.Is(err, otp.ErrInvalidCode) || repo.created != 0 {
		t.Fatalf("err=%v created=%d", err, repo.created)
	}
}

func TestMobileRefreshKeepsSessionKind(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roles []string
		kind  string
	}{
		{"customer", []string{rbac.RoleCustomer}, jwt.MobileKindCustomer},
		{"staff", []string{rbac.RoleSuperAdmin}, jwt.MobileKindStaff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uc, repo, manager, user := newMobileCustomerUC(t, tc.roles, false)
			family := uuid.New()
			repo.session = &model.RefreshSession{
				UUID: uuid.New(), UserID: user.ID, Realm: jwt.AudienceMobile, Client: model.ClientMobile,
				FamilyID: family, Device: &model.DeviceInfo{ID: "phone-1", Platform: "ios"},
				ExpiresAt: time.Now().Add(time.Hour),
			}
			got, err := uc.MobileRefresh(context.Background(), "raw", "", model.SessionMeta{})
			if err != nil {
				t.Fatal(err)
			}
			claims, err := manager.ParseAccess(got.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			if claims.SessionKind() != tc.kind || repo.meta.FamilyID != family || repo.meta.Realm != jwt.AudienceMobile {
				t.Fatalf("kind=%q meta=%+v", claims.SessionKind(), repo.meta)
			}
		})
	}
}

func TestPortalPhoneLoginUnchanged(t *testing.T) {
	uc, repo, manager, _ := newMobileCustomerUC(t, nil, true)
	got, created, err := uc.LoginWithVerifiedPhone(context.Background(), "+905321234567", "tr", model.SessionMeta{})
	if err != nil || !created || repo.created != 1 {
		t.Fatalf("err=%v created=%v", err, created)
	}
	claims, err := manager.ParseAccess(got.AccessToken)
	if err != nil || claims.Realm() != jwt.AudiencePortal || claims.MobileKind != "" || claims.SessionKind() != "" {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}
