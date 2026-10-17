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
	for _, tc := range []struct{ name, role, want string }{
		{"staff", rbac.RoleSuperAdmin, ""},
		{"customer", rbac.RoleCustomer, "NO_PANEL_ACCESS"},
		{"unknown", "", "INVALID_OTP_CODE"},
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
			if tc.want != "" {
				wantStatus := 403
				if tc.name == "unknown" {
					wantStatus = 401
				}
				if status != wantStatus || errCode(env) != tc.want {
					t.Fatalf("verify: %d %s", status, errCode(env))
				}
			} else {
				tokens := it.tokensFrom(status, env)
				claims, err := it.tokens.ParseAccess(tokens.AccessToken)
				if err != nil || claims.Realm() != jwt.AudienceMobile || !strings.Contains(string(env.Data), `"session":`) {
					t.Fatalf("mobile response: %v %s", err, env.Data)
				}
				status, env, _ = it.doMobile("POST", "/v1/mobile/auth/otp/verify", "", "1", body)
				if status != http.StatusUnauthorized || errCode(env) != "INVALID_OTP_CODE" {
					t.Fatalf("replay: %d %s", status, errCode(env))
				}
			}
			if tc.name == "unknown" {
				var count int
				if err := it.pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE phone_e164=$1", ph).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatal("mobile OTP created an account")
				}
			}
		})
	}
}
