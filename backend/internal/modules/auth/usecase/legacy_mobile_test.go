package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
)

// TEC-284 (F2-FIX-3): the old hub app's legacy token lives as long as its
// device session and dies with it (revoked, rotated, expired, user no
// longer active), checked against the session row, not Redis.
func TestLegacyMobileAccess(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", 15*time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tokens.SetClock(func() time.Time { return now })
	uc := New(repo, tokens)
	uc.now = func() time.Time { return now }

	user, _ := repo.CreateUser(ctx, model.User{Email: "legacy@example.com", Status: "active"}, true)
	sid, oid := uuid.New(), uuid.New()
	session := model.RefreshSession{
		UUID: sid, UserID: user.ID, Client: model.ClientMobile, Realm: jwt.AudienceMobile,
		Device: &model.DeviceInfo{ID: "legacy-1", Platform: "android"}, ExpiresAt: now.Add(30 * 24 * time.Hour),
	}
	repo.refreshByUUID = map[uuid.UUID]model.RefreshSession{sid: session}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{
		UserID: user.UUID, SessionID: sid, OrganizationID: &oid, Audience: jwt.AudienceMobile,
	})
	if err != nil {
		t.Fatal(err)
	}

	legacy, err := uc.IssueLegacyMobileAccess(ctx, access)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.ParseAccess(legacy); err == nil {
		t.Fatal("the regular manager must reject the legacy token")
	}
	c, err := tokens.AcceptLegacy().ParseAccess(legacy)
	if err != nil || !c.Legacy || c.SessionUUID() != sid || !c.ExpiresAt.Time.Equal(session.ExpiresAt) {
		t.Fatalf("legacy claims = %+v %v", c, err)
	}
	if o, _ := c.OrganizationUUID(); o == nil || *o != oid {
		t.Fatalf("legacy oid = %v", o)
	}
	if err := uc.CheckLegacyMobileSession(ctx, c); err != nil {
		t.Fatalf("live session: %v", err)
	}

	// A panel token is not turned into a legacy one.
	panel, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: user.UUID, SessionID: sid})
	if _, err := uc.IssueLegacyMobileAccess(ctx, panel); !errors.Is(err, ErrNotMobileSession) {
		t.Fatalf("panel token = %v, want ErrNotMobileSession", err)
	}

	end := func(name string, mutate func(*model.RefreshSession), want error) {
		t.Helper()
		s := session
		mutate(&s)
		repo.refreshByUUID[sid] = s
		if err := uc.CheckLegacyMobileSession(ctx, c); !errors.Is(err, want) {
			t.Fatalf("%s: %v, want %v", name, err, want)
		}
		repo.refreshByUUID[sid] = session
	}
	revoked := now
	end("revoked (logout / device revoke)", func(s *model.RefreshSession) { s.RevokedAt = &revoked }, ErrSessionRevoked)
	end("rotated", func(s *model.RefreshSession) { s.RotatedAt = &revoked }, ErrSessionRevoked)
	end("expired", func(s *model.RefreshSession) { s.ExpiresAt = now }, ErrSessionRevoked)
	end("another user's session", func(s *model.RefreshSession) { s.UserID = user.ID + 1 }, ErrNotMobileSession)
	delete(repo.refreshByUUID, sid)
	if err := uc.CheckLegacyMobileSession(ctx, c); !errors.Is(err, ErrNotMobileSession) {
		t.Fatalf("missing session: %v", err)
	}
	repo.refreshByUUID[sid] = session

	// Deactivation ends it too.
	u := repo.users[user.ID]
	u.Status = "disabled"
	repo.users[user.ID], repo.byUUID[user.UUID] = u, u
	if err := uc.CheckLegacyMobileSession(ctx, c); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("disabled user: %v", err)
	}

	// A regular token is not checked here (Redis markers cover its 15 minutes).
	if err := uc.CheckLegacyMobileSession(ctx, jwt.Claims{}); err != nil {
		t.Fatalf("regular claims: %v", err)
	}
}
