package httpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// Uses the existing mock wuzapi/Redis harness; never sends a real message.
func TestIntegrationMobilePhoneAuth(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	ctx := context.Background()
	for _, path := range []string{"/v1/mobile/auth/identify", "/v1/mobile/auth/otp/verify"} {
		for _, version := range []string{"", "2"} {
			status, env, _ := it.doMobile("POST", path, "", version, map[string]string{})
			if status != 426 || errCode(env) != "MOBILE_API_VERSION_UNSUPPORTED" {
				t.Fatalf("version: %s %d %s", path, status, errCode(env))
			}
		}
	}
	// Staff get a staff session; customer / fleet accounts and unknown
	// numbers (a new customer account, portal OTP rules) a customer session.
	for _, tc := range []struct{ name, role, kind string }{
		{"staff", rbac.RoleSuperAdmin, jwt.MobileKindStaff},
		{"customer", rbac.RoleCustomer, jwt.MobileKindCustomer},
		{"unknown", "", jwt.MobileKindCustomer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ph := itPhone()
			if tc.role != "" {
				user, _ := it.user("phone-"+tc.name, tc.role)
				if _, err := it.pool.Exec(ctx, "UPDATE users SET phone_e164=$1 WHERE id=$2", ph, user.ID); err != nil {
					t.Fatal(err)
				}
			}
			status, env, _ := it.doMobile("POST", "/v1/mobile/auth/identify", "", "1", map[string]string{"identifier": ph})
			if status != 200 || !strings.Contains(string(env.Data), `"next":"otp"`) {
				t.Fatalf("identify: %d %s", status, errCode(env))
			}
			code, _ := sentCode(t, fw, ph)
			body := map[string]any{"phone": ph, "code": code, "device": map[string]string{"id": "phone-test-" + tc.name, "platform": "android"}}
			status, env, _ = it.doMobile("POST", "/v1/mobile/auth/otp/verify", "", "1", body)
			tokens := it.tokensFrom(status, env)
			claims, err := it.tokens.ParseAccess(tokens.AccessToken)
			if err != nil || claims.Realm() != jwt.AudienceMobile || claims.SessionKind() != tc.kind ||
				!strings.Contains(string(env.Data), `"session":`) || !strings.Contains(string(env.Data), `"session_kind":"`+tc.kind+`"`) {
				t.Fatalf("mobile response: %v %s", err, env.Data)
			}
			status, env, _ = it.doMobile("POST", "/v1/mobile/auth/otp/verify", "", "1", body)
			if status != http.StatusUnauthorized || errCode(env) != "INVALID_OTP_CODE" {
				t.Fatalf("replay: %d %s", status, errCode(env))
			}
			status, env, _ = it.doMobile("GET", "/v1/mobile/auth/me", tokens.AccessToken, "1", nil)
			if status != 200 || !strings.Contains(string(env.Data), `"session_kind":"`+tc.kind+`"`) {
				t.Fatalf("me: %d %s", status, env.Data)
			}
			// Portal mirror: customer sessions only.
			status, env, _ = it.doMobile("GET", "/v1/mobile/portal/consents/pending", tokens.AccessToken, "1", nil)
			wantMirror, wantStaffRoute := 200, 403
			if tc.kind == jwt.MobileKindStaff {
				wantMirror, wantStaffRoute = 403, 0
			}
			if status != wantMirror {
				t.Fatalf("portal mirror: %d %s", status, errCode(env))
			}
			if wantStaffRoute != 0 {
				status, env, _ = it.doMobile("POST", "/v1/mobile/auth/organization-context", tokens.AccessToken, "1", map[string]string{"organization_slug": "x"})
				if status != wantStaffRoute || errCode(env) != "REALM_FORBIDDEN" {
					t.Fatalf("customer on staff route: %d %s", status, errCode(env))
				}
			}
			if tc.name == "unknown" {
				var count int
				if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM users u JOIN user_roles ur ON ur.user_id = u.id
					JOIN roles r ON r.id = ur.role_id WHERE u.phone_e164=$1 AND r.slug=$2`, ph, rbac.RoleCustomer).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Fatal("mobile OTP did not create the customer account")
				}
			}
		})
	}
}
