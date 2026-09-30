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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	searchadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/slug"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/google/uuid"
)

// ListPlatformUsers lists users for platform admin with roles and login methods.
func (u *AuthUseCase) ListPlatformUsers(
	ctx context.Context,
	limit, offset int32,
	q, status, roleSlug string,
) ([]model.PlatformUserDetail, int64, error) {
	rows, total, err := u.repo.ListUsersFiltered(ctx, limit, offset, q, status, roleSlug)
	if err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 {
		return []model.PlatformUserDetail{}, total, nil
	}

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	rolesByUser, err := u.repo.ListRolesForUserIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	oauthByUser, err := u.repo.ListOAuthAccountsForUserIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	passkeysByUser, err := u.repo.ListPasskeysForUserIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}

	out := make([]model.PlatformUserDetail, 0, len(rows))
	for _, row := range rows {
		roles := rolesByUser[row.ID]
		if roles == nil {
			roles = []model.RoleSummary{}
		}
		isSA := false
		for _, role := range roles {
			if role.Slug == rbac.RoleSuperAdmin {
				isSA = true
				break
			}
		}
		out = append(out, model.PlatformUserDetail{
			PublicUser:  model.ToPublicUser(row, isSA),
			Roles:       roles,
			AuthMethods: buildAuthMethods(row, oauthByUser[row.ID], passkeysByUser[row.ID]),
		})
	}
	return out, total, nil
}

// CreatePlatformUser creates a user from platform ops.
func (u *AuthUseCase) CreatePlatformUser(ctx context.Context, in model.CreatePlatformUserInput) (model.PublicUser, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	in.Surname = strings.TrimSpace(in.Surname)
	in.Status = strings.TrimSpace(in.Status)
	if in.Email == "" || in.Name == "" || in.Surname == "" || strings.TrimSpace(in.Password) == "" {
		return model.PublicUser{}, fmt.Errorf("%w: email, password, name, surname are required", ErrInvalidRequest)
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.Status != "active" && in.Status != "disabled" && in.Status != "pending" {
		return model.PublicUser{}, fmt.Errorf("%w: status must be active, disabled, or pending", ErrInvalidRequest)
	}
	if len(in.RoleUUIDs) > 0 {
		if err := u.guardSuperAdminRoleGrant(ctx, in.RoleUUIDs); err != nil {
			return model.PublicUser{}, err
		}
	}
	if _, err := u.repo.FindUserByEmail(ctx, in.Email); err == nil {
		return model.PublicUser{}, fmt.Errorf("%w: email already registered", ErrConflict)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return model.PublicUser{}, err
	}
	hash, err := passwordHash(in.Password)
	if err != nil {
		return model.PublicUser{}, err
	}
	user, err := u.repo.CreateUser(ctx, model.User{
		Email: in.Email, PasswordHash: hash, Name: in.Name, Surname: in.Surname, Status: in.Status,
	}, in.Status == "active")
	if err != nil {
		return model.PublicUser{}, err
	}
	if len(in.RoleUUIDs) > 0 {
		roleIDs, err := u.repo.ResolveRoleIDsByUUIDs(ctx, in.RoleUUIDs)
		if err != nil {
			return model.PublicUser{}, err
		}
		if err := u.repo.ReplaceUserRoles(ctx, user.ID, roleIDs); err != nil {
			return model.PublicUser{}, err
		}
	}
	isSA, err := u.repo.UserHasRoleSlug(ctx, user.ID, rbac.RoleSuperAdmin)
	if err != nil {
		return model.PublicUser{}, err
	}
	out := model.ToPublicUser(user, isSA)
	u.indexUserSearch(ctx, user.UUID.String())
	return out, nil
}

// GetPlatformUser returns a user with role summary.
func (u *AuthUseCase) GetPlatformUser(ctx context.Context, userUUID uuid.UUID) (model.PlatformUserDetail, error) {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.PlatformUserDetail{}, ErrNotFound
		}
		return model.PlatformUserDetail{}, err
	}
	roles, err := u.repo.ListUserRolesByUserUUID(ctx, userUUID)
	if err != nil {
		return model.PlatformUserDetail{}, err
	}
	if roles == nil {
		roles = []model.RoleSummary{}
	}
	isSA := false
	for _, role := range roles {
		if role.Slug == rbac.RoleSuperAdmin {
			isSA = true
			break
		}
	}
	oauth, err := u.repo.ListOAuthAccountsByUserID(ctx, user.ID)
	if err != nil {
		return model.PlatformUserDetail{}, err
	}
	passkeys, err := u.repo.ListPasskeysByUserID(ctx, user.ID)
	if err != nil {
		return model.PlatformUserDetail{}, err
	}
	return model.PlatformUserDetail{
		PublicUser:  model.ToPublicUser(user, isSA),
		Roles:       roles,
		AuthMethods: buildAuthMethods(user, oauth, passkeys),
	}, nil
}

func buildAuthMethods(
	user model.User,
	oauth []model.OAuthAccountRecord,
	passkeys []model.PasskeyRecord,
) []model.UserAuthMethod {
	out := make([]model.UserAuthMethod, 0, 1+len(oauth)+len(passkeys))
	if strings.TrimSpace(user.PasswordHash) != "" {
		linkedAt := user.CreatedAt
		if linkedAt.IsZero() {
			linkedAt = time.Now().UTC()
		}
		out = append(out, model.UserAuthMethod{
			Kind:     "password",
			LinkedAt: linkedAt,
		})
	}
	for _, pk := range passkeys {
		label := pk.Name
		out = append(out, model.UserAuthMethod{
			Kind:     "passkey",
			Label:    label,
			LinkedAt: pk.CreatedAt,
		})
	}
	for _, account := range oauth {
		provider := account.Provider
		out = append(out, model.UserAuthMethod{
			Kind:     "oauth",
			Provider: &provider,
			LinkedAt: account.CreatedAt,
		})
	}
	return out
}

// PatchPlatformUser updates profile fields and optional roles.
func (u *AuthUseCase) PatchPlatformUser(
	ctx context.Context,
	userUUID uuid.UUID,
	in model.PatchPlatformUserInput,
) (model.PublicUser, error) {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.PublicUser{}, ErrNotFound
		}
		return model.PublicUser{}, err
	}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if v == "" {
			return model.PublicUser{}, fmt.Errorf("%w: name cannot be empty", ErrInvalidRequest)
		}
		in.Name = &v
	}
	if in.Surname != nil {
		v := strings.TrimSpace(*in.Surname)
		if v == "" {
			return model.PublicUser{}, fmt.Errorf("%w: surname cannot be empty", ErrInvalidRequest)
		}
		in.Surname = &v
	}
	if in.Status != nil {
		v := strings.TrimSpace(*in.Status)
		if v != "active" && v != "disabled" && v != "pending" {
			return model.PublicUser{}, fmt.Errorf("%w: status must be active, disabled, or pending", ErrInvalidRequest)
		}
		in.Status = &v
	}
	hasSuperAdmin, err := u.repo.UserHasRoleSlug(ctx, user.ID, rbac.RoleSuperAdmin)
	if err != nil {
		return model.PublicUser{}, err
	}
	if hasSuperAdmin && !actorIsSuperAdmin(ctx) {
		return model.PublicUser{}, fmt.Errorf("%w: only a super admin can modify a super admin", ErrForbidden)
	}
	if in.RoleUUIDs != nil {
		if err := u.guardSuperAdminRoleGrant(ctx, *in.RoleUUIDs); err != nil {
			return model.PublicUser{}, err
		}
		roleIDs, err := u.repo.ResolveRoleIDsByUUIDs(ctx, *in.RoleUUIDs)
		if err != nil {
			return model.PublicUser{}, err
		}
		willHaveSuperAdmin := false
		for _, roleUUID := range *in.RoleUUIDs {
			role, err := u.repo.GetRoleByUUID(ctx, roleUUID)
			if err != nil {
				return model.PublicUser{}, err
			}
			if role.Slug == rbac.RoleSuperAdmin {
				willHaveSuperAdmin = true
				break
			}
		}
		if hasSuperAdmin && !willHaveSuperAdmin {
			count, err := u.repo.CountUsersWithRole(ctx, rbac.RoleSuperAdmin)
			if err != nil {
				return model.PublicUser{}, err
			}
			if count <= 1 {
				return model.PublicUser{}, ErrLastSuperAdmin
			}
		}
		if err := u.repo.ReplaceUserRoles(ctx, user.ID, roleIDs); err != nil {
			return model.PublicUser{}, err
		}
		hasSuperAdmin = willHaveSuperAdmin
	}
	if in.Status != nil && *in.Status == "disabled" && hasSuperAdmin {
		count, err := u.repo.CountUsersWithRole(ctx, rbac.RoleSuperAdmin)
		if err != nil {
			return model.PublicUser{}, err
		}
		if count <= 1 {
			return model.PublicUser{}, ErrLastSuperAdmin
		}
	}
	if in.Name == nil && in.Surname == nil && in.Status == nil && in.RoleUUIDs == nil {
		return model.PublicUser{}, fmt.Errorf("%w: at least one field is required", ErrInvalidRequest)
	}
	updated, err := u.repo.UpdateUserPlatform(ctx, userUUID, in.Name, in.Surname, in.Status)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.PublicUser{}, ErrNotFound
		}
		return model.PublicUser{}, err
	}
	out := model.ToPublicUser(updated, hasSuperAdmin)
	u.indexUserSearch(ctx, userUUID.String())
	return out, nil
}

// SetPlatformUserPassword admin-sets a password and revokes all refresh tokens.
func (u *AuthUseCase) SetPlatformUserPassword(ctx context.Context, userUUID uuid.UUID, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return fmt.Errorf("%w: password is required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if !actorIsSuperAdmin(ctx) {
		isSA, err := u.repo.UserHasRoleSlug(ctx, user.ID, rbac.RoleSuperAdmin)
		if err != nil {
			return err
		}
		if isSA {
			return fmt.Errorf("%w: only a super admin can set a super admin's password", ErrForbidden)
		}
	}
	hash, err := passwordHash(newPassword)
	if err != nil {
		return err
	}
	if err := u.repo.UpdatePassword(ctx, user.ID, hash); err != nil {
		return err
	}
	return u.repo.RevokeAllRefresh(ctx, user.ID)
}

// ListPlatformRoles lists roles.
func (u *AuthUseCase) ListPlatformRoles(ctx context.Context, limit, offset int32, q string) ([]model.RoleSummary, int64, error) {
	return u.repo.ListRolesFiltered(ctx, limit, offset, q)
}

// GetPlatformRole returns role detail with permissions.
func (u *AuthUseCase) GetPlatformRole(ctx context.Context, roleUUID uuid.UUID) (model.RoleDetail, error) {
	role, err := u.repo.GetRoleByUUID(ctx, roleUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.RoleDetail{}, ErrNotFound
		}
		return model.RoleDetail{}, err
	}
	perms, err := u.repo.ListRolePermissionSlugs(ctx, roleUUID)
	if err != nil {
		return model.RoleDetail{}, err
	}
	return model.RoleDetail{RoleSummary: role, PermissionSlugs: perms}, nil
}

// CreatePlatformRole creates a custom role.
func (u *AuthUseCase) CreatePlatformRole(ctx context.Context, in model.CreateRoleInput) (model.RoleDetail, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	if in.Name == "" || in.Slug == "" {
		return model.RoleDetail{}, fmt.Errorf("%w: name and slug are required", ErrInvalidRequest)
	}
	if rbac.IsSystemRole(in.Slug) {
		return model.RoleDetail{}, fmt.Errorf("%w: slug is reserved", ErrConflict)
	}
	normalized := slug.FromName(in.Slug)
	if normalized == "" {
		return model.RoleDetail{}, fmt.Errorf("%w: invalid slug", ErrInvalidRequest)
	}
	role, err := u.repo.CreateRole(ctx, in.Name, normalized, in.Description)
	if err != nil {
		return model.RoleDetail{}, err
	}
	roleID, err := u.repo.GetRoleIDBySlug(ctx, role.Slug)
	if err != nil {
		return model.RoleDetail{}, err
	}
	if err := u.repo.SetRolePermissions(ctx, roleID, uniqueStrings(in.PermissionSlugs)); err != nil {
		return model.RoleDetail{}, err
	}
	perms, err := u.repo.ListRolePermissionSlugs(ctx, role.UUID)
	if err != nil {
		return model.RoleDetail{}, err
	}
	out := model.RoleDetail{RoleSummary: role, PermissionSlugs: perms}
	u.indexRoleSearch(ctx, role.UUID.String())
	return out, nil
}

// PatchPlatformRole updates a non-system role.
func (u *AuthUseCase) PatchPlatformRole(ctx context.Context, roleUUID uuid.UUID, in model.PatchRoleInput) (model.RoleDetail, error) {
	role, err := u.repo.GetRoleByUUID(ctx, roleUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.RoleDetail{}, ErrNotFound
		}
		return model.RoleDetail{}, err
	}
	if role.IsSystem {
		return model.RoleDetail{}, ErrSystemRole
	}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if v == "" {
			return model.RoleDetail{}, fmt.Errorf("%w: name cannot be empty", ErrInvalidRequest)
		}
		in.Name = &v
	}
	if in.Name == nil && in.Description == nil && in.PermissionSlugs == nil {
		return model.RoleDetail{}, fmt.Errorf("%w: at least one field is required", ErrInvalidRequest)
	}
	updated, err := u.repo.UpdateRole(ctx, roleUUID, in.Name, in.Description)
	if err != nil {
		return model.RoleDetail{}, err
	}
	if in.PermissionSlugs != nil {
		roleID, err := u.repo.GetRoleIDBySlug(ctx, updated.Slug)
		if err != nil {
			return model.RoleDetail{}, err
		}
		if err := u.repo.SetRolePermissions(ctx, roleID, uniqueStrings(*in.PermissionSlugs)); err != nil {
			return model.RoleDetail{}, err
		}
	}
	perms, err := u.repo.ListRolePermissionSlugs(ctx, roleUUID)
	if err != nil {
		return model.RoleDetail{}, err
	}
	out := model.RoleDetail{RoleSummary: updated, PermissionSlugs: perms}
	u.indexRoleSearch(ctx, roleUUID.String())
	return out, nil
}

// DeletePlatformRole deletes a non-system role.
func (u *AuthUseCase) DeletePlatformRole(ctx context.Context, roleUUID uuid.UUID) error {
	role, err := u.repo.GetRoleByUUID(ctx, roleUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if role.IsSystem {
		return ErrSystemRole
	}
	if err := u.repo.DeleteRole(ctx, roleUUID); err != nil {
		return err
	}
	u.deleteRoleSearch(ctx, roleUUID.String())
	return nil
}

// ListPlatformPermissions lists the permission catalog.
func (u *AuthUseCase) ListPlatformPermissions(ctx context.Context, limit, offset int32, q string) ([]model.PermissionSummary, int64, error) {
	return u.repo.ListPermissionsFiltered(ctx, limit, offset, q)
}

// CanSubscribeChannel authorizes Centrifugo subscription channels.
func (u *AuthUseCase) CanSubscribeChannel(
	ctx context.Context,
	userUUID uuid.UUID,
	isSuperAdmin bool,
	channel string,
) (bool, error) {
	if channel == realtime.ChannelSystemNotifications {
		return isSuperAdmin, nil
	}
	if uid, ok := realtime.ParseUserChannel(channel); ok {
		return uid == userUUID, nil
	}
	// conversation:{uuid} used to be granted to any signed-in user without an
	// ownership check (cross-tenant eavesdropping once anything publishes
	// there). Nothing publishes to it today, so fail closed; a future
	// publisher must add an ownership check here.
	return false, nil
}

func passwordHash(raw string) (string, error) {
	return password.Hash(raw)
}

func (u *AuthUseCase) indexUserSearch(ctx context.Context, id string) {
	if u.searchIndexer == nil {
		return
	}
	u.searchIndexer.EnqueueUpsert(ctx, searchadapters.SpecUsers, id)
}

func (u *AuthUseCase) indexRoleSearch(ctx context.Context, id string) {
	if u.searchIndexer == nil {
		return
	}
	u.searchIndexer.EnqueueUpsert(ctx, searchadapters.SpecRoles, id)
}

func (u *AuthUseCase) deleteRoleSearch(ctx context.Context, id string) {
	if u.searchIndexer == nil {
		return
	}
	u.searchIndexer.EnqueueDelete(ctx, searchadapters.SpecRoles, id)
}

// actorIsSuperAdmin reports whether the request principal is a super admin.
// Calls without a principal (CLI / internal) are treated as trusted.
func actorIsSuperAdmin(ctx context.Context) bool {
	p, ok := authctx.PrincipalFrom(ctx)
	if !ok {
		return true
	}
	return p.IsSuperAdmin
}

// guardSuperAdminRoleGrant stops holders of platform.users.write from
// granting the super_admin role (to themselves or others) unless they are
// super admins already.
func (u *AuthUseCase) guardSuperAdminRoleGrant(ctx context.Context, roleUUIDs []uuid.UUID) error {
	if actorIsSuperAdmin(ctx) {
		return nil
	}
	for _, roleUUID := range roleUUIDs {
		role, err := u.repo.GetRoleByUUID(ctx, roleUUID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				continue
			}
			return err
		}
		if role.Slug == rbac.RoleSuperAdmin {
			return fmt.Errorf("%w: only a super admin can grant the super_admin role", ErrForbidden)
		}
	}
	return nil
}
