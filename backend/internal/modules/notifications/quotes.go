package notifications

import (
	"context"
	"errors"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// quoteSender is the part of the notification service quote.sent uses.
type quoteSender interface {
	Dispatch(ctx context.Context, in notifmodel.DispatchInput) (notifmodel.DispatchResult, error)
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

var _ quoteSender = (*notifusecase.Service)(nil)

// quoteSentNotify sends quote.sent. A lead linked to a customer user gets a
// catalog dispatch (TEC-1034: in-app row of the portal/mobile inbox plus
// WhatsApp, preferences and delivery log apply). When the quote went to
// another phone than the user's (the lead's candidate phone) or there is no
// user, WhatsApp goes straight to that phone as before.
func quoteSentNotify(ctx context.Context, svc quoteSender, event events.Event) error {
	phone := stringFromPayload(event.Payload, "recipient_phone")
	brandID, _ := int64FromPayload(event.Payload, "brand_id")
	recipientUserID, _ := int64FromPayload(event.Payload, "recipient_user_id")
	quoteURL := stringFromPayload(event.Payload, "quote_url")
	vars := map[string]string{
		"recipient_name":    stringFromPayload(event.Payload, "recipient_name"),
		"organization_name": stringFromPayload(event.Payload, "organization_name"),
		"quote_url":         quoteURL,
		"total_amount":      stringFromPayload(event.Payload, "total_amount"),
	}
	payload := map[string]any{
		"quote_uuid": stringFromPayload(event.Payload, "quote_uuid"),
		"quote_no":   stringFromPayload(event.Payload, "display_no"),
		"reminder":   boolFromPayload(event.Payload, "reminder"),
		"brand_id":   brandID,
	}
	userPhone := boolFromPayload(event.Payload, "recipient_phone_is_customer")
	var errs []error
	if recipientUserID > 0 {
		in := notifmodel.DispatchInput{
			EventID: event.EventID, EventCode: catalog.EventQuoteSent, OrganizationID: event.TenantID,
			UserIDs: []int64{recipientUserID}, Vars: vars, Payload: payload,
			ActionURL: stringPtr(quoteURL),
		}
		if brandID > 0 {
			in.BrandID = &brandID
		}
		if !userPhone && phone != "" {
			in.Channels = []string{notifmodel.ChannelInapp}
		}
		if _, err := svc.Dispatch(ctx, in); err != nil {
			errs = append(errs, err)
		}
		if userPhone || phone == "" {
			return errors.Join(errs...)
		}
	}
	if phone == "" {
		return nil
	}
	in := notifmodel.EnqueueInput{
		TenantID:     event.TenantID,
		Channels:     []string{notifmodel.ChannelWhatsApp},
		Priority:     notifmodel.PriorityNormal,
		Recipient:    &phone,
		TemplateCode: catalog.EventQuoteSent,
		SourceEvent:  catalog.EventQuoteSent,
		Language:     stringFromPayload(event.Payload, "language"),
		ActionURL:    stringPtr(quoteURL),
		TemplateVars: vars,
		Payload:      payload,
	}
	if recipientUserID > 0 {
		in.UserID = &recipientUserID
	}
	if _, err := svc.Enqueue(ctx, in); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func stringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
