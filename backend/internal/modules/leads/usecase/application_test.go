package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeModules struct {
	system bool
	off    map[int64]bool
}

func (m *fakeModules) Enabled(_ context.Context, orgID int64, key string) (bool, error) {
	if key != features.ModuleLeads {
		return false, fmt.Errorf("unexpected key %s", key)
	}
	return m.system && !m.off[orgID], nil
}

func (m *fakeModules) SystemEnabled(_ context.Context, key string) (bool, error) {
	if key != features.ModuleLeads {
		return false, fmt.Errorf("unexpected key %s", key)
	}
	return m.system, nil
}

type fakeSettings map[string]bool

func (s fakeSettings) Bool(_ context.Context, key string) bool { return s[key] }

type fakeOutbox struct{ events []events.Event }

func (o *fakeOutbox) Enqueue(_ context.Context, tx pgx.Tx, ev events.Event) error {
	if tx == nil {
		return errors.New("no tx")
	}
	o.events = append(o.events, ev)
	return nil
}

type appFixture struct {
	*fixture
	apps    *Applications
	modules *fakeModules
	out     *fakeOutbox
	de      db.Country
	berlin  db.Province
	dist    db.Organization
	staff   db.User
}

func newAppFixture(t *testing.T) *appFixture {
	f := newFixture(t)
	a := &appFixture{fixture: f, modules: &fakeModules{system: true, off: map[int64]bool{}}, out: &fakeOutbox{}}
	var err error
	if a.de, err = f.q.GetCountryByISO2(f.ctx, "DE"); err != nil {
		t.Fatalf("DE: %v", err)
	}
	if !a.de.IsActive {
		if _, err := f.tx.Exec(f.ctx, `UPDATE countries SET is_active = TRUE WHERE id = $1`, a.de.ID); err != nil {
			t.Fatal(err)
		}
		a.de.IsActive = true
	}
	// A fresh province keeps the test independent of territories other
	// tests committed: a province territory beats any country-level one.
	n := time.Now().UnixNano()
	if a.berlin, err = f.q.CreateProvince(f.ctx, db.CreateProvinceParams{
		CountryID: a.de.ID, Code: fmt.Sprintf("B%d", n%1_000_000_000), Name: fmt.Sprintf("Berlin %d", n),
	}); err != nil {
		t.Fatalf("province: %v", err)
	}
	slug := fmt.Sprintf("tec317-de-dist-%d", n)
	if a.dist, err = f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: slug, Name: "DE Distributor " + slug, Status: "active", Type: "distributor",
		ParentID: pgtype.Int8{Int64: f.center.ID, Valid: true}, BrandID: f.brand,
		Currency: "EUR", Locale: "de", Timezone: "UTC", Settings: []byte(`{}`),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}); err != nil {
		t.Fatalf("distributor: %v", err)
	}
	if _, err := f.q.CreateTerritory(f.ctx, db.CreateTerritoryParams{
		BrandID: f.brand, OrganizationID: a.dist.ID, CountryID: a.de.ID,
		ProvinceID: pgtype.Int8{Int64: a.berlin.ID, Valid: true},
	}); err != nil {
		t.Fatalf("territory: %v", err)
	}
	// A distributor member holding leads.read is the notification target.
	a.staff = f.userRow("de-staff")
	var memberID int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO organization_members (organization_id, user_id) VALUES ($1, $2) RETURNING id`,
		a.dist.ID, a.staff.ID).Scan(&memberID); err != nil {
		t.Fatalf("member: %v", err)
	}
	if _, err := f.tx.Exec(f.ctx, `INSERT INTO organization_member_roles (member_id, role_id)
		SELECT $1, rp.role_id FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
		JOIN roles r ON r.id = rp.role_id
		WHERE p.slug = $2 AND r.slug = 'distributor_owner' LIMIT 1`, memberID, rbac.PermLeadsRead); err != nil {
		t.Fatalf("member role: %v", err)
	}
	a.apps = NewApplications(f.tx, f.q, geo.New(nil, f.q), a.modules,
		fakeSettings{sysconfig.KeyLeadsDealerApplicationEnabled: true}, a.out)
	return a
}

func (a *appFixture) input() ApplicationInput {
	return ApplicationInput{
		CompanyName: "Kuzey Folien GmbH", ContactName: "Max Mustermann", Phone: "030 12345678",
		Email: "max@kuzey.example", CountryID: a.de.ID, ProvinceID: &a.berlin.ID,
		Message: "Wir möchten Händler werden.", KVKKConsent: true, Language: "de",
	}
}

func (a *appFixture) submit(in ApplicationInput) ApplicationResult {
	a.t.Helper()
	app, err := a.apps.Validate(a.ctx, in)
	if err != nil {
		a.t.Fatalf("validate: %v", err)
	}
	res, err := a.apps.Submit(a.ctx, a.brand, app)
	if err != nil {
		a.t.Fatalf("submit: %v", err)
	}
	return res
}

// TEC-317 acceptance: a Berlin (DE) application lands in the German
// distributor's lead list as a dealer_candidate / application_form lead
// with the E.164 phone, and its members are notified.
func TestApplicationBerlinRoutesToGermanDistributor(t *testing.T) {
	a := newAppFixture(t)
	res := a.submit(a.input())
	if res.RoutedBy != RoutedByTerritory || res.OrganizationID != a.dist.ID || res.Lead == nil {
		t.Fatalf("result = %+v, want territory routing to %d", res, a.dist.ID)
	}
	l := res.Lead
	if l.TargetType != TargetDealerCandidate || l.Source != SourceApplicationForm || l.Status != StatusNew ||
		l.CandidatePhoneE164.String != "+493012345678" || l.ProvinceID.Int64 != a.berlin.ID || l.CreatedByUserID.Valid {
		t.Fatalf("lead = %+v", l)
	}
	rows, total, err := a.svc.List(a.ctx, a.caller(a.dist, rbac.ScopeManaged), ListFilter{TargetType: TargetDealerCandidate, Limit: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].UUID != l.Uuid {
		t.Fatalf("distributor list = %d %+v, want the application", total, rows)
	}
	evs, err := a.svc.Events(a.ctx, a.caller(a.dist, rbac.ScopeManaged), l.Uuid)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var consent bool
	for _, e := range evs {
		if e.EventType == EventMessage && e.Payload["kvkk_consent"] == true && e.Payload["language"] == "de" {
			consent = true
		}
	}
	if !consent || len(evs) != 2 {
		t.Fatalf("timeline = %+v, want created + consent message", evs)
	}
	if len(a.out.events) != 1 || a.out.events[0].Name != events.LeadsApplicationReceived || a.out.events[0].TenantID == nil || *a.out.events[0].TenantID != a.dist.ID {
		t.Fatalf("outbox = %+v", a.out.events)
	}
	if ids, _ := a.out.events[0].Payload["notify_user_ids"].([]int64); len(ids) != 1 || ids[0] != a.staff.ID {
		t.Fatalf("notify = %v, want [%d]", a.out.events[0].Payload["notify_user_ids"], a.staff.ID)
	}
}

// TEC-317 acceptance: a country without territory goes to the brand
// center; so does a territory whose distributor has the leads module off.
func TestApplicationWithoutTerritoryFallsToCenter(t *testing.T) {
	a := newAppFixture(t)
	var free db.Country
	if err := a.tx.QueryRow(a.ctx, `SELECT id FROM countries c WHERE c.is_active AND c.iso2 <> 'DE'
		AND NOT EXISTS (SELECT 1 FROM territories t WHERE t.brand_id = $1 AND t.country_id = c.id)
		ORDER BY c.iso2 = 'FR' DESC, c.id LIMIT 1`, a.brand).Scan(&free.ID); err != nil {
		t.Fatalf("free country: %v", err)
	}
	in := a.input()
	in.CountryID, in.ProvinceID, in.Phone = free.ID, nil, "+33 1 23 45 67 89"
	res := a.submit(in)
	if res.RoutedBy != RoutedByCenter || res.OrganizationID != a.center.ID {
		t.Fatalf("result = %+v, want center %d", res, a.center.ID)
	}

	a.modules.off[a.dist.ID] = true
	in = a.input()
	in.Phone = "+49 30 87654321"
	res = a.submit(in)
	if res.RoutedBy != RoutedByCenter || res.OrganizationID != a.center.ID {
		t.Fatalf("module off: result = %+v, want center", res)
	}
}

// TEC-317 acceptance: closed form; switching on only while leads is open
// system wide (422 LEADS_MODULE_DISABLED).
func TestApplicationEnabledAndGuard(t *testing.T) {
	a := newAppFixture(t)
	if on, err := a.apps.Enabled(a.ctx); err != nil || !on {
		t.Fatalf("enabled = %v %v", on, err)
	}
	a.apps.settings = fakeSettings{}
	if on, _ := a.apps.Enabled(a.ctx); on {
		t.Fatal("default must be closed")
	}
	a.apps.settings = fakeSettings{sysconfig.KeyLeadsDealerApplicationEnabled: true}
	a.modules.system = false
	if on, _ := a.apps.Enabled(a.ctx); on {
		t.Fatal("leads closed system wide must close the form")
	}
	var re *sysconfig.RuleError
	if err := a.apps.SettingGuard(a.ctx, json.RawMessage(`true`)); !errors.As(err, &re) || re.Code != CodeLeadsModuleDisabled {
		t.Fatalf("guard = %v", err)
	}
	if err := a.apps.SettingGuard(a.ctx, json.RawMessage(`false`)); err != nil {
		t.Fatalf("switching off: %v", err)
	}
	a.modules.system = true
	if err := a.apps.SettingGuard(a.ctx, json.RawMessage(`true`)); err != nil {
		t.Fatalf("guard with leads open: %v", err)
	}
}

// TEC-317 acceptance: invalid phone and missing KVKK consent are
// validation errors; a filled honeypot stores nothing.
func TestApplicationValidation(t *testing.T) {
	a := newAppFixture(t)
	cases := []struct {
		field string
		mut   func(*ApplicationInput)
	}{
		{"phone", func(in *ApplicationInput) { in.Phone = "12" }},
		{"phone", func(in *ApplicationInput) { in.Phone = "" }},
		{"kvkk_consent", func(in *ApplicationInput) { in.KVKKConsent = false }},
		{"company_name", func(in *ApplicationInput) { in.CompanyName = "  " }},
		{"email", func(in *ApplicationInput) { in.Email = "not-an-email" }},
		{"language", func(in *ApplicationInput) { in.Language = "xx" }},
		{"country_id", func(in *ApplicationInput) { in.CountryID = 0 }},
		{"province_id", func(in *ApplicationInput) { bad := int64(-1); in.ProvinceID = &bad }},
	}
	for _, tc := range cases {
		in := a.input()
		tc.mut(&in)
		_, err := a.apps.Validate(a.ctx, in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != tc.field {
			t.Fatalf("%s: err = %v", tc.field, err)
		}
	}
	in := a.input()
	in.Honeypot = "http://spam.example"
	app, err := a.apps.Validate(a.ctx, in)
	if err != nil || !app.Spam {
		t.Fatalf("honeypot = %+v %v", app, err)
	}
	if res, err := a.apps.Submit(a.ctx, a.brand, app); err != nil || res.Lead != nil || len(a.out.events) != 0 {
		t.Fatalf("honeypot stored: %+v %v", res, err)
	}
}
