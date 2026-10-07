package model

import "testing"

func TestValidTaxNumber(t *testing.T) {
	valid := []string{
		// VKN (10 digits).
		"1089325650", "9249799759", "6768656307", "1234567890",
		// TCKN (11 digits).
		"10000000146",
	}
	for _, s := range valid {
		if !ValidTaxNumber(s) {
			t.Errorf("%s must be valid", s)
		}
	}
	invalid := []string{
		"", "123", "1089325651", "9249799750", "6768656308", "0010000006",
		"10000000147", "00000000146", "108932565a", "108932565 ", "123456789012",
	}
	for _, s := range invalid {
		if ValidTaxNumber(s) {
			t.Errorf("%q must be invalid", s)
		}
	}
}
