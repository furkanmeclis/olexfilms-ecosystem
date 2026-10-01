package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

// TEC-91 (F0-15): mobile app sessions. The mobile app signs in with e-mail +
// password and a device description, keeps its tokens itself (Bearer, aud
// "mobile") and rotates the refresh token through /v1/mobile/auth/refresh.
// Each device sign-in starts one refresh chain (family_id); a rotated token
// that comes back revokes the whole chain of that device.

var (
	// ErrRefreshReused: a rotated mobile refresh token was presented again;
	// the device chain is revoked.
	ErrRefreshReused = errors.New("refresh token reuse detected")
	// ErrNotMobileSession: the access token's session is not a mobile one.
	ErrNotMobileSession = errors.New("not a mobile session")
)

// DefaultMobileRefreshTTL is the refresh lifetime of mobile sessions.
const DefaultMobileRefreshTTL = 30 * 24 * time.Hour

// SetMobileRefreshTTL sets the refresh lifetime of mobile sessions
// (JWT_MOBILE_REFRESH_TTL, default 30 days).
func (u *AuthUseCase) SetMobileRefreshTTL(d time.Duration) {
	u.mobileRefreshTTL = d
}

func (u *AuthUseCase) refreshTTLFor(meta model.SessionMeta) time.Duration {
	if meta.Client == model.ClientMobile {
		if u.mobileRefreshTTL > 0 {
			return u.mobileRefreshTTL
		}
		return DefaultMobileRefreshTTL
	}
	return u.tokens.RefreshTTL()
}

// NormalizeDevice validates the device description of a mobile sign-in.
func NormalizeDevice(d model.DeviceInfo) (model.DeviceInfo, error) {
	d.ID = strings.TrimSpace(d.ID)
	d.Name = strings.TrimSpace(d.Name)
	d.Platform = strings.ToLower(strings.TrimSpace(d.Platform))
	d.AppVersion = strings.TrimSpace(d.AppVersion)
	switch {
	case d.ID == "" || len(d.ID) > 128:
		return d, fmt.Errorf("%w: device.id is required (max 128)", ErrInvalidRequest)
	case len(d.Name) > 128:
		return d, fmt.Errorf("%w: device.name is too long (max 128)", ErrInvalidRequest)
	case d.Platform != "ios" && d.Platform != "android":
		return d, fmt.Errorf("%w: device.platform must be ios or android", ErrInvalidRequest)
	case len(d.AppVersion) > 32:
		return d, fmt.Errorf("%w: device.app_version is too long (max 32)", ErrInvalidRequest)
	}
	return d, nil
}

// MobileLogin signs a staff user in from the mobile app. The realm rule is
// the panel one (customer / fleet only accounts use the portal). Older
// sessions of the same device end, so a device has one active chain.
func (u *AuthUseCase) MobileLogin(ctx context.Context, email, rawPassword, totpCode, organizationSlug string, device model.DeviceInfo, meta model.SessionMeta) (model.Tokens, error) {
	dev, err := NormalizeDevice(device)
	if err != nil {
		return model.Tokens{}, err
	}
	meta.Realm = jwt.AudienceMobile
	meta.Client = model.ClientMobile
	meta.Device = &dev
	meta.FamilyID = uuid.Nil
	meta.ImpersonatorUserID = nil
	tokens, err := u.passwordLogin(ctx, email, rawPassword, totpCode, organizationSlug, meta)
	if err != nil {
		return model.Tokens{}, err
	}
	claims, err := u.tokens.ParseAccess(tokens.AccessToken)
	if err != nil {
		return model.Tokens{}, err
	}
	userID, err := claims.UserUUID()
	if err != nil {
		return model.Tokens{}, err
	}
	user, err := u.repo.FindUserByUUID(ctx, userID)
	if err != nil {
		return model.Tokens{}, err
	}
	u.endDeviceSessions(ctx, user.ID, dev.ID, claims.SessionUUID())
	return tokens, nil
}

// PrincipalFromAccess describes a just issued access token (user, session,
// active organization) so the sign-in response can carry the hydration.
func (u *AuthUseCase) PrincipalFromAccess(ctx context.Context, access string) (authctx.Principal, error) {
	claims, err := u.tokens.ParseAccess(access)
	if err != nil {
		return authctx.Principal{}, err
	}
	userUUID, err := claims.UserUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		return authctx.Principal{}, err
	}
	orgUUID, err := claims.OrganizationUUID()
	if err != nil {
		return authctx.Principal{}, err
	}
	return authctx.Principal{
		UserID: user.UUID, UserInternal: user.ID, Email: user.Email,
		SessionID: claims.SessionUUID(), OrganizationUUID: orgUUID, Realm: claims.Realm(),
	}, nil
}

// MobileRefresh rotates a mobile refresh token. appVersion (optional)
// updates the stored app version of the device.
func (u *AuthUseCase) MobileRefresh(ctx context.Context, rawToken, appVersion string, meta model.SessionMeta) (model.Tokens, error) {
	if len(strings.TrimSpace(appVersion)) > 32 {
		return model.Tokens{}, fmt.Errorf("%w: app_version is too long (max 32)", ErrInvalidRequest)
	}
	meta.Device = &model.DeviceInfo{AppVersion: strings.TrimSpace(appVersion)}
	return u.refresh(ctx, rawToken, meta, true)
}

// detectRefreshReuse reports (and handles) a rotated mobile refresh token
// presented again: the device chain is revoked and its access tokens stop
// working now.
func (u *AuthUseCase) detectRefreshReuse(ctx context.Context, hash string) bool {
	row, err := u.repo.FindRefreshAny(ctx, hash)
	if err != nil || row.Client != model.ClientMobile || row.RotatedAt == nil || row.FamilyID == uuid.Nil {
		return false
	}
	ids, err := u.repo.RevokeRefreshFamily(ctx, row.FamilyID)
	if err != nil {
		if u.log != nil {
			u.log.Warn("auth_refresh_family_revoke_failed", "error", err)
		}
		return true
	}
	u.RevokeAccessSession(ctx, row.UUID)
	for _, id := range ids {
		u.RevokeAccessSession(ctx, id)
	}
	if u.log != nil {
		u.log.Warn("auth_mobile_refresh_reuse", "family_id", row.FamilyID.String(), "revoked", len(ids))
	}
	return true
}

func (u *AuthUseCase) endDeviceSessions(ctx context.Context, userID int64, deviceID string, keep uuid.UUID) {
	ids, err := u.repo.RevokeMobileDeviceSessions(ctx, userID, deviceID, keep)
	if err != nil {
		if u.log != nil {
			u.log.Warn("auth_mobile_device_revoke_failed", "error", err)
		}
		return
	}
	for _, id := range ids {
		u.RevokeAccessSession(ctx, id)
	}
}

// MobileSession returns the active mobile session of an access token sid.
func (u *AuthUseCase) MobileSession(ctx context.Context, userID int64, sid uuid.UUID) (model.RefreshSession, error) {
	if sid == uuid.Nil {
		return model.RefreshSession{}, ErrNotMobileSession
	}
	s, err := u.repo.FindRefreshByUUID(ctx, sid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.RefreshSession{}, ErrNotMobileSession
		}
		return model.RefreshSession{}, err
	}
	if s.UserID != userID || s.Client != model.ClientMobile || s.Device == nil {
		return model.RefreshSession{}, ErrNotMobileSession
	}
	if s.RevokedAt != nil {
		return model.RefreshSession{}, ErrSessionRevoked
	}
	return s, nil
}

// MobileLogout ends every session of the caller's device (the access token
// stops working now) and returns the device id, so its push tokens can be
// dropped too.
func (u *AuthUseCase) MobileLogout(ctx context.Context, userID int64, sid uuid.UUID) (string, error) {
	s, err := u.MobileSession(ctx, userID, sid)
	if err != nil {
		if errors.Is(err, ErrSessionRevoked) {
			u.RevokeAccessSession(ctx, sid)
			return "", nil
		}
		return "", err
	}
	u.endDeviceSessions(ctx, userID, s.Device.ID, uuid.Nil)
	u.RevokeAccessSession(ctx, sid)
	return s.Device.ID, nil
}

// MobileLogoutAll ends every session of the user (web and mobile, this
// device included).
func (u *AuthUseCase) MobileLogoutAll(ctx context.Context, userID int64, sid uuid.UUID) error {
	sessions, err := u.repo.ListActiveSessions(ctx, userID)
	if err != nil {
		return err
	}
	if err := u.repo.RevokeAllRefresh(ctx, userID); err != nil {
		return err
	}
	for _, s := range sessions {
		u.RevokeAccessSession(ctx, s.UUID)
	}
	u.RevokeAccessSession(ctx, sid)
	return nil
}

// MobileSwitchOrganization re-issues the device's token pair with another
// active organization (oid). The new pair continues the device chain and the
// current session ends.
func (u *AuthUseCase) MobileSwitchOrganization(ctx context.Context, userUUID, sid uuid.UUID, organizationSlug string, meta model.SessionMeta) (model.Tokens, error) {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.Tokens{}, ErrInvalidCredentials
		}
		return model.Tokens{}, err
	}
	if user.Status != "active" {
		return model.Tokens{}, ErrUserDisabled
	}
	s, err := u.MobileSession(ctx, user.ID, sid)
	if err != nil {
		return model.Tokens{}, err
	}
	if u.orgResolver == nil {
		return model.Tokens{}, ErrNoTenantMembership
	}
	orgUUID, err := u.orgResolver.ResolveLoginOrganization(ctx, user.ID, organizationSlug)
	if err != nil {
		return model.Tokens{}, mapOrganizationError(err)
	}
	meta.Realm = jwt.AudienceMobile
	meta.Client = model.ClientMobile
	dev := *s.Device
	meta.Device = &dev
	meta.FamilyID = s.FamilyID
	meta.ImpersonatorUserID = nil
	tokens, err := u.issueTokensForUser(ctx, user, meta, &orgUUID)
	if err != nil {
		return model.Tokens{}, err
	}
	if err := u.repo.RevokeSession(ctx, user.ID, sid); err != nil && !errors.Is(err, repository.ErrNotFound) {
		return model.Tokens{}, err
	}
	u.RevokeAccessSession(ctx, sid)
	return tokens, nil
}
