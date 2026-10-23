//go:build integration

package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// TEC-961: a mobile session earns the step-up grant through the mirrored
// password route (same verification, rate limit and audit as the web
// dialog) and a step-up guarded mirrored route then passes the gate. The
// status and passkey routes stay off the mirror; a panel token never uses it.
func TestIntegrationMobileStepUp(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	wh, pw := it.user("t961-wh")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM refresh_tokens WHERE user_id = $1", wh.ID)
	})
	mobile := it.mobileLogin(wh.Email.String, pw, center.Slug, "t961-stepup").AccessToken
	approve := "/v1/mobile/panel/stock/reclassifications/" + uuid.NewString() + "/approve"

	if code, env, _ := it.doMobile("POST", approve, mobile, "1", nil); code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("approve without step-up = %d %s", code, errCode(env))
	}
	if code, env, _ := it.doMobile("POST", "/v1/mobile/panel/auth/step-up/password", mobile, "1",
		map[string]string{"password": "wrong-password"}); code != http.StatusUnauthorized || errCode(env) != "INVALID_CREDENTIALS" {
		t.Fatalf("wrong password = %d %s", code, errCode(env))
	}
	for _, path := range []string{"/v1/mobile/panel/auth/step-up", "/v1/mobile/panel/auth/step-up/passkey/options"} {
		if code, _, _ := it.doMobile("POST", path, mobile, "1", nil); code != http.StatusNotFound {
			t.Fatalf("%s = %d, want 404", path, code)
		}
	}
	panel := it.loginOrg(wh, pw, center)
	if code, env, _ := it.doMobile("POST", "/v1/mobile/panel/auth/step-up/password", panel, "1",
		map[string]string{"password": pw}); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("panel token on mirror = %d %s", code, errCode(env))
	}

	if code, env, _ := it.doMobile("POST", "/v1/mobile/panel/auth/step-up/password", mobile, "1",
		map[string]string{"password": pw}); code != http.StatusOK {
		t.Fatalf("mobile step-up = %d %s", code, errCode(env))
	}
	// The gate passes now; the unknown request is the handler's 404.
	if code, env, _ := it.doMobile("POST", approve, mobile, "1", nil); code != http.StatusNotFound {
		t.Fatalf("approve after step-up = %d %s", code, errCode(env))
	}
	var n int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM activity_events WHERE actor_user_id = $1 AND action = 'stepup.verified'`, wh.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("stepup.verified audit rows = %d, %v", n, err)
	}
}
