package jwt

import (
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestMobileKindClaim(t *testing.T) {
	m, err := NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		aud, kind, want, claim string
	}{
		{AudienceMobile, MobileKindCustomer, MobileKindCustomer, MobileKindCustomer},
		{AudienceMobile, "", MobileKindStaff, ""},
		{AudienceMobile, MobileKindStaff, MobileKindStaff, ""},
		{AudiencePortal, MobileKindCustomer, "", ""},
		{AudiencePanel, MobileKindCustomer, "", ""},
	} {
		tok, _, err := m.IssueAccess(AccessInput{UserID: uuid.New(), Audience: tc.aud, MobileKind: tc.kind})
		if err != nil {
			t.Fatal(err)
		}
		c, err := m.ParseAccess(tok)
		if err != nil {
			t.Fatal(err)
		}
		if c.SessionKind() != tc.want || c.MobileKind != tc.claim {
			t.Errorf("%s/%s: kind=%q claim=%q", tc.aud, tc.kind, c.SessionKind(), c.MobileKind)
		}
	}
	// A legacy token never carries the customer kind.
	tok, _, err := m.IssueLegacyAccess(AccessInput{UserID: uuid.New(), SessionID: uuid.New(), MobileKind: MobileKindCustomer}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if c, err := m.AcceptLegacy().ParseAccess(tok); err != nil || c.MobileKind != "" {
		t.Fatalf("legacy kind=%q err=%v", c.MobileKind, err)
	}
	// A signed token whose kind does not fit its audience is refused.
	bad := Claims{MobileKind: MobileKindCustomer, RegisteredClaims: jwtlib.RegisteredClaims{
		Subject: uuid.NewString(), Audience: jwtlib.ClaimStrings{AudiencePortal},
		ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Minute)),
	}}
	signed, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, bad).SignedString(m.accessSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ParseAccess(signed); err != ErrInvalidToken {
		t.Fatalf("portal token with mobile kind: %v", err)
	}
}
