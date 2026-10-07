package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	oauthmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	oauthusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestMaskArguments(t *testing.T) {
	got := MaskArguments(json.RawMessage(`{"contact_name":"Ali Veli","phone":"+905321234567","status":"open",
		"limit":5,"q":"ali","note":"x","service":"DS0001","items":[{"customer_email":"a@b.co"}],
		"free":"call +90 532 123 45 67","flag":true,"due_date":"2026-10-07","lead_uuid":"7b6e1d2c-0000-4000-8000-000000000000"}`))
	raw, _ := json.Marshal(got)
	s := string(raw)
	for _, leak := range []string{"Ali Veli", "905321234567", "a@b.co", "532 123", `"ali"`} {
		if strings.Contains(s, leak) {
			t.Fatalf("personal value %q logged: %s", leak, s)
		}
	}
	m := got.(map[string]any)
	if m["status"] != "open" || m["limit"] != float64(5) || m["service"] != "DS0001" || m["flag"] != true || m["due_date"] != "2026-10-07" ||
		m["lead_uuid"] != "7b6e1d2c-0000-4000-8000-000000000000" {
		t.Fatalf("non-personal values changed: %s", s)
	}
	if MaskArguments(json.RawMessage(`{bad`)) != Masked {
		t.Fatal("invalid JSON must not be logged")
	}
	if long := MaskArguments(json.RawMessage(`{"service":"` + strings.Repeat("x", 100) + `"}`)).(map[string]any)["service"].(string); len([]rune(long)) != 65 {
		t.Fatalf("long value not cut: %d", len([]rune(long)))
	}
}

func TestBearer(t *testing.T) {
	for in, want := range map[string]string{"Bearer abc": "abc", "bearer  abc ": "abc", "Basic abc": "", "": "", "Bearer": ""} {
		if got := bearer(in); got != want {
			t.Fatalf("bearer(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- HTTP gate ---------------------------------------------------------------

type fakeTokens struct {
	tok oauthmodel.AccessToken
	err error
}

func (f fakeTokens) ValidateAccessToken(_ context.Context, raw, resource string) (oauthmodel.AccessToken, error) {
	if raw != "good" {
		return oauthmodel.AccessToken{}, oauthusecase.ErrUnauthorized
	}
	if f.err != nil {
		return oauthmodel.AccessToken{}, f.err
	}
	t := f.tok
	t.Resource = resource
	return t, nil
}

func (fakeTokens) ResourceMetadataURL(resource string) string {
	return "https://olex.test/.well-known/oauth-protected-resource" + resource
}

type fakeResolver struct{ err error }

func (f fakeResolver) Resolve(context.Context, oauthmodel.AccessToken) (aitools.Principal, error) {
	return aitools.Principal{Realm: aitools.RealmPanel}, f.err
}

type fakeLimiter struct {
	limits map[string]int
	deny   string
}

func (f *fakeLimiter) Allow(_ context.Context, action, _ string, limit int, _ time.Duration) (bool, time.Duration) {
	f.limits[action] = limit
	if action == f.deny {
		return false, 90 * time.Second
	}
	return true, 0
}

type fakeSettings int

func (f fakeSettings) MCPRequestsPerHourPerOrg(context.Context) int { return int(f) }

type emptyRegistry struct{}

func (emptyRegistry) Available(context.Context, aitools.Principal) ([]aitools.Tool, error) {
	return nil, nil
}
func (emptyRegistry) Call(context.Context, aitools.Principal, string, json.RawMessage) (aitools.Result, error) {
	return aitools.Result{}, nil
}

func serve(s *Server, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/mcp/dealer", strings.NewReader(`{"jsonrpc":"2.0","id":"r1","method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestServerGate(t *testing.T) {
	tok := oauthmodel.AccessToken{Family: uuid.New(), OrganizationID: 9, UserID: 3}
	lim := &fakeLimiter{limits: map[string]int{}}
	s := New(Config{Tokens: fakeTokens{tok: tok}, Principals: fakeResolver{}, Tools: emptyRegistry{},
		Limiter: lim, Settings: fakeSettings(25)})

	rec := serve(s, "")
	if rec.Code != http.StatusUnauthorized ||
		rec.Header().Get("WWW-Authenticate") != `Bearer resource_metadata="https://olex.test/.well-known/oauth-protected-resource/mcp/dealer"` {
		t.Fatalf("no token = %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	if rec := serve(s, "Bearer bad"); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Header().Get("WWW-Authenticate"), `error="invalid_token"`) {
		t.Fatalf("bad token = %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}

	// Allowed: both limits applied, the organization one from sysconfig.
	if rec := serve(s, "Bearer good"); rec.Code != http.StatusOK || lim.limits["mcp_token"] != TokenRequestsPerMinute || lim.limits["mcp_org"] != 25 {
		t.Fatalf("allowed = %d %s limits %v", rec.Code, rec.Body, lim.limits)
	}

	lim.deny = "mcp_org"
	rec = serve(s, "Bearer good")
	var body struct {
		JSONRPC string `json:"jsonrpc"`
		ID      string `json:"id"`
		Error   struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusTooManyRequests || body.JSONRPC != "2.0" || body.ID != "r1" || body.Error.Code != CodeRateLimited ||
		rec.Header().Get("Retry-After") != "90" {
		t.Fatalf("org limit = %d %s %q", rec.Code, rec.Body, rec.Header().Get("Retry-After"))
	}

	off := New(Config{Tokens: fakeTokens{tok: tok, err: oauthusecase.ErrFeatureDisabled}, Principals: fakeResolver{}, Tools: emptyRegistry{}})
	if rec := serve(off, "Bearer good"); rec.Code != http.StatusForbidden {
		t.Fatalf("module off = %d", rec.Code)
	}
	gone := New(Config{Tokens: fakeTokens{tok: tok}, Principals: fakeResolver{err: ErrForbidden}, Tools: emptyRegistry{}})
	if rec := serve(gone, "Bearer good"); rec.Code != http.StatusForbidden {
		t.Fatalf("membership lost = %d", rec.Code)
	}
}

// --- principal ----------------------------------------------------------------

type fakeQ struct {
	org    db.Organization
	member bool
}

func (f fakeQ) GetUserByID(_ context.Context, id int64) (db.User, error) {
	return db.User{ID: id, Uuid: uuid.New(), Status: "active"}, nil
}
func (f fakeQ) GetOrganizationByID(context.Context, int64) (db.Organization, error) {
	return f.org, nil
}
func (f fakeQ) GetBrandByID(_ context.Context, id int64) (db.Brand, error) {
	return db.Brand{ID: id, Slug: "olex", Status: "active"}, nil
}
func (f fakeQ) GetOrganizationMemberByUserAndOrgUUID(context.Context, db.GetOrganizationMemberByUserAndOrgUUIDParams) (db.GetOrganizationMemberByUserAndOrgUUIDRow, error) {
	if !f.member {
		return db.GetOrganizationMemberByUserAndOrgUUIDRow{}, pgx.ErrNoRows
	}
	return db.GetOrganizationMemberByUserAndOrgUUIDRow{Role: "owner"}, nil
}

type fakeAccess struct{ acc authusecase.Access }

func (f fakeAccess) ResolveStoredAccess(context.Context, int64, *uuid.UUID) (authusecase.Access, error) {
	return f.acc, nil
}

func TestStoreResolver(t *testing.T) {
	ctx := context.Background()
	dealer := db.Organization{ID: 5, Uuid: uuid.New(), Slug: "d", Type: rbac.OrgTypeDealer, Status: "active", BrandID: 1}
	center := dealer
	center.Type = rbac.OrgTypeCenter
	connect := authusecase.Access{Grants: map[string]rbac.Scope{rbac.PermMCPConnect: rbac.ScopeOwn}}
	tok := func(resource string) oauthmodel.AccessToken {
		return oauthmodel.AccessToken{UserID: 3, OrganizationID: 5, BrandID: 1, Resource: resource}
	}

	p, err := StoreResolver{Q: fakeQ{org: dealer, member: true}, Access: fakeAccess{connect}}.Resolve(ctx, tok(oauthmodel.ResourceDealer))
	if err != nil || p.Realm != aitools.RealmPanel || p.Org == nil || p.Org.InternalID != 5 || p.Org.BrandSlug != "olex" || p.Auth.UserInternal != 3 {
		t.Fatalf("dealer principal = %+v %v", p, err)
	}
	p, err = StoreResolver{Q: fakeQ{org: center}, Access: fakeAccess{}}.Resolve(ctx, tok(oauthmodel.ResourceCustomer))
	if err != nil || p.Realm != aitools.RealmCustomer || p.Org != nil || p.Brand == nil || p.Brand.ID != 1 {
		t.Fatalf("customer principal = %+v %v", p, err)
	}

	suspended := dealer
	suspended.Status = "suspended"
	ended := dealer
	ended.AccessEndsAt = pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}
	for name, c := range map[string]struct {
		q        fakeQ
		acc      authusecase.Access
		resource string
	}{
		"membership lost":      {fakeQ{org: dealer}, connect, oauthmodel.ResourceDealer},
		"mcp.connect lost":     {fakeQ{org: dealer, member: true}, authusecase.Access{Grants: map[string]rbac.Scope{}}, oauthmodel.ResourceDealer},
		"center on dealer":     {fakeQ{org: center, member: true}, connect, oauthmodel.ResourceDealer},
		"suspended":            {fakeQ{org: suspended, member: true}, connect, oauthmodel.ResourceUser},
		"access window closed": {fakeQ{org: ended, member: true}, connect, oauthmodel.ResourceUser},
	} {
		if _, err := (StoreResolver{Q: c.q, Access: fakeAccess{c.acc}}).Resolve(ctx, tok(c.resource)); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s: err = %v, want ErrForbidden", name, err)
		}
	}
	// The platform admin acts for the center without a membership.
	if _, err := (StoreResolver{Q: fakeQ{org: center}, Access: fakeAccess{authusecase.Access{IsSuperAdmin: true}}}).Resolve(ctx, tok(oauthmodel.ResourceUser)); err != nil {
		t.Fatalf("super admin on center: %v", err)
	}
}

func TestToResultAndSummary(t *testing.T) {
	r := toResult("list_leads", aitools.Result{Content: `{"items":[{"a":1}],"returned":1,"total":7}`})
	if r.IsError || len(r.Content) != 2 || r.StructuredContent == nil {
		t.Fatalf("list result = %+v", r)
	}
	if got := summary("list_leads", r.StructuredContent.(map[string]any)); got != "list_leads: 1 of 7 items." {
		t.Fatalf("summary = %q", got)
	}
	e := toResult("x", aitools.ErrorResult(aitools.CodeNotFound, "nope"))
	if !e.IsError || e.StructuredContent.(map[string]any)["error"].(map[string]string)["code"] != aitools.CodeNotFound {
		t.Fatalf("error result = %+v", e)
	}
	if u := ApprovalURL("https://olex.test", "bayi-1", uuid.Nil); u != "https://olex.test/t/bayi-1/ai/approvals?action=00000000-0000-0000-0000-000000000000" {
		t.Fatalf("approval url = %q", u)
	}
}
