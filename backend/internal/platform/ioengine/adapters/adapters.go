package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const ResourceUsers = "platform.users"

// UsersAdapter exports/imports platform users.
type UsersAdapter struct {
	q *db.Queries
}

// NewUsers creates a users resource adapter.
func NewUsers(q *db.Queries) *UsersAdapter {
	return &UsersAdapter{q: q}
}

func (a *UsersAdapter) Resource() string { return ResourceUsers }

func (a *UsersAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "uuid", LabelKey: "users.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "email", LabelKey: "users.email", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "users.name", Type: ioengine.ColumnTypeString},
		{Key: "surname", LabelKey: "users.surname", Type: ioengine.ColumnTypeString},
		{Key: "status", LabelKey: "users.status", Type: ioengine.ColumnTypeEnum},
		{Key: "locale", LabelKey: "users.locale", Type: ioengine.ColumnTypeString},
		{Key: "role_slugs", LabelKey: "users.role_slugs", Type: ioengine.ColumnTypeString},
		{Key: "created_at", LabelKey: "users.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *UsersAdapter) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	createdFrom, createdBefore, err := rangeArgs(query, "created")
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.q.ListUsersForExport(ctx, db.ListUsersForExportParams{
		Q: textArg(query["q"]), Statuses: apiquery.SplitCSV(query["status"]), RoleSlugs: apiquery.SplitCSV(query["role"]),
		CreatedFrom: createdFrom, CreatedBefore: createdBefore,
	})
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, u := range rows {
		roleSlugs, err := a.q.ListUserRoleSlugs(ctx, u.ID)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		out = append(out, map[string]any{
			"uuid": u.Uuid.String(), "email": u.Email, "name": u.Name, "surname": u.Surname,
			"status": u.Status, "locale": u.Locale.String,
			"role_slugs": strings.Join(roleSlugs, ","),
			"created_at": u.CreatedAt.Time,
		})
	}
	return ioengine.Dataset{Resource: ResourceUsers, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *UsersAdapter) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "email", LabelKey: "users.email", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "name", LabelKey: "users.name", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "surname", LabelKey: "users.surname", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "status", LabelKey: "users.status", Type: ioengine.ColumnTypeEnum, DefaultHint: "pending"},
		{Key: "locale", LabelKey: "users.locale", Type: ioengine.ColumnTypeString, DefaultHint: "tr"},
		{Key: "role_slugs", LabelKey: "users.role_slugs", Type: ioengine.ColumnTypeString},
	}
}

func (a *UsersAdapter) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	email := strings.ToLower(strings.TrimSpace(strVal(row, "email")))
	name := strings.TrimSpace(strVal(row, "name"))
	surname := strings.TrimSpace(strVal(row, "surname"))
	if email == "" || name == "" || surname == "" {
		return ioengine.RowResult{OK: false, Error: "email, name, surname required"}, nil
	}
	status := strings.TrimSpace(strVal(row, "status"))
	if status == "" {
		status = strVal(defaults, "status")
	}
	if status == "" {
		status = "pending"
	}
	locale := strings.TrimSpace(strVal(row, "locale"))
	if locale == "" {
		locale = strVal(defaults, "locale")
	}
	// Empty keeps users.locale NULL (inherit from the organization).
	if locale != "" {
		l, ok := i18n.Parse(locale)
		if !ok {
			return ioengine.RowResult{OK: false, Error: "unsupported locale: " + locale}, nil
		}
		locale = string(l)
	}
	if _, err := a.q.GetUserByEmail(ctx, pgtype.Text{String: email, Valid: true}); err == nil {
		return ioengine.RowResult{OK: false, Error: "email already exists"}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ioengine.RowResult{}, err
	}
	pw, err := randomPassword()
	if err != nil {
		return ioengine.RowResult{}, err
	}
	hash, err := password.Hash(pw)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	created, err := a.q.CreateUser(ctx, db.CreateUserParams{
		Email: pgtype.Text{String: email, Valid: true}, PasswordHash: hash, Name: name, Surname: surname, Status: status,
	})
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	if locale != "" {
		_ = a.q.UpdateUserLocale(ctx, db.UpdateUserLocaleParams{ID: created.ID, Locale: pgtype.Text{String: locale, Valid: true}})
	}
	roleSlugs := parseSlugs(strVal(row, "role_slugs"))
	if len(roleSlugs) == 0 {
		roleSlugs = parseSlugs(strVal(defaults, "role_slugs"))
	}
	for _, rs := range roleSlugs {
		// Imports run as platform.users.import holders who need not be super
		// admins; never let a spreadsheet grant super_admin.
		if rs == rbac.RoleSuperAdmin {
			continue
		}
		_ = a.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: created.ID, Slug: rs})
	}
	return ioengine.RowResult{
		OK: true, EntityType: "user", EntityUUID: created.Uuid.String(), Op: "create",
	}, nil
}

func (a *UsersAdapter) RevertRow(ctx context.Context, entityType, entityUUID string, _ map[string]any) error {
	if entityType != "user" {
		return fmt.Errorf("unsupported entity")
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	user, err := a.q.GetUserByUUID(ctx, id)
	if err != nil {
		return err
	}
	// soft-delete not available — disable user and clear roles
	_, err = a.q.UpdateUserPlatform(ctx, db.UpdateUserPlatformParams{
		Uuid: id, Status: pgtype.Text{String: "disabled", Valid: true},
	})
	if err != nil {
		return err
	}
	return a.q.ReplaceUserRoles(ctx, user.ID)
}

const ResourceRoles = "platform.roles"

// RolesAdapter exports/imports roles.
type RolesAdapter struct {
	q *db.Queries
}

func NewRoles(q *db.Queries) *RolesAdapter {
	return &RolesAdapter{q: q}
}

func (a *RolesAdapter) Resource() string { return ResourceRoles }

func (a *RolesAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "uuid", LabelKey: "roles.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "name", LabelKey: "roles.name", Type: ioengine.ColumnTypeString},
		{Key: "slug", LabelKey: "roles.slug", Type: ioengine.ColumnTypeString},
		{Key: "description", LabelKey: "roles.description", Type: ioengine.ColumnTypeString},
		{Key: "is_system", LabelKey: "roles.is_system", Type: ioengine.ColumnTypeBoolean},
		{Key: "permission_slugs", LabelKey: "roles.permission_slugs", Type: ioengine.ColumnTypeString},
		{Key: "created_at", LabelKey: "roles.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *RolesAdapter) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	isSystem, err := boolArg(query, "is_system")
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.q.ListRolesForExport(ctx, db.ListRolesForExportParams{Q: textArg(query["q"]), IsSystem: isSystem})
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		perms, err := a.q.ListPermissionSlugsByRoleID(ctx, r.ID)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		desc := ""
		if r.Description.Valid {
			desc = r.Description.String
		}
		out = append(out, map[string]any{
			"uuid": r.Uuid.String(), "name": r.Name, "slug": r.Slug, "description": desc,
			"is_system": r.IsSystem, "permission_slugs": strings.Join(perms, ","),
			"created_at": r.CreatedAt.Time,
		})
	}
	return ioengine.Dataset{Resource: ResourceRoles, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *RolesAdapter) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "name", LabelKey: "roles.name", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "slug", LabelKey: "roles.slug", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "description", LabelKey: "roles.description", Type: ioengine.ColumnTypeString},
		{Key: "permission_slugs", LabelKey: "roles.permission_slugs", Type: ioengine.ColumnTypeString},
	}
}

func (a *RolesAdapter) ApplyRow(ctx context.Context, row map[string]any, _ map[string]any) (ioengine.RowResult, error) {
	name := strings.TrimSpace(strVal(row, "name"))
	slugVal := strings.TrimSpace(strVal(row, "slug"))
	if name == "" || slugVal == "" {
		return ioengine.RowResult{OK: false, Error: "name and slug required"}, nil
	}
	slugVal = normalizeSlug(slugVal)
	if rbac.IsSystemRole(slugVal) {
		return ioengine.RowResult{OK: false, Error: "system role slug not allowed"}, nil
	}
	if _, err := a.q.GetRoleBySlug(ctx, slugVal); err == nil {
		return ioengine.RowResult{OK: false, Error: "slug already exists"}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ioengine.RowResult{}, err
	}
	desc := pgtype.Text{}
	if d := strings.TrimSpace(strVal(row, "description")); d != "" {
		desc = pgtype.Text{String: d, Valid: true}
	}
	created, err := a.q.CreateRole(ctx, db.CreateRoleParams{Name: name, Slug: slugVal, Description: desc})
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	for _, p := range parseSlugs(strVal(row, "permission_slugs")) {
		slug, want := rbac.ParseGrant(p)
		perm, err := a.q.GetPermissionBySlug(ctx, slug)
		if err != nil {
			continue
		}
		scope, ok := rbac.GrantScope(perm.Scopes, perm.SuperAdminOnly, want)
		if !ok {
			continue
		}
		_ = a.q.InsertRolePermission(ctx, db.InsertRolePermissionParams{
			RoleID: created.ID, PermissionID: perm.ID, Scope: string(scope),
		})
	}
	return ioengine.RowResult{
		OK: true, EntityType: "role", EntityUUID: created.Uuid.String(), Op: "create",
	}, nil
}

func (a *RolesAdapter) RevertRow(ctx context.Context, entityType, entityUUID string, _ map[string]any) error {
	if entityType != "role" {
		return fmt.Errorf("unsupported entity")
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	role, err := a.q.GetRoleByUUID(ctx, id)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return fmt.Errorf("cannot rollback system role")
	}
	return a.q.DeleteRole(ctx, id)
}

const ResourceNotifications = "platform.notifications"

// NotificationsAdapter export-only.
type NotificationsAdapter struct {
	q *db.Queries
}

func NewNotifications(q *db.Queries) *NotificationsAdapter {
	return &NotificationsAdapter{q: q}
}

func (a *NotificationsAdapter) Resource() string { return ResourceNotifications }

func (a *NotificationsAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "uuid", LabelKey: "notifications.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "channel", LabelKey: "notifications.channel", Type: ioengine.ColumnTypeEnum},
		{Key: "status", LabelKey: "notifications.status", Type: ioengine.ColumnTypeEnum},
		{Key: "priority", LabelKey: "notifications.priority", Type: ioengine.ColumnTypeEnum},
		{Key: "title", LabelKey: "notifications.title", Type: ioengine.ColumnTypeString},
		{Key: "created_at", LabelKey: "notifications.created_at", Type: ioengine.ColumnTypeDatetime},
		{Key: "read_at", LabelKey: "notifications.read_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *NotificationsAdapter) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	params := db.ListPlatformNotificationsForExportParams{
		Statuses: apiquery.SplitCSV(query["status"]), Channel: textArg(query["channel"]), Q: textArg(query["q"]),
	}
	if raw := strings.TrimSpace(query["user_uuid"]); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return ioengine.Dataset{}, fmt.Errorf("user_uuid: %w", err)
		}
		u, err := a.q.GetUserByUUID(ctx, id)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		params.UserID = pgtype.Int8{Int64: u.ID, Valid: true}
	}
	rows, err := a.q.ListPlatformNotificationsForExport(ctx, params)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, n := range rows {
		var read any
		if n.ReadAt.Valid {
			read = n.ReadAt.Time
		}
		out = append(out, map[string]any{
			"uuid": n.Uuid.String(), "channel": n.Channel, "status": n.Status,
			"priority": n.Priority, "title": n.Title, "created_at": n.CreatedAt.Time, "read_at": read,
		})
	}
	return ioengine.Dataset{Resource: ResourceNotifications, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *NotificationsAdapter) ImportSchema() []ioengine.ImportField { return nil }

func (a *NotificationsAdapter) ApplyRow(_ context.Context, _ map[string]any, _ map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "import not supported"}, nil
}

func (a *NotificationsAdapter) RevertRow(_ context.Context, _, _ string, _ map[string]any) error {
	return fmt.Errorf("import not supported")
}

func textArg(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func strVal(row map[string]any, key string) string {
	if row == nil {
		return ""
	}
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func parseSlugs(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '|' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func randomPassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "Imp!" + hex.EncodeToString(b[:8]) + "1aA", nil
}

func normalizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

// SampleRows returns demo import rows for a resource.
func SampleRows(resource string) []map[string]any {
	switch resource {
	case ResourceUsers:
		return []map[string]any{
			{"email": "import@example.com", "name": "Import", "surname": "User", "status": "pending", "locale": "tr", "role_slugs": "organization_user"},
		}
	case ResourceRoles:
		return []map[string]any{
			{"name": "Import Role", "slug": "import_role", "description": "Sample role", "permission_slugs": "notifications.read"},
		}
	default:
		return nil
	}
}

var _ ioengine.ResourceAdapter = (*UsersAdapter)(nil)

// Activity export adapter lives in activity module — placeholder to satisfy registry if needed.

const ResourceActivity = "platform.activity"

type ActivityAdapter struct {
	q *db.Queries
}

func NewActivity(q *db.Queries) *ActivityAdapter {
	return &ActivityAdapter{q: q}
}

func (a *ActivityAdapter) Resource() string { return ResourceActivity }

func (a *ActivityAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "action", LabelKey: "activity.action", Type: ioengine.ColumnTypeString},
		{Key: "resource", LabelKey: "activity.resource", Type: ioengine.ColumnTypeString},
		{Key: "actor_user_id", LabelKey: "activity.actor", Type: ioengine.ColumnTypeString},
		{Key: "created_at", LabelKey: "activity.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *ActivityAdapter) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	params, err := activity.ListParams(queryValues(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	params.LimitCount, params.OffsetCount = 10000, 0
	rows, err := a.q.ListActivityEvents(ctx, params)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, e := range rows {
		var actor any
		if e.ActorUserID.Valid {
			actor = e.ActorUserID.Int64
		}
		out = append(out, map[string]any{
			"action": e.Action, "resource": e.Resource, "actor_user_id": actor,
			"created_at": e.CreatedAt.Time,
		})
	}
	return ioengine.Dataset{Resource: ResourceActivity, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *ActivityAdapter) ImportSchema() []ioengine.ImportField { return nil }
func (a *ActivityAdapter) ApplyRow(_ context.Context, _ map[string]any, _ map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{OK: false, Error: "import not supported"}, nil
}

func (a *ActivityAdapter) RevertRow(_ context.Context, _, _ string, _ map[string]any) error {
	return fmt.Errorf("import not supported")
}

// unused import guard
var _ = time.Time{}
