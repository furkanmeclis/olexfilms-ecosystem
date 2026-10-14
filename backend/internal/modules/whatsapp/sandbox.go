package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	platwa "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// sandboxGateway swallows outbound messages to simulated numbers
// (WHATSAPP_SANDBOX_PREFIXES, TEC-640): cmd/wa-sim posts signed inbound
// webhooks from such numbers, and the replies must be recorded as sent
// without ever reaching wuzapi. Every other recipient goes to the wrapped
// gateway (the recipient allowlist included), so a sandbox number needs no
// allowlist entry.
type sandboxGateway struct {
	usecase.Gateway
	prefixes []string
	log      *slog.Logger
	seq      atomic.Uint64
}

// withSandbox wraps gw when prefixes is non-empty; otherwise gw is returned
// unchanged. Inbound media download stays exposed when gw has it.
func withSandbox(gw usecase.Gateway, prefixes []string, log *slog.Logger) usecase.Gateway {
	var ps []string
	for _, p := range prefixes {
		if d := digits(p); d != "" {
			ps = append(ps, d)
		}
	}
	if len(ps) == 0 {
		return gw
	}
	if log == nil {
		log = slog.Default()
	}
	log.Warn("whatsapp_sandbox_active", "prefixes", strings.Join(ps, ","))
	g := &sandboxGateway{Gateway: gw, prefixes: ps, log: log}
	if d, ok := gw.(platwa.MediaDownloader); ok {
		return &sandboxMediaGateway{sandboxGateway: g, downloader: d}
	}
	return g
}

type sandboxMediaGateway struct {
	*sandboxGateway
	downloader platwa.MediaDownloader
}

func (g *sandboxMediaGateway) DownloadMedia(ctx context.Context, m platwa.InboundMedia, max int64) ([]byte, error) {
	return g.downloader.DownloadMedia(ctx, m, max)
}

// isSandbox reports whether to starts with one of the sandbox prefixes.
func (g *sandboxGateway) isSandbox(to string) bool {
	d := digits(to)
	for _, p := range g.prefixes {
		if strings.HasPrefix(d, p) {
			return true
		}
	}
	return false
}

func (g *sandboxGateway) swallow(to, kind string, opts platwa.SendOptions) platwa.MsgRef {
	id := opts.ID
	if id == "" {
		id = fmt.Sprintf("SANDBOX%d%d", time.Now().UnixNano(), g.seq.Add(1))
	}
	g.log.Info("whatsapp_sandbox_send", "kind", kind, "to", phone.Mask(to), "id", id)
	return platwa.MsgRef{ID: id, Provider: "sandbox", Timestamp: time.Now()}
}

func (g *sandboxGateway) SendText(ctx context.Context, to, body string, opts platwa.SendOptions) (platwa.MsgRef, error) {
	if g.isSandbox(to) {
		return g.swallow(to, "text", opts), nil
	}
	return g.Gateway.SendText(ctx, to, body, opts)
}

func (g *sandboxGateway) SendDocument(ctx context.Context, to string, doc platwa.Media, opts platwa.SendOptions) (platwa.MsgRef, error) {
	if g.isSandbox(to) {
		return g.swallow(to, "document", opts), nil
	}
	return g.Gateway.SendDocument(ctx, to, doc, opts)
}

func (g *sandboxGateway) SendImage(ctx context.Context, to string, img platwa.Media, opts platwa.SendOptions) (platwa.MsgRef, error) {
	if g.isSandbox(to) {
		return g.swallow(to, "image", opts), nil
	}
	return g.Gateway.SendImage(ctx, to, img, opts)
}

func digits(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
