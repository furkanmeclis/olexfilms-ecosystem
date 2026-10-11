package whatsapp

import (
	"context"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	platwa "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// allowlistGateway refuses outbound messages to numbers outside the
// configured recipient allowlist (WUZAPI_ALLOWED_RECIPIENTS). Every sender
// (OTP, notifications, campaigns, conversations, vehicle transfer) goes
// through this gateway, so the guard is a single choke point.
type allowlistGateway struct {
	usecase.Gateway
	list platwa.RecipientAllowlist
	log  *slog.Logger
}

// withRecipientAllowlist wraps gw when numbers is non-empty; otherwise gw is
// returned unchanged.
func withRecipientAllowlist(gw usecase.Gateway, numbers []string, log *slog.Logger) usecase.Gateway {
	list := platwa.NewRecipientAllowlist(numbers)
	if !list.Active() {
		return gw
	}
	if log == nil {
		log = slog.Default()
	}
	log.Warn("whatsapp_recipient_allowlist_active", "count", list.Len())
	return &allowlistGateway{Gateway: gw, list: list, log: log}
}

func (g *allowlistGateway) check(to, kind string) error {
	if err := g.list.Check(to); err != nil {
		g.log.Warn("whatsapp_recipient_blocked", "kind", kind, "to", phone.Mask(to))
		return err
	}
	return nil
}

func (g *allowlistGateway) SendText(ctx context.Context, to, body string, opts platwa.SendOptions) (platwa.MsgRef, error) {
	if err := g.check(to, "text"); err != nil {
		return platwa.MsgRef{}, err
	}
	return g.Gateway.SendText(ctx, to, body, opts)
}

func (g *allowlistGateway) SendDocument(ctx context.Context, to string, doc platwa.Media, opts platwa.SendOptions) (platwa.MsgRef, error) {
	if err := g.check(to, "document"); err != nil {
		return platwa.MsgRef{}, err
	}
	return g.Gateway.SendDocument(ctx, to, doc, opts)
}

func (g *allowlistGateway) SendImage(ctx context.Context, to string, img platwa.Media, opts platwa.SendOptions) (platwa.MsgRef, error) {
	if err := g.check(to, "image"); err != nil {
		return platwa.MsgRef{}, err
	}
	return g.Gateway.SendImage(ctx, to, img, opts)
}
