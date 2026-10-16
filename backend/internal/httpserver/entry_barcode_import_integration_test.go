package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

func TestIntegrationEntryBarcodeImportRoutes(t *testing.T) {
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = storage.NewMemory() })
	ctx := context.Background()
	center := it.brandCenter("olex")
	user, password := it.user("t760-wh")
	it.member(center, user, "staff", rbac.RoleCenterWarehouse)
	token := it.loginOrg(user, password, center)
	product := it.product(center, "T760P")
	var warehouseUUID string
	if err := it.pool.QueryRow(ctx, `INSERT INTO warehouses(organization_id,code,name) VALUES($1,$2,$3) RETURNING uuid::text`, center.ID, "T760"+it.suffix, "import warehouse").Scan(&warehouseUUID); err != nil {
		t.Fatal(err)
	}
	status, env := it.do("POST", "/v1/warehouse/stock-entries", hostOlex, token, map[string]any{"warehouse_uuid": warehouseUUID, "mode": "with_existing"})
	if status != http.StatusCreated {
		t.Fatalf("entry: %d %s", status, errCode(env))
	}
	var entry struct {
		UUID      string `json:"uuid"`
		LineCount int    `json:"line_count"`
	}
	if err := json.Unmarshal(env.Data, &entry); err != nil {
		t.Fatal(err)
	}
	existing := "T760-OLD-" + it.suffix
	fresh := "T760-NEW-" + it.suffix
	if _, err := it.q.CreateUnit(ctx, db.CreateUnitParams{OrganizationID: center.ID, BrandID: center.BrandID, ProductID: product.ID, Barcode: existing, UnitKind: "serial", Source: "imported", Status: "printed"}); err != nil {
		t.Fatal(err)
	}
	path := "/v1/warehouse/stock-entries/" + entry.UUID + "/import"
	status, _ = it.do("GET", path+"/sample?format=csv", hostOlex, token, nil)
	if status != http.StatusOK {
		t.Fatalf("sample = %d", status)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "barcodes.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("product_sku,barcode\n" + product.Sku + "," + existing + "\n" + product.Sku + "," + fresh + "\n"))
	_ = form.WriteField("format", "csv")
	_ = form.WriteField("locale", "en")
	_ = form.WriteField("existing_policy", "none")
	_ = form.Close()
	rec := it.raw("POST", path, token, form.FormDataContentType(), body.Bytes(), nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body.String())
	}
	var uploaded envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &uploaded)
	var job stockImportJob
	if err := json.Unmarshal(uploaded.Data, &job); err != nil {
		t.Fatal(err)
	}
	base := "/v1/tenant/imports/" + job.UUID
	it.importJob("PATCH", base+"/mapping", token, map[string]any{"mapping": map[string]string{"product_sku": "product_sku", "barcode": "barcode"}}, http.StatusOK)
	preview := it.importJob("POST", base+"/preview", token, nil, http.StatusOK)
	if preview.PreviewSummary.Counts["link"] != 1 || preview.PreviewSummary.Counts["new"] != 1 {
		t.Fatalf("preview %+v", preview)
	}
	applied := it.importJob("POST", base+"/confirm", token, nil, http.StatusAccepted)
	if applied.Status != "applied" || applied.PreviewSummary.Counts["applied"] != 2 {
		t.Fatalf("applied %+v", applied)
	}
	status, env = it.do("GET", "/v1/warehouse/stock-entries/"+entry.UUID, hostOlex, token, nil)
	if status != http.StatusOK {
		t.Fatalf("get = %d", status)
	}
	if err := json.Unmarshal(env.Data, &entry); err != nil || entry.LineCount != 2 {
		t.Fatalf("entry count %d %v", entry.LineCount, err)
	}
	rolled := it.importJob("POST", base+"/rollback", token, nil, http.StatusOK)
	if rolled.PreviewSummary.Counts["undone"] != 2 {
		t.Fatalf("undo %+v", rolled)
	}
	// Distributor warehouse staff have warehouse.write but no tenant.imports.read.
	// They may still preview a record-scoped job in their own organization.
	dist := it.org("t760-dist", "distributor", center)
	staff, staffPassword := it.user("t760-dist-wh")
	it.member(dist, staff, "staff", rbac.RoleDistributorWarehouseStaff)
	staffToken := it.loginOrg(staff, staffPassword, dist)
	var distWarehouseUUID string
	if err := it.pool.QueryRow(ctx, `INSERT INTO warehouses(organization_id,code,name) VALUES($1,$2,$3) RETURNING uuid::text`, dist.ID, "D760"+it.suffix, "dist import warehouse").Scan(&distWarehouseUUID); err != nil {
		t.Fatal(err)
	}
	status, env = it.do("POST", "/v1/warehouse/stock-entries", hostOlex, staffToken, map[string]any{"warehouse_uuid": distWarehouseUUID, "mode": "with_existing"})
	if status != http.StatusCreated {
		t.Fatalf("distributor entry = %d %s", status, errCode(env))
	}
	var distEntry struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(env.Data, &distEntry); err != nil {
		t.Fatal(err)
	}
	var distBody bytes.Buffer
	distForm := multipart.NewWriter(&distBody)
	distFile, err := distForm.CreateFormFile("file", "dist.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = distFile.Write([]byte("product_sku,barcode\n" + product.Sku + "," + existing + "\n"))
	_ = distForm.WriteField("format", "csv")
	_ = distForm.WriteField("locale", "en")
	_ = distForm.Close()
	distRec := it.raw("POST", "/v1/warehouse/stock-entries/"+distEntry.UUID+"/import", staffToken, distForm.FormDataContentType(), distBody.Bytes(), nil)
	if distRec.Code != http.StatusCreated {
		t.Fatalf("distributor upload = %d %s", distRec.Code, distRec.Body.String())
	}
	var distUpload envelope
	_ = json.Unmarshal(distRec.Body.Bytes(), &distUpload)
	var distJob stockImportJob
	if err := json.Unmarshal(distUpload.Data, &distJob); err != nil {
		t.Fatal(err)
	}
	distBase := "/v1/tenant/imports/" + distJob.UUID
	it.importJob("PATCH", distBase+"/mapping", staffToken, map[string]any{"mapping": map[string]string{"product_sku": "product_sku", "barcode": "barcode"}}, http.StatusOK)
	distPreview := it.importJob("POST", distBase+"/preview", staffToken, nil, http.StatusOK)
	if distPreview.PreviewSummary.Counts["link"] != 1 {
		t.Fatalf("distributor preview %+v", distPreview)
	}
}
