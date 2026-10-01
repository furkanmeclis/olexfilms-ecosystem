package middleware

import "testing"

func TestRealmAllows(t *testing.T) {
	cases := []struct {
		realm, path string
		want        bool
	}{
		{"panel", "/v1/platform/users", true},
		{"panel", "/v1/tenant/settings", true},
		{"", "/v1/platform/users", true},
		{"panel", "/v1/portal/consents/pending", false},
		{"", "/v1/portal/consents", false},
		{"portal", "/v1/portal/consents/pending", true},
		{"portal", "/v1/auth/me", true},
		{"portal", "/v1/auth/logout", true},
		{"portal", "/v1/auth/sessions/123", true},
		{"portal", "/v1/auth/password/change", true},
		{"portal", "/v1/auth/meta", false},
		{"portal", "/v1/platform/users", false},
		{"portal", "/v1/tenant/organizations", false},
		{"portal", "/v1/platform/organizations", false},
		{"portal", "/v1/me/organizations", false},
		{"portal", "/v1/auth/organization-context", false},
		{"portal", "/v1/auth/impersonation/stop", false},
		{"portal", "/v1/auth/step-up", false},
		{"portal", "/v1/realtime/token", false},
		{"portal", "/v1/portal", false},
	}
	for _, c := range cases {
		if got := RealmAllows(c.realm, c.path); got != c.want {
			t.Errorf("RealmAllows(%q, %q) = %v, want %v", c.realm, c.path, got, c.want)
		}
	}
}
