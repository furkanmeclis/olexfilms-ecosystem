package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-637: authenticator 2FA is for panel accounts and, on the portal, for
// fleet users only.
func TestTOTPPrincipalRealms(t *testing.T) {
	cases := []struct {
		name  string
		p     authctx.Principal
		allow bool
	}{
		{"panel user", authctx.Principal{Realm: jwt.AudiencePanel}, true},
		{"portal fleet user", authctx.Principal{Realm: jwt.AudiencePortal, Permissions: []string{rbac.PermFleetPortalRead}}, true},
		{"portal OTP customer", authctx.Principal{Realm: jwt.AudiencePortal}, false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/v1/auth/totp", nil)
		req = req.WithContext(authctx.WithPrincipal(req.Context(), tc.p))
		rec := httptest.NewRecorder()
		_, ok := totpPrincipal(rec, req)
		if ok != tc.allow {
			t.Fatalf("%s: allowed = %v, want %v", tc.name, ok, tc.allow)
		}
		if !tc.allow && rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403", tc.name, rec.Code)
		}
	}
}
