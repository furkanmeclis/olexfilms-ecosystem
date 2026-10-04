package middleware

import (
	"context"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// FeatureChecker answers whether a module is switched on for an organization.
// The server passes features.Service (DB flags + Redis 30 s cache, TEC-86):
// core modules are always on, unknown keys are off.
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// RequireFeature blocks a module the organization does not have
// (403 FEATURE_DISABLED). It runs after RequireOrganization; without an
// organization context (platform routes) or with a nil checker (unit tests
// of other middleware) it passes.
func RequireFeature(ent FeatureChecker, key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := orgctx.ScopeFrom(r.Context())
			if ok && ent != nil {
				on, err := ent.Enabled(r.Context(), scope.InternalID, key)
				if err != nil {
					response.InternalErr(w, r, err, "feature check failed")
					return
				}
				if !on {
					response.Error(w, r, http.StatusForbidden, response.CodeFeatureDisabled, "This feature is not enabled for your organization")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireFeatureForOrgType is RequireFeature for organizations of one type
// only (TEC-342): other organization types pass unchanged. Accounting writes
// use it so a dealer needs dealer_accounting while the center and
// distributors keep their F1 behaviour.
func RequireFeatureForOrgType(ent FeatureChecker, orgType, key string) func(http.Handler) http.Handler {
	gate := RequireFeature(ent, key)
	return func(next http.Handler) http.Handler {
		gated := gate(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if scope, ok := orgctx.ScopeFrom(r.Context()); ok && scope.OrgType == orgType {
				gated.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
