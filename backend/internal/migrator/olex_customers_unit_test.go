package migrator

import (
	"database/sql"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

func TestCustomersQueryIsReadOnly(t *testing.T) {
	if err := source.CheckReadOnly(customersQuery + " WHERE COALESCE(updated_at, created_at) > ? ORDER BY id"); err != nil {
		t.Fatal(err)
	}
}

// The legacy fixture phones (TEC-253) resolve as K26/K29 expect: the two
// pairs collapse to one E.164 each, the empty and the broken phone stay
// unverified.
func TestNormalizeCustomerPhones(t *testing.T) {
	cases := []struct {
		phone, country, e164 string
		unverified           bool
		note                 string
	}{
		{"05551112233", "TR", "+905551112233", false, ""},
		{"+90 555 111 22 33", "TR", "+905551112233", false, ""},
		{"(0532) 444 55 66", "TR", "+905324445566", false, ""},
		{"905324445566", "TR", "+905324445566", false, ""},
		{"", "TR", "", true, "phone_missing"},
		{"12345", "TR", "", true, "phone_unresolved"},
		{"5419876543", "TR", "+905419876543", false, ""},
		{"+49 151 23456789", "DE", "+4915123456789", false, ""},
		{"05551112233", "", "+905551112233", false, ""},
	}
	for _, c := range cases {
		d, notes := normalizeCustomer(legacyCustomer{Name: "Ad Soyad", Type: "individual", Phone: c.phone, Country: c.country})
		if d.Phone != c.e164 || d.Unverified != c.unverified {
			t.Errorf("%q/%s -> phone %q unverified %v, want %q %v", c.phone, c.country, d.Phone, d.Unverified, c.e164, c.unverified)
		}
		if c.note != "" && !slices.Contains(notes, c.note) {
			t.Errorf("%q: notes %v, want %s", c.phone, notes, c.note)
		}
		if c.unverified && d.PhoneRaw != c.phone {
			t.Errorf("%q: raw = %q", c.phone, d.PhoneRaw)
		}
	}
}

func TestNormalizeCustomerFields(t *testing.T) {
	d, notes := normalizeCustomer(legacyCustomer{
		Type: "corporate", Name: "  Sentetik Kurumsal A.Ş. ", TaxNo: "222 222-2222", TaxOff: "Sentetik VD",
		NationalID: "12", Email: " Kurumsal@Example.TEST ", Phone: "5419876543",
		DeletedAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	if d.Type != customerCorporate || d.Name != "Sentetik Kurumsal A.Ş." || d.Surname != "" || d.Company.String != "Sentetik Kurumsal A.Ş." {
		t.Errorf("corporate = %+v", d)
	}
	if d.TaxNo != "2222222222" || d.NationalID != "" || !slices.Contains(notes, "national_id_invalid") {
		t.Errorf("identity numbers = %q %q %v", d.TaxNo, d.NationalID, notes)
	}
	if d.Email != "kurumsal@example.test" || d.Status != "disabled" {
		t.Errorf("email/status = %q %q", d.Email, d.Status)
	}

	d, notes = normalizeCustomer(legacyCustomer{Type: "vip", Name: "Bir İki Üç"})
	if d.Type != customerIndividual || d.Name != "Bir İki" || d.Surname != "Üç" || !slices.Contains(notes, "type_unknown") {
		t.Errorf("individual = %+v %v", d, notes)
	}
}

func TestNotificationPrefs(t *testing.T) {
	var got map[string]bool
	if err := json.Unmarshal(notificationPrefs([]byte(`{"sms": true, "push": false, "fax": true, "email": "yes"}`)), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"whatsapp": true, "email": true, "sms": true, "push": false}
	if len(got) != len(want) {
		t.Fatalf("prefs = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("prefs[%s] = %v, want %v (%v)", k, got[k], v, got)
		}
	}
	if err := json.Unmarshal(notificationPrefs(nil), &got); err != nil || got["sms"] || !got["whatsapp"] {
		t.Errorf("default prefs = %v %v", got, err)
	}
}
