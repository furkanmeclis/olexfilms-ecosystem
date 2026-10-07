package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WhatsApp visitor leads (TEC-397, F4-02e): an unidentified WhatsApp
// contact asking for an appointment or an offer becomes a customer lead
// (source whatsapp) in the organization the public lead rule picks (the
// territory distributor, else the brand center). One lead per number and
// brand per VisitorLeadWindow: a later request updates it.
const (
	TargetCustomer = "customer"
	SourceWhatsApp = "whatsapp"

	// VisitorLeadWindow is how long a number keeps its lead.
	VisitorLeadWindow = 30 * 24 * time.Hour

	// Visitor lead intents.
	IntentAppointment = "appointment"
	IntentQuote       = "quote"
	IntentInfo        = "info"

	maxVisitorText = 500
)

// ErrKVKKNoticeRequired: no visitor lead is written before the KVKK notice
// was delivered to the contact.
var ErrKVKKNoticeRequired = errors.New("leads: the KVKK notice was not sent to the contact")

// KVKKNotice is the notice the contact received before the lead.
type KVKKNotice struct {
	Locale  string
	Version int32
	SentAt  time.Time
}

// VisitorLeadInput is what the WhatsApp assistant collected.
type VisitorLeadInput struct {
	// PhoneE164 is the WhatsApp number (the lead's key).
	PhoneE164 string
	Name      string
	// City / District are free text (province / district names).
	City     string
	District string
	Vehicle  string
	Intent   string
	Message  string
	Language string
	// ConversationUUID links the timeline event to the conversation.
	ConversationUUID string
	Notice           *KVKKNotice
}

// VisitorLeadResult is the stored lead.
type VisitorLeadResult struct {
	Lead db.Lead
	// Created is false when the lead of the last 30 days was updated.
	Created        bool
	OrganizationID int64
	RoutedBy       string
}

func optionalText(field, v string, max int) (string, error) {
	s := strings.TrimSpace(v)
	if utf8.RuneCountInString(s) > max {
		return "", invalid(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return s, nil
}

func validIntent(v string) (string, error) {
	switch v = strings.TrimSpace(v); v {
	case "":
		return IntentInfo, nil
	case IntentAppointment, IntentQuote, IntentInfo:
		return v, nil
	}
	return "", invalid("intent", "must be appointment, quote or info")
}

type visitorLead struct {
	phone, name, vehicle, intent, message, language, city, district string
	address                                                         *geo.Address
}

func (a *Applications) validateVisitorLead(ctx context.Context, in VisitorLeadInput) (visitorLead, error) {
	var (
		v   visitorLead
		err error
	)
	if !e164Re.MatchString(in.PhoneE164) {
		return v, invalid("phone", "must be an E.164 number")
	}
	v.phone = in.PhoneE164
	if v.name, err = requiredText("name", in.Name, maxName); err != nil {
		return v, err
	}
	if v.city, err = requiredText("city", in.City, maxName); err != nil {
		return v, err
	}
	if v.district, err = optionalText("district", in.District, maxName); err != nil {
		return v, err
	}
	if v.vehicle, err = optionalText("vehicle", in.Vehicle, maxVisitorText); err != nil {
		return v, err
	}
	if v.message, err = optionalText("message", in.Message, maxVisitorText); err != nil {
		return v, err
	}
	if v.intent, err = validIntent(in.Intent); err != nil {
		return v, err
	}
	if v.language, _ = validLanguage(in.Language); v.language == "" {
		v.language = defaultFormLanguage
	}
	v.address, err = a.visitorAddress(ctx, v.city, v.district)
	return v, err
}

// visitorAddress resolves the city (and district) names to the geo chain;
// nil when the city is unknown (the lead then goes to the center).
func (a *Applications) visitorAddress(ctx context.Context, city, district string) (*geo.Address, error) {
	p, err := a.q.FindProvinceByName(ctx, city)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leads: province: %w", err)
	}
	pid := p.ID
	var did *int64
	if district != "" {
		d, err := a.q.FindDistrictByName(ctx, db.FindDistrictByNameParams{ProvinceID: p.ID, Name: district})
		switch {
		case err == nil:
			did = &d.ID
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, fmt.Errorf("leads: district: %w", err)
		}
	}
	addr, err := a.geo.ValidateAddress(ctx, p.CountryID, &pid, did)
	if err != nil {
		return nil, fmt.Errorf("leads: address: %w", err)
	}
	return &addr, nil
}

func (v visitorLead) notes() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "WhatsApp: %s", v.intent)
	if v.vehicle != "" {
		fmt.Fprintf(&sb, "\nVehicle: %s", v.vehicle)
	}
	loc := v.city
	if v.district != "" {
		loc = v.district + ", " + v.city
	}
	fmt.Fprintf(&sb, "\nLocation: %s", loc)
	if v.message != "" {
		fmt.Fprintf(&sb, "\n%s", v.message)
	}
	return sb.String()
}

// UpsertVisitorLead stores the request of a WhatsApp visitor: a new
// customer lead (source whatsapp, temperature warm) routed by the public
// lead rule, or, when the number already has a WhatsApp lead of the last
// 30 days in the brand, an update of that lead (name, address when it had
// none, notes) with a timeline message. Refused (ErrKVKKNoticeRequired)
// unless the KVKK notice was sent first.
func (a *Applications) UpsertVisitorLead(ctx context.Context, brandID int64, in VisitorLeadInput) (VisitorLeadResult, error) {
	if in.Notice == nil || in.Notice.Version < 1 || in.Notice.SentAt.IsZero() {
		return VisitorLeadResult{}, ErrKVKKNoticeRequired
	}
	v, err := a.validateVisitorLead(ctx, in)
	if err != nil {
		return VisitorLeadResult{}, err
	}
	now := a.now().UTC()
	event := map[string]any{
		"kind": SourceWhatsApp, "intent": v.intent, "body": v.notes(), "language": v.language,
		"kvkk_notice_locale": in.Notice.Locale, "kvkk_notice_version": in.Notice.Version,
		"kvkk_notice_sent_at": in.Notice.SentAt.UTC().Format(time.RFC3339),
	}
	if in.ConversationUUID != "" {
		event["conversation_uuid"] = in.ConversationUUID
	}

	cur, err := a.q.GetRecentWhatsAppVisitorLead(ctx, db.GetRecentWhatsAppVisitorLeadParams{
		BrandID: brandID, PhoneE164: pgtype.Text{String: v.phone, Valid: true},
		Since: pgtype.Timestamptz{Time: now.Add(-VisitorLeadWindow), Valid: true},
	})
	switch {
	case err == nil:
		return a.updateVisitorLead(ctx, cur, v, event)
	case !errors.Is(err, pgx.ErrNoRows):
		return VisitorLeadResult{}, fmt.Errorf("leads: visitor lead: %w", err)
	}

	app := Application{}
	if v.address != nil {
		app.address = *v.address
	}
	var (
		org    db.Organization
		routed string
	)
	if v.address != nil {
		org, routed, err = a.target(ctx, brandID, app)
	} else {
		org, err = a.q.GetBrandCenter(ctx, brandID)
		routed = RoutedByCenter
	}
	if err != nil {
		return VisitorLeadResult{}, err
	}
	c, p, d := app.address.IDs()
	event["routed_by"] = routed
	var res VisitorLeadResult
	err = a.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		row, err := q.CreateLead(ctx, db.CreateLeadParams{
			OrganizationID: org.ID, BrandID: brandID, TargetType: TargetCustomer,
			CandidateContactName: pgtype.Text{String: v.name, Valid: true},
			CandidatePhoneE164:   pgtype.Text{String: v.phone, Valid: true},
			CountryID:            c, ProvinceID: p, DistrictID: d,
			Source: SourceWhatsApp, Temperature: "warm", Status: StatusNew, Notes: v.notes(),
		})
		if err != nil {
			return fmt.Errorf("leads: visitor lead: %w", err)
		}
		if err := addEvent(ctx, q, row, EventMessage, event, pgtype.Int8{}); err != nil {
			return fmt.Errorf("leads: visitor lead event: %w", err)
		}
		res = VisitorLeadResult{Lead: row, Created: true, OrganizationID: org.ID, RoutedBy: routed}
		return nil
	})
	if err != nil {
		return VisitorLeadResult{}, err
	}
	return res, nil
}

func (a *Applications) updateVisitorLead(ctx context.Context, cur db.Lead, v visitorLead, event map[string]any) (VisitorLeadResult, error) {
	country, province, district := cur.CountryID, cur.ProvinceID, cur.DistrictID
	if !country.Valid && v.address != nil {
		country, province, district = v.address.IDs()
	}
	notes := strings.TrimSpace(cur.Notes + "\n\n" + v.notes())
	if utf8.RuneCountInString(notes) > maxNotes {
		notes = string([]rune(notes)[:maxNotes])
	}
	event["updated"] = true
	var res VisitorLeadResult
	err := a.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		row, err := q.UpdateLead(ctx, db.UpdateLeadParams{
			ID: cur.ID, OrganizationID: cur.OrganizationID, TargetType: cur.TargetType,
			CustomerUserID: cur.CustomerUserID, VehicleID: cur.VehicleID,
			CandidateCompanyName: cur.CandidateCompanyName,
			CandidateContactName: pgtype.Text{String: v.name, Valid: true},
			CandidatePhoneE164:   cur.CandidatePhoneE164, CandidateEmail: cur.CandidateEmail,
			CountryID: country, ProvinceID: province, DistrictID: district,
			Source: cur.Source, Temperature: cur.Temperature, LostReason: cur.LostReason,
			FollowUpDate: cur.FollowUpDate, AssigneeUserID: cur.AssigneeUserID, Notes: notes,
		})
		if err != nil {
			return fmt.Errorf("leads: visitor lead update: %w", err)
		}
		if err := addEvent(ctx, q, row, EventMessage, event, pgtype.Int8{}); err != nil {
			return fmt.Errorf("leads: visitor lead event: %w", err)
		}
		res = VisitorLeadResult{Lead: row, OrganizationID: row.OrganizationID}
		return nil
	})
	if err != nil {
		return VisitorLeadResult{}, err
	}
	return res, nil
}
