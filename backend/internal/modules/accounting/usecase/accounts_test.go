package usecase

import (
	"errors"
	"testing"
)

func TestNormalizeIBAN(t *testing.T) {
	ok := map[string]string{
		"TR33 0006 1005 1978 6457 8413 26": "TR330006100519786457841326",
		"de89370400440532013000":           "DE89370400440532013000",
		"GB82-WEST-1234-5698-7654-32":      "GB82WEST12345698765432",
		"NL91ABNA0417164300":               "NL91ABNA0417164300",
	}
	for in, want := range ok {
		got, err := NormalizeIBAN(in)
		if err != nil || got != want {
			t.Errorf("NormalizeIBAN(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"",
		"TR330006100519786457841327", // check digits
		"TR33000610051978645784132",  // TR length
		"DE8937040044053201300",      // DE length
		"1234567890123456",
		"TR33 0006 1005 1978 6457 8413 2!",
	}
	for _, in := range bad {
		_, err := NormalizeIBAN(in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != "iban" {
			t.Errorf("NormalizeIBAN(%q) err = %v, want iban validation error", in, err)
		}
	}
}

func TestIBANOnlyOnBankAccounts(t *testing.T) {
	iban := "TR330006100519786457841326"
	if _, err := ibanArg(AccountCash, &iban); err == nil {
		t.Fatal("cash account accepted an IBAN")
	}
	if v, err := ibanArg(AccountBank, &iban); err != nil || v.String != iban {
		t.Fatalf("bank IBAN = %v, %v", v, err)
	}
	empty := ""
	if v, err := ibanArg(AccountCash, &empty); err != nil || v.Valid {
		t.Fatalf("empty IBAN = %v, %v", v, err)
	}
}
