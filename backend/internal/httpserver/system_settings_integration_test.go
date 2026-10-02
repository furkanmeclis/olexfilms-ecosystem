package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// TEC-215: admin read/write of the system settings store. This test writes
// forecast_min_days and smtp.password; the sysconfig package test writes
// other keys so the two never race.
func TestIntegrationSystemSettings(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	cleanup := func() {
		_, _ = it.pool.Exec(ctx, "DELETE FROM system_settings WHERE key = ANY($1)",
			[]string{sysconfig.KeyForecastMinDays, sysconfig.KeySMTPPassword})
	}
	cleanup()
	t.Cleanup(cleanup)
	admin := it.adminToken()
	base := "/v1/platform/system-settings"

	// A plain user (no platform.settings.*) is refused.
	plain, ppw := it.user("sys-plain")
	plainTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": plain.Email.String, "password": ppw,
	})).AccessToken
	if code, _ := it.do("GET", base, hostOlex, plainTok, nil); code != http.StatusForbidden {
		t.Fatalf("plain user list = %d, want 403", code)
	}

	// List exposes every catalog key with its default.
	code, env := it.do("GET", base, hostOlex, admin, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d %s", code, errCode(env))
	}
	var list struct {
		Items []sysconfig.Entry `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatal(err)
	}
	found := map[string]sysconfig.Entry{}
	for _, e := range list.Items {
		found[e.Key] = e
	}
	if e, ok := found[sysconfig.KeyContractGraceDays]; !ok || string(e.Value) != "0" {
		t.Fatalf("contract_grace_days in list = %+v, want default 0", e)
	}
	if e, ok := found[sysconfig.KeyForecastMinDays]; !ok || !e.IsDefault {
		t.Fatalf("forecast_min_days in list = %+v, want default", e)
	}

	// Write a valid value: visible to the typed accessor at once (cache
	// dropped), and the entry is no longer default.
	code, env = it.do("PUT", base+"/"+sysconfig.KeyForecastMinDays, hostOlex, admin, map[string]any{"value": 60})
	if code != http.StatusOK {
		t.Fatalf("put = %d %s", code, errCode(env))
	}
	var e sysconfig.Entry
	if err := json.Unmarshal(env.Data, &e); err != nil {
		t.Fatal(err)
	}
	if e.IsDefault || string(e.Value) != "60" {
		t.Fatalf("put entry = %+v", e)
	}
	if got := it.srv.sysconfig.ForecastMinDays(ctx); got != 60 {
		t.Fatalf("typed read after put = %d, want 60", got)
	}

	// Schema mismatch: 400 VALIDATION_ERROR, value unchanged.
	for _, bad := range []any{0, -3, "60", 1.5, true, nil} {
		code, env = it.do("PUT", base+"/"+sysconfig.KeyForecastMinDays, hostOlex, admin, map[string]any{"value": bad})
		if code != http.StatusBadRequest || errCode(env) != response.CodeValidationError {
			t.Fatalf("put %v = %d %s, want 400 VALIDATION_ERROR", bad, code, errCode(env))
		}
	}
	if got := it.srv.sysconfig.ForecastMinDays(ctx); got != 60 {
		t.Fatalf("after rejected puts = %d, want 60", got)
	}

	// Unknown key: 404.
	if code, _ = it.do("PUT", base+"/unknown.key", hostOlex, admin, map[string]any{"value": 1}); code != http.StatusNotFound {
		t.Fatalf("unknown key = %d, want 404", code)
	}

	// Secrets are masked on read and the mask does not overwrite them.
	if code, env = it.do("PUT", base+"/"+sysconfig.KeySMTPPassword, hostOlex, admin, map[string]any{"value": "s3cret"}); code != http.StatusOK {
		t.Fatalf("put secret = %d %s", code, errCode(env))
	}
	code, env = it.do("GET", base+"/"+sysconfig.KeySMTPPassword, hostOlex, admin, nil)
	if code != http.StatusOK {
		t.Fatalf("get secret = %d", code)
	}
	if err := json.Unmarshal(env.Data, &e); err != nil {
		t.Fatal(err)
	}
	if string(e.Value) != `"`+sysconfig.SecretMask+`"` {
		t.Fatalf("secret value = %s, want masked", e.Value)
	}
	if code, _ = it.do("PUT", base+"/"+sysconfig.KeySMTPPassword, hostOlex, admin, map[string]any{"value": sysconfig.SecretMask}); code != http.StatusOK {
		t.Fatalf("put mask = %d", code)
	}
	if got := it.srv.sysconfig.SMTP(ctx).Password; got != "s3cret" {
		t.Fatalf("secret after mask write = %q, want s3cret", got)
	}

	// Reset returns the default.
	code, env = it.do("DELETE", base+"/"+sysconfig.KeyForecastMinDays, hostOlex, admin, nil)
	if code != http.StatusOK {
		t.Fatalf("reset = %d %s", code, errCode(env))
	}
	if err := json.Unmarshal(env.Data, &e); err != nil {
		t.Fatal(err)
	}
	if !e.IsDefault || string(e.Value) != "30" {
		t.Fatalf("reset entry = %+v, want default 30", e)
	}
}
