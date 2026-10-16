package httpserver

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// The record-scoped route enforces warehouse ownership; tenant import jobs
// then preview and apply a distributor's location tree.
func TestIntegrationWarehouseLocationImport(t *testing.T) {
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = storage.NewMemory() })
	center := it.brandCenter("olex")
	dist := it.org("t762-dist", "distributor", center)
	other := it.org("t762-other", "distributor", center)
	dealer := it.org("t762-dealer", "dealer", dist)
	owner, pw := it.user("t762-owner")
	it.member(dist, owner, "owner")
	otherOwner, opw := it.user("t762-other-owner")
	it.member(other, otherOwner, "owner")
	dealerOwner, dpw := it.user("t762-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	token := it.loginOrg(owner, pw, dist)
	otherToken := it.loginOrg(otherOwner, opw, other)
	dealerToken := it.loginOrg(dealerOwner, dpw, dealer)
	var warehouse whItem
	it.whDo(token, "POST", "/v1/warehouse/warehouses", map[string]any{"code": "W" + it.suffix[len(it.suffix)-8:], "name": "Import"}, http.StatusCreated, &warehouse)
	base := "/v1/warehouse/warehouses/" + warehouse.UUID + "/locations/import"
	csv := "room_code,room_name,aisle_code,shelf_code,bin_code,name,active\nO1,Main,A1,R1,G1,Bin one,evet\n"
	upload := func(auth string) (int, stockImportJob) {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		file, err := form.CreateFormFile("file", "locations.csv")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.Write([]byte(csv))
		_ = form.WriteField("format", "csv")
		_ = form.WriteField("locale", "en")
		_ = form.Close()
		rec := it.raw("POST", base, auth, form.FormDataContentType(), body.Bytes(), nil)
		var env envelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		var job stockImportJob
		if rec.Code < 300 {
			if err = json.Unmarshal(env.Data, &job); err != nil {
				t.Fatal(err)
			}
		}
		return rec.Code, job
	}
	if code, _ := upload(dealerToken); code != http.StatusForbidden {
		t.Fatalf("dealer upload: %d", code)
	}
	if code, _ := upload(otherToken); code != http.StatusNotFound {
		t.Fatalf("other organization upload: %d", code)
	}
	if code := it.raw("GET", base+"/sample?format=csv", token, "", nil, nil).Code; code != http.StatusOK {
		t.Fatalf("sample: %d", code)
	}
	code, job := upload(token)
	if code != http.StatusCreated || job.UUID == "" {
		t.Fatalf("upload: %d %+v", code, job)
	}
	jobBase := "/v1/tenant/imports/" + job.UUID
	mapping := map[string]string{"room_code": "room_code", "room_name": "room_name", "aisle_code": "aisle_code", "shelf_code": "shelf_code", "bin_code": "bin_code", "name": "name", "active": "active"}
	it.importJob("PATCH", jobBase+"/mapping", token, map[string]any{"mapping": mapping, "defaults": map[string]string{"warehouse_uuid": warehouse.UUID}}, http.StatusOK)
	preview := it.importJob("POST", jobBase+"/preview", token, nil, http.StatusOK)
	if preview.PreviewSummary.Counts["new"] != 1 || preview.PreviewSummary.Counts["bins"] != 1 {
		t.Fatalf("preview: %+v", preview.PreviewSummary.Counts)
	}
	if target := preview.rowTarget(1)["full_code"]; !strings.HasSuffix(target.(string), "-O1-A1-R1-G1") {
		t.Fatalf("target: %v", target)
	}
	applied := it.importJob("POST", jobBase+"/confirm", token, nil, http.StatusAccepted)
	if applied.Status != "applied" || applied.PreviewSummary.Counts["applied"] != 1 {
		t.Fatalf("confirm: %+v", applied)
	}
	code, again := upload(token)
	if code != http.StatusCreated {
		t.Fatalf("reupload: %d", code)
	}
	againBase := "/v1/tenant/imports/" + again.UUID
	it.importJob("PATCH", againBase+"/mapping", token, map[string]any{"mapping": mapping, "defaults": map[string]string{"warehouse_uuid": warehouse.UUID}}, http.StatusOK)
	preview = it.importJob("POST", againBase+"/preview", token, nil, http.StatusOK)
	if preview.PreviewSummary.Counts["exists"] != 1 {
		t.Fatalf("reupload preview: %+v", preview.PreviewSummary.Counts)
	}
}
