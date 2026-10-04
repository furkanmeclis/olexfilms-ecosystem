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

// CustomerConverter is the customer module entry point used by conversion.
type CustomerConverter interface {
	CreateCustomer(ctx context.Context, c customeruc.Caller, in customeruc.CreateCustomerInput) (customeruc.CustomerWrite, error)
}

// ServiceDrafter is the service module entry point used by conversion.
type ServiceDrafter interface {
	Create(ctx context.Context, c serviceuc.Caller, in serviceuc.CreateInput) (serviceuc.ServiceView, error)
}

// OrganizationRegistrar is the organization module entry point used by conversion.
type OrganizationRegistrar interface {
	RegisterOrganization(ctx context.Context, in orguc.RegisterInput, ownerUserID int64) (orguc.RegisterResult, error)
}

// SetConverters wires the domain services needed by POST /v1/leads/{uuid}/convert.
func (s *Service) SetConverters(customers CustomerConverter, services ServiceDrafter, organizations OrganizationRegistrar) {
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

// Convert wins a lead and creates the matching draft service or organization.
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
	if row.Status == StatusWon || row.WonRefType.Valid || row.WonRefID.Valid {
		return ConvertResult{}, ErrAlreadyConverted
	}
	switch kind {
	case ConvertKindCustomer:
		return s.convertCustomer(ctx, c, row)
	case ConvertKindDealerCandidate:
		return s.convertDealerCandidate(ctx, c, row, in)
	case ConvertKindDistributorCandidate:
		return s.convertDistributorCandidate(ctx, c, row, in)
	default:
		return ConvertResult{}, invalid("kind", "is not supported")
	}
}

func (s *Service) convertCustomer(ctx context.Context, c Caller, row db.Lead) (ConvertResult, error) {
	if s.customers == nil || s.services == nil {
		return ConvertResult{}, fmt.Errorf("leads: converters are not configured")
	}
	if !row.VehicleID.Valid {
		return ConvertResult{}, invalid("vehicle_id", "is required for customer conversion")
	}
	cust, err := s.customerForLead(ctx, c, row)
	if err != nil {
		return ConvertResult{}, err
	}
	veh, err := s.q.GetVehicleByID(ctx, row.VehicleID.Int64)
	custRow, custErr := s.q.GetUserByUUID(ctx, cust.UUID)
	if errors.Is(custErr, pgx.ErrNoRows) {
		return ConvertResult{}, invalid("customer_user_id", "customer not found")
	}
	if custErr != nil {
		return ConvertResult{}, fmt.Errorf("leads: customer ref: %w", custErr)
	}
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && veh.UserID != custRow.ID) {
		return ConvertResult{}, invalid("vehicle_id", "vehicle not found for this customer")
	}
	if err != nil {
		return ConvertResult{}, fmt.Errorf("leads: vehicle: %w", err)
	}
	serviceCaller := serviceuc.Caller{
		Principal: withInternalPermission(c.Principal, rbac.PermServicesWrite, rbac.PermLeadsWrite),
		Org:       c.Org,
		Filter:    c.Filter,
	}
	draft, err := s.services.Create(ctx, serviceCaller, serviceuc.CreateInput{
		CustomerUUID: cust.UUID.String(), VehicleUUID: veh.Uuid.String(),
	})
	if err != nil {
		return ConvertResult{}, err
	}
	serviceRow, err := s.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: draft.UUID, BrandID: c.Org.BrandID})
	if err != nil {
		return ConvertResult{}, fmt.Errorf("leads: service ref: %w", err)
	}
	lead, err := s.markConverted(ctx, c, row, WonRefService, serviceRow.ID, map[string]any{
		"kind": "service_draft", "service_uuid": draft.UUID.String(), "customer_uuid": cust.UUID.String(),
	})
	if err != nil {
		return ConvertResult{}, err
	}
	return ConvertResult{Lead: lead, ServiceDraft: &draft}, nil
}

func (s *Service) customerForLead(ctx context.Context, c Caller, row db.Lead) (customeruc.CustomerWrite, error) {
	cc := customeruc.Caller{UserID: c.Principal.UserInternal, Org: c.Org, Filter: c.Filter}
	if row.CustomerUserID.Valid {
		u, err := s.q.GetUserByID(ctx, row.CustomerUserID.Int64)
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
		return s.customers.CreateCustomer(ctx, cc, customeruc.CreateCustomerInput{
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
	return s.customers.CreateCustomer(ctx, cc, customeruc.CreateCustomerInput{
		Phone: row.CandidatePhoneE164.String, Name: name, Surname: surname, Email: textString(row.CandidateEmail),
	})
}

func (s *Service) convertDealerCandidate(ctx context.Context, c Caller, row db.Lead, in ConvertInput) (ConvertResult, error) {
	if s.organizations == nil {
		return ConvertResult{}, fmt.Errorf("leads: organization converter is not configured")
	}
	if !c.Principal.HasPermission(rbac.PermLeadsConvertOrg) {
		return ConvertResult{}, ErrForbidden
	}
	owner, err := s.createCandidateOwner(ctx, row)
	if err != nil {
		return ConvertResult{}, err
	}
	parent, err := s.dealerParent(ctx, c, row, in.DistributorUUID)
	if err != nil {
		return ConvertResult{}, err
	}
	result, err := s.organizations.RegisterOrganization(ctx, orgInput(row, orguc.TypeDealer, &parent.Uuid, parent.Currency, false, "read_only"), owner.ID)
	if err != nil {
		return ConvertResult{}, err
	}
	orgRow, err := s.q.GetOrganizationByUUID(ctx, result.Organization.UUID)
	if err != nil {
		return ConvertResult{}, fmt.Errorf("leads: organization ref: %w", err)
	}
	lead, err := s.markConverted(ctx, c, row, WonRefOrganization, orgRow.ID, map[string]any{
		"kind": "dealer_org", "organization_uuid": result.Organization.UUID.String(),
	})
	if err != nil {
		return ConvertResult{}, err
	}
	return ConvertResult{Lead: lead, Organization: &result.Organization}, nil
}

func (s *Service) convertDistributorCandidate(ctx context.Context, c Caller, row db.Lead, in ConvertInput) (ConvertResult, error) {
	if s.organizations == nil {
		return ConvertResult{}, fmt.Errorf("leads: organization converter is not configured")
	}
	if c.Org.OrgType != rbac.OrgTypeCenter || !c.Principal.IsSuperAdmin {
		return ConvertResult{}, ErrForbidden
	}
	if strings.TrimSpace(in.Currency) == "" {
		return ConvertResult{}, invalid("currency", "is required for distributor conversion")
	}
	owner, err := s.createCandidateOwner(ctx, row)
	if err != nil {
		return ConvertResult{}, err
	}
	result, err := s.organizations.RegisterOrganization(ctx, orgInput(row, orguc.TypeDistributor, &c.Org.UUID, in.Currency, in.RegisterAsWarehouse, ""), owner.ID)
	if err != nil {
		return ConvertResult{}, err
	}
	orgRow, err := s.q.GetOrganizationByUUID(ctx, result.Organization.UUID)
	if err != nil {
		return ConvertResult{}, fmt.Errorf("leads: organization ref: %w", err)
	}
	lead, err := s.markConverted(ctx, c, row, WonRefOrganization, orgRow.ID, map[string]any{
		"kind": "distributor_org", "organization_uuid": result.Organization.UUID.String(),
	})
	if err != nil {
		return ConvertResult{}, err
	}
	return ConvertResult{Lead: lead, Organization: &result.Organization}, nil
}

func (s *Service) dealerParent(ctx context.Context, c Caller, row db.Lead, selected *uuid.UUID) (db.Organization, error) {
	owner, err := s.q.GetOrganizationByID(ctx, row.OrganizationID)
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
	parent, err := s.q.GetOrganizationByUUID(ctx, *selected)
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

func (s *Service) createCandidateOwner(ctx context.Context, row db.Lead) (db.User, error) {
	if !row.CandidatePhoneE164.Valid {
		return db.User{}, invalid("candidate_phone_e164", "is required")
	}
	if u, err := s.q.GetUserByPhone(ctx, row.CandidatePhoneE164); err == nil {
		return db.User{}, &UserConflictError{User: maskedUser(u)}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, fmt.Errorf("leads: phone lookup: %w", err)
	}
	email := strings.ToLower(strings.TrimSpace(textString(row.CandidateEmail)))
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			return db.User{}, invalid("candidate_email", "must be a valid email")
		}
		if u, err := s.q.GetUserByEmail(ctx, pgtype.Text{String: email, Valid: true}); err == nil {
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
	u, err := s.q.CreateUser(ctx, db.CreateUserParams{
		Email: pgtype.Text{String: email, Valid: email != ""}, PasswordHash: hash,
		Name: name, Surname: surname, Status: "active", PhoneE164: row.CandidatePhoneE164,
	})
	if err != nil {
		return db.User{}, err
	}
	return u, nil
}

func (s *Service) markConverted(ctx context.Context, c Caller, row db.Lead, refType string, refID int64, payload map[string]any) (Lead, error) {
	var out db.Lead
	err := s.inTx(ctx, func(q *db.Queries) error {
		locked, err := q.GetLeadByID(ctx, db.GetLeadByIDParams{ID: row.ID, BrandID: row.BrandID})
		if err != nil {
			return err
		}
		if locked.Status == StatusWon || locked.WonRefType.Valid || locked.WonRefID.Valid {
			return ErrAlreadyConverted
		}
		out, err = q.SetLeadStatus(ctx, db.SetLeadStatusParams{
			ID: locked.ID, OrganizationID: locked.OrganizationID, Status: StatusWon,
			WonRefType: pgtype.Text{String: refType, Valid: true}, WonRefID: pgtype.Int8{Int64: refID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("leads: convert status: %w", err)
		}
		payload["won_ref_type"] = refType
		payload["won_ref_id"] = refID
		return addEvent(ctx, q, out, EventConverted, payload, c.actor())
	})
	if err != nil {
		return Lead{}, err
	}
	return s.toLead(ctx, out)
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
