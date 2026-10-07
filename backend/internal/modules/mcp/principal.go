package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	oauthmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrForbidden: the token is valid but its user may no longer act there
// (membership or mcp.connect lost, organization suspended or expired,
// endpoint realm mismatch). The endpoint answers 403.
var ErrForbidden = errors.New("mcp: forbidden")

// AccessResolver resolves a user's permissions inside an organization from
// the stored roles (*authusecase.AuthUseCase).
type AccessResolver interface {
	ResolveStoredAccess(ctx context.Context, userID int64, orgUUID *uuid.UUID) (authusecase.Access, error)
}

// PrincipalQueries are the reads of the resolver (*db.Queries).
type PrincipalQueries interface {
	GetUserByID(ctx context.Context, id int64) (db.User, error)
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
	GetBrandByID(ctx context.Context, id int64) (db.Brand, error)
	GetOrganizationMemberByUserAndOrgUUID(ctx context.Context, arg db.GetOrganizationMemberByUserAndOrgUUIDParams) (db.GetOrganizationMemberByUserAndOrgUUIDRow, error)
}

// StoreResolver builds the tool principal of a token on every request: the
// token's user, organization and endpoint realm with the permissions the
// user holds there now, so revocations apply at once.
type StoreResolver struct {
	Q      PrincipalQueries
	Access AccessResolver
	Now    func() time.Time
}

// Resolve implements Resolver.
func (s StoreResolver) Resolve(ctx context.Context, tok oauthmodel.AccessToken) (aitools.Principal, error) {
	user, err := s.Q.GetUserByID(ctx, tok.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return aitools.Principal{}, ErrForbidden
	}
	if err != nil {
		return aitools.Principal{}, fmt.Errorf("mcp: user: %w", err)
	}
	org, err := s.Q.GetOrganizationByID(ctx, tok.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return aitools.Principal{}, ErrForbidden
	}
	if err != nil {
		return aitools.Principal{}, fmt.Errorf("mcp: organization: %w", err)
	}
	brand, err := s.Q.GetBrandByID(ctx, org.BrandID)
	if err != nil {
		return aitools.Principal{}, fmt.Errorf("mcp: brand: %w", err)
	}
	if org.BrandID != tok.BrandID || brand.Status != "active" || !s.orgOpen(org) {
		return aitools.Principal{}, ErrForbidden
	}
	auth := authctx.Principal{UserID: user.Uuid, UserInternal: user.ID, Email: user.Email.String}
	// TEC-461: the "waiting for approval" summary is in the user's language.
	locale := i18n.Resolve(i18n.Sources{UserLocale: user.Locale.String, OrgLocale: org.Locale}).Locale

	if oauthmodel.RealmOf(tok.Resource) == oauthmodel.RealmCustomer {
		// Customer tools read the customer's own records in the brand of the
		// center the token is bound to.
		acc, err := s.Access.ResolveStoredAccess(ctx, user.ID, nil)
		if err != nil {
			return aitools.Principal{}, fmt.Errorf("mcp: access: %w", err)
		}
		fill(&auth, acc, jwt.AudiencePortal)
		return aitools.Principal{
			Auth: auth, Realm: aitools.RealmCustomer, Locale: locale,
			Brand: &brandctx.Brand{ID: brand.ID, Slug: brand.Slug, Name: brand.Name, Status: brand.Status},
		}, nil
	}

	if oauthmodel.RealmOf(tok.Resource) == oauthmodel.RealmDealer &&
		!slices.Contains([]string{rbac.OrgTypeDistributor, rbac.OrgTypeDealer}, org.Type) {
		return aitools.Principal{}, ErrForbidden
	}
	acc, err := s.Access.ResolveStoredAccess(ctx, user.ID, &org.Uuid)
	if err != nil {
		return aitools.Principal{}, fmt.Errorf("mcp: access: %w", err)
	}
	memberRole := ""
	member, err := s.Q.GetOrganizationMemberByUserAndOrgUUID(ctx, db.GetOrganizationMemberByUserAndOrgUUIDParams{UserID: user.ID, Uuid: org.Uuid})
	switch {
	case err == nil:
		memberRole = member.Role
	case !errors.Is(err, pgx.ErrNoRows):
		return aitools.Principal{}, fmt.Errorf("mcp: membership: %w", err)
	case !acc.IsSuperAdmin:
		// The platform admin acts for the center without a membership
		// (TEC-401 consent); everyone else must still be a member.
		return aitools.Principal{}, ErrForbidden
	}
	if _, ok := acc.Grants[rbac.PermMCPConnect]; !ok && !acc.IsSuperAdmin {
		return aitools.Principal{}, ErrForbidden
	}
	fill(&auth, acc, jwt.AudiencePanel)
	orgUUID := org.Uuid
	auth.OrganizationUUID = &orgUUID
	return aitools.Principal{
		Auth: auth, Realm: aitools.RealmPanel, Locale: locale,
		Org: &orgctx.Scope{
			InternalID: org.ID, UUID: org.Uuid, Slug: org.Slug, Name: org.Name, MemberRole: memberRole,
			Status: org.Status, OrgType: org.Type, BrandID: org.BrandID, BrandSlug: brand.Slug,
		},
	}, nil
}

func fill(p *authctx.Principal, acc authusecase.Access, realm string) {
	p.Roles, p.Permissions, p.PermissionScopes, p.IsSuperAdmin = acc.Roles, acc.Permissions, acc.Grants, acc.IsSuperAdmin
	p.Realm = realm
}

// orgOpen mirrors the panel organization gate: suspended or expired
// organizations and closed access windows are refused (read_only keeps
// reads; its writes only become pending actions).
func (s StoreResolver) orgOpen(org db.Organization) bool {
	if org.Status == "suspended" || org.Status == "expired" {
		return false
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	if org.AccessStartsAt.Valid && org.AccessStartsAt.Time.After(now) {
		return false
	}
	if org.AccessEndsAt.Valid && !org.AccessEndsAt.Time.After(now) {
		return false
	}
	return true
}
