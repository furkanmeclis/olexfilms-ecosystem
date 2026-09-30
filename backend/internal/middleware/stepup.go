package middleware

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/stepup"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// RequireStepUp ensures the caller has a recent step-up grant.
func RequireStepUp(svc *stepup.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			valid, err := svc.HasValidGrant(r.Context(), p.UserID)
			if err != nil {
				response.Internal(w, r, "Failed to verify step-up grant")
				return
			}
			if !valid {
				response.StepUpRequired(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
