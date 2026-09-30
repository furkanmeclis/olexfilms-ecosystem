package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const ResourceUsers = "platform.users"

// UsersAdapter bulk actions for platform users.
type UsersAdapter struct {
	q *db.Queries
}

func NewUsers(q *db.Queries) *UsersAdapter {
	return &UsersAdapter{q: q}
}

func (a *UsersAdapter) Resource() string { return ResourceUsers }

func (a *UsersAdapter) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "disable", LabelKey: "bulk.actions.users.disable",
			Permission: rbac.PermPlatformUsersBulkDisable, Destructive: true, Reversible: true,
			ConfirmKey: "bulk.confirm.users.disable",
		},
		{
			ID: "enable", LabelKey: "bulk.actions.users.enable",
			Permission: rbac.PermPlatformUsersBulkEnable, Destructive: false, Reversible: true,
		},
	}
}

func (a *UsersAdapter) ResolveTargets(ctx context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
	if target.Scope == "ids" {
		return uniqueNonEmpty(target.IDs)
	}
	if target.Scope != "query" {
		return nil, fmt.Errorf("invalid target scope")
	}
	rows, err := a.q.ListUserUUIDsForBulk(ctx, db.ListUserUUIDsForBulkParams{
		Q: textArg(target.Query["q"]), Status: textArg(target.Query["status"]),
		RoleSlug: textArg(target.Query["role"]),
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, id := range rows {
		out = append(out, id.String())
	}
	return out, nil
}

func (a *UsersAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	user, err := a.q.GetUserByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	roleSlugs, err := a.q.ListUserRoleSlugs(ctx, user.ID)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	previous := map[string]any{
		"status":     user.Status,
		"role_slugs": roleSlugs,
	}
	switch action {
	case "disable":
		if user.Status == "disabled" {
			return bulkengine.BulkItemResult{
				EntityUUID: entityUUID, EntityType: "user", OK: true, Op: "update", Previous: previous,
			}, nil
		}
		if err := a.guardLastSuperAdmin(ctx, user.ID, "disabled"); err != nil {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
		}
		_, err = a.q.UpdateUserPlatform(ctx, db.UpdateUserPlatformParams{
			Uuid: id, Status: pgtype.Text{String: "disabled", Valid: true},
		})
	case "enable":
		if user.Status == "active" {
			return bulkengine.BulkItemResult{
				EntityUUID: entityUUID, EntityType: "user", OK: true, Op: "update", Previous: previous,
			}, nil
		}
		_, err = a.q.UpdateUserPlatform(ctx, db.UpdateUserPlatformParams{
			Uuid: id, Status: pgtype.Text{String: "active", Valid: true},
		})
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "user", OK: true, Op: "update", Previous: previous,
	}, nil
}

func (a *UsersAdapter) RevertItem(ctx context.Context, action, entityUUID string, previous map[string]any) error {
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	user, err := a.q.GetUserByUUID(ctx, id)
	if err != nil {
		return err
	}
	prevStatus, _ := previous["status"].(string)
	if prevStatus == "" {
		prevStatus = "active"
	}
	if action == "disable" && prevStatus != "disabled" {
		_, err = a.q.UpdateUserPlatform(ctx, db.UpdateUserPlatformParams{
			Uuid: id, Status: pgtype.Text{String: prevStatus, Valid: true},
		})
		if err != nil {
			return err
		}
	}
	if action == "enable" && prevStatus == "disabled" {
		_, err = a.q.UpdateUserPlatform(ctx, db.UpdateUserPlatformParams{
			Uuid: id, Status: pgtype.Text{String: "disabled", Valid: true},
		})
		if err != nil {
			return err
		}
	}
	if slugs, ok := previous["role_slugs"].([]any); ok {
		_ = a.q.ReplaceUserRoles(ctx, user.ID)
		for _, s := range slugs {
			if slug, ok := s.(string); ok && slug != "" {
				_ = a.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: user.ID, Slug: slug})
			}
		}
	} else if slugs, ok := previous["role_slugs"].([]string); ok {
		_ = a.q.ReplaceUserRoles(ctx, user.ID)
		for _, slug := range slugs {
			if slug != "" {
				_ = a.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: user.ID, Slug: slug})
			}
		}
	}
	return nil
}

func (a *UsersAdapter) guardLastSuperAdmin(ctx context.Context, userID int64, nextStatus string) error {
	if nextStatus != "disabled" {
		return nil
	}
	has, err := a.q.UserHasRoleSlug(ctx, db.UserHasRoleSlugParams{UserID: userID, Slug: rbac.RoleSuperAdmin})
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	count, err := a.q.CountUsersWithRole(ctx, rbac.RoleSuperAdmin)
	if err != nil {
		return err
	}
	if count <= 1 {
		return fmt.Errorf("cannot disable the last super admin")
	}
	return nil
}

const ResourceRoles = "platform.roles"

// RolesAdapter bulk actions for platform roles.
type RolesAdapter struct {
	q *db.Queries
}

func NewRoles(q *db.Queries) *RolesAdapter {
	return &RolesAdapter{q: q}
}

func (a *RolesAdapter) Resource() string { return ResourceRoles }

func (a *RolesAdapter) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "delete", LabelKey: "bulk.actions.roles.delete",
			Permission: rbac.PermPlatformRolesBulkDelete, Destructive: true, Reversible: true,
			ConfirmKey: "bulk.confirm.roles.delete",
		},
	}
}

func (a *RolesAdapter) ResolveTargets(ctx context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
	if target.Scope == "ids" {
		return uniqueNonEmpty(target.IDs)
	}
	if target.Scope != "query" {
		return nil, fmt.Errorf("invalid target scope")
	}
	rows, err := a.q.ListRoleUUIDsForBulk(ctx, textArg(target.Query["q"]))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, id := range rows {
		out = append(out, id.String())
	}
	return out, nil
}

func (a *RolesAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	if action != "delete" {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	role, err := a.q.GetRoleByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	if role.IsSystem {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "system role cannot be deleted"}, nil
	}
	perms, err := a.q.ListPermissionSlugsByRoleID(ctx, role.ID)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	desc := ""
	if role.Description.Valid {
		desc = role.Description.String
	}
	previous := map[string]any{
		"name": role.Name, "slug": role.Slug, "description": desc,
		"permission_slugs": perms,
	}
	if err := a.q.DeleteRole(ctx, id); err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "role", OK: true, Op: "delete", Previous: previous,
	}, nil
}

func (a *RolesAdapter) RevertItem(ctx context.Context, action, entityUUID string, previous map[string]any) error {
	if action != "delete" {
		return fmt.Errorf("unsupported action")
	}
	name, _ := previous["name"].(string)
	slugVal, _ := previous["slug"].(string)
	if name == "" || slugVal == "" {
		return fmt.Errorf("missing role snapshot")
	}
	desc := pgtype.Text{}
	if d, ok := previous["description"].(string); ok && d != "" {
		desc = pgtype.Text{String: d, Valid: true}
	}
	created, err := a.q.CreateRole(ctx, db.CreateRoleParams{Name: name, Slug: slugVal, Description: desc})
	if err != nil {
		return err
	}
	perms := stringSlice(previous["permission_slugs"])
	for _, p := range perms {
		perm, err := a.q.GetPermissionBySlug(ctx, p)
		if err != nil {
			continue
		}
		_ = a.q.InsertRolePermission(ctx, db.InsertRolePermissionParams{RoleID: created.ID, PermissionID: perm.ID})
	}
	_ = entityUUID
	return nil
}

func textArg(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func uniqueNonEmpty(ids []string) ([]string, error) {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no target ids")
	}
	return out, nil
}

func stringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
