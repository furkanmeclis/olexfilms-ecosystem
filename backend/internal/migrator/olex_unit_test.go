package migrator

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/normalize"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
)

// A Laravel $2y$ hash survives the migration and logs in with the legacy
// password; anything else is marked "reset required" (TEC-254).
func TestMigratedPasswordHash(t *testing.T) {
	// bcrypt cost 10 of "eski-sifre", in Laravel's $2y$ form.
	const legacy = "$2y$10$Ncl7Rt8nC3FBorv/7YtFnuoBuNhkUwYPBuFp7em6/7fDphhJDmidO"

	got := migratedPasswordHash(legacy)
	if got != legacy {
		t.Fatalf("bcrypt hash not kept: %q", got)
	}
	if ok, err := password.Verify(got, "eski-sifre"); err != nil || !ok {
		t.Fatalf("login with the legacy password = %v, %v", ok, err)
	}
	if ok, _ := password.Verify(got, "baska"); ok {
		t.Fatal("wrong password verified")
	}

	for _, bad := range []string{
		"",
		"$2y$12$syntheticsyntheticsyntheticsyntheticsyntheticsynthe", // legacy fixture placeholder
		"md5:5f4dcc3b5aa765d61d8327deb882cf99",
	} {
		if got := migratedPasswordHash(bad); got != password.ResetRequired {
			t.Errorf("migratedPasswordHash(%q) = %q, want reset marker", bad, got)
		}
	}
}

func TestSplitName(t *testing.T) {
	cases := []struct{ in, name, surname string }{
		{"Sentetik Bayi Bir Sahibi", "Sentetik Bayi Bir", "Sahibi"},
		{"  Ayşe   Yılmaz ", "Ayşe", "Yılmaz"},
		{"Tek", "Tek", ""},
		{"", "-", ""},
	}
	for _, c := range cases {
		n, s := splitName(c.in)
		if n != c.name || s != c.surname {
			t.Errorf("splitName(%q) = %q, %q; want %q, %q", c.in, n, s, c.name, c.surname)
		}
	}
}

func TestDefaultRoleMapTargets(t *testing.T) {
	for name, target := range DefaultRoleMap() {
		switch target.At {
		case RoleAtPlatform, RoleAtCenter, RoleAtDealer:
		default:
			t.Errorf("%s: unknown target %q", name, target.At)
		}
		if target.Owner && target.At != RoleAtDealer {
			t.Errorf("%s: owner flag outside a dealer", name)
		}
	}
	if DefaultRoleMap()["dealer_owner"] != (RoleTarget{Slug: "dealer_owner", At: RoleAtDealer, Owner: true}) {
		t.Error("dealer_owner must become an owner membership")
	}
}

// The legacy fixture's phone forms resolve to E.164 (K29).
func TestFixturePhonesNormalize(t *testing.T) {
	for raw, want := range map[string]string{
		"0555 000 01 01": "+905550000101",
		"05550000001":    "+905550000001",
	} {
		got := normalize.NormalizePhone(raw, TRCountry)
		if !got.Verified || got.E164 != want {
			t.Errorf("%q -> %+v, want %s", raw, got, want)
		}
	}
}

// Step queries stay inside the read-only guard, in full and delta form.
func TestOlexQueriesPassGuard(t *testing.T) {
	for _, q := range []string{
		dealersQuery + " ORDER BY id",
		dealersQuery + " WHERE COALESCE(updated_at, created_at) > ? ORDER BY id",
		usersQuery + " ORDER BY id",
		usersQuery + " WHERE COALESCE(updated_at, created_at) > ? ORDER BY id",
		legacyRolesQuery,
	} {
		if err := source.CheckReadOnly(q); err != nil {
			t.Errorf("guard refused %q: %v", q, err)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncate("İstanbul", 3); got != "İst" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("truncate = %q", got)
	}
}
