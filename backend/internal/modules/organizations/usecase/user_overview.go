package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserOverview is the platform "user 360" (TEC-1036): every organization
// the user belongs to (with roles and warehouses), global roles, the
// customer side and active sessions in one read.
type UserOverview struct {
	UserUUID              uuid.UUID                 `json:"user_uuid"`
	LastLoginAt           *time.Time                `json:"last_login_at"`
	GlobalRoles           []string                  `json:"global_roles"`
	Memberships           []UserOverviewMembership  `json:"memberships"`
	Customer              UserOverviewCustomer      `json:"customer"`
	CustomerOrganizations []UserOverviewCustomerOrg `json:"customer_organizations"`
	Sessions              []UserOverviewSession     `json:"sessions"`
}

// UserOverviewMembership is one organization membership.
type UserOverviewMembership struct {
	Organization UserOverviewOrganization `json:"organization"`
	Role         string                   `json:"role"`
	RoleSlugs    []string                 `json:"role_slugs"`
	Warehouses   []UserOverviewWarehouse  `json:"warehouses"`
	CreatedAt    *time.Time               `json:"created_at"`
}

// UserOverviewOrganization is the organization of a membership.
type UserOverviewOrganization struct {
	UUID       uuid.UUID `json:"uuid"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	BrandSlug  string    `json:"brand_slug"`
	ParentName *string   `json:"parent_name"`
}

// UserOverviewWarehouse is a warehouse membership (TEC-769).
type UserOverviewWarehouse struct {
	UUID      uuid.UUID `json:"uuid"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	IsDefault bool      `json:"is_default"`
}

// UserOverviewCustomer is the customer side of the user (K11).
type UserOverviewCustomer struct {
	HasProfile   bool    `json:"has_profile"`
	ProfileType  *string `json:"profile_type"`
	CompanyName  *string `json:"company_name"`
	VehicleCount int64   `json:"vehicle_count"`
}

// UserOverviewCustomerOrg is an organization the user is a customer of.
type UserOverviewCustomerOrg struct {
	UUID           uuid.UUID  `json:"uuid"`
	Name           string     `json:"name"`
	Type           string     `json:"type"`
	BrandSlug      string     `json:"brand_slug"`
	FirstServiceAt *time.Time `json:"first_service_at"`
	CreatedAt      *time.Time `json:"created_at"`
}

// UserOverviewSession is an active (unrevoked, unexpired) session.
type UserOverviewSession struct {
	UUID             uuid.UUID  `json:"uuid"`
	Realm            string     `json:"realm"`
	Client           string     `json:"client"`
	DeviceName       *string    `json:"device_name"`
	UserAgent        *string    `json:"user_agent"`
	IPAddress        *string    `json:"ip_address"`
	OrganizationName *string    `json:"organization_name"`
	Impersonated     bool       `json:"impersonated"`
	CreatedAt        *time.Time `json:"created_at"`
	ExpiresAt        *time.Time `json:"expires_at"`
}

// UserOverview builds the user 360 view (platform only; every brand).
func (s *Service) UserOverview(ctx context.Context, userUUID uuid.UUID) (UserOverview, error) {
	user, err := s.q.GetUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserOverview{}, ErrNotFound
		}
		return UserOverview{}, err
	}
	roles, err := s.q.ListUserRoleSlugs(ctx, user.ID)
	if err != nil {
		return UserOverview{}, err
	}
	out := UserOverview{
		UserUUID:              user.Uuid,
		LastLoginAt:           timePtr(user.LastLoginAt),
		GlobalRoles:           roles,
		Memberships:           []UserOverviewMembership{},
		CustomerOrganizations: []UserOverviewCustomerOrg{},
		Sessions:              []UserOverviewSession{},
	}

	memberships, err := s.q.UserOverviewMemberships(ctx, user.ID)
	if err != nil {
		return UserOverview{}, err
	}
	for _, row := range memberships {
		warehouses := []UserOverviewWarehouse{}
		if len(row.Warehouses) > 0 {
			if err := json.Unmarshal(row.Warehouses, &warehouses); err != nil {
				return UserOverview{}, err
			}
		}
		out.Memberships = append(out.Memberships, UserOverviewMembership{
			Organization: UserOverviewOrganization{
				UUID: row.OrganizationUuid, Name: row.OrganizationName, Slug: row.OrganizationSlug,
				Type: row.OrganizationType, Status: row.OrganizationStatus, BrandSlug: row.BrandSlug,
				ParentName: textPtr(row.ParentName),
			},
			Role:       row.Role,
			RoleSlugs:  row.RoleSlugs,
			Warehouses: warehouses,
			CreatedAt:  timePtr(row.CreatedAt),
		})
	}

	customer, err := s.q.UserOverviewCustomer(ctx, user.ID)
	if err != nil {
		return UserOverview{}, err
	}
	out.Customer = UserOverviewCustomer{
		HasProfile:   customer.HasProfile,
		ProfileType:  nonEmpty(customer.ProfileType),
		CompanyName:  nonEmpty(customer.CompanyName),
		VehicleCount: customer.VehicleCount,
	}
	customerOrgs, err := s.q.UserOverviewCustomerOrganizations(ctx, user.ID)
	if err != nil {
		return UserOverview{}, err
	}
	for _, row := range customerOrgs {
		out.CustomerOrganizations = append(out.CustomerOrganizations, UserOverviewCustomerOrg{
			UUID: row.OrganizationUuid, Name: row.OrganizationName, Type: row.OrganizationType,
			BrandSlug: row.BrandSlug, FirstServiceAt: timePtr(row.FirstServiceAt), CreatedAt: timePtr(row.CreatedAt),
		})
	}

	sessions, err := s.q.UserOverviewActiveSessions(ctx, user.ID)
	if err != nil {
		return UserOverview{}, err
	}
	for _, row := range sessions {
		var ip *string
		if row.IpAddress != nil {
			v := row.IpAddress.String()
			ip = &v
		}
		out.Sessions = append(out.Sessions, UserOverviewSession{
			UUID: row.Uuid, Realm: row.Realm, Client: row.Client,
			DeviceName: textPtr(row.DeviceName), UserAgent: textPtr(row.UserAgent), IPAddress: ip,
			OrganizationName: textPtr(row.OrganizationName), Impersonated: row.Impersonated,
			CreatedAt: timePtr(row.CreatedAt), ExpiresAt: timePtr(row.ExpiresAt),
		})
	}
	return out, nil
}

func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func textPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func nonEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
