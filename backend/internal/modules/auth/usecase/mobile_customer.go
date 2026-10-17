package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
)

// Customer mobile sessions: a customer / fleet only account (the accounts the
// panel refuses with NO_PANEL_ACCESS) signs in to the mobile app as well. Its
// mobile token keeps aud=mobile and carries mobile_kind=customer; the route
// rule (middleware.MobileCustomerRealm) limits it to its own account routes
// and the portal mirror (/v1/mobile/portal/*).
//
// The kind is not stored on the refresh row: it is derived from the account
// every time a mobile token is issued (sign-in, refresh, organization switch),
// so a rotation keeps it and a role change applies on the next refresh.

// mobileSessionKind classifies an account for a mobile session with the
// panel realm rule: an account the panel refuses (customer / fleet roles
// only, no organization membership) is a customer, everything else is staff.
func (u *AuthUseCase) mobileSessionKind(ctx context.Context, user model.User) (string, error) {
	err := u.checkRealmAccess(ctx, user, jwt.AudienceMobile)
	switch {
	case errors.Is(err, ErrNoPanelAccess):
		return jwt.MobileKindCustomer, nil
	case err != nil:
		return "", err
	}
	return jwt.MobileKindStaff, nil
}

// mobileKindFor is the kind of the token issueTokensForUser is about to
// issue: "" for panel / portal sessions.
func (u *AuthUseCase) mobileKindFor(ctx context.Context, user model.User, meta model.SessionMeta) (string, error) {
	if jwt.NormalizeAudience(meta.Realm) != jwt.AudienceMobile {
		return "", nil
	}
	return u.mobileSessionKind(ctx, user)
}

// checkCustomerMobileMFA: a customer signs in to the portal with the phone
// OTP alone, so the staff password-login 2FA policy does not apply to the
// customer mobile sign-in either; an authenticator the account enrolled
// itself (fleet users, TEC-637) is still required.
func (u *AuthUseCase) checkCustomerMobileMFA(ctx context.Context, userID int64, totpCode string) error {
	hasTOTP, err := u.UserHasEnabledTOTP(ctx, userID)
	if err != nil || !hasTOTP {
		return err
	}
	code := strings.TrimSpace(totpCode)
	if code == "" {
		return ErrMFARequired
	}
	valid, _, err := u.verifyUserTOTPCode(ctx, userID, code, true)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidMFACode
	}
	return nil
}
