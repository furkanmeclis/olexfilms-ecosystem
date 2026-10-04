package notifications

import (
	"context"
	"log/slog"
	"strconv"

	announcementsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/announcements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
)

type eventHandlerConfig struct {
	announcementTargets announcementsusecase.TargetResolver
	announcementQueue   announcementsusecase.TaskEnqueuer
}

// EventHandlerOption customizes notification event listeners.
type EventHandlerOption func(*eventHandlerConfig)

// WithAnnouncementFanout wires announcement target resolution and batch task enqueueing.
func WithAnnouncementFanout(q announcementsusecase.TargetResolver, enq announcementsusecase.TaskEnqueuer) EventHandlerOption {
	return func(cfg *eventHandlerConfig) {
		cfg.announcementTargets = q
		cfg.announcementQueue = enq
	}
}

// RegisterEventHandlers attaches Notification Center listeners to the
// platform bus. Each outbox event maps to a catalog event; the outbox event
// id is the delivery idempotency key, so a redelivered event sends nothing
// twice. Titles and bodies come from notification_templates (event x role x
// channel x language); nothing is hard-coded here.
func RegisterEventHandlers(bus events.Bus, svc *notifusecase.Service, log *slog.Logger, opts ...EventHandlerOption) {
	if bus == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	var cfg eventHandlerConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	bus.Subscribe("customers.*", func(_ context.Context, event events.Event) error {
		log.Info(
			"notifications_bus_observed",
			"event", event.Name,
			"event_id", event.EventID.String(),
			"entity_type", event.EntityType,
			"customer_event_uuid", event.Payload["customer_event_uuid"],
		)
		return nil
	})
	if svc == nil {
		return
	}
	on := func(name string, build func(events.Event) (notifmodel.DispatchInput, bool)) {
		bus.Subscribe(name, func(ctx context.Context, event events.Event) error {
			in, ok := build(event)
			if !ok {
				return nil
			}
			in.EventID = event.EventID
			in.OrganizationID = event.TenantID
			if _, err := svc.Dispatch(ctx, in); err != nil {
				log.Error("notifications_dispatch_failed", "event", event.Name, "event_id", event.EventID, "error", err)
			}
			return nil // fail-soft: the bus must not block on a notification
		})
	}
	on(events.CustomersAssigned, func(event events.Event) (notifmodel.DispatchInput, bool) {
		userID, ok := int64FromPayload(event.Payload, "assigned_user_id")
		if !ok || userID <= 0 {
			return notifmodel.DispatchInput{}, false
		}
		return notifmodel.DispatchInput{
			EventCode: catalog.EventCustomersAssigned, UserIDs: []int64{userID},
			Vars: map[string]string{"customer_name": customerLabel(event)},
			Payload: map[string]any{
				"customer_event_uuid": event.Payload["customer_event_uuid"],
				"customer_uuid":       entityUUIDString(event.EntityUUID),
			},
		}, true
	})
	on(events.CustomersStatusChanged, func(event events.Event) (notifmodel.DispatchInput, bool) {
		to, _ := event.Payload["to"].(string)
		userID, ok := int64FromPayload(event.Payload, "assigned_user_id")
		if to != "blocked" || !ok || userID <= 0 {
			return notifmodel.DispatchInput{}, false
		}
		return notifmodel.DispatchInput{
			EventCode: catalog.EventCustomersStatusBlocked, UserIDs: []int64{userID},
			Priority: notifmodel.PriorityHigh,
			Vars:     map[string]string{"customer_name": customerLabel(event)},
			Payload: map[string]any{
				"customer_event_uuid": event.Payload["customer_event_uuid"],
				"customer_uuid":       entityUUIDString(event.EntityUUID),
				"to":                  to,
			},
		}, true
	})
	on(events.ConversationsAssigned, func(event events.Event) (notifmodel.DispatchInput, bool) {
		userID, ok := int64FromPayload(event.Payload, "assigned_user_id")
		if !ok || userID <= 0 {
			return notifmodel.DispatchInput{}, false // unassign
		}
		return notifmodel.DispatchInput{
			EventCode: catalog.EventConversationsAssigned, UserIDs: []int64{userID},
			Payload: map[string]any{"conversation_uuid": entityUUIDString(event.EntityUUID), "assigned_user_id": userID},
		}, true
	})
	on(events.AIPipelineEscalated, func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromAIEvent(event)
		return notifmodel.DispatchInput{
			EventCode: catalog.EventAIPipelineEscalated, UserIDs: ids, Priority: notifmodel.PriorityHigh,
			Vars: map[string]string{"intent": stringFromPayload(event.Payload, "intent")},
			Payload: map[string]any{
				"conversation_uuid": entityUUIDString(event.EntityUUID),
				"intent":            stringFromPayload(event.Payload, "intent"),
			},
		}, len(ids) > 0
	})
	// TEC-187: warranty cron events go to the warranty holder (WhatsApp +
	// in-app by default); the event id keeps a replay from sending twice.
	on(events.WarrantyExpiringSoon, func(event events.Event) (notifmodel.DispatchInput, bool) {
		days, ok := int64FromPayload(event.Payload, "days")
		if !ok || days <= 0 {
			return notifmodel.DispatchInput{}, false
		}
		return warrantyDispatch(event, catalog.EventWarrantyExpiringSoon, days)
	})
	on(events.WarrantyExpired, func(event events.Event) (notifmodel.DispatchInput, bool) {
		return warrantyDispatch(event, catalog.EventWarrantyExpired, 0)
	})
	on(events.WarrantyClaimStatusChanged, warrantyClaimDispatch)
	// TEC-192: the delayed review request goes to the service customer
	// (WhatsApp); the task writes the event once per service.
	on(events.ServiceReviewRequested, serviceReviewDispatch)
	// TEC-164: a new customer gets the WhatsApp welcome with the portal link.
	on(events.CustomerCreated, customerWelcomeDispatch)
	// TEC-200: sibling stock transfer events go to the notified sides
	// (notify_user_ids resolved by the transfer use case).
	for name, code := range TransferEventCodes {
		on(name, transferDispatcher(code))
	}
	// TEC-221: task assignment and due date reminders go to notify_user_ids
	// (the assignee, or the creator of an unassigned task).
	for name, code := range TaskEventCodes {
		on(name, taskDispatcher(code))
	}
	bus.Subscribe(events.AnnouncementPublished, func(ctx context.Context, event events.Event) error {
		err := announcementsusecase.EnqueuePublishedBatches(
			ctx,
			cfg.announcementTargets,
			cfg.announcementQueue,
			svc,
			event,
		)
		if err != nil {
			log.Error("announcement_notifications_enqueue_failed", "event_id", event.EventID, "error", err)
		}
		return nil
	})
	on(events.AIDraftCreated, func(event events.Event) (notifmodel.DispatchInput, bool) {
		ids := userIDsFromAIEvent(event)
		return notifmodel.DispatchInput{
			EventCode: catalog.EventAIDraftPending, UserIDs: ids,
			Payload: map[string]any{
				"conversation_uuid": entityUUIDString(event.EntityUUID),
				"draft_uuid":        stringFromPayload(event.Payload, "draft_uuid"),
			},
		}, len(ids) > 0
	})
}

// warrantyDispatch maps a warranty cron event to a dispatch for the holder;
// the brand comes from the warranty (K20), the variables from the payload.
func warrantyDispatch(event events.Event, code string, days int64) (notifmodel.DispatchInput, bool) {
	holder, ok := int64FromPayload(event.Payload, "holder_user_id")
	if !ok || holder <= 0 {
		return notifmodel.DispatchInput{}, false
	}
	vars := map[string]string{}
	for _, k := range []string{"plate", "product_name", "end_date", "organization_name", "verify_url"} {
		vars[k] = stringFromPayload(event.Payload, k)
	}
	payload := map[string]any{
		"warranty_uuid": stringFromPayload(event.Payload, "warranty_uuid"),
		"public_code":   stringFromPayload(event.Payload, "public_code"),
	}
	if days > 0 {
		vars["days"] = strconv.FormatInt(days, 10)
		payload["days"] = days
	}
	in := notifmodel.DispatchInput{
		EventCode: code, UserIDs: []int64{holder}, Vars: vars, Payload: payload,
	}
	if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
		in.BrandID = &brand
	}
	if u := vars["verify_url"]; u != "" {
		in.ActionURL = &u
	}
	return in, true
}

func warrantyClaimDispatch(event events.Event) (notifmodel.DispatchInput, bool) {
	action := stringFromPayload(event.Payload, "action")
	to := stringFromPayload(event.Payload, "to")
	code := catalog.EventWarrantyClaimStatusChanged
	if action == "opened" {
		code = catalog.EventWarrantyClaimOpened
	}
	if to == "approved" || to == "rejected" {
		code = catalog.EventWarrantyClaimResult
	}
	ids := userIDsFromPayload(event.Payload, "notify_user_ids")
	if len(ids) == 0 {
		return notifmodel.DispatchInput{}, false
	}
	vars := map[string]string{}
	for _, k := range []string{"claim_no", "from", "to", "organization_name", "product_name", "plate"} {
		vars[k] = stringFromPayload(event.Payload, k)
	}
	in := notifmodel.DispatchInput{
		EventCode: code, UserIDs: ids, Vars: vars,
		Payload: map[string]any{
			"claim_uuid":    stringFromPayload(event.Payload, "claim_uuid"),
			"warranty_uuid": stringFromPayload(event.Payload, "warranty_uuid"),
			"public_code":   stringFromPayload(event.Payload, "public_code"),
			"from":          vars["from"],
			"to":            vars["to"],
		},
	}
	if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
		in.BrandID = &brand
	}
	return in, true
}

// serviceReviewDispatch maps service.review_requested to a dispatch for the
// customer; the brand is the service brand (K20), review_url the dealer's
// google_business_url.
func serviceReviewDispatch(event events.Event) (notifmodel.DispatchInput, bool) {
	customer, ok := int64FromPayload(event.Payload, "customer_user_id")
	url := stringFromPayload(event.Payload, "review_url")
	if !ok || customer <= 0 || url == "" {
		return notifmodel.DispatchInput{}, false
	}
	vars := map[string]string{}
	for _, k := range []string{"organization_name", "review_url", "plate", "service_no"} {
		vars[k] = stringFromPayload(event.Payload, k)
	}
	in := notifmodel.DispatchInput{
		EventCode: catalog.EventServiceReviewRequest, UserIDs: []int64{customer}, Vars: vars,
		Payload: map[string]any{
			"service_uuid": stringFromPayload(event.Payload, "service_uuid"),
			"service_no":   vars["service_no"],
		},
		ActionURL: &url,
	}
	if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
		in.BrandID = &brand
	}
	return in, true
}

// customerWelcomeDispatch maps customer.created to the welcome message for
// the new customer; a customer without a phone (or an event without the
// portal link) sends nothing. The brand is the creating organization's
// (K20).
func customerWelcomeDispatch(event events.Event) (notifmodel.DispatchInput, bool) {
	customer, ok := int64FromPayload(event.Payload, "customer_user_id")
	url := stringFromPayload(event.Payload, "portal_url")
	if !ok || customer <= 0 || url == "" {
		return notifmodel.DispatchInput{}, false
	}
	if hasPhone, _ := event.Payload["has_phone"].(bool); !hasPhone {
		return notifmodel.DispatchInput{}, false
	}
	vars := map[string]string{}
	for _, k := range []string{"customer_name", "organization_name", "portal_url"} {
		vars[k] = stringFromPayload(event.Payload, k)
	}
	in := notifmodel.DispatchInput{
		EventCode: catalog.EventCustomerWelcome, UserIDs: []int64{customer}, Vars: vars,
		Payload:   map[string]any{"customer_uuid": stringFromPayload(event.Payload, "customer_uuid")},
		ActionURL: &url,
	}
	if brand, ok := int64FromPayload(event.Payload, "brand_id"); ok && brand > 0 {
		in.BrandID = &brand
	}
	return in, true
}

// userIDsFromAIEvent prefers assigned_user_id, else notify_user_ids slice in payload.
func userIDsFromAIEvent(event events.Event) []int64 {
	if id, ok := int64FromPayload(event.Payload, "assigned_user_id"); ok && id > 0 {
		return []int64{id}
	}
	raw, ok := event.Payload["notify_user_ids"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []int64:
		return v
	case []any:
		out := make([]int64, 0, len(v))
		for _, item := range v {
			switch n := item.(type) {
			case int64:
				out = append(out, n)
			case int:
				out = append(out, int64(n))
			case float64:
				out = append(out, int64(n))
			}
		}
		return out
	default:
		return nil
	}
}

func customerLabel(event events.Event) string {
	if name := stringFromPayload(event.Payload, "display_name"); name != "" {
		return name
	}
	if event.EntityUUID != nil {
		return event.EntityUUID.String()
	}
	return "Customer"
}

func entityUUIDString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func int64FromPayload(payload map[string]any, key string) (int64, bool) {
	if payload == nil {
		return 0, false
	}
	v, ok := payload[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func stringFromPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	v, _ := payload[key].(string)
	return v
}
