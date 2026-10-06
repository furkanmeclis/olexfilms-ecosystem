package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	SpecUsers = "users"
	SpecRoles = "roles"
)

// UsersAdapter indexes platform users.
type UsersAdapter struct {
	q *db.Queries
}

func NewUsers(q *db.Queries) *UsersAdapter {
	return &UsersAdapter{q: q}
}

func (a *UsersAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SpecUsers,
		LabelKey:   "search.specs_users",
		Permission: rbac.PermPlatformUsersRead,
		Icon:       "users",
		Searchable: []string{"title", "subtitle", "keywords", "email", "name", "surname"},
	}
}

func (a *UsersAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListUsersForExport(ctx, db.ListUsersForExportParams{})
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0, len(rows))
	for _, u := range rows {
		doc, err := a.documentFromUser(ctx, u)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

func (a *UsersAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("users search: invalid uuid")
	}
	user, err := a.q.GetUserByUUID(ctx, parsed)
	if err != nil {
		if err == pgx.ErrNoRows {
			return searchengine.Document{}, fmt.Errorf("users search: not found")
		}
		return searchengine.Document{}, err
	}
	return a.documentFromUser(ctx, user)
}

func (a *UsersAdapter) documentFromUser(ctx context.Context, u db.User) (searchengine.Document, error) {
	roleSlugs, err := a.q.ListUserRoleSlugs(ctx, u.ID)
	if err != nil {
		return searchengine.Document{}, err
	}
	title := strings.TrimSpace(u.Name + " " + u.Surname)
	keywords := []string{u.Email.String, u.PhoneE164.String, u.Status}
	if len(roleSlugs) > 0 {
		keywords = append(keywords, strings.Join(roleSlugs, " "))
	}
	return searchengine.Document{
		ID:       u.Uuid.String(),
		Spec:     SpecUsers,
		Title:    title,
		Subtitle: firstNonEmpty(u.Email.String, u.PhoneE164.String),
		Keywords: keywords,
		Href:     "/platform/users/" + u.Uuid.String(),
		Icon:     "users",
	}, nil
}

// RolesAdapter indexes platform roles.
type RolesAdapter struct {
	q *db.Queries
}

func NewRoles(q *db.Queries) *RolesAdapter {
	return &RolesAdapter{q: q}
}

func (a *RolesAdapter) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         SpecRoles,
		LabelKey:   "search.specs_roles",
		Permission: rbac.PermPlatformRolesRead,
		Icon:       "shield",
		Searchable: []string{"title", "subtitle", "keywords", "name", "slug"},
	}
}

func (a *RolesAdapter) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	rows, err := a.q.ListRolesForExport(ctx, db.ListRolesForExportParams{})
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0, len(rows))
	for _, role := range rows {
		out = append(out, a.documentFromRole(role))
	}
	return out, nil
}

func (a *RolesAdapter) Document(ctx context.Context, id string) (searchengine.Document, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("roles search: invalid uuid")
	}
	role, err := a.q.GetRoleByUUID(ctx, parsed)
	if err != nil {
		if err == pgx.ErrNoRows {
			return searchengine.Document{}, fmt.Errorf("roles search: not found")
		}
		return searchengine.Document{}, err
	}
	return a.documentFromRole(role), nil
}

func (a *RolesAdapter) documentFromRole(role db.Role) searchengine.Document {
	subtitle := role.Slug
	if role.Description.Valid && strings.TrimSpace(role.Description.String) != "" {
		subtitle = role.Description.String
	}
	keywords := []string{role.Slug}
	if role.IsSystem {
		keywords = append(keywords, "system")
	}
	return searchengine.Document{
		ID:       role.Uuid.String(),
		Spec:     SpecRoles,
		Title:    role.Name,
		Subtitle: subtitle,
		Keywords: keywords,
		Href:     "/platform/roles/" + role.Uuid.String(),
		Icon:     "shield",
	}
}

var (
	_ searchengine.Adapter = (*UsersAdapter)(nil)
	_ searchengine.Adapter = (*RolesAdapter)(nil)
)

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
