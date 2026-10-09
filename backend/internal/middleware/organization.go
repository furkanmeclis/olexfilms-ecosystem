package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	CodeOrganizationContextRequired = "ORGANIZATION_CONTEXT_REQUIRED"
	CodeOrganizationReadOnly        = response.CodeOrganizationReadOnly
)

// OrganizationResolver loads membership for the JWT organization claim.
type OrganizationResolver interface {
	GetOrganizationMemberByUserAndOrgUUID(
		ctx context.Context,
		userID int64,
		orgUUID uuid.UUID,
	) (db.GetOrganizationMemberByUserAndOrgUUIDRow, error)
}

// BrandCenterResolver is implemented by resolvers that can load a brand's
// center organization; RequireOrganization uses it for the super admin
// fallback (TEC-522).
type BrandCenterResolver interface {
	GetBrandCenter(ctx context.Context, brandID int64) (db.Organization, error)
}

// RequireOrganization validates JWT oid, membership, and organization access.
// A super admin without an organization claim (the platform panel token)
// works in the domain brand's center organization instead: the platform
// screens for brand-scoped settings (contract templates, service catalog,
// certificate types ...) are center screens.
func RequireOrganization(tokens *jwt.Manager, q *db.Queries) func(http.Handler) http.Handler {
	return RequireOrganizationResolver(tokens, orgResolver{q: q})
}

type orgResolver struct {
	q *db.Queries
}

func (r orgResolver) GetBrandCenter(ctx context.Context, brandID int64) (db.Organization, error) {
	return r.q.GetBrandCenter(ctx, brandID)
}

func (r orgResolver) GetOrganizationMemberByUserAndOrgUUID(
	ctx context.Context,
	userID int64,
	orgUUID uuid.UUID,
) (db.GetOrganizationMemberByUserAndOrgUUIDRow, error) {
	return r.q.GetOrganizationMemberByUserAndOrgUUID(ctx, db.GetOrganizationMemberByUserAndOrgUUIDParams{
		UserID: userID, Uuid: orgUUID,
	})
}

func RequireOrganizationResolver(tokens *jwt.Manager, resolver OrganizationResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			raw := bearerToken(r.Header.Get("Authorization"))
			claims, err := tokens.ParseAccess(raw)
			if err != nil {
				response.Unauthorized(w, r, "Invalid or expired access token")
				return
			}
			orgUUID, err := claims.OrganizationUUID()
			if err != nil || orgUUID == nil || *orgUUID == uuid.Nil {
				if p.IsSuperAdmin {
					if scope, ok := superAdminCenterScope(w, r, resolver); ok {
						next.ServeHTTP(w, r.WithContext(orgctx.WithScope(r.Context(), scope)))
					}
					return
				}
				response.Error(w, r, http.StatusForbidden, CodeOrganizationContextRequired,
					"Organization context is required. Sign in with organization_slug.")
				return
			}
			row, err := resolver.GetOrganizationMemberByUserAndOrgUUID(r.Context(), p.UserInternal, *orgUUID)
			if err != nil {
				if err == pgx.ErrNoRows {
					response.Error(w, r, http.StatusForbidden, response.CodeNoTenantMembership,
						"You are not a member of this organization")
					return
				}
				response.InternalErr(w, r, err, "failed to resolve organization membership")
				return
			}
			if row.OrganizationStatus == "suspended" {
				response.Forbidden(w, r, "Organization is suspended")
				return
			}
			if !orgAccessAllowed(row.OrganizationStatus, row.AccessStartsAt, row.AccessEndsAt) {
				response.Error(w, r, http.StatusForbidden, response.CodeOrganizationAccessExpired,
					"Organization access has expired")
				return
			}
			brand, ok := brandctx.From(r.Context())
			if !ok {
				response.InternalErr(w, r, errBrandUnresolved, "failed to resolve request brand")
				return
			}
			if row.OrganizationBrandID != brand.ID {
				response.Error(w, r, http.StatusForbidden, CodeBrandMismatch,
					"Organization does not belong to this domain's brand")
				return
			}
			// K23: read_only organizations keep read access only.
			if row.OrganizationStatus == "read_only" && !isSafeMethod(r.Method) {
				response.Error(w, r, http.StatusForbidden, CodeOrganizationReadOnly,
					"Organization is read-only")
				return
			}
			scope := orgctx.Scope{
				InternalID: row.OrganizationID,
				UUID:       row.OrganizationUuid,
				Slug:       row.OrganizationSlug,
				Name:       row.OrganizationName,
				MemberRole: row.Role,
				Status:     row.OrganizationStatus,
				OrgType:    row.OrganizationType,
				BrandID:    row.OrganizationBrandID,
				BrandSlug:  row.BrandSlug,
			}
			next.ServeHTTP(w, r.WithContext(orgctx.WithScope(r.Context(), scope)))
		})
	}
}

var errBrandUnresolved = errors.New("request brand is not resolved")

// superAdminCenterScope resolves the domain brand's center organization for
// an organization-less super admin. ok false means a response was written.
func superAdminCenterScope(w http.ResponseWriter, r *http.Request, resolver OrganizationResolver) (orgctx.Scope, bool) {
	centers, ok := resolver.(BrandCenterResolver)
	if !ok {
		response.Error(w, r, http.StatusForbidden, CodeOrganizationContextRequired,
			"Organization context is required. Sign in with organization_slug.")
		return orgctx.Scope{}, false
	}
	brand, ok := brandctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errBrandUnresolved, "failed to resolve request brand")
		return orgctx.Scope{}, false
	}
	center, err := centers.GetBrandCenter(r.Context(), brand.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			response.Error(w, r, http.StatusForbidden, CodeOrganizationContextRequired,
				"The brand has no center organization.")
			return orgctx.Scope{}, false
		}
		response.InternalErr(w, r, err, "failed to resolve brand center")
		return orgctx.Scope{}, false
	}
	return orgctx.Scope{
		InternalID:         center.ID,
		UUID:               center.Uuid,
		Slug:               center.Slug,
		Name:               center.Name,
		MemberRole:         rbac.RoleSuperAdmin,
		Status:             center.Status,
		OrgType:            center.Type,
		BrandID:            center.BrandID,
		BrandSlug:          brand.Slug,
		SuperAdminFallback: true,
	}, true
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func orgAccessAllowed(status string, accessStarts, accessEnds pgtype.Timestamptz) bool {
	if status == "suspended" || status == "expired" {
		return false
	}
	now := time.Now().UTC()
	if accessStarts.Valid && accessStarts.Time.After(now) {
		return false
	}
	if accessEnds.Valid && !accessEnds.Time.After(now) {
		return false
	}
	return true
}
