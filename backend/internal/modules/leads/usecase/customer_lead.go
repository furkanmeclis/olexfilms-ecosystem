package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Customer appointment requests (TEC-639): the customer assistant cannot
// book an appointment, so a signed-in customer asking for one becomes a
// customer lead (source whatsapp, intent appointment) linked to the
// customer user, in the dealer of the customer's newest service in the
// brand (else the brand center). The dealer schedules it from the lead.
// The customer account already covers the KVKK notice. Like visitor
// leads, one WhatsApp lead per number and brand per VisitorLeadWindow: a
// later request updates it.

// RoutedByCustomerDealer: the lead went to the dealer of the customer's
// newest service.
const RoutedByCustomerDealer = "customer_dealer"

// customerServiceLookback is how many of the newest services are scanned
// for an active dealer.
const customerServiceLookback = 5

// CustomerAppointmentInput is what the customer assistant collected.
type CustomerAppointmentInput struct {
	CustomerUserID int64
	// PreferredTime is the customer's wish as written ("15 Ekim 13:00").
	PreferredTime    string
	Vehicle          string
	Message          string
	Language         string
	ConversationUUID string
}

// UpsertCustomerAppointmentLead stores an appointment request of a
// customer user of the brand. The user must have a phone number (the
// lead's contact and key).
func (a *Applications) UpsertCustomerAppointmentLead(ctx context.Context, brandID int64, in CustomerAppointmentInput) (VisitorLeadResult, error) {
	user, err := a.q.GetUserByID(ctx, in.CustomerUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return VisitorLeadResult{}, ErrNotFound
	}
	if err != nil {
		return VisitorLeadResult{}, fmt.Errorf("leads: customer: %w", err)
	}
	if !user.PhoneE164.Valid || !e164Re.MatchString(user.PhoneE164.String) {
		return VisitorLeadResult{}, invalid("phone", "the customer has no phone number")
	}
	v := visitorLead{phone: user.PhoneE164.String, intent: IntentAppointment}
	if v.name, err = requiredText("name", strings.TrimSpace(user.Name+" "+user.Surname), maxName); err != nil {
		return VisitorLeadResult{}, err
	}
	preferred, err := optionalText("preferred_time", in.PreferredTime, maxName)
	if err != nil {
		return VisitorLeadResult{}, err
	}
	if v.vehicle, err = optionalText("vehicle", in.Vehicle, maxVisitorText); err != nil {
		return VisitorLeadResult{}, err
	}
	if v.message, err = optionalText("message", in.Message, maxVisitorText); err != nil {
		return VisitorLeadResult{}, err
	}
	if v.language, _ = validLanguage(in.Language); v.language == "" {
		v.language = defaultFormLanguage
	}
	notes := customerAppointmentNotes(preferred, v)
	event := map[string]any{"kind": SourceWhatsApp, "intent": v.intent, "body": notes, "language": v.language, "customer": true}
	if preferred != "" {
		event["preferred_time"] = preferred
	}
	if in.ConversationUUID != "" {
		event["conversation_uuid"] = in.ConversationUUID
	}

	now := a.now().UTC()
	cur, err := a.q.GetRecentWhatsAppVisitorLead(ctx, db.GetRecentWhatsAppVisitorLeadParams{
		BrandID: brandID, PhoneE164: pgtype.Text{String: v.phone, Valid: true},
		Since: pgtype.Timestamptz{Time: now.Add(-VisitorLeadWindow), Valid: true},
	})
	switch {
	case err == nil:
		return a.updateVisitorLead(ctx, cur, v, notes, event)
	case !errors.Is(err, pgx.ErrNoRows):
		return VisitorLeadResult{}, fmt.Errorf("leads: customer lead: %w", err)
	}

	org, routed, err := a.customerDealer(ctx, brandID, user.ID)
	if err != nil {
		return VisitorLeadResult{}, err
	}
	event["routed_by"] = routed
	var res VisitorLeadResult
	err = a.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		row, err := q.CreateLead(ctx, db.CreateLeadParams{
			OrganizationID: org.ID, BrandID: brandID, TargetType: TargetCustomer,
			CustomerUserID:       pgtype.Int8{Int64: user.ID, Valid: true},
			CandidateContactName: pgtype.Text{String: v.name, Valid: true},
			CandidatePhoneE164:   pgtype.Text{String: v.phone, Valid: true},
			Source:               SourceWhatsApp, Temperature: "warm", Status: StatusNew, Notes: notes,
		})
		if err != nil {
			return fmt.Errorf("leads: customer lead: %w", err)
		}
		if err := EnqueueChange(ctx, q, events.LeadsCreated, row, pgtype.Int8{}); err != nil {
			return err
		}
		if err := addEvent(ctx, q, row, EventMessage, event, pgtype.Int8{}); err != nil {
			return fmt.Errorf("leads: customer lead event: %w", err)
		}
		res = VisitorLeadResult{Lead: row, Created: true, OrganizationID: org.ID, RoutedBy: routed}
		return nil
	})
	if err != nil {
		return VisitorLeadResult{}, err
	}
	return res, nil
}

// customerDealer is the active organization of the customer's newest
// service in the brand, else the brand center.
func (a *Applications) customerDealer(ctx context.Context, brandID, userID int64) (db.Organization, string, error) {
	rows, err := a.q.ListServicesByCustomer(ctx, db.ListServicesByCustomerParams{
		CustomerUserID: userID, BrandID: pgtype.Int8{Int64: brandID, Valid: true}, RowLimit: customerServiceLookback,
	})
	if err != nil {
		return db.Organization{}, "", fmt.Errorf("leads: customer services: %w", err)
	}
	for _, s := range rows {
		if s.Status == "cancelled" {
			continue
		}
		org, err := a.q.GetOrganizationByID(ctx, s.OrganizationID)
		if err != nil {
			return db.Organization{}, "", fmt.Errorf("leads: customer dealer: %w", err)
		}
		if org.BrandID == brandID && org.Status == "active" && !org.DeletedAt.Valid {
			return org, RoutedByCustomerDealer, nil
		}
	}
	org, err := a.q.GetBrandCenter(ctx, brandID)
	if err != nil {
		return db.Organization{}, "", fmt.Errorf("leads: brand center: %w", err)
	}
	return org, RoutedByCenter, nil
}

func customerAppointmentNotes(preferred string, v visitorLead) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "WhatsApp: %s (customer)", IntentAppointment)
	if preferred != "" {
		fmt.Fprintf(&sb, "\nPreferred time: %s", preferred)
	}
	if v.vehicle != "" {
		fmt.Fprintf(&sb, "\nVehicle: %s", v.vehicle)
	}
	if v.message != "" {
		fmt.Fprintf(&sb, "\n%s", v.message)
	}
	return sb.String()
}
