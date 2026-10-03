package migrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/legacyfixture"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
)

// fixtureEmails are the hub users of the legacy fixture (TEC-253).
var fixtureEmails = []string{
	"admin@example.test", "merkez@example.test", "sahip1@example.test",
	"personel1@example.test", "sahip2@example.test",
}

// orgsUsersEnv runs the TEC-254 steps over the legacy fixture under a
// private migration_map source system and removes everything they created.
type orgsUsersEnv struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	system string
	runner *Runner
}

func newOrgsUsersEnv(t *testing.T) *orgsUsersEnv {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	legacyfixture.LoadAndHold(t, pool)

	var taken int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE email = ANY($1) AND deleted_at IS NULL`,
		fixtureEmails).Scan(&taken); err != nil {
		t.Fatal(err)
	}
	if taken != 0 {
		t.Fatalf("%d fixture e-mails already exist in the test database", taken)
	}

	e := &orgsUsersEnv{t: t, ctx: ctx, pool: pool, system: "t" + uuid.NewString()[:8]}
	q := db.New(pool)
	brand, err := q.GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		t.Fatal(err)
	}
	country, err := q.GetCountryByISO2(ctx, TRCountry)
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.MigratorCountryDistributor(ctx, db.MigratorCountryDistributorParams{BrandID: brand.ID, CountryID: country.ID})
	hadDistributor := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		exec := func(sql string, args ...any) {
			if _, err := pool.Exec(ctx, sql, args...); err != nil {
				t.Logf("cleanup %q: %v", sql, err)
			}
		}
		exec(`DELETE FROM users WHERE uuid IN (SELECT target_uuid FROM migration_map WHERE source_system = $1 AND target_table = 'users')
			AND email = ANY($2)`, e.system, fixtureEmails)
		exec(`DELETE FROM organizations WHERE uuid IN (SELECT target_uuid FROM migration_map WHERE source_system = $1 AND target_table = 'organizations')`, e.system)
		exec(`DELETE FROM migration_map WHERE source_system = $1`, e.system)
		if !hadDistributor {
			exec(`DELETE FROM territories WHERE organization_id IN (SELECT id FROM organizations WHERE slug = $1)`, TRDistributorSlug)
			exec(`DELETE FROM organizations WHERE slug = $1 AND type = 'distributor'
				AND NOT EXISTS (SELECT 1 FROM organizations c WHERE c.parent_id = organizations.id)`, TRDistributorSlug)
		}
	})

	e.runner = &Runner{
		Pool: pool,
		Profiles: map[string]Profile{
			"olexfx": {Name: "olexfx", Enabled: true, Sources: []string{SourceHub, SourceWH}, Steps: func() []Step {
				return []Step{OrganizationsStep{System: e.system}, UsersStep{System: e.system}}
			}},
			"olexfx-unmapped": {Name: "olexfx-unmapped", Enabled: true, Sources: []string{SourceHub, SourceWH}, Steps: func() []Step {
				roles := DefaultRoleMap()
				delete(roles, "center_staff")
				return []Step{UsersStep{System: e.system, RoleMap: roles}}
			}},
		},
		Open: func(_ context.Context, name string) (source.LegacySource, error) {
			return source.NewPostgres(name, pool, source.FixtureSchemas[name])
		},
	}
	return e
}

func (e *orgsUsersEnv) run(profile string, opts Options) RunReport {
	e.t.Helper()
	opts.Profile = profile
	rep, err := e.runner.Run(e.ctx, opts)
	if rep.RunID != 0 {
		cleanupRun(e.t, e.pool, rep.RunID)
	}
	if err != nil {
		e.t.Fatalf("run %s %+v: %v (report %+v)", profile, opts, err, rep)
	}
	return rep
}

func stepCounts(t *testing.T, rep RunReport, step string) map[string]int64 {
	t.Helper()
	for _, s := range rep.Steps {
		if s.Name == step {
			return s.Counts
		}
	}
	t.Fatalf("step %s missing from %+v", step, rep)
	return nil
}

// tableCounts are the rows the steps own, for the rerun comparison.
type tableCounts struct{ Dealers, Users, Members, MemberRoles, UserRoles, Maps int64 }

func (e *orgsUsersEnv) counts() tableCounts {
	e.t.Helper()
	var c tableCounts
	err := e.pool.QueryRow(e.ctx, `
		WITH mu AS (SELECT u.id FROM users u JOIN migration_map m ON m.target_uuid = u.uuid
		            WHERE m.source_system = $1 AND m.target_table = 'users')
		SELECT
		  (SELECT COUNT(*) FROM organizations o JOIN migration_map m ON m.target_uuid = o.uuid
		     WHERE m.source_system = $1 AND m.source_table = 'dealers'),
		  (SELECT COUNT(*) FROM mu),
		  (SELECT COUNT(*) FROM organization_members WHERE user_id IN (SELECT id FROM mu)),
		  (SELECT COUNT(*) FROM organization_member_roles omr JOIN organization_members om ON om.id = omr.member_id
		     WHERE om.user_id IN (SELECT id FROM mu)),
		  (SELECT COUNT(*) FROM user_roles WHERE user_id IN (SELECT id FROM mu)),
		  (SELECT COUNT(*) FROM migration_map WHERE source_system = $1)`, e.system).
		Scan(&c.Dealers, &c.Users, &c.Members, &c.MemberRoles, &c.UserRoles, &c.Maps)
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

// TEC-254 acceptance over the legacy fixture: the three dealers land under
// the Türkiye distributor, users / memberships / roles match the fixture, a
// rerun creates nothing and an unmapped role shows up in the report.
func TestOlexOrganizationsAndUsers(t *testing.T) {
	e := newOrgsUsersEnv(t)
	ctx, pool := e.ctx, e.pool

	first := e.run("olexfx", Options{})
	orgs := stepCounts(t, first, "organizations")
	if orgs[cntRead] != 3 || orgs[cntCreated] != 3 {
		t.Errorf("organizations counts = %v", orgs)
	}
	if orgs["geo_unmatched"] != 0 {
		t.Errorf("fixture addresses must all match the geo tables: %v", orgs)
	}
	users := stepCounts(t, first, "users")
	if users[cntRead] != 5 || users[cntCreated] != 5 {
		t.Errorf("users counts = %v", users)
	}
	// The fixture hashes are placeholders, not valid bcrypt.
	if users["password_reset_required"] != 5 {
		t.Errorf("password_reset_required = %d, want 5", users["password_reset_required"])
	}
	if users["unmapped_role"] != 0 {
		t.Errorf("default role map left roles unmapped: %v", users)
	}

	// The tree: center -> TR distributor (with the TR territory) -> dealers.
	var distributorID, centerID int64
	var distributorParent int64
	if err := pool.QueryRow(ctx, `
		SELECT d.id, d.parent_id, c.id
		FROM territories t
		JOIN organizations d ON d.id = t.organization_id AND d.type = 'distributor'
		JOIN organizations c ON c.type = 'center' AND c.brand_id = d.brand_id
		JOIN brands b ON b.id = d.brand_id AND b.slug = 'olex'
		JOIN countries co ON co.id = t.country_id AND co.iso2 = 'TR'
		WHERE t.province_id IS NULL`).Scan(&distributorID, &distributorParent, &centerID); err != nil {
		t.Fatalf("TR distributor territory: %v", err)
	}
	if distributorParent != centerID {
		t.Errorf("distributor parent = %d, want center %d", distributorParent, centerID)
	}

	type dealer struct {
		Parent                  int64
		Type, Status, Phone     string
		Province, District, Brd bool
	}
	got := map[string]dealer{}
	rows, err := pool.Query(ctx, `
		SELECT m.source_id, o.parent_id, o.type, o.status, o.phone,
		       o.province_id IS NOT NULL, o.district_id IS NOT NULL, b.slug = 'olex'
		FROM migration_map m
		JOIN organizations o ON o.uuid = m.target_uuid
		JOIN brands b ON b.id = o.brand_id
		WHERE m.source_system = $1 AND m.source_table = 'dealers'`, e.system)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		var d dealer
		if err := rows.Scan(&id, &d.Parent, &d.Type, &d.Status, &d.Phone, &d.Province, &d.District, &d.Brd); err != nil {
			t.Fatal(err)
		}
		got[id] = d
	}
	rows.Close()
	if len(got) != 3 {
		t.Fatalf("dealers = %+v", got)
	}
	for id, d := range got {
		if d.Parent != distributorID || d.Type != "dealer" || !d.Brd {
			t.Errorf("dealer %s = %+v, want an olex dealer under distributor %d", id, d, distributorID)
		}
		if !d.Province || !d.District {
			t.Errorf("dealer %s address not mapped: %+v", id, d)
		}
	}
	if got["1"].Phone != "+905550000101" {
		t.Errorf("dealer 1 phone = %q, want E.164", got["1"].Phone)
	}
	if got["1"].Status != "active" || got["3"].Status != "suspended" {
		t.Errorf("dealer statuses = %s / %s", got["1"].Status, got["3"].Status)
	}

	// Users, memberships and roles per fixture user.
	type member struct{ Org, Role, Roles string }
	user := func(email string) (status, phone, hash string, members []member, globals string) {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `SELECT id, status, COALESCE(phone_e164, ''), password_hash FROM users WHERE email = $1`, email).
			Scan(&id, &status, &phone, &hash); err != nil {
			t.Fatalf("user %s: %v", email, err)
		}
		rows, err := pool.Query(ctx, `
			SELECT o.type, om.role, COALESCE(string_agg(r.slug, ',' ORDER BY r.slug), '')
			FROM organization_members om
			JOIN organizations o ON o.id = om.organization_id
			LEFT JOIN organization_member_roles omr ON omr.member_id = om.id
			LEFT JOIN roles r ON r.id = omr.role_id
			WHERE om.user_id = $1
			GROUP BY o.type, om.role ORDER BY o.type`, id)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var mm member
			if err := rows.Scan(&mm.Org, &mm.Role, &mm.Roles); err != nil {
				t.Fatal(err)
			}
			members = append(members, mm)
		}
		rows.Close()
		if err := pool.QueryRow(ctx, `SELECT COALESCE(string_agg(r.slug, ',' ORDER BY r.slug), '')
			FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1`, id).Scan(&globals); err != nil {
			t.Fatal(err)
		}
		return status, phone, hash, members, globals
	}

	status, phone, hash, members, globals := user("admin@example.test")
	if status != "active" || phone != "+905550000001" || globals != "super_admin" || len(members) != 0 {
		t.Errorf("admin = %s %s %s %+v", status, phone, globals, members)
	}
	if hash != password.ResetRequired {
		t.Errorf("placeholder hash must become the reset marker, got %q", hash)
	}
	if _, _, _, members, _ = user("merkez@example.test"); len(members) != 1 || members[0] != (member{"center", "staff", "center_staff"}) {
		t.Errorf("center staff memberships = %+v", members)
	}
	if _, _, _, members, _ = user("sahip1@example.test"); len(members) != 1 || members[0] != (member{"dealer", "owner", "dealer_owner"}) {
		t.Errorf("dealer owner memberships = %+v", members)
	}
	if _, _, _, members, _ = user("personel1@example.test"); len(members) != 1 || members[0] != (member{"dealer", "staff", "dealer_staff"}) {
		t.Errorf("dealer staff memberships = %+v", members)
	}
	if status, _, _, members, _ = user("sahip2@example.test"); status != "disabled" || len(members) != 1 || members[0].Role != "owner" {
		t.Errorf("inactive dealer owner = %s %+v", status, members)
	}

	before := e.counts()
	if before != (tableCounts{Dealers: 3, Users: 5, Members: 4, MemberRoles: 4, UserRoles: 1, Maps: 8}) {
		t.Errorf("rows after first run = %+v", before)
	}

	// Second run: everything is mapped already, nothing new is written.
	second := e.run("olexfx", Options{})
	for _, step := range []string{"organizations", "users"} {
		c := stepCounts(t, second, step)
		for _, k := range []string{cntCreated, cntUpdated, "members_created", "member_roles_created", "user_roles_created",
			"center_created", "distributor_created", "territory_created"} {
			if c[k] != 0 {
				t.Errorf("rerun %s.%s = %d, want 0 (%v)", step, k, c[k], c)
			}
		}
	}
	if after := e.counts(); after != before {
		t.Errorf("rerun changed rows: %+v -> %+v", before, after)
	}

	// A legacy role without a mapping is not granted and is listed in the
	// report, also in the stored run row.
	unmapped := e.run("olexfx-unmapped", Options{DryRun: true})
	uc := stepCounts(t, unmapped, "users")
	if uc["unmapped_role"] != 1 || uc["unmapped_role:center_staff"] != 1 {
		t.Errorf("unmapped role counts = %v", uc)
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT counts FROM migration_runs WHERE id = $1`, unmapped.Steps[0].RunID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var stored map[string]int64
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if stored["unmapped_role:center_staff"] != 1 {
		t.Errorf("stored report = %s", raw)
	}
	if after := e.counts(); after != before {
		t.Errorf("dry run changed rows: %+v -> %+v", before, after)
	}
}
