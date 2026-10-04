package contracts

import (
	"context"
	"log/slog"

	contractsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// RegisterEventHandlers subscribes the executed-contract PDF renderer to the
// platform bus. The use case is idempotent through contract_instances.pdf_key.
func RegisterEventHandlers(bus events.Bus, svc *contractsusecase.Service, log *slog.Logger) {
	if bus == nil || svc == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	bus.Subscribe(events.ContractExecuted, func(ctx context.Context, event events.Event) error {
		if err := svc.ProcessExecutedEvent(ctx, event); err != nil {
			log.Error("contract_pdf_render_failed", "event_id", event.EventID, "error", err)
		}
		return nil
	})
}
