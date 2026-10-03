package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/normalize"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
)

// Where a mapped legacy role is granted.
const (
	// RoleAtPlatform is a global role (user_roles).
	RoleAtPlatform = "platform"
	// RoleAtCenter is a membership role in the Olex center.
	RoleAtCenter = "center"
	// RoleAtDealer is a membership role in the user's own dealer
	// (users.dealer_id).
	RoleAtDealer = "dealer"
)

// RoleTarget is the new role a legacy (Spatie) role becomes.
type RoleTarget struct {
	// Slug is the role slug in this application (rbac catalog).
	Slug string
	// At is RoleAtPlatform, RoleAtCenter or RoleAtDealer.
	At string
	// Owner makes the membership an owner membership.
	Owner bool
}

// DefaultRoleMap maps the legacy hub role names (roles.name, guard web) to
// the new roles. A legacy role missing here is not granted and is listed in
// the run report as "unmapped_role:<name>".
func DefaultRoleMap() map[string]RoleTarget {
	return map[string]RoleTarget{
		"super_admin":  {Slug: "super_admin", At: RoleAtPlatform},
		"center_staff": {Slug: "center_staff", At: RoleAtCenter},
		"dealer_owner": {Slug: "dealer_owner", At: RoleAtDealer, Owner: true},
		"dealer_staff": {Slug: "dealer_staff", At: RoleAtDealer},
	}
}

// defaultDealerRole is granted to a user who belongs to a dealer
// (users.dealer_id) but holds no mapped dealer role: the least privileged
// dealer role.
const defaultDealerRole = "dealer_staff"

// legacyRolesQuery lists the role names of every user (Spatie pivot).
const legacyRolesQuery = `SELECT mhr.model_id, r.name FROM model_has_roles mhr
JOIN roles r ON r.id = mhr.role_id
WHERE mhr.model_type = ?
ORDER BY mhr.model_id, r.name`

// legacyUserModel is model_has_roles.model_type of hub users.
const legacyUserModel = `App\Models\User`

// UsersStep imports hub users with their roles: users, organization
// memberships and role grants (TEC-254). It runs after OrganizationsStep.
type UsersStep struct {
	// System is the migration_map source system; empty means SourceHub.
	System string
	// RoleMap overrides DefaultRoleMap.
	RoleMap map[string]RoleTarget
}

// Name implements Step.
func (UsersStep) Name() string { return "users" }

func (s UsersStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

func (s UsersStep) roleMap() map[string]RoleTarget {
	if s.RoleMap == nil {
		return DefaultRoleMap()
	}
	return s.RoleMap
}

const usersQuery = `SELECT id, name, COALESCE(email, ''), COALESCE(phone, ''), is_active, COALESCE(locale, ''),
	email_verified_at, password, dealer_id, created_at, updated_at
FROM users`

type legacyUser struct {
	ID                         int64
	Name, Email, Phone, Locale string
	Active                     bool
	EmailVerifiedAt            sql.NullTime
	Password                   string
	DealerID                   sql.NullInt64
	CreatedAt, UpdatedAt       sql.NullTime
}

// supportedLocales mirrors chk_users_locale.
var supportedLocales = map[string]string{
	"tr": "tr", "en": "en", "bg": "bg", "de": "de", "el": "el", "uk": "uk", "ru": "ru",
	"fr": "fr", "es": "es", "it": "it", "zh_cn": "zh-CN", "zh-cn": "zh-CN", "az": "az", "ar": "ar",
}

// Run implements Step.
func (s UsersStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	roleMap := s.roleMap()
	if err := checkRoleMap(ctx, dst.Q, roleMap); err != nil {
		return StepResult{}, err
	}
	tree, err := resolveOlexTree(ctx, dst.Q, false, c)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	roles, err := loadLegacyRoles(ctx, hub)
	if err != nil {
		return StepResult{Counts: c}, err
	}

	query, args := usersQuery, []any{}
	if dst.Mode == ModeDelta && !dst.Since.IsZero() {
		query += " WHERE COALESCE(updated_at, created_at) > ?"
		args = append(args, dst.Since)
	}
	rows, err := hub.Query(ctx, query+" ORDER BY id", args...)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	var users []legacyUser
	for rows.Next() {
		var u legacyUser
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.Active, &u.Locale, &u.EmailVerifiedAt,
			&u.Password, &u.DealerID, &u.CreatedAt, &u.UpdatedAt); err != nil {
			_ = rows.Close()
			return StepResult{Counts: c}, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Close(); err != nil {
		return StepResult{Counts: c}, fmt.Errorf("read users: %w", err)
	}

	var watermark time.Time
	dealers := map[int64]int64{}
	for _, u := range users {
		c.inc(cntRead)
		if ts := latest(u.CreatedAt, u.UpdatedAt); ts.After(watermark) {
			watermark = ts
		}
		userID, ok, err := s.importUser(ctx, dst.Q, m, u, c)
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("user %d: %w", u.ID, err)
		}
		if !ok {
			continue
		}
		if err := s.grantRoles(ctx, dst.Q, m, tree, roleMap, dealers, u, userID, roles[u.ID], c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("user %d roles: %w", u.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

// checkRoleMap fails fast when a target role is missing or belongs to
// another organization type.
func checkRoleMap(ctx context.Context, q *db.Queries, roleMap map[string]RoleTarget) error {
	want := map[string]string{defaultDealerRole: RoleAtDealer}
	for name, t := range roleMap {
		switch t.At {
		case RoleAtPlatform, RoleAtCenter, RoleAtDealer:
		default:
			return fmt.Errorf("role map %q: unknown target %q", name, t.At)
		}
		want[t.Slug] = t.At
	}
	slugs := make([]string, 0, len(want))
	for s := range want {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	rows, err := q.MigratorRoleOrgTypes(ctx, slugs)
	if err != nil {
		return fmt.Errorf("read roles: %w", err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.Slug] = r.OrgType.String
	}
	for _, slugValue := range slugs {
		orgType, ok := got[slugValue]
		if !ok {
			return fmt.Errorf("role %q does not exist", slugValue)
		}
		if orgType != want[slugValue] {
			return fmt.Errorf("role %q is a %s role, mapped as %s", slugValue, orgType, want[slugValue])
		}
	}
	return nil
}

// loadLegacyRoles returns the role names of every hub user.
func loadLegacyRoles(ctx context.Context, hub source.LegacySource) (map[int64][]string, error) {
	rows, err := hub.Query(ctx, legacyRolesQuery, legacyUserModel)
	if err != nil {
		return nil, err
	}
	out := map[int64][]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan role: %w", err)
		}
		out[id] = append(out[id], name)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read roles: %w", err)
	}
	return out, nil
}

// importUser creates or updates the user of a legacy row and returns its id.
// ok is false when the row was skipped (no usable e-mail or phone).
func (s UsersStep) importUser(ctx context.Context, q *db.Queries, m *Mapper, u legacyUser, c counts) (int64, bool, error) {
	email := truncate(normalize.NormalizeEmail(u.Email), 255)
	ph := normalize.NormalizePhone(u.Phone, TRCountry)
	var phone string
	if ph.Verified {
		phone = ph.E164
	} else if ph.Raw != "" {
		c.inc("phone_unverified")
	}
	name, surname := splitName(u.Name)
	status := "active"
	if !u.Active {
		status = "disabled"
	}
	locale := pgtype.Text{}
	if l, ok := supportedLocales[strings.ToLower(strings.TrimSpace(u.Locale))]; ok {
		locale = pgtype.Text{String: l, Valid: true}
	}
	hash := migratedPasswordHash(u.Password)

	key := Key{System: s.system(), Table: "users", ID: strconv.FormatInt(u.ID, 10), TargetTable: "users"}
	sum := Checksum(u.Name, u.Email, u.Phone, u.Active, u.Locale, u.EmailVerifiedAt.Time, u.EmailVerifiedAt.Valid, u.Password)

	_, mapped, err := m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return 0, false, err
	}
	if !mapped {
		// An account that already exists here (same e-mail; the phone only
		// when the legacy row has no e-mail) is linked, not duplicated, and
		// left as it is.
		find := db.MigratorFindUserByContactParams{Email: pgText(email)}
		if email == "" {
			find = db.MigratorFindUserByContactParams{PhoneE164: pgText(phone)}
		}
		existing, err := q.MigratorFindUserByContact(ctx, find)
		switch {
		case err == nil:
			linked, err := m.Link(ctx, key, existing.Uuid, sum)
			if err != nil {
				return 0, false, err
			}
			if linked {
				c.inc("linked_existing")
				return existing.ID, true, nil
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return 0, false, fmt.Errorf("find user: %w", err)
		}
	}

	res, err := m.Upsert(ctx, key, sum)
	if err != nil {
		return 0, false, err
	}
	current, err := q.MigratorUserByUUID(ctx, res.UUID)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, fmt.Errorf("read user: %w", err)
	}
	if exists && !res.Changed {
		c.inc(cntUnchanged)
		return current.ID, true, nil
	}

	// Another account may hold the e-mail or the phone (two legacy users on
	// one phone, an account created in the new app): drop the clashing value.
	selfID := int64(0)
	if exists {
		selfID = current.ID
	}
	taken, err := q.MigratorContactTaken(ctx, db.MigratorContactTakenParams{
		UserID: selfID, Email: pgText(email), PhoneE164: pgText(phone),
	})
	if err != nil {
		return 0, false, fmt.Errorf("contact check: %w", err)
	}
	if taken.EmailTaken {
		c.inc("email_conflict:user:" + key.ID)
		email = ""
		if exists {
			email = current.Email.String
		}
	}
	if taken.PhoneTaken {
		c.inc("phone_conflict:user:" + key.ID)
		phone = ""
		if exists {
			phone = current.PhoneE164.String
		}
	}
	if email == "" && phone == "" {
		// chk_users_email_or_phone: nothing to log in with.
		c.inc("skipped_no_contact:user:" + key.ID)
		return 0, false, nil
	}

	if !exists {
		id, err := q.MigratorInsertUser(ctx, db.MigratorInsertUserParams{
			Uuid: res.UUID, Email: pgText(email), PasswordHash: hash, Name: name, Surname: surname,
			Status: status, EmailVerifiedAt: pgTime(u.EmailVerifiedAt), Locale: locale,
			PhoneE164: pgText(phone), CreatedAt: pgTime(u.CreatedAt),
		})
		if err != nil {
			return 0, false, fmt.Errorf("insert user: %w", err)
		}
		c.inc(cntCreated)
		if hash == password.ResetRequired {
			c.inc("password_reset_required")
		} else {
			c.inc("password_kept")
		}
		return id, true, nil
	}
	if err := q.MigratorUpdateUser(ctx, db.MigratorUpdateUserParams{
		ID: current.ID, Email: pgText(email), Name: name, Surname: surname, Status: status, Locale: locale,
		PhoneE164: pgText(phone), EmailVerifiedAt: pgTime(u.EmailVerifiedAt), PasswordHash: hash,
	}); err != nil {
		return 0, false, fmt.Errorf("update user: %w", err)
	}
	c.inc(cntUpdated)
	return current.ID, true, nil
}

// grantRoles turns the legacy roles of a user into role grants and
// memberships. Every write is idempotent; only new rows are counted.
func (s UsersStep) grantRoles(ctx context.Context, q *db.Queries, m *Mapper, tree olexTree, roleMap map[string]RoleTarget,
	dealers map[int64]int64, u legacyUser, userID int64, legacyRoles []string, c counts,
) error {
	dealerOrg := int64(0)
	if u.DealerID.Valid {
		id, ok, err := s.dealerOrg(ctx, q, m, dealers, u.DealerID.Int64)
		if err != nil {
			return err
		}
		if ok {
			dealerOrg = id
		} else {
			c.inc("dealer_unmapped:user:" + strconv.FormatInt(u.ID, 10))
		}
	}

	dealerRole := false
	for _, roleName := range legacyRoles {
		t, ok := roleMap[roleName]
		if !ok {
			c.inc("unmapped_role")
			c.inc("unmapped_role:" + roleName)
			continue
		}
		switch t.At {
		case RoleAtPlatform:
			n, err := q.MigratorAssignUserRole(ctx, db.MigratorAssignUserRoleParams{UserID: userID, Slug: t.Slug})
			if err != nil {
				return fmt.Errorf("grant %s: %w", t.Slug, err)
			}
			c.add("user_roles_created", n)
		case RoleAtCenter:
			if err := grantMember(ctx, q, tree.CenterID, userID, t, c); err != nil {
				return err
			}
		case RoleAtDealer:
			if dealerOrg == 0 {
				c.inc("role_without_dealer:" + roleName)
				continue
			}
			dealerRole = true
			if err := grantMember(ctx, q, dealerOrg, userID, t, c); err != nil {
				return err
			}
		}
	}
	if dealerOrg != 0 && !dealerRole {
		c.inc("dealer_default_role")
		return grantMember(ctx, q, dealerOrg, userID, RoleTarget{Slug: defaultDealerRole, At: RoleAtDealer}, c)
	}
	return nil
}

func grantMember(ctx context.Context, q *db.Queries, orgID, userID int64, t RoleTarget, c counts) error {
	role := "staff"
	if t.Owner {
		role = "owner"
	}
	member, err := q.MigratorEnsureMember(ctx, db.MigratorEnsureMemberParams{OrganizationID: orgID, UserID: userID, Role: role})
	if err != nil {
		return fmt.Errorf("membership: %w", err)
	}
	if member.Inserted {
		c.inc("members_created")
	}
	n, err := q.MigratorAssignMemberRole(ctx, db.MigratorAssignMemberRoleParams{MemberID: member.ID, Slug: t.Slug})
	if err != nil {
		return fmt.Errorf("grant %s: %w", t.Slug, err)
	}
	c.add("member_roles_created", n)
	return nil
}

// dealerOrg resolves a hub dealer id to its organization through
// migration_map (OrganizationsStep wrote it).
func (s UsersStep) dealerOrg(ctx context.Context, q *db.Queries, m *Mapper, cache map[int64]int64, dealerID int64) (int64, bool, error) {
	if id, ok := cache[dealerID]; ok {
		return id, id != 0, nil
	}
	target, ok, err := m.Lookup(ctx, s.system(), "dealers", strconv.FormatInt(dealerID, 10))
	if err != nil || !ok {
		cache[dealerID] = 0
		return 0, false, err
	}
	id, err := q.MigratorOrganizationIDByUUID(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		cache[dealerID] = 0
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("dealer organization: %w", err)
	}
	cache[dealerID] = id
	return id, true, nil
}

// migratedPasswordHash keeps a Laravel bcrypt hash ($2y$), which
// password.Verify accepts and the first login upgrades to Argon2id. Anything
// else becomes password.ResetRequired: the user signs in with OTP or resets
// the password.
func migratedPasswordHash(legacy string) string {
	legacy = strings.TrimSpace(legacy)
	if password.IsBcrypt(legacy) {
		return legacy
	}
	return password.ResetRequired
}

// splitName splits the single legacy name into name and surname: the last
// word is the surname. A one-word name has an empty surname.
func splitName(full string) (string, string) {
	parts := strings.Fields(full)
	switch len(parts) {
	case 0:
		return "-", ""
	case 1:
		return truncate(parts[0], 100), ""
	}
	return truncate(strings.Join(parts[:len(parts)-1], " "), 100), truncate(parts[len(parts)-1], 100)
}

func pgText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
