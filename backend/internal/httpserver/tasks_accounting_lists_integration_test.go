package httpserver

// TEC-379 (DT-BE-8): list contract of the task list (+ bulk set_status /
// set_priority, "select all matching"), the accounting account, cari, entry
// and dispute lists, and the entry list export.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	accountingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// dt8List GETs a page and returns the code, the item uuids and the total.
func (it *itest) dt8List(token, path string) (int, []string, int64) {
	it.t.Helper()
	code, env := it.do("GET", path, hostOlex, token, nil)
	if code != http.StatusOK {
		return code, nil, 0
	}
	var page struct {
		Items []struct {
			UUID string `json:"uuid"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		it.t.Fatalf("%s: %s", path, env.Data)
	}
	out := make([]string, 0, len(page.Items))
	for _, i := range page.Items {
		out = append(out, i.UUID)
	}
	return code, out, page.Total
}

// dt8Expect checks the uuid order of a list page.
func (it *itest) dt8Expect(token, path string, want ...string) {
	it.t.Helper()
	code, got, _ := it.dt8List(token, path)
	if code != http.StatusOK {
		it.t.Fatalf("%s = %d", path, code)
	}
	if !slices.Equal(got, want) {
		it.t.Fatalf("%s = %v, want %v", path, got, want)
	}
}

func (it *itest) dt8Status(token, path string, want int) {
	it.t.Helper()
	if code, env := it.do("GET", path, hostOlex, token, nil); code != want {
		it.t.Fatalf("%s = %d %s, want %d", path, code, errCode(env), want)
	}
}

func dt8Rev(in ...string) []string {
	out := slices.Clone(in)
	slices.Reverse(out)
	return out
}

func (it *itest) taskState(id string) (status, priority string, closed bool) {
	it.t.Helper()
	if err := it.pool.QueryRow(context.Background(),
		"SELECT status, priority, closed_at IS NOT NULL FROM tasks WHERE uuid = $1", id).Scan(&status, &priority, &closed); err != nil {
		it.t.Fatalf("task %s: %v", id, err)
	}
	return status, priority, closed
}

// Task list: every sort field both ways, CSV status / priority / subject,
// q, due and created ranges, 400s; bulk set_status / set_priority on ids
// and on "every matching task" with undo.
func TestIntegrationTaskAccountingListsTasks(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("dt8-dist", "distributor", center)
	dealer := it.org("dt8-dealer", "dealer", dist)
	staff, staffPW := it.user("dt8-staff")
	it.member(center, staff, "staff")
	dealerUser, dealerPW := it.user("dt8-dealerown")
	it.member(dealer, dealerUser, "owner")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM tasks WHERE subject_org_id = ANY($1)", []int64{dist.ID, dealer.ID})
	})
	tok := it.catalogLogin(staff, staffPW, center, hostOlex)
	dealerTok := it.catalogLogin(dealerUser, dealerPW, dealer, hostOlex)

	now := time.Now().UTC().Truncate(time.Second)
	mk := func(subject db.Organization, title, desc, prio string, due *time.Time) string {
		body := map[string]any{"subject_organization_uuid": subject.Uuid.String(), "title": title + " " + it.suffix,
			"description": desc, "priority": prio}
		if due != nil {
			body["due_at"] = due.Format(time.RFC3339)
		}
		return decodeTask(t, it.taskDo("POST", "/v1/tasks", tok, body, http.StatusCreated)).UUID
	}
	d24, d48, d72 := now.Add(24*time.Hour), now.Add(48*time.Hour), now.Add(72*time.Hour)
	t1 := mk(dist, "Alpha", "Kira %50 indirim", "low", &d48)
	t2 := mk(dealer, "Beta", "", "urgent", nil)
	t3 := mk(dealer, "Gamma", "", "high", &d24)
	t4 := mk(dist, "Delta", "", "normal", &d72)
	it.taskDo("PATCH", "/v1/tasks/"+t3, tok, map[string]any{"status": "in_progress"}, http.StatusOK)
	it.taskDo("PATCH", "/v1/tasks/"+t4, tok, map[string]any{"status": "done"}, http.StatusOK)

	base := "/v1/tasks?subject_organization_uuid=" + dist.Uuid.String() + "," + dealer.Uuid.String()
	it.dt8Expect(tok, base, t4, t3, t2, t1)
	for sort, asc := range map[string][]string{
		"created_at": {t1, t2, t3, t4},
		"title":      {t1, t2, t4, t3},
		"priority":   {t1, t4, t3, t2},
		"subject":    {t2, t3, t1, t4}, // dealer name < distributor name
	} {
		it.dt8Expect(tok, base+"&sort="+sort, asc...)
		it.dt8Expect(tok, base+"&sort=-"+sort, dt8Rev(asc...)...)
	}
	it.dt8Expect(tok, base+"&sort=status", t1, t2, t3, t4)
	it.dt8Expect(tok, base+"&sort=-status", t4, t3, t2, t1)
	// Tasks without a deadline stay last both ways.
	it.dt8Expect(tok, base+"&sort=due_at", t3, t1, t4, t2)
	it.dt8Expect(tok, base+"&sort=-due_at", t4, t1, t3, t2)
	it.dt8Expect(tok, base+"&sort=updated_at&status=open", t1, t2)

	it.dt8Expect(tok, base+"&status=active", t3, t2, t1)
	it.dt8Expect(tok, base+"&status=open,done", t4, t2, t1)
	it.dt8Expect(tok, base+"&priority=high,urgent", t3, t2)
	it.dt8Expect(tok, "/v1/tasks?subject_organization_uuid="+dealer.Uuid.String(), t3, t2)
	it.dt8Expect(tok, base+"&q=kira", t1)
	it.dt8Expect(tok, base+"&q=GAMMA", t3)
	it.dt8Expect(tok, base+"&q=%25", t1) // a literal %, not a wildcard
	it.dt8Expect(tok, base+"&q=a_p")     // _ is literal too
	mid := url.QueryEscape(now.Add(36 * time.Hour).Format(time.RFC3339))
	it.dt8Expect(tok, base+"&due_from="+mid, t4, t1)
	it.dt8Expect(tok, base+"&due_to="+mid, t3)
	today := now.Format(time.DateOnly)
	it.dt8Expect(tok, base+"&created_from="+today+"&created_to="+today, t4, t3, t2, t1)
	it.dt8Expect(tok, base+"&created_from="+now.AddDate(0, 0, 1).Format(time.DateOnly))
	if _, _, total := it.dt8List(tok, base+"&limit=2&sort=title"); total != 4 {
		t.Fatalf("total = %d", total)
	}
	for _, bad := range []string{
		"&sort=comment_count", "&status=closed", "&priority=p0", "&source=rule", "&subject_organization_uuid=x",
		"&created_from=yesterday", "&due_from=" + today + "&due_after=" + url.QueryEscape(now.Format(time.RFC3339)),
		"&due_after=" + today, "&mine=maybe",
	} {
		it.dt8Status(tok, base+bad, http.StatusBadRequest)
	}
	it.dt8Status(dealerTok, base, http.StatusForbidden)

	// Bulk set_status on ids: closing stamps closed_at; undo reopens.
	run := it.bulkRun("/v1/tasks/bulk", tok, map[string]any{
		"action": "set_status",
		"target": map[string]any{"scope": "ids", "ids": []string{t1, t2}, "params": map[string]string{"status": "done"}},
	})
	if run.Summary.Succeeded != 2 {
		t.Fatalf("set_status run = %+v", run)
	}
	if st, _, closed := it.taskState(t1); st != "done" || !closed {
		t.Fatalf("t1 after bulk done = %s closed=%v", st, closed)
	}
	undone, _ := it.bulkUndo("/v1/tenant/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, tok, http.StatusOK)
	if undone.UndoStatus != bulkusecase.UndoUndone || undone.UndoResult.Restored != 2 {
		t.Fatalf("status undo = %+v", undone)
	}
	if st, _, closed := it.taskState(t2); st != "open" || closed {
		t.Fatalf("t2 after undo = %s closed=%v", st, closed)
	}

	// "Every matching task": the active dealer tasks (t2 open, t3
	// in_progress) are cancelled; t4 (done, distributor) is untouched.
	run = it.bulkRun("/v1/tasks/bulk", tok, map[string]any{
		"action": "set_status",
		"target": map[string]any{"scope": "query", "params": map[string]string{"status": "cancelled"},
			"query": map[string]string{"subject_organization_uuid": dealer.Uuid.String(), "status": "active"}},
	})
	if run.Summary.Total != 2 || run.Summary.Succeeded != 2 {
		t.Fatalf("query set_status run = %+v", run)
	}
	if st, _, _ := it.taskState(t3); st != "cancelled" {
		t.Fatalf("t3 = %s", st)
	}
	it.bulkUndo("/v1/tenant/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, tok, http.StatusOK)
	if st, _, closed := it.taskState(t3); st != "in_progress" || closed {
		t.Fatalf("t3 after undo = %s closed=%v", st, closed)
	}
	// done → cancelled keeps the closing; undo restores done.
	run = it.bulkRun("/v1/tasks/bulk", tok, map[string]any{
		"action": "set_status",
		"target": map[string]any{"scope": "ids", "ids": []string{t4}, "params": map[string]string{"status": "cancelled"}},
	})
	if st, _, closed := it.taskState(t4); st != "cancelled" || !closed {
		t.Fatalf("t4 = %s closed=%v", st, closed)
	}
	it.bulkUndo("/v1/tenant/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, tok, http.StatusOK)
	if st, _, closed := it.taskState(t4); st != "done" || !closed {
		t.Fatalf("t4 after undo = %s closed=%v", st, closed)
	}

	// set_priority with undo.
	run = it.bulkRun("/v1/tasks/bulk", tok, map[string]any{
		"action": "set_priority",
		"target": map[string]any{"scope": "ids", "ids": []string{t1, t3}, "params": map[string]string{"priority": "urgent"}},
	})
	if _, p, _ := it.taskState(t1); p != "urgent" || run.Summary.Succeeded != 2 {
		t.Fatalf("t1 priority = %s, run %+v", p, run)
	}
	it.bulkUndo("/v1/tenant/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, tok, http.StatusOK)
	if _, p, _ := it.taskState(t1); p != "low" {
		t.Fatalf("t1 priority after undo = %s", p)
	}
	if _, p, _ := it.taskState(t3); p != "high" {
		t.Fatalf("t3 priority after undo = %s", p)
	}

	// Bad params and bad target queries are refused before anything runs;
	// a dealer has no tasks.write.
	for name, body := range map[string]map[string]any{
		"bad status":   {"action": "set_status", "target": map[string]any{"scope": "ids", "ids": []string{t1}, "params": map[string]string{"status": "closed"}}},
		"bad priority": {"action": "set_priority", "target": map[string]any{"scope": "ids", "ids": []string{t1}, "params": map[string]string{"priority": "p0"}}},
		"bad sort":     {"action": "set_priority", "target": map[string]any{"scope": "query", "query": map[string]string{"sort": "x"}, "params": map[string]string{"priority": "low"}}},
		"mine":         {"action": "set_priority", "target": map[string]any{"scope": "query", "query": map[string]string{"mine": "true"}, "params": map[string]string{"priority": "low"}}},
	} {
		if code, env := it.do("POST", "/v1/tasks/bulk", hostOlex, tok, body); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, code, errCode(env))
		}
	}
	if code, _ := it.do("POST", "/v1/tasks/bulk", hostOlex, dealerTok, map[string]any{
		"action": "set_status", "target": map[string]any{"scope": "ids", "ids": []string{t1}, "params": map[string]string{"status": "done"}},
	}); code != http.StatusForbidden {
		t.Fatalf("dealer bulk = %d", code)
	}
	if _, p, _ := it.taskState(t1); p != "low" {
		t.Fatalf("t1 changed by a refused run: %s", p)
	}

	// TEC-496: source filter (auto = opened by a weak dealer rule).
	auto, err := db.New(it.pool).InsertTask(context.Background(), db.InsertTaskParams{
		OrganizationID: center.ID, BrandID: center.BrandID, SubjectOrgID: dealer.ID,
		Title: "Auto " + it.suffix, Priority: "normal", Source: "auto",
	})
	if err != nil {
		t.Fatalf("auto task: %v", err)
	}
	it.dt8Expect(tok, base+"&source=auto", auto.Uuid.String())
	it.dt8Expect(tok, base+"&source=manual", t4, t3, t2, t1)
	it.dt8Expect(tok, base+"&source=manual,auto", auto.Uuid.String(), t4, t3, t2, t1)
}

// dt8Book is a fresh distributor book with accounts, caris and entries.
type dt8Book struct {
	it                     *itest
	center, dist, d1, d2   db.Organization
	tok, dealerTok         string
	cashA, bankB, cashC    string
	cariD1, cariD2         string
	cariCust, cariCust2    string
	e1, e2, e3, e4, e5, e6 string
}

func dt8NewBook(t *testing.T, it *itest) *dt8Book {
	t.Helper()
	b := &dt8Book{it: it}
	b.center = it.brandCenter("olex")
	b.dist = it.org("dt8b-dist", "distributor", b.center)
	b.d1 = it.org("tal-a-dealer", "dealer", b.dist)
	b.d2 = it.org("tal-b-dealer", "dealer", b.dist)
	owner, pw := it.user("dt8b-owner")
	it.member(b.dist, owner, "owner")
	dealerOwner, dpw := it.user("dt8b-dealer")
	it.member(b.d1, dealerOwner, "owner")
	cust, _ := it.user("tal-c-cust")
	cust2, _ := it.user("tal-d-cust")
	it.serveCustomer(b.dist, cust)
	it.serveCustomer(b.dist, cust2)
	b.tok = it.loginOrg(owner, pw, b.dist)
	b.dealerTok = it.loginOrg(dealerOwner, dpw, b.d1)

	account := func(typ, name string, iban any) string {
		body := map[string]any{"type": typ, "name": name + " " + it.suffix}
		if iban != nil {
			body["iban"] = iban
		}
		return decodeData[accAccount](t, it.accDo("POST", "/v1/accounting/accounts", b.tok, body, http.StatusCreated)).UUID
	}
	b.cashA = account("cash", "Kasa A", nil)
	b.bankB = account("bank", "Banka B", "TR330006100519786457841326")
	b.cashC = account("cash", "Kasa C", nil)
	openCari := func(u db.User) string {
		return decodeData[accCari](t, it.accDo("POST", "/v1/accounting/cari", b.tok, map[string]any{
			"counterparty_type": "user", "counterparty_uuid": u.Uuid.String(),
		}, http.StatusCreated)).UUID
	}
	b.cariCust = openCari(cust)
	b.cariCust2 = openCari(cust2)
	entry := func(body map[string]any) accEntry {
		return decodeData[accEntry](t, it.accDo("POST", "/v1/accounting/entries", b.tok, body, http.StatusCreated))
	}
	e := entry(map[string]any{"direction": "charge", "category": "opening_balance", "amount": "1000",
		"counterparty_organization_uuid": b.d1.Uuid.String(), "description": "Açılış A"})
	b.e1, b.cariD1 = e.UUID, *e.CariUUID
	e = entry(map[string]any{"direction": "charge", "category": "adjustment", "amount": "200",
		"counterparty_organization_uuid": b.d2.Uuid.String(), "description": "Düzeltme"})
	b.e2, b.cariD2 = e.UUID, *e.CariUUID
	b.e3 = entry(map[string]any{"direction": "charge", "category": "opening_balance", "amount": "50",
		"cari_uuid": b.cariCust, "description": "Müşteri"}).UUID
	b.e4 = entry(map[string]any{"direction": "charge", "category": "adjustment", "amount": "100",
		"cari_uuid": b.cariD2, "description": "50% indirim"}).UUID
	b.e5 = entry(map[string]any{"direction": "income", "category": "other_income", "amount": "100",
		"account_uuid": b.cashA, "description": "Faiz"}).UUID
	b.e6 = entry(map[string]any{"direction": "expense", "category": "rent", "amount": "40",
		"account_uuid": b.cashC, "description": "Kira"}).UUID
	return b
}

// Accounts (full array), cari and entry lists of one book; the entry
// export carries the list filters and sort.
func TestIntegrationTaskAccountingListsBooks(t *testing.T) {
	it := dt5Integration(t)
	b := dt8NewBook(t, it)
	tok := b.tok

	// Accounts: default type (then name), every sort both ways, type CSV, q.
	acc := "/v1/accounting/accounts?"
	it.dt8Expect(tok, acc, b.bankB, b.cashA, b.cashC)
	it.dt8Expect(tok, acc+"sort=-type", b.cashC, b.cashA, b.bankB)
	it.dt8Expect(tok, acc+"sort=name", b.bankB, b.cashA, b.cashC)
	it.dt8Expect(tok, acc+"sort=balance", b.cashC, b.bankB, b.cashA)
	it.dt8Expect(tok, acc+"sort=-balance", b.cashA, b.bankB, b.cashC)
	it.dt8Expect(tok, acc+"sort=created_at", b.cashA, b.bankB, b.cashC)
	it.dt8Expect(tok, acc+"sort=-last_entry_at", b.cashC, b.cashA, b.bankB)
	it.dt8Expect(tok, acc+"sort=last_entry_at", b.cashA, b.cashC, b.bankB)
	it.dt8Expect(tok, acc+"type=cash", b.cashA, b.cashC)
	it.dt8Expect(tok, acc+"type=cash,bank&sort=-name", b.cashC, b.cashA, b.bankB)
	it.dt8Expect(tok, acc+"q=banka", b.bankB)
	it.dt8Expect(tok, acc+"q=TR3300061", b.bankB)
	for _, bad := range []string{"type=card", "sort=iban", "active=yes"} {
		it.dt8Status(tok, acc+bad, http.StatusBadRequest)
	}

	// Cari: balances d1 1000, d2 300 (2 entries), cust 50, cust2 0 (no entry).
	cari := "/v1/accounting/cari?"
	it.dt8Expect(tok, cari, b.cariD1, b.cariD2, b.cariCust, b.cariCust2)
	it.dt8Expect(tok, cari+"sort=-name", b.cariCust2, b.cariCust, b.cariD2, b.cariD1)
	it.dt8Expect(tok, cari+"sort=balance", b.cariCust2, b.cariCust, b.cariD2, b.cariD1)
	it.dt8Expect(tok, cari+"sort=-balance", b.cariD1, b.cariD2, b.cariCust, b.cariCust2)
	it.dt8Expect(tok, cari+"sort=last_entry_at", b.cariD1, b.cariCust, b.cariD2, b.cariCust2)
	it.dt8Expect(tok, cari+"sort=-last_entry_at", b.cariD2, b.cariCust, b.cariD1, b.cariCust2)
	it.dt8Expect(tok, cari+"sort=-entry_count", b.cariD2, b.cariD1, b.cariCust, b.cariCust2)
	it.dt8Expect(tok, cari+"sort=created_at", b.cariCust, b.cariCust2, b.cariD1, b.cariD2)
	it.dt8Expect(tok, cari+"counterparty_kind=customer", b.cariCust, b.cariCust2)
	it.dt8Expect(tok, cari+"counterparty_kind=dealer,customer&balance_min=50", b.cariD1, b.cariD2, b.cariCust)
	it.dt8Expect(tok, cari+"balance_min=100&balance_max=500", b.cariD2)
	it.dt8Expect(tok, cari+"q=tal-c", b.cariCust)
	it.dt8Expect(tok, cari+"q=%25")
	if _, _, total := it.dt8List(tok, cari+"limit=1"); total != 4 {
		t.Fatalf("cari total = %d", total)
	}
	for _, bad := range []string{"counterparty_kind=supplier", "balance_min=x", "balance_min=5&balance_max=1", "sort=currency"} {
		it.dt8Status(tok, cari+bad, http.StatusBadRequest)
	}

	// Entries: 1000 (e1), 200 (e2), 50 (e3), 100 (e4), 100 (e5), 40 (e6).
	ent := "/v1/accounting/entries?"
	it.dt8Expect(tok, ent, b.e6, b.e5, b.e4, b.e3, b.e2, b.e1)
	it.dt8Expect(tok, ent+"sort=created_at", b.e1, b.e2, b.e3, b.e4, b.e5, b.e6)
	it.dt8Expect(tok, ent+"sort=amount", b.e6, b.e3, b.e4, b.e5, b.e2, b.e1)
	it.dt8Expect(tok, ent+"sort=-amount", b.e1, b.e2, b.e5, b.e4, b.e3, b.e6)
	it.dt8Expect(tok, ent+"sort=direction", b.e1, b.e2, b.e3, b.e4, b.e6, b.e5)
	it.dt8Expect(tok, ent+"sort=-category", b.e6, b.e5, b.e3, b.e1, b.e4, b.e2)
	it.dt8Expect(tok, ent+"direction=income,expense", b.e6, b.e5)
	it.dt8Expect(tok, ent+"category=adjustment,rent", b.e6, b.e4, b.e2)
	it.dt8Expect(tok, ent+"source_type=manual,order&amount_min=100&amount_max=200", b.e5, b.e4, b.e2)
	it.dt8Expect(tok, ent+"cari_uuid="+b.cariD2, b.e4, b.e2)
	it.dt8Expect(tok, ent+"account_uuid="+b.cashA, b.e5)
	it.dt8Expect(tok, ent+"q=kira", b.e6)
	it.dt8Expect(tok, ent+"q=%25", b.e4)
	it.dt8Expect(tok, ent+"q=tal-a", b.e1)           // cari counterparty name
	it.dt8Expect(tok, ent+"q=kasa+a", b.e5)          // account name
	it.dt8Expect(tok, ent+"q=tal-c-cust+test", b.e3) // customer name
	today := time.Now().UTC().Format(time.DateOnly)
	it.dt8Expect(tok, ent+"created_from="+today+"&direction=expense", b.e6)
	it.dt8Expect(tok, ent+"date_to="+today+"&direction=expense", b.e6)
	it.dt8Expect(tok, ent+"created_from="+time.Now().UTC().AddDate(0, 0, 1).Format(time.DateOnly))
	for _, bad := range []string{
		"direction=refund", "category=Nope", "source_type=BAD", "amount_min=x", "sort=description",
		"date_from=" + today + "&created_from=" + today, "date_from=2026-13-01",
	} {
		it.dt8Status(tok, ent+bad, http.StatusBadRequest)
	}
	// The center reads the book below it; a dealer cannot.
	it.dt8Status(b.dealerTok, ent+"organization_uuid="+b.dist.Uuid.String(), http.StatusNotFound)

	// Export: list filters + q + sort; bad values are 400 at request time.
	for name, body := range map[string]map[string]any{
		"format": {"format": "docx"},
		"sort":   {"format": "csv", "query": map[string]string{"sort": "description"}},
		"enum":   {"format": "csv", "query": map[string]string{"direction": "refund"}},
		"org":    {"format": "csv", "query": map[string]string{"organization_uuid": "x"}},
	} {
		if code, env := it.do("POST", "/v1/accounting/entries/export", hostOlex, tok, body); code != http.StatusBadRequest {
			t.Fatalf("export %s = %d %s", name, code, errCode(env))
		}
	}
	if code, _ := it.do("POST", "/v1/accounting/entries/export", hostOlex, b.dealerTok, map[string]any{
		"format": "csv", "query": map[string]string{"organization_uuid": b.dist.Uuid.String()},
	}); code != http.StatusNotFound {
		t.Fatalf("dealer export of the distributor book = %d, want 404", code)
	}
	job := decodeData[exportJob](t, it.accDo("POST", "/v1/accounting/entries/export", tok, map[string]any{
		"format": "csv", "locale": "en",
		"query": map[string]string{"direction": "charge", "sort": "-amount", "limit": "1", "unknown": "x"},
	}, http.StatusAccepted))
	if job.Resource != accountingusecase.ResourceEntries {
		t.Fatalf("job = %+v", job)
	}
	stored := it.dt5JobQuery(job.UUID)
	if stored[accountingusecase.QueryBookOrganizationID] != strconv.FormatInt(b.dist.ID, 10) ||
		stored["sort"] != "-amount" || stored["direction"] != "charge" || stored["limit"] != "" || stored["unknown"] != "" {
		t.Fatalf("stored query = %v", stored)
	}
	it.accDo("GET", "/v1/accounting/exports/"+job.UUID, tok, nil, http.StatusOK)
	stored[ioengine.QueryOrganizationID] = strconv.FormatInt(b.dist.ID, 10) // set by the worker
	adapter := accountingusecase.NewEntriesAdapter(accountingusecase.New(it.pool, it.q, nil, nil))
	ds, err := adapter.Export(context.Background(), stored, i18n.Locale("en"))
	if err != nil {
		t.Fatal(err)
	}
	var amounts []string
	for _, r := range ds.Rows {
		amounts = append(amounts, r["amount"].(string))
	}
	if strings.Join(amounts, ",") != "1000.00,200.00,100.00,50.00" || ds.Rows[0]["direction"] != "Charge" ||
		ds.Rows[0]["counterparty"] != b.d1.Name {
		t.Fatalf("export rows = %v", ds.Rows)
	}
	// The worker re-checks the book: a dealer job cannot export its parent.
	stored[ioengine.QueryOrganizationID] = strconv.FormatInt(b.d1.ID, 10)
	if _, err := adapter.Export(context.Background(), stored, i18n.Locale("en")); err == nil {
		t.Fatal("a dealer job exported the distributor book")
	}
}

// Disputes: sort fields both ways, CSV status / organization filters,
// created range, q, scope. A dispute names a row on the disputing
// organization's cari with its parent, so two fresh distributors charge
// the center and dispute those rows.
func TestIntegrationTaskAccountingListsDisputes(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	distA := it.org("dt8d-alpha", "distributor", center)
	distB := it.org("dt8d-beta", "distributor", center)
	dealer := it.org("dt8d-dealer", "dealer", distA)
	ownerA, pwA := it.user("dt8d-a")
	it.member(distA, ownerA, "owner")
	ownerB, pwB := it.user("dt8d-b")
	it.member(distB, ownerB, "owner")
	dealerOwner, dpw := it.user("dt8d-dealer")
	it.member(dealer, dealerOwner, "owner")
	accUser, apw := it.user("dt8d-acc")
	it.member(center, accUser, "staff", rbac.RoleCenterAccounting)
	tokA := it.loginOrg(ownerA, pwA, distA)
	tokB := it.loginOrg(ownerB, pwB, distB)
	dealerTok := it.loginOrg(dealerOwner, dpw, dealer)
	tok := it.loginOrg(accUser, apw, center)

	ctx := context.Background()
	poster := posting.New(it.q, nil, nil)
	// sale posts a sourced center → distributor sale; the dispute names
	// the buyer row (on the distributor's cari with the center).
	sale := func(buyer db.Organization, amount string) db.FinanceEntry {
		tx, err := it.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		res, err := poster.PostHierarchicalSaleTx(ctx, tx, posting.Sale{
			Source:      posting.Source{Type: "order", UUID: uuid.New()},
			SellerOrgID: center.ID, BuyerOrgID: buyer.ID, Amount: amount, Currency: buyer.Currency,
			RateDate: time.Now().UTC(), Description: "TEC-379",
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("sale: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return res.Buyer.Entry
	}
	eA1, eA2, eB := sale(distA, "1000.00"), sale(distA, "50.00"), sale(distB, "200.00")

	now := time.Now().UTC()
	insert := func(org db.Organization, e db.FinanceEntry, status, reason string, age time.Duration) string {
		var id uuid.UUID
		if err := it.pool.QueryRow(ctx, `INSERT INTO accounting_disputes
			(organization_id, brand_id, counterparty_org_id, entry_id, source_type, source_uuid, reason, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING uuid`,
			org.ID, org.BrandID, center.ID, e.ID, e.SourceType.String, e.SourceUuid, reason+" "+it.suffix,
			now.Add(-age)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if status == "rejected" {
			if _, err := it.pool.Exec(ctx, `UPDATE accounting_disputes
				SET status = 'rejected', resolution_note = 'Red', resolved_at = NOW() WHERE uuid = $1`, id); err != nil {
				t.Fatal(err)
			}
		}
		return id.String()
	}
	dp1 := insert(distA, eA1, "open", "Fatura hatalı A", 3*time.Hour) // 1000
	dp2 := insert(distB, eB, "rejected", "Tutar yanlış", 2*time.Hour) // 200
	dp3 := insert(distA, eA2, "open", "Eksik ürün", time.Hour)        // 50
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM accounting_disputes WHERE uuid = ANY($1::uuid[])", []string{dp1, dp2, dp3})
	})

	// The center (brand scope) sees every dispute of the brand; the
	// organization filter keeps this run's rows.
	base := "/v1/accounting/disputes?organization_uuid=" + distA.Uuid.String() + "," + distB.Uuid.String() + "&"
	it.dt8Expect(tok, base, dp3, dp2, dp1)
	it.dt8Expect(tok, base+"sort=created_at", dp1, dp2, dp3)
	it.dt8Expect(tok, base+"sort=status", dp1, dp3, dp2)
	it.dt8Expect(tok, base+"sort=-status", dp2, dp3, dp1)
	it.dt8Expect(tok, base+"sort=amount", dp3, dp2, dp1)
	it.dt8Expect(tok, base+"sort=-amount", dp1, dp2, dp3)
	it.dt8Expect(tok, base+"sort=organization", dp1, dp3, dp2)
	it.dt8Expect(tok, base+"sort=-organization", dp2, dp3, dp1)
	// Open disputes (no resolved_at) stay last both ways.
	it.dt8Expect(tok, base+"sort=resolved_at", dp2, dp1, dp3)
	it.dt8Expect(tok, base+"sort=-resolved_at", dp2, dp3, dp1)
	it.dt8Expect(tok, base+"status=open", dp3, dp1)
	it.dt8Expect(tok, base+"status=open,rejected", dp3, dp2, dp1)
	it.dt8Expect(tok, "/v1/accounting/disputes?organization_uuid="+distB.Uuid.String(), dp2)
	it.dt8Expect(tok, base+"counterparty_organization_uuid="+center.Uuid.String(), dp3, dp2, dp1)
	it.dt8Expect(tok, base+"counterparty_organization_uuid="+distA.Uuid.String())
	it.dt8Expect(tok, base+"q=hatal%C4%B1+a", dp1)
	it.dt8Expect(tok, base+"q=dt8d-beta", dp2)
	it.dt8Expect(tok, base+"q=%25")
	from := url.QueryEscape(now.Add(-150 * time.Minute).Format(time.RFC3339))
	it.dt8Expect(tok, base+"created_from="+from, dp3, dp2)
	it.dt8Expect(tok, base+"created_to="+from, dp1)
	if _, _, total := it.dt8List(tok, base+"limit=1"); total != 3 {
		t.Fatalf("dispute total = %d", total)
	}
	for _, bad := range []string{"status=closed", "organization_uuid=x", "sort=reason", "created_from=x", "counterparty_organization_uuid=x"} {
		it.dt8Status(tok, "/v1/accounting/disputes?"+bad, http.StatusBadRequest)
	}
	// Scope is unchanged: each distributor sees its own, a dealer none.
	it.dt8Expect(tokA, "/v1/accounting/disputes?sort=amount", dp3, dp1)
	it.dt8Expect(tokB, "/v1/accounting/disputes", dp2)
	it.dt8Expect(dealerTok, "/v1/accounting/disputes")
}
