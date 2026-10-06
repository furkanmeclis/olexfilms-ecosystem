package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

// TEC-365: platform core lists (users, roles, permissions, organizations,
// activity, exports/imports) — sort, filters, date ranges, the permission
// catalog mode and the new organizations bulk / export endpoints.

func (it *itest) exec(sql string, args ...any) {
	it.t.Helper()
	if _, err := it.pool.Exec(context.Background(), sql, args...); err != nil {
		it.t.Fatalf("%s: %v", sql, err)
	}
}

func (it *itest) insertUUID(sql string, args ...any) string {
	it.t.Helper()
	var id string
	if err := it.pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		it.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func TestIntegrationListPlatformUsersCreatedAndRoles(t *testing.T) {
	it := newIntegration(t)
	tag := "c365" + it.suffix[len(it.suffix)-9:]
	u1, _ := it.user(tag+"-1", "super_admin")
	u2, _ := it.user(tag + "-2")
	u3, _ := it.user(tag + "-3")
	it.exec("UPDATE users SET created_at = $2 WHERE id = $1", u1.ID, time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC))
	it.exec("UPDATE users SET created_at = $2 WHERE id = $1", u2.ID, time.Date(2025, 3, 2, 23, 0, 0, 0, time.UTC))
	it.exec("UPDATE users SET created_at = $2 WHERE id = $1", u3.ID, time.Date(2025, 3, 5, 9, 0, 0, 0, time.UTC))
	role := "c365-role-" + it.suffix
	it.exec("INSERT INTO roles (name, slug, description, is_system) VALUES ($1, $1, NULL, false)", role)
	t.Cleanup(func() { _, _ = it.pool.Exec(context.Background(), "DELETE FROM roles WHERE slug = $1", role) })
	if err := it.q.AssignUserRoleBySlug(context.Background(), db.AssignUserRoleBySlugParams{UserID: u3.ID, Slug: role}); err != nil {
		t.Fatal(err)
	}
	a, b, c := u1.Uuid.String(), u2.Uuid.String(), u3.Uuid.String()
	lc := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/users", extra: url.Values{"q": {tag}, "sort": {"created_at"}}}

	lc.expect(nil, a, b, c)
	// Date-only created_to covers the whole day.
	lc.expect(url.Values{"created_from": {"2025-03-01"}, "created_to": {"2025-03-02"}}, a, b)
	lc.expect(url.Values{"created_from": {"2025-03-03"}}, c)
	lc.expect(url.Values{"created_to": {"2025-03-01T10:00:00Z"}}, a)
	// role: CSV multi-value.
	lc.expect(url.Values{"role": {"super_admin," + role}}, a, c)
	lc.expect(url.Values{"role": {role}}, c)
	lc.expect400(url.Values{"created_from": {"03/01/2025"}}, "created_from")
	lc.expect400(url.Values{"created_from": {"2025-03-05"}, "created_to": {"2025-03-01"}}, "created_from")

	// The list item carries created_at / updated_at.
	_, env := it.do("GET", "/v1/platform/users?q="+tag+"&sort=created_at", hostOlex, lc.admin, nil)
	var page struct {
		Items []struct {
			CreatedAt *time.Time `json:"created_at"`
			UpdatedAt *time.Time `json:"updated_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil || len(page.Items) != 3 {
		t.Fatalf("users page: %v %s", err, env.Data)
	}
	if page.Items[0].CreatedAt == nil || !page.Items[0].CreatedAt.Equal(time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)) || page.Items[0].UpdatedAt == nil {
		t.Fatalf("user created_at/updated_at = %v / %v", page.Items[0].CreatedAt, page.Items[0].UpdatedAt)
	}
}

func TestIntegrationListPlatformRolesSortFilter(t *testing.T) {
	it := newIntegration(t)
	tag := "r365" + it.suffix[len(it.suffix)-9:]
	at := time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC)
	var ids []string
	for i, name := range []string{"b", "a", "c"} {
		ids = append(ids, it.insertUUID(`INSERT INTO roles (name, slug, description, is_system, created_at)
			VALUES ($1, $2, NULL, false, $3) RETURNING uuid::text`,
			tag+"-"+name, tag+"-slug-"+string(rune('z'-i)), at.Add(time.Duration(i)*time.Hour)))
	}
	t.Cleanup(func() { _, _ = it.pool.Exec(context.Background(), "DELETE FROM roles WHERE name LIKE $1", tag+"%") })
	r1, r2, r3 := ids[0], ids[1], ids[2] // names b, a, c; slugs z, y, x
	lc := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/roles", extra: url.Values{"q": {tag}}}

	lc.expect(nil, r2, r1, r3) // default name
	lc.expect(sortParam("-name"), r3, r1, r2)
	lc.expect(sortParam("slug"), r3, r2, r1)
	lc.expect(sortParam("-created_at"), r3, r2, r1)
	lc.expect(url.Values{"is_system": {"false"}, "sort": {"name"}}, r2, r1, r3)
	lc.expect(url.Values{"is_system": {"true"}})
	lc.expect400(sortParam("description"), "sort")
	lc.expect400(url.Values{"is_system": {"yes"}}, "is_system")

	// Without q, is_system=true lists only system roles.
	code, env := it.do("GET", "/v1/platform/roles?is_system=true&limit=100", hostOlex, lc.admin, nil)
	var page struct {
		Items []struct {
			IsSystem  bool       `json:"is_system"`
			CreatedAt *time.Time `json:"created_at"`
		} `json:"items"`
	}
	if code != http.StatusOK || json.Unmarshal(env.Data, &page) != nil || len(page.Items) == 0 {
		t.Fatalf("system roles: %d %s", code, env.Data)
	}
	for _, r := range page.Items {
		if !r.IsSystem || r.CreatedAt == nil {
			t.Fatalf("is_system=true item = %+v", r)
		}
	}
	def, fields := it.metaSort(lc.admin, "/v1/platform/roles/meta")
	if def != "name" || !containsStr(fields, "slug") || !containsStr(fields, "created_at") {
		t.Fatalf("roles meta default_sort=%q sortable=%v", def, fields)
	}
}

func TestIntegrationPlatformPermissionsCatalog(t *testing.T) {
	it := newIntegration(t)
	admin := it.adminToken()
	type permPage struct {
		Items []struct {
			Slug string `json:"slug"`
		} `json:"items"`
		Total int64 `json:"total"`
		Limit int32 `json:"limit"`
	}
	get := func(qs string) permPage {
		t.Helper()
		code, env := it.do("GET", "/v1/platform/permissions?"+qs, hostOlex, admin, nil)
		if code != http.StatusOK {
			t.Fatalf("permissions?%s = %d %s", qs, code, errCode(env))
		}
		var p permPage
		if err := json.Unmarshal(env.Data, &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	all := get("all=true")
	if all.Total <= 100 || int64(len(all.Items)) != all.Total {
		t.Fatalf("all=true returned %d of %d (limit %d)", len(all.Items), all.Total, all.Limit)
	}
	big := get("limit=500")
	if int64(len(big.Items)) != all.Total || big.Limit != 500 {
		t.Fatalf("limit=500 returned %d of %d (limit %d)", len(big.Items), big.Total, big.Limit)
	}
	if def := get(""); len(def.Items) != 20 {
		t.Fatalf("default page = %d items", len(def.Items))
	}
	if code, _ := it.do("GET", "/v1/platform/permissions?all=maybe", hostOlex, admin, nil); code != http.StatusBadRequest {
		t.Fatalf("all=maybe = %d", code)
	}
}

func TestIntegrationListPlatformOrganizationsFilters(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	tag := "o365" + it.suffix[len(it.suffix)-9:]
	dist := it.org(tag+"-d", "distributor", center)
	d1 := it.org(tag+"-1", "dealer", dist)
	d2 := it.org(tag+"-2", "dealer", center)
	it.exec("UPDATE organizations SET plan_code = 'pro', access_ends_at = '2026-02-10T08:00:00Z' WHERE id = $1", dist.ID)
	it.exec("UPDATE organizations SET plan_code = 'trial', access_ends_at = '2026-02-11T22:00:00Z' WHERE id = $1", d1.ID)
	it.exec("UPDATE organizations SET plan_code = 'basic', access_ends_at = NULL WHERE id = $1", d2.ID)
	o0, o1, o2 := dist.Uuid.String(), d1.Uuid.String(), d2.Uuid.String()
	lc := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/organizations", extra: url.Values{"q": {tag}, "sort": {"name"}}}

	lc.expect(nil, o1, o2, o0) // names "<tag>-1", "-2", "-d"
	lc.expect(url.Values{"plan_code": {"pro,trial"}}, o1, o0)
	lc.expect(url.Values{"access_ends_from": {"2026-02-10"}, "access_ends_to": {"2026-02-10"}}, o0)
	lc.expect(url.Values{"access_ends_from": {"2026-02-10"}}, o1, o0)
	lc.expect(url.Values{"type": {"dealer"}}, o1, o2)
	lc.expect(url.Values{"type": {"dealer,distributor"}, "parent_uuid": {o0}}, o1)
	lc.expect(url.Values{"parent_uuid": {"00000000-0000-0000-0000-000000000000"}})
	lc.expect400(url.Values{"type": {"shop"}}, "type")
	lc.expect400(url.Values{"access_ends_from": {"soon"}}, "access_ends_from")
	lc.expect400(url.Values{"parent_uuid": {"nope"}}, "parent_uuid")

	// /children stays (full array of direct children).
	code, env := it.do("GET", "/v1/platform/organizations/"+o0+"/children", hostOlex, lc.admin, nil)
	if code != http.StatusOK || !json.Valid(env.Data) {
		t.Fatalf("children = %d", code)
	}
}

func (it *itest) orgState(id int64) (string, *time.Time) {
	it.t.Helper()
	var status string
	var ends *time.Time
	if err := it.pool.QueryRow(context.Background(), "SELECT status, access_ends_at FROM organizations WHERE id = $1", id).Scan(&status, &ends); err != nil {
		it.t.Fatal(err)
	}
	return status, ends
}

func TestIntegrationPlatformOrganizationsBulk(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	tag := "b365" + it.suffix[len(it.suffix)-9:]
	d1 := it.org(tag+"-1", "dealer", center)
	d2 := it.org(tag+"-2", "dealer", center)
	foreign := it.org(tag+"-g", "dealer", it.brandCenter("glorian"))
	ends := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	it.exec("UPDATE organizations SET access_ends_at = $2 WHERE id = $1", d1.ID, ends)
	it.exec("UPDATE organizations SET access_ends_at = NULL WHERE id = $1", d2.ID)
	admin := it.adminToken()
	path := "/v1/platform/organizations/bulk"

	// Status change by ids; the other brand's organization is not found.
	run := it.bulkRun(path, admin, map[string]any{
		"action": "suspend",
		"target": map[string]any{"scope": "ids", "ids": []string{d1.Uuid.String(), d2.Uuid.String(), foreign.Uuid.String()}},
	})
	if run.Summary.Succeeded != 2 || run.Summary.Failed != 1 {
		t.Fatalf("suspend summary = %+v", run.Summary)
	}
	for _, o := range []db.Organization{d1, d2} {
		if s, _ := it.orgState(o.ID); s != "suspended" {
			t.Fatalf("%s status = %s", o.Name, s)
		}
	}
	if s, _ := it.orgState(foreign.ID); s != "active" {
		t.Fatalf("foreign brand organization changed: %s", s)
	}
	undone, _ := it.bulkUndo("/v1/platform/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, admin, http.StatusOK)
	if undone.UndoStatus != bulkusecase.UndoUndone || undone.UndoResult.Restored != 2 {
		t.Fatalf("undo = %+v", undone)
	}
	if s, _ := it.orgState(d1.ID); s != "active" {
		t.Fatalf("status after undo = %s", s)
	}

	// Extend access over "all matching" (q + filter): from the current end
	// when it is in the future, from now when there is none.
	before := time.Now().UTC()
	ext := it.bulkRun(path, admin, map[string]any{
		"action": "extend_access",
		"target": map[string]any{"scope": "query", "query": map[string]string{"q": tag, "status": "active"},
			"params": map[string]string{"days": "30"}},
	})
	if ext.Summary.Succeeded != 2 || ext.Summary.Total != 2 {
		t.Fatalf("extend summary = %+v", ext.Summary)
	}
	if _, e := it.orgState(d1.ID); e == nil || !e.Equal(ends.AddDate(0, 0, 30)) {
		t.Fatalf("d1 access end = %v, want %v", e, ends.AddDate(0, 0, 30))
	}
	if _, e := it.orgState(d2.ID); e == nil || e.Before(before.AddDate(0, 0, 30).Add(-time.Minute)) || e.After(time.Now().UTC().AddDate(0, 0, 30)) {
		t.Fatalf("d2 access end = %v", e)
	}
	if _, e := it.orgState(foreign.ID); e != nil {
		t.Fatal("query scope reached the other brand")
	}
	it.bulkUndo("/v1/platform/bulk-operations/"+ext.Operation.UUID.String()+"/undo", hostOlex, admin, http.StatusOK)
	if _, e := it.orgState(d2.ID); e != nil {
		t.Fatalf("d2 access end after undo = %v", e)
	}
	if _, e := it.orgState(d1.ID); e == nil || !e.Equal(ends) {
		t.Fatalf("d1 access end after undo = %v", e)
	}

	// Bad days → 400; the brand center is refused per item.
	for _, days := range []string{"0", "x", "4000"} {
		if code, env := it.do("POST", path, hostOlex, admin, map[string]any{
			"action": "extend_access",
			"target": map[string]any{"scope": "ids", "ids": []string{d1.Uuid.String()}, "params": map[string]string{"days": days}},
		}); code != http.StatusBadRequest {
			t.Fatalf("days=%s: %d %s", days, code, errCode(env))
		}
	}
	cen := it.bulkRun(path, admin, map[string]any{
		"action": "suspend", "target": map[string]any{"scope": "ids", "ids": []string{center.Uuid.String()}},
	})
	if cen.Summary.Failed != 1 {
		t.Fatalf("center suspend summary = %+v", cen.Summary)
	}

	// Permission: organizations.read reaches the route, the action needs write.
	reader := it.platformUserWith(tag+"-reader", "platform.organizations.read")
	if code, _ := it.do("POST", path, hostOlex, reader, map[string]any{
		"action": "suspend", "target": map[string]any{"scope": "ids", "ids": []string{d1.Uuid.String()}},
	}); code != http.StatusForbidden {
		t.Fatalf("reader bulk = %d", code)
	}
	nobody := it.platformUserWith(tag+"-nobody", "platform.users.read")
	if code, _ := it.do("POST", path, hostOlex, nobody, map[string]any{
		"action": "suspend", "target": map[string]any{"scope": "ids", "ids": []string{d1.Uuid.String()}},
	}); code != http.StatusForbidden {
		t.Fatalf("no-permission bulk = %d", code)
	}

	// Meta advertises the bulk actions and export.
	code, env := it.do("GET", "/v1/platform/organizations/meta", hostOlex, admin, nil)
	var meta struct {
		Capabilities struct{ Bulk, Export bool } `json:"capabilities"`
		BulkActions  []struct {
			ID string `json:"id"`
		} `json:"bulk_actions"`
	}
	if code != http.StatusOK || json.Unmarshal(env.Data, &meta) != nil || !meta.Capabilities.Bulk || !meta.Capabilities.Export || len(meta.BulkActions) != 4 {
		t.Fatalf("organizations meta = %s", env.Data)
	}
}

// platformUserWith logs in a platform user holding a custom role with the
// given permissions.
func (it *itest) platformUserWith(name string, perms ...string) string {
	it.t.Helper()
	slug := "t365-" + name
	it.exec("INSERT INTO roles (name, slug, description, is_system) VALUES ($1, $1, NULL, false)", slug)
	it.t.Cleanup(func() { _, _ = it.pool.Exec(context.Background(), "DELETE FROM roles WHERE slug = $1", slug) })
	for _, p := range perms {
		it.exec(`INSERT INTO role_permissions (role_id, permission_id)
			SELECT r.id, p.id FROM roles r, permissions p WHERE r.slug = $1 AND p.slug = $2`, slug, p)
	}
	u, pw := it.user(name, slug)
	return it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": u.Email.String, "password": pw,
	})).AccessToken
}

func TestIntegrationPlatformOrganizationsExport(t *testing.T) {
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store; d.Queue = qc })
	center := it.brandCenter("olex")
	tag := "e365" + it.suffix[len(it.suffix)-9:]
	it.org(tag+"-1", "dealer", center)
	d2 := it.org(tag+"-2", "dealer", center)
	it.org(tag+"-g", "dealer", it.brandCenter("glorian"))
	it.exec("UPDATE organizations SET status = 'suspended' WHERE id = $1", d2.ID)
	admin, pw := it.user(tag+"-admin", "super_admin")
	tok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": pw,
	})).AccessToken
	path := "/v1/platform/organizations/export"
	body := map[string]any{"format": "csv", "query": map[string]string{"q": tag, "sort": "-name"}}

	// Step-up is required like the other platform exports.
	if code, env := it.do("POST", path, hostOlex, tok, body); code != http.StatusForbidden && code != http.StatusUnauthorized {
		t.Fatalf("export without step-up = %d %s", code, errCode(env))
	}
	it.stepUp(admin.Uuid)
	if code, env := it.do("POST", path, hostOlex, tok, map[string]any{"format": "csv", "query": map[string]string{"sort": "phone"}}); code != http.StatusBadRequest {
		t.Fatalf("bad sort = %d %s", code, errCode(env))
	}
	request := func(body map[string]any) string {
		t.Helper()
		code, env := it.do("POST", path, hostOlex, tok, body)
		if code != http.StatusAccepted {
			t.Fatalf("export = %d %s", code, errCode(env))
		}
		var job struct {
			UUID     string `json:"uuid"`
			Resource string `json:"resource"`
		}
		if err := json.Unmarshal(env.Data, &job); err != nil || job.Resource != orgusecase.ResourcePlatformList {
			t.Fatalf("job = %s", env.Data)
		}
		return job.UUID
	}
	all := request(body)
	suspended := request(map[string]any{"format": "json", "query": map[string]string{"q": tag, "status": "suspended"}})

	// The worker runs the jobs without a request brand: the stamped brand
	// keeps them inside olex (2 rows, not the glorian one).
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueExports)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("pending export tasks = %d (%v)", len(tasks), err)
	}
	reg := ioengine.NewRegistry(orgusecase.NewListExportAdapter(orgusecase.New(it.pool, it.q)))
	worker := exportusecase.New(it.q, store, reg, nil, nil, nil, nil)
	for _, task := range tasks {
		p, err := queue.ParseExportProcessPayload(task.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.ProcessExport(context.Background(), p.ExportJobID); err != nil {
			t.Fatalf("process export: %v", err)
		}
	}
	for id, want := range map[string]int{all: 2, suspended: 1} {
		var rows int
		var status string
		if err := it.pool.QueryRow(context.Background(), "SELECT status, row_count FROM export_jobs WHERE uuid = $1", id).Scan(&status, &rows); err != nil {
			t.Fatal(err)
		}
		if status != "completed" || rows != want {
			t.Fatalf("export job %s status=%s rows=%d, want %d", id, status, rows, want)
		}
	}
	// Permission: without organizations.read the route is closed.
	nobody := it.platformUserWith(tag+"-nobody", "platform.users.read")
	if code, _ := it.do("POST", path, hostOlex, nobody, body); code != http.StatusForbidden {
		t.Fatalf("no-permission export = %d", code)
	}
}

func TestIntegrationListPlatformActivity(t *testing.T) {
	it := newIntegration(t)
	tag := "a365" + it.suffix[len(it.suffix)-9:]
	actor, _ := it.user(tag + "-actor")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM activity_events WHERE resource LIKE $1", tag+"%")
	})
	ins := func(actorID any, action, resource string, at time.Time) string {
		return it.insertUUID(`INSERT INTO activity_events (actor_user_id, action, resource, created_at)
			VALUES ($1, $2, $3, $4) RETURNING uuid::text`, actorID, action, resource, at)
	}
	day := time.Date(2025, 6, 10, 12, 0, 0, 0, time.UTC)
	e1 := ins(actor.ID, "b.update", tag+".x", day)
	e2 := ins(nil, "a.create", tag+".y", day.AddDate(0, 0, 1))
	e3 := ins(actor.ID, "c.delete", tag+".x", day.AddDate(0, 0, 2))
	lc := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/activity", extra: url.Values{"q": {tag}}}

	lc.expect(nil, e3, e2, e1) // -created_at
	lc.expect(sortParam("created_at"), e1, e2, e3)
	lc.expect(sortParam("action"), e2, e1, e3)
	lc.expect(sortParam("-action"), e3, e1, e2)
	lc.expect(sortParam("resource"), e1, e3, e2)
	lc.expect(url.Values{"actor": {actor.Uuid.String()}, "sort": {"created_at"}}, e1, e3)
	lc.expect(url.Values{"resource": {tag + ".y," + tag + ".x"}, "action": {"a.create,c.delete"}, "sort": {"created_at"}}, e2, e3)
	lc.expect(url.Values{"created_from": {"2025-06-11"}, "created_to": {"2025-06-11"}}, e2)
	lc.expect400(sortParam("payload"), "sort")
	lc.expect400(url.Values{"actor": {"me"}}, "actor")
	lc.expect400(url.Values{"created_to": {"tomorrow"}}, "created_to")

	def, fields := it.metaSort(lc.admin, "/v1/platform/activity/meta")
	if def != "-created_at" || len(fields) != 3 {
		t.Fatalf("activity meta default_sort=%q sortable=%v", def, fields)
	}
}

func TestIntegrationListExportImportJobs(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	tag := "j365" + it.suffix[len(it.suffix)-9:]
	owner, _ := it.user(tag + "-owner")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM export_jobs WHERE resource LIKE $1", tag+"%")
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM import_jobs WHERE resource LIKE $1", tag+"%")
	})
	center := it.brandCenter("olex")
	dealer := it.org(tag+"-dealer", "dealer", center)
	staff, spw := it.user(tag + "-staff")
	it.member(dealer, staff, "owner")
	day := time.Date(2025, 7, 1, 9, 0, 0, 0, time.UTC)
	exp := func(org any, resource, format, status string, at time.Time) string {
		return it.insertUUID(`INSERT INTO export_jobs (resource, actor_id, organization_id, format, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING uuid::text`, resource, owner.ID, org, format, status, at)
	}
	x1 := exp(nil, tag+".b", "csv", "completed", day)
	x2 := exp(nil, tag+".a", "pdf", "failed", day.AddDate(0, 0, 1))
	x3 := exp(nil, tag+".c", "xlsx", "queued", day.AddDate(0, 0, 2))
	xt := exp(dealer.ID, tag+".t", "csv", "completed", day)
	admin := it.adminToken()

	ex := listSortCase{it: it, admin: admin, base: "/v1/platform/exports", extra: url.Values{"q": {tag}}}
	// The platform list (admin) shows every job, tenant ones included.
	ex.expect(nil, x3, x2, xt, x1) // same created_at: id DESC tiebreak
	ex.expect(sortParam("resource"), x2, x1, x3, xt)
	ex.expect(sortParam("status"), x1, xt, x2, x3)
	ex.expect(url.Values{"status": {"failed,queued"}, "sort": {"created_at"}}, x2, x3)
	ex.expect(url.Values{"format": {"csv"}, "sort": {"created_at"}}, x1, xt)
	ex.expect(url.Values{"resource": {tag + ".a," + tag + ".c"}, "sort": {"created_at"}}, x2, x3)
	ex.expect(url.Values{"created_from": {"2025-07-02"}, "created_to": {"2025-07-02"}}, x2)
	ex.expect400(sortParam("file_key"), "sort")
	ex.expect400(url.Values{"status": {"done"}}, "status")
	ex.expect400(url.Values{"format": {"tsv"}}, "format")

	staffTok := it.loginOrg(staff, spw, dealer)
	tex := listSortCase{it: it, admin: staffTok, base: "/v1/tenant/exports", extra: url.Values{"q": {tag}}}
	tex.expect(nil, xt)
	tex.expect(url.Values{"status": {"failed"}})
	tex.expect400(sortParam("bogus"), "sort")

	imp := func(org any, resource, file, format, status string, at time.Time) string {
		return it.insertUUID(`INSERT INTO import_jobs (resource, actor_id, organization_id, format, status, source_filename, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING uuid::text`, resource, owner.ID, org, format, status, file, at)
	}
	i1 := imp(nil, tag+".b", "people.csv", "csv", "applied", day)
	i2 := imp(nil, tag+".a", "stock.xlsx", "xlsx", "rolled_back", day.AddDate(0, 0, 1))
	it3 := imp(dealer.ID, tag+".t", "tenant.tsv", "tsv", "failed", day)
	im := listSortCase{it: it, admin: admin, base: "/v1/platform/imports", extra: url.Values{"q": {tag}}}
	// The platform imports list keeps excluding tenant jobs.
	im.expect(nil, i2, i1)
	im.expect(sortParam("resource"), i2, i1)
	im.expect(url.Values{"status": {"rolled_back"}}, i2)
	im.expect(url.Values{"format": {"csv,xlsx"}, "sort": {"created_at"}}, i1, i2)
	im.expect400(url.Values{"format": {"pdf"}}, "format")
	// q also matches the source file name.
	byFile := listSortCase{it: it, admin: admin, base: "/v1/platform/imports", extra: url.Values{"q": {"stock.xlsx"}, "resource": {tag + ".a"}}}
	byFile.expect(nil, i2)
	tim := listSortCase{it: it, admin: staffTok, base: "/v1/tenant/imports", extra: url.Values{"q": {tag}}}
	tim.expect(nil, it3)
	tim.expect(url.Values{"status": {"failed"}, "format": {"tsv"}}, it3)
	_ = ctx
}
