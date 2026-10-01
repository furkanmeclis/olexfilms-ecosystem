package crypto

import (
	"bytes"
	"errors"
	"testing"
)

func TestPIIBoxRoundTrip(t *testing.T) {
	box, err := NewPIIBox(testKey(t))
	if err != nil {
		t.Fatalf("NewPIIBox: %v", err)
	}
	aad := PIIAAD("customer_profiles.national_id", 42)
	sealed, err := box.Seal("12345678901", aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, []byte("12345678901")) || sealed[0] != piiVersion {
		t.Fatalf("unexpected sealed form %x", sealed)
	}
	again, _ := box.Seal("12345678901", aad)
	if bytes.Equal(sealed, again) {
		t.Fatal("nonce must differ between seals")
	}
	got, err := box.Open(sealed, aad)
	if err != nil || got != "12345678901" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	if _, err := box.Open(sealed, PIIAAD("customer_profiles.national_id", 43)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("other row aad: err = %v", err)
	}
	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 1
	if _, err := box.Open(tampered, aad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("tampered: err = %v", err)
	}
}

func TestPIIBoxEmpty(t *testing.T) {
	box, err := NewPIIBox(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal("", nil)
	if err != nil || sealed != nil {
		t.Fatalf("empty seal = %x, %v", sealed, err)
	}
	got, err := box.Open(nil, nil)
	if err != nil || got != "" {
		t.Fatalf("empty open = %q, %v", got, err)
	}
}

func TestNewPIIBoxKey(t *testing.T) {
	if _, err := NewPIIBox(""); !errors.Is(err, ErrMissingPIIKey) {
		t.Fatalf("missing key: err = %v", err)
	}
	// A raw 32-character string is not accepted: the key must be base64.
	if _, err := NewPIIBox("app-dev-encryption-key-32bytes!!"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("raw key: err = %v", err)
	}
	if _, err := NewPIIBox("c2hvcnQ="); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("short key: err = %v", err)
	}
}

func TestLast4(t *testing.T) {
	cases := map[string]string{"12345678901": "8901", "12-3": "123", "": "", "de 123 456 ab": "56AB"}
	for in, want := range cases {
		if got := Last4(in); got != want {
			t.Fatalf("Last4(%q) = %q, want %q", in, got, want)
		}
	}
}
