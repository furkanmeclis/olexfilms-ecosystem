package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authrevoke"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrUserDisabled         = errors.New("user is disabled")
	ErrSessionRevoked       = errors.New("session was signed out")
	ErrForbidden            = errors.New("forbidden")
	ErrNotFound             = errors.New("not found")
	ErrConflict             = errors.New("conflict")
	ErrInvalidRequest       = errors.New("invalid request")
	ErrLastSuperAdmin       = errors.New("cannot demote the last super admin")
	ErrSystemRole           = errors.New("system role cannot be modified")
	ErrAlreadyImpersonating = errors.New("already impersonating another user")
)

// Repository is the persistence port for auth use cases.
type Repository interface {
	CreateUser(ctx context.Context, u model.User, emailVerified bool) (model.User, error)
	FindUserByEmail(ctx context.Context, email string) (model.User, error)
	FindUserByUUID(ctx context.Context, id uuid.UUID) (model.User, error)
	FindUserByID(ctx context.Context, id int64) (model.User, error)
	UpdateLastLogin(ctx context.Context, userID int64) error
	UpdatePassword(ctx context.Context, userID int64, hash string) error
	UpdateProfile(ctx context.Context, userUUID uuid.UUID, in model.ProfilePatch) (model.User, error)
	// LocaleSources loads the stored locale/timezone chain (user, active org,
	// brand center) for i18n.Resolve.
	LocaleSources(ctx context.Context, userID int64, orgUUID *uuid.UUID, brandID *int64) (i18n.Sources, error)
	SetEmailVerified(ctx context.Context, userID int64) (model.User, error)
	UpdateUserPlatform(ctx context.Context, id uuid.UUID, name, surname, status *string) (model.User, error)
	ListUsersFiltered(ctx context.Context, limit, offset int32, q, status, roleSlug string) ([]model.User, int64, error)
	CountUsersWithRole(ctx context.Context, roleSlug string) (int64, error)
	UpsertSuperAdmin(ctx context.Context, email, name, surname, hash string) (model.User, bool, error)
	GetRoleIDBySlug(ctx context.Context, slug string) (int64, error)
	ListGrantsByRoleSlugs(ctx context.Context, slugs []string) ([]model.Grant, error)
	ListMemberGrants(ctx context.Context, userID int64, orgUUID uuid.UUID) ([]model.Grant, error)
	ListUserRoleSlugs(ctx context.Context, userID int64) ([]string, error)
	UserHasRoleSlug(ctx context.Context, userID int64, slug string) (bool, error)
	ListUserRolesByUserUUID(ctx context.Context, userUUID uuid.UUID) ([]model.RoleSummary, error)
	ListRolesForUserIDs(ctx context.Context, userIDs []int64) (map[int64][]model.RoleSummary, error)
	ListOAuthAccountsForUserIDs(ctx context.Context, userIDs []int64) (map[int64][]model.OAuthAccountRecord, error)
	ListPasskeysForUserIDs(ctx context.Context, userIDs []int64) (map[int64][]model.PasskeyRecord, error)
	ReplaceUserRoles(ctx context.Context, userID int64, roleIDs []int64) error
	ResolveRoleIDsByUUIDs(ctx context.Context, roleUUIDs []uuid.UUID) ([]int64, error)
	SaveRefresh(ctx context.Context, userID int64, hash string, expiresAt time.Time, meta model.SessionMeta) (uuid.UUID, error)
	GetRefreshSession(ctx context.Context, hash string) (model.RefreshSession, error)
	ResolveOrganizationInternalID(ctx context.Context, orgUUID uuid.UUID) (int64, error)
	RevokeRefresh(ctx context.Context, hash string) error
	RevokeAllRefresh(ctx context.Context, userID int64) error
	ListActiveSessions(ctx context.Context, userID int64) ([]model.DeviceSession, error)
	RevokeSession(ctx context.Context, userID int64, sessionUUID uuid.UUID) error
	RevokeOtherSessions(ctx context.Context, userID int64, keep uuid.UUID) error
	CreateOTP(ctx context.Context, userID *int64, email, codeHash, otpType string, expiresAt time.Time) error
	InvalidateOTPs(ctx context.Context, email, otpType string) error
	GetActiveOTP(ctx context.Context, email, otpType string) (id int64, codeHash string, attempts, maxAttempts int32, err error)
	IncrementOTPAttempts(ctx context.Context, id int64) (attempts, maxAttempts int32, err error)
	ConsumeOTP(ctx context.Context, id int64) error
	GetPasskeyByCredentialID(ctx context.Context, credentialID string) (model.PasskeyRecord, error)
	ListPasskeysByUserID(ctx context.Context, userID int64) ([]model.PasskeyRecord, error)
	CreatePasskey(ctx context.Context, userID int64, in model.CreateAdapterAuthenticatorInput) (model.PasskeyRecord, error)
	UpdatePasskeyCounter(ctx context.Context, credentialID string, counter int64) error
	UpdatePasskeyName(ctx context.Context, userUUID, passkeyUUID uuid.UUID, name *string) (model.PasskeyRecord, error)
	DeletePasskeyByCredentialID(ctx context.Context, credentialID string) error
	DeletePasskeyByUUID(ctx context.Context, userUUID, passkeyUUID uuid.UUID) error
	GetOAuthAccountByProviderAccount(ctx context.Context, provider, providerAccountID string) (model.OAuthAccountRecord, error)
	GetOAuthAccountByUserProvider(ctx context.Context, userID int64, provider string) (model.OAuthAccountRecord, error)
	ListOAuthAccountsByUserID(ctx context.Context, userID int64) ([]model.OAuthAccountRecord, error)
	CreateOAuthAccount(ctx context.Context, in model.CreateOAuthAccountInput) (model.OAuthAccountRecord, error)
	DeleteOAuthAccountByProviderAccount(ctx context.Context, provider, providerAccountID string) error
	DeleteOAuthAccountByUserProvider(ctx context.Context, userID int64, provider string) error
	GetUserTOTP(ctx context.Context, userID int64) (model.UserTOTP, error)
	UpsertUserTOTPSetup(ctx context.Context, userID int64, secretEnc string) (model.UserTOTP, error)
	ConfirmUserTOTP(ctx context.Context, userID int64, recoveryHashes []string) (model.UserTOTP, error)
	UpdateUserTOTPRecoveryHashes(ctx context.Context, userID int64, recoveryHashes []string) error
	DeleteUserTOTP(ctx context.Context, userID int64) error
	ListRolesFiltered(ctx context.Context, limit, offset int32, q string) ([]model.RoleSummary, int64, error)
	GetRoleByUUID(ctx context.Context, id uuid.UUID) (model.RoleSummary, error)
	ListRolePermissionSlugs(ctx context.Context, roleUUID uuid.UUID) ([]string, error)
	CreateRole(ctx context.Context, name, slug string, description *string) (model.RoleSummary, error)
	UpdateRole(ctx context.Context, roleUUID uuid.UUID, name, description *string) (model.RoleSummary, error)
	DeleteRole(ctx context.Context, roleUUID uuid.UUID) error
	SetRolePermissions(ctx context.Context, roleID int64, grants map[string]string) error
	ListRoleGrants(ctx context.Context, roleUUID uuid.UUID) ([]model.RoleGrant, error)
	ListPermissionsFiltered(ctx context.Context, limit, offset int32, q string) ([]model.PermissionSummary, int64, error)
}

// AuthUseCase coordinates identity and session flows.
type AuthUseCase struct {
	repo          Repository
	tokens        *jwt.Manager
	now           func() time.Time
	notifier      Notifier
	log           *slog.Logger
	realtime      RealtimeHints
	searchIndexer SearchIndexer
	// Optional registration / method policy (nil = allow password login, deny register).
	authSettings AuthSettingsGate
	accessPolicy AccessPolicyGate
	box          SecretBox
	totpIssuer   string
	orgResolver  OrganizationResolver
	// Optional: rejects access tokens of signed-out sessions before expiry.
	revocations *authrevoke.Store
}

// SecretBox encrypts at-rest secrets (TOTP).
type SecretBox interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(encoded string) (string, error)
}

// AuthSettingsGate is implemented by authsettings usecase.Service.
type AuthSettingsGate interface {
	CanPasswordRegister(ctx context.Context) (bool, error)
	CanPasswordLogin(ctx context.Context) (bool, error)
	CanPasskeyLogin(ctx context.Context) (bool, error)
	RegistrationEnabled(ctx context.Context) (bool, error)
	DefaultRoleID(ctx context.Context) (*int64, error)
}

// AccessPolicyGate reads admin access / verification policy.
type AccessPolicyGate interface {
	PasswordLoginTOTPRequired(ctx context.Context) (bool, error)
}

// SearchIndexer enqueues search index updates (optional, fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// RealtimeHints carries non-secret Centrifugo metadata for /me.
type RealtimeHints struct {
	Enabled bool
	WSURL   string
}

// New creates an AuthUseCase.
func New(repo Repository, tokens *jwt.Manager) *AuthUseCase {
	return &AuthUseCase{repo: repo, tokens: tokens, now: time.Now}
}

// SetAuthSettings attaches registration / login method policy.
func (u *AuthUseCase) SetAuthSettings(gate AuthSettingsGate) {
	u.authSettings = gate
}

// SetAccessPolicy attaches step-up / login verification policy.
func (u *AuthUseCase) SetAccessPolicy(gate AccessPolicyGate) {
	u.accessPolicy = gate
}

// SetSecretBox attaches encryption used for TOTP secrets.
func (u *AuthUseCase) SetSecretBox(box SecretBox, issuer string) {
	u.box = box
	u.totpIssuer = strings.TrimSpace(issuer)
}

// SetRealtimeHints attaches Centrifugo metadata for session hydration.
func (u *AuthUseCase) SetRealtimeHints(h RealtimeHints) {
	u.realtime = h
}

// Register creates a user-only account (no roles/tokens unless default role configured).
func (u *AuthUseCase) Register(ctx context.Context, in model.RegisterInput) (model.User, error) {
	if u.authSettings != nil {
		ok, err := u.authSettings.CanPasswordRegister(ctx)
		if err != nil {
			return model.User{}, err
		}
		if !ok {
			return model.User{}, fmt.Errorf("%w: registration is disabled", ErrForbidden)
		}
	} else {
		return model.User{}, fmt.Errorf("%w: registration is disabled", ErrForbidden)
	}

	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	in.Surname = strings.TrimSpace(in.Surname)
	if in.Email == "" || in.Name == "" || in.Surname == "" {
		return model.User{}, fmt.Errorf("%w: email, name, surname are required", ErrInvalidRequest)
	}
	if strings.TrimSpace(in.Password) == "" {
		return model.User{}, fmt.Errorf("%w: password is required", ErrInvalidRequest)
	}
	if _, err := u.repo.FindUserByEmail(ctx, in.Email); err == nil {
		return model.User{}, fmt.Errorf("%w: email already registered", ErrConflict)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return model.User{}, err
	}

	hash, err := password.Hash(in.Password)
	if err != nil {
		return model.User{}, err
	}

	user, err := u.repo.CreateUser(ctx, model.User{
		Email: in.Email, PasswordHash: hash, Name: in.Name, Surname: in.Surname, Status: "active",
	}, false)
	if err != nil {
		return model.User{}, err
	}
	if err := u.assignDefaultRole(ctx, user.ID); err != nil {
		return model.User{}, err
	}
	u.notifyWelcome(ctx, user)
	return user, nil
}

func (u *AuthUseCase) assignDefaultRole(ctx context.Context, userID int64) error {
	if u.authSettings == nil {
		return nil
	}
	roleID, err := u.authSettings.DefaultRoleID(ctx)
	if err != nil {
		return err
	}
	if roleID == nil {
		return nil
	}
	return u.repo.ReplaceUserRoles(ctx, userID, []int64{*roleID})
}

// RegisterOAuthUser creates a user for OAuth self-registration (no password login).
func (u *AuthUseCase) RegisterOAuthUser(ctx context.Context, email, name, surname string, emailVerified bool) (model.AdapterUser, error) {
	if u.authSettings == nil {
		return model.AdapterUser{}, fmt.Errorf("%w: registration is disabled", ErrForbidden)
	}
	enabled, err := u.authSettings.RegistrationEnabled(ctx)
	if err != nil {
		return model.AdapterUser{}, err
	}
	if !enabled {
		return model.AdapterUser{}, fmt.Errorf("%w: registration is disabled", ErrForbidden)
	}

	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	surname = strings.TrimSpace(surname)
	if email == "" {
		return model.AdapterUser{}, fmt.Errorf("%w: email is required", ErrInvalidRequest)
	}
	if name == "" {
		name = "User"
	}
	if surname == "" {
		surname = "Account"
	}
	if _, err := u.repo.FindUserByEmail(ctx, email); err == nil {
		return model.AdapterUser{}, fmt.Errorf("%w: email already registered", ErrConflict)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return model.AdapterUser{}, err
	}

	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return model.AdapterUser{}, err
	}
	hash, err := password.Hash(base64.RawURLEncoding.EncodeToString(random))
	if err != nil {
		return model.AdapterUser{}, err
	}

	user, err := u.repo.CreateUser(ctx, model.User{
		Email: email, PasswordHash: hash, Name: name, Surname: surname, Status: "active",
	}, emailVerified)
	if err != nil {
		return model.AdapterUser{}, err
	}
	if err := u.assignDefaultRole(ctx, user.ID); err != nil {
		return model.AdapterUser{}, err
	}
	u.notifyWelcome(ctx, user)
	return toAdapterUser(user), nil
}

// Login authenticates and issues tokens for any active user.
// totpCode is required when the user has authenticator 2FA enabled, or when
// admin policy requires 2FA for password login.
func (u *AuthUseCase) Login(ctx context.Context, email, rawPassword, totpCode, organizationSlug string, meta model.SessionMeta) (model.Tokens, error) {
	if u.authSettings != nil {
		ok, err := u.authSettings.CanPasswordLogin(ctx)
		if err != nil {
			return model.Tokens{}, err
		}
		if !ok {
			return model.Tokens{}, fmt.Errorf("%w: password login is disabled", ErrForbidden)
		}
	}

	email = strings.ToLower(strings.TrimSpace(email))
	user, err := u.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Spend the same Argon2 work as a real check so response timing
			// does not reveal which emails are registered.
			burnPasswordVerify(rawPassword)
			return model.Tokens{}, ErrInvalidCredentials
		}
		return model.Tokens{}, err
	}
	// Verify the password before revealing account status (disabled vs.
	// unknown) so status cannot be probed without the credential.
	ok, err := password.Verify(user.PasswordHash, rawPassword)
	if err != nil || !ok {
		return model.Tokens{}, ErrInvalidCredentials
	}
	if user.Status != "active" {
		return model.Tokens{}, ErrUserDisabled
	}

	hasTOTP, err := u.UserHasEnabledTOTP(ctx, user.ID)
	if err != nil {
		return model.Tokens{}, err
	}
	requirePolicy := false
	if u.accessPolicy != nil {
		requirePolicy, err = u.accessPolicy.PasswordLoginTOTPRequired(ctx)
		if err != nil {
			return model.Tokens{}, err
		}
	}
	if requirePolicy && !hasTOTP {
		return model.Tokens{}, ErrMFANotEnrolled
	}
	if hasTOTP {
		code := strings.TrimSpace(totpCode)
		if code == "" {
			return model.Tokens{}, ErrMFARequired
		}
		valid, _, err := u.verifyUserTOTPCode(ctx, user.ID, code, true)
		if err != nil {
			return model.Tokens{}, err
		}
		if !valid {
			return model.Tokens{}, ErrInvalidMFACode
		}
	}

	if err := u.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		return model.Tokens{}, err
	}
	var orgUUID *uuid.UUID
	if strings.TrimSpace(organizationSlug) != "" {
		if u.orgResolver == nil {
			return model.Tokens{}, ErrNoTenantMembership
		}
		resolved, err := u.orgResolver.ResolveLoginOrganization(ctx, user.ID, organizationSlug)
		if err != nil {
			return model.Tokens{}, mapOrganizationError(err)
		}
		orgUUID = &resolved
	}
	return u.issueTokensForUser(ctx, user, meta, orgUUID)
}

// Refresh rotates an opaque refresh token.
func (u *AuthUseCase) Refresh(ctx context.Context, rawToken string, meta model.SessionMeta) (model.Tokens, error) {
	hash := tokenHash(rawToken)
	session, err := u.repo.GetRefreshSession(ctx, hash)
	if err != nil {
		return model.Tokens{}, ErrInvalidCredentials
	}
	// Revoke must win the race: a concurrent refresh with the same token
	// finds no active row and is rejected, so a refresh token is single-use.
	if err := u.repo.RevokeRefresh(ctx, hash); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.Tokens{}, ErrInvalidCredentials
		}
		return model.Tokens{}, err
	}
	user, err := u.repo.FindUserByID(ctx, session.UserID)
	if err != nil {
		return model.Tokens{}, ErrInvalidCredentials
	}
	if user.Status == "disabled" {
		return model.Tokens{}, ErrUserDisabled
	}
	meta.ImpersonatorUserID = session.ImpersonatorUserID

	var orgUUID *uuid.UUID
	if session.OrganizationUUID != nil && u.orgResolver != nil {
		// Re-validate membership + access; drop oid quietly if no longer valid.
		resolved, err := u.orgResolver.ResolveOrganizationUUID(ctx, user.ID, *session.OrganizationUUID)
		if err == nil {
			orgUUID = &resolved
		}
	}
	return u.issueTokensForUser(ctx, user, meta, orgUUID)
}

// Logout revokes a refresh token. An empty token is a no-op so the BFF can
// still clear the browser session when the refresh cookie is already gone.
func (u *AuthUseCase) Logout(ctx context.Context, rawToken string) error {
	if strings.TrimSpace(rawToken) == "" {
		return nil
	}
	if err := u.repo.RevokeRefresh(ctx, tokenHash(rawToken)); err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	return nil
}

// Me returns session hydration. orgUUID is the active organization of the
// session (JWT oid); its membership roles add scoped grants.
func (u *AuthUseCase) Me(ctx context.Context, userUUID uuid.UUID, impersonatorUUID *uuid.UUID, orgUUID *uuid.UUID) (model.Me, error) {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		return model.Me{}, ErrNotFound
	}
	roles, err := u.repo.ListUserRoleSlugs(ctx, user.ID)
	if err != nil {
		return model.Me{}, err
	}
	access, err := u.ResolveAccess(ctx, user.ID, roles, orgUUID)
	if err != nil {
		return model.Me{}, err
	}
	grants := make(map[string]string, len(access.Grants))
	for slug, scope := range access.Grants {
		grants[slug] = string(scope)
	}
	out := model.Me{
		User:               model.ToPublicUser(user, access.IsSuperAdmin),
		Roles:              access.Roles,
		Permissions:        access.Permissions,
		Grants:             grants,
		ActiveOrganization: access.OrganizationUUID,
		OrganizationRoles:  access.OrganizationRoles,
		Organizations:      []model.OrganizationSummary{},
		Links:              model.DefaultMeLinks(),
		Channels:           model.MeChannels{User: "user:" + user.UUID.String()},
		Realtime:           model.MeRealtime{Enabled: u.realtime.Enabled, WSURL: u.realtime.WSURL, UserChannel: "user:" + user.UUID.String()},
	}
	resolved, err := u.ResolveLocale(ctx, user.ID, access.OrganizationUUID)
	if err != nil {
		return model.Me{}, err
	}
	out.EffectiveLocale = string(resolved.Locale)
	out.EffectiveTimezone = resolved.Timezone
	if u.orgResolver != nil {
		memberships, err := u.orgResolver.ListMembershipsForUser(ctx, user.ID)
		if err != nil {
			return model.Me{}, err
		}
		out.Organizations = mapOrganizationSummaries(memberships)
	}
	if impersonatorUUID != nil {
		impUser, err := u.repo.FindUserByUUID(ctx, *impersonatorUUID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return model.Me{}, ErrNotFound
			}
			return model.Me{}, err
		}
		isSA, err := u.repo.UserHasRoleSlug(ctx, impUser.ID, rbac.RoleSuperAdmin)
		if err != nil {
			return model.Me{}, err
		}
		out.Impersonation = &model.MeImpersonation{
			User: model.ToPublicUser(impUser, isSA),
		}
	}
	return out, nil
}

// Access is the resolved authorization of a user in one organization context.
type Access struct {
	// Roles are the global roles followed by the active organization roles.
	Roles             []string
	OrganizationRoles []string
	IsSuperAdmin      bool
	Permissions       []string
	Grants            map[string]rbac.Scope
	OrganizationUUID  *uuid.UUID
}

// ResolveAccess merges the grants of the user's global roles (platform,
// customer, fleet) with the grants of their roles in the active organization.
// Organization-type roles found among the global roles are ignored: they only
// apply through a membership. Super admins hold the whole catalog.
func (u *AuthUseCase) ResolveAccess(ctx context.Context, userID int64, globalRoles []string, orgUUID *uuid.UUID) (Access, error) {
	out := Access{Grants: map[string]rbac.Scope{}, OrganizationRoles: []string{}}
	global := make([]string, 0, len(globalRoles))
	for _, role := range uniqueStrings(globalRoles) {
		if def, ok := rbac.RoleBySlug(role); ok && rbac.IsOrganizationRoleType(def.OrgType) {
			continue
		}
		if role == rbac.RoleSuperAdmin {
			out.IsSuperAdmin = true
		}
		global = append(global, role)
	}
	if out.IsSuperAdmin {
		sa, _ := rbac.RoleBySlug(rbac.RoleSuperAdmin)
		for slug, scope := range rbac.RoleGrants(sa) {
			out.Grants[slug] = scope
		}
	}
	rows, err := u.repo.ListGrantsByRoleSlugs(ctx, global)
	if err != nil {
		return Access{}, err
	}
	mergeGrants(out.Grants, rows)
	if orgUUID != nil && *orgUUID != uuid.Nil {
		memberRows, err := u.repo.ListMemberGrants(ctx, userID, *orgUUID)
		if err != nil {
			return Access{}, err
		}
		mergeGrants(out.Grants, memberRows)
		seen := map[string]struct{}{}
		for _, g := range memberRows {
			if _, ok := seen[g.Role]; !ok {
				seen[g.Role] = struct{}{}
				out.OrganizationRoles = append(out.OrganizationRoles, g.Role)
			}
		}
		sort.Strings(out.OrganizationRoles)
		id := *orgUUID
		out.OrganizationUUID = &id
	}
	out.Roles = append(append([]string{}, global...), out.OrganizationRoles...)
	out.Permissions = rbac.SortedGrantSlugs(out.Grants)
	return out, nil
}

func mergeGrants(dst map[string]rbac.Scope, rows []model.Grant) {
	for _, g := range rows {
		if rbac.SuperAdminOnly(g.Permission) && g.Role != rbac.RoleSuperAdmin {
			continue
		}
		scope, ok := rbac.ParseScope(g.Scope)
		if !ok {
			continue
		}
		dst[g.Permission] = rbac.Broader(dst[g.Permission], scope)
	}
}

// ResolvePermissions returns permission slugs of global roles (+ super admin
// expansion), without organization roles.
func (u *AuthUseCase) ResolvePermissions(ctx context.Context, roles []string, isSuperAdmin bool) ([]string, error) {
	if isSuperAdmin {
		roles = append(append([]string{}, roles...), rbac.RoleSuperAdmin)
	}
	access, err := u.ResolveAccess(ctx, 0, roles, nil)
	if err != nil {
		return nil, err
	}
	return access.Permissions, nil
}

// LoadUserByUUID is used by middleware identity hydration.
func (u *AuthUseCase) LoadUserByUUID(ctx context.Context, id uuid.UUID) (model.User, error) {
	user, err := u.repo.FindUserByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return user, nil
}

func (u *AuthUseCase) resolveUserAccess(ctx context.Context, userID int64) ([]string, bool, []string, error) {
	roles, err := u.repo.ListUserRoleSlugs(ctx, userID)
	if err != nil {
		return nil, false, nil, err
	}
	isSuperAdmin := false
	for _, role := range roles {
		if role == rbac.RoleSuperAdmin {
			isSuperAdmin = true
			break
		}
	}
	perms, err := u.ResolvePermissions(ctx, roles, isSuperAdmin)
	if err != nil {
		return nil, false, nil, err
	}
	return uniqueStrings(roles), isSuperAdmin, perms, nil
}

func (u *AuthUseCase) issueTokensForUser(ctx context.Context, user model.User, meta model.SessionMeta, organizationID *uuid.UUID) (model.Tokens, error) {
	roles, isSuperAdmin, _, err := u.resolveUserAccess(ctx, user.ID)
	if err != nil {
		return model.Tokens{}, err
	}
	var impersonatorUUID *uuid.UUID
	if meta.ImpersonatorUserID != nil {
		impUser, err := u.repo.FindUserByID(ctx, *meta.ImpersonatorUserID)
		if err != nil {
			return model.Tokens{}, err
		}
		impersonatorUUID = &impUser.UUID
	}
	rawRefresh, err := newOpaqueToken()
	if err != nil {
		return model.Tokens{}, err
	}
	refreshExp := u.now().UTC().Add(u.tokens.RefreshTTL())
	saveMeta := meta
	if organizationID != nil && *organizationID != uuid.Nil {
		if internalID, err := u.repo.ResolveOrganizationInternalID(ctx, *organizationID); err == nil {
			saveMeta.OrganizationID = &internalID
		}
	}
	sessionID, err := u.repo.SaveRefresh(ctx, user.ID, tokenHash(rawRefresh), refreshExp, saveMeta)
	if err != nil {
		return model.Tokens{}, err
	}
	access, accessExp, err := u.tokens.IssueAccess(jwt.AccessInput{
		UserID: user.UUID, Roles: roles, IsSuperAdmin: isSuperAdmin,
		ImpersonatorID: impersonatorUUID, SessionID: sessionID, OrganizationID: organizationID,
	})
	if err != nil {
		return model.Tokens{}, err
	}
	return model.Tokens{
		AccessToken: access, RefreshToken: rawRefresh,
		TokenType: "Bearer", ExpiresIn: int64(accessExp.Sub(u.now().UTC()).Seconds()),
		RefreshExpiresAt: refreshExp,
	}, nil
}

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// burnPasswordVerify runs an Argon2 verification against a throwaway hash.
func burnPasswordVerify(raw string) {
	dummyHashOnce.Do(func() {
		dummyHash, _ = password.Hash("Timing-Equalizer-0")
	})
	if dummyHash != "" {
		_, _ = password.Verify(dummyHash, raw)
	}
}

func newOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
