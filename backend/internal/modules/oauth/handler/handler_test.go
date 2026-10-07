package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	oauthmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
)

const issuer = "https://olexfilms.test"

// mux mounts the real routes; metadata needs no database.
func mux() *http.ServeMux {
	m := http.NewServeMux()
	oauthmodule.RegisterRoutes(m, usecase.New(nil, nil, nil, issuer+"/", nil), nil)
	return m
}

func getJSON(t *testing.T, m http.Handler, path string) (int, http.Header, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: body %q: %v", path, rec.Body.String(), err)
	}
	return rec.Code, rec.Header(), body
}

func stringList(t *testing.T, v any) []string {
	t.Helper()
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("not an array: %#v", v)
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		out = append(out, x.(string))
	}
	return out
}

// RFC 8414 fields: issuer, endpoints, S256 only, public clients.
func TestAuthorizationServerMetadata(t *testing.T) {
	code, h, body := getJSON(t, mux(), "/.well-known/oauth-authorization-server")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if h.Get("Access-Control-Allow-Origin") != "*" || h.Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", h)
	}
	want := map[string]string{
		"issuer":                 issuer,
		"authorization_endpoint": issuer + "/oauth/authorize",
		"token_endpoint":         issuer + "/oauth/token",
		"registration_endpoint":  issuer + "/oauth/register",
		"revocation_endpoint":    issuer + "/oauth/revoke",
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %v, want %s", k, body[k], v)
		}
	}
	if got := stringList(t, body["code_challenge_methods_supported"]); !slices.Equal(got, []string{"S256"}) {
		t.Errorf("code_challenge_methods_supported = %v", got)
	}
	if got := stringList(t, body["response_types_supported"]); !slices.Equal(got, []string{"code"}) {
		t.Errorf("response_types_supported = %v", got)
	}
	if got := stringList(t, body["grant_types_supported"]); !slices.Equal(got, []string{"authorization_code", "refresh_token"}) {
		t.Errorf("grant_types_supported = %v", got)
	}
	if got := stringList(t, body["token_endpoint_auth_methods_supported"]); !slices.Equal(got, []string{"none"}) {
		t.Errorf("token_endpoint_auth_methods_supported = %v", got)
	}
	if got := stringList(t, body["scopes_supported"]); !slices.Equal(got, []string{"mcp"}) {
		t.Errorf("scopes_supported = %v", got)
	}
}

// RFC 9728 fields for each MCP endpoint; unknown endpoints are 404.
func TestProtectedResourceMetadata(t *testing.T) {
	m := mux()
	for _, res := range model.Resources {
		code, _, body := getJSON(t, m, "/.well-known/oauth-protected-resource"+res)
		if code != http.StatusOK {
			t.Fatalf("%s: status = %d", res, code)
		}
		if body["resource"] != issuer+res {
			t.Errorf("%s: resource = %v", res, body["resource"])
		}
		if got := stringList(t, body["authorization_servers"]); !slices.Equal(got, []string{issuer}) {
			t.Errorf("%s: authorization_servers = %v", res, got)
		}
		if got := stringList(t, body["bearer_methods_supported"]); !slices.Equal(got, []string{"header"}) {
			t.Errorf("%s: bearer_methods_supported = %v", res, got)
		}
		if got := stringList(t, body["scopes_supported"]); !slices.Equal(got, []string{"mcp"}) {
			t.Errorf("%s: scopes_supported = %v", res, got)
		}
	}
	if code, _, _ := getJSON(t, m, "/.well-known/oauth-protected-resource/mcp/admin"); code != http.StatusNotFound {
		t.Fatalf("unknown resource: status = %d", code)
	}
}

func TestPreflight(t *testing.T) {
	rec := httptest.NewRecorder()
	mux().ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/oauth/token", nil))
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("preflight = %d %v", rec.Code, rec.Header())
	}
}

// fakeService records the inputs and returns preset errors.
type fakeService struct {
	handler.Service
	registerErr error
	tokenIn     model.TokenInput
	tokenErr    error
	ip          string
}

func (f *fakeService) Register(_ context.Context, ip string, in model.RegisterInput) (model.Client, error) {
	f.ip = ip
	if f.registerErr != nil {
		return model.Client{}, f.registerErr
	}
	return model.Client{ClientID: "mcp_x", ClientName: in.ClientName, RedirectURIs: in.RedirectURIs, IssuedAt: time.Unix(100, 0)}, nil
}

func (f *fakeService) Exchange(_ context.Context, ip string, in model.TokenInput) (model.TokenResponse, error) {
	f.ip, f.tokenIn = ip, in
	if f.tokenErr != nil {
		return model.TokenResponse{}, f.tokenErr
	}
	return model.TokenResponse{AccessToken: "a", TokenType: "Bearer", ExpiresIn: 3600, RefreshToken: "r", Scope: "mcp"}, nil
}

func TestRegisterResponses(t *testing.T) {
	svc := &fakeService{}
	h := handler.New(svc, nil)
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"]}`))
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	if rec.Code != http.StatusCreated || svc.ip != "203.0.113.5" {
		t.Fatalf("register = %d ip=%s body=%s", rec.Code, svc.ip, rec.Body)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["client_id"] != "mcp_x" || body["token_endpoint_auth_method"] != "none" || body["client_id_issued_at"] != float64(100) {
		t.Fatalf("body = %v", body)
	}

	rec = httptest.NewRecorder()
	h.Register(rec, httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`not json`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_client_metadata"`) {
		t.Fatalf("bad json = %d %s", rec.Code, rec.Body)
	}
}

// The 21st registration of an IP is 429 slow_down with Retry-After.
func TestRegisterRateLimited(t *testing.T) {
	svc := &fakeService{registerErr: &model.Error{Status: http.StatusTooManyRequests, Code: model.ErrSlowDown, Description: "too many requests", RetryAfter: 30 * time.Minute}}
	rec := httptest.NewRecorder()
	handler.New(svc, nil).Register(rec, httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{"redirect_uris":["https://c.example/cb"]}`)))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "1801" {
		t.Fatalf("rate limited = %d retry=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "slow_down" {
		t.Fatalf("body = %v", body)
	}
}

func TestTokenForm(t *testing.T) {
	svc := &fakeService{}
	h := handler.New(svc, nil)
	form := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {"mcp_x"}, "code": {"c"},
		"redirect_uri": {"https://c.example/cb"}, "code_verifier": {"v"}, "resource": {issuer + "/mcp/dealer"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.Token(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("token = %d %v", rec.Code, rec.Header())
	}
	if svc.tokenIn.Resource != issuer+"/mcp/dealer" || svc.tokenIn.CodeVerifier != "v" || svc.tokenIn.GrantType != "authorization_code" {
		t.Fatalf("input = %+v", svc.tokenIn)
	}

	svc.tokenErr = model.BadRequest(model.ErrInvalidGrant, "PKCE verification failed")
	req = httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.Token(rec, req)
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusBadRequest || body["error"] != "invalid_grant" || body["error_description"] == "" {
		t.Fatalf("invalid_grant = %d %v", rec.Code, body)
	}
}
