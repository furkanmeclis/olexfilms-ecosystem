package whatsapp

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	platwa "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// stubGateway records sends; the embedded nil Gateway covers the methods the
// allowlist never touches.
type stubGateway struct {
	usecase.Gateway
	sent []string
}

func (s *stubGateway) SendText(_ context.Context, to, _ string, _ platwa.SendOptions) (platwa.MsgRef, error) {
	s.sent = append(s.sent, "text:"+to)
	return platwa.MsgRef{ID: "ok"}, nil
}

func (s *stubGateway) SendDocument(_ context.Context, to string, _ platwa.Media, _ platwa.SendOptions) (platwa.MsgRef, error) {
	s.sent = append(s.sent, "doc:"+to)
	return platwa.MsgRef{ID: "ok"}, nil
}

func (s *stubGateway) SendImage(_ context.Context, to string, _ platwa.Media, _ platwa.SendOptions) (platwa.MsgRef, error) {
	s.sent = append(s.sent, "img:"+to)
	return platwa.MsgRef{ID: "ok"}, nil
}

func TestWithRecipientAllowlistEmptyReturnsGatewayUnchanged(t *testing.T) {
	stub := &stubGateway{}
	if got := withRecipientAllowlist(stub, nil, nil); got != usecase.Gateway(stub) {
		t.Fatal("empty allowlist must not wrap the gateway")
	}
}

func TestAllowlistGatewayBlocksOtherRecipients(t *testing.T) {
	stub := &stubGateway{}
	gw := withRecipientAllowlist(stub, []string{"+90 555 111 22 33"}, nil)
	ctx := context.Background()
	if _, err := gw.SendText(ctx, "+905551112233", "hi", platwa.SendOptions{}); err != nil {
		t.Fatalf("allowed text: %v", err)
	}
	if _, err := gw.SendImage(ctx, "905551112233", platwa.Media{}, platwa.SendOptions{}); err != nil {
		t.Fatalf("allowed image: %v", err)
	}
	for name, call := range map[string]func() error{
		"text": func() error { _, err := gw.SendText(ctx, "+905558800001", "hi", platwa.SendOptions{}); return err },
		"doc": func() error {
			_, err := gw.SendDocument(ctx, "+905558800001", platwa.Media{}, platwa.SendOptions{})
			return err
		},
		"img": func() error {
			_, err := gw.SendImage(ctx, "+905558800001", platwa.Media{}, platwa.SendOptions{})
			return err
		},
	} {
		if err := call(); !errors.Is(err, platwa.ErrRecipientNotAllowed) {
			t.Fatalf("%s to a seed number: err = %v, want ErrRecipientNotAllowed", name, err)
		}
	}
	if len(stub.sent) != 2 {
		t.Fatalf("provider received %v, want only the two allowed sends", stub.sent)
	}
}

// downloaderGateway is a stub gateway that can also download inbound media.
type downloaderGateway struct {
	stubGateway
	downloads int
}

func (d *downloaderGateway) DownloadMedia(_ context.Context, _ platwa.InboundMedia, _ int64) ([]byte, error) {
	d.downloads++
	return []byte("media"), nil
}

// TEC-634: the wrapper keeps media download support exactly when the wrapped
// gateway has it, and still blocks sends outside the list.
func TestAllowlistGatewayKeepsMediaDownloader(t *testing.T) {
	ctx := context.Background()
	inner := &downloaderGateway{}
	gw := withRecipientAllowlist(inner, []string{"+905551112233"}, nil)
	d, ok := gw.(platwa.MediaDownloader)
	if !ok {
		t.Fatal("wrapped downloader gateway must stay a MediaDownloader")
	}
	if data, err := d.DownloadMedia(ctx, platwa.InboundMedia{}, 10); err != nil || string(data) != "media" || inner.downloads != 1 {
		t.Fatalf("download = %q, %v (calls %d)", data, err, inner.downloads)
	}
	if _, err := gw.SendText(ctx, "+905558800001", "hi", platwa.SendOptions{}); !errors.Is(err, platwa.ErrRecipientNotAllowed) {
		t.Fatalf("blocked send err = %v", err)
	}

	plain := withRecipientAllowlist(&stubGateway{}, []string{"+905551112233"}, nil)
	if _, ok := plain.(platwa.MediaDownloader); ok {
		t.Fatal("a gateway without media download must not gain it through the wrapper")
	}
}
