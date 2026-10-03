package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	bulkhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/handler"
	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
)

type bulkSyncView struct {
	Sync      bool `json:"sync"`
	Summary   struct{ Total, Succeeded, Failed int }
	Operation *bulkusecase.OperationView `json:"operation"`
}

func (it *itest) bulkRun(path, token string, body map[string]any) bulkSyncView {
	it.t.Helper()
	code, env := it.do("POST", path, hostOlex, token, body)
	if code != http.StatusOK {
		it.t.Fatalf("bulk %s: %d %s", path, code, errCode(env))
	}
	var v bulkSyncView
	if err := json.Unmarshal(env.Data, &v); err != nil {
		it.t.Fatal(err)
	}
	if !v.Sync || v.Operation == nil {
		it.t.Fatalf("bulk %s: not a logged sync run: %s", path, env.Data)
	}
	return v
}

func (it *itest) bulkUndo(path, host, token string, want int) (bulkusecase.OperationView, string) {
	it.t.Helper()
	code, env := it.do("POST", path, host, token, nil)
	if code != want {
		it.t.Fatalf("undo %s: %d %s, want %d", path, code, errCode(env), want)
	}
	var v bulkusecase.OperationView
	_ = json.Unmarshal(env.Data, &v)
	return v, errCode(env)
}

func (it *itest) productActive(uuid string) bool {
	it.t.Helper()
	var active bool
	if err := it.pool.QueryRow(context.Background(), "SELECT active FROM products WHERE uuid = $1", uuid).Scan(&active); err != nil {
		it.t.Fatalf("product %s: %v", uuid, err)
	}
	return active
}

func (it *itest) taskAssignee(uuid string) *string {
	it.t.Helper()
	var assignee *string
	if err := it.pool.QueryRow(context.Background(),
		"SELECT u.uuid::text FROM tasks t LEFT JOIN users u ON u.id = t.assignee_user_id WHERE t.uuid = $1", uuid).Scan(&assignee); err != nil {
		it.t.Fatalf("task %s: %v", uuid, err)
	}
	return assignee
}

// TEC-212 acceptance: a bulk activate/deactivate, assignment and platform
// enable/disable run is logged and undo restores the previous values; a
// record changed after the run is skipped (conflict), the second undo and
// an expired undo are refused with 409, and another organization cannot
// see or undo the operation.
func TestIntegrationBulkUndo(t *testing.T) {
	it := newIntegration(t)
	it.cleanupCatalog()
	ctx := context.Background()
	olexCenter := it.brandCenter("olex")
	glorianCenter := it.brandCenter("glorian")
	dist := it.org("undo-dist", "distributor", olexCenter)
	dealer := it.org("undo-dealer", "dealer", dist)

	staff, staffPW := it.user("undo-staff")
	it.member(olexCenter, staff, "staff")
	acc, accPW := it.user("undo-acc")
	it.member(olexCenter, acc, "staff")
	glorianUser, glorianPW := it.user("undo-glorian")
	it.member(glorianCenter, glorianUser, "staff")
	dealerUser, dealerPW := it.user("undo-dealerown")
	it.member(dealer, dealerUser, "owner")
	t.Cleanup(func() {
		_, _ = it.pool.Exec(ctx, "DELETE FROM tasks WHERE subject_org_id = ANY($1)", []int64{dist.ID, dealer.ID})
	})

	staffTok := it.catalogLogin(staff, staffPW, olexCenter, hostOlex)
	accTok := it.catalogLogin(acc, accPW, olexCenter, hostOlex)
	glorianTok := it.catalogLogin(glorianUser, glorianPW, glorianCenter, hostGlorian)
	dealerTok := it.catalogLogin(dealerUser, dealerPW, dealer, hostOlex)

	// --- catalog products: deactivate, conflict on one, undo ------------
	p1 := it.createCatalogProduct(staffTok, hostOlex, "UNDO1-"+it.suffix)
	p2 := it.createCatalogProduct(staffTok, hostOlex, "UNDO2-"+it.suffix)
	run := it.bulkRun("/v1/catalog/products/bulk", staffTok, map[string]any{
		"action": "deactivate",
		"target": map[string]any{"scope": "ids", "ids": []string{p1.UUID, p2.UUID}},
	})
	if run.Summary.Succeeded != 2 || run.Operation.UndoStatus != bulkusecase.UndoAvailable || run.Operation.UndoUntil == nil {
		t.Fatalf("deactivate run = %+v", run)
	}
	if it.productActive(p1.UUID) || it.productActive(p2.UUID) {
		t.Fatal("products still active after bulk deactivate")
	}
	opPath := "/v1/tenant/bulk-operations/" + run.Operation.UUID.String() + "/undo"

	// Another organization (other brand's center, a dealer of this brand)
	// does not see the operation.
	it.bulkUndo(opPath, hostGlorian, glorianTok, http.StatusNotFound)
	it.bulkUndo(opPath, hostOlex, dealerTok, http.StatusNotFound)

	// p2 is changed after the run: undo must leave it alone.
	if _, err := it.pool.Exec(ctx, "UPDATE products SET active = true WHERE uuid = $1", p2.UUID); err != nil {
		t.Fatal(err)
	}
	undone, _ := it.bulkUndo(opPath, hostOlex, staffTok, http.StatusOK)
	if undone.UndoStatus != bulkusecase.UndoPartial || undone.UndoResult == nil ||
		undone.UndoResult.Restored != 1 || len(undone.UndoResult.Skipped) != 1 ||
		undone.UndoResult.Skipped[0].EntityUUID != p2.UUID || undone.UndoResult.Skipped[0].Reason != bulkusecase.SkipConflict {
		t.Fatalf("partial undo = %+v", undone)
	}
	if !it.productActive(p1.UUID) {
		t.Fatal("p1 not restored")
	}
	// Second undo: 409.
	if _, code := it.bulkUndo(opPath, hostOlex, staffTok, http.StatusConflict); code != bulkhandler.CodeBulkUndoUnavailable {
		t.Fatalf("second undo code = %s", code)
	}

	// The organization's undo log lists the operation.
	code, env := it.do("GET", "/v1/tenant/bulk-operations", hostOlex, staffTok, nil)
	if code != http.StatusOK {
		t.Fatalf("list operations: %d %s", code, errCode(env))
	}
	var page struct {
		Items []bulkusecase.OperationView `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &page)
	found := false
	for _, op := range page.Items {
		if op.UUID == run.Operation.UUID && op.UndoStatus == bulkusecase.UndoPartial {
			found = true
		}
	}
	if !found {
		t.Fatalf("operation missing from list: %s", env.Data)
	}

	// Expired window: 409 BULK_UNDO_EXPIRED, nothing changes.
	run2 := it.bulkRun("/v1/catalog/products/bulk", staffTok, map[string]any{
		"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{p1.UUID}},
	})
	if _, err := it.pool.Exec(ctx, "UPDATE bulk_operations SET undo_until = NOW() - interval '1 hour' WHERE uuid = $1", run2.Operation.UUID); err != nil {
		t.Fatal(err)
	}
	if _, code := it.bulkUndo("/v1/tenant/bulk-operations/"+run2.Operation.UUID.String()+"/undo", hostOlex, staffTok, http.StatusConflict); code != bulkhandler.CodeBulkUndoExpired {
		t.Fatalf("expired undo code = %s", code)
	}
	if it.productActive(p1.UUID) {
		t.Fatal("expired undo changed the product")
	}

	// --- tasks: assign two tasks, undo restores assignee / null --------
	t1 := decodeTask(t, it.taskDo("POST", "/v1/tasks", staffTok, map[string]any{
		"subject_organization_uuid": dealer.Uuid.String(), "title": "Undo A " + it.suffix,
		"assignee_user_uuid": acc.Uuid.String(),
	}, http.StatusCreated))
	t2 := decodeTask(t, it.taskDo("POST", "/v1/tasks", staffTok, map[string]any{
		"subject_organization_uuid": dist.Uuid.String(), "title": "Undo B " + it.suffix,
	}, http.StatusCreated))
	assign := it.bulkRun("/v1/tasks/bulk", accTok, map[string]any{
		"action": "assign",
		"target": map[string]any{
			"scope": "ids", "ids": []string{t1.UUID, t2.UUID},
			"params": map[string]string{"assignee_uuid": staff.Uuid.String()},
		},
	})
	if assign.Summary.Succeeded != 2 {
		t.Fatalf("assign run = %+v", assign)
	}
	for _, id := range []string{t1.UUID, t2.UUID} {
		if got := it.taskAssignee(id); got == nil || *got != staff.Uuid.String() {
			t.Fatalf("task %s assignee after bulk = %v", id, got)
		}
	}
	assignPath := "/v1/tenant/bulk-operations/" + assign.Operation.UUID.String() + "/undo"
	it.bulkUndo(assignPath, hostGlorian, glorianTok, http.StatusNotFound)
	undone, _ = it.bulkUndo(assignPath, hostOlex, accTok, http.StatusOK)
	if undone.UndoStatus != bulkusecase.UndoUndone || undone.UndoResult == nil || undone.UndoResult.Restored != 2 {
		t.Fatalf("assign undo = %+v", undone)
	}
	if got := it.taskAssignee(t1.UUID); got == nil || *got != acc.Uuid.String() {
		t.Fatalf("t1 assignee after undo = %v", got)
	}
	if got := it.taskAssignee(t2.UUID); got != nil {
		t.Fatalf("t2 assignee after undo = %v, want null", *got)
	}
	it.bulkUndo(assignPath, hostOlex, accTok, http.StatusConflict)
	// An assignee outside the center is refused before anything runs.
	if code, env := it.do("POST", "/v1/tasks/bulk", hostOlex, accTok, map[string]any{
		"action": "assign",
		"target": map[string]any{
			"scope": "ids", "ids": []string{t1.UUID},
			"params": map[string]string{"assignee_uuid": dealerUser.Uuid.String()},
		},
	}); code != http.StatusBadRequest {
		t.Fatalf("assign to non-member: %d %s", code, errCode(env))
	}

	// --- platform users: disable, undo (no organization) ----------------
	admin := it.adminToken()
	victim, _ := it.user("undo-victim")
	disable := it.bulkRun("/v1/platform/users/bulk", admin, map[string]any{
		"action": "disable", "target": map[string]any{"scope": "ids", "ids": []string{victim.Uuid.String()}},
	})
	var status string
	_ = it.pool.QueryRow(ctx, "SELECT status FROM users WHERE id = $1", victim.ID).Scan(&status)
	if status != "disabled" {
		t.Fatalf("user status after bulk disable = %s", status)
	}
	platformPath := "/v1/platform/bulk-operations/" + disable.Operation.UUID.String() + "/undo"
	// A platform operation is invisible on the tenant route.
	it.bulkUndo("/v1/tenant/bulk-operations/"+disable.Operation.UUID.String()+"/undo", hostOlex, staffTok, http.StatusNotFound)
	undone, _ = it.bulkUndo(platformPath, hostOlex, admin, http.StatusOK)
	if undone.UndoStatus != bulkusecase.UndoUndone || undone.UndoResult.Restored != 1 {
		t.Fatalf("platform undo = %+v", undone)
	}
	_ = it.pool.QueryRow(ctx, "SELECT status FROM users WHERE id = $1", victim.ID).Scan(&status)
	if status != "active" {
		t.Fatalf("user status after undo = %s", status)
	}
	it.bulkUndo(platformPath, hostOlex, admin, http.StatusConflict)
}
