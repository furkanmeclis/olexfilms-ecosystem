package notifications

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

func quoteSentNotify(ctx context.Context, svc *notifusecase.Service, event events.Event) error {
	phone := stringFromPayload(event.Payload, "recipient_phone")
	if phone == "" {
		return nil
	}
	orgID := event.TenantID
	brandID, _ := int64FromPayload(event.Payload, "brand_id")
	recipientUserID, _ := int64FromPayload(event.Payload, "recipient_user_id")
	in := notifmodel.EnqueueInput{
		TenantID:     orgID,
		Channels:     []string{notifmodel.ChannelWhatsApp},
		Priority:     notifmodel.PriorityNormal,
		Recipient:    &phone,
		TemplateCode: catalog.EventQuoteSent,
		SourceEvent:  catalog.EventQuoteSent,
		Language:     stringFromPayload(event.Payload, "language"),
		ActionURL:    stringPtr(stringFromPayload(event.Payload, "quote_url")),
		TemplateVars: map[string]string{
			"recipient_name":    stringFromPayload(event.Payload, "recipient_name"),
			"organization_name": stringFromPayload(event.Payload, "organization_name"),
			"quote_url":         stringFromPayload(event.Payload, "quote_url"),
			"total_amount":      stringFromPayload(event.Payload, "total_amount"),
		},
		Payload: map[string]any{
			"quote_uuid": stringFromPayload(event.Payload, "quote_uuid"),
			"quote_no":   stringFromPayload(event.Payload, "display_no"),
			"reminder":   boolFromPayload(event.Payload, "reminder"),
			"brand_id":   brandID,
		},
	}
	if recipientUserID > 0 {
		in.UserID = &recipientUserID
	}
	_, err := svc.Enqueue(ctx, in)
	return err
}

func stringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
