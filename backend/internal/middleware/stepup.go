package middleware

import (
	"context"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// StepUpChecker reports whether a user has a recent step-up grant
// (*stepup.Service).
type StepUpChecker interface {
	HasValidGrant(ctx context.Context, userID uuid.UUID) (bool, error)
}

// RequireStepUp ensures the caller has a recent step-up grant. Sensitive
// endpoints (price changes, supplier changes, anonymization) use it.
func RequireStepUp(svc StepUpChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.PrincipalFrom(r.Context())
			if !ok {
				response.Unauthorized(w, r, "Authentication is required")
				return
			}
			if svc == nil {
				// Fail closed: without a step-up service nothing proves a
				// recent authentication.
				response.StepUpRequired(w, r)
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
