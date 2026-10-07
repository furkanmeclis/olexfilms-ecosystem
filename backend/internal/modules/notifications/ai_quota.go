package notifications

import (
	"context"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// AIQuotaRecipients resolves the recipients and the organization name of
// ai.quota.threshold (TEC-389; *db.Queries).
type AIQuotaRecipients interface {
	ListAIQuotaNotifyUserIDs(ctx context.Context, arg db.ListAIQuotaNotifyUserIDsParams) ([]int64, error)
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
}

// WithAIQuotaRecipients enables the AI quota threshold notification.
func WithAIQuotaRecipients(q AIQuotaRecipients) EventHandlerOption {
	return func(cfg *eventHandlerConfig) { cfg.aiQuotaRecipients = q }
}

// aiQuotaThresholdDispatch maps ai.quota.threshold to the catalog event for
// the organization's ai.usage.read holders (org pool) or the platform
// admins (system pool). The event id is fixed per organization, pool,
// month and threshold (ai/usecase.ThresholdEventID), so the delivery
// idempotency key keeps a threshold to one notification a month. No
// recipient sends nothing.
func aiQuotaThresholdDispatch(ctx context.Context, q AIQuotaRecipients, event events.Event) (notifmodel.DispatchInput, bool, error) {
	orgID, ok := int64FromPayload(event.Payload, "organization_id")
	pool := stringFromPayload(event.Payload, "pool")
	if !ok || orgID <= 0 || (pool != "org" && pool != "system") {
		return notifmodel.DispatchInput{}, false, nil
	}
	ids, err := q.ListAIQuotaNotifyUserIDs(ctx, db.ListAIQuotaNotifyUserIDsParams{Pool: pool, OrganizationID: orgID})
	if err != nil || len(ids) == 0 {
		return notifmodel.DispatchInput{}, false, err
	}
	org, err := q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return notifmodel.DispatchInput{}, false, err
	}
	num := func(key string) string {
		n, _ := int64FromPayload(event.Payload, key)
		return strconv.FormatInt(n, 10)
	}
	in := notifmodel.DispatchInput{
		EventCode: catalog.EventAIQuotaThreshold, UserIDs: ids,
		Vars: map[string]string{
			"organization_name": org.Name, "threshold": num("threshold"), "period": stringFromPayload(event.Payload, "period"),
			"used_tokens": num("used_tokens"), "quota_tokens": num("quota_tokens"),
		},
		Payload: map[string]any{
			"organization_uuid": org.Uuid.String(), "pool": pool, "period": stringFromPayload(event.Payload, "period"),
			"threshold": event.Payload["threshold"],
		},
	}
	if num("threshold") == "100" {
		in.Priority = notifmodel.PriorityHigh
	}
	brand := org.BrandID
	in.BrandID = &brand
	return in, true, nil
}
