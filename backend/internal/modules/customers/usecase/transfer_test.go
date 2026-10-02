package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
)

func transferService() *Service {
	s := New(nil, nil, nil, nil)
	s.SetTransfers(nil, TransferConfig{Key: []byte("k")})
	return s
}

func TestTransferCodeHash(t *testing.T) {
	s := transferService()
	h1, err := s.hashTransferCode("from", "123456")
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := s.hashTransferCode("from", "123456")
	if h1 == h2 {
		t.Fatal("same code must hash differently (salt)")
	}
	if strings.Contains(h1, "123456") || len(h1) > 255 {
		t.Fatalf("hash = %q", h1)
	}
	if !s.checkTransferCode(h1, "from", "123456") || !s.checkTransferCode(h2, "from", "123456") {
		t.Fatal("right code refused")
	}
	if s.checkTransferCode(h1, "from", "123457") {
		t.Fatal("wrong code accepted")
	}
	if s.checkTransferCode(h1, "to", "123456") {
		t.Fatal("the other side's code accepted")
	}
	other := New(nil, nil, nil, nil)
	other.SetTransfers(nil, TransferConfig{Key: []byte("other")})
	if other.checkTransferCode(h1, "from", "123456") {
		t.Fatal("hash verified with another key")
	}
	if s.checkTransferCode("garbage", "from", "123456") {
		t.Fatal("malformed hash accepted")
	}
}

func TestGenerateTransferCode(t *testing.T) {
	for range 50 {
		c, err := generateTransferCode()
		if err != nil || !validTransferCode(c) {
			t.Fatalf("code %q (%v)", c, err)
		}
	}
	for _, bad := range []string{"", "12345", "1234567", "12a456", "１２３４５６"} {
		if validTransferCode(bad) {
			t.Fatalf("%q accepted", bad)
		}
	}
}

// Every supported locale has both texts with every placeholder (K10).
func TestTransferMessagesCoverEveryLocale(t *testing.T) {
	for _, l := range i18n.Supported {
		tpl, ok := transferMessages[l]
		if !ok {
			t.Errorf("%s: missing transfer messages", l)
			continue
		}
		for i, body := range tpl {
			for _, ph := range []string{"{app}", "{org}", "{vehicle}", "{code}", "{minutes}"} {
				if !strings.Contains(body, ph) {
					t.Errorf("%s[%d]: missing %s", l, i, ph)
				}
			}
		}
		if tpl[0] == tpl[1] {
			t.Errorf("%s: owner and new owner texts are the same", l)
		}
	}
	got := RenderTransferMessage("de-DE", "to", "Olexfilms", "Tech Oto", "34 ABC 123", "042517", 15)
	if !strings.Contains(got, "042517") || !strings.Contains(got, "34 ABC 123") || !strings.Contains(got, "Übernahme") {
		t.Fatalf("de message = %q", got)
	}
	if fb := RenderTransferMessage("xx", "from", "A", "B", "C", "111111", 15); !strings.Contains(fb, "devri") {
		t.Fatalf("fallback = %q", fb)
	}
}

func TestTransferGuards(t *testing.T) {
	ctx := context.Background()
	s := New(nil, nil, nil, nil)
	if _, err := s.StartTransfer(ctx, Caller{}, uuid.New(), StartTransferInput{Phone: "+905551112233"}, activity.Meta{}); !errors.Is(err, ErrTransferUnavailable) {
		t.Fatalf("start without sender = %v", err)
	}
	s = transferService()
	var ve *ValidationError
	if _, err := s.VerifyTransfer(ctx, Caller{}, uuid.New(), VerifyTransferInput{}, activity.Meta{}); !errors.As(err, &ve) {
		t.Fatalf("no code = %v", err)
	}
	if _, err := s.VerifyTransfer(ctx, Caller{}, uuid.New(), VerifyTransferInput{FromCode: "12"}, activity.Meta{}); !errors.As(err, &ve) || ve.Field != "from_code" {
		t.Fatalf("short code = %v", err)
	}
}
