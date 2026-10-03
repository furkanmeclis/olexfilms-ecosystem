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
