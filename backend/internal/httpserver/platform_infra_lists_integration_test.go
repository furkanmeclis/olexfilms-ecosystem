package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine/adapters"
)

// TEC-367 (DT-BE-2): platform infrastructure list endpoints follow the list
// contract (docs/list-contract.md): whitelisted sort with an id tiebreak,
// q, multi-value enum filters, date ranges and paging with total.

func TestIntegrationInfraListPlatformNotifications(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	owner, pw := it.user("infra-notif")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM notifications WHERE user_id = $1", owner.ID)
	})
	day := time.Date(2026, 2, 3, 10, 0, 0, 0, time.UTC)
	var ns []string
	for i, row := range []struct {
		channel, priority, tpl, recipient string
		at                                time.Time
	}{
		{"inapp", "high", "orders.created", "", day},
		{"email", "low", "infra.special_tpl", "", day.Add(time.Hour)},
		{"sms", "critical", "orders.shipped", "+905551112233", day.AddDate(0, 0, 1)},
		{"email", "normal", "orders.created", "infra-rcpt@example.test", day.AddDate(0, 0, 2)},
	} {
		var id string
		if err := it.pool.QueryRow(ctx, `INSERT INTO notifications
			(user_id, channel, status, priority, title, template_code, recipient, created_at)
			VALUES ($1, $2, 'sent', $3, $4, $5, NULLIF($6, ''), $7) RETURNING uuid::text`,
			owner.ID, row.channel, row.priority, fmt.Sprintf("infra %d", i), row.tpl, row.recipient, row.at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ns = append(ns, id)
	}
	n1, n2, n3, n4 := ns[0], ns[1], ns[2], ns[3]
	c := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/notifications",
		extra: url.Values{"user_uuid": {owner.Uuid.String()}}}

	c.expect(url.Values{"priority": {"high,critical"}, "sort": {"priority"}}, n1, n3)
	c.expect(url.Values{"channel": {"email,sms"}, "sort": {"created_at"}}, n2, n3, n4)
	c.expect(url.Values{"created_from": {"2026-02-03"}, "created_to": {"2026-02-03"}, "sort": {"created_at"}}, n1, n2)
	c.expect(url.Values{"created_from": {"2026-02-04"}, "sort": {"-created_at"}}, n4, n3)
	// q also searches template_code and recipient.
	c.expect(url.Values{"q": {"special_tpl"}}, n2)
	c.expect(url.Values{"q": {"infra-rcpt@"}}, n4)
	c.expect400(url.Values{"priority": {"urgent"}}, "priority")
	c.expect400(url.Values{"channel": {"fax"}}, "channel")
	c.expect400(url.Values{"created_from": {"03.02.2026"}}, "created_from")

	// The export adapter reads the same filters and sort.
	ds, err := adapters.NewNotifications(it.q).Export(ctx, ioengine.ExportQuery{
		"user_uuid": owner.Uuid.String(), "priority": "low,critical,normal", "created_from": "2026-02-03",
		"sort": "priority",
	}, i18n.DefaultLocale)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range ds.Rows {
		got = append(got, row["uuid"].(string))
	}
	if !slices.Equal(got, []string{n2, n4, n3}) {
		t.Fatalf("export rows = %v, want %v", got, []string{n2, n4, n3})
	}
	if _, err := adapters.NewNotifications(it.q).Export(ctx, ioengine.ExportQuery{"sort": "title"}, i18n.DefaultLocale); err == nil {
		t.Fatal("export with unknown sort succeeded")
	}

	// Tenant inbox: the validated sort is applied.
	org := it.org("infra-notif-dist", "distributor", it.brandCenter("olex"))
	it.member(org, owner, "owner")
	tok := it.loginOrg(owner, pw, org)
	inbox := listSortCase{it: it, admin: tok, base: "/v1/notifications"}
	inbox.expect(sortParam("priority"), n2, n4, n1, n3)
	inbox.expect(sortParam("-created_at"), n4, n3, n2, n1)
	inbox.expect(sortParam("channel"), n2, n4, n1, n3)
	inbox.expect400(sortParam("title"), "sort")
}

func TestIntegrationInfraListNotificationDeliveries(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	u, _ := it.user("infra-deliv")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM notification_deliveries WHERE user_id = $1", u.ID)
	})
	day := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	var ds []string
	for _, row := range []struct {
		code, channel, status string
		at                    time.Time
	}{
		{"orders.created", "email", "sent", day},
		{"infra.zeta", "inapp", "failed", day.Add(time.Hour)},
		{"orders.created", "sms", "skipped_disabled", day.AddDate(0, 0, 3)},
	} {
		var id string
		if err := it.pool.QueryRow(ctx, `INSERT INTO notification_deliveries
			(event_id, event_code, user_id, channel, status, created_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5) RETURNING uuid::text`,
			row.code, u.ID, row.channel, row.status, row.at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ds = append(ds, id)
	}
	d1, d2, d3 := ds[0], ds[1], ds[2]
	c := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/notification-deliveries",
		extra: url.Values{"user_uuid": {u.Uuid.String()}}}

	c.expect(nil, d3, d2, d1) // default -created_at
	c.expect(sortParam("created_at"), d1, d2, d3)
	c.expect(sortParam("status"), d2, d1, d3)
	c.expect(sortParam("-channel"), d3, d2, d1)
	c.expect(sortParam("event_code"), d2, d1, d3)
	c.expect(url.Values{"status": {"sent,failed"}, "sort": {"created_at"}}, d1, d2)
	c.expect(url.Values{"channel": {"sms,email"}, "sort": {"created_at"}}, d1, d3)
	c.expect(url.Values{"created_from": {"2026-04-01"}, "created_to": {"2026-04-01"}, "sort": {"created_at"}}, d1, d2)
	c.expect(url.Values{"q": {"zeta"}}, d2)
	c.expect(url.Values{"q": {u.Email.String}, "sort": {"created_at"}}, d1, d2, d3)
	c.expect400(sortParam("user_email"), "sort")
	c.expect400(url.Values{"status": {"read"}}, "status")
	c.expect400(url.Values{"channel": {"push"}}, "channel")
}

func TestIntegrationInfraListDocumentTemplates(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	tag := "infratpl" + it.suffix[len(it.suffix)-8:]
	var olexID int64
	if err := it.pool.QueryRow(ctx, "SELECT id FROM brands WHERE slug = 'olex'").Scan(&olexID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM document_templates WHERE name LIKE $1", tag+"%")
	})
	base := 9000 + int(time.Now().UnixNano()%500)*3
	var ts []string
	for i, row := range []struct {
		kind, lang, name string
	}{
		{"measurement", "en", tag + " b"},
		{"contract", "tr", tag + " c"},
		{"measurement", "de", tag + " a"},
	} {
		var id string
		// Published, not active: superseded versions (no active/draft
		// uniqueness clash with the seed templates).
		if err := it.pool.QueryRow(ctx, `INSERT INTO document_templates
			(kind, brand_id, language, name, version, is_active, html, content_hash, published_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, FALSE, '<p>x</p>', repeat('a', 64), NOW(), NOW() + make_interval(mins => $6::int))
			RETURNING uuid::text`, row.kind, olexID, row.lang, row.name, base+i, i).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ts = append(ts, id)
	}
	t1, t2, t3 := ts[0], ts[1], ts[2]
	c := listSortCase{it: it, admin: it.adminToken(), base: "/v1/platform/document-templates",
		extra: url.Values{"q": {tag}, "current": {"false"}}}

	c.expect(sortParam("name"), t3, t1, t2)
	c.expect(sortParam("-name"), t2, t1, t3)
	c.expect(sortParam("language"), t3, t1, t2)
	c.expect(sortParam("-version"), t3, t2, t1)
	c.expect(sortParam("-updated_at"), t3, t2, t1)
	c.expect(nil, t2, t3, t1) // default kind; then brand, language, version DESC
	c.expect(url.Values{"kind": {"measurement"}, "sort": {"name"}}, t3, t1)
	c.expect(url.Values{"language": {"de,tr"}, "sort": {"name"}}, t3, t2)
	c.expect(url.Values{"status": {"superseded"}, "sort": {"name"}}, t3, t1, t2)
	c.expect(url.Values{"status": {"draft,active"}})
	c.expect(url.Values{"brand": {"olex"}, "sort": {"name"}}, t3, t1, t2)
	c.expect(url.Values{"platform_default": {"true"}})
	c.expect400(sortParam("html"), "sort")
	c.expect400(url.Values{"status": {"published"}}, "status")
	c.expect400(url.Values{"kind": {"poster"}}, "kind")
	c.expect400(url.Values{"language": {"xx"}}, "language")
}

func TestIntegrationInfraListDealerModules(t *testing.T) {
	it := newIntegration(t)
	net := it.featureNet(3)
	key := features.ModuleLeads
	b := net.dealers[1]
	code, env := it.do("PUT", "/v1/platform/organizations/"+b.Uuid.String()+"/modules/"+key, hostOlex, net.admin, map[string]any{"enabled": false})
	if code != http.StatusOK {
		t.Fatalf("admin -> dealer = %d %s", code, errCode(env))
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM module_flags WHERE organization_id = $1", b.ID)
	})
	a, c3 := net.dealers[0].Uuid.String(), net.dealers[2].Uuid.String()
	bu := b.Uuid.String()
	list := func(q string) ([]string, int64, int32) {
		t.Helper()
		code, env := it.do("GET", "/v1/tenant/modules/dealers?"+q, hostOlex, net.distTok, nil)
		if code != http.StatusOK {
			t.Fatalf("dealers %s = %d %s", q, code, errCode(env))
		}
		var page struct {
			Items []struct {
				UUID string `json:"uuid"`
			} `json:"items"`
			Total int64 `json:"total"`
			Limit int32 `json:"limit"`
		}
		if err := json.Unmarshal(env.Data, &page); err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, d := range page.Items {
			out = append(out, d.UUID)
		}
		return out, page.Total, page.Limit
	}
	if got, total, _ := list(""); total != 3 || !slices.Equal(got, []string{a, bu, c3}) {
		t.Fatalf("default = %v total=%d", got, total)
	}
	if got, _, _ := list("sort=-name"); !slices.Equal(got, []string{c3, bu, a}) {
		t.Fatalf("-name = %v", got)
	}
	if got, total, limit := list("sort=name&limit=1&offset=1"); total != 3 || limit != 1 || !slices.Equal(got, []string{bu}) {
		t.Fatalf("page = %v total=%d limit=%d", got, total, limit)
	}
	if got, total, _ := list("q=" + url.QueryEscape("feat-dealer-c")); total != 1 || !slices.Equal(got, []string{c3}) {
		t.Fatalf("q = %v", got)
	}
	if got, _, _ := list("module=" + key + "&state=disabled"); !slices.Equal(got, []string{bu}) {
		t.Fatalf("state disabled = %v", got)
	}
	if got, _, _ := list("module=" + key + "&state=enabled"); !slices.Equal(got, []string{a, c3}) {
		t.Fatalf("state enabled = %v", got)
	}
	if got, _, _ := list("module=" + key + "&source=admin"); !slices.Equal(got, []string{bu}) {
		t.Fatalf("source admin = %v", got)
	}
	for _, q := range []string{"sort=created_at", "state=enabled", "module=" + key + "&state=on", "module=" + key + "&source=nope"} {
		if code, env := it.do("GET", "/v1/tenant/modules/dealers?"+q, hostOlex, net.distTok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
			t.Fatalf("dealers %s = %d %s; want 400", q, code, errCode(env))
		}
	}
}

func TestIntegrationInfraListAnnouncementsManage(t *testing.T) {
	it := newIntegration(t)
	net := it.featureNet(1)
	tag := "infra-ann-" + it.suffix
	create := func(title string, pinned bool) string {
		t.Helper()
		code, env := it.do("POST", "/v1/announcements", hostOlex, net.distTok, map[string]any{
			"title": tag + " " + title, "body": "body", "default_locale": "tr", "body_format": "markdown",
			"pinned": pinned, "audiences": []map[string]any{{"target_type": "subtree", "target_organization_uuid": net.dist.Uuid.String()}},
		})
		if code != http.StatusCreated {
			t.Fatalf("create %s = %d %s", title, code, errCode(env))
		}
		var a struct {
			UUID string `json:"uuid"`
		}
		_ = json.Unmarshal(env.Data, &a)
		return a.UUID
	}
	a1 := create("b", false)
	a2 := create("a", true)
	a3 := create("c", false)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM announcements WHERE title LIKE $1", tag+"%")
	})
	if code, env := it.do("POST", "/v1/announcements/"+a2+"/publish", hostOlex, net.distTok, nil); code != http.StatusOK {
		t.Fatalf("publish = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/announcements/"+a3+"/archive", hostOlex, net.distTok, nil); code != http.StatusOK {
		t.Fatalf("archive = %d %s", code, errCode(env))
	}
	c := listSortCase{it: it, admin: net.distTok, base: "/v1/announcements/manage", extra: url.Values{"q": {tag}}}
	c.expect(nil, a3, a2, a1) // default -created_at
	c.expect(sortParam("title"), a2, a1, a3)
	c.expect(sortParam("-status"), a2, a1, a3)
	c.expect(url.Values{"status": {"draft,archived"}, "sort": {"title"}}, a1, a3)
	c.expect(url.Values{"pinned": {"true"}}, a2)
	today := time.Now().UTC().Format(time.DateOnly)
	c.expect(url.Values{"publish_from": {today}, "publish_to": {today}}, a2)
	c.expect(sortParam("-publish_at"), a2, a3, a1) // NULL publish_at last, id tiebreak
	c.expect400(sortParam("body"), "sort")
	c.expect400(url.Values{"status": {"deleted"}}, "status")
	c.expect400(url.Values{"pinned": {"yes"}}, "pinned")

	// The reader feed still shows published rows only.
	if code, env := it.do("GET", "/v1/announcements?limit=100", hostOlex, net.distTok, nil); code != http.StatusOK ||
		strings.Contains(string(env.Data), a1) || strings.Contains(string(env.Data), a3) {
		t.Fatalf("reader feed = %d %s", code, env.Data)
	}
	// A dealer (no write scope) cannot use the author list.
	if code, _ := it.do("GET", "/v1/announcements/manage", hostOlex, net.dealerTok, nil); code != http.StatusForbidden {
		t.Fatalf("dealer manage = %d; want 403", code)
	}

	// Read report: limit above 100 is honoured (capped at 500).
	code, env := it.do("GET", "/v1/announcements/"+a2+"/reads?limit=200", hostOlex, net.distTok, nil)
	if code != http.StatusOK {
		t.Fatalf("reads = %d %s", code, errCode(env))
	}
	var rep struct {
		Limit int32 `json:"limit"`
		Items []any `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &rep); err != nil || rep.Limit != 200 || rep.Items == nil {
		t.Fatalf("reads = %s", env.Data)
	}
	code, env = it.do("GET", "/v1/announcements/"+a2+"/reads?limit=5000", hostOlex, net.distTok, nil)
	if err := json.Unmarshal(env.Data, &rep); code != http.StatusOK || err != nil || rep.Limit != 500 {
		t.Fatalf("reads cap = %d %s", code, env.Data)
	}
}

func TestIntegrationInfraPlateFormatReorder(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	admin := it.adminToken()
	type pf struct {
		Country   string `json:"country_iso2"`
		SortOrder int32  `json:"sort_order"`
	}
	list := func() []pf {
		t.Helper()
		code, env := it.do("GET", "/v1/platform/plate-formats", hostOlex, admin, nil)
		if code != http.StatusOK {
			t.Fatalf("list = %d %s", code, errCode(env))
		}
		var out struct {
			Items []pf `json:"items"`
		}
		_ = json.Unmarshal(env.Data, &out)
		return out.Items
	}
	before := list()
	if len(before) < 3 {
		t.Skipf("need 3 plate formats, have %d", len(before))
	}
	t.Cleanup(func() {
		for _, f := range before {
			_, _ = it.pool.Exec(context.Background(), `UPDATE plate_formats SET sort_order = $2
				WHERE country_id = (SELECT id FROM countries WHERE iso2 = $1)`, f.Country, f.SortOrder)
		}
	})
	_ = ctx
	codes := func(xs []pf) []string {
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			out = append(out, x.Country)
		}
		return out
	}
	orig := codes(before)

	// Full reverse.
	rev := slices.Clone(orig)
	slices.Reverse(rev)
	code, env := it.do("PUT", "/v1/platform/plate-formats/order", hostOlex, admin, map[string]any{"countries": rev})
	if code != http.StatusOK {
		t.Fatalf("reorder = %d %s", code, errCode(env))
	}
	after := list()
	if !slices.Equal(codes(after), rev) {
		t.Fatalf("after reverse = %v, want %v", codes(after), rev)
	}
	for i, f := range after {
		if f.SortOrder != int32((i+1)*10) {
			t.Fatalf("sort_order[%d] = %d", i, f.SortOrder)
		}
	}
	// Subset: swapping the first and third keeps the others in place.
	sub := []string{strings.ToLower(rev[2]), rev[0]}
	if code, env := it.do("PUT", "/v1/platform/plate-formats/order", hostOlex, admin, map[string]any{"countries": sub}); code != http.StatusOK {
		t.Fatalf("subset reorder = %d %s", code, errCode(env))
	}
	want := slices.Clone(rev)
	want[0], want[2] = rev[2], rev[0]
	if got := codes(list()); !slices.Equal(got, want) {
		t.Fatalf("after subset = %v, want %v", got, want)
	}

	for _, body := range []map[string]any{
		{"countries": []string{}},
		{"countries": []string{rev[0], rev[0]}},
		{"countries": []string{"T"}},
		{"countries": []string{"QQ"}},
	} {
		if code, env := it.do("PUT", "/v1/platform/plate-formats/order", hostOlex, admin, body); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
			t.Fatalf("reorder %v = %d %s; want 400", body, code, errCode(env))
		}
	}
	// Writers only.
	u, pw := it.user("infra-plate")
	tok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": u.Email.String, "password": pw,
	})).AccessToken
	if code, _ := it.do("PUT", "/v1/platform/plate-formats/order", hostOlex, tok, map[string]any{"countries": rev}); code != http.StatusForbidden {
		t.Fatalf("plain user reorder = %d; want 403", code)
	}
}
