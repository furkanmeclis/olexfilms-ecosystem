package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/stepup"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	hostOlex    = "olexfilms.app"
	hostGlorian = "warranty.glorianppf.com"
)

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code string `json:"code"`
	} `json:"error"`
}

type tokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type itest struct {
	t       *testing.T
	handler http.Handler
	pool    *pgxpool.Pool
	q       *db.Queries
	tokens  *jwt.Manager
	rdb     *redis.Client
	srv     *Server
	suffix  string
}

func newIntegration(t *testing.T) *itest {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	var cfg config.Config
	cfg.App.Name, cfg.App.Env, cfg.App.DefaultBrandSlug = "test", "test", "olex"
	cfg.JWT.AccessSecret = strings.Repeat("a", 40)
	cfg.JWT.RefreshSecret = strings.Repeat("b", 40)
	cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL = 15*time.Minute, time.Hour
	cfg.Encryption.Key = "app-dev-encryption-key-32bytes!!"
	cfg.Auth.AdapterSecret = strings.Repeat("s", 40)
	cfg.Auth.FrontendURL = "http://localhost:3000"

	q := db.New(pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	srv, err := New(cfg, log, Deps{DB: pool, Queries: q, Redis: rdb})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	tokens, err := jwt.NewManager(cfg.JWT.AccessSecret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	if err != nil {
		t.Fatal(err)
	}
	return &itest{
		t: t, handler: srv.http.Handler, pool: pool, q: q, tokens: tokens, rdb: rdb, srv: srv,
		suffix: fmt.Sprintf("%d", time.Now().UnixNano()),
	}
}

// stepUp gives the user a fresh step-up grant (as after a password check).
func (it *itest) stepUp(userUUID uuid.UUID) {
	it.t.Helper()
	if _, err := stepup.NewStore(it.rdb, "test").SetGrant(context.Background(), userUUID, "password", 10*time.Minute); err != nil {
		it.t.Fatalf("step-up grant: %v", err)
	}
}

func (it *itest) do(method, path, host, bearer string, body any) (int, envelope) {
	it.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", host)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

func (it *itest) brandCenter(slug string) db.Organization {
	it.t.Helper()
	ctx := context.Background()
	b, err := it.q.GetBrandBySlug(ctx, slug)
	if err != nil {
		it.t.Fatalf("brand %s: %v", slug, err)
	}
	c, err := it.q.GetBrandCenter(ctx, b.ID)
	if err != nil {
		it.t.Fatalf("center %s: %v", slug, err)
	}
	return c
}

func (it *itest) org(name, typ string, parent db.Organization) db.Organization {
	it.t.Helper()
	ctx := context.Background()
	row, err := it.q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t83-" + name + "-" + it.suffix, Name: name + " " + it.suffix, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           typ, ParentID: pgtype.Int8{Int64: parent.ID, Valid: true},
		BrandID: parent.BrandID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul",
		Settings: []byte("{}"),
	})
	if err != nil {
		it.t.Fatalf("create org %s: %v", name, err)
	}
	it.t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM refresh_tokens WHERE organization_id = $1", row.ID)
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", row.ID)
	})
	return row
}

func (it *itest) user(name string, roles ...string) (db.User, string) {
	it.t.Helper()
	ctx := context.Background()
	pw := "Test-Passw0rd!x"
	hash, err := password.Hash(pw)
	if err != nil {
		it.t.Fatal(err)
	}
	u, err := it.q.CreateUser(ctx, db.CreateUserParams{
		Email: fmt.Sprintf("t83-%s-%s@example.test", name, it.suffix), PasswordHash: hash,
		Name: name, Surname: "Test", Status: "active",
		EmailVerifiedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		it.t.Fatalf("create user: %v", err)
	}
	for _, role := range roles {
		if err := it.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: u.ID, Slug: role}); err != nil {
			it.t.Fatalf("assign %s: %v", role, err)
		}
	}
	it.t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", u.ID)
	})
	return u, pw
}

// member adds a membership with the default organization role of the org
// type (or the explicit roleSlugs).
func (it *itest) member(org db.Organization, u db.User, role string, roleSlugs ...string) {
	it.t.Helper()
	ctx := context.Background()
	m, err := it.q.CreateOrganizationMember(ctx, db.CreateOrganizationMemberParams{
		OrganizationID: org.ID, UserID: u.ID, Role: role,
	})
	if err != nil {
		it.t.Fatalf("member: %v", err)
	}
	if len(roleSlugs) == 0 {
		roleSlugs = []string{rbac.DefaultMemberRole(org.Type, role)}
	}
	for _, slug := range roleSlugs {
		if err := it.q.AssignMemberRoleBySlug(ctx, db.AssignMemberRoleBySlugParams{MemberID: m.ID, Slug: slug}); err != nil {
			it.t.Fatalf("member role %s: %v", slug, err)
		}
	}
}

func (it *itest) tokensFrom(code int, env envelope) tokenPair {
	it.t.Helper()
	if code != http.StatusOK {
		it.t.Fatalf("expected 200 with tokens, got %d (%+v)", code, env.Error)
	}
	var tp tokenPair
	if err := json.Unmarshal(env.Data, &tp); err != nil || tp.AccessToken == "" {
		it.t.Fatalf("token payload: %s", env.Data)
	}
	return tp
}

func (it *itest) oid(access string) string {
	it.t.Helper()
	claims, err := it.tokens.ParseAccess(access)
	if err != nil {
		it.t.Fatal(err)
	}
	id, err := claims.OrganizationUUID()
	if err != nil {
		it.t.Fatal(err)
	}
	if id == nil {
		return ""
	}
	return id.String()
}

func errCode(env envelope) string {
	if env.Error == nil {
		return ""
	}
	return env.Error.Code
}

// Acceptance 1: one user, two organizations, two roles; data stays per org;
// oid survives refresh; membership removal drops the org scope.
func TestIntegrationMultiOrgRoles(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	orgA := it.org("a", "dealer", center)
	orgB := it.org("b", "dealer", center)
	u, pw := it.user("multi")
	it.member(orgA, u, "owner")
	it.member(orgB, u, "staff")

	tp := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": u.Email, "password": pw, "organization_slug": orgA.Slug,
	}))
	if got := it.oid(tp.AccessToken); got != orgA.Uuid.String() {
		t.Fatalf("login oid = %s, want A", got)
	}
	if code, env := it.do("GET", "/v1/tenant/settings", hostOlex, tp.AccessToken, nil); code != http.StatusOK {
		t.Fatalf("owner endpoint in A: %d %s", code, errCode(env))
	}

	tp = it.tokensFrom(it.do("POST", "/v1/auth/organization-context", hostOlex, tp.AccessToken,
		map[string]string{"organization_slug": orgB.Slug}))
	if got := it.oid(tp.AccessToken); got != orgB.Uuid.String() {
		t.Fatalf("switch oid = %s, want B", got)
	}
	if code, _ := it.do("GET", "/v1/tenant/settings", hostOlex, tp.AccessToken, nil); code != http.StatusForbidden {
		t.Fatalf("owner endpoint in B (staff) must be 403, got %d", code)
	}

	tp = it.tokensFrom(it.do("POST", "/v1/auth/refresh", hostOlex, "", map[string]string{"refresh_token": tp.RefreshToken}))
	if got := it.oid(tp.AccessToken); got != orgB.Uuid.String() {
		t.Fatalf("refresh must keep oid B, got %q", got)
	}

	if _, err := it.pool.Exec(context.Background(),
		"DELETE FROM organization_members WHERE organization_id = $1 AND user_id = $2", orgB.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	tp = it.tokensFrom(it.do("POST", "/v1/auth/refresh", hostOlex, "", map[string]string{"refresh_token": tp.RefreshToken}))
	if got := it.oid(tp.AccessToken); got != "" {
		t.Fatalf("refresh after membership removal must drop oid, got %s", got)
	}
	if code, env := it.do("GET", "/v1/tenant/settings", hostOlex, tp.AccessToken, nil); code != http.StatusForbidden ||
		errCode(env) != "ORGANIZATION_CONTEXT_REQUIRED" {
		t.Fatalf("org route after removal: %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/auth/organization-context", hostOlex, tp.AccessToken,
		map[string]string{"organization_slug": orgB.Slug}); code != http.StatusForbidden || errCode(env) != "NO_TENANT_MEMBERSHIP" {
		t.Fatalf("switch back to removed org: %d %s", code, errCode(env))
	}
}

// Acceptance 2: Olex domain never exposes Glorian organizations.
func TestIntegrationBrandIsolation(t *testing.T) {
	it := newIntegration(t)
	olexCenter := it.brandCenter("olex")
	glorianCenter := it.brandCenter("glorian")
	orgOlex := it.org("olex-dealer", "dealer", olexCenter)
	orgGlorian := it.org("glorian-dealer", "dealer", glorianCenter)
	_ = it.org("glorian-dealer-2", "dealer", glorianCenter)

	u, pw := it.user("brand")
	it.member(orgOlex, u, "owner")
	it.member(orgGlorian, u, "owner")

	tp := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": u.Email, "password": pw,
	}))
	if code, env := it.do("POST", "/v1/auth/organization-context", hostOlex, tp.AccessToken,
		map[string]string{"organization_slug": orgGlorian.Slug}); code != http.StatusForbidden || errCode(env) != "BRAND_MISMATCH" {
		t.Fatalf("switch to glorian org on olex domain: %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": u.Email, "password": pw, "organization_slug": orgGlorian.Slug,
	}); code != http.StatusForbidden || errCode(env) != "BRAND_MISMATCH" {
		t.Fatalf("login into glorian org on olex domain: %d %s", code, errCode(env))
	}

	// A token scoped on the glorian domain is rejected on the olex domain.
	gtp := it.tokensFrom(it.do("POST", "/v1/auth/organization-context", hostGlorian, tp.AccessToken,
		map[string]string{"organization_slug": orgGlorian.Slug}))
	if code, env := it.do("GET", "/v1/tenant/settings", hostOlex, gtp.AccessToken, nil); code != http.StatusForbidden ||
		errCode(env) != "BRAND_MISMATCH" {
		t.Fatalf("glorian-scoped token on olex domain: %d %s", code, errCode(env))
	}

	slugsOf := func(host string) map[string]bool {
		code, env := it.do("GET", "/v1/me/organizations", host, tp.AccessToken, nil)
		if code != http.StatusOK {
			t.Fatalf("me/organizations: %d", code)
		}
		var payload struct {
			Items []struct {
				Slug  string `json:"slug"`
				Type  string `json:"type"`
				Brand struct {
					Slug string `json:"slug"`
				} `json:"brand"`
				Parent *struct {
					UUID string `json:"uuid"`
				} `json:"parent"`
			} `json:"items"`
		}
		if err := json.Unmarshal(env.Data, &payload); err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, item := range payload.Items {
			if item.Type != "dealer" || item.Parent == nil {
				t.Fatalf("membership missing tree fields: %+v", item)
			}
			out[item.Brand.Slug+"/"+item.Slug] = true
		}
		return out
	}
	olexView := slugsOf(hostOlex)
	if !olexView["olex/"+orgOlex.Slug] || len(olexView) != 1 {
		t.Fatalf("olex domain memberships: %v", olexView)
	}
	glorianView := slugsOf(hostGlorian)
	if !glorianView["glorian/"+orgGlorian.Slug] || len(glorianView) != 1 {
		t.Fatalf("glorian domain memberships: %v", glorianView)
	}

	// Platform list: counts per domain match the brand's rows only.
	admin, apw := it.user("admin", rbac.RoleSuperAdmin)
	_ = admin
	atp := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email, "password": apw,
	}))
	totalOf := func(host string) int64 {
		code, env := it.do("GET", "/v1/platform/organizations?limit=100", host, atp.AccessToken, nil)
		if code != http.StatusOK {
			t.Fatalf("platform list on %s: %d %s", host, code, errCode(env))
		}
		var page struct {
			Total int64 `json:"total"`
			Items []struct {
				Brand struct {
					Slug string `json:"slug"`
				} `json:"brand"`
			} `json:"items"`
		}
		if err := json.Unmarshal(env.Data, &page); err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			want := map[string]string{hostOlex: "olex", hostGlorian: "glorian"}[host]
			if item.Brand.Slug != want {
				t.Fatalf("%s list leaked brand %s", host, item.Brand.Slug)
			}
		}
		return page.Total
	}
	count := func(brandID int64) int64 {
		var n int64
		if err := it.pool.QueryRow(context.Background(),
			"SELECT COUNT(*) FROM organizations WHERE brand_id = $1 AND deleted_at IS NULL", brandID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got, want := totalOf(hostOlex), count(olexCenter.BrandID); got != want {
		t.Fatalf("olex total = %d, want %d", got, want)
	}
	if got, want := totalOf(hostGlorian), count(glorianCenter.BrandID); got != want {
		t.Fatalf("glorian total = %d, want %d", got, want)
	}
	if code, _ := it.do("GET", "/v1/platform/organizations/"+orgGlorian.Uuid.String(), hostOlex, atp.AccessToken, nil); code != http.StatusNotFound {
		t.Fatalf("glorian org detail on olex domain must be 404, got %d", code)
	}

	// Public brand endpoint.
	code, env := it.do("GET", "/v1/public/brand", hostGlorian, "", nil)
	var pb struct {
		Slug   string `json:"slug"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(env.Data, &pb)
	if code != http.StatusOK || pb.Slug != "glorian" || pb.Status != "inactive" {
		t.Fatalf("public brand glorian: %d %+v", code, pb)
	}
	code, env = it.do("GET", "/v1/public/brand", "unknown.example", "", nil)
	_ = json.Unmarshal(env.Data, &pb)
	if code != http.StatusOK || pb.Slug != "olex" {
		t.Fatalf("public brand default: %d %+v", code, pb)
	}
}

// Platform tree endpoints: create distributor with warehouse preset, dealer
// below it, list children, and move the dealer back to the center.
func TestIntegrationPlatformTree(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	admin, apw := it.user("tree-admin", rbac.RoleSuperAdmin)
	owner, _ := it.user("tree-owner")
	atp := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email, "password": apw,
	}))
	cleanup := func(uuid string) {
		t.Cleanup(func() {
			_, _ = it.pool.Exec(context.Background(), "DELETE FROM organizations WHERE uuid = $1", uuid)
		})
	}
	type orgPayload struct {
		UUID     string         `json:"uuid"`
		Type     string         `json:"type"`
		Settings map[string]any `json:"settings"`
		Parent   *struct {
			UUID string `json:"uuid"`
		} `json:"parent"`
	}
	create := func(body map[string]any) (int, orgPayload, string) {
		code, env := it.do("POST", "/v1/platform/organizations", hostOlex, atp.AccessToken, body)
		var o orgPayload
		_ = json.Unmarshal(env.Data, &o)
		return code, o, errCode(env)
	}
	code, dist, ec := create(map[string]any{
		"name": "Dist " + it.suffix, "city": "", "district": "", "phone": "", "address": "",
		"owner_user_uuid": owner.Uuid.String(), "type": "distributor", "register_as_warehouse": true,
	})
	if code != http.StatusCreated || dist.Type != "distributor" || dist.Settings["register_as_warehouse"] != true ||
		dist.Parent == nil || dist.Parent.UUID != center.Uuid.String() {
		t.Fatalf("create distributor: %d %s %+v", code, ec, dist)
	}
	// Created first, removed last (cleanups run LIFO; the dealer references it).
	cleanup(dist.UUID)
	code, dealer, ec := create(map[string]any{
		"name": "Dealer " + it.suffix, "city": "", "district": "", "phone": "", "address": "",
		"owner_user_uuid": owner.Uuid.String(), "type": "dealer", "parent_uuid": dist.UUID,
	})
	if code != http.StatusCreated || dealer.Parent == nil || dealer.Parent.UUID != dist.UUID {
		t.Fatalf("create dealer: %d %s %+v", code, ec, dealer)
	}
	cleanup(dealer.UUID)
	if code, _, _ := create(map[string]any{
		"name": "Bad " + it.suffix, "city": "", "district": "", "phone": "", "address": "",
		"owner_user_uuid": owner.Uuid.String(), "type": "distributor", "parent_uuid": dealer.UUID,
	}); code != http.StatusBadRequest {
		t.Fatalf("distributor under dealer must be 400, got %d", code)
	}

	code, env := it.do("GET", "/v1/platform/organizations/"+dist.UUID+"/children", hostOlex, atp.AccessToken, nil)
	var children struct {
		Items []orgPayload `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &children)
	if code != http.StatusOK || len(children.Items) != 1 || children.Items[0].UUID != dealer.UUID {
		t.Fatalf("children: %d %+v", code, children)
	}

	// TEC-85: a supplier (parent) change is sensitive and needs a step-up.
	if code, env := it.do("PATCH", "/v1/platform/organizations/"+dealer.UUID, hostOlex, atp.AccessToken,
		map[string]any{"parent_uuid": center.Uuid.String()}); code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("parent change without step-up: %d %s", code, errCode(env))
	}
	it.stepUp(admin.Uuid)

	// Cycle: distributor under its own dealer is rejected.
	if code, _ := it.do("PATCH", "/v1/platform/organizations/"+dist.UUID, hostOlex, atp.AccessToken,
		map[string]any{"parent_uuid": dealer.UUID}); code != http.StatusBadRequest {
		t.Fatalf("cycle must be 400, got %d", code)
	}
	code, env = it.do("PATCH", "/v1/platform/organizations/"+dealer.UUID, hostOlex, atp.AccessToken,
		map[string]any{"parent_uuid": center.Uuid.String(), "status": "read_only"})
	var moved orgPayload
	_ = json.Unmarshal(env.Data, &moved)
	if code != http.StatusOK || moved.Parent == nil || moved.Parent.UUID != center.Uuid.String() {
		t.Fatalf("move dealer: %d %s %+v", code, errCode(env), moved)
	}
}
