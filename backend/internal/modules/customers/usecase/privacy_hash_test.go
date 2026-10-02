package usecase

import "testing"

// A random unusable password must always satisfy the password policy; a bare
// base64 value without a digit used to fail anonymization with a 500.
func TestUnusablePasswordHashAlwaysValid(t *testing.T) {
	for i := 0; i < 200; i++ {
		if _, err := unusablePasswordHash(); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
}
