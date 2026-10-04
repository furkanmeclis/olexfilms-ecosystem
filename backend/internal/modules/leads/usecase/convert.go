package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	customeruc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	orguc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	serviceuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ConvertKindCustomer             = "customer"
	ConvertKindDealerCandidate      = "dealer_candidate"
	ConvertKindDistributorCandidate = "distributor_candidate"

	WonRefService      = "service"
	WonRefOrganization = "organization"

	CodeLeadAlreadyConverted = "LEAD_ALREADY_CONVERTED"
	CodeLeadUserConflict     = "LEAD_USER_CONFLICT"
)

var (
	ErrAlreadyConverted = errors.New("leads: already converted")
	ErrUserConflict     = errors.New("leads: user conflict")
)

// SetConverters wires the domain services needed by POST /v1/leads/{uuid}/convert.
// Conversion binds each of them to its own transaction (WithTx) so the lead,
// its event and the created customer/service/organization commit together.
func (s *Service) SetConverters(customers *customeruc.Service, services *serviceuc.Service, organizations *orguc.Service) {
	s.customers = customers
	s.services = services
	s.organizations = organizations
}

// ConvertInput selects the conversion target.
type ConvertInput struct {
	Kind                string
	DistributorUUID     *uuid.UUID
	Currency            string
	RegisterAsWarehouse bool
}

// ExistingUser is returned with 409 when an org candidate collides with a user.
type ExistingUser struct {
	UUID        uuid.UUID `json:"uuid"`
	Name        string    `json:"name"`
	Surname     string    `json:"surname"`
	EmailMasked *string   `json:"email_masked,omitempty"`
	PhoneMasked *string   `json:"phone_masked,omitempty"`
}

// UserConflictError carries masked information about the existing user.
type UserConflictError struct {
	User ExistingUser
}

func (e *UserConflictError) Error() string { return ErrUserConflict.Error() }
func (e *UserConflictError) Unwrap() error { return ErrUserConflict }

// ConvertResult is the response of a lead conversion.
type ConvertResult struct {
	Lead         Lead                   `json:"lead"`
	ServiceDraft *serviceuc.ServiceView `json:"service_draft,omitempty"`
	Organization *orguc.Organization    `json:"organization,omitempty"`
	ExistingUser *ExistingUser          `json:"existing_user,omitempty"`
}

func validConvertKind(kind string) bool {
	switch kind {
	case ConvertKindCustomer, ConvertKindDealerCandidate, ConvertKindDistributorCandidate:
		return true
	default:
		return false
	}
}

// converted is what a conversion branch created inside the transaction.
type converted struct {
	refType string
	refID   int64
	payload map[string]any
	result  ConvertResult
	flush   func(context.Context)
}

// Convert wins a lead and creates the matching draft service or organization.
// Everything (created user/customer link/service or organization, lead won +
// won_ref, converted event) commits in one transaction; the lead row is
// locked first so a concurrent second convert waits and then gets 409.
func (s *Service) Convert(ctx context.Context, c Caller, id uuid.UUID, in ConvertInput) (ConvertResult, error) {
	row, err := s.getRow(ctx, c, id)
	if err != nil {
		return ConvertResult{}, err
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = row.TargetType
	}
	if !validConvertKind(kind) {
		return ConvertResult{}, invalid("kind", "must be customer, dealer_candidate or distributor_candidate")
	}
	if row.TargetType != kind {
		return ConvertResult{}, invalid("kind", "must match the lead target_type")
	}
	if converted := row.Status == StatusWon || row.WonRefType.Valid || row.WonRefID.Valid; converted {
		return ConvertResult{}, ErrAlreadyConverted
	}
	if err := s.authorizeConvert(c, kind, in); err != nil {
		return ConvertResult{}, err
	}
	if s.pool == nil || s.customers == nil || s.services == nil || s.organizations == nil {
		return ConvertResult{}, fmt.Errorf("leads: converters are not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ConvertResult{}, fmt.Errorf("leads: convert begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	locked, err := q.GetLeadByIDForUpdate(ctx, db.GetLeadByIDForUpdateParams{ID: row.ID, BrandID: row.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ConvertResult{}, ErrNotFound
	}
	if err != nil {
		return ConvertResult{}, fmt.Errorf("leads: convert lock: %w", err)
	}
	if locked.Status == StatusWon || locked.WonRefType.Valid || locked.WonRefID.Valid {
		return ConvertResult{}, ErrAlreadyConverted
	}
	if locked.TargetType != kind {
		return ConvertResult{}, invalid("kind", "must match the lead target_type")
	}
	var out converted
	switch kind {
	case ConvertKindCustomer:
		out, err = s.convertCustomer(ctx, tx, q, c, locked)
	case ConvertKindDealerCandidate:
		out, err = s.convertDealerCandidate(ctx, tx, q, c, locked, in)
	case ConvertKindDistributorCandidate:
		out, err = s.convertDistributorCandidate(ctx, tx, q, c, locked, in)
	default:
		err = invalid("kind", "is not supported")
	}
	if err != nil {
		return ConvertResult{}, convertError(err)
	}
	won, err := q.SetLeadStatus(ctx, db.SetLeadStatusParams{
		ID: locked.ID, OrganizationID: locked.OrganizationID, Status: StatusWon,
		WonRefType: pgtype.Text{String: out.refType, Valid: true}, WonRefID: pgtype.Int8{Int64: out.refID, Valid: true},
	})
	if err != nil {
		return ConvertResult{}, fmt.Errorf("leads: convert status: %w", err)
	}
	out.payload["won_ref_type"] = out.refType
	out.payload["won_ref_id"] = out.refID
	if err := addEvent(ctx, q, won, EventConverted, out.payload, c.actor()); err != nil {
		return ConvertResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ConvertResult{}, fmt.Errorf("leads: convert commit: %w", err)
	}
	if out.flush != nil {
		out.flush(ctx)
	}
	lead, err := s.toLead(ctx, won)
	if err != nil {
		return ConvertResult{}, err
	}
	out.result.Lead = lead
	return out.result, nil
}

// authorizeConvert applies the org-conversion permissions before any write.
func (s *Service) authorizeConvert(c Caller, kind string, in ConvertInput) error {
	switch kind {
	case ConvertKindDealerCandidate:
		if !c.Principal.HasPermission(rbac.PermLeadsConvertOrg) {
			return ErrForbidden
		}
	case ConvertKindDistributorCandidate:
		if c.Org.OrgType != rbac.OrgTypeCenter || !c.Principal.IsSuperAdmin {
			return ErrForbidden
		}
		if strings.TrimSpace(in.Currency) == "" {
			return invalid("currency", "is required for distributor conversion")
		}
	}
	return nil
}

// convertError maps the reused use cases' errors onto the lead API contract.
func convertError(err error) error {
	var cve *customeruc.ValidationError
	var sve *serviceuc.ValidationError
	switch {
	case errors.As(err, &cve):
		return invalid(cve.Field, cve.Message)
	case errors.As(err, &sve):
		return invalid(sve.Field, sve.Message)
	case errors.Is(err, orguc.ErrInvalidRequest):
		msg := strings.TrimPrefix(err.Error(), orguc.ErrInvalidRequest.Error()+": ")
		return invalid("organization", msg)
	case errors.Is(err, customeruc.ErrForbidden), errors.Is(err, serviceuc.ErrForbidden):
		return ErrForbidden
	default:
		return err
	}
}

func (s *Service) convertCustomer(ctx context.Context, tx pgx.Tx, q *db.Queries, c Caller, row db.Lead) (converted, error) {
	if !row.VehicleID.Valid {
		return converted{}, invalid("vehicle_id", "is required for customer conversion")
	}
	customers, flush := s.customers.WithTx(tx)
	cust, err := s.customerForLead(ctx, customers, q, c, row)
	if err != nil {
		return converted{}, err
	}
	veh, err := q.GetVehicleByID(ctx, row.VehicleID.Int64)
	custRow, custErr := q.GetUserByUUID(ctx, cust.UUID)
	if errors.Is(custErr, pgx.ErrNoRows) {
		return converted{}, invalid("customer_user_id", "customer not found")
	}
	if custErr != nil {
		return converted{}, fmt.Errorf("leads: customer ref: %w", custErr)
	}
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && veh.UserID != custRow.ID) {
		return converted{}, invalid("vehicle_id", "vehicle not found for this customer")
	}
	if err != nil {
		return converted{}, fmt.Errorf("leads: vehicle: %w", err)
	}
	serviceCaller := serviceuc.Caller{
		Principal: withInternalPermission(c.Principal, rbac.PermServicesWrite, rbac.PermLeadsWrite),
		Org:       c.Org,
		Filter:    c.Filter,
	}
	draft, err := s.services.WithTx(tx).Create(ctx, serviceCaller, serviceuc.CreateInput{
		CustomerUUID: cust.UUID.String(), VehicleUUID: veh.Uuid.String(),
	})
	if err != nil {
		return converted{}, err
	}
	serviceRow, err := q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: draft.UUID, BrandID: c.Org.BrandID})
	if err != nil {
		return converted{}, fmt.Errorf("leads: service ref: %w", err)
	}
	return converted{
		refType: WonRefService, refID: serviceRow.ID,
		payload: map[string]any{"kind": "service_draft", "service_uuid": draft.UUID.String(), "customer_uuid": cust.UUID.String()},
		result:  ConvertResult{ServiceDraft: &draft},
		flush:   flush,
	}, nil
}

func (s *Service) customerForLead(ctx context.Context, customers *customeruc.Service, q *db.Queries, c Caller, row db.Lead) (customeruc.CustomerWrite, error) {
	cc := customeruc.Caller{UserID: c.Principal.UserInternal, Org: c.Org, Filter: c.Filter}
	if row.CustomerUserID.Valid {
		u, err := q.GetUserByID(ctx, row.CustomerUserID.Int64)
		if errors.Is(err, pgx.ErrNoRows) {
			return customeruc.CustomerWrite{}, invalid("customer_user_id", "customer not found")
		}
		if err != nil {
			return customeruc.CustomerWrite{}, fmt.Errorf("leads: customer: %w", err)
		}
		phoneText := ""
		if u.PhoneE164.Valid {
			phoneText = u.PhoneE164.String
		} else if row.CandidatePhoneE164.Valid {
			phoneText = row.CandidatePhoneE164.String
		}
		return customers.CreateCustomer(ctx, cc, customeruc.CreateCustomerInput{
			Phone: phoneText, Name: firstNonEmpty(u.Name, textString(row.CandidateContactName), "Customer"), Surname: firstNonEmpty(u.Surname, "-"),
			Email: textString(u.Email),
		})
	}
	if !row.CandidatePhoneE164.Valid {
		return customeruc.CustomerWrite{}, invalid("candidate_phone_e164", "is required")
	}
	name, surname := splitName(textString(row.CandidateContactName))
	if name == "" {
		name = "Customer"
	}
	if surname == "" {
		surname = "-"
	}
	return customers.CreateCustomer(ctx, cc, customeruc.CreateCustomerInput{
		Phone: row.CandidatePhoneE164.String, Name: name, Surname: surname, Email: textString(row.CandidateEmail),
	})
}

func (s *Service) convertDealerCandidate(ctx context.Context, tx pgx.Tx, q *db.Queries, c Caller, row db.Lead, in ConvertInput) (converted, error) {
	parent, err := s.dealerParent(ctx, q, c, row, in.DistributorUUID)
	if err != nil {
		return converted{}, err
	}
	owner, err := s.createCandidateOwner(ctx, q, row)
	if err != nil {
		return converted{}, err
	}
	// K23: a dealer opened from a lead starts read-only (no grace period).
	return s.registerCandidateOrg(ctx, tx, q, owner, orgInput(row, orguc.TypeDealer, &parent.Uuid, parent.Currency, false, "read_only"), "dealer_org")
}

func (s *Service) convertDistributorCandidate(ctx context.Context, tx pgx.Tx, q *db.Queries, c Caller, row db.Lead, in ConvertInput) (converted, error) {
	owner, err := s.createCandidateOwner(ctx, q, row)
	if err != nil {
		return converted{}, err
	}
	return s.registerCandidateOrg(ctx, tx, q, owner, orgInput(row, orguc.TypeDistributor, &c.Org.UUID, in.Currency, in.RegisterAsWarehouse, ""), "distributor_org")
}

func (s *Service) registerCandidateOrg(ctx context.Context, tx pgx.Tx, q *db.Queries, owner db.User, in orguc.RegisterInput, eventKind string) (converted, error) {
	result, err := s.organizations.WithTx(tx).RegisterOrganization(ctx, in, owner.ID)
	if err != nil {
		return converted{}, err
	}
	orgRow, err := q.GetOrganizationByUUID(ctx, result.Organization.UUID)
	if err != nil {
		return converted{}, fmt.Errorf("leads: organization ref: %w", err)
	}
	return converted{
		refType: WonRefOrganization, refID: orgRow.ID,
		payload: map[string]any{"kind": eventKind, "organization_uuid": result.Organization.UUID.String()},
		result:  ConvertResult{Organization: &result.Organization},
	}, nil
}

func (s *Service) dealerParent(ctx context.Context, q *db.Queries, c Caller, row db.Lead, selected *uuid.UUID) (db.Organization, error) {
	owner, err := q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return db.Organization{}, fmt.Errorf("leads: owner organization: %w", err)
	}
	if owner.Type == orguc.TypeDistributor {
		return owner, nil
	}
	if owner.Type != orguc.TypeCenter || c.Org.OrgType != rbac.OrgTypeCenter {
		return db.Organization{}, ErrForbidden
	}
	if selected == nil {
		return db.Organization{}, invalid("distributor_uuid", "is required for center-owned dealer candidates")
	}
	parent, err := q.GetOrganizationByUUID(ctx, *selected)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (parent.BrandID != row.BrandID || parent.Type != orguc.TypeDistributor)) {
		return db.Organization{}, invalid("distributor_uuid", "distributor not found")
	}
	if err != nil {
		return db.Organization{}, fmt.Errorf("leads: distributor: %w", err)
	}
	return parent, nil
}

func orgInput(row db.Lead, typ string, parent *uuid.UUID, currency string, warehouse bool, status string) orguc.RegisterInput {
	name := textString(row.CandidateCompanyName)
	if name == "" {
		name = textString(row.CandidateContactName)
	}
	return orguc.RegisterInput{
		OrganizationName: name, Phone: textString(row.CandidatePhoneE164), Type: typ, ParentUUID: parent,
		RegisterAsWarehouse: warehouse, InitialStatus: status, Currency: currency,
		Location: orguc.AddressInput{CountryID: intPtr(row.CountryID), ProvinceID: intPtr(row.ProvinceID), DistrictID: intPtr(row.DistrictID)},
	}
}

func (s *Service) createCandidateOwner(ctx context.Context, q *db.Queries, row db.Lead) (db.User, error) {
	if !row.CandidatePhoneE164.Valid {
		return db.User{}, invalid("candidate_phone_e164", "is required")
	}
	if u, err := q.GetUserByPhone(ctx, row.CandidatePhoneE164); err == nil {
		return db.User{}, &UserConflictError{User: maskedUser(u)}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, fmt.Errorf("leads: phone lookup: %w", err)
	}
	email := strings.ToLower(strings.TrimSpace(textString(row.CandidateEmail)))
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			return db.User{}, invalid("candidate_email", "must be a valid email")
		}
		if u, err := q.GetUserByEmail(ctx, pgtype.Text{String: email, Valid: true}); err == nil {
			return db.User{}, &UserConflictError{User: maskedUser(u)}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, fmt.Errorf("leads: email lookup: %w", err)
		}
	}
	name, surname := splitName(textString(row.CandidateContactName))
	if name == "" {
		name = firstNonEmpty(textString(row.CandidateCompanyName), "Owner")
	}
	if surname == "" {
		surname = "-"
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return db.User{}, err
	}
	hash, err := password.Hash(fmt.Sprintf("lead-owner-%d-%s", time.Now().UnixNano(), base64.RawURLEncoding.EncodeToString(raw)))
	if err != nil {
		return db.User{}, err
	}
	u, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: pgtype.Text{String: email, Valid: email != ""}, PasswordHash: hash,
		Name: name, Surname: surname, Status: "active", PhoneE164: row.CandidatePhoneE164,
	})
	if err != nil {
		return db.User{}, err
	}
	return u, nil
}

func withInternalPermission(p authctx.Principal, add, from string) authctx.Principal {
	scope, ok := p.ScopeFor(from)
	if !ok {
		return p
	}
	cp := p
	cp.PermissionScopes = map[string]rbac.Scope{}
	for k, v := range p.PermissionScopes {
		cp.PermissionScopes[k] = v
	}
	cp.PermissionScopes[add] = scope
	return cp
}

func maskedUser(u db.User) ExistingUser {
	out := ExistingUser{UUID: u.Uuid, Name: u.Name, Surname: u.Surname}
	if u.Email.Valid {
		v := maskEmail(u.Email.String)
		out.EmailMasked = &v
	}
	if u.PhoneE164.Valid {
		v := phone.Mask(u.PhoneE164.String)
		out.PhoneMasked = &v
	}
	return out
}

func maskEmail(v string) string {
	v = strings.TrimSpace(v)
	at := strings.IndexByte(v, '@')
	if at < 0 {
		return "***"
	}
	if at <= 1 {
		return "***" + v[at:]
	}
	return v[:1] + "***" + v[at:]
}

func splitName(v string) (string, string) {
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func textString(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return strings.TrimSpace(v.String)
}
