package identity

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
)

// Loader hydrates Principal from JWT claims via AuthUseCase.
type Loader struct {
	UC *usecase.AuthUseCase
}

// LoadPrincipal implements middleware.IdentityLoader.
func (l Loader) LoadPrincipal(r *http.Request, claims jwt.Claims) (authctx.Principal, error) {
	ctx := r.Context()
	userUUID, err := claims.UserUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	var issuedAt time.Time
	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.Time
	}
	if l.UC.AccessRevoked(ctx, userUUID, claims.SessionUUID(), issuedAt) {
		return authctx.Principal{}, usecase.ErrSessionRevoked
	}
	user, err := l.UC.LoadUserByUUID(ctx, userUUID)
	if err != nil {
		return authctx.Principal{}, err
	}
	if user.Status == "disabled" {
		return authctx.Principal{}, usecase.ErrUserDisabled
	}
	isSuperAdmin := claims.IsSuperAdmin
	perms, err := l.UC.ResolvePermissions(ctx, claims.Roles, isSuperAdmin)
	if err != nil {
		return authctx.Principal{}, err
	}
	impersonatorUUID, err := claims.ImpersonatorUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	orgUUID, err := claims.OrganizationUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	return authctx.Principal{
		UserID:             user.UUID,
		UserInternal:       user.ID,
		Email:              user.Email,
		Roles:              claims.Roles,
		Permissions:        perms,
		IsSuperAdmin:       isSuperAdmin,
		ImpersonatorUserID: impersonatorUUID,
		SessionID:          claims.SessionUUID(),
		OrganizationUUID:   orgUUID,
	}, nil
}
