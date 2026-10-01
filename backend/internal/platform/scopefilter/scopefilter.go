// Package scopefilter turns a scoped permission grant into the record filter
// repositories apply. There is no RLS (AGENTS §3): every tenant query of a
// business module must go through a Filter.
package scopefilter

import (
	"context"
	"errors"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	// ErrForbidden: the principal does not hold the permission.
	ErrForbidden = errors.New("scopefilter: permission not granted")
	// ErrOrganizationRequired: the grant is organization relative but the
	// request has no active organization.
	ErrOrganizationRequired = errors.New("scopefilter: organization context required")
)

// TreeReader lists the organizations below one organization.
type TreeReader interface {
	Descendants(ctx context.Context, id int64) ([]db.Organization, error)
}

// Filter is the resolved reach of one permission for one request.
type Filter struct {
	Permission string
	Scope      rbac.Scope
	// UserID is the internal user id (own, assigned and customer scopes).
	UserID int64
	// OrgID is the active organization (0 without organization context).
	OrgID int64
	// OrgIDs restricts records to these organizations (own, assigned,
	// managed, subtree). Nil means no organization restriction.
	OrgIDs []int64
	// BrandID restricts records to one brand (brand scope). 0 means none.
	BrandID int64
}

// Resolve builds the filter of permission slug for principal p in the active
// organization org (nil when the route has no organization context).
func Resolve(ctx context.Context, tree TreeReader, p authctx.Principal, org *orgctx.Scope, slug string) (Filter, error) {
	scope, ok := p.ScopeFor(slug)
	if !ok {
		return Filter{}, ErrForbidden
	}
	f := Filter{Permission: slug, Scope: scope, UserID: p.UserInternal}
	if org != nil {
		f.OrgID = org.InternalID
	}
	switch scope {
	case rbac.ScopeAll:
		return f, nil
	case rbac.ScopeCustomer:
		return f, nil
	case rbac.ScopeBrand:
		if org == nil {
			return Filter{}, ErrOrganizationRequired
		}
		f.BrandID = org.BrandID
		return f, nil
	case rbac.ScopeSubtree:
		if org == nil {
			return Filter{}, ErrOrganizationRequired
		}
		below, err := tree.Descendants(ctx, org.InternalID)
		if err != nil {
			return Filter{}, err
		}
		f.OrgIDs = make([]int64, 0, len(below)+1)
		f.OrgIDs = append(f.OrgIDs, org.InternalID)
		for _, o := range below {
			f.OrgIDs = append(f.OrgIDs, o.ID)
		}
		return f, nil
	default: // managed, assigned, own
		if org == nil {
			return Filter{}, ErrOrganizationRequired
		}
		f.OrgIDs = []int64{org.InternalID}
		return f, nil
	}
}

// AllowsOrg reports whether a record owned by organization orgID of brand
// brandID is inside the organization reach of the filter. Own/assigned
// filters additionally need a per-record user check (UserOnly).
func (f Filter) AllowsOrg(orgID, brandID int64) bool {
	switch f.Scope {
	case rbac.ScopeAll:
		return true
	case rbac.ScopeBrand:
		return brandID == f.BrandID
	case rbac.ScopeCustomer:
		return false
	default:
		for _, id := range f.OrgIDs {
			if id == orgID {
				return true
			}
		}
		return false
	}
}

// UserOnly reports whether records must also belong to the user (created by
// or assigned to UserID, or the customer themself).
func (f Filter) UserOnly() bool {
	return f.Scope == rbac.ScopeOwn || f.Scope == rbac.ScopeAssigned || f.Scope == rbac.ScopeCustomer
}

// OrgIDsArg is the nullable bigint[] argument for sqlc queries: nil (no
// organization restriction) for all/brand, an empty set for customer (the
// customer scope never reaches organization records), the id set otherwise.
func (f Filter) OrgIDsArg() []int64 {
	switch f.Scope {
	case rbac.ScopeAll, rbac.ScopeBrand:
		return nil
	case rbac.ScopeCustomer:
		return []int64{}
	default:
		if f.OrgIDs == nil {
			return []int64{}
		}
		return f.OrgIDs
	}
}

// BrandIDArg is the nullable brand argument for sqlc queries.
func (f Filter) BrandIDArg() pgtype.Int8 {
	if f.Scope != rbac.ScopeBrand {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: f.BrandID, Valid: true}
}

type ctxKey struct{}

// With stores a resolved filter on the context.
func With(ctx context.Context, f Filter) context.Context {
	return context.WithValue(ctx, ctxKey{}, f)
}

// From returns the filter stored by middleware.RequireScope.
func From(ctx context.Context) (Filter, bool) {
	f, ok := ctx.Value(ctxKey{}).(Filter)
	return f, ok
}
