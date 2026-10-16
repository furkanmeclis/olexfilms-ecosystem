package usecase_test

import (
	"context"
	"encoding/json"
	"testing"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/google/uuid"
)

func TestLocationImportStagesAppliesAndUndoes(t *testing.T) {
	e := newEntryEnv(t)
	ctx := context.Background()
	warehouse, err := e.tree.CreateWarehouse(ctx, e.caller(e.dist).Caller, wh.WarehouseInput{Code: e.prefix("W"), Name: "Import warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	importer := wh.NewLocationImporter(e.pool)
	defaults := map[string]any{"warehouse_uuid": warehouse.UUID.String()}
	newJob := func() ioengine.ImportJob {
		t.Helper()
		var id, actor int64
		var u uuid.UUID
		if err := e.pool.QueryRow(ctx, `SELECT id FROM users LIMIT 1`).Scan(&actor); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(defaults)
		err := e.pool.QueryRow(ctx, `INSERT INTO import_jobs(resource,actor_id,organization_id,format,locale,status,defaults_json) VALUES($1,$2,$3,'xlsx','tr','previewed',$4) RETURNING id,uuid`, wh.LocationImportResource, actor, e.dist.ID, data).Scan(&id, &u)
		if err != nil {
			t.Fatal(err)
		}
		return ioengine.ImportJob{ID: id, UUID: u, ActorID: actor, OrganizationID: e.dist.ID, Locale: "tr"}
	}
	row := map[string]any{"room_code": "O1", "room_name": "Import room", "aisle_code": "A1", "shelf_code": "R1", "bin_code": "G1", "name": "First bin", "active": "evet"}
	job := newJob()
	preview, err := importer.Stage(ctx, job, []map[string]any{row, row, {"room_code": "O1", "bin_code": "B1"}}, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Counts["new"] != 1 || preview.Counts["invalid"] != 2 || preview.Counts["rooms"] != 1 || preview.Counts["aisles"] != 1 || preview.Counts["shelves"] != 1 || preview.Counts["bins"] != 1 {
		t.Fatalf("unexpected preview: %+v", preview.Counts)
	}
	if preview.Rows[0].Target["full_code"] != warehouse.Code+"-O1-A1-R1-G1" {
		t.Fatalf("wrong target: %+v", preview.Rows[0].Target)
	}
	if preview.Rows[0].Target["create_count"] != 4 {
		t.Fatalf("new node count: %+v", preview.Rows[0].Target)
	}
	result, err := importer.Apply(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if result.Counts["applied"] != 1 {
		t.Fatalf("apply: %+v", result.Counts)
	}
	if _, err = importer.Apply(ctx, job); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	var count int
	if err = e.pool.QueryRow(ctx, `SELECT count(*) FROM warehouse_locations WHERE full_code=$1`, warehouse.Code+"-O1-A1-R1-G1").Scan(&count); err != nil || count != 1 {
		t.Fatalf("created bins: %d, %v", count, err)
	}
	reupload := newJob()
	preview, err = importer.Stage(ctx, reupload, []map[string]any{row}, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Counts["exists"] != 1 || preview.Counts["new"] != 0 {
		t.Fatalf("reupload: %+v", preview.Counts)
	}
	if _, err = importer.Apply(ctx, reupload); err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Undo(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err = e.pool.QueryRow(ctx, `SELECT count(*) FROM warehouse_locations WHERE full_code=$1`, warehouse.Code+"-O1-A1-R1-G1").Scan(&count); err != nil || count != 0 {
		t.Fatalf("undo bins: %d, %v", count, err)
	}
}

func TestLocationImportTypeConflictAndUndoRejected(t *testing.T) {
	e := newEntryEnv(t)
	ctx := context.Background()
	w, err := e.tree.CreateWarehouse(ctx, e.caller(e.dist).Caller, wh.WarehouseInput{Code: e.prefix("X"), Name: "Conflict warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[string]any{"warehouse_uuid": w.UUID.String()}
	var actor, id int64
	var jobUUID uuid.UUID
	if err = e.pool.QueryRow(ctx, `SELECT id FROM users LIMIT 1`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(defaults)
	if err = e.pool.QueryRow(ctx, `INSERT INTO import_jobs(resource,actor_id,organization_id,format,locale,status,defaults_json) VALUES($1,$2,$3,'xlsx','tr','previewed',$4) RETURNING id,uuid`, wh.LocationImportResource, actor, e.dist.ID, data).Scan(&id, &jobUUID); err != nil {
		t.Fatal(err)
	}
	job := ioengine.ImportJob{ID: id, UUID: jobUUID, ActorID: actor, OrganizationID: e.dist.ID, Locale: "tr"}
	im := wh.NewLocationImporter(e.pool)
	rows := []map[string]any{{"room_code": "O1", "aisle_code": "A1", "shelf_code": "R1"}, {"room_code": "O1", "shelf_code": "A1"}}
	preview, err := im.Stage(ctx, job, rows, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Counts["conflict"] != 1 {
		t.Fatalf("type conflict: %+v", preview.Counts)
	}
	if _, err = im.Apply(ctx, job); err != nil {
		t.Fatal(err)
	}
	var shelfID int64
	if err = e.pool.QueryRow(ctx, `SELECT id FROM warehouse_locations WHERE full_code=$1`, w.Code+"-O1-A1-R1").Scan(&shelfID); err != nil {
		t.Fatal(err)
	}
	_, err = e.pool.Exec(ctx, `INSERT INTO warehouse_locations(organization_id,room_id,parent_id,type,code,name,active) SELECT $1,room_id,$2,'bin','EXTRA','Extra',true FROM warehouse_locations WHERE id=$2`, e.dist.ID, shelfID)
	if err != nil {
		t.Fatal(err)
	}
	undone, err := im.Undo(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if undone.Counts["undo_rejected"] != 1 {
		t.Fatalf("undo rejected: %+v", undone.Counts)
	}
}
