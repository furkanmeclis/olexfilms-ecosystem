package whatsapp

import (
	"errors"
	"testing"
)

func TestRecipientAllowlistEmptyAllowsEveryone(t *testing.T) {
	a := NewRecipientAllowlist(nil)
	if a.Active() {
		t.Fatal("empty list must be inactive")
	}
	if !a.Allows("+905551112233") || a.Check("+1 555 000") != nil {
		t.Fatal("inactive list must allow every recipient")
	}
}

func TestRecipientAllowlistMatchesByDigits(t *testing.T) {
	a := NewRecipientAllowlist([]string{" +90 555 111 22 33 ", "", "whatsapp:+4915123456789"})
	if !a.Active() || a.Len() != 2 {
		t.Fatalf("active=%v len=%d, want active with 2 numbers", a.Active(), a.Len())
	}
	for _, ok := range []string{"+905551112233", "905551112233", "90 555 111 22 33", "905551112233@s.whatsapp.net", "+4915123456789"} {
		if err := a.Check(ok); err != nil {
			t.Fatalf("%q should be allowed: %v", ok, err)
		}
	}
	for _, blocked := range []string{"+905551112234", "+905558800001", "", "abc"} {
		if err := a.Check(blocked); !errors.Is(err, ErrRecipientNotAllowed) {
			t.Fatalf("%q should be blocked, got %v", blocked, err)
		}
	}
}
