package identity

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
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
	if user.Status == "disabled" || user.Status == "anonymized" {
		return authctx.Principal{}, usecase.ErrUserDisabled
	}
	impersonatorUUID, err := claims.ImpersonatorUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	orgUUID, err := claims.OrganizationUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	roles := claims.Roles
	if claims.IsSuperAdmin {
		roles = append(append([]string{}, roles...), rbac.RoleSuperAdmin)
	}
	// Global roles come from the token; organization roles are read for the
	// active organization (oid) on every request so revocations apply at once.
	access, err := l.UC.ResolveAccess(ctx, user.ID, roles, orgUUID)
	if err != nil {
		return authctx.Principal{}, err
	}
	return authctx.Principal{
		UserID:             user.UUID,
		UserInternal:       user.ID,
		Email:              user.Email,
		Roles:              access.Roles,
		Permissions:        access.Permissions,
		PermissionScopes:   access.Grants,
		IsSuperAdmin:       access.IsSuperAdmin,
		ImpersonatorUserID: impersonatorUUID,
		SessionID:          claims.SessionUUID(),
		OrganizationUUID:   orgUUID,
		Realm:              claims.Realm(),
	}, nil
}
