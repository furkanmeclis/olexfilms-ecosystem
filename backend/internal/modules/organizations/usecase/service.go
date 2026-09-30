package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/slug"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound                  = errors.New("not found")
	ErrConflict                  = errors.New("conflict")
	ErrInvalidRequest            = errors.New("invalid request")
	ErrNoTenantMembership        = errors.New("no tenant membership")
	ErrOrganizationAccessExpired = errors.New("organization access expired")
	ErrOrganizationSuspended     = errors.New("organization suspended")
)

var reservedSlugs = map[string]struct{}{
	"platform": {}, "api": {}, "share": {}, "t": {}, "register": {},
	"login": {}, "admin": {}, "health": {}, "forbidden": {}, "unauthorized": {},
}

const trialDays = 14

// Service manages organizations and memberships.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New creates an organizations service.
func New(pool *pgxpool.Pool, q *db.Queries) *Service {
	return &Service{pool: pool, q: q}
}

// Organization is the public organization projection.
type Organization struct {
	UUID           uuid.UUID  `json:"uuid"`
	Slug           string     `json:"slug"`
	Name           string     `json:"name"`
	City           string     `json:"city"`
	District       string     `json:"district"`
	Phone          string     `json:"phone"`
	Address        string     `json:"address"`
	Status         string     `json:"status"`
	PlanCode       *string    `json:"plan_code,omitempty"`
	AccessStartsAt time.Time  `json:"access_starts_at"`
	AccessEndsAt   *time.Time `json:"access_ends_at,omitempty"`
	LogoURL        *string    `json:"logo_url,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// PublicOrganization is branding-safe subset for login pages.
type PublicOrganization struct {
	UUID     uuid.UUID `json:"uuid"`
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	LogoURL  *string   `json:"logo_url,omitempty"`
	AccessOK bool      `json:"access_ok"`
}

// MembershipSummary is a user's organization membership.
type MembershipSummary struct {
	UUID         uuid.UUID  `json:"uuid"`
	Slug         string     `json:"slug"`
	Name         string     `json:"name"`
	Role         string     `json:"role"`
	LogoURL      *string    `json:"logo_url,omitempty"`
	Status       string     `json:"status"`
	AccessEndsAt *time.Time `json:"access_ends_at,omitempty"`
}

// RegisterInput is public business self-registration.
type RegisterInput struct {
	Name             string
	Surname          string
	Email            string
	Password         string
	OrganizationName string
	City             string
	District         string
	Phone            string
	Address          string
}

// RegisterResult is created org + owner user id after signup.
type RegisterResult struct {
	Organization Organization
	OwnerUserID  int64
	OwnerUUID    uuid.UUID
}

// PatchInput updates organization fields from platform admin.
type PatchInput struct {
	Name           *string
	City           *string
	District       *string
	Phone          *string
	Address        *string
	Status         *string
	PlanCode       *string
	AccessStartsAt *time.Time
	AccessEndsAt   *time.Time
	ClearAccessEnd bool
}

// AddMemberInput assigns an existing user to an organization.
type AddMemberInput struct {
	UserUUID uuid.UUID
	Role     string
}

func mapOrganization(row db.Organization) Organization {
	var plan *string
	if row.PlanCode.Valid && row.PlanCode.String != "" {
		s := row.PlanCode.String
		plan = &s
	}
	var accessEnds *time.Time
	if row.AccessEndsAt.Valid {
		t := row.AccessEndsAt.Time
		accessEnds = &t
	}
	return Organization{
		UUID: row.Uuid, Slug: row.Slug, Name: row.Name,
		City: row.City, District: row.District, Phone: row.Phone, Address: row.Address,
		Status: row.Status, PlanCode: plan,
		AccessStartsAt: row.AccessStartsAt.Time,
		AccessEndsAt:   accessEnds,
		LogoURL:        logoURL(row.Uuid, row.LogoObjectKey),
		CreatedAt:      row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func logoURL(orgUUID uuid.UUID, key pgtype.Text) *string {
	if !key.Valid || strings.TrimSpace(key.String) == "" {
		return nil
	}
	u := fmt.Sprintf("/v1/public/organizations/logo/%s", orgUUID.String())
	return &u
}

func (s *Service) allocateSlug(ctx context.Context, name string) (string, error) {
	base := slug.FromName(name)
	if base == "" {
		base = "isletme"
	}
	if _, reserved := reservedSlugs[base]; reserved {
		base = base + "-isletme"
	}
	candidate := base
	for i := 0; i < 100; i++ {
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		exists, err := s.q.SlugExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: could not allocate slug", ErrConflict)
}

// Register creates user, organization, and owner membership in one transaction.
func (s *Service) Register(ctx context.Context, in RegisterInput) (RegisterResult, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Surname = strings.TrimSpace(in.Surname)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.OrganizationName = strings.TrimSpace(in.OrganizationName)
	if in.Name == "" || in.Surname == "" || in.Email == "" {
		return RegisterResult{}, fmt.Errorf("%w: name, surname, and email are required", ErrInvalidRequest)
	}
	if in.OrganizationName == "" {
		return RegisterResult{}, fmt.Errorf("%w: organization_name is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(in.Password) == "" {
		return RegisterResult{}, fmt.Errorf("%w: password is required", ErrInvalidRequest)
	}
	if _, err := s.q.GetUserByEmail(ctx, in.Email); err == nil {
		return RegisterResult{}, fmt.Errorf("%w: email already registered", ErrConflict)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return RegisterResult{}, err
	}
	hash, err := password.Hash(in.Password)
	if err != nil {
		return RegisterResult{}, err
	}
	orgSlug, err := s.allocateSlug(ctx, in.OrganizationName)
	if err != nil {
		return RegisterResult{}, err
	}
	now := time.Now().UTC()
	trialEnd := now.Add(trialDays * 24 * time.Hour)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RegisterResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	user, err := qtx.CreateUser(ctx, db.CreateUserParams{
		Email: in.Email, PasswordHash: hash, Name: in.Name, Surname: in.Surname,
		Status: "active", EmailVerifiedAt: pgtype.Timestamptz{},
	})
	if err != nil {
		return RegisterResult{}, err
	}
	org, err := qtx.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: orgSlug, Name: in.OrganizationName,
		City: strings.TrimSpace(in.City), District: strings.TrimSpace(in.District),
		Phone: strings.TrimSpace(in.Phone), Address: strings.TrimSpace(in.Address),
		Status: "active", PlanCode: pgtype.Text{String: "trial", Valid: true},
		AccessStartsAt: pgtype.Timestamptz{Time: now, Valid: true},
		AccessEndsAt:   pgtype.Timestamptz{Time: trialEnd, Valid: true},
	})
	if err != nil {
		return RegisterResult{}, err
	}
	if _, err := qtx.CreateOrganizationMember(ctx, db.CreateOrganizationMemberParams{
		OrganizationID: org.ID, UserID: user.ID, Role: "owner",
	}); err != nil {
		return RegisterResult{}, err
	}
	if err := qtx.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{
		UserID: user.ID, Slug: rbac.RoleOrganizationUser,
	}); err != nil {
		return RegisterResult{}, err
	}
	if err := qtx.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{
		UserID: user.ID, Slug: rbac.RoleOrganizationOwner,
	}); err != nil {
		return RegisterResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{
		Organization: mapOrganization(org),
		OwnerUserID:  user.ID,
		OwnerUUID:    user.Uuid,
	}, nil
}

// RegisterOrganization attaches a new organization to an existing user.
func (s *Service) RegisterOrganization(ctx context.Context, in RegisterInput, ownerUserID int64) (RegisterResult, error) {
	in.OrganizationName = strings.TrimSpace(in.OrganizationName)
	if in.OrganizationName == "" {
		return RegisterResult{}, fmt.Errorf("%w: organization_name is required", ErrInvalidRequest)
	}
	orgSlug, err := s.allocateSlug(ctx, in.OrganizationName)
	if err != nil {
		return RegisterResult{}, err
	}
	now := time.Now().UTC()
	trialEnd := now.Add(trialDays * 24 * time.Hour)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RegisterResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	org, err := qtx.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: orgSlug, Name: in.OrganizationName,
		City: strings.TrimSpace(in.City), District: strings.TrimSpace(in.District),
		Phone: strings.TrimSpace(in.Phone), Address: strings.TrimSpace(in.Address),
		Status: "active", PlanCode: pgtype.Text{String: "trial", Valid: true},
		AccessStartsAt: pgtype.Timestamptz{Time: now, Valid: true},
		AccessEndsAt:   pgtype.Timestamptz{Time: trialEnd, Valid: true},
	})
	if err != nil {
		return RegisterResult{}, err
	}
	if _, err := qtx.CreateOrganizationMember(ctx, db.CreateOrganizationMemberParams{
		OrganizationID: org.ID, UserID: ownerUserID, Role: "owner",
	}); err != nil {
		return RegisterResult{}, err
	}
	if err := qtx.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{
		UserID: ownerUserID, Slug: rbac.RoleOrganizationUser,
	}); err != nil {
		return RegisterResult{}, err
	}
	if err := qtx.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{
		UserID: ownerUserID, Slug: rbac.RoleOrganizationOwner,
	}); err != nil {
		return RegisterResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{Organization: mapOrganization(org)}, nil
}

// GetPublicBySlug returns branding info for tenant login.
func (s *Service) GetPublicBySlug(ctx context.Context, slugValue string) (PublicOrganization, error) {
	row, err := s.q.GetOrganizationBySlug(ctx, strings.TrimSpace(slugValue))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublicOrganization{}, ErrNotFound
		}
		return PublicOrganization{}, err
	}
	return PublicOrganization{
		UUID: row.Uuid, Slug: row.Slug, Name: row.Name, Status: row.Status,
		LogoURL: logoURL(row.Uuid, row.LogoObjectKey), AccessOK: accessAllowed(row),
	}, nil
}

// GetByUUID returns organization detail.
func (s *Service) GetByUUID(ctx context.Context, id uuid.UUID) (Organization, error) {
	row, err := s.q.GetOrganizationByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Organization{}, ErrNotFound
		}
		return Organization{}, err
	}
	return mapOrganization(row), nil
}

// List returns paginated organizations for platform admin.
func (s *Service) List(ctx context.Context, limit, offset int32, q, status string) ([]Organization, int64, error) {
	var statusArg pgtype.Text
	if status != "" {
		statusArg = pgtype.Text{String: status, Valid: true}
	}
	var qArg pgtype.Text
	if q != "" {
		qArg = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListOrganizationsFiltered(ctx, db.ListOrganizationsFilteredParams{
		Status: statusArg, Q: qArg, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountOrganizations(ctx, db.CountOrganizationsParams{Status: statusArg, Q: qArg})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapOrganization(row))
	}
	return out, total, nil
}

// Patch updates organization from platform admin.
func (s *Service) Patch(ctx context.Context, id uuid.UUID, in PatchInput) (Organization, error) {
	params := db.UpdateOrganizationPlatformParams{Uuid: id}
	if in.Name != nil {
		params.Name = pgtype.Text{String: strings.TrimSpace(*in.Name), Valid: true}
	}
	if in.City != nil {
		params.City = pgtype.Text{String: strings.TrimSpace(*in.City), Valid: true}
	}
	if in.District != nil {
		params.District = pgtype.Text{String: strings.TrimSpace(*in.District), Valid: true}
	}
	if in.Phone != nil {
		params.Phone = pgtype.Text{String: strings.TrimSpace(*in.Phone), Valid: true}
	}
	if in.Address != nil {
		params.Address = pgtype.Text{String: strings.TrimSpace(*in.Address), Valid: true}
	}
	if in.Status != nil {
		params.Status = pgtype.Text{String: strings.TrimSpace(*in.Status), Valid: true}
	}
	if in.PlanCode != nil {
		params.PlanCode = pgtype.Text{String: strings.TrimSpace(*in.PlanCode), Valid: true}
	}
	if in.AccessStartsAt != nil {
		params.AccessStartsAt = pgtype.Timestamptz{Time: *in.AccessStartsAt, Valid: true}
	}
	if in.ClearAccessEnd {
		params.AccessEndsAt = pgtype.Timestamptz{Valid: false}
	} else if in.AccessEndsAt != nil {
		params.AccessEndsAt = pgtype.Timestamptz{Time: *in.AccessEndsAt, Valid: true}
	}
	row, err := s.q.UpdateOrganizationPlatform(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Organization{}, ErrNotFound
		}
		return Organization{}, err
	}
	return mapOrganization(row), nil
}

// SetLogo stores logo object key.
func (s *Service) SetLogo(ctx context.Context, id uuid.UUID, objectKey string) (Organization, error) {
	row, err := s.q.SetOrganizationLogo(ctx, db.SetOrganizationLogoParams{
		Uuid: id, LogoObjectKey: pgtype.Text{String: objectKey, Valid: objectKey != ""},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Organization{}, ErrNotFound
		}
		return Organization{}, err
	}
	return mapOrganization(row), nil
}

// ClearLogo removes organization logo.
func (s *Service) ClearLogo(ctx context.Context, id uuid.UUID) (Organization, error) {
	row, err := s.q.ClearOrganizationLogo(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Organization{}, ErrNotFound
		}
		return Organization{}, err
	}
	return mapOrganization(row), nil
}

// LogoObjectKey returns stored logo key.
func (s *Service) LogoObjectKey(ctx context.Context, id uuid.UUID) (string, error) {
	row, err := s.q.GetOrganizationByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if !row.LogoObjectKey.Valid || row.LogoObjectKey.String == "" {
		return "", ErrNotFound
	}
	return row.LogoObjectKey.String, nil
}

// ListMembershipsForUser lists organizations for session hydration.
func (s *Service) ListMembershipsForUser(ctx context.Context, userID int64) ([]MembershipSummary, error) {
	rows, err := s.q.ListOrganizationMembersByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]MembershipSummary, 0, len(rows))
	for _, row := range rows {
		var accessEnds *time.Time
		if row.AccessEndsAt.Valid {
			t := row.AccessEndsAt.Time
			accessEnds = &t
		}
		out = append(out, MembershipSummary{
			UUID: row.Uuid, Slug: row.Slug, Name: row.Name, Role: row.Role,
			LogoURL: logoURL(row.Uuid, row.LogoObjectKey), Status: row.Status,
			AccessEndsAt: accessEnds,
		})
	}
	return out, nil
}

// ResolveLoginOrganization validates slug membership and access for login.
func (s *Service) ResolveLoginOrganization(ctx context.Context, userID int64, slugValue string) (uuid.UUID, error) {
	row, err := s.q.GetOrganizationMemberByUserAndSlug(ctx, db.GetOrganizationMemberByUserAndSlugParams{
		UserID: userID, Slug: strings.TrimSpace(slugValue),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNoTenantMembership
		}
		return uuid.Nil, err
	}
	org := db.Organization{
		Status: row.OrganizationStatus, AccessStartsAt: row.AccessStartsAt, AccessEndsAt: row.AccessEndsAt,
	}
	if row.OrganizationStatus == "suspended" {
		return uuid.Nil, ErrOrganizationSuspended
	}
	if !accessAllowed(org) {
		return uuid.Nil, ErrOrganizationAccessExpired
	}
	return row.OrganizationUuid, nil
}

// ResolveOrganizationUUID re-validates membership + access for a known organization UUID
// (used when preserving oid across refresh).
func (s *Service) ResolveOrganizationUUID(ctx context.Context, userID int64, orgUUID uuid.UUID) (uuid.UUID, error) {
	row, err := s.q.GetOrganizationMemberByUserAndOrgUUID(ctx, db.GetOrganizationMemberByUserAndOrgUUIDParams{
		UserID: userID, Uuid: orgUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNoTenantMembership
		}
		return uuid.Nil, err
	}
	org := db.Organization{
		Status: row.OrganizationStatus, AccessStartsAt: row.AccessStartsAt, AccessEndsAt: row.AccessEndsAt,
	}
	if row.OrganizationStatus == "suspended" {
		return uuid.Nil, ErrOrganizationSuspended
	}
	if !accessAllowed(org) {
		return uuid.Nil, ErrOrganizationAccessExpired
	}
	return row.OrganizationUuid, nil
}

// AddMember assigns a user to an organization (platform only).
func (s *Service) AddMember(ctx context.Context, orgUUID uuid.UUID, in AddMemberInput) error {
	org, err := s.q.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	user, err := s.q.GetUserByUUID(ctx, in.UserUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: user not found", ErrNotFound)
		}
		return err
	}
	role := strings.TrimSpace(in.Role)
	if role == "" {
		role = "staff"
	}
	if role != "owner" && role != "staff" {
		return fmt.Errorf("%w: invalid role", ErrInvalidRequest)
	}
	_, err = s.q.CreateOrganizationMember(ctx, db.CreateOrganizationMemberParams{
		OrganizationID: org.ID, UserID: user.ID, Role: role,
	})
	if err != nil {
		if strings.Contains(err.Error(), "uq_organization_members") {
			return ErrConflict
		}
		return err
	}
	if err := s.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{
		UserID: user.ID, Slug: rbac.RoleOrganizationUser,
	}); err != nil {
		return err
	}
	if role == "owner" {
		if err := s.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{
			UserID: user.ID, Slug: rbac.RoleOrganizationOwner,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ListMembers lists organization members.
func (s *Service) ListMembers(ctx context.Context, orgUUID uuid.UUID) ([]db.ListOrganizationMembersRow, error) {
	org, err := s.q.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.q.ListOrganizationMembers(ctx, org.ID)
}

func accessAllowed(row db.Organization) bool {
	if row.Status == "suspended" || row.Status == "expired" {
		return false
	}
	now := time.Now().UTC()
	if row.AccessStartsAt.Valid && row.AccessStartsAt.Time.After(now) {
		return false
	}
	if row.AccessEndsAt.Valid && !row.AccessEndsAt.Time.After(now) {
		return false
	}
	return true
}
