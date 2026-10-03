package password

import "testing"

func TestHashVerify(t *testing.T) {
	t.Parallel()

	hash, err := Hash("Password1")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	ok, err := Verify(hash, "Password1")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}
	ok, err = Verify(hash, "WrongPass1")
	if err != nil {
		t.Fatalf("Verify wrong: %v", err)
	}
	if ok {
		t.Fatal("expected wrong password to fail")
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "ok", value: "Password1", wantErr: false},
		{name: "short", value: "Pa1", wantErr: true},
		{name: "no_digit", value: "Password", wantErr: true},
		{name: "no_upper", value: "password1", wantErr: true},
		{name: "no_lower", value: "PASSWORD1", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePassword(tc.value)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// LegacyBcryptHash is a Laravel-style bcrypt hash ($2y$, cost 10) of
// "eski-sifre", the form the legacy hub stores in users.password (TEC-254).
const LegacyBcryptHash = "$2y$10$Ncl7Rt8nC3FBorv/7YtFnuoBuNhkUwYPBuFp7em6/7fDphhJDmidO"

func TestVerifyLegacyBcrypt(t *testing.T) {
	t.Parallel()

	if !IsBcrypt(LegacyBcryptHash) || !NeedsRehash(LegacyBcryptHash) {
		t.Fatal("$2y$ hash must be recognised as bcrypt")
	}
	ok, err := Verify(LegacyBcryptHash, "eski-sifre")
	if err != nil || !ok {
		t.Fatalf("Verify legacy = %v, %v", ok, err)
	}
	ok, err = Verify(LegacyBcryptHash, "yanlis")
	if err != nil || ok {
		t.Fatalf("Verify legacy wrong = %v, %v", ok, err)
	}

	// The legacy password predates the policy; Rehash still upgrades it.
	upgraded, err := Rehash("eski-sifre")
	if err != nil {
		t.Fatal(err)
	}
	if NeedsRehash(upgraded) {
		t.Fatal("argon2id hash must not need a rehash")
	}
	if ok, err := Verify(upgraded, "eski-sifre"); err != nil || !ok {
		t.Fatalf("Verify upgraded = %v, %v", ok, err)
	}
}

func TestIsBcryptRejectsMalformed(t *testing.T) {
	t.Parallel()

	for _, h := range []string{
		"",
		ResetRequired,
		// The legacy fixture's placeholder: right prefix, too short.
		"$2y$12$syntheticsyntheticsyntheticsyntheticsyntheticsynthe",
		"$2x$10$Ncl7Rt8nC3FBorv/7YtFnuoBuNhkUwYPBuFp7em6/7fDphhJDmidO",
		"$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"plain-text",
	} {
		if IsBcrypt(h) {
			t.Errorf("IsBcrypt(%q) = true", h)
		}
	}
	if ok, err := Verify(ResetRequired, "anything"); ok || err != nil {
		t.Fatalf("ResetRequired verify = %v, %v", ok, err)
	}
}
