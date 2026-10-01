package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
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
	geo  GeoResolver
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
	// Structured address (TEC-84); city/district above are the display copy.
	CountryID  *int64 `json:"country_id,omitempty"`
	ProvinceID *int64 `json:"province_id,omitempty"`
	DistrictID *int64 `json:"district_id,omitempty"`
	// Type is center, distributor or dealer.
	Type     string     `json:"type"`
	Brand    BrandRef   `json:"brand"`
	Parent   *ParentRef `json:"parent,omitempty"`
	Currency string     `json:"currency"`
	Locale   string     `json:"locale"`
	Timezone string     `json:"timezone"`
	// ContractValidUntil is a calendar date (YYYY-MM-DD).
	ContractValidUntil *string        `json:"contract_valid_until,omitempty"`
	Settings           map[string]any `json:"settings"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
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
	Type         string     `json:"type"`
	Brand        BrandRef   `json:"brand"`
	Parent       *ParentRef `json:"parent,omitempty"`
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
	// Structured address (TEC-84). A dealer without an explicit parent is
	// placed under the distributor whose territory covers it (K5).
	Location AddressInput
	// Platform create only. Type defaults to dealer; ParentUUID defaults to
	// the brand center.
	Type                string
	ParentUUID          *uuid.UUID
	RegisterAsWarehouse bool
	Currency            string
	Locale              string
	Timezone            string
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
	Currency       *string
	Locale         *string
	Timezone       *string
	// ParentUUID moves the organization in the tree (platform only, K25).
	ParentUUID *uuid.UUID
	// Location replaces the structured address when set; the parent does not
	// follow (only the platform admin changes a distributor, K25).
	Location *AddressInput
}

// AddMemberInput assigns an existing user to an organization.
type AddMemberInput struct {
	UserUUID uuid.UUID
	Role     string
	// RoleSlugs are optional organization roles (e.g. dealer_accounting);
	// empty means the default role of the org type and Role.
	RoleSlugs []string
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
		Type:           row.Type,
		CountryID:      int8Ptr(row.CountryID), ProvinceID: int8Ptr(row.ProvinceID), DistrictID: int8Ptr(row.DistrictID),
		Currency: row.Currency, Locale: row.Locale, Timezone: row.Timezone,
		ContractValidUntil: contractDate(row.ContractValidUntil),
		Settings:           decodeSettings(row.Settings),
		CreatedAt:          row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func contractDate(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	v := d.Time.Format("2006-01-02")
	return &v
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
	brand, err := RequestBrand(ctx)
	if err != nil {
		return RegisterResult{}, err
	}
	if !brand.Active() {
		return RegisterResult{}, ErrBrandInactive
	}
	center, err := s.q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("brand center: %w", err)
	}
	addr, err := s.address(ctx, in.Location)
	if err != nil {
		return RegisterResult{}, err
	}
	orgPhone, err := normalizePhone(in.Phone, addressISO2(addr))
	if err != nil {
		return RegisterResult{}, err
	}
	parent := center
	if dist, err := s.territoryParent(ctx, brand.ID, addr); err != nil {
		return RegisterResult{}, err
	} else if dist != nil {
		parent = *dist
	}
	if _, err := s.q.GetUserByEmail(ctx, pgtype.Text{String: in.Email, Valid: true}); err == nil {
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
		Email: pgtype.Text{String: in.Email, Valid: true}, PasswordHash: hash, Name: in.Name, Surname: in.Surname,
		Status: "active", EmailVerifiedAt: pgtype.Timestamptz{},
	})
	if err != nil {
		return RegisterResult{}, err
	}
	city, district := addressText(addr, in.City, in.District)
	countryID, provinceID, districtID := addressIDs(addr)
	org, err := qtx.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: orgSlug, Name: in.OrganizationName,
		City: city, District: district,
		Phone: orgPhone, Address: strings.TrimSpace(in.Address),
		Status: "active", PlanCode: pgtype.Text{String: "trial", Valid: true},
		AccessStartsAt: pgtype.Timestamptz{Time: now, Valid: true},
		AccessEndsAt:   pgtype.Timestamptz{Time: trialEnd, Valid: true},
		Type:           TypeDealer,
		ParentID:       pgtype.Int8{Int64: parent.ID, Valid: true},
		BrandID:        brand.ID,
		Currency:       parent.Currency, Locale: parent.Locale, Timezone: parent.Timezone,
		Settings:  []byte("{}"),
		CountryID: countryID, ProvinceID: provinceID, DistrictID: districtID,
	})
	if err != nil {
		return RegisterResult{}, err
	}
	if err := addMember(ctx, qtx, org, user.ID, "owner", nil); err != nil {
		return RegisterResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{
		Organization: withBrandParent(mapOrganization(org), brand.Slug, parent),
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
	place, err := s.placement(ctx, in)
	if err != nil {
		return RegisterResult{}, err
	}
	orgPhone, err := normalizePhone(in.Phone, addressISO2(place.Address))
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
	city, district := addressText(place.Address, in.City, in.District)
	countryID, provinceID, districtID := addressIDs(place.Address)
	org, err := qtx.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: orgSlug, Name: in.OrganizationName,
		City: city, District: district,
		CountryID: countryID, ProvinceID: provinceID, DistrictID: districtID,
		Phone: orgPhone, Address: strings.TrimSpace(in.Address),
		Status: "active", PlanCode: pgtype.Text{String: "trial", Valid: true},
		AccessStartsAt: pgtype.Timestamptz{Time: now, Valid: true},
		AccessEndsAt:   pgtype.Timestamptz{Time: trialEnd, Valid: true},
		Type:           place.Type,
		ParentID:       pgtype.Int8{Int64: place.Parent.ID, Valid: true},
		BrandID:        place.Parent.BrandID,
		Currency:       place.Currency, Locale: place.Locale, Timezone: place.Timezone,
		Settings: place.Settings,
	})
	if err != nil {
		return RegisterResult{}, err
	}
	if err := addMember(ctx, qtx, org, ownerUserID, "owner", nil); err != nil {
		return RegisterResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{Organization: withBrandParent(mapOrganization(org), place.BrandSlug, place.Parent)}, nil
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
	if brand, ok := brandctx.From(ctx); !ok || brand.ID != row.BrandID {
		return PublicOrganization{}, ErrNotFound
	}
	return PublicOrganization{
		UUID: row.Uuid, Slug: row.Slug, Name: row.Name, Status: row.Status,
		LogoURL: logoURL(row.Uuid, row.LogoObjectKey), AccessOK: accessAllowed(row),
	}, nil
}

// GetByUUID returns organization detail within the request brand.
func (s *Service) GetByUUID(ctx context.Context, id uuid.UUID) (Organization, error) {
	row, err := s.brandOrg(ctx, id)
	if err != nil {
		return Organization{}, err
	}
	return mapTree(row.Organization, row.BrandSlug, row.ParentUuid, row.ParentName), nil
}

// EnsureInBrand returns ErrNotFound when the organization is outside the request brand.
func (s *Service) EnsureInBrand(ctx context.Context, id uuid.UUID) error {
	_, err := s.brandOrg(ctx, id)
	return err
}

// ListFilter narrows the platform organization list.
type ListFilter struct {
	Q          string
	Status     string
	Type       string
	ParentUUID *uuid.UUID
}

// List returns paginated organizations of the request brand for platform admin.
func (s *Service) List(ctx context.Context, limit, offset int32, f ListFilter) ([]Organization, int64, error) {
	brand, err := RequestBrand(ctx)
	if err != nil {
		return nil, 0, err
	}
	q, status := f.Q, f.Status
	brandArg := pgtype.Int8{Int64: brand.ID, Valid: true}
	var typeArg pgtype.Text
	if f.Type != "" {
		typeArg = pgtype.Text{String: f.Type, Valid: true}
	}
	var parentArg pgtype.Int8
	if f.ParentUUID != nil {
		parent, err := s.brandOrg(ctx, *f.ParentUUID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return []Organization{}, 0, nil
			}
			return nil, 0, err
		}
		parentArg = pgtype.Int8{Int64: parent.Organization.ID, Valid: true}
	}
	var statusArg pgtype.Text
	if status != "" {
		statusArg = pgtype.Text{String: status, Valid: true}
	}
	var qArg pgtype.Text
	if q != "" {
		qArg = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListOrganizationsFiltered(ctx, db.ListOrganizationsFilteredParams{
		Status: statusArg, BrandID: brandArg, Type: typeArg, ParentID: parentArg,
		Q: qArg, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountOrganizations(ctx, db.CountOrganizationsParams{
		Status: statusArg, BrandID: brandArg, Type: typeArg, ParentID: parentArg, Q: qArg,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapTree(row.Organization, row.BrandSlug, row.ParentUuid, row.ParentName))
	}
	return out, total, nil
}

// Patch updates organization from platform admin.
func (s *Service) Patch(ctx context.Context, id uuid.UUID, in PatchInput) (Organization, error) {
	current, err := s.brandOrg(ctx, id)
	if err != nil {
		return Organization{}, err
	}
	if in.Status != nil {
		if _, ok := validStatuses[strings.TrimSpace(*in.Status)]; !ok {
			return Organization{}, fmt.Errorf("%w: invalid status", ErrInvalidRequest)
		}
	}
	params := db.UpdateOrganizationPlatformParams{Uuid: id}
	if in.Currency != nil {
		c := strings.ToUpper(strings.TrimSpace(*in.Currency))
		if !validCurrency(c) {
			return Organization{}, fmt.Errorf("%w: currency must be an ISO 4217 code", ErrInvalidRequest)
		}
		params.Currency = pgtype.Text{String: c, Valid: true}
	}
	if in.Locale != nil && strings.TrimSpace(*in.Locale) != "" {
		l, err := parseOrgLocale(*in.Locale)
		if err != nil {
			return Organization{}, err
		}
		params.Locale = pgtype.Text{String: l, Valid: true}
	}
	if in.Timezone != nil && strings.TrimSpace(*in.Timezone) != "" {
		tz := strings.TrimSpace(*in.Timezone)
		if _, err := time.LoadLocation(tz); err != nil {
			return Organization{}, fmt.Errorf("%w: invalid timezone", ErrInvalidRequest)
		}
		params.Timezone = pgtype.Text{String: tz, Valid: true}
	}
	if in.Name != nil {
		params.Name = pgtype.Text{String: strings.TrimSpace(*in.Name), Valid: true}
	}
	if in.City != nil {
		params.City = pgtype.Text{String: strings.TrimSpace(*in.City), Valid: true}
	}
	if in.District != nil {
		params.District = pgtype.Text{String: strings.TrimSpace(*in.District), Valid: true}
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
	var phoneISO2 string
	var phoneISO2Set bool
	if in.Location != nil {
		addr, err := s.address(ctx, *in.Location)
		if err != nil {
			return Organization{}, err
		}
		params.SetAddress = true
		params.CountryID, params.ProvinceID, params.DistrictID = addressIDs(addr)
		phoneISO2, phoneISO2Set = addressISO2(addr), true
	}
	if in.Phone != nil {
		// K29: parsed with the new country when the address changes too.
		if !phoneISO2Set {
			if phoneISO2, err = s.orgCountryISO2(ctx, current.Organization.ID); err != nil {
				return Organization{}, err
			}
		}
		p, err := normalizePhone(*in.Phone, phoneISO2)
		if err != nil {
			return Organization{}, err
		}
		params.Phone = pgtype.Text{String: p, Valid: true}
	}
	if in.ParentUUID != nil {
		if err := s.ChangeParent(ctx, id, *in.ParentUUID); err != nil {
			return Organization{}, err
		}
	}
	if _, err := s.q.UpdateOrganizationPlatform(ctx, params); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Organization{}, ErrNotFound
		}
		return Organization{}, err
	}
	return s.GetByUUID(ctx, id)
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
	// Only memberships of the request brand are visible (K1). Without a
	// resolved brand nothing is listed (fail closed).
	brand, ok := brandctx.From(ctx)
	if !ok {
		return []MembershipSummary{}, nil
	}
	rows, err := s.q.ListOrganizationMembersByUserID(ctx, db.ListOrganizationMembersByUserIDParams{
		UserID: userID, BrandID: pgtype.Int8{Int64: brand.ID, Valid: true},
	})
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
		var parent *ParentRef
		if row.ParentUuid.Valid {
			parent = &ParentRef{UUID: uuid.UUID(row.ParentUuid.Bytes), Slug: row.ParentSlug.String, Name: row.ParentName.String}
		}
		out = append(out, MembershipSummary{
			UUID: row.Uuid, Slug: row.Slug, Name: row.Name, Role: row.Role,
			LogoURL: logoURL(row.Uuid, row.LogoObjectKey), Status: row.Status,
			AccessEndsAt: accessEnds,
			Type:         row.Type,
			Brand:        BrandRef{Slug: row.BrandSlug, Name: row.BrandName},
			Parent:       parent,
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
	if err := checkBrand(ctx, row.OrganizationBrandID); err != nil {
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
	if err := checkBrand(ctx, row.OrganizationBrandID); err != nil {
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
	tree, err := s.brandOrg(ctx, orgUUID)
	if err != nil {
		return err
	}
	org := tree.Organization
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
	roleSlugs, err := validMemberRoles(org.Type, in.RoleSlugs)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := addMember(ctx, s.q.WithTx(tx), org, user.ID, role, roleSlugs); err != nil {
		if strings.Contains(err.Error(), "uq_organization_members") {
			return ErrConflict
		}
		return err
	}
	return tx.Commit(ctx)
}

// validMemberRoles checks that explicit membership roles are system roles of
// the organization's type (a dealer member cannot get a center role).
func validMemberRoles(orgType string, slugs []string) ([]string, error) {
	out := make([]string, 0, len(slugs))
	seen := map[string]struct{}{}
	for _, raw := range slugs {
		slug := strings.TrimSpace(raw)
		if slug == "" {
			continue
		}
		def, ok := rbac.RoleBySlug(slug)
		if !ok || def.OrgType != orgType {
			return nil, fmt.Errorf("%w: role %q cannot be granted in a %s organization", ErrInvalidRequest, slug, orgType)
		}
		if _, dup := seen[slug]; dup {
			continue
		}
		seen[slug] = struct{}{}
		out = append(out, slug)
	}
	return out, nil
}

// addMember creates a membership and grants its organization roles. Without
// explicit roles the default role of the org type and membership kind is used.
func addMember(ctx context.Context, q *db.Queries, org db.Organization, userID int64, memberRole string, roleSlugs []string) error {
	member, err := q.CreateOrganizationMember(ctx, db.CreateOrganizationMemberParams{
		OrganizationID: org.ID, UserID: userID, Role: memberRole,
	})
	if err != nil {
		return err
	}
	if len(roleSlugs) == 0 {
		roleSlugs = []string{rbac.DefaultMemberRole(org.Type, memberRole)}
	}
	for _, slug := range roleSlugs {
		if err := q.AssignMemberRoleBySlug(ctx, db.AssignMemberRoleBySlugParams{MemberID: member.ID, Slug: slug}); err != nil {
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
