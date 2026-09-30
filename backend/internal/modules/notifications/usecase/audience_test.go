package usecase

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

func TestResolvePlatformAudienceDefaultsToSelf(t *testing.T) {
	t.Parallel()

	target := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	lookup := func(id uuid.UUID) (int64, error) {
		if id == target {
			return 99, nil
		}
		return 0, ErrNotFound
	}

	self := authctx.Principal{UserInternal: 7}
	got, err := resolvePlatformAudience(self, audienceAll, target.String(), lookup)
	if err != nil {
		t.Fatal(err)
	}
	if !got.UserID.Valid || got.UserID.Int64 != 7 || got.IncludeUser {
		t.Fatalf("expected self-only filter, got %+v", got)
	}

	admin := authctx.Principal{
		UserInternal: 7,
		Permissions:  []string{rbac.PermPlatformNotificationsReadAll},
	}
	all, err := resolvePlatformAudience(admin, audienceAll, "", lookup)
	if err != nil {
		t.Fatal(err)
	}
	if all.UserID.Valid || !all.IncludeUser {
		t.Fatalf("expected unscoped list with user column, got %+v", all)
	}

	one, err := resolvePlatformAudience(admin, "", target.String(), lookup)
	if err != nil {
		t.Fatal(err)
	}
	if !one.UserID.Valid || one.UserID.Int64 != 99 || !one.IncludeUser {
		t.Fatalf("expected target user filter, got %+v", one)
	}

	me, err := resolvePlatformAudience(admin, "", "", lookup)
	if err != nil {
		t.Fatal(err)
	}
	if !me.UserID.Valid || me.UserID.Int64 != 7 || me.IncludeUser {
		t.Fatalf("expected admin default of self, got %+v", me)
	}
}
