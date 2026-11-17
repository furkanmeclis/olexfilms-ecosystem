package usecase

// TEC-1216: tenant detail summaries for the organization and the staff
// user detail pages (web TEC-1217 / TEC-1218, mobile TEC-1219). Every
// relation is a refs.Ref; a counter is present only when the caller holds
// the read permission of its module (nil otherwise), so a summary never
// reveals more than the module lists would.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/refs"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PermissionChecker reports whether the caller holds a permission (any
// scope).
type PermissionChecker func(slug string) bool

// OrganizationSummary is GET /v1/tenant/organizations/{uuid}/summary.
type OrganizationSummary struct {
	Organization Organization       `json:"organization"`
	Ref          refs.Ref           `json:"ref"`
	Parent       *refs.Ref          `json:"parent"`
	Counts       OrganizationCounts `json:"counts"`
}

// OrganizationCounts are the counters of an organization; nil = the
// caller may not read that module.
type OrganizationCounts struct {
	Members    *int64 `json:"members"`
	Children   *int64 `json:"children"`
	Customers  *int64 `json:"customers"`
	Services   *int64 `json:"services"`
	Warranties *int64 `json:"warranties"`
	Orders     *int64 `json:"orders"`
	OpenTasks  *int64 `json:"open_tasks"`
	Leads      *int64 `json:"leads"`
	Warehouses *int64 `json:"warehouses"`
}

func gated(has PermissionChecker, perm string, v int64) *int64 {
	if has == nil || !has(perm) {
		return nil
	}
	return &v
}

// OrganizationSummaryInScope returns the summary of an organization inside
// the caller's organizations.read scope (ErrNotFound outside it).
func (s *Service) OrganizationSummaryInScope(ctx context.Context, f scopefilter.Filter, id uuid.UUID, has PermissionChecker) (OrganizationSummary, error) {
	row, err := s.brandOrg(ctx, id)
	if err != nil {
		return OrganizationSummary{}, err
	}
	o := row.Organization
	if !f.AllowsOrg(o.ID, o.BrandID) {
		return OrganizationSummary{}, ErrNotFound
	}
	c, err := s.q.OrganizationSummaryCounts(ctx, o.ID)
	if err != nil {
		return OrganizationSummary{}, fmt.Errorf("organizations: summary counts: %w", err)
	}
	out := OrganizationSummary{
		Organization: mapTree(o, row.BrandSlug, row.ParentUuid, row.ParentName),
		Ref:          refs.OrgRef(o.Uuid, o.Name, o.Type),
		Counts: OrganizationCounts{
			Members:    gated(has, rbac.PermMembersRead, c.Members),
			Children:   gated(has, rbac.PermOrganizationsRead, c.Children),
			Customers:  gated(has, rbac.PermCustomersRead, c.Customers),
			Services:   gated(has, rbac.PermServicesRead, c.Services),
			Warranties: gated(has, rbac.PermWarrantiesRead, c.Warranties),
			Orders:     gated(has, rbac.PermOrdersRead, c.Orders),
			OpenTasks:  gated(has, rbac.PermTasksRead, c.OpenTasks),
			Leads:      gated(has, rbac.PermLeadsRead, c.Leads),
			Warehouses: gated(has, rbac.PermWarehouseRead, c.Warehouses),
		},
	}
	if row.ParentUuid.Valid {
		p := refs.New(refs.KindOrganization, row.ParentUuid.Bytes, row.ParentName.String)
		out.Parent = &p
	}
	return out, nil
}

// MemberSummary is GET /v1/tenant/users/{uuid}/summary: a staff user seen
// from the caller's members.read scope.
type MemberSummary struct {
	User        MemberUser         `json:"user"`
	Ref         refs.Ref           `json:"ref"`
	Memberships []MemberMembership `json:"memberships"`
	Counts      MemberCounts       `json:"counts"`
}

// MemberUser is the identity block of a member summary.
type MemberUser struct {
	UUID        uuid.UUID  `json:"uuid"`
	Name        string     `json:"name"`
	Surname     string     `json:"surname"`
	Email       *string    `json:"email"`
	PhoneE164   *string    `json:"phone_e164"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   *time.Time `json:"created_at"`
}

// MemberMembership is one membership inside the caller's scope.
type MemberMembership struct {
	Organization refs.Ref   `json:"organization"`
	Role         string     `json:"role"`
	RoleSlugs    []string   `json:"role_slugs"`
	Warehouses   []refs.Ref `json:"warehouses"`
	CreatedAt    *time.Time `json:"created_at"`
}

// MemberCounts are what the user did inside the caller's scope; nil = the
// caller may not read that module.
type MemberCounts struct {
	ServicesCreated   *int64 `json:"services_created"`
	ServicesPerformed *int64 `json:"services_performed"`
	OrdersCreated     *int64 `json:"orders_created"`
	OpenTasks         *int64 `json:"open_tasks"`
	LeadsAssigned     *int64 `json:"leads_assigned"`
}

// MemberSummaryInScope returns the summary of a staff user who is a member
// of an organization inside the caller's members.read scope; a user
// outside it (or a customer only account) is ErrNotFound.
func (s *Service) MemberSummaryInScope(ctx context.Context, f scopefilter.Filter, id uuid.UUID, has PermissionChecker) (MemberSummary, error) {
	brand, err := RequestBrand(ctx)
	if err != nil {
		return MemberSummary{}, err
	}
	if f.Scope == rbac.ScopeCustomer || f.Scope == "" {
		return MemberSummary{}, ErrNotFound
	}
	orgIDs := f.OrgIDsArg()
	u, err := s.q.TenantMemberUser(ctx, db.TenantMemberUserParams{Uuid: id, BrandID: brand.ID, OrgIds: orgIDs})
	if errors.Is(err, pgx.ErrNoRows) {
		return MemberSummary{}, ErrNotFound
	}
	if err != nil {
		return MemberSummary{}, fmt.Errorf("organizations: member: %w", err)
	}
	rows, err := s.q.TenantMemberMemberships(ctx, db.TenantMemberMembershipsParams{UserID: u.ID, BrandID: brand.ID, OrgIds: orgIDs})
	if err != nil {
		return MemberSummary{}, fmt.Errorf("organizations: member memberships: %w", err)
	}
	c, err := s.q.TenantMemberCounts(ctx, db.TenantMemberCountsParams{UserID: u.ID, BrandID: brand.ID, OrgIds: orgIDs})
	if err != nil {
		return MemberSummary{}, fmt.Errorf("organizations: member counts: %w", err)
	}
	out := MemberSummary{
		User: MemberUser{
			UUID: u.Uuid, Name: u.Name, Surname: u.Surname, Email: textPtr(u.Email), PhoneE164: textPtr(u.PhoneE164),
			Status: u.Status, LastLoginAt: timePtr(u.LastLoginAt), CreatedAt: timePtr(u.CreatedAt),
		},
		Ref:         refs.UserRef(u.Uuid, refs.FullName(u.Name, u.Surname)),
		Memberships: make([]MemberMembership, 0, len(rows)),
		Counts: MemberCounts{
			ServicesCreated:   gated(has, rbac.PermServicesRead, c.ServicesCreated),
			ServicesPerformed: gated(has, rbac.PermServicesRead, c.ServicesPerformed),
			OrdersCreated:     gated(has, rbac.PermOrdersRead, c.OrdersCreated),
			OpenTasks:         gated(has, rbac.PermTasksRead, c.OpenTasks),
			LeadsAssigned:     gated(has, rbac.PermLeadsRead, c.LeadsAssigned),
		},
	}
	for _, r := range rows {
		var whs []struct {
			UUID      uuid.UUID `json:"uuid"`
			Name      string    `json:"name"`
			Code      string    `json:"code"`
			IsDefault bool      `json:"is_default"`
		}
		if len(r.Warehouses) > 0 {
			if err := json.Unmarshal(r.Warehouses, &whs); err != nil {
				return MemberSummary{}, err
			}
		}
		m := MemberMembership{
			Organization: refs.OrgRef(r.OrganizationUuid, r.OrganizationName, r.OrganizationType),
			Role:         r.Role, RoleSlugs: r.RoleSlugs, Warehouses: make([]refs.Ref, 0, len(whs)),
			CreatedAt: timePtr(r.CreatedAt),
		}
		if m.RoleSlugs == nil {
			m.RoleSlugs = []string{}
		}
		for _, w := range whs {
			m.Warehouses = append(m.Warehouses, refs.WithSub(refs.KindWarehouse, w.UUID, w.Name, w.Code))
		}
		out.Memberships = append(out.Memberships, m)
	}
	return out, nil
}
