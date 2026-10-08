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
		{KeyForecastDefaultWarningDays, "14", "14"},
		{KeyForecastCriticalDays, "7", "7"},
		{KeyForecastDefaultCoverDays, "30", "30"},
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
		{KeyCertificatesRequireAdminApproval, "false", "false"},
		{KeyCertificatesExpiryNoticeDays, "30", "30"},
		{KeyCertificatesExpiryNoticeDays, "0", ""},
		{KeyCertificatesExpiryNoticeDays, "366", ""},
		{KeyEfficiencyNetworkWindowDays, "180", "180"},
		{KeyEfficiencyNetworkMinSamples, "20", "20"},
		{KeyEfficiencyWarningWasteRatio, `"0.15"`, `"0.15"`},
		{KeyEfficiencyWarningWasteRatio, `"1.2345"`, `"1.2345"`},
		{KeyEfficiencyWarningWasteRatio, `"-0.1"`, ""},
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
	if d, _ := Lookup(KeyForecastMinDays); d.Default != int64(90) {
		t.Fatalf("forecast_min_days default = %v, want 90", d.Default)
	}
	if d, _ := Lookup(KeyForecastDefaultWarningDays); d.Default != int64(14) {
		t.Fatalf("forecast.default_warning_days default = %v, want 14", d.Default)
	}
	if d, _ := Lookup(KeyForecastCriticalDays); d.Default != int64(7) {
		t.Fatalf("forecast.critical_days default = %v, want 7", d.Default)
	}
	if d, _ := Lookup(KeyForecastDefaultCoverDays); d.Default != int64(30) {
		t.Fatalf("forecast.default_cover_days default = %v, want 30", d.Default)
	}
	if d, _ := Lookup(KeySMTPPassword); !d.Secret {
		t.Fatal("smtp.password must be secret")
	}
	if d, _ := Lookup(KeyCertificatesRequireAdminApproval); d.Default != false || d.Group != GroupCertificates {
		t.Fatalf("certificate approval default/group = %v/%s, want false/certificates", d.Default, d.Group)
	}
	if d, _ := Lookup(KeyCertificatesExpiryNoticeDays); d.Default != int64(DefaultCertificatesExpiryNoticeDays) {
		t.Fatalf("certificate notice default = %v, want %d", d.Default, DefaultCertificatesExpiryNoticeDays)
	}
	if d, _ := Lookup(KeyEfficiencyNetworkWindowDays); d.Default != int64(DefaultEfficiencyNetworkWindowDays) || d.Group != GroupEfficiency {
		t.Fatalf("efficiency network window = %v/%s", d.Default, d.Group)
	}
	if d, _ := Lookup(KeyEfficiencyNetworkMinSamples); d.Default != int64(DefaultEfficiencyNetworkMinSamples) || d.Group != GroupEfficiency {
		t.Fatalf("efficiency min samples = %v/%s", d.Default, d.Group)
	}
	if d, _ := Lookup(KeyEfficiencyWarningWasteRatio); d.Default != DefaultEfficiencyWarningWasteRatio || d.Group != GroupEfficiency {
		t.Fatalf("efficiency warning ratio = %v/%s", d.Default, d.Group)
	}
	if _, ok := Lookup("certificates.notify_customer"); ok {
		t.Fatal("certificates.notify_customer must not exist per F5 S13")
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

// TEC-337: the warranty labor rule keys validate their values.
func TestWarrantyLaborSettings(t *testing.T) {
	cases := []struct {
		key, raw string
		ok       bool
	}{
		{KeyWarrantyClaimsLaborRule, `"dealer"`, true},
		{KeyWarrantyClaimsLaborRule, `"center"`, true},
		{KeyWarrantyClaimsLaborRule, `"shared"`, true},
		{KeyWarrantyClaimsLaborRule, `"nobody"`, false},
		{KeyWarrantyClaimsLaborAmount, `"150.00"`, true},
		{KeyWarrantyClaimsLaborAmount, `"150"`, true},
		{KeyWarrantyClaimsLaborAmount, `"-1"`, false},
		{KeyWarrantyClaimsLaborAmount, `"1.234"`, false},
		{KeyWarrantyClaimsLaborSharePercent, `50`, true},
		{KeyWarrantyClaimsLaborSharePercent, `101`, false},
	}
	for _, c := range cases {
		d, ok := Lookup(c.key)
		if !ok {
			t.Fatalf("%s missing", c.key)
		}
		if _, err := d.Validate([]byte(c.raw)); (err == nil) != c.ok {
			t.Errorf("%s %s: err = %v, want ok=%v", c.key, c.raw, err, c.ok)
		}
	}
	if d, _ := Lookup(KeyWarrantyClaimsLaborRule); d.Default != LaborRuleDealer {
		t.Errorf("labor rule default = %v, want dealer", d.Default)
	}
}

// TEC-466: showcase approval is off by default (F5 S4) and the gallery holds
// 12 photos unless the admin changes it.
func TestShowcaseSettings(t *testing.T) {
	d, ok := Lookup(KeyShowcaseApprovalRequired)
	if !ok || d.Default != false || d.Group != GroupShowcase || d.Kind != KindBool {
		t.Fatalf("showcase.approval_required = %+v", d)
	}
	d, ok = Lookup(KeyShowcaseMaxPhotos)
	if !ok || d.Default != int64(12) || d.Group != GroupShowcase {
		t.Fatalf("showcase.max_photos = %+v", d)
	}
	for raw, valid := range map[string]bool{`12`: true, `1`: true, `50`: true, `0`: false, `51`: false} {
		if _, err := d.Validate([]byte(raw)); (err == nil) != valid {
			t.Errorf("max_photos %s: err = %v, want ok=%v", raw, err, valid)
		}
	}
}
