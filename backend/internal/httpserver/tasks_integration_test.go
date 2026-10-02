package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type taskView struct {
	UUID    string `json:"uuid"`
	Title   string `json:"title"`
	Subject struct {
		UUID string `json:"uuid"`
		Type string `json:"type"`
	} `json:"subject_organization"`
	Assignee *struct {
		UUID string `json:"uuid"`
	} `json:"assignee"`
	CreatedBy *struct {
		UUID string `json:"uuid"`
	} `json:"created_by"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	Source       string     `json:"source"`
	DueAt        *time.Time `json:"due_at"`
	ClosedAt     *time.Time `json:"closed_at"`
	CommentCount int64      `json:"comment_count"`
}

type taskPage struct {
	Items []taskView `json:"items"`
	Total int64      `json:"total"`
}

type taskCommentPage struct {
	Items []struct {
		UUID   string `json:"uuid"`
		Body   string `json:"body"`
		Author *struct {
			UUID string `json:"uuid"`
		} `json:"author"`
	} `json:"items"`
	Total int64 `json:"total"`
}

func (it *itest) taskDo(method, path, token string, body any, want int) envelope {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, token, body)
	if code != want {
		it.t.Fatalf("%s %s: %d (%s), want %d", method, path, code, errCode(env), want)
	}
	return env
}

func decodeTask(t *testing.T, env envelope) taskView {
	t.Helper()
	var v taskView
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatalf("task payload: %s", env.Data)
	}
	return v
}

// TEC-214 acceptance: the center opens, updates, closes and comments on a
// task about one of its dealers; the subject must be a distributor or dealer
// of the brand and the assignee a center member; distributor and dealer
// members get 403 on every task route; another brand's center does not see
// the task. Each step writes an outbox event and an audit row.
func TestIntegrationTasks(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	olexCenter := it.brandCenter("olex")
	glorianCenter := it.brandCenter("glorian")
	dist := it.org("task-dist", "distributor", olexCenter)
	dealer := it.org("task-dealer", "dealer", dist)
	glorianDist := it.org("task-gdist", "distributor", glorianCenter)

	staff, staffPW := it.user("task-staff")
	it.member(olexCenter, staff, "staff")
	acc, accPW := it.user("task-acc")
	it.member(olexCenter, acc, "staff", rbac.RoleCenterAccounting)
	glorianUser, glorianPW := it.user("task-glorian")
	it.member(glorianCenter, glorianUser, "staff")
	distUser, distPW := it.user("task-distown")
	it.member(dist, distUser, "owner")
	dealerUser, dealerPW := it.user("task-dealerown")
	it.member(dealer, dealerUser, "owner")

	// Registered after the orgs and users: runs before their cleanups.
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(),
			"DELETE FROM tasks WHERE subject_org_id = ANY($1)", []int64{dist.ID, dealer.ID, glorianDist.ID})
	})

	staffTok := it.catalogLogin(staff, staffPW, olexCenter, hostOlex)
	accTok := it.catalogLogin(acc, accPW, olexCenter, hostOlex)
	glorianTok := it.catalogLogin(glorianUser, glorianPW, glorianCenter, hostGlorian)
	distTok := it.catalogLogin(distUser, distPW, dist, hostOlex)
	dealerTok := it.catalogLogin(dealerUser, dealerPW, dealer, hostOlex)

	// Create: about the dealer, assigned to the center accountant.
	due := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	created := decodeTask(t, it.taskDo("POST", "/v1/tasks", staffTok, map[string]any{
		"subject_organization_uuid": dealer.Uuid.String(),
		"title":                     "Sözleşme yenileme " + it.suffix,
		"description":               "Bayi sözleşmesi bitiyor",
		"assignee_user_uuid":        acc.Uuid.String(),
		"priority":                  "high",
		"due_at":                    due.Format(time.RFC3339),
	}, http.StatusCreated))
	if created.Status != "open" || created.Source != "manual" || created.Priority != "high" ||
		created.Subject.UUID != dealer.Uuid.String() || created.Subject.Type != "dealer" ||
		created.Assignee == nil || created.Assignee.UUID != acc.Uuid.String() ||
		created.CreatedBy == nil || created.CreatedBy.UUID != staff.Uuid.String() ||
		created.DueAt == nil || !created.DueAt.Equal(due) || created.ClosedAt != nil {
		t.Fatalf("created task = %+v", created)
	}
	path := "/v1/tasks/" + created.UUID

	// A distributor is a valid subject; default priority is normal.
	distTask := decodeTask(t, it.taskDo("POST", "/v1/tasks", accTok, map[string]any{
		"subject_organization_uuid": dist.Uuid.String(), "title": "Ziyaret " + it.suffix,
	}, http.StatusCreated))
	if distTask.Priority != "normal" || distTask.Assignee != nil || distTask.Subject.Type != "distributor" {
		t.Fatalf("distributor task = %+v", distTask)
	}

	// Subject must be a distributor or dealer of the brand; assignee a
	// member of the center.
	for name, body := range map[string]map[string]any{
		"center subject":        {"subject_organization_uuid": olexCenter.Uuid.String(), "title": "x"},
		"other brand subject":   {"subject_organization_uuid": glorianDist.Uuid.String(), "title": "x"},
		"dealer member as user": {"subject_organization_uuid": dealer.Uuid.String(), "title": "x", "assignee_user_uuid": dealerUser.Uuid.String()},
		"empty title":           {"subject_organization_uuid": dealer.Uuid.String(), "title": " "},
		"bad priority":          {"subject_organization_uuid": dealer.Uuid.String(), "title": "x", "priority": "p0"},
	} {
		if code, env := it.do("POST", "/v1/tasks", hostOlex, staffTok, body); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, code, errCode(env))
		}
	}
	if code, env := it.do("PATCH", path, hostOlex, staffTok, map[string]any{
		"subject_organization_uuid": olexCenter.Uuid.String(),
	}); code != http.StatusBadRequest {
		t.Fatalf("patch center subject: %d %s", code, errCode(env))
	}

	// Update: title and start working.
	upd := decodeTask(t, it.taskDo("PATCH", path, accTok, map[string]any{
		"title": "Sözleşme yenileme (arandı) " + it.suffix, "status": "in_progress",
	}, http.StatusOK))
	if upd.Status != "in_progress" || upd.ClosedAt != nil || upd.Title != "Sözleşme yenileme (arandı) "+it.suffix {
		t.Fatalf("updated task = %+v", upd)
	}

	// Comment.
	env := it.taskDo("POST", path+"/comments", accTok, map[string]any{"body": "Bayi arandı, evrak bekleniyor"}, http.StatusCreated)
	var cm struct {
		UUID   string `json:"uuid"`
		Author *struct {
			UUID string `json:"uuid"`
		} `json:"author"`
	}
	_ = json.Unmarshal(env.Data, &cm)
	if cm.UUID == "" || cm.Author == nil || cm.Author.UUID != acc.Uuid.String() {
		t.Fatalf("comment = %s", env.Data)
	}
	if code, env := it.do("POST", path+"/comments", hostOlex, accTok, map[string]any{"body": "  "}); code != http.StatusBadRequest {
		t.Fatalf("empty comment: %d %s", code, errCode(env))
	}
	var comments taskCommentPage
	_ = json.Unmarshal(it.taskDo("GET", path+"/comments", staffTok, nil, http.StatusOK).Data, &comments)
	if comments.Total != 1 || len(comments.Items) != 1 || comments.Items[0].UUID != cm.UUID {
		t.Fatalf("comments = %+v", comments)
	}

	// Close and clear the due date.
	closed := decodeTask(t, it.taskDo("PATCH", path, accTok, map[string]any{"status": "done", "due_at": nil}, http.StatusOK))
	if closed.Status != "done" || closed.ClosedAt == nil || closed.DueAt != nil || closed.CommentCount != 1 {
		t.Fatalf("closed task = %+v", closed)
	}
	if code, env := it.do("PATCH", path, hostOlex, accTok, map[string]any{"status": "closed"}); code != http.StatusBadRequest {
		t.Fatalf("unknown status: %d %s", code, errCode(env))
	}

	// Lists: by status, by subject and the caller's own tasks.
	var page taskPage
	_ = json.Unmarshal(it.taskDo("GET", "/v1/tasks?status=done&subject_organization_uuid="+dealer.Uuid.String(), staffTok, nil, http.StatusOK).Data, &page)
	if page.Total != 1 || page.Items[0].UUID != created.UUID {
		t.Fatalf("done list = %+v", page)
	}
	_ = json.Unmarshal(it.taskDo("GET", "/v1/tasks?status=active&subject_organization_uuid="+dist.Uuid.String(), staffTok, nil, http.StatusOK).Data, &page)
	if page.Total != 1 || page.Items[0].UUID != distTask.UUID {
		t.Fatalf("active list = %+v", page)
	}
	_ = json.Unmarshal(it.taskDo("GET", "/v1/tasks?mine=true&subject_organization_uuid="+dealer.Uuid.String(), accTok, nil, http.StatusOK).Data, &page)
	if page.Total != 1 || page.Items[0].UUID != created.UUID {
		t.Fatalf("mine list = %+v", page)
	}

	// Distributor and dealer members cannot reach any task route.
	for name, tok := range map[string]string{"distributor": distTok, "dealer": dealerTok} {
		for _, rt := range []struct {
			method, path string
			body         any
		}{
			{"GET", "/v1/tasks", nil},
			{"POST", "/v1/tasks", map[string]any{"subject_organization_uuid": dealer.Uuid.String(), "title": "x"}},
			{"GET", path, nil},
			{"PATCH", path, map[string]any{"status": "open"}},
			{"GET", path + "/comments", nil},
			{"POST", path + "/comments", map[string]any{"body": "x"}},
		} {
			if code, env := it.do(rt.method, rt.path, hostOlex, tok, rt.body); code != http.StatusForbidden {
				t.Fatalf("%s %s %s: %d %s", name, rt.method, rt.path, code, errCode(env))
			}
		}
	}

	// Another brand's center does not see the task (K1).
	if code, env := it.do("GET", path, hostGlorian, glorianTok, nil); code != http.StatusNotFound {
		t.Fatalf("glorian get: %d %s", code, errCode(env))
	}

	// Outbox events and audit rows.
	for _, ev := range []string{"tasks.created", "tasks.assigned", "tasks.updated", "tasks.status_changed"} {
		if n := it.countRows(`SELECT COUNT(*) FROM outbox_events WHERE event_name = $1 AND payload->>'entity_uuid' = $2`,
			ev, created.UUID); n == 0 {
			t.Fatalf("no %s outbox event", ev)
		}
	}
	if n := it.countRows(`SELECT COUNT(*) FROM outbox_events WHERE event_name = 'tasks.comment_added' AND payload->>'entity_uuid' = $1`,
		cm.UUID); n != 1 {
		t.Fatalf("comment events = %d", n)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM activity_events WHERE resource = 'task' AND resource_uuid = $1::uuid`, created.UUID); n < 4 {
		t.Fatalf("task audit rows = %d", n)
	}

	// The schema guard holds on its own: a cross-brand subject is rejected.
	if _, err := it.pool.Exec(ctx, `INSERT INTO tasks (organization_id, brand_id, subject_org_id, title)
		VALUES ($1, $2, $3, 'direct')`, olexCenter.ID, olexCenter.BrandID, glorianDist.ID); err == nil {
		t.Fatal("cross-brand subject accepted by the database")
	}
}
