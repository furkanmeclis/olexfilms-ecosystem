package whatsapp

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	platwa "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

func TestWithSandboxEmptyReturnsGatewayUnchanged(t *testing.T) {
	stub := &stubGateway{}
	if got := withSandbox(stub, []string{"", " "}, nil); got != usecase.Gateway(stub) {
		t.Fatal("empty sandbox prefixes must not wrap the gateway")
	}
}

// TEC-640: sandbox numbers never reach the provider (nor the allowlist
// behind it) and count as sent; every other number goes through unchanged.
func TestSandboxGatewaySwallowsSandboxNumbers(t *testing.T) {
	ctx := context.Background()
	stub := &stubGateway{}
	gw := withSandbox(withRecipientAllowlist(stub, []string{"+905551112233"}, nil), []string{"+90 500 000"}, nil)

	for name, call := range map[string]func() (platwa.MsgRef, error){
		"text": func() (platwa.MsgRef, error) {
			return gw.SendText(ctx, "+905000001234", "hi", platwa.SendOptions{ID: "C1"})
		},
		"doc": func() (platwa.MsgRef, error) {
			return gw.SendDocument(ctx, "905000001234", platwa.Media{}, platwa.SendOptions{})
		},
		"img": func() (platwa.MsgRef, error) {
			return gw.SendImage(ctx, "+90 500 000 12 34", platwa.Media{}, platwa.SendOptions{})
		},
	} {
		ref, err := call()
		if err != nil {
			t.Fatalf("%s to a sandbox number: %v", name, err)
		}
		if ref.ID == "" || ref.Provider != "sandbox" || ref.Timestamp.IsZero() {
			t.Fatalf("%s ref = %+v", name, ref)
		}
		if name == "text" && ref.ID != "C1" {
			t.Fatalf("client message id not kept: %q", ref.ID)
		}
	}
	if len(stub.sent) != 0 {
		t.Fatalf("provider received %v for sandbox numbers", stub.sent)
	}

	if _, err := gw.SendText(ctx, "+905551112233", "hi", platwa.SendOptions{}); err != nil {
		t.Fatalf("allowed real number: %v", err)
	}
	if _, err := gw.SendText(ctx, "+905008800001", "hi", platwa.SendOptions{}); !errors.Is(err, platwa.ErrRecipientNotAllowed) {
		t.Fatalf("non-sandbox number outside the allowlist: err = %v", err)
	}
	if len(stub.sent) != 1 || stub.sent[0] != "text:+905551112233" {
		t.Fatalf("provider received %v", stub.sent)
	}
}

func TestSandboxGatewayKeepsMediaDownloader(t *testing.T) {
	inner := &downloaderGateway{}
	if _, ok := withSandbox(inner, []string{"90500000"}, nil).(platwa.MediaDownloader); !ok {
		t.Fatal("wrapped downloader gateway must stay a MediaDownloader")
	}
	if _, ok := withSandbox(&stubGateway{}, []string{"90500000"}, nil).(platwa.MediaDownloader); ok {
		t.Fatal("a gateway without media download must not gain it through the wrapper")
	}
}
