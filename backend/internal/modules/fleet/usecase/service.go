// Package usecase holds the fleet business rules (F5-02). TEC-472 brings
// opening a fleet (organization + profile + the opener's link); TEC-473
// (F5-02b) the management API: dealer links and their portal decision,
// fleet users (invitation), fleet vehicles and their import, the fleet card
// and the fleet statement.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// RuleError is a named business rule violation; handlers answer it with
// Status() and Code.
type RuleError struct {
	Code    string
	Message string
	status  int
}

func (e *RuleError) Error() string { return "fleet: " + e.Message }

// Status is the HTTP status of the rule.
func (e *RuleError) Status() int { return e.status }

var (
	// ErrInvalidTaxNumber: the VKN/TCKN checksum fails (422). Checked
	// before any query.
	ErrInvalidTaxNumber = &RuleError{
		Code: model.CodeInvalidTaxNumber, Message: "tax number checksum is invalid",
		status: http.StatusUnprocessableEntity,
	}
	// ErrFleetExists: the brand already has a fleet with this tax number;
	// the dealer requests a link to it instead (409).
	ErrFleetExists = &RuleError{
		Code: model.CodeFleetExists, Message: "a fleet with this tax number already exists",
		status: http.StatusConflict,
	}
	// ErrLinkExists: the dealer already has an open link to the fleet (409).
	ErrLinkExists = &RuleError{
		Code: model.CodeLinkExists, Message: "an open link to this dealer already exists",
		status: http.StatusConflict,
	}
	// ErrNotLinkable: only dealers and distributors link to fleets.
	ErrNotLinkable = errors.New("fleet: only a dealer or a distributor opens a fleet")
)

// ValidationError is a 400 VALIDATION_ERROR on one field.
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Conn is a pool: CreateFleet runs in one transaction on it.
type Conn interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Service is the fleet usecase.
type Service struct {
	conn         Conn
	now          func() time.Time
	out          outbox.Enqueuer
	plates       PlateValidator
	inviter      PasswordInviter
	appointments AppointmentStarter
}

// New creates the fleet usecase.
func New(conn Conn) *Service { return &Service{conn: conn, now: time.Now} }

// PlateValidator checks a plate against its country's format (geo).
type PlateValidator interface {
	ValidatePlate(ctx context.Context, iso2, plate string) (geo.PlateCheck, error)
}

// PasswordInviter starts the existing password set / reset flow for a new
// fleet user (auth ForgotPassword: a one-time code by e-mail).
type PasswordInviter interface {
	ForgotPassword(ctx context.Context, email string) error
}

// SetOutbox makes the fleet writes publish fleet.* (and vehicle.*) events.
func (s *Service) SetOutbox(out outbox.Enqueuer) { s.out = out }

// SetPlates sets the plate validator of the vehicle writes.
func (s *Service) SetPlates(p PlateValidator) { s.plates = p }

// SetInviter sets the password flow of the user invitation.
func (s *Service) SetInviter(i PasswordInviter) { s.inviter = i }

// CreateFleetInput opens a fleet for a dealer (or a serving distributor).
type CreateFleetInput struct {
	Opener          db.Organization
	ActorUserID     int64
	Name            string
	LegalName       string
	TaxNumber       string
	TaxOffice       string
	ContactName     string
	ContactPhone    string
	BillingEmail    string
	ReportFrequency string
	ReportLocale    string
}

// Fleet is an opened fleet with its opener link.
type Fleet struct {
	Organization db.Organization
	Profile      db.FleetProfile
	Link         db.FleetDealerLink
}

// normalize validates the input. The tax number checksum is a named rule
// (ErrInvalidTaxNumber, 422); the other fields are 400 validation errors.
func normalize(in CreateFleetInput) (CreateFleetInput, error) {
	in.TaxNumber = strings.TrimSpace(in.TaxNumber)
	if in.TaxNumber == "" {
		return in, &ValidationError{Field: "tax_number", Message: "required"}
	}
	if !model.ValidTaxNumber(in.TaxNumber) {
		return in, ErrInvalidTaxNumber
	}
	in.LegalName = strings.TrimSpace(in.LegalName)
	if in.LegalName == "" || len(in.LegalName) > 255 {
		return in, &ValidationError{Field: "legal_name", Message: "required, at most 255 characters"}
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		in.Name = in.LegalName
	}
	in.TaxOffice = strings.TrimSpace(in.TaxOffice)
	if len(in.TaxOffice) > 120 {
		return in, &ValidationError{Field: "tax_office", Message: "at most 120 characters"}
	}
	in.ContactName = strings.TrimSpace(in.ContactName)
	if len(in.ContactName) > 160 {
		return in, &ValidationError{Field: "contact_name", Message: "at most 160 characters"}
	}
	if p := strings.TrimSpace(in.ContactPhone); p != "" {
		e164, err := phone.NormalizeE164(p, "")
		if err != nil {
			return in, &ValidationError{Field: "contact_phone", Message: "invalid phone number"}
		}
		in.ContactPhone = e164
	} else {
		in.ContactPhone = ""
	}
	if e := strings.TrimSpace(in.BillingEmail); e != "" {
		addr, err := mail.ParseAddress(e)
		if err != nil || addr.Address != e || len(e) > 255 {
			return in, &ValidationError{Field: "billing_email", Message: "invalid e-mail address"}
		}
		in.BillingEmail = e
	} else {
		in.BillingEmail = ""
	}
	switch in.ReportFrequency {
	case "":
		in.ReportFrequency = model.ReportMonthly
	case model.ReportMonthly, model.ReportQuarterly, model.ReportOff:
	default:
		return in, &ValidationError{Field: "report_frequency", Message: "monthly, quarterly or off"}
	}
	if in.ReportLocale == "" {
		in.ReportLocale = string(i18n.DefaultLocale)
	}
	if !i18n.IsSupported(in.ReportLocale) {
		return in, &ValidationError{Field: "report_locale", Message: "unsupported locale"}
	}
	return in, nil
}

// CreateFleet validates the input and opens the fleet: a fleet organization
// (no parent), its profile and an active link to the opener. A tax number
// already used in the brand is ErrFleetExists (the caller requests a link).
func (s *Service) CreateFleet(ctx context.Context, in CreateFleetInput) (Fleet, error) {
	in, err := normalize(in)
	if err != nil {
		return Fleet{}, err
	}
	if in.Opener.Type != "dealer" && in.Opener.Type != "distributor" {
		return Fleet{}, ErrNotLinkable
	}
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return Fleet{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	repo := repository.New(tx)
	q := repo.Queries()

	if _, err := repo.FindByTaxNumber(ctx, in.Opener.BrandID, in.TaxNumber); err == nil {
		return Fleet{}, ErrFleetExists
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Fleet{}, err
	}
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("fleet-%d-%s", in.Opener.BrandID, in.TaxNumber), Name: in.Name, Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: s.now(), Valid: true},
		Type:           "fleet", BrandID: in.Opener.BrandID, Currency: in.Opener.Currency,
		Locale: in.ReportLocale, Timezone: in.Opener.Timezone, Settings: []byte("{}"),
	})
	if err != nil {
		return Fleet{}, err
	}
	actor := pgtype.Int8{Int64: in.ActorUserID, Valid: in.ActorUserID != 0}
	profile, err := repo.CreateProfile(ctx, db.CreateFleetProfileParams{
		OrganizationID: org.ID, BrandID: org.BrandID, TaxNumber: in.TaxNumber,
		TaxOffice: optText(in.TaxOffice), LegalName: in.LegalName,
		ContactName: optText(in.ContactName), ContactPhone: optText(in.ContactPhone),
		BillingEmail: optText(in.BillingEmail), ReportFrequency: in.ReportFrequency,
		ReportLocale: in.ReportLocale, CreatedByUserID: actor,
	})
	if errors.Is(err, repository.ErrFleetExists) {
		return Fleet{}, ErrFleetExists
	}
	if err != nil {
		return Fleet{}, err
	}
	link, err := repo.CreateLink(ctx, db.CreateFleetDealerLinkParams{
		FleetOrgID: org.ID, DealerOrgID: in.Opener.ID, BrandID: org.BrandID, Status: model.LinkActive,
		CreatedByOrgID: in.Opener.ID, CreatedByUserID: actor,
	})
	if errors.Is(err, repository.ErrLinkExists) {
		return Fleet{}, ErrLinkExists
	}
	if err != nil {
		return Fleet{}, err
	}
	// TEC-473: the fleet cari in the opener's ledger, recorded on the link.
	cari, err := repository.EnsureFleetCari(ctx, q, in.Opener, org.ID)
	if err != nil {
		return Fleet{}, err
	}
	link.CariAccountID = pgtype.Int8{Int64: cari.ID, Valid: true}
	if err := s.emit(ctx, tx, events.FleetCreated, in.Opener.ID, in.ActorUserID, org, map[string]any{
		"link_uuid": link.Uuid.String(), "dealer_org_id": in.Opener.ID,
	}); err != nil {
		return Fleet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Fleet{}, err
	}
	return Fleet{Organization: org, Profile: profile, Link: link}, nil
}

func optText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
