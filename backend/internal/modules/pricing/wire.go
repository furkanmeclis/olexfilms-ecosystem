package pricing

import (
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// RegisterEventHandlers subscribes the price list PDF publication to
// pricing.recommended_published (TEC-506): each country / currency of the
// batch is queued on the docs queue (when the auto publication is on).
func RegisterEventHandlers(bus events.Bus, svc *pricingusecase.Recommended) {
	if bus == nil || svc == nil {
		return
	}
	bus.Subscribe(events.PricingRecommendedPublished, svc.EnqueuePublished)
}
