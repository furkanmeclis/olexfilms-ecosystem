package sysconfig

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		key, raw string
		want     string // canonical, "" = reject
	}{
		{KeyContractGraceDays, "0", "0"},
		{KeyContractGraceDays, "30", "30"},
		{KeyContractGraceDays, "-1", ""},
		{KeyContractGraceDays, "366", ""},
		{KeyContractGraceDays, "1.5", ""},
		{KeyContractGraceDays, `"7"`, ""},
		{KeyContractGraceDays, "true", ""},
		{KeyForecastMinDays, "0", ""},
		{KeyForecastMinDays, "90", "90"},
		{KeyPhotoStandardEnabled, "true", "true"},
		{KeyPhotoStandardEnabled, "1", ""},
		{KeyPhotoStandardEnabled, `"yes"`, ""},
		{KeySMTPPort, "587", "587"},
		{KeySMTPPort, "70000", ""},
		{KeySMTPHost, `"smtp.example.com"`, `"smtp.example.com"`},
		{KeySMTPHost, "12", ""},
		{KeySMTPHost, "", ""},
		{KeySMTPHost, "null", ""},
		{KeyScanShortCodeEnabled, "false", "false"},
		{KeyScanShortCodePrefix, `"OLEX"`, `"OLEX"`},
		{KeyScanShortCodePrefix, `"TOOLONGPFX"`, ""},
		{KeyMobileAppMinVersion, `""`, `""`},
		{KeyMobileAppMinVersion, `"2.4.0"`, `"2.4.0"`},
		{KeyMobileAppMinVersion, `" v3.1 "`, `"v3.1"`},
		{KeyMobileAppMinVersion, `"latest"`, ""},
		{KeyMobileAppMinVersion, "2", ""},
		{KeyMobileAppStoreURLIOS, `"https://apps.apple.com/app/id1"`, `"https://apps.apple.com/app/id1"`},
		{KeyMobileAppStoreURLIOS, `""`, `""`},
		{KeyMobileAppStoreURLIOS, `"http://apps.apple.com/app/id1"`, ""},
		{KeyMobileAppStoreURLAndroid, `"play.google.com/store"`, ""},
		{KeyMobileAppVersionRequired, "true", "true"},
	}
	for _, c := range cases {
		d, ok := Lookup(c.key)
		if !ok {
			t.Fatalf("%s missing from catalog", c.key)
		}
		got, err := d.Validate(json.RawMessage(c.raw))
		if c.want == "" {
			var ve *ValidationError
			if err == nil || !errors.As(err, &ve) {
				t.Errorf("%s %q: want ValidationError, got %v (%s)", c.key, c.raw, err, got)
			}
			continue
		}
		if err != nil || string(got) != c.want {
			t.Errorf("%s %q: got %s, %v; want %s", c.key, c.raw, got, err, c.want)
		}
	}
}

func TestCatalogDefaults(t *testing.T) {
	d, _ := Lookup(KeyContractGraceDays)
	if d.Default != int64(0) {
		t.Fatalf("contract_grace_days default = %v, want 0 (K23)", d.Default)
	}
	if d, _ := Lookup(KeyPhotoStandardEnabled); d.Default != false {
		t.Fatalf("photo_standard_enabled default = %v, want false", d.Default)
	}
	if d, _ := Lookup(KeySMTPPassword); !d.Secret {
		t.Fatal("smtp.password must be secret")
	}
	if _, ok := Lookup("nope"); ok {
		t.Fatal("unknown key found")
	}
	if n := len(Catalog()); n != len(catalog) {
		t.Fatalf("Catalog() len %d != %d", n, len(catalog))
	}
}

func TestMobileAppWithFallback(t *testing.T) {
	env := MobileApp{MinVersion: "2.0.0", StoreURLIOS: "https://ios.env", StoreURLAndroid: "https://android.env"}
	if got := (MobileApp{}).WithFallback(env); got != env {
		t.Fatalf("unset = %+v, want env %+v", got, env)
	}
	stored := MobileApp{MinVersion: "2.5.0", StoreURLAndroid: "https://android.db", VersionRequired: true}
	want := MobileApp{MinVersion: "2.5.0", StoreURLIOS: "https://ios.env", StoreURLAndroid: "https://android.db", VersionRequired: true}
	if got := stored.WithFallback(env); got != want {
		t.Fatalf("stored = %+v, want %+v", got, want)
	}
	if got := (MobileApp{}).WithFallback(MobileApp{VersionRequired: true}); !got.VersionRequired {
		t.Fatal("env VersionRequired lost")
	}
	for _, k := range []string{KeyMobileAppMinVersion, KeyMobileAppStoreURLIOS, KeyMobileAppStoreURLAndroid} {
		if d, _ := Lookup(k); d.Default != "" || d.Group != GroupMobile {
			t.Fatalf("%s default %v group %s, want empty (env fallback) in mobile", k, d.Default, d.Group)
		}
	}
}
