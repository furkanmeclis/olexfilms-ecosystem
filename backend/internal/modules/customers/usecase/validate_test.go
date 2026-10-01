package usecase

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestNormalizeVIN(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"", "", true},
		{"wvwzzz1jzxw000001", "WVWZZZ1JZXW000001", true},
		{"WVW ZZZ1JZ-XW000001", "WVWZZZ1JZXW000001", true},
		{"WVWZZZ1JZXW00000", "", false},   // 16
		{"WVWZZZ1JZXW0000011", "", false}, // 18
		{"WVWZZZ1JZXW00000I", "", false},  // I is not allowed
		{"WVWZZZ1JZXW00000O", "", false},
		{"WVWZZZ1JZXW00000Q", "", false},
		{"WVWZZZ1JZXW00000*", "", false},
	}
	for _, c := range cases {
		got, err := NormalizeVIN(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("NormalizeVIN(%q) = %q, %v; want %q ok=%v", c.in, got, err, c.want, c.ok)
		}
		var ve *ValidationError
		if err != nil && (!errors.As(err, &ve) || ve.Field != "vin") {
			t.Errorf("NormalizeVIN(%q) error %v is not a vin validation error", c.in, err)
		}
	}
}

func TestNormalizeIdentityNumber(t *testing.T) {
	if v, err := normalizeIdentityNumber(" 123 456-789.01 ", "national_id"); err != nil || v != "12345678901" {
		t.Fatalf("got %q %v", v, err)
	}
	if v, err := normalizeIdentityNumber("", "tax_no"); err != nil || v != "" {
		t.Fatalf("empty: %q %v", v, err)
	}
	for _, bad := range []string{"123", "12345678901234567890123", "1234*5678"} {
		if _, err := normalizeIdentityNumber(bad, "tax_no"); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if v, err := normalizeEmail("  Ali@Example.COM "); err != nil || v != "ali@example.com" {
		t.Fatalf("got %q %v", v, err)
	}
	for _, bad := range []string{"ali", "ali@", "Ali <ali@example.com>", "ali@localhost"} {
		if _, err := normalizeEmail(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestMergeNotificationPrefs(t *testing.T) {
	b, err := mergeNotificationPrefs([]byte(`{"whatsapp":true,"email":true,"sms":false,"push":true}`), map[string]bool{"sms": true, "email": false})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]bool
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !got["sms"] || got["email"] || !got["whatsapp"] || !got["push"] {
		t.Fatalf("merged = %v", got)
	}
	if _, err := mergeNotificationPrefs(nil, map[string]bool{"fax": true}); err == nil {
		t.Fatal("unknown channel must be refused")
	}
}

func TestNormalizeAddress(t *testing.T) {
	if b, err := normalizeAddress(nil); err != nil || string(b) != "{}" {
		t.Fatalf("nil: %s %v", b, err)
	}
	if _, err := normalizeAddress(json.RawMessage(`["a"]`)); err == nil {
		t.Fatal("array must be refused")
	}
	if _, err := normalizeAddress(json.RawMessage(`"x"`)); err == nil {
		t.Fatal("string must be refused")
	}
	if b, err := normalizeAddress(json.RawMessage(`{"line1":"A","district_id":3}`)); err != nil || !jsonEqual(b, []byte(`{"district_id":3,"line1":"A"}`)) {
		t.Fatalf("object: %s %v", b, err)
	}
}

func TestValidateModelYear(t *testing.T) {
	for _, y := range []int{1900, 2026, 2027} {
		if err := validateModelYear(y, 2026); err != nil {
			t.Errorf("%d: %v", y, err)
		}
	}
	for _, y := range []int{1899, 2028} {
		if err := validateModelYear(y, 2026); err == nil {
			t.Errorf("%d must be refused", y)
		}
	}
}

func TestOptionalUnmarshal(t *testing.T) {
	var in UpdateCustomerInput
	if err := json.Unmarshal([]byte(`{"name":"Ayşe","email":null}`), &in); err != nil {
		t.Fatal(err)
	}
	if !in.Name.Set || in.Name.Value == nil || *in.Name.Value != "Ayşe" {
		t.Fatalf("name = %+v", in.Name)
	}
	if !in.Email.Set || in.Email.Value != nil {
		t.Fatalf("email null = %+v", in.Email)
	}
	if in.Surname.Set || in.TaxNo.Set {
		t.Fatal("absent keys must stay unset")
	}
}

// A shared customer's stored name is never overwritten; differing values
// are reported, empty ones filled.
func TestFillNames(t *testing.T) {
	u := db.User{Name: "Ali", Surname: ""}
	ignored, n, sn := fillNames(u, "Veli", "Yılmaz", nil)
	if n.Valid {
		t.Fatal("stored name must not be replaced")
	}
	if !sn.Valid || sn.String != "Yılmaz" {
		t.Fatalf("empty surname must be filled, got %+v", sn)
	}
	if len(ignored) != 1 || ignored[0] != "name" {
		t.Fatalf("ignored = %v", ignored)
	}
	if ignored, _, _ := fillNames(u, "Ali", "", nil); len(ignored) != 0 {
		t.Fatalf("same name must not be reported: %v", ignored)
	}
}

func TestWritableUser(t *testing.T) {
	if err := writableUser(db.User{Status: StatusActive}); err != nil {
		t.Fatal(err)
	}
	if err := writableUser(db.User{Status: StatusAnonymized}); !errors.Is(err, ErrAnonymized) {
		t.Fatalf("anonymized: %v", err)
	}
	if err := writableUser(db.User{Status: "disabled"}); !errors.Is(err, ErrInactive) {
		t.Fatalf("disabled: %v", err)
	}
	merged := db.User{Status: StatusActive, MergedIntoUserID: pgtype.Int8{Int64: 9, Valid: true}}
	if err := writableUser(merged); !errors.Is(err, ErrInactive) {
		t.Fatalf("merged: %v", err)
	}
}

func TestMaskSummary(t *testing.T) {
	email, ph, company := "a@b.co", "+905551112233", "ACME"
	s := CustomerSummary{Name: "Ali", Surname: "Veli", Email: &email, Phone: &ph, CompanyName: &company, Status: StatusAnonymized}
	m := maskSummary(s, i18n.LocaleTR)
	if !m.Anonymized || m.Name != i18n.Translate(i18n.LocaleTR, AnonymizedNameKey) || m.Name == AnonymizedNameKey {
		t.Fatalf("masked name = %q", m.Name)
	}
	if m.Surname != "" || m.Email != nil || m.Phone != nil || m.CompanyName != nil {
		t.Fatalf("personal data left: %+v", m)
	}
	s.Status = StatusActive
	if got := maskSummary(s, i18n.LocaleTR); got.Anonymized || got.Name != "Ali" {
		t.Fatalf("active customer must not be masked: %+v", got)
	}
}
