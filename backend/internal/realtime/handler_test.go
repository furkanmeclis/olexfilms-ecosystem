package realtime_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/google/uuid"
)

// loader returns a principal whose permissions depend on whether the access
// token carries an organization (oid), like the real identity loader:
// auth.session comes from the active organization's roles.
type loader struct{}

func (loader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, err := c.UserUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	org, err := c.OrganizationUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	p := authctx.Principal{UserID: id, Realm: c.Realm(), OrganizationUUID: org}
	if org != nil {
		p.Permissions = []string{rbac.PermAuthSession}
	}
	return p, nil
}

// authz allows the caller's own user channel and one org channel.
type authz struct{ orgChannel string }

func (a authz) CanSubscribeChannel(_ context.Context, user uuid.UUID, _ bool, ch string) (bool, error) {
	if id, ok := realtime.ParseUserChannel(ch); ok {
		return id == user, nil
	}
	return ch == a.orgChannel, nil
}

type rtEnv struct {
	mux    *http.ServeMux
	tokens *jwt.Manager
	user   uuid.UUID
	org    string
}

func newRTEnv(t *testing.T) rtEnv {
	t.Helper()
	issuer, err := realtime.NewTokenIssuer(config.CentrifugoConfig{
		Enabled: true, TokenHMAC: "unit-test-hmac-secret", TokenTTL: time.Hour,
		WSURL: "ws://127.0.0.1:8000/connection/websocket",
	})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := jwt.NewManager(strings.Repeat("a", 40), 15*time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	org := "workspace:" + uuid.NewString()
	mux := http.NewServeMux()
	realtime.RegisterRoutes(mux, realtime.NewHandler(issuer, authz{orgChannel: org}), tokens, loader{})
	return rtEnv{mux: mux, tokens: tokens, user: uuid.New(), org: org}
}

func (e rtEnv) post(t *testing.T, path, body string, withOrg bool) int {
	t.Helper()
	in := jwt.AccessInput{UserID: e.user, SessionID: uuid.New()}
	if withOrg {
		org := uuid.New()
		in.OrganizationID = &org
	}
	access, _, err := e.tokens.IssueAccess(in)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec.Code
}

// TEC-142: the first tenant page load asks for a connection token before the
// organization context is set; that must not be 403.
func TestConnectionTokenWithoutOrganizationContext(t *testing.T) {
	e := newRTEnv(t)
	if got := e.post(t, "/v1/realtime/connection-token", "", false); got != http.StatusOK {
		t.Fatalf("no org context: status %d, want 200", got)
	}
	if got := e.post(t, "/v1/realtime/connection-token", "", true); got != http.StatusOK {
		t.Fatalf("with org context: status %d, want 200", got)
	}
}

func TestSubscriptionTokenOrganizationContext(t *testing.T) {
	e := newRTEnv(t)
	own := `{"channel":"user:` + e.user.String() + `"}`
	other := `{"channel":"user:` + uuid.NewString() + `"}`
	org := `{"channel":"` + e.org + `"}`

	cases := []struct {
		name    string
		body    string
		withOrg bool
		want    int
	}{
		{"own user channel without org", own, false, http.StatusOK},
		{"own user channel with org", own, true, http.StatusOK},
		{"other user channel without org", other, false, http.StatusForbidden},
		{"other user channel with org", other, true, http.StatusForbidden},
		{"org channel without org", org, false, http.StatusForbidden},
		{"org channel with org", org, true, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.post(t, "/v1/realtime/subscription-token", tc.body, tc.withOrg); got != tc.want {
				t.Fatalf("status %d, want %d", got, tc.want)
			}
		})
	}
}
