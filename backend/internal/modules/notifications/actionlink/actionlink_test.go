package actionlink

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSignVerify(t *testing.T) {
	t.Parallel()

	secret := []byte("unit-test-action-secret")
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	dest := "/v1/platform/exports/22222222-2222-2222-2222-222222222222/download"
	exp := time.Now().Add(time.Hour).Unix()
	sig := Sign(secret, id, dest, exp)

	if !Verify(secret, id, dest, sig, exp) {
		t.Fatal("expected valid signature")
	}
	if Verify(secret, id, dest+"/x", sig, exp) {
		t.Fatal("tampered destination must fail")
	}
	if Verify(secret, id, dest, sig, exp+60) {
		t.Fatal("tampered expiry must fail")
	}
	if Verify([]byte("other"), id, dest, sig, exp) {
		t.Fatal("wrong secret must fail")
	}
	if Verify(secret, id, dest, sig, time.Now().Add(-time.Minute).Unix()) {
		t.Fatal("expired link must fail")
	}
}

func TestURL(t *testing.T) {
	t.Parallel()

	secret := []byte("unit-test-action-secret")
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	got := URL(secret, id, "/platform/exports/1", time.Hour)
	if got == "" {
		t.Fatal("expected signed url")
	}
	if URL(nil, id, "/platform/exports/1", time.Hour) != "" {
		t.Fatal("empty secret must not sign")
	}
}
