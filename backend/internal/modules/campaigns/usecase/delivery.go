package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-635: a WhatsApp recipient counts as sent once its message is handed
// to the conversation queue; the send runs later. When that message fails
// for good, whatsapp.message.failed moves the recipient to failed (the
// counters follow by trigger) and a campaign that already finished as sent
// becomes partially_failed.

// EventDeliveryFailed is the campaign_events type of that late failure.
const EventDeliveryFailed = "delivery_failed"

// RegisterEventHandlers subscribes the campaign use case to the events it
// follows (where the outbox is published).
func (s *Service) RegisterEventHandlers(bus events.Bus) {
	bus.Subscribe(events.WhatsAppMessageFailed, s.HandleWhatsAppMessageFailed)
}

// HandleWhatsAppMessageFailed applies whatsapp.message.failed. Idempotent:
// a recipient that is not sent (already failed) is left alone.
func (s *Service) HandleWhatsAppMessageFailed(ctx context.Context, ev events.Event) error {
	rid, ok := payloadInt64(ev.Payload, "campaign_recipient_id")
	if !ok {
		return nil
	}
	reason, _ := ev.Payload["reason"].(string)
	if reason == "" {
		reason = "whatsapp send failed"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	r, err := q.FailSentCampaignRecipient(ctx, db.FailSentCampaignRecipientParams{ID: rid, Reason: truncate(reason)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("campaigns: fail recipient: %w", err)
	}
	cur, err := q.GetCampaignByID(ctx, r.CampaignID)
	if err != nil {
		return err
	}
	// The row lock orders this with the sender's finish of the campaign.
	row, err := q.GetCampaignByIDForUpdate(ctx, db.GetCampaignByIDForUpdateParams{ID: cur.ID, BrandID: cur.BrandID})
	if err != nil {
		return err
	}
	if row.Status == StatusSent {
		moved, err := q.MarkSentCampaignPartiallyFailed(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("campaigns: partially failed: %w", err)
		}
		payload, err := json.Marshal(map[string]any{
			"recipient_id": r.ID, "channel": r.Channel,
			"recipients_sent": moved.RecipientsSent, "recipients_failed": moved.RecipientsFailed,
		})
		if err != nil {
			return err
		}
		if _, err := q.InsertCampaignEvent(ctx, db.InsertCampaignEventParams{
			CampaignID: moved.ID, OrganizationID: moved.OrganizationID, BrandID: moved.BrandID,
			EventType:  EventDeliveryFailed,
			FromStatus: pgtype.Text{String: row.Status, Valid: true}, ToStatus: pgtype.Text{String: moved.Status, Valid: true},
			Payload: payload,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func payloadInt64(p map[string]any, key string) (int64, bool) {
	switch v := p[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		if v != float64(int64(v)) {
			return 0, false
		}
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	}
	return 0, false
}
