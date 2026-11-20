package usecase

import (
	"errors"
	"testing"
)

// TEC-1259: organization names reject markup (public page JSON-LD XSS).
func TestCheckOrgDisplayText(t *testing.T) {
	for _, v := range []string{"Olex Kadıköy", "A & B Oto Kaplama", "Müller's \"Garage\""} {
		if err := checkOrgDisplayText("name", v); err != nil {
			t.Errorf("%q rejected: %v", v, err)
		}
	}
	for _, v := range []string{"Bayi</script><script>alert(1)</script>", "<b>Bayi</b>", "Bayi >"} {
		if err := checkOrgDisplayText("name", v); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%q accepted: %v", v, err)
		}
	}
}
