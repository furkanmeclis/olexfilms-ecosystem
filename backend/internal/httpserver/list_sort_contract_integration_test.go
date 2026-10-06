package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// TEC-363: platform users, organizations, notifications and logs apply the
// whitelisted primary sort in SQL with an id tiebreak, reject unknown sort
// fields and accept multi-value status filters (docs/list-contract.md).

type listSortCase struct {
	it    *itest
	admin string
	base  string
	extra url.Values
}

// uuids returns the item uuids of one list call in response order.
func (c listSortCase) uuids(params url.Values) []string {
	c.it.t.Helper()
	v := url.Values{}
	for k, vals := range c.extra {
		v[k] = vals
	}
	for k, vals := range params {
		v[k] = vals
	}
	code, env := c.it.do("GET", c.base+"?"+v.Encode(), hostOlex, c.admin, nil)
	if code != http.StatusOK {
		c.it.t.Fatalf("GET %s?%s = %d %s", c.base, v.Encode(), code, errCode(env))
	}
	var page struct {
		Items []struct {
			UUID string `json:"uuid"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		c.it.t.Fatal(err)
	}
	if int(page.Total) != len(page.Items) {
		c.it.t.Fatalf("GET %s?%s total %d != items %d", c.base, v.Encode(), page.Total, len(page.Items))
	}
	out := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, item.UUID)
	}
	return out
}

func (c listSortCase) expect(params url.Values, want ...string) {
	c.it.t.Helper()
	got := c.uuids(params)
	if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
		c.it.t.Fatalf("GET %s %v order = %v, want %v", c.base, params, got, want)
	}
}

func (c listSortCase) expect400(params url.Values, field string) {
	c.it.t.Helper()
	v := url.Values{}
	for k, vals := range c.extra {
		v[k] = vals
	}
	for k, vals := range params {
		v[k] = vals
	}
	req := httptest.NewRequest("GET", c.base+"?"+v.Encode(), nil)
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("Authorization", "Bearer "+c.admin)
	rec := httptest.NewRecorder()
	c.it.handler.ServeHTTP(rec, req)
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details []struct {
				Field string `json:"field"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusBadRequest || body.Error.Code != response.CodeValidationError ||
		len(body.Error.Details) == 0 || body.Error.Details[0].Field != field {
		c.it.t.Fatalf("GET %s?%s = %d %s, want 400 VALIDATION_ERROR on %q", c.base, v.Encode(), rec.Code, rec.Body.String(), field)
	}
}

func sortParam(s string) url.Values { return url.Values{"sort": {s}} }

func (it *itest) metaSort(admin, path string) (string, []string) {
	it.t.Helper()
	code, env := it.do("GET", path, hostOlex, admin, nil)
	if code != http.StatusOK {
		it.t.Fatalf("meta %s = %d", path, code)
	}
	var m struct {
		DefaultSort    string   `json:"default_sort"`
		SortableFields []string `json:"sortable_fields"`
	}
	if err := json.Unmarshal(env.Data, &m); err != nil {
		it.t.Fatal(err)
	}
	return m.DefaultSort, m.SortableFields
}

func TestIntegrationListSortPlatformUsers(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	tag := "srt" + it.suffix[len(it.suffix)-9:]
	var us []db.User
	for _, n := range []string{"1", "2", "3", "4"} {
		u, _ := it.user(tag + "-" + n)
		us = append(us, u)
	}
	// names: u1=b, u2=a, u3=c, u4=a (tie with u2); statuses: u1 pending, u3 disabled.
	for i, name := range []string{"b", "a", "c", "a"} {
		if _, err := it.pool.Exec(ctx, "UPDATE users SET name = $2 WHERE id = $1", us[i].ID, tag+"-"+name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := it.pool.Exec(ctx, "UPDATE users SET status = 'pending' WHERE id = $1", us[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := it.pool.Exec(ctx, "UPDATE users SET status = 'disabled' WHERE id = $1", us[2].ID); err != nil {
		t.Fatal(err)
	}
	u1, u2, u3, u4 := us[0].Uuid.String(), us[1].Uuid.String(), us[2].Uuid.String(), us[3].Uuid.String()
	c := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/users", extra: url.Values{"q": {tag}}}

	c.expect(sortParam("name"), u2, u4, u1, u3)
	c.expect(sortParam("-name"), u3, u1, u4, u2)
	c.expect(sortParam("-name,email"), u3, u1, u4, u2) // extra fields ignored
	c.expect(nil, u4, u3, u2, u1)                      // default -created_at, id tiebreak
	c.expect(url.Values{"status": {"active"}, "sort": {"name"}}, u2, u4)
	c.expect(url.Values{"status": {"active,pending"}, "sort": {"name"}}, u2, u4, u1)
	c.expect400(sortParam("bogus"), "sort")
	c.expect400(url.Values{"status": {"active,bogus"}}, "status")

	def, fields := it.metaSort(c.admin, "/v1/platform/users/meta")
	if def != "-created_at" || len(fields) != 6 {
		t.Fatalf("users meta default_sort=%q sortable=%v", def, fields)
	}
}

func TestIntegrationListSortPlatformOrganizations(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	tag := "srt" + it.suffix[len(it.suffix)-9:]
	var os []db.Organization
	for _, n := range []string{"1", "2", "3", "4"} {
		os = append(os, it.org(tag+"-"+n, "dealer", center))
	}
	cities := []string{"Bursa", "Adana", "Ceyhan", "Adana"}
	ends := []any{time.Now().Add(24 * time.Hour), nil, time.Now().Add(72 * time.Hour), nil}
	for i, o := range os {
		if _, err := it.pool.Exec(ctx, "UPDATE organizations SET city = $2, access_ends_at = $3 WHERE id = $1",
			o.ID, cities[i], ends[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := it.pool.Exec(ctx, "UPDATE organizations SET status = 'suspended' WHERE id = $1", os[0].ID); err != nil {
		t.Fatal(err)
	}
	o1, o2, o3, o4 := os[0].Uuid.String(), os[1].Uuid.String(), os[2].Uuid.String(), os[3].Uuid.String()
	c := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/organizations", extra: url.Values{"q": {tag}}}

	c.expect(sortParam("city"), o2, o4, o1, o3)
	c.expect(sortParam("-city"), o3, o1, o4, o2)
	// Nullable column: NULLs last in both directions, id tiebreak follows direction.
	c.expect(sortParam("access_ends_at"), o1, o3, o2, o4)
	c.expect(sortParam("-access_ends_at"), o3, o1, o4, o2)
	c.expect(sortParam("-name"), o4, o3, o2, o1)
	c.expect(nil, o4, o3, o2, o1)
	c.expect(url.Values{"status": {"suspended,read_only"}}, o1)
	c.expect(url.Values{"status": {"active,suspended"}, "sort": {"name"}}, o1, o2, o3, o4)
	c.expect400(sortParam("phone"), "sort")
	c.expect400(url.Values{"status": {"archived"}}, "status")

	def, fields := it.metaSort(c.admin, "/v1/platform/organizations/meta")
	if def != "-created_at" || !containsStr(fields, "city") || !containsStr(fields, "access_ends_at") {
		t.Fatalf("organizations meta default_sort=%q sortable=%v", def, fields)
	}
}

func TestIntegrationListSortPlatformNotifications(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	owner, _ := it.user("srt-notif")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM notifications WHERE user_id = $1", owner.ID)
	})
	at := time.Now().UTC().Truncate(time.Second)
	var ns []string
	for _, row := range []struct{ status, priority string }{
		{"sent", "high"}, {"queued", "low"}, {"failed", "critical"}, {"queued", "low"},
	} {
		var id string
		if err := it.pool.QueryRow(ctx, `INSERT INTO notifications (user_id, channel, status, priority, title, created_at)
			VALUES ($1, 'inapp', $2, $3, 'sort test', $4) RETURNING uuid::text`,
			owner.ID, row.status, row.priority, at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ns = append(ns, id)
	}
	n1, n2, n3, n4 := ns[0], ns[1], ns[2], ns[3]
	c := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/notifications",
		extra: url.Values{"user_uuid": {owner.Uuid.String()}}}

	// priority sorts by severity rank, ties by id.
	c.expect(sortParam("priority"), n2, n4, n1, n3)
	c.expect(sortParam("-priority"), n3, n1, n4, n2)
	c.expect(sortParam("status"), n3, n2, n4, n1)
	c.expect(nil, n4, n3, n2, n1) // same created_at: id DESC tiebreak
	c.expect(url.Values{"status": {"queued,failed"}, "sort": {"priority"}}, n2, n4, n3)
	c.expect400(sortParam("title"), "sort")
	c.expect400(url.Values{"status": {"queued,nope"}}, "status")

	def, _ := it.metaSort(c.admin, "/v1/platform/notifications/meta")
	if def != "-created_at" {
		t.Fatalf("notifications meta default_sort=%q", def)
	}
}

func TestIntegrationListSortPlatformLogs(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	tag := "srt-log-" + it.suffix
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM app_logs WHERE message LIKE $1", tag+"%")
	})
	at := time.Date(2026, 1, 5, 15, 0, 0, 0, time.UTC)
	var ls []string
	for _, row := range []struct{ level, source string }{
		{"warn", "b"}, {"debug", "a"}, {"error", "c"}, {"debug", "a"},
	} {
		var id string
		if err := it.pool.QueryRow(ctx, `INSERT INTO app_logs (level, message, source, created_at)
			VALUES ($1, $2, $3, $4) RETURNING uuid::text`, row.level, tag, row.source, at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ls = append(ls, id)
	}
	l1, l2, l3, l4 := ls[0], ls[1], ls[2], ls[3]
	c := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/logs", extra: url.Values{"q": {tag}}}

	c.expect(sortParam("level"), l2, l4, l1, l3) // severity rank, id tiebreak
	c.expect(sortParam("-level"), l3, l1, l4, l2)
	c.expect(sortParam("-source"), l3, l1, l4, l2)
	c.expect(nil, l4, l3, l2, l1)
	c.expect(url.Values{"level": {"warn,error"}, "sort": {"level"}}, l1, l3)
	// Date-only created_to includes the whole day.
	c.expect(url.Values{"created_from": {"2026-01-05"}, "created_to": {"2026-01-05"}, "sort": {"created_at"}}, l1, l2, l3, l4)
	c.expect(url.Values{"created_to": {"2026-01-04"}})
	c.expect400(sortParam("message"), "sort")
	c.expect400(url.Values{"level": {"info"}}, "level")
	c.expect400(url.Values{"created_from": {"2026-01-06"}, "created_to": {"2026-01-05"}}, "created_from")

	def, fields := it.metaSort(c.admin, "/v1/platform/logs/meta")
	if def != "-created_at" || containsStr(fields, "message") {
		t.Fatalf("logs meta default_sort=%q sortable=%v", def, fields)
	}
}

func containsStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
