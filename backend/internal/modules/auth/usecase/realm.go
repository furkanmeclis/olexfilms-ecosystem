package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

var (
	// ErrNoPortalAccess: the account has no customer / fleet role (staff
	// sign in to the panel, not the portal).
	ErrNoPortalAccess = errors.New("no portal access")
	// ErrNoPanelAccess: a customer / fleet only account (no staff role, no
	// organization membership) signs in to the portal, not the panel.
	ErrNoPanelAccess = errors.New("no panel access")
	// ErrRealmNotAllowed: the operation is panel only (organization context,
	// impersonation) and was called from a portal session.
	ErrRealmNotAllowed = errors.New("operation is not available in this session realm")
)

// ParseRealm validates a client supplied realm; "" means panel.
func ParseRealm(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", jwt.AudiencePanel:
		return jwt.AudiencePanel, nil
	case jwt.AudiencePortal:
		return jwt.AudiencePortal, nil
	default:
		return "", fmt.Errorf("%w: realm must be panel or portal", ErrInvalidRequest)
	}
}

func isPortalRole(slug string) bool {
	return slug == rbac.RoleCustomer || slug == rbac.RoleFleet
}

// checkRealmAccess applies the realm rule of TEC-90 to a signed-in user:
//   - portal: the user holds the customer or fleet global role;
//   - panel: a user whose only roles are customer / fleet and who has no
//     organization membership is refused. Users without any role (fresh
//     sign-ups that still have to create an organization) keep panel access.
func (u *AuthUseCase) checkRealmAccess(ctx context.Context, user model.User, realm string) error {
	roles, err := u.repo.ListUserRoleSlugs(ctx, user.ID)
	if err != nil {
		return err
	}
	portal, other := false, false
	for _, r := range roles {
		if isPortalRole(r) {
			portal = true
		} else if r != "" {
			other = true
		}
	}
	if jwt.NormalizeAudience(realm) == jwt.AudiencePortal {
		if !portal {
			return ErrNoPortalAccess
		}
		return nil
	}
	if !portal || other {
		return nil
	}
	if u.orgResolver != nil {
		memberships, err := u.orgResolver.ListMembershipsForUser(ctx, user.ID)
		if err != nil {
			return err
		}
		if len(memberships) > 0 {
			return nil
		}
	}
	return ErrNoPanelAccess
}

// requirePanelRealm refuses panel-only operations from a portal session and
// pins the issued session to the panel realm.
func requirePanelRealm(meta *model.SessionMeta) error {
	if jwt.NormalizeAudience(meta.Realm) == jwt.AudiencePortal {
		return ErrRealmNotAllowed
	}
	meta.Realm = jwt.AudiencePanel
	return nil
}
