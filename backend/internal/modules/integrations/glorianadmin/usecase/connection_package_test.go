package usecase

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TEC-1265: the connection package format is shared with Glorian. Both
// repos keep the same fixture (glorian tests/Fixtures/inventory-connection-package/v1).
const fixtureDir = "testdata/connection-package/v1"

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestConnectionPackageFixtureOpens(t *testing.T) {
	plain, err := openPackage(readFixture(t, "package.json"), string(readFixture(t, "password.txt")))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if !bytes.Equal(plain, readFixture(t, "payload.json")) {
		t.Fatalf("payload differs:\n%s", plain)
	}
	var p packagePayload
	if err := json.Unmarshal(plain, &p); err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || p.APIKey != "sahte-anahtar-0123456789abcdefghijklmnopqrstuv" || !p.Permissions.OrderStatusActions || p.Permissions.DealerWrite {
		t.Fatalf("payload fields = %+v", p)
	}
}

func TestConnectionPackageSealMatchesFixture(t *testing.T) {
	var fixture packageEnvelope
	if err := json.Unmarshal(readFixture(t, "package.json"), &fixture); err != nil {
		t.Fatal(err)
	}
	salt, _ := base64.StdEncoding.DecodeString(fixture.KDF.Salt)
	nonce, _ := base64.StdEncoding.DecodeString(fixture.Cipher.Nonce)

	sealed, err := sealPackage(readFixture(t, "payload.json"), string(readFixture(t, "password.txt")), salt, nonce)
	if err != nil {
		t.Fatal(err)
	}
	var got packageEnvelope
	if err := json.Unmarshal(sealed, &got); err != nil {
		t.Fatal(err)
	}
	if got != fixture {
		t.Fatalf("envelope differs:\n got  %+v\n want %+v", got, fixture)
	}
}

func TestConnectionPackageWrongPasswordAndTamper(t *testing.T) {
	raw := readFixture(t, "package.json")
	if _, err := openPackage(raw, "yanlis-sifre"); !errors.Is(err, errPackageOpen) {
		t.Fatalf("wrong password = %v", err)
	}
	var env packageEnvelope
	_ = json.Unmarshal(raw, &env)
	ct, _ := base64.StdEncoding.DecodeString(env.Ciphertext)
	ct[3] ^= 1
	env.Ciphertext = base64.StdEncoding.EncodeToString(ct)
	tampered, _ := json.Marshal(env)
	if _, err := openPackage(tampered, string(readFixture(t, "password.txt"))); !errors.Is(err, errPackageOpen) {
		t.Fatalf("tampered = %v", err)
	}
}

func TestKeyFingerprintMatchesGlorian(t *testing.T) {
	// substr(hash('sha256', 'sahte-anahtar-...'), 0, 12) on the Glorian side.
	if got := KeyFingerprint("sahte-anahtar-0123456789abcdefghijklmnopqrstuv"); got != "3c4bff14e533" {
		t.Fatalf("fingerprint = %q", got)
	}
}

func TestNewAPIKey(t *testing.T) {
	a, err := newAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newAPIKey()
	if len(a) != 43 || a == b {
		t.Fatalf("keys %q %q", a, b)
	}
}

func TestPackageInputValidate(t *testing.T) {
	yes, no := true, false
	zero, big, ttl := 0, 99999, 100
	good := PackageInput{BaseURL: "https://glorian.example.test/", Password: "uzun-bir-sahte-sifre"}
	set, err := good.validate()
	if err != nil {
		t.Fatal(err)
	}
	if set.baseURL != "https://glorian.example.test" || set.rateLimit != DefaultRateLimit || set.permissions.DealerWrite || !set.permissions.OrderStatusActions || len(set.ipAllowlist) != 0 {
		t.Fatalf("defaults = %+v", set)
	}
	withAll := good
	withAll.DealerWrite, withAll.OrderStatusActions = &yes, &no
	withAll.IPAllowlist = []string{"203.0.113.10", " 203.0.113.10 ", "2001:db8::1"}
	set, err = withAll.validate()
	if err != nil || !set.permissions.DealerWrite || set.permissions.OrderStatusActions || len(set.ipAllowlist) != 2 {
		t.Fatalf("options = %+v, %v", set, err)
	}

	cases := map[string]PackageInput{
		"password": {BaseURL: good.BaseURL, Password: "kisa"},
		"base_url": {BaseURL: "glorian", Password: good.Password},
		"rate":     {BaseURL: good.BaseURL, Password: good.Password, RateLimitPerMinute: &zero},
		"rate big": {BaseURL: good.BaseURL, Password: good.Password, RateLimitPerMinute: &big},
		"ip":       {BaseURL: good.BaseURL, Password: good.Password, IPAllowlist: []string{"999.0.0.1"}},
		"ttl":      {BaseURL: good.BaseURL, Password: good.Password, TTLHours: &ttl},
	}
	for name, in := range cases {
		var verr *ValidationError
		if _, err := in.validate(); !errors.As(err, &verr) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
