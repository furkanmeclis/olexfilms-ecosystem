package repository

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// Postgres implements auth persistence via sqlc.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres creates a repository.
func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

func (r *Postgres) withTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(r.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Postgres) CreateUser(ctx context.Context, u model.User, emailVerified bool) (model.User, error) {
	var verified pgtype.Timestamptz
	if emailVerified {
		verified = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}
	row, err := r.q.CreateUser(ctx, db.CreateUserParams{
		Email: u.Email, PasswordHash: u.PasswordHash, Name: u.Name, Surname: u.Surname,
		Status: u.Status, EmailVerifiedAt: verified,
	})
	if err != nil {
		return model.User{}, err
	}
	return mapUser(row), nil
}

func (r *Postgres) FindUserByEmail(ctx context.Context, email string) (model.User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return mapUser(row), nil
}

func (r *Postgres) FindUserByUUID(ctx context.Context, id uuid.UUID) (model.User, error) {
	row, err := r.q.GetUserByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return mapUser(row), nil
}

func (r *Postgres) FindUserByID(ctx context.Context, id int64) (model.User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return mapUser(row), nil
}

func (r *Postgres) UpdateLastLogin(ctx context.Context, userID int64) error {
	return r.q.UpdateUserLastLogin(ctx, userID)
}

func (r *Postgres) UpdatePassword(ctx context.Context, userID int64, hash string) error {
	return r.q.UpdateUserPasswordByID(ctx, db.UpdateUserPasswordByIDParams{ID: userID, PasswordHash: hash})
}

func (r *Postgres) UpsertSuperAdmin(ctx context.Context, email, name, surname, hash string) (model.User, bool, error) {
	existing, err := r.FindUserByEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return model.User{}, false, err
	}
	if errors.Is(err, ErrNotFound) {
		var user model.User
		err := r.withTx(ctx, func(q *db.Queries) error {
			row, err := q.CreateUser(ctx, db.CreateUserParams{
				Email: email, PasswordHash: hash, Name: name, Surname: surname, Status: "active",
				EmailVerifiedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			})
			if err != nil {
				return err
			}
			user = mapUser(row)
			return q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: user.ID, Slug: rbac.RoleSuperAdmin})
		})
		return user, true, err
	}
	if err := r.q.UpdateUserPasswordByID(ctx, db.UpdateUserPasswordByIDParams{ID: existing.ID, PasswordHash: hash}); err != nil {
		return model.User{}, false, err
	}
	if err := r.q.UpdateUserProfileBasics(ctx, db.UpdateUserProfileBasicsParams{
		ID: existing.ID, Name: name, Surname: surname, Status: "active",
	}); err != nil {
		return model.User{}, false, err
	}
	if err := r.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: existing.ID, Slug: rbac.RoleSuperAdmin}); err != nil {
		return model.User{}, false, err
	}
	user, err := r.FindUserByID(ctx, existing.ID)
	return user, false, err
}

func (r *Postgres) GetRoleIDBySlug(ctx context.Context, slug string) (int64, error) {
	role, err := r.q.GetRoleBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return role.ID, nil
}

func (r *Postgres) ListPermissionsByRoleSlug(ctx context.Context, slug string) ([]string, error) {
	if slug == rbac.RoleSuperAdmin {
		return r.q.ListAllPermissionSlugs(ctx)
	}
	perms, err := r.q.ListPermissionSlugsByRoleSlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if perms == nil {
		return []string{}, nil
	}
	return perms, nil
}

func (r *Postgres) ListUserRoleSlugs(ctx context.Context, userID int64) ([]string, error) {
	slugs, err := r.q.ListUserRoleSlugs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if slugs == nil {
		return []string{}, nil
	}
	return slugs, nil
}

func (r *Postgres) UserHasRoleSlug(ctx context.Context, userID int64, slug string) (bool, error) {
	return r.q.UserHasRoleSlug(ctx, db.UserHasRoleSlugParams{UserID: userID, Slug: slug})
}

func (r *Postgres) ListUserRolesByUserUUID(ctx context.Context, userUUID uuid.UUID) ([]model.RoleSummary, error) {
	rows, err := r.q.ListUserRolesByUserUUID(ctx, userUUID)
	if err != nil {
		return nil, err
	}
	out := make([]model.RoleSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapRole(row))
	}
	return out, nil
}

func (r *Postgres) ListRolesForUserIDs(ctx context.Context, userIDs []int64) (map[int64][]model.RoleSummary, error) {
	out := make(map[int64][]model.RoleSummary, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListRolesForUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		var desc *string
		if row.Description.Valid {
			v := row.Description.String
			desc = &v
		}
		out[row.UserID] = append(out[row.UserID], model.RoleSummary{
			UUID: row.Uuid, Name: row.Name, Slug: row.Slug, Description: desc, IsSystem: row.IsSystem,
		})
	}
	return out, nil
}

func (r *Postgres) ListOAuthAccountsForUserIDs(ctx context.Context, userIDs []int64) (map[int64][]model.OAuthAccountRecord, error) {
	out := make(map[int64][]model.OAuthAccountRecord, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListOAuthAccountsForUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = append(out[row.UserID], mapOAuthAccountFromUserIDs(row))
	}
	return out, nil
}

func (r *Postgres) ListPasskeysForUserIDs(ctx context.Context, userIDs []int64) (map[int64][]model.PasskeyRecord, error) {
	out := make(map[int64][]model.PasskeyRecord, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListWebAuthnCredentialsForUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = append(out[row.UserID], mapPasskey(row))
	}
	return out, nil
}

func (r *Postgres) ReplaceUserRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	return r.withTx(ctx, func(q *db.Queries) error {
		if err := q.ReplaceUserRoles(ctx, userID); err != nil {
			return err
		}
		for _, roleID := range roleIDs {
			if err := q.InsertUserRole(ctx, db.InsertUserRoleParams{UserID: userID, RoleID: roleID}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Postgres) ResolveRoleIDsByUUIDs(ctx context.Context, roleUUIDs []uuid.UUID) ([]int64, error) {
	out := make([]int64, 0, len(roleUUIDs))
	for _, id := range roleUUIDs {
		role, err := r.q.GetRoleByUUID(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		out = append(out, role.ID)
	}
	return out, nil
}

func (r *Postgres) SaveRefresh(ctx context.Context, userID int64, hash string, expiresAt time.Time, meta model.SessionMeta) (uuid.UUID, error) {
	var ua pgtype.Text
	if meta.UserAgent != "" {
		ua = pgtype.Text{String: meta.UserAgent, Valid: true}
	}
	var ip *netip.Addr
	if meta.IP != "" {
		if addr, err := netip.ParseAddr(meta.IP); err == nil {
			ip = &addr
		}
	}
	var impersonator pgtype.Int8
	if meta.ImpersonatorUserID != nil {
		impersonator = pgtype.Int8{Int64: *meta.ImpersonatorUserID, Valid: true}
	}
	var organizationID pgtype.Int8
	if meta.OrganizationID != nil {
		organizationID = pgtype.Int8{Int64: *meta.OrganizationID, Valid: true}
	}
	row, err := r.q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID: userID, TokenHash: hash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt.UTC(), Valid: true},
		UserAgent: ua, IpAddress: ip,
		ImpersonatorUserID: impersonator,
		OrganizationID:     organizationID,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return row.Uuid, nil
}

func (r *Postgres) GetRefreshSession(ctx context.Context, hash string) (model.RefreshSession, error) {
	row, err := r.q.GetValidRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.RefreshSession{}, ErrNotFound
		}
		return model.RefreshSession{}, err
	}
	out := model.RefreshSession{UUID: row.Uuid, UserID: row.UserID}
	if row.ImpersonatorUserID.Valid {
		id := row.ImpersonatorUserID.Int64
		out.ImpersonatorUserID = &id
	}
	if row.OrganizationID.Valid {
		org, err := r.q.GetOrganizationByID(ctx, row.OrganizationID.Int64)
		if err == nil {
			id := org.Uuid
			out.OrganizationUUID = &id
		}
	}
	return out, nil
}

func (r *Postgres) ResolveOrganizationInternalID(ctx context.Context, orgUUID uuid.UUID) (int64, error) {
	org, err := r.q.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return org.ID, nil
}

// RevokeRefresh revokes an active refresh token. It returns ErrNotFound when
// no active row matched, so callers can treat rotation as single-use.
func (r *Postgres) RevokeRefresh(ctx context.Context, hash string) error {
	n, err := r.q.RevokeRefreshTokenByHash(ctx, hash)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) RevokeAllRefresh(ctx context.Context, userID int64) error {
	return r.q.RevokeAllRefreshTokensForUser(ctx, userID)
}

func (r *Postgres) ListActiveSessions(ctx context.Context, userID int64) ([]model.DeviceSession, error) {
	rows, err := r.q.ListActiveRefreshTokensByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.DeviceSession, 0, len(rows))
	for _, row := range rows {
		item := model.DeviceSession{
			UUID:         row.Uuid,
			CreatedAt:    row.CreatedAt.Time,
			ExpiresAt:    row.ExpiresAt.Time,
			Impersonated: row.ImpersonatorUserID.Valid,
		}
		if row.UserAgent.Valid && row.UserAgent.String != "" {
			ua := row.UserAgent.String
			item.UserAgent = &ua
		}
		if row.IpAddress != nil {
			ip := row.IpAddress.String()
			item.IPAddress = &ip
		}
		out = append(out, item)
	}
	return out, nil
}

func (r *Postgres) RevokeSession(ctx context.Context, userID int64, sessionUUID uuid.UUID) error {
	n, err := r.q.RevokeRefreshTokenByUUIDForUser(ctx, db.RevokeRefreshTokenByUUIDForUserParams{
		Uuid: sessionUUID, UserID: userID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) RevokeOtherSessions(ctx context.Context, userID int64, keep uuid.UUID) error {
	return r.q.RevokeOtherRefreshTokensForUser(ctx, db.RevokeOtherRefreshTokensForUserParams{
		UserID: userID, Uuid: keep,
	})
}

func (r *Postgres) UpdateProfile(ctx context.Context, userUUID uuid.UUID, name, surname, locale *string) (model.User, error) {
	params := db.UpdateUserProfileByUUIDParams{Uuid: userUUID}
	if name != nil {
		params.Name = pgtype.Text{String: *name, Valid: true}
	}
	if surname != nil {
		params.Surname = pgtype.Text{String: *surname, Valid: true}
	}
	if locale != nil {
		params.Locale = pgtype.Text{String: *locale, Valid: true}
	}
	row, err := r.q.UpdateUserProfileByUUID(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return mapUser(row), nil
}

func (r *Postgres) SetEmailVerified(ctx context.Context, userID int64) (model.User, error) {
	row, err := r.q.SetUserEmailVerified(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return mapUser(row), nil
}

func (r *Postgres) CreateOTP(ctx context.Context, userID *int64, email, codeHash, otpType string, expiresAt time.Time) error {
	var uid pgtype.Int8
	if userID != nil {
		uid = pgtype.Int8{Int64: *userID, Valid: true}
	}
	_, err := r.q.CreateOTPCode(ctx, db.CreateOTPCodeParams{
		UserID: uid, Email: email, CodeHash: codeHash, Type: otpType,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt.UTC(), Valid: true}, MaxAttempts: 5,
	})
	return err
}

func (r *Postgres) InvalidateOTPs(ctx context.Context, email, otpType string) error {
	return r.q.InvalidateActiveOTPs(ctx, db.InvalidateActiveOTPsParams{Email: email, Type: otpType})
}

func (r *Postgres) GetActiveOTP(ctx context.Context, email, otpType string) (id int64, codeHash string, attempts, maxAttempts int32, err error) {
	row, err := r.q.GetActiveOTPByEmailType(ctx, db.GetActiveOTPByEmailTypeParams{Email: email, Type: otpType})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", 0, 0, ErrNotFound
		}
		return 0, "", 0, 0, err
	}
	return row.ID, row.CodeHash, row.AttemptCount, row.MaxAttempts, nil
}

func (r *Postgres) IncrementOTPAttempts(ctx context.Context, id int64) (attempts, maxAttempts int32, err error) {
	row, err := r.q.IncrementOTPAttempts(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	return row.AttemptCount, row.MaxAttempts, nil
}

func (r *Postgres) ConsumeOTP(ctx context.Context, id int64) error {
	return r.q.ConsumeOTP(ctx, id)
}

func (r *Postgres) ListUsersFiltered(ctx context.Context, limit, offset int32, q, status, roleSlug string) ([]model.User, int64, error) {
	params := db.ListUsersFilteredParams{LimitCount: limit, OffsetCount: offset}
	countParams := db.CountUsersParams{}
	if q != "" {
		params.Q = pgtype.Text{String: q, Valid: true}
		countParams.Q = params.Q
	}
	if status != "" {
		params.Status = pgtype.Text{String: status, Valid: true}
		countParams.Status = params.Status
	}
	if roleSlug != "" {
		params.RoleSlug = pgtype.Text{String: roleSlug, Valid: true}
		countParams.RoleSlug = params.RoleSlug
	}
	rows, err := r.q.ListUsersFiltered(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountUsers(ctx, countParams)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapUser(row))
	}
	return out, total, nil
}

func (r *Postgres) UpdateUserPlatform(ctx context.Context, id uuid.UUID, name, surname, status *string) (model.User, error) {
	params := db.UpdateUserPlatformParams{Uuid: id}
	if name != nil {
		params.Name = pgtype.Text{String: *name, Valid: true}
	}
	if surname != nil {
		params.Surname = pgtype.Text{String: *surname, Valid: true}
	}
	if status != nil {
		params.Status = pgtype.Text{String: *status, Valid: true}
	}
	row, err := r.q.UpdateUserPlatform(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return mapUser(row), nil
}

func (r *Postgres) CountUsersWithRole(ctx context.Context, roleSlug string) (int64, error) {
	return r.q.CountUsersWithRole(ctx, roleSlug)
}

func (r *Postgres) ListRolesFiltered(ctx context.Context, limit, offset int32, q string) ([]model.RoleSummary, int64, error) {
	params := db.ListRolesFilteredParams{LimitCount: limit, OffsetCount: offset}
	if q != "" {
		params.Q = pgtype.Text{String: q, Valid: true}
	}
	rows, err := r.q.ListRolesFiltered(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountRoles(ctx, params.Q)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.RoleSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapRole(row))
	}
	return out, total, nil
}

func (r *Postgres) GetRoleByUUID(ctx context.Context, id uuid.UUID) (model.RoleSummary, error) {
	row, err := r.q.GetRoleByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.RoleSummary{}, ErrNotFound
		}
		return model.RoleSummary{}, err
	}
	return mapRole(row), nil
}

func (r *Postgres) ListRolePermissionSlugs(ctx context.Context, roleUUID uuid.UUID) ([]string, error) {
	slugs, err := r.q.ListRolePermissionSlugsByRoleUUID(ctx, roleUUID)
	if err != nil {
		return nil, err
	}
	if slugs == nil {
		return []string{}, nil
	}
	return slugs, nil
}

func (r *Postgres) CreateRole(ctx context.Context, name, slug string, description *string) (model.RoleSummary, error) {
	var desc pgtype.Text
	if description != nil {
		desc = pgtype.Text{String: *description, Valid: true}
	}
	row, err := r.q.CreateRole(ctx, db.CreateRoleParams{Name: name, Slug: slug, Description: desc})
	if err != nil {
		return model.RoleSummary{}, err
	}
	return mapRole(row), nil
}

func (r *Postgres) UpdateRole(ctx context.Context, roleUUID uuid.UUID, name, description *string) (model.RoleSummary, error) {
	params := db.UpdateRoleParams{Uuid: roleUUID}
	if name != nil {
		params.Name = pgtype.Text{String: *name, Valid: true}
	}
	if description != nil {
		params.Description = pgtype.Text{String: *description, Valid: true}
	}
	row, err := r.q.UpdateRole(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.RoleSummary{}, ErrNotFound
		}
		return model.RoleSummary{}, err
	}
	return mapRole(row), nil
}

func (r *Postgres) DeleteRole(ctx context.Context, roleUUID uuid.UUID) error {
	err := r.q.DeleteRole(ctx, roleUUID)
	if err != nil {
		return err
	}
	return nil
}

func (r *Postgres) SetRolePermissions(ctx context.Context, roleID int64, permissionSlugs []string) error {
	return r.withTx(ctx, func(q *db.Queries) error {
		if err := q.SetRolePermissions(ctx, roleID); err != nil {
			return err
		}
		for _, slug := range permissionSlugs {
			perm, err := q.GetPermissionBySlug(ctx, slug)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("%w: unknown permission %s", ErrNotFound, slug)
				}
				return err
			}
			if err := q.InsertRolePermission(ctx, db.InsertRolePermissionParams{RoleID: roleID, PermissionID: perm.ID}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Postgres) ListPermissionsFiltered(ctx context.Context, limit, offset int32, q string) ([]model.PermissionSummary, int64, error) {
	params := db.ListPermissionsFilteredParams{LimitCount: limit, OffsetCount: offset}
	if q != "" {
		params.Q = pgtype.Text{String: q, Valid: true}
	}
	rows, err := r.q.ListPermissionsFiltered(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountPermissions(ctx, params.Q)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.PermissionSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.PermissionSummary{UUID: row.Uuid, Name: row.Name, Slug: row.Slug})
	}
	return out, total, nil
}

func mapUser(row db.User) model.User {
	locale := row.Locale
	if locale == "" {
		locale = "tr"
	}
	return model.User{
		ID: row.ID, UUID: row.Uuid, Email: row.Email, PasswordHash: row.PasswordHash,
		Name: row.Name, Surname: row.Surname, Status: row.Status, Locale: locale,
		EmailVerified: row.EmailVerifiedAt.Valid, CreatedAt: row.CreatedAt.Time,
	}
}

func mapRole(row db.Role) model.RoleSummary {
	var desc *string
	if row.Description.Valid {
		v := row.Description.String
		desc = &v
	}
	return model.RoleSummary{
		UUID: row.Uuid, Name: row.Name, Slug: row.Slug, Description: desc, IsSystem: row.IsSystem,
	}
}

func mapPasskey(row db.WebauthnCredential) model.PasskeyRecord {
	var transports *string
	if row.Transports.Valid {
		v := row.Transports.String
		transports = &v
	}
	var name *string
	if row.Name.Valid {
		v := row.Name.String
		name = &v
	}
	var lastUsed *time.Time
	if row.LastUsedAt.Valid {
		t := row.LastUsedAt.Time
		lastUsed = &t
	}
	return model.PasskeyRecord{
		ID: row.ID, UUID: row.Uuid, UserID: row.UserID, CredentialID: row.CredentialID,
		PublicKey: row.PublicKey, Counter: row.Counter, DeviceType: row.DeviceType,
		BackedUp: row.BackedUp, Transports: transports, ProviderAccountID: row.ProviderAccountID,
		Name: name, LastUsedAt: lastUsed, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func (r *Postgres) GetPasskeyByCredentialID(ctx context.Context, credentialID string) (model.PasskeyRecord, error) {
	row, err := r.q.GetWebAuthnCredentialByCredentialID(ctx, credentialID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.PasskeyRecord{}, ErrNotFound
		}
		return model.PasskeyRecord{}, err
	}
	return mapPasskey(row), nil
}

func (r *Postgres) ListPasskeysByUserID(ctx context.Context, userID int64) ([]model.PasskeyRecord, error) {
	rows, err := r.q.ListWebAuthnCredentialsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.PasskeyRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapPasskey(row))
	}
	return out, nil
}

func (r *Postgres) CreatePasskey(ctx context.Context, userID int64, in model.CreateAdapterAuthenticatorInput) (model.PasskeyRecord, error) {
	var transports pgtype.Text
	if in.Transports != nil && strings.TrimSpace(*in.Transports) != "" {
		transports = pgtype.Text{String: strings.TrimSpace(*in.Transports), Valid: true}
	}
	deviceType := strings.TrimSpace(in.CredentialDeviceType)
	if deviceType == "" {
		deviceType = "unknown"
	}
	row, err := r.q.CreateWebAuthnCredential(ctx, db.CreateWebAuthnCredentialParams{
		UserID: userID, CredentialID: strings.TrimSpace(in.CredentialID),
		PublicKey: strings.TrimSpace(in.CredentialPublicKey), Counter: in.Counter,
		DeviceType: deviceType, BackedUp: in.CredentialBackedUp, Transports: transports,
		ProviderAccountID: strings.TrimSpace(in.ProviderAccountID), Name: pgtype.Text{},
	})
	if err != nil {
		return model.PasskeyRecord{}, err
	}
	return mapPasskey(row), nil
}

func (r *Postgres) UpdatePasskeyCounter(ctx context.Context, credentialID string, counter int64) error {
	return r.q.UpdateWebAuthnCredentialCounter(ctx, db.UpdateWebAuthnCredentialCounterParams{
		CredentialID: credentialID, Counter: counter,
	})
}

func (r *Postgres) UpdatePasskeyName(ctx context.Context, userUUID, passkeyUUID uuid.UUID, name *string) (model.PasskeyRecord, error) {
	var nameParam pgtype.Text
	if name != nil {
		nameParam = pgtype.Text{String: *name, Valid: true}
	}
	row, err := r.q.UpdateWebAuthnCredentialName(ctx, db.UpdateWebAuthnCredentialNameParams{
		Uuid: passkeyUUID, Uuid_2: userUUID, Name: nameParam,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.PasskeyRecord{}, ErrNotFound
		}
		return model.PasskeyRecord{}, err
	}
	return mapPasskey(row), nil
}

func (r *Postgres) DeletePasskeyByCredentialID(ctx context.Context, credentialID string) error {
	return r.q.DeleteWebAuthnCredentialByCredentialID(ctx, credentialID)
}

func (r *Postgres) DeletePasskeyByUUID(ctx context.Context, userUUID, passkeyUUID uuid.UUID) error {
	return nil
}

func mapOAuthAccount(row db.GetOAuthAccountByProviderAccountRow) model.OAuthAccountRecord {
	return mapOAuthAccountRow(
		row.ID, row.Uuid, row.UserID, row.UserUuid, row.Provider, row.ProviderAccountID,
		row.Type, row.GithubLogin, row.CreatedAt, row.UpdatedAt,
	)
}

func mapOAuthAccountFromUserProvider(row db.GetOAuthAccountByUserProviderRow) model.OAuthAccountRecord {
	return mapOAuthAccountRow(
		row.ID, row.Uuid, row.UserID, row.UserUuid, row.Provider, row.ProviderAccountID,
		row.Type, row.GithubLogin, row.CreatedAt, row.UpdatedAt,
	)
}

func mapOAuthAccountFromList(row db.ListOAuthAccountsByUserIDRow) model.OAuthAccountRecord {
	return mapOAuthAccountRow(
		row.ID, row.Uuid, row.UserID, row.UserUuid, row.Provider, row.ProviderAccountID,
		row.Type, row.GithubLogin, row.CreatedAt, row.UpdatedAt,
	)
}

func mapOAuthAccountFromUserIDs(row db.ListOAuthAccountsForUserIDsRow) model.OAuthAccountRecord {
	return mapOAuthAccountRow(
		row.ID, row.Uuid, row.UserID, row.UserUuid, row.Provider, row.ProviderAccountID,
		row.Type, row.GithubLogin, row.CreatedAt, row.UpdatedAt,
	)
}

func mapOAuthAccountFromCreate(row db.OauthAccount, userUUID uuid.UUID) model.OAuthAccountRecord {
	var login *string
	if row.GithubLogin.Valid {
		v := row.GithubLogin.String
		login = &v
	}
	return model.OAuthAccountRecord{
		ID: row.ID, UUID: row.Uuid, UserID: row.UserID, UserUUID: userUUID,
		Provider: row.Provider, ProviderAccountID: row.ProviderAccountID, Type: row.Type,
		GitHubLogin: login, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func mapOAuthAccountRow(
	id int64,
	accountUUID uuid.UUID,
	userID int64,
	userUUID uuid.UUID,
	provider string,
	providerAccountID string,
	accountType string,
	githubLogin pgtype.Text,
	createdAt pgtype.Timestamptz,
	updatedAt pgtype.Timestamptz,
) model.OAuthAccountRecord {
	var login *string
	if githubLogin.Valid {
		v := githubLogin.String
		login = &v
	}
	return model.OAuthAccountRecord{
		ID: id, UUID: accountUUID, UserID: userID, UserUUID: userUUID,
		Provider: provider, ProviderAccountID: providerAccountID, Type: accountType,
		GitHubLogin: login, CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time,
	}
}

func (r *Postgres) GetOAuthAccountByProviderAccount(ctx context.Context, provider, providerAccountID string) (model.OAuthAccountRecord, error) {
	row, err := r.q.GetOAuthAccountByProviderAccount(ctx, db.GetOAuthAccountByProviderAccountParams{
		Provider: provider, ProviderAccountID: providerAccountID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.OAuthAccountRecord{}, ErrNotFound
		}
		return model.OAuthAccountRecord{}, err
	}
	return mapOAuthAccount(row), nil
}

func (r *Postgres) GetOAuthAccountByUserProvider(ctx context.Context, userID int64, provider string) (model.OAuthAccountRecord, error) {
	row, err := r.q.GetOAuthAccountByUserProvider(ctx, db.GetOAuthAccountByUserProviderParams{
		UserID: userID, Provider: provider,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.OAuthAccountRecord{}, ErrNotFound
		}
		return model.OAuthAccountRecord{}, err
	}
	return mapOAuthAccountFromUserProvider(row), nil
}

func (r *Postgres) ListOAuthAccountsByUserID(ctx context.Context, userID int64) ([]model.OAuthAccountRecord, error) {
	rows, err := r.q.ListOAuthAccountsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.OAuthAccountRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapOAuthAccountFromList(row))
	}
	return out, nil
}

func (r *Postgres) CreateOAuthAccount(ctx context.Context, in model.CreateOAuthAccountInput) (model.OAuthAccountRecord, error) {
	params := db.CreateOAuthAccountParams{
		UserID: in.UserID, Provider: in.Provider, ProviderAccountID: in.ProviderAccountID, Type: in.Type,
	}
	if in.AccessTokenEnc != nil {
		params.AccessTokenEnc = pgtype.Text{String: *in.AccessTokenEnc, Valid: true}
	}
	if in.RefreshTokenEnc != nil {
		params.RefreshTokenEnc = pgtype.Text{String: *in.RefreshTokenEnc, Valid: true}
	}
	if in.ExpiresAt != nil {
		params.ExpiresAt = pgtype.Timestamptz{Time: *in.ExpiresAt, Valid: true}
	}
	if in.TokenType != nil {
		params.TokenType = pgtype.Text{String: *in.TokenType, Valid: true}
	}
	if in.Scope != nil {
		params.Scope = pgtype.Text{String: *in.Scope, Valid: true}
	}
	if in.GitHubLogin != nil {
		params.GithubLogin = pgtype.Text{String: *in.GitHubLogin, Valid: true}
	}
	row, err := r.q.CreateOAuthAccount(ctx, params)
	if err != nil {
		return model.OAuthAccountRecord{}, err
	}
	user, err := r.FindUserByID(ctx, in.UserID)
	if err != nil {
		return model.OAuthAccountRecord{}, err
	}
	return mapOAuthAccountFromCreate(row, user.UUID), nil
}

func (r *Postgres) DeleteOAuthAccountByProviderAccount(ctx context.Context, provider, providerAccountID string) error {
	if err := r.q.DeleteOAuthAccountByProviderAccount(ctx, db.DeleteOAuthAccountByProviderAccountParams{
		Provider: provider, ProviderAccountID: providerAccountID,
	}); err != nil {
		return err
	}
	return nil
}

func (r *Postgres) DeleteOAuthAccountByUserProvider(ctx context.Context, userID int64, provider string) error {
	_, err := r.q.GetOAuthAccountByUserProvider(ctx, db.GetOAuthAccountByUserProviderParams{
		UserID: userID, Provider: provider,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return r.q.DeleteOAuthAccountByUserProvider(ctx, db.DeleteOAuthAccountByUserProviderParams{
		UserID: userID, Provider: provider,
	})
}

func (r *Postgres) GetUserTOTP(ctx context.Context, userID int64) (model.UserTOTP, error) {
	row, err := r.q.GetUserTOTPByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.UserTOTP{}, ErrNotFound
		}
		return model.UserTOTP{}, err
	}
	return mapUserTOTP(row), nil
}

func (r *Postgres) UpsertUserTOTPSetup(ctx context.Context, userID int64, secretEnc string) (model.UserTOTP, error) {
	row, err := r.q.UpsertUserTOTPSetup(ctx, db.UpsertUserTOTPSetupParams{
		UserID: userID, SecretEnc: secretEnc,
	})
	if err != nil {
		return model.UserTOTP{}, err
	}
	return mapUserTOTP(row), nil
}

func (r *Postgres) ConfirmUserTOTP(ctx context.Context, userID int64, recoveryHashes []string) (model.UserTOTP, error) {
	row, err := r.q.ConfirmUserTOTP(ctx, db.ConfirmUserTOTPParams{
		UserID: userID, RecoveryHashes: recoveryHashes,
	})
	if err != nil {
		return model.UserTOTP{}, err
	}
	return mapUserTOTP(row), nil
}

func (r *Postgres) UpdateUserTOTPRecoveryHashes(ctx context.Context, userID int64, recoveryHashes []string) error {
	return r.q.UpdateUserTOTPRecoveryHashes(ctx, db.UpdateUserTOTPRecoveryHashesParams{
		UserID: userID, RecoveryHashes: recoveryHashes,
	})
}

func (r *Postgres) DeleteUserTOTP(ctx context.Context, userID int64) error {
	return r.q.DeleteUserTOTP(ctx, userID)
}

func mapUserTOTP(row db.UserTotp) model.UserTOTP {
	out := model.UserTOTP{
		UserID:         row.UserID,
		SecretEnc:      row.SecretEnc,
		Enabled:        row.Enabled,
		RecoveryHashes: row.RecoveryHashes,
	}
	if row.ConfirmedAt.Valid {
		t := row.ConfirmedAt.Time.UTC()
		out.ConfirmedAt = &t
	}
	if out.RecoveryHashes == nil {
		out.RecoveryHashes = []string{}
	}
	return out
}
