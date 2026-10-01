package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// These tests switch measurements, campaigns and dealer_showcase; the
// features package tests use other keys so the two never race.

type featuresPayload struct {
	Items   []features.State `json:"items"`
	Enabled []string         `json:"enabled"`
}

func (it *itest) features(access string) featuresPayload {
	it.t.Helper()
	code, env := it.do("GET", "/v1/features", hostOlex, access, nil)
	if code != http.StatusOK {
		it.t.Fatalf("GET /v1/features = %d %s", code, errCode(env))
	}
	var p featuresPayload
	if err := json.Unmarshal(env.Data, &p); err != nil {
		it.t.Fatal(err)
	}
	return p
}

func hasKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

func (it *itest) adminToken() string {
	it.t.Helper()
	admin, pw := it.user("feat-admin", rbac.RoleSuperAdmin)
	return it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": pw,
	})).AccessToken
}

func (it *itest) reopenSystem(key string) {
	it.t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM module_flags WHERE scope = 'system' AND module_key = $1", key)
	})
}

type featureNet struct {
	dist, other               db.Organization
	dealers                   []db.Organization
	foreign                   db.Organization
	distTok, dealerTok, admin string
}

func (it *itest) featureNet(n int) featureNet {
	it.t.Helper()
	center := it.brandCenter("olex")
	f := featureNet{}
	f.dist = it.org("feat-dist", "distributor", center)
	f.other = it.org("feat-other", "distributor", center)
	for i := 0; i < n; i++ {
		f.dealers = append(f.dealers, it.org("feat-dealer-"+string(rune('a'+i)), "dealer", f.dist))
	}
	f.foreign = it.org("feat-foreign", "dealer", f.other)
	downer, dpw := it.user("feat-downer")
	it.member(f.dist, downer, "owner")
	f.distTok = it.loginOrg(downer, dpw, f.dist)
	if len(f.dealers) > 0 {
		owner, opw := it.user("feat-dealer-owner")
		it.member(f.dealers[0], owner, "owner")
		f.dealerTok = it.loginOrg(owner, opw, f.dealers[0])
	}
	f.admin = it.adminToken()
	return f
}

// Acceptance 1 (HTTP): the admin closes a standard module system wide; no
// distributor can open it (409 FEATURE_DISABLED) and RequireFeature answers
// 403 at once, without waiting for the 30 s cache.
func TestIntegrationSystemClosedModule(t *testing.T) {
	it := newIntegration(t)
	key := features.ModuleMeasurements
	it.reopenSystem(key)
	authn := middleware.Authenticate(it.srv.tokens, it.srv.loader)
	org := middleware.RequireOrganization(it.srv.tokens, it.q)
	it.srv.mux.Handle("GET /v1/tenant/_test/measurements", middleware.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
		}), authn, org, middleware.RequireFeature(it.srv.features, key)))

	net := it.featureNet(1)
	if code, ec := it.status("GET", "/v1/tenant/_test/measurements", net.dealerTok); code != http.StatusOK {
		t.Fatalf("standard module before close = %d %s", code, ec)
	}
	if !hasKey(it.features(net.dealerTok).Enabled, key) {
		t.Fatal("measurements starts on")
	}

	code, env := it.do("PATCH", "/v1/platform/modules/"+key, hostOlex, net.admin, map[string]any{"enabled": false})
	if code != http.StatusOK {
		t.Fatalf("admin close = %d %s", code, errCode(env))
	}
	// Cached "on" is dropped by the generation bump: 403 immediately.
	if code, ec := it.status("GET", "/v1/tenant/_test/measurements", net.dealerTok); code != http.StatusForbidden || ec != response.CodeFeatureDisabled {
		t.Fatalf("after close = %d %s, want 403 FEATURE_DISABLED", code, ec)
	}
	p := it.features(net.dealerTok)
	if hasKey(p.Enabled, key) {
		t.Fatal("closed module still enabled")
	}
	for _, st := range p.Items {
		if st.Key == key {
			t.Fatal("closed module must not be listed")
		}
	}

	code, env = it.do("PUT", "/v1/tenant/modules/dealer-standard/"+key, hostOlex, net.distTok, map[string]any{"enabled": true})
	if code != http.StatusConflict || errCode(env) != response.CodeFeatureDisabled {
		t.Fatalf("distributor standard = %d %s, want 409", code, errCode(env))
	}
	code, env = it.do("PUT", "/v1/tenant/modules/dealers/"+net.dealers[0].Uuid.String()+"/"+key, hostOlex, net.distTok, map[string]any{"enabled": true})
	if code != http.StatusConflict || errCode(env) != response.CodeFeatureDisabled {
		t.Fatalf("distributor dealer = %d %s, want 409", code, errCode(env))
	}
	code, env = it.do("PUT", "/v1/platform/organizations/"+net.dist.Uuid.String()+"/modules/"+key, hostOlex, net.admin, map[string]any{"enabled": true})
	if code != http.StatusConflict {
		t.Fatalf("admin per-org below a closed system = %d %s, want 409", code, errCode(env))
	}

	// Core modules cannot be closed.
	code, env = it.do("PATCH", "/v1/platform/modules/"+features.ModuleServices, hostOlex, net.admin, map[string]any{"enabled": false})
	if code != http.StatusUnprocessableEntity || errCode(env) != response.CodeModuleCore {
		t.Fatalf("close core = %d %s, want 422 MODULE_CORE", code, errCode(env))
	}
}

// Acceptance 2 (HTTP): a distributor opens an add-on for 3 dealers in one
// call; a batch with another distributor's dealer is refused.
func TestIntegrationBulkAddon(t *testing.T) {
	it := newIntegration(t)
	key := features.ModuleCampaigns
	net := it.featureNet(3)

	// The admin first gives the add-on to the distributor.
	code, env := it.do("PUT", "/v1/platform/organizations/"+net.dist.Uuid.String()+"/modules/"+key, hostOlex, net.admin, map[string]any{"enabled": true})
	if code != http.StatusOK {
		t.Fatalf("admin -> distributor = %d %s", code, errCode(env))
	}
	uuids := []string{}
	for _, d := range net.dealers {
		uuids = append(uuids, d.Uuid.String())
	}
	code, env = it.do("POST", "/v1/tenant/modules/dealers/bulk", hostOlex, net.distTok, map[string]any{
		"dealer_uuids": append(append([]string{}, uuids...), net.foreign.Uuid.String()), "key": key, "enabled": true,
	})
	if code != http.StatusForbidden {
		t.Fatalf("bulk with foreign dealer = %d %s, want 403", code, errCode(env))
	}
	if hasKey(it.features(net.dealerTok).Enabled, key) {
		t.Fatal("rejected batch changed a dealer")
	}
	code, env = it.do("POST", "/v1/tenant/modules/dealers/bulk", hostOlex, net.distTok, map[string]any{
		"dealer_uuids": uuids, "key": key, "enabled": true,
	})
	if code != http.StatusOK {
		t.Fatalf("bulk = %d %s", code, errCode(env))
	}
	if !hasKey(it.features(net.dealerTok).Enabled, key) {
		t.Fatal("dealer GET /features misses the add-on")
	}
	code, env = it.do("GET", "/v1/tenant/modules/dealers", hostOlex, net.distTok, nil)
	if code != http.StatusOK {
		t.Fatalf("matrix = %d %s", code, errCode(env))
	}
	var matrix struct {
		Items []struct {
			UUID    string `json:"uuid"`
			Modules []struct {
				Key     string `json:"key"`
				Enabled bool   `json:"enabled"`
			} `json:"modules"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &matrix)
	on := 0
	for _, d := range matrix.Items {
		for _, m := range d.Modules {
			if m.Key == key && m.Enabled {
				on++
			}
		}
	}
	if len(matrix.Items) != 3 || on != 3 {
		t.Fatalf("matrix dealers=%d enabled=%d", len(matrix.Items), on)
	}

	// The dealer owner has modules.read (managed) but cannot manage.
	code, _ = it.do("POST", "/v1/tenant/modules/dealers/bulk", hostOlex, net.dealerTok, map[string]any{
		"dealer_uuids": uuids, "key": key, "enabled": false,
	})
	if code != http.StatusForbidden {
		t.Fatalf("dealer owner bulk = %d, want 403", code)
	}
}

// New dealers get the dealer standard; the admin exception works below a
// distributor without the module; a dealer can request a module.
func TestIntegrationStandardAndAdminException(t *testing.T) {
	it := newIntegration(t)
	key := features.ModuleDealerShowcase
	net := it.featureNet(1)
	code, env := it.do("PUT", "/v1/platform/organizations/"+net.dealers[0].Uuid.String()+"/modules/"+key, hostOlex, net.admin, map[string]any{"enabled": true})
	if code != http.StatusOK {
		t.Fatalf("admin -> dealer = %d %s", code, errCode(env))
	}
	if !hasKey(it.features(net.dealerTok).Enabled, key) {
		t.Fatal("admin exception below a distributor without the add-on")
	}

	code, env = it.do("PUT", "/v1/tenant/modules/dealer-standard/"+features.ModuleAnnouncements, hostOlex, net.distTok, map[string]any{"enabled": false})
	if code != http.StatusOK {
		t.Fatalf("standard = %d %s", code, errCode(env))
	}
	fresh := it.org("feat-fresh", "dealer", net.dist)
	freshOwner, fpw := it.user("feat-fresh-owner")
	it.member(fresh, freshOwner, "owner")
	freshTok := it.loginOrg(freshOwner, fpw, fresh)
	if hasKey(it.features(freshTok).Enabled, features.ModuleAnnouncements) {
		t.Fatal("new dealer must get the dealer standard (off)")
	}

	code, env = it.do("POST", "/v1/features/"+features.ModuleStockForecast+"/request", hostOlex, freshTok, map[string]any{"note": "please"})
	if code != http.StatusAccepted {
		t.Fatalf("request = %d %s", code, errCode(env))
	}
}
