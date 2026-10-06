package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type memRepo struct {
	users     map[int64]model.User
	byEmail   map[string]model.User
	byUUID    map[uuid.UUID]model.User
	userRoles map[int64][]string
	nextID    int64
	perms     map[string][]string
	roles     map[string]int64
	// Membership grants per organization (organization_member_roles).
	memberGrants map[uuid.UUID][]model.Grant
	// Refresh sessions revoked through RevokeSession.
	revokedSessions []uuid.UUID
	// refreshByUUID serves FindRefreshByUUID (TEC-284 legacy session tests).
	refreshByUUID map[uuid.UUID]model.RefreshSession
	// orgLocale holds {locale, timezone} per organization; center is the
	// brand center's pair (LocaleSources).
	orgLocale map[uuid.UUID][2]string
	center    [2]string
}

func (r *memRepo) LocaleSources(_ context.Context, userID int64, orgUUID *uuid.UUID, _ *int64) (i18n.Sources, error) {
	u, ok := r.users[userID]
	if !ok {
		return i18n.Sources{}, repository.ErrNotFound
	}
	src := i18n.Sources{
		UserLocale: u.Locale, UserTimezone: u.Timezone,
		CenterLocale: r.center[0], CenterTimezone: r.center[1],
	}
	if orgUUID != nil {
		if o, ok := r.orgLocale[*orgUUID]; ok {
			src.OrgLocale, src.OrgTimezone = o[0], o[1]
		}
	}
	return src, nil
}

func newMemRepo() *memRepo {
	r := &memRepo{
		users:     map[int64]model.User{},
		byEmail:   map[string]model.User{},
		byUUID:    map[uuid.UUID]model.User{},
		userRoles: map[int64][]string{},
		perms: map[string][]string{
			rbac.RoleSuperAdmin: {
				rbac.PermPlatformUsersRead, rbac.PermPlatformUsersWrite,
				rbac.PermPlatformRolesRead, rbac.PermPlatformRolesWrite,
				rbac.PermAuthSession,
			},
			rbac.RoleCustomer: {rbac.PermAuthSession, rbac.PermNotificationsRead},
		},
		roles: map[string]int64{rbac.RoleSuperAdmin: 1, rbac.RoleCustomer: 2},
	}
	return r
}

func (r *memRepo) CreateUser(_ context.Context, u model.User, emailVerified bool) (model.User, error) {
	r.nextID++
	u.ID = r.nextID
	u.UUID = uuid.New()
	if emailVerified {
		u.EmailVerified = true
	}
	r.users[u.ID] = u
	r.byEmail[u.Email] = u
	r.byUUID[u.UUID] = u
	return u, nil
}

func (r *memRepo) FindUserByEmail(_ context.Context, email string) (model.User, error) {
	u, ok := r.byEmail[email]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	return u, nil
}

func (r *memRepo) FindUserByUUID(_ context.Context, id uuid.UUID) (model.User, error) {
	u, ok := r.byUUID[id]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	return u, nil
}

func (r *memRepo) FindUserByID(_ context.Context, id int64) (model.User, error) {
	u, ok := r.users[id]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	return u, nil
}

func (r *memRepo) UpdateLastLogin(context.Context, int64) error { return nil }

func (r *memRepo) UpdatePassword(_ context.Context, userID int64, hash string) error {
	u, ok := r.users[userID]
	if !ok {
		return repository.ErrNotFound
	}
	u.PasswordHash = hash
	r.users[userID] = u
	r.byEmail[u.Email] = u
	r.byUUID[u.UUID] = u
	return nil
}

func (r *memRepo) UpdateProfile(_ context.Context, userUUID uuid.UUID, in model.ProfilePatch) (model.User, error) {
	u, ok := r.byUUID[userUUID]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	if in.Name != nil {
		u.Name = *in.Name
	}
	if in.Surname != nil {
		u.Surname = *in.Surname
	}
	if in.Locale != nil {
		u.Locale = *in.Locale
	}
	if in.Timezone != nil {
		u.Timezone = *in.Timezone
	}
	r.users[u.ID] = u
	r.byEmail[u.Email] = u
	r.byUUID[u.UUID] = u
	return u, nil
}

func (r *memRepo) UpdateUserPlatform(_ context.Context, id uuid.UUID, name, surname, status *string) (model.User, error) {
	u, ok := r.byUUID[id]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	if name != nil {
		u.Name = *name
	}
	if surname != nil {
		u.Surname = *surname
	}
	if status != nil {
		u.Status = *status
	}
	r.users[u.ID] = u
	r.byEmail[u.Email] = u
	r.byUUID[u.UUID] = u
	return u, nil
}

func (r *memRepo) ListUsersFiltered(_ context.Context, _ model.UserListFilter) ([]model.User, int64, error) {
	out := make([]model.User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, u)
	}
	return out, int64(len(out)), nil
}

func (r *memRepo) CountUsersWithRole(_ context.Context, roleSlug string) (int64, error) {
	var n int64
	for userID, roles := range r.userRoles {
		for _, role := range roles {
			if role == roleSlug && r.users[userID].Status != "disabled" {
				n++
			}
		}
	}
	return n, nil
}

func (r *memRepo) UpsertSuperAdmin(_ context.Context, email, name, surname, hash string) (model.User, bool, error) {
	if u, err := r.FindUserByEmail(context.Background(), email); err == nil {
		u.PasswordHash = hash
		u.Name = name
		u.Surname = surname
		u.Status = "active"
		r.users[u.ID] = u
		r.byEmail[u.Email] = u
		r.byUUID[u.UUID] = u
		r.userRoles[u.ID] = []string{rbac.RoleSuperAdmin}
		return u, false, nil
	}
	u, err := r.CreateUser(context.Background(), model.User{
		Email: email, PasswordHash: hash, Name: name, Surname: surname, Status: "active",
	}, true)
	if err != nil {
		return model.User{}, false, err
	}
	r.userRoles[u.ID] = []string{rbac.RoleSuperAdmin}
	return u, true, nil
}

func (r *memRepo) GetRoleIDBySlug(_ context.Context, slug string) (int64, error) {
	id, ok := r.roles[slug]
	if !ok {
		return 0, repository.ErrNotFound
	}
	return id, nil
}

func (r *memRepo) ListGrantsByRoleSlugs(_ context.Context, slugs []string) ([]model.Grant, error) {
	out := []model.Grant{}
	for _, slug := range slugs {
		for _, perm := range r.perms[slug] {
			scope := rbac.ScopeAll
			if def, ok := rbac.PermissionBySlug(perm); ok {
				scope = rbac.Broadest(def.Scopes)
			}
			out = append(out, model.Grant{Role: slug, Permission: perm, Scope: string(scope)})
		}
	}
	return out, nil
}

func (r *memRepo) ListMemberGrants(_ context.Context, userID int64, orgUUID uuid.UUID) ([]model.Grant, error) {
	return r.memberGrants[orgUUID], nil
}

func (r *memRepo) ListUserRoleSlugs(_ context.Context, userID int64) ([]string, error) {
	return r.userRoles[userID], nil
}

func (r *memRepo) UserHasRoleSlug(_ context.Context, userID int64, slug string) (bool, error) {
	for _, role := range r.userRoles[userID] {
		if role == slug {
			return true, nil
		}
	}
	return false, nil
}

func (r *memRepo) ListUserRolesByUserUUID(_ context.Context, userUUID uuid.UUID) ([]model.RoleSummary, error) {
	u, err := r.FindUserByUUID(context.Background(), userUUID)
	if err != nil {
		return nil, err
	}
	out := make([]model.RoleSummary, 0)
	for _, slug := range r.userRoles[u.ID] {
		out = append(out, model.RoleSummary{UUID: uuid.New(), Name: slug, Slug: slug, IsSystem: slug == rbac.RoleSuperAdmin})
	}
	return out, nil
}

func (r *memRepo) ListRolesForUserIDs(ctx context.Context, userIDs []int64) (map[int64][]model.RoleSummary, error) {
	out := make(map[int64][]model.RoleSummary, len(userIDs))
	for _, id := range userIDs {
		u, err := r.FindUserByID(ctx, id)
		if err != nil {
			continue
		}
		roles, err := r.ListUserRolesByUserUUID(ctx, u.UUID)
		if err != nil {
			return nil, err
		}
		out[id] = roles
	}
	return out, nil
}

func (r *memRepo) ListOAuthAccountsForUserIDs(ctx context.Context, userIDs []int64) (map[int64][]model.OAuthAccountRecord, error) {
	out := make(map[int64][]model.OAuthAccountRecord, len(userIDs))
	for _, id := range userIDs {
		rows, err := r.ListOAuthAccountsByUserID(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = rows
	}
	return out, nil
}

func (r *memRepo) ListPasskeysForUserIDs(ctx context.Context, userIDs []int64) (map[int64][]model.PasskeyRecord, error) {
	out := make(map[int64][]model.PasskeyRecord, len(userIDs))
	for _, id := range userIDs {
		rows, err := r.ListPasskeysByUserID(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = rows
	}
	return out, nil
}

func (r *memRepo) ReplaceUserRoles(_ context.Context, userID int64, roleIDs []int64) error {
	slugs := make([]string, 0, len(roleIDs))
	for slug, id := range r.roles {
		for _, rid := range roleIDs {
			if rid == id {
				slugs = append(slugs, slug)
			}
		}
	}
	r.userRoles[userID] = slugs
	return nil
}

func (r *memRepo) ResolveRoleIDsByUUIDs(_ context.Context, roleUUIDs []uuid.UUID) ([]int64, error) {
	return []int64{2}, nil
}

func (r *memRepo) SaveRefresh(context.Context, int64, string, time.Time, model.SessionMeta) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (r *memRepo) ListActiveSessions(context.Context, int64) ([]model.DeviceSession, error) {
	return nil, nil
}

func (r *memRepo) RevokeSession(_ context.Context, _ int64, sid uuid.UUID) error {
	r.revokedSessions = append(r.revokedSessions, sid)
	return nil
}

func (r *memRepo) RevokeOtherSessions(context.Context, int64, uuid.UUID) error {
	return nil
}

func (r *memRepo) GetRefreshSession(_ context.Context, _ string) (model.RefreshSession, error) {
	return model.RefreshSession{}, repository.ErrNotFound
}

func (r *memRepo) ResolveOrganizationInternalID(context.Context, uuid.UUID) (int64, error) {
	return 0, repository.ErrNotFound
}

func (r *memRepo) RevokeRefresh(context.Context, string) error { return nil }
func (r *memRepo) RotateRefresh(context.Context, string) error { return nil }
func (r *memRepo) FindRefreshAny(context.Context, string) (model.RefreshSession, error) {
	return model.RefreshSession{}, repository.ErrNotFound
}
func (r *memRepo) FindRefreshByUUID(_ context.Context, id uuid.UUID) (model.RefreshSession, error) {
	if s, ok := r.refreshByUUID[id]; ok {
		return s, nil
	}
	return model.RefreshSession{}, repository.ErrNotFound
}
func (r *memRepo) RevokeRefreshFamily(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *memRepo) RevokeMobileDeviceSessions(context.Context, int64, string, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *memRepo) RevokeAllRefresh(context.Context, int64) error { return nil }

func (r *memRepo) CreateOTP(context.Context, *int64, string, string, string, time.Time) error {
	return nil
}
func (r *memRepo) InvalidateOTPs(context.Context, string, string) error { return nil }
func (r *memRepo) GetActiveOTP(context.Context, string, string) (int64, string, int32, int32, error) {
	return 0, "", 0, 0, repository.ErrNotFound
}

func (r *memRepo) IncrementOTPAttempts(context.Context, int64) (int32, int32, error) {
	return 0, 0, nil
}
func (r *memRepo) ConsumeOTP(context.Context, int64) error { return nil }
func (r *memRepo) SetEmailVerified(_ context.Context, userID int64) (model.User, error) {
	u, ok := r.users[userID]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	u.EmailVerified = true
	r.users[userID] = u
	return u, nil
}

func (r *memRepo) GetPasskeyByCredentialID(context.Context, string) (model.PasskeyRecord, error) {
	return model.PasskeyRecord{}, repository.ErrNotFound
}

func (r *memRepo) ListPasskeysByUserID(context.Context, int64) ([]model.PasskeyRecord, error) {
	return nil, nil
}

func (r *memRepo) CreatePasskey(context.Context, int64, model.CreateAdapterAuthenticatorInput) (model.PasskeyRecord, error) {
	return model.PasskeyRecord{}, nil
}
func (r *memRepo) UpdatePasskeyCounter(context.Context, string, int64) error { return nil }
func (r *memRepo) UpdatePasskeyName(context.Context, uuid.UUID, uuid.UUID, *string) (model.PasskeyRecord, error) {
	return model.PasskeyRecord{}, repository.ErrNotFound
}
func (r *memRepo) DeletePasskeyByCredentialID(context.Context, string) error       { return nil }
func (r *memRepo) DeletePasskeyByUUID(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func (r *memRepo) GetOAuthAccountByProviderAccount(context.Context, string, string) (model.OAuthAccountRecord, error) {
	return model.OAuthAccountRecord{}, repository.ErrNotFound
}

func (r *memRepo) GetOAuthAccountByUserProvider(context.Context, int64, string) (model.OAuthAccountRecord, error) {
	return model.OAuthAccountRecord{}, repository.ErrNotFound
}

func (r *memRepo) ListOAuthAccountsByUserID(context.Context, int64) ([]model.OAuthAccountRecord, error) {
	return nil, nil
}

func (r *memRepo) CreateOAuthAccount(context.Context, model.CreateOAuthAccountInput) (model.OAuthAccountRecord, error) {
	return model.OAuthAccountRecord{}, nil
}

func (r *memRepo) DeleteOAuthAccountByProviderAccount(context.Context, string, string) error {
	return nil
}

func (r *memRepo) DeleteOAuthAccountByUserProvider(context.Context, int64, string) error {
	return repository.ErrNotFound
}

func (r *memRepo) GetUserTOTP(context.Context, int64) (model.UserTOTP, error) {
	return model.UserTOTP{}, repository.ErrNotFound
}

func (r *memRepo) UpsertUserTOTPSetup(context.Context, int64, string) (model.UserTOTP, error) {
	return model.UserTOTP{}, nil
}

func (r *memRepo) ConfirmUserTOTP(context.Context, int64, []string) (model.UserTOTP, error) {
	return model.UserTOTP{}, nil
}

func (r *memRepo) UpdateUserTOTPRecoveryHashes(context.Context, int64, []string) error { return nil }

func (r *memRepo) DeleteUserTOTP(context.Context, int64) error { return nil }

func (r *memRepo) ListRolesFiltered(context.Context, int32, int32, string) ([]model.RoleSummary, int64, error) {
	return []model.RoleSummary{{UUID: uuid.New(), Name: "Organization", Slug: rbac.RoleCustomer}}, 1, nil
}

func (r *memRepo) GetRoleByUUID(context.Context, uuid.UUID) (model.RoleSummary, error) {
	return model.RoleSummary{UUID: uuid.New(), Slug: rbac.RoleCustomer}, nil
}

func (r *memRepo) ListRolePermissionSlugs(context.Context, uuid.UUID) ([]string, error) {
	return []string{rbac.PermAuthSession}, nil
}

func (r *memRepo) CreateRole(_ context.Context, name, slug string, description *string) (model.RoleSummary, error) {
	return model.RoleSummary{UUID: uuid.New(), Name: name, Slug: slug}, nil
}

func (r *memRepo) UpdateRole(context.Context, uuid.UUID, *string, *string) (model.RoleSummary, error) {
	return model.RoleSummary{}, nil
}
func (r *memRepo) DeleteRole(context.Context, uuid.UUID) error                        { return nil }
func (r *memRepo) SetRolePermissions(context.Context, int64, map[string]string) error { return nil }
func (r *memRepo) ListRoleGrants(context.Context, uuid.UUID) ([]model.RoleGrant, error) {
	return []model.RoleGrant{}, nil
}
func (r *memRepo) ListPermissionsFiltered(context.Context, int32, int32, string) ([]model.PermissionSummary, int64, error) {
	return []model.PermissionSummary{{Slug: rbac.PermAuthSession, Name: "Auth session"}}, 1, nil
}

func TestRegisterLoginMeWithoutRoles(t *testing.T) {
	repo := newMemRepo()
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uc := New(repo, tokens)
	uc.SetAuthSettings(testAuthGate{
		passwordRegister: true,
		passwordLogin:    true,
		passkeyLogin:     true,
		registration:     true,
	})

	user, err := uc.Register(context.Background(), model.RegisterInput{
		Email: "user@example.com", Password: "Password1!", Name: "Test", Surname: "User",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err = uc.Login(context.Background(), "user@example.com", "Password1!", "", "", model.SessionMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	me, err := uc.Me(context.Background(), user.UUID, nil, nil)
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if len(me.Roles) != 0 {
		t.Fatalf("roles = %v", me.Roles)
	}
}

type testAuthGate struct {
	passwordRegister bool
	passwordLogin    bool
	passkeyLogin     bool
	registration     bool
	defaultRoleID    *int64
}

func (g testAuthGate) CanPasswordRegister(context.Context) (bool, error) {
	return g.passwordRegister && g.registration, nil
}

func (g testAuthGate) CanPasswordLogin(context.Context) (bool, error) {
	return g.passwordLogin, nil
}

func (g testAuthGate) CanPasskeyLogin(context.Context) (bool, error) {
	return g.passkeyLogin, nil
}

func (g testAuthGate) RegistrationEnabled(context.Context) (bool, error) {
	return g.registration, nil
}

func (g testAuthGate) DefaultRoleID(context.Context) (*int64, error) {
	return g.defaultRoleID, nil
}

func TestSuperAdminLogin(t *testing.T) {
	repo := newMemRepo()
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uc := New(repo, tokens)
	hash, _ := password.Hash("Password1!")
	user, _, err := repo.UpsertSuperAdmin(context.Background(), "admin@example.com", "Admin", "User", hash)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := uc.Login(context.Background(), "admin@example.com", "Password1!", "", "", model.SessionMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if tok.AccessToken == "" {
		t.Fatal("expected access token")
	}
	me, err := uc.Me(context.Background(), user.UUID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !me.User.IsSuperAdmin {
		t.Fatal("expected super admin")
	}
}

func TestLastSuperAdminProtection(t *testing.T) {
	repo := newMemRepo()
	tokens, _ := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	uc := New(repo, tokens)
	hash, _ := password.Hash("Password1!")
	user, _, _ := repo.UpsertSuperAdmin(context.Background(), "admin@example.com", "Admin", "User", hash)
	empty := []uuid.UUID{}
	if _, err := uc.PatchPlatformUser(context.Background(), user.UUID, model.PatchPlatformUserInput{RoleUUIDs: &empty}); !errors.Is(err, ErrLastSuperAdmin) {
		t.Fatalf("expected last super admin error, got %v", err)
	}
}

// revokedRepo simulates a refresh token already rotated by a concurrent call.
type revokedRepo struct{ *memRepo }

func (r revokedRepo) GetRefreshSession(context.Context, string) (model.RefreshSession, error) {
	return model.RefreshSession{UserID: 1}, nil
}

func (r revokedRepo) RevokeRefresh(context.Context, string) error { return repository.ErrNotFound }

func TestRefreshIsSingleUse(t *testing.T) {
	tokens, _ := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	uc := New(revokedRepo{newMemRepo()}, tokens)
	if _, err := uc.Refresh(context.Background(), "raw", model.SessionMeta{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for already-rotated token, got %v", err)
	}
	if err := uc.Logout(context.Background(), "raw"); err != nil {
		t.Fatalf("logout of already-revoked token must be a no-op, got %v", err)
	}
}

func TestLoginDisabledUserRequiresPassword(t *testing.T) {
	repo := newMemRepo()
	hash, _ := password.Hash("Secret123")
	_, _ = repo.CreateUser(context.Background(), model.User{Email: "d@x.io", PasswordHash: hash, Status: "disabled"}, true)
	tokens, _ := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	uc := New(repo, tokens)
	if _, err := uc.Login(context.Background(), "d@x.io", "wrong", "", "", model.SessionMeta{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password on disabled account must not reveal status, got %v", err)
	}
	if _, err := uc.Login(context.Background(), "d@x.io", "Secret123", "", "", model.SessionMeta{}); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled, got %v", err)
	}
}

// A user migrated from the legacy hub keeps the Laravel bcrypt hash ($2y$);
// the first successful login accepts it and upgrades it to Argon2id
// (TEC-254).
func TestLoginLegacyBcryptHash(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	const legacy = "$2y$10$Ncl7Rt8nC3FBorv/7YtFnuoBuNhkUwYPBuFp7em6/7fDphhJDmidO" // "eski-sifre"
	u, _ := repo.CreateUser(ctx, model.User{Email: "eski@x.io", PasswordHash: legacy, Status: "active"}, true)
	tokens, _ := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	uc := New(repo, tokens)

	if _, err := uc.Login(ctx, "eski@x.io", "yanlis", "", "", model.SessionMeta{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: got %v", err)
	}
	if repo.users[u.ID].PasswordHash != legacy {
		t.Fatal("a failed login must not touch the hash")
	}
	tok, err := uc.Login(ctx, "eski@x.io", "eski-sifre", "", "", model.SessionMeta{})
	if err != nil || tok.AccessToken == "" {
		t.Fatalf("legacy login: %v", err)
	}
	upgraded := repo.users[u.ID].PasswordHash
	if upgraded == legacy || password.NeedsRehash(upgraded) {
		t.Fatalf("hash not upgraded: %q", upgraded)
	}
	if _, err := uc.Login(ctx, "eski@x.io", "eski-sifre", "", "", model.SessionMeta{}); err != nil {
		t.Fatalf("login after upgrade: %v", err)
	}

	// A migrated account without a usable hash cannot log in by password.
	_, _ = repo.CreateUser(ctx, model.User{Email: "reset@x.io", PasswordHash: password.ResetRequired, Status: "active"}, true)
	if _, err := uc.Login(ctx, "reset@x.io", password.ResetRequired, "", "", model.SessionMeta{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("reset-required login: got %v", err)
	}
}

func TestNonSuperAdminCannotEscalateToSuperAdmin(t *testing.T) {
	repo := newMemRepo()
	tokens, _ := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	uc := New(repo, tokens)
	hash, _ := password.Hash("Secret123")
	sa, _ := repo.CreateUser(context.Background(), model.User{Email: "sa@x.io", PasswordHash: hash, Status: "active"}, true)
	repo.userRoles[sa.ID] = []string{rbac.RoleSuperAdmin}

	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{IsSuperAdmin: false})
	if err := uc.SetPlatformUserPassword(ctx, sa.UUID, "Another123"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-super-admin reset of super admin password: got %v", err)
	}
	name := "X"
	if _, err := uc.PatchPlatformUser(ctx, sa.UUID, model.PatchPlatformUserInput{Name: &name}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-super-admin patch of super admin: got %v", err)
	}
}

func TestCanSubscribeChannelScopes(t *testing.T) {
	uc := New(newMemRepo(), nil)
	me, other := uuid.New(), uuid.New()
	ctx := context.Background()
	cases := []struct {
		ch   string
		sa   bool
		want bool
	}{
		{"user:" + me.String(), false, true},
		{"user:" + other.String(), false, false},
		{"conversation:" + other.String(), false, false},
		{"system.notifications", false, false},
		{"system.notifications", true, true},
	}
	for _, c := range cases {
		got, err := uc.CanSubscribeChannel(ctx, me, c.sa, c.ch)
		if err != nil || got != c.want {
			t.Fatalf("%s (sa=%v): got %v,%v want %v", c.ch, c.sa, got, err, c.want)
		}
	}
}
