package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

func TestDenyPortalReadOnly(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := DenyPortalReadOnly(ok)
	cases := []struct {
		name  string
		roles []string
		want  int
	}{
		{"customer", []string{rbac.RoleCustomer}, http.StatusNoContent},
		{"fleet", []string{rbac.RoleFleet}, http.StatusForbidden},
		{"customer and fleet", []string{rbac.RoleFleet, rbac.RoleCustomer}, http.StatusNoContent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/portal/vehicles/x/transfers", nil)
			req = req.WithContext(authctx.WithPrincipal(req.Context(), authctx.Principal{Roles: c.roles, Realm: "portal"}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
			if c.want == http.StatusForbidden {
				var env struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				_ = json.Unmarshal(rec.Body.Bytes(), &env)
				if env.Error.Code != "PORTAL_READ_ONLY" {
					t.Fatalf("code = %q", env.Error.Code)
				}
			}
		})
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/portal/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no principal = %d", rec.Code)
	}
}
