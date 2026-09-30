package crypto

import (
	"encoding/base64"
	"testing"
)

func testKey(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func TestSecretBoxRoundTrip(t *testing.T) {
	box, err := NewSecretBox(testKey(t))
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}
	plain := "super-secret-client-secret"
	enc, err := box.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == plain {
		t.Fatal("expected ciphertext")
	}
	got, err := box.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("got %q want %q", got, plain)
	}
}

func TestSecretBoxEmpty(t *testing.T) {
	box, err := NewSecretBox(testKey(t))
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}
	enc, err := box.Encrypt("")
	if err != nil || enc != "" {
		t.Fatalf("empty encrypt: enc=%q err=%v", enc, err)
	}
	got, err := box.Decrypt("")
	if err != nil || got != "" {
		t.Fatalf("empty decrypt: got=%q err=%v", got, err)
	}
}
