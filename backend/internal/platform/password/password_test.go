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
