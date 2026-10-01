// Package rbacsync reconciles a database with the Go permission catalog and
// system role packages (internal/platform/rbac). It is idempotent: a second
// run reports no changes. Custom (non-system) roles are left untouched.
package rbacsync

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TxBeginner starts a transaction (*pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Options control a sync run.
type Options struct {
	// DryRun computes the report and rolls back.
	DryRun bool
	// Prune deletes permissions that are not in the catalog (their grants
	// cascade). Without it they are only reported.
	Prune bool
}

// Report lists what a run changed (or would change).
type Report struct {
	PermissionsCreated []string `json:"permissions_created"`
	PermissionsUpdated []string `json:"permissions_updated"`
	PermissionsUnknown []string `json:"permissions_unknown"`
	PermissionsPruned  []string `json:"permissions_pruned"`
	RolesCreated       []string `json:"roles_created"`
	RolesUpdated       []string `json:"roles_updated"`
	GrantsAdded        []string `json:"grants_added"`
	GrantsUpdated      []string `json:"grants_updated"`
	GrantsRemoved      []string `json:"grants_removed"`
}

// Changes counts the changes of a run (unknown permissions are not changes
// unless pruned).
func (r Report) Changes() int {
	return len(r.PermissionsCreated) + len(r.PermissionsUpdated) + len(r.PermissionsPruned) +
		len(r.RolesCreated) + len(r.RolesUpdated) +
		len(r.GrantsAdded) + len(r.GrantsUpdated) + len(r.GrantsRemoved)
}

// Sync reconciles the database with the catalog in one transaction.
func Sync(ctx context.Context, pool TxBeginner, opts Options) (Report, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rep, err := syncTx(ctx, db.New(tx), opts)
	if err != nil {
		return Report{}, err
	}
	if opts.DryRun {
		return rep, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return Report{}, err
	}
	return rep, nil
}

func syncTx(ctx context.Context, q *db.Queries, opts Options) (Report, error) {
	var rep Report
	if err := syncPermissions(ctx, q, opts, &rep); err != nil {
		return Report{}, err
	}
	if err := syncRoles(ctx, q, &rep); err != nil {
		return Report{}, err
	}
	return rep, nil
}

func syncPermissions(ctx context.Context, q *db.Queries, opts Options, rep *Report) error {
	existing, err := q.ListAllPermissions(ctx)
	if err != nil {
		return err
	}
	bySlug := make(map[string]db.Permission, len(existing))
	for _, p := range existing {
		bySlug[p.Slug] = p
	}
	for _, def := range rbac.Permissions {
		scopes := make([]string, len(def.Scopes))
		for i, s := range def.Scopes {
			scopes[i] = string(s)
		}
		desc := pgtype.Text{String: def.Description, Valid: def.Description != ""}
		cur, ok := bySlug[def.Slug]
		if ok && cur.Name == def.Name && cur.Module == def.Module && slices.Equal(cur.Scopes, scopes) &&
			cur.IsSensitive == def.Sensitive && cur.SuperAdminOnly == def.SuperAdminOnly &&
			cur.Description == desc && cur.SortOrder == def.SortOrder() {
			continue
		}
		if err := q.UpsertPermission(ctx, db.UpsertPermissionParams{
			Name: def.Name, Slug: def.Slug, Module: def.Module, Scopes: scopes,
			IsSensitive: def.Sensitive, SuperAdminOnly: def.SuperAdminOnly,
			Description: desc, SortOrder: def.SortOrder(),
		}); err != nil {
			return fmt.Errorf("upsert permission %s: %w", def.Slug, err)
		}
		if ok {
			rep.PermissionsUpdated = append(rep.PermissionsUpdated, def.Slug)
		} else {
			rep.PermissionsCreated = append(rep.PermissionsCreated, def.Slug)
		}
	}
	for _, p := range existing {
		if _, ok := rbac.PermissionBySlug(p.Slug); ok {
			continue
		}
		if !opts.Prune {
			rep.PermissionsUnknown = append(rep.PermissionsUnknown, p.Slug)
			continue
		}
		if err := q.DeletePermissionBySlug(ctx, p.Slug); err != nil {
			return fmt.Errorf("prune permission %s: %w", p.Slug, err)
		}
		rep.PermissionsPruned = append(rep.PermissionsPruned, p.Slug)
	}
	return nil
}

func syncRoles(ctx context.Context, q *db.Queries, rep *Report) error {
	existing, err := q.ListAllRoles(ctx)
	if err != nil {
		return err
	}
	bySlug := make(map[string]db.Role, len(existing))
	for _, r := range existing {
		bySlug[r.Slug] = r
	}
	for _, def := range rbac.Roles {
		desc := pgtype.Text{String: def.Description, Valid: def.Description != ""}
		orgType := pgtype.Text{String: def.OrgType, Valid: def.OrgType != ""}
		cur, ok := bySlug[def.Slug]
		role := cur
		if !ok || cur.Name != def.Name || cur.Description != desc || cur.OrgType != orgType || !cur.IsSystem {
			role, err = q.UpsertSystemRole(ctx, db.UpsertSystemRoleParams{
				Name: def.Name, Slug: def.Slug, Description: desc, OrgType: orgType,
			})
			if err != nil {
				return fmt.Errorf("upsert role %s: %w", def.Slug, err)
			}
			if ok {
				rep.RolesUpdated = append(rep.RolesUpdated, def.Slug)
			} else {
				rep.RolesCreated = append(rep.RolesCreated, def.Slug)
			}
		}
		if err := syncGrants(ctx, q, role, rbac.RoleGrants(def), rep); err != nil {
			return err
		}
	}
	return nil
}

func syncGrants(ctx context.Context, q *db.Queries, role db.Role, want map[string]rbac.Scope, rep *Report) error {
	rows, err := q.ListRoleGrantsByRoleID(ctx, role.ID)
	if err != nil {
		return err
	}
	have := make(map[string]string, len(rows))
	for _, row := range rows {
		have[row.Slug] = row.Scope
	}
	for _, slug := range rbac.SortedGrantSlugs(want) {
		scope := string(want[slug])
		cur, ok := have[slug]
		if ok && cur == scope {
			continue
		}
		perm, err := q.GetPermissionBySlug(ctx, slug)
		if err != nil {
			return fmt.Errorf("role %s grant %s: %w", role.Slug, slug, err)
		}
		if err := q.InsertRolePermission(ctx, db.InsertRolePermissionParams{
			RoleID: role.ID, PermissionID: perm.ID, Scope: scope,
		}); err != nil {
			return fmt.Errorf("role %s grant %s: %w", role.Slug, slug, err)
		}
		label := role.Slug + ":" + slug + ":" + scope
		if ok {
			rep.GrantsUpdated = append(rep.GrantsUpdated, label)
		} else {
			rep.GrantsAdded = append(rep.GrantsAdded, label)
		}
	}
	extra := make([]string, 0)
	for slug := range have {
		if _, ok := want[slug]; !ok {
			extra = append(extra, slug)
		}
	}
	sort.Strings(extra)
	for _, slug := range extra {
		if err := q.DeleteRolePermission(ctx, db.DeleteRolePermissionParams{RoleID: role.ID, PermissionSlug: slug}); err != nil {
			return fmt.Errorf("role %s revoke %s: %w", role.Slug, slug, err)
		}
		rep.GrantsRemoved = append(rep.GrantsRemoved, role.Slug+":"+slug)
	}
	return nil
}
