package jwt

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestManager(t *testing.T, now *time.Time) *Manager {
	t.Helper()
	m, err := NewManager("test-secret-test-secret-test-secret", 15*time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	m.SetClock(func() time.Time { return *now })
	return m
}

// TEC-284 (F2-FIX-3): the legacy token outlives the access TTL until its
// session's end, is refused by the regular manager and parsed only by the
// AcceptLegacy copy; a regular token keeps its 15 minutes on both.
func TestLegacyAccessToken(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	m := newTestManager(t, &now)
	legacy := m.AcceptLegacy()
	oid, sid, user := uuid.New(), uuid.New(), uuid.New()
	in := AccessInput{UserID: user, SessionID: sid, OrganizationID: &oid, Roles: []string{"x"}}

	tok, exp, err := m.IssueLegacyAccess(in, now.Add(30*24*time.Hour))
	if err != nil || !exp.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("issue: %v %v", exp, err)
	}
	regular, _, err := m.IssueAccess(AccessInput{UserID: user, SessionID: sid, Audience: AudienceMobile})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.ParseAccess(tok); err == nil {
		t.Fatal("the regular manager must reject a legacy token")
	}
	c, err := legacy.ParseAccess(tok)
	if err != nil || !c.Legacy || c.Realm() != AudienceMobile || c.SessionUUID() != sid || c.Subject != user.String() {
		t.Fatalf("legacy parse = %+v %v", c, err)
	}
	if o, _ := c.OrganizationUUID(); o == nil || *o != oid {
		t.Fatalf("oid = %v", o)
	}
	if c, err := legacy.ParseAccess(regular); err != nil || c.Legacy {
		t.Fatalf("the legacy manager must still parse a regular token: %v", err)
	}

	// 16 minutes later: the regular token is expired, the legacy one is not.
	now = now.Add(16 * time.Minute)
	if _, err := m.ParseAccess(regular); err == nil {
		t.Fatal("regular token must expire after the access TTL")
	}
	if _, err := legacy.ParseAccess(regular); err == nil {
		t.Fatal("regular token must expire on the legacy manager too")
	}
	if _, err := legacy.ParseAccess(tok); err != nil {
		t.Fatalf("legacy token after 16 minutes: %v", err)
	}
	// Past the session's end it expires too.
	now = now.Add(30 * 24 * time.Hour)
	if _, err := legacy.ParseAccess(tok); err == nil {
		t.Fatal("legacy token must expire with its session")
	}
}

func TestLegacyAccessTokenNeedsSessionAndFutureExpiry(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	m := newTestManager(t, &now)
	if _, _, err := m.IssueLegacyAccess(AccessInput{UserID: uuid.New()}, now.Add(time.Hour)); err == nil {
		t.Fatal("a legacy token without a session must be refused")
	}
	if _, _, err := m.IssueLegacyAccess(AccessInput{UserID: uuid.New(), SessionID: uuid.New()}, now); err == nil {
		t.Fatal("a legacy token expiring now must be refused")
	}
	// The audience is always mobile, whatever the input says.
	tok, _, err := m.IssueLegacyAccess(AccessInput{UserID: uuid.New(), SessionID: uuid.New(), Audience: AudiencePanel}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if c, err := m.AcceptLegacy().ParseAccess(tok); err != nil || c.Realm() != AudienceMobile {
		t.Fatalf("legacy realm = %v %v", c.Realm(), err)
	}
}
