package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Public dealer application form (TEC-317, F3-03f).
const (
	// TargetDealerCandidate / SourceApplicationForm are the lead columns of
	// every application.
	TargetDealerCandidate = "dealer_candidate"
	SourceApplicationForm = "application_form"

	// RoutedByTerritory / RoutedByCenter tell how the receiving organization
	// was chosen.
	RoutedByTerritory = "territory"
	RoutedByCenter    = "center"

	// CodeLeadsModuleDisabled refuses switching the form on while the leads
	// module is closed system wide (422).
	CodeLeadsModuleDisabled = "LEADS_MODULE_DISABLED"

	maxApplicationMessage = 5000
	defaultFormLanguage   = "tr"
)

// emailRe mirrors chk_leads_candidate_email.
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// AddressResolver validates a geo chain and finds the territory distributor
// (geo.Service).
type AddressResolver interface {
	ValidateAddress(ctx context.Context, countryID int64, provinceID, districtID *int64) (geo.Address, error)
	ResolveDistributor(ctx context.Context, brandID, countryID int64, provinceID, districtID *int64) (*geo.Match, error)
}

// ModuleChecker answers module switches (features.Service).
type ModuleChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
	SystemEnabled(ctx context.Context, key string) (bool, error)
}

// BoolSettings reads a boolean system setting (sysconfig.Service).
type BoolSettings interface {
	Bool(ctx context.Context, key string) bool
}

// Outbox writes an event in the caller's transaction.
type Outbox interface {
	Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error
}

// Applications is the public dealer application use case.
type Applications struct {
	pool     TxBeginner
	q        *db.Queries
	geo      AddressResolver
	modules  ModuleChecker
	settings BoolSettings
	out      Outbox
	now      func() time.Time
}

// NewApplications creates the use case.
func NewApplications(pool TxBeginner, q *db.Queries, g AddressResolver, m ModuleChecker, s BoolSettings, out Outbox) *Applications {
	return &Applications{pool: pool, q: q, geo: g, modules: m, settings: s, out: out, now: time.Now}
}

// Enabled reports whether the form is open: the system setting is on and
// the leads module is open system wide.
func (a *Applications) Enabled(ctx context.Context) (bool, error) {
	if !a.settings.Bool(ctx, sysconfig.KeyLeadsDealerApplicationEnabled) {
		return false, nil
	}
	return a.modules.SystemEnabled(ctx, features.ModuleLeads)
}

// SettingGuard is the sysconfig write guard of the switch: it can only be
// turned on while the leads module is open system wide.
func (a *Applications) SettingGuard(ctx context.Context, value json.RawMessage) error {
	if strings.TrimSpace(string(value)) != "true" {
		return nil
	}
	open, err := a.modules.SystemEnabled(ctx, features.ModuleLeads)
	if err != nil {
		return err
	}
	if !open {
		return &sysconfig.RuleError{
			Key: sysconfig.KeyLeadsDealerApplicationEnabled, Code: CodeLeadsModuleDisabled,
			Message: "the leads module is closed system wide",
		}
	}
	return nil
}

// ApplicationInput is one public form submission.
type ApplicationInput struct {
	CompanyName string
	ContactName string
	Phone       string
	Email       string
	CountryID   int64
	ProvinceID  *int64
	DistrictID  *int64
	Message     string
	KVKKConsent bool
	Language    string
	// Honeypot is the hidden field bots fill in; a filled one is accepted
	// silently and stores nothing.
	Honeypot string
}

// Application is the normalized submission, available before Submit stores
// it (the handler rate limits by the E.164 phone).
type Application struct {
	company  string
	contact  string
	phone    string
	email    pgtype.Text
	message  string
	language string
	address  geo.Address
	// Spam is a filled honeypot: Submit stores nothing.
	Spam bool
	// PhoneE164 is the normalized phone (K29).
	PhoneE164 string
}

func validLanguage(l string) (string, bool) {
	l = strings.TrimSpace(l)
	if l == "" {
		return defaultFormLanguage, true
	}
	if l == "zh_CN" {
		l = "zh-CN"
	}
	for _, x := range msgtemplate.Locales {
		if x == l {
			return l, true
		}
	}
	return "", false
}

func requiredText(field, v string, max int) (string, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", invalid(field, "is required")
	}
	if utf8.RuneCountInString(s) > max {
		return "", invalid(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return s, nil
}

// Validate checks and normalizes a submission (400 VALIDATION_ERROR on a
// *ValidationError). A filled honeypot short-circuits as Spam.
func (a *Applications) Validate(ctx context.Context, in ApplicationInput) (Application, error) {
	if strings.TrimSpace(in.Honeypot) != "" {
		return Application{Spam: true}, nil
	}
	out := Application{}
	var err error
	if out.company, err = requiredText("company_name", in.CompanyName, maxName); err != nil {
		return Application{}, err
	}
	if out.contact, err = requiredText("contact_name", in.ContactName, maxName); err != nil {
		return Application{}, err
	}
	if !in.KVKKConsent {
		return Application{}, invalid("kvkk_consent", "must be accepted")
	}
	lang, ok := validLanguage(in.Language)
	if !ok {
		return Application{}, invalid("language", "is not a supported locale")
	}
	out.language = lang
	if email := strings.TrimSpace(in.Email); email != "" {
		if utf8.RuneCountInString(email) > maxEmail || !emailRe.MatchString(email) {
			return Application{}, invalid("email", "must be a valid e-mail address")
		}
		out.email = pgtype.Text{String: email, Valid: true}
	}
	msg := strings.TrimSpace(in.Message)
	if utf8.RuneCountInString(msg) > maxApplicationMessage {
		return Application{}, invalid("message", fmt.Sprintf("must be at most %d characters", maxApplicationMessage))
	}
	out.message = msg
	if in.CountryID <= 0 {
		return Application{}, invalid("country_id", "is required")
	}
	addr, err := a.geo.ValidateAddress(ctx, in.CountryID, in.ProvinceID, in.DistrictID)
	if err != nil {
		if errors.Is(err, geo.ErrInvalid) {
			return Application{}, invalid(addressField(err), strings.TrimPrefix(err.Error(), geo.ErrInvalid.Error()+": "))
		}
		return Application{}, err
	}
	if !addr.Country.IsActive {
		return Application{}, invalid("country_id", "country_id is unknown")
	}
	out.address = addr
	// K29: national numbers are read in the selected country.
	e164, perr := phone.NormalizeE164(in.Phone, addr.Country.Iso2)
	if perr != nil {
		return Application{}, invalid("phone", "must be a valid phone number")
	}
	out.phone, out.PhoneE164 = e164, e164
	return out, nil
}

func addressField(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "district"):
		return "district_id"
	case strings.Contains(msg, "province"):
		return "province_id"
	default:
		return "country_id"
	}
}

// ApplicationResult is what the use case did (not shown to the applicant).
type ApplicationResult struct {
	Lead           *db.Lead
	OrganizationID int64
	RoutedBy       string
	NotifyUserIDs  []int64
}

// target picks the receiving organization: the most specific territory
// distributor (district > province > country) whose leads module is on and
// which is active, else the brand center.
func (a *Applications) target(ctx context.Context, brandID int64, app Application) (db.Organization, string, error) {
	c, p, d := app.address.IDs()
	match, err := a.geo.ResolveDistributor(ctx, brandID, c.Int64, intPtr(p), intPtr(d))
	if err != nil {
		return db.Organization{}, "", fmt.Errorf("leads: territory: %w", err)
	}
	if match != nil {
		org, err := a.q.GetOrganizationByID(ctx, match.OrganizationID)
		if err != nil {
			return db.Organization{}, "", fmt.Errorf("leads: distributor: %w", err)
		}
		if org.Status == "active" && !org.DeletedAt.Valid && org.BrandID == brandID {
			on, err := a.modules.Enabled(ctx, org.ID, features.ModuleLeads)
			if err != nil {
				return db.Organization{}, "", fmt.Errorf("leads: distributor module: %w", err)
			}
			if on {
				return org, RoutedByTerritory, nil
			}
		}
	}
	center, err := a.q.GetBrandCenter(ctx, brandID)
	if err != nil {
		return db.Organization{}, "", fmt.Errorf("leads: brand center: %w", err)
	}
	return center, RoutedByCenter, nil
}

func (app Application) location() string {
	parts := []string{}
	if app.address.District != nil {
		parts = append(parts, app.address.District.Name)
	}
	if app.address.Province != nil {
		parts = append(parts, app.address.Province.Name)
	}
	parts = append(parts, app.address.Country.NameEn)
	return strings.Join(parts, ", ")
}

// Submit stores a validated application in the brand: one lead
// (dealer_candidate, application_form) in the receiving organization, the
// applicant's message, form language and KVKK consent on the timeline, and
// the leads.application_received outbox event for the notification. A
// spam submission stores nothing.
func (a *Applications) Submit(ctx context.Context, brandID int64, app Application) (ApplicationResult, error) {
	if app.Spam {
		return ApplicationResult{}, nil
	}
	org, routed, err := a.target(ctx, brandID, app)
	if err != nil {
		return ApplicationResult{}, err
	}
	c, p, d := app.address.IDs()
	now := a.now().UTC()
	var res ApplicationResult
	err = a.inTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		row, err := q.CreateLead(ctx, db.CreateLeadParams{
			OrganizationID: org.ID, BrandID: brandID, TargetType: TargetDealerCandidate,
			CandidateCompanyName: pgtype.Text{String: app.company, Valid: true},
			CandidateContactName: pgtype.Text{String: app.contact, Valid: true},
			CandidatePhoneE164:   pgtype.Text{String: app.phone, Valid: true},
			CandidateEmail:       app.email,
			CountryID:            c, ProvinceID: p, DistrictID: d,
			Source: SourceApplicationForm, Temperature: "cold", Status: StatusNew, Notes: app.message,
		})
		if err != nil {
			return fmt.Errorf("leads: application: %w", err)
		}
		if err := addEvent(ctx, q, row, EventMessage, map[string]any{
			"kind": SourceApplicationForm, "body": app.message, "language": app.language,
			"kvkk_consent": true, "kvkk_consented_at": now.Format(time.RFC3339),
			"routed_by": routed,
		}, pgtype.Int8{}); err != nil {
			return fmt.Errorf("leads: application event: %w", err)
		}
		ids, err := q.ListTransferNotifyUserIDs(ctx, db.ListTransferNotifyUserIDsParams{
			OrganizationID: org.ID, PermissionSlug: rbac.PermLeadsRead,
		})
		if err != nil {
			return fmt.Errorf("leads: recipients: %w", err)
		}
		if ids == nil {
			ids = []int64{}
		}
		id, uid := row.ID, row.Uuid
		ev := events.New(events.LeadsApplicationReceived).WithTenant(org.ID).
			WithEntity("lead", &id, &uid).WithPayload(map[string]any{
			"lead_uuid": row.Uuid.String(), "brand_id": brandID, "organization_id": org.ID,
			"routed_by": routed, "company_name": app.company, "contact_name": app.contact,
			"location": app.location(), "language": app.language, "notify_user_ids": ids,
		})
		if a.out != nil && tx != nil {
			if err := a.out.Enqueue(ctx, tx, ev); err != nil {
				return fmt.Errorf("leads: outbox: %w", err)
			}
		}
		res = ApplicationResult{Lead: &row, OrganizationID: org.ID, RoutedBy: routed, NotifyUserIDs: ids}
		return nil
	})
	if err != nil {
		return ApplicationResult{}, err
	}
	return res, nil
}

func (a *Applications) inTx(ctx context.Context, fn func(pgx.Tx, *db.Queries) error) error {
	if a.pool == nil {
		return fn(nil, a.q)
	}
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(tx, a.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
