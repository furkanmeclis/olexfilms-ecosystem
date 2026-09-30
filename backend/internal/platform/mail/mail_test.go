package mail

import "testing"

func TestStripHeaderBreaks(t *testing.T) {
	got := stripHeaderBreaks("Hi Ali\r\nBcc: attacker@example.com\nX")
	if got != "Hi Ali Bcc: attacker@example.com X" {
		t.Fatalf("got %q", got)
	}
}
