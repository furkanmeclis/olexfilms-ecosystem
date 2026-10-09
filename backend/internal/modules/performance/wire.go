package performance

import (
	"context"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

func RegisterEventHandlers(bus events.Bus, svc *usecase.Service, log *slog.Logger) {
	if bus == nil || svc == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	bus.Subscribe(events.PerformanceComputed, func(ctx context.Context, event events.Event) error {
		brandID, ok := int64FromPayload(event.Payload, "brand_id")
		if !ok || brandID <= 0 {
			log.Warn("performance_computed_no_brand", "event_id", event.EventID.String())
			return nil
		}
		period, _ := event.Payload["period"].(string)
		if period == "" {
			period, _ = event.Payload["month"].(string)
		}
		ownerIDs := int64SliceFromPayload(event.Payload, "owner_org_ids")
		if len(ownerIDs) == 0 && event.TenantID != nil {
			ownerIDs = []int64{*event.TenantID}
		}
		if _, err := svc.RunRules(ctx, brandID, ownerIDs, period); err != nil {
			log.Error("performance_rule_run_failed", "event_id", event.EventID.String(), "error", err)
		}
		return nil
	})
}

func int64FromPayload(p map[string]any, key string) (int64, bool) {
	switch v := p[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		if v != float64(int64(v)) {
			return 0, false
		}
		return int64(v), true
	}
	return 0, false
}

func int64SliceFromPayload(p map[string]any, key string) []int64 {
	switch v := p[key].(type) {
	case []int64:
		return v
	case []int:
		out := make([]int64, 0, len(v))
		for _, n := range v {
			out = append(out, int64(n))
		}
		return out
	case []float64:
		out := make([]int64, 0, len(v))
		for _, n := range v {
			if n == float64(int64(n)) {
				out = append(out, int64(n))
			}
		}
		return out
	case []any:
		out := make([]int64, 0, len(v))
		for _, item := range v {
			switch n := item.(type) {
			case int64:
				out = append(out, n)
			case int:
				out = append(out, int64(n))
			case float64:
				if n == float64(int64(n)) {
					out = append(out, int64(n))
				}
			}
		}
		return out
	}
	return nil
}
