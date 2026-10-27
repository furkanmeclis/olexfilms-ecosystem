package usecase

// Tenant account opening (demo flow follow-up of TEC-1024/1028): an
// organization owner adds staff to their own organization (members.write)
// and a center or distributor opens a dealer with its owner directly under
// itself (organizations.write). Accounts are phone-first: the phone (E.164)
// is what the mobile OTP sign-in looks up, so it is required; the e-mail is
// optional. A phone that already belongs to a user attaches that user (no
// second account); the password is a random one the user never needs.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AccountInput is the person of a new account.
type AccountInput struct {
	Name    string
	Surname string
	// Phone in E.164 or national form (the organization's country, TR by
	// default).
	Phone string
	Email string
}

// TeamMemberInput adds a staff member to the caller's organization.
type TeamMemberInput struct {
	AccountInput
	// RoleSlugs are system roles of the organization's type; owner roles
	// cannot be granted. Empty: the default staff role.
	RoleSlugs []string
	// WarehouseUUIDs are warehouses of the organization the member joins.
	WarehouseUUIDs []uuid.UUID
}

// TeamMember is the account and membership created by CreateTeamMember.
type TeamMember struct {
	UserUUID   uuid.UUID   `json:"user_uuid"`
	Name       string      `json:"name"`
	Surname    string      `json:"surname"`
	Phone      string      `json:"phone"`
	Email      *string     `json:"email,omitempty"`
	RoleSlugs  []string    `json:"role_slugs"`
	Warehouses []uuid.UUID `json:"warehouse_uuids"`
	// Existing is true when the phone already belonged to a user.
	Existing bool `json:"existing"`
}

// DealerInput opens a dealer under the caller's organization.
type DealerInput struct {
	OrganizationName string
	Phone            string
	City             string
	District         string
	Address          string
	Location         AddressInput
	Owner            AccountInput
}

// DealerResult is the dealer and its owner.
type DealerResult struct {
	Organization Organization `json:"organization"`
	Owner        TeamMember   `json:"owner"`
}

// ownerRoles cannot be granted through the team endpoint.
var ownerRoles = map[string]bool{
	rbac.RoleCenterOwner: true, rbac.RoleDistributorOwner: true, rbac.RoleDealerOwner: true,
}

// CreateTeamMember creates (or attaches by phone) a staff member of orgID
// with the given roles and warehouses. actor is the calling user.
func (s *Service) CreateTeamMember(ctx context.Context, orgID int64, actor int64, in TeamMemberInput) (TeamMember, error) {
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return TeamMember{}, err
	}
	roles, err := validMemberRoles(org.Type, in.RoleSlugs)
	if err != nil {
		return TeamMember{}, err
	}
	for _, r := range roles {
		if ownerRoles[r] {
			return TeamMember{}, fmt.Errorf("%w: role %q cannot be granted here", ErrInvalidRequest, r)
		}
	}
	if len(roles) == 0 {
		roles = []string{rbac.DefaultMemberRole(org.Type, "staff")}
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return TeamMember{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	warehouses := make([]db.Warehouse, 0, len(in.WarehouseUUIDs))
	for _, wu := range in.WarehouseUUIDs {
		w, err := q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: wu, OrganizationID: org.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return TeamMember{}, fmt.Errorf("%w: warehouse %s not found", ErrInvalidRequest, wu)
		}
		if err != nil {
			return TeamMember{}, err
		}
		warehouses = append(warehouses, w)
	}
	iso2, err := s.orgCountryISO2(ctx, org.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return TeamMember{}, err
	}
	user, existing, err := accountUser(ctx, q, in.AccountInput, iso2)
	if err != nil {
		return TeamMember{}, err
	}
	if err := addMember(ctx, q, org, user.ID, "staff", roles); err != nil {
		if strings.Contains(err.Error(), "uq_organization_members") {
			return TeamMember{}, fmt.Errorf("%w: the user is already a member of this organization", ErrConflict)
		}
		return TeamMember{}, err
	}
	out := teamMember(user, existing, roles)
	for i, w := range warehouses {
		if _, err := q.InsertWarehouseMember(ctx, db.InsertWarehouseMemberParams{
			OrganizationID: org.ID, WarehouseID: w.ID, UserID: user.ID, IsDefault: i == 0,
			CreatedByUserID: pgtype.Int8{Int64: actor, Valid: actor != 0},
		}); err != nil {
			return TeamMember{}, fmt.Errorf("warehouse member: %w", err)
		}
		out.Warehouses = append(out.Warehouses, w.Uuid)
	}
	if s.out != nil {
		if err := s.out.Enqueue(ctx, tx, MemberAddedEvent(org.ID, user.ID, "staff")); err != nil {
			return TeamMember{}, fmt.Errorf("organizations: outbox: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return TeamMember{}, err
	}
	return out, nil
}

// CreateDealer opens an active dealer directly under parentID (a center or
// a distributor) with its owner (dealer_owner), in one transaction. The
// dealer gets no trial end: it is the parent's own dealer.
func (s *Service) CreateDealer(ctx context.Context, parentID int64, in DealerInput) (DealerResult, error) {
	parent, err := s.q.GetOrganizationByID(ctx, parentID)
	if err != nil {
		return DealerResult{}, err
	}
	if parent.Type != TypeCenter && parent.Type != TypeDistributor {
		return DealerResult{}, fmt.Errorf("%w: only a center or a distributor opens dealers", ErrInvalidRequest)
	}
	if strings.TrimSpace(in.OrganizationName) == "" {
		return DealerResult{}, fmt.Errorf("%w: organization_name is required", ErrInvalidRequest)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return DealerResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	inTx := s.WithTx(tx)
	iso2, err := s.orgCountryISO2(ctx, parent.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return DealerResult{}, err
	}
	user, existing, err := accountUser(ctx, inTx.q, in.Owner, iso2)
	if err != nil {
		return DealerResult{}, err
	}
	parentUUID := parent.Uuid
	reg, err := inTx.RegisterOrganization(ctx, RegisterInput{
		OrganizationName: in.OrganizationName, Phone: in.Phone, City: in.City, District: in.District,
		Address: in.Address, Location: in.Location, Type: TypeDealer, ParentUUID: &parentUUID,
		InitialStatus: "active", NoTrial: true,
	}, user.ID)
	if err != nil {
		return DealerResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DealerResult{}, err
	}
	return DealerResult{
		Organization: reg.Organization,
		Owner:        teamMember(user, existing, []string{rbac.DefaultMemberRole(TypeDealer, "owner")}),
	}, nil
}

// accountUser returns the user of the phone, or creates one (active, the
// phone stored for OTP sign-in, a random password).
func accountUser(ctx context.Context, q *db.Queries, in AccountInput, iso2 string) (db.User, bool, error) {
	phone, err := normalizePhone(in.Phone, iso2)
	if err != nil {
		return db.User{}, false, err
	}
	if phone == "" {
		return db.User{}, false, fmt.Errorf("%w: phone is required", ErrInvalidRequest)
	}
	if u, err := q.GetUserByPhone(ctx, pgtype.Text{String: phone, Valid: true}); err == nil {
		return u, true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, false, err
	}
	name, surname := strings.TrimSpace(in.Name), strings.TrimSpace(in.Surname)
	if name == "" {
		return db.User{}, false, fmt.Errorf("%w: name is required", ErrInvalidRequest)
	}
	if surname == "" {
		surname = "-"
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			return db.User{}, false, fmt.Errorf("%w: email is invalid", ErrInvalidRequest)
		}
		if _, err := q.GetUserByEmail(ctx, pgtype.Text{String: email, Valid: true}); err == nil {
			return db.User{}, false, fmt.Errorf("%w: email already registered", ErrConflict)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, false, err
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return db.User{}, false, err
	}
	hash, err := password.Hash(base64.RawURLEncoding.EncodeToString(raw))
	if err != nil {
		return db.User{}, false, err
	}
	u, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: pgtype.Text{String: email, Valid: email != ""}, PasswordHash: hash,
		Name: name, Surname: surname, Status: "active", PhoneE164: pgtype.Text{String: phone, Valid: true},
	})
	if err != nil {
		return db.User{}, false, err
	}
	return u, false, nil
}

func teamMember(u db.User, existing bool, roles []string) TeamMember {
	out := TeamMember{
		UserUUID: u.Uuid, Name: u.Name, Surname: u.Surname, Phone: u.PhoneE164.String,
		RoleSlugs: roles, Warehouses: []uuid.UUID{}, Existing: existing,
	}
	if u.Email.Valid {
		e := u.Email.String
		out.Email = &e
	}
	return out
}
