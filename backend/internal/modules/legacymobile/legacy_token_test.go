package legacymobile

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// fakeSessions issues legacy tokens with the real manager and keeps a set
// of revoked sessions.
type fakeSessions struct {
	tokens  *jwt.Manager
	revoked map[uuid.UUID]bool
	issued  []string
}

func (f *fakeSessions) IssueLegacyMobileAccess(_ context.Context, access string) (string, error) {
	f.issued = append(f.issued, access)
	c, err := f.tokens.ParseAccess(access)
	if err != nil {
		return "", err
	}
	user, _ := c.UserUUID()
	org, _ := c.OrganizationUUID()
	tok, _, err := f.tokens.IssueLegacyAccess(jwt.AccessInput{
		UserID: user, SessionID: c.SessionUUID(), OrganizationID: org,
	}, time.Now().Add(30*24*time.Hour))
	return tok, err
}

func (f *fakeSessions) CheckLegacyMobileSession(_ context.Context, c jwt.Claims) error {
	if f.revoked[c.SessionUUID()] {
		return errors.New("revoked")
	}
	return nil
}

// TEC-284 (F2-FIX-3): the legacy login answers the long-lived legacy token
// of the final (switched) session instead of the 15 minute access token.
func TestLoginAnswersLegacyToken(t *testing.T) {
	tokens := newTokens(t)
	user, sid := uuid.New(), uuid.New()
	org := sampleOrg{UUID: uuid.New(), Slug: "bayi-1", Name: "Olex", Role: "owner", Status: "active", Type: "dealer"}
	access, _, _ := tokens.IssueAccess(jwt.AccessInput{
		UserID: user, SessionID: sid, OrganizationID: &org.UUID, Audience: jwt.AudienceMobile,
	})
	sessions := &fakeSessions{tokens: tokens}
	a := &adapters{h: Handlers{
		Login: func(w http.ResponseWriter, r *http.Request) {
			response.JSON(w, r, http.StatusOK, map[string]any{"access_token": access, "me": sampleMe(user, org, true)})
		},
		Sessions: sessions,
	}, authn: authnFor(tokens)}
	b := check(t, "login", serve("login", a.login, fixtureReq(t, "login")))
	var data struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(sessions.issued) != 1 || sessions.issued[0] != access || data.Token == access {
		t.Fatalf("login must answer the legacy token of the session: issued %v, token %q", sessions.issued, data.Token)
	}
	c, err := tokens.AcceptLegacy().ParseAccess(data.Token)
	if err != nil || !c.Legacy || c.SessionUUID() != sid || c.Realm() != jwt.AudienceMobile {
		t.Fatalf("legacy claims = %+v %v", c, err)
	}

	// The legacy token cannot be issued: the login fails (old 500), it
	// never falls back to a short token silently.
	a.h.Sessions = &failingSessions{}
	rec := serve("login", a.login, fixtureReq(t, "login"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("issue failure = %d %s", rec.Code, rec.Body.String())
	}
}

type failingSessions struct{}

func (failingSessions) IssueLegacyMobileAccess(context.Context, string) (string, error) {
	return "", errors.New("no session")
}

func (failingSessions) CheckLegacyMobileSession(context.Context, jwt.Claims) error {
	return errors.New("no session")
}

// The aliases accept the legacy token while its session lives (also after
// the 15 minute access TTL), refuse it once the session ends or when no
// session checker is wired, and a route outside the aliases refuses it.
func TestLegacyTokenOnlyOnAliases(t *testing.T) {
	now := time.Now()
	tokens, err := jwt.NewManager("test-secret-test-secret-test-secret", 15*time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tokens.SetClock(func() time.Time { return now })
	sid := uuid.New()
	legacy, _, err := tokens.IssueLegacyAccess(jwt.AccessInput{UserID: uuid.New(), SessionID: sid}, now.Add(30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	sessions := &fakeSessions{tokens: tokens, revoked: map[uuid.UUID]bool{}}
	h := stubHandlers()
	h.Sessions = sessions
	mux := http.NewServeMux()
	RegisterRoutes(mux, true, h, tokens, okLoader{}, nil, nil)
	// A regular mobile route behind the regular manager.
	mux.Handle("GET /v1/mobile/auth/me", middleware.Authenticate(tokens, okLoader{})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(299) })))

	me, err := LoadFixture(FixtureDir, "me", testVars)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(16 * time.Minute)
	if rec, _ := call(mux, me, legacy); rec.Code != 299 {
		t.Fatalf("legacy token on the alias after 16 minutes = %d %s", rec.Code, rec.Body.String())
	}
	if rec, _ := call(mux, Fixture{Method: "GET", Path: "/v1/mobile/auth/me"}, legacy); rec.Code != http.StatusUnauthorized {
		t.Fatalf("legacy token on /v1/mobile/auth/me = %d %s, want 401", rec.Code, rec.Body.String())
	}
	sessions.revoked[sid] = true
	if rec, _ := call(mux, me, legacy); rec.Code != http.StatusUnauthorized {
		t.Fatalf("legacy token of a revoked session = %d, want 401", rec.Code)
	} else {
		decodeErr(t, rec)
	}

	h.Sessions = nil
	bare := http.NewServeMux()
	RegisterRoutes(bare, true, h, tokens, okLoader{}, nil, nil)
	if rec, _ := call(bare, me, legacy); rec.Code != http.StatusUnauthorized {
		t.Fatalf("legacy token without a session checker = %d, want 401", rec.Code)
	}
}
