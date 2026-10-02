package usecase

import (
	"testing"
	"time"
)

func TestEndDateInOrganizationZone(t *testing.T) {
	ist, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		t.Skip("tzdata missing")
	}
	// End of 31 March in Istanbul (23:59:59.999999 local) = 20:59:59 UTC.
	end := time.Date(2027, 3, 31, 23, 59, 59, 999999000, ist)
	if got := EndDate(end, "Europe/Istanbul"); got != "2027-03-31" {
		t.Fatalf("istanbul = %s", got)
	}
	// The next local midnight as the exclusive end gives the same day.
	if got := EndDate(time.Date(2027, 4, 1, 0, 0, 0, 0, ist), "Europe/Istanbul"); got != "2027-03-31" {
		t.Fatalf("midnight end = %s", got)
	}
	// An unknown zone falls back to UTC.
	if got := EndDate(end, "Mars/Base"); got != "2027-03-31" {
		t.Fatalf("fallback = %s", got)
	}
}

func TestVerifyURL(t *testing.T) {
	s := NewCron(nil, nil, nil, "https://olexfilms.app/")
	if got := s.VerifyURL("Ab3dEf6hIj9k"); got != "https://olexfilms.app/garanti/Ab3dEf6hIj9k" {
		t.Fatalf("verify url = %s", got)
	}
	if got := NewCron(nil, nil, nil, "").VerifyURL("x"); got != "" {
		t.Fatalf("no base url = %q", got)
	}
}
