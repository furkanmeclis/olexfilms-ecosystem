package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

func newLocaleTestUseCase(t *testing.T) (*AuthUseCase, *memRepo, model.User) {
	t.Helper()
	repo := newMemRepo()
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uc := New(repo, tokens)
	user, err := repo.CreateUser(context.Background(), model.User{Email: "loc@example.com", Name: "Loc", Surname: "User", Status: "active"}, true)
	if err != nil {
		t.Fatal(err)
	}
	return uc, repo, user
}

func strPtr(s string) *string { return &s }

func TestUpdateProfileLocaleAndTimezone(t *testing.T) {
	uc, repo, user := newLocaleTestUseCase(t)
	ctx := context.Background()

	me, err := uc.UpdateProfile(ctx, user.UUID, nil, nil, model.ProfilePatch{Locale: strPtr("zh_CN"), Timezone: strPtr("Asia/Dubai")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := repo.byUUID[user.UUID].Locale; got != "zh-CN" {
		t.Fatalf("stored locale = %q, want zh-CN", got)
	}
	if me.User.Locale == nil || *me.User.Locale != "zh-CN" {
		t.Fatalf("me.user.locale = %v", me.User.Locale)
	}
	if me.EffectiveLocale != "zh-CN" || me.EffectiveTimezone != "Asia/Dubai" {
		t.Fatalf("effective = %q / %q", me.EffectiveLocale, me.EffectiveTimezone)
	}

	// tr-TR is stored as tr.
	if _, err := uc.UpdateProfile(ctx, user.UUID, nil, nil, model.ProfilePatch{Locale: strPtr("tr-TR")}); err != nil {
		t.Fatal(err)
	}
	if got := repo.byUUID[user.UUID].Locale; got != "tr" {
		t.Fatalf("stored locale = %q, want tr", got)
	}

	// Empty clears back to inherit.
	me, err = uc.UpdateProfile(ctx, user.UUID, nil, nil, model.ProfilePatch{Locale: strPtr(""), Timezone: strPtr("")})
	if err != nil {
		t.Fatal(err)
	}
	if me.User.Locale != nil || me.User.Timezone != nil {
		t.Fatalf("cleared locale/timezone = %v / %v", me.User.Locale, me.User.Timezone)
	}
	if me.EffectiveLocale != "tr" || me.EffectiveTimezone != i18n.DefaultTimezone {
		t.Fatalf("effective after clear = %q / %q", me.EffectiveLocale, me.EffectiveTimezone)
	}
}

func TestUpdateProfileRejectsInvalidLocaleAndTimezone(t *testing.T) {
	uc, repo, user := newLocaleTestUseCase(t)
	ctx := context.Background()
	for _, bad := range []string{"xx", "klingon", "en_XX_extra!"} {
		_, err := uc.UpdateProfile(ctx, user.UUID, nil, nil, model.ProfilePatch{Locale: strPtr(bad)})
		if bad == "en_XX_extra!" {
			// Primary subtag "en" is supported, so this still maps to en.
			if err != nil {
				t.Fatalf("%q: %v", bad, err)
			}
			continue
		}
		if !errors.Is(err, i18n.ErrInvalidLocale) {
			t.Fatalf("%q: err = %v, want ErrInvalidLocale", bad, err)
		}
	}
	for _, bad := range []string{"Mars/Olympus", "Local", "+03:00"} {
		_, err := uc.UpdateProfile(ctx, user.UUID, nil, nil, model.ProfilePatch{Timezone: strPtr(bad)})
		if !errors.Is(err, i18n.ErrInvalidTimezone) {
			t.Fatalf("%q: err = %v, want ErrInvalidTimezone", bad, err)
		}
	}
	if repo.byUUID[user.UUID].Timezone != "" {
		t.Fatal("invalid timezone must not be stored")
	}
}

func TestMeInheritsOrganizationLocale(t *testing.T) {
	uc, repo, user := newLocaleTestUseCase(t)
	ctx := i18n.WithAcceptLanguage(context.Background(), "fr-FR,fr;q=0.9")
	org := uuid.New()
	repo.orgLocale = map[uuid.UUID][2]string{org: {"de", "Europe/Berlin"}}
	repo.center = [2]string{"tr", "Europe/Istanbul"}

	me, err := uc.Me(ctx, user.UUID, nil, &org)
	if err != nil {
		t.Fatal(err)
	}
	if me.User.Locale != nil {
		t.Fatalf("user locale = %v, want null", *me.User.Locale)
	}
	if me.EffectiveLocale != "de" || me.EffectiveTimezone != "Europe/Berlin" {
		t.Fatalf("effective = %q / %q, want de / Europe/Berlin", me.EffectiveLocale, me.EffectiveTimezone)
	}

	// No active org: the brand center wins over Accept-Language.
	me, err = uc.Me(ctx, user.UUID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if me.EffectiveLocale != "tr" {
		t.Fatalf("effective without org = %q, want tr (center)", me.EffectiveLocale)
	}

	// No org and no center: Accept-Language.
	repo.center = [2]string{}
	me, err = uc.Me(ctx, user.UUID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if me.EffectiveLocale != "fr" || me.EffectiveTimezone != i18n.DefaultTimezone {
		t.Fatalf("effective = %q / %q, want fr / default", me.EffectiveLocale, me.EffectiveTimezone)
	}
}
