package middleware

import (
	"context"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// FeatureChecker answers whether a module is switched on for an organization.
//
// TODO(TEC-86): back this with DB feature flags (+ Redis 30s cache). Until
// then every caller passes AllowAllFeatures (or nil) and every request passes.
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// AllowAllFeatures is the interim FeatureChecker: every module is on.
//
// TODO(TEC-86): replace with the DB-backed feature flag service.
type AllowAllFeatures struct{}

// Enabled always reports true.
func (AllowAllFeatures) Enabled(context.Context, int64, string) (bool, error) { return true, nil }

// RequireFeature blocks a module the organization has turned off.
// It runs after RequireOrganization. A nil checker allows everything.
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
