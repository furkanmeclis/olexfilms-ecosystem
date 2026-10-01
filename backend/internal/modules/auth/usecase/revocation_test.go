package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authrevoke"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
)

func newRevocationUC(t *testing.T) (*AuthUseCase, *memRepo) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	repo := newMemRepo()
	uc := New(repo, tokens)
	uc.SetRevocations(authrevoke.New(rdb, "test", time.Minute))
	return uc, repo
}

func TestEndAccessSessionWithoutRefreshRevokesSession(t *testing.T) {
	ctx := context.Background()
	uc, repo := newRevocationUC(t)
	user, sid, other := uuid.New(), uuid.New(), uuid.New()

	uc.EndAccessSession(ctx, "  ", 7, sid)

	if len(repo.revokedSessions) == 0 || repo.revokedSessions[0] != sid {
		t.Fatalf("refresh session not revoked: %v", repo.revokedSessions)
	}
	if !uc.AccessRevoked(ctx, user, sid, time.Now()) {
		t.Fatal("access token of the logged-out session still accepted")
	}
	if uc.AccessRevoked(ctx, user, other, time.Now()) {
		t.Fatal("logout revoked another session")
	}
}

func TestEndAccessSessionWithRefreshKeepsRepoUntouched(t *testing.T) {
	ctx := context.Background()
	uc, repo := newRevocationUC(t)
	sid := uuid.New()

	uc.EndAccessSession(ctx, "refresh-token", 7, sid)

	if len(repo.revokedSessions) != 0 {
		t.Fatalf("refresh token path must not revoke by sid: %v", repo.revokedSessions)
	}
	if !uc.AccessRevoked(ctx, uuid.New(), sid, time.Now()) {
		t.Fatal("access token still accepted after logout")
	}
}

func TestAccessRevokedWithoutStoreFailsOpen(t *testing.T) {
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uc := New(newMemRepo(), tokens)
	uc.EndAccessSession(context.Background(), "", 7, uuid.New())
	if uc.AccessRevoked(context.Background(), uuid.New(), uuid.New(), time.Now()) {
		t.Fatal("nil store must not revoke")
	}
}
