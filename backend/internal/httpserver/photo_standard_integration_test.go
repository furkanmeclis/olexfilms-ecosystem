package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

type photoAngleResp struct {
	UUID string `json:"uuid"`
	Key  string `json:"key"`
}

type intakeResp struct {
	ServiceUUID string   `json:"service_uuid"`
	Missing     []string `json:"missing"`
	Angles      []struct {
		Required bool `json:"required"`
		Missing  bool `json:"missing"`
		Photo    *struct {
			UUID   string `json:"uuid"`
			Mime   string `json:"mime"`
			Width  *int32 `json:"width"`
			Height *int32 `json:"height"`
		} `json:"photo"`
		Angle struct {
			Key      string `json:"key"`
			Required bool   `json:"required"`
		} `json:"angle"`
	} `json:"angles"`
}

func (it *itest) postIntakePhoto(serviceUUID, angleKey, bearer, declared string, data []byte) *httptest.ResponseRecorder {
	it.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="image"; filename="img"`)
	h.Set("Content-Type", declared)
	part, err := mw.CreatePart(h)
	if err != nil {
		it.t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = mw.Close()
	return it.raw("POST", "/v1/services/"+serviceUUID+"/intake-photos/"+angleKey, bearer, mw.FormDataContentType(), buf.Bytes(), nil)
}

func TestIntegrationPhotoStandardIntake(t *testing.T) {
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	ctx := context.Background()
	angleKey := "front_" + it.suffix[len(it.suffix)-6:]
	center := it.brandCenter("olex")
	dist := it.org("ps-dist", "distributor", center)
	dealer := it.org("ps-dealer", "dealer", dist)
	centerUser, centerPW := it.user("ps-center")
	it.member(center, centerUser, "staff", rbac.RoleCenterStaff)
	distUser, distPW := it.user("ps-dist")
	it.member(dist, distUser, "owner")
	dealerUser, dealerPW := it.user("ps-dealer")
	it.member(dealer, dealerUser, "owner")
	centerTok := it.loginOrg(centerUser, centerPW, center)
	distTok := it.loginOrg(distUser, distPW, dist)
	dealerTok := it.loginOrg(dealerUser, dealerPW, dealer)
	cust, veh := it.svcCustomer(dealer, "ps-cust", "34PS"+it.suffix[len(it.suffix)-6:])
	body := map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String(), "km": 1200, "package": "PPF"}
	svc, _ := it.svcCall("POST", "/v1/services", dealerTok, body, http.StatusCreated)

	if code, env := it.do("GET", "/v1/services/"+svc.UUID+"/intake-photos", hostOlex, dealerTok, nil); code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
		t.Fatalf("module off = %d %s, want 403 FEATURE_DISABLED", code, errCode(env))
	}

	admin, _ := it.user("ps-admin", rbac.RoleSuperAdmin)
	for _, org := range []int64{center.ID, dist.ID, dealer.ID} {
		if _, err := it.srv.features.SetByAdmin(ctx, admin.ID, org, features.ModulePhotoStandard, true); err != nil {
			t.Fatalf("enable feature: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, org := range []int64{center.ID, dist.ID, dealer.ID} {
			_, _ = it.srv.features.ClearByAdmin(context.Background(), org, features.ModulePhotoStandard)
		}
		// TEC-499: the angle is brand-wide; a leftover one breaks reruns.
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM intake_photos WHERE organization_id = $1`, dealer.ID)
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM photo_angles WHERE key = $1`, angleKey)
	})

	code, env := it.do("POST", "/v1/platform/photo-standard/angles", hostOlex, centerTok, map[string]any{
		"key": angleKey, "name": map[string]string{"tr": "Ön", "en": "Front"},
		"hint":     map[string]string{"tr": "Önden çek", "en": "Shoot from the front"},
		"required": true, "sort_order": 10,
	})
	if code != http.StatusCreated {
		t.Fatalf("create angle = %d %s", code, errCode(env))
	}
	var angle photoAngleResp
	_ = json.Unmarshal(env.Data, &angle)
	if angle.Key != angleKey {
		t.Fatalf("angle = %+v", angle)
	}

	code, env = it.do("PUT", "/v1/photo-standard/overrides", hostOlex, distTok, map[string]any{
		"organization_uuid": dist.Uuid.String(),
		"overrides":         []map[string]any{{"angle_key": angleKey, "required": false, "hidden": false}},
	})
	if code != http.StatusOK {
		t.Fatalf("dist override = %d %s", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/services/"+svc.UUID+"/intake-photos", hostOlex, dealerTok, nil)
	if code != http.StatusOK {
		t.Fatalf("intake = %d %s", code, errCode(env))
	}
	var before intakeResp
	_ = json.Unmarshal(env.Data, &before)
	if len(before.Missing) != 0 || len(before.Angles) != 1 || before.Angles[0].Required {
		t.Fatalf("dist override did not make angle optional: %+v", before)
	}

	code, env = it.do("PUT", "/v1/photo-standard/overrides", hostOlex, distTok, map[string]any{
		"organization_uuid": dealer.Uuid.String(),
		"overrides":         []map[string]any{{"angle_key": angleKey, "required": true, "hidden": false}},
	})
	if code != http.StatusOK {
		t.Fatalf("dealer override = %d %s", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/services/"+svc.UUID+"/intake-photos", hostOlex, dealerTok, nil)
	if code != http.StatusOK {
		t.Fatalf("intake dealer override = %d %s", code, errCode(env))
	}
	before = intakeResp{}
	_ = json.Unmarshal(env.Data, &before)
	if len(before.Missing) != 1 || before.Missing[0] != angleKey || !before.Angles[0].Required {
		t.Fatalf("dealer override did not win: %+v", before)
	}

	if rec := it.postIntakePhoto(svc.UUID, angleKey, dealerTok, "application/pdf", []byte("%PDF-1.7")); rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("pdf upload = %d %s", rec.Code, rec.Body.String())
	}
	if rec := it.postIntakePhoto(svc.UUID, angleKey, dealerTok, "image/png", bytes.Repeat([]byte{0}, (12<<20)+1)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large upload = %d %s", rec.Code, rec.Body.String())
	}

	rec := it.postIntakePhoto(svc.UUID, angleKey, dealerTok, "image/png", pngBytes)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first upload = %d %s", rec.Code, rec.Body.String())
	}
	rec = it.postIntakePhoto(svc.UUID, angleKey, dealerTok, "image/png", pngBytes)
	if rec.Code != http.StatusCreated {
		t.Fatalf("second upload = %d %s", rec.Code, rec.Body.String())
	}
	var active, deleted int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE deleted_at IS NULL), COUNT(*) FILTER (WHERE deleted_at IS NOT NULL) FROM intake_photos WHERE service_id = (SELECT id FROM services WHERE uuid = $1)`, svc.UUID).Scan(&active, &deleted); err != nil {
		t.Fatal(err)
	}
	if active != 1 || deleted != 1 {
		t.Fatalf("photo rows active=%d deleted=%d", active, deleted)
	}

	code, env = it.do("GET", "/v1/services/"+svc.UUID+"/intake-photos", hostOlex, dealerTok, nil)
	if code != http.StatusOK {
		t.Fatalf("intake after = %d %s", code, errCode(env))
	}
	var after intakeResp
	_ = json.Unmarshal(env.Data, &after)
	if len(after.Missing) != 0 || len(after.Angles) != 1 || after.Angles[0].Photo == nil || after.Angles[0].Photo.Mime != "image/png" {
		t.Fatalf("after upload = %+v", after)
	}

	if _, err := it.pool.Exec(ctx, `UPDATE services SET status = 'completed', completed_at = NOW() WHERE uuid = $1`, svc.UUID); err != nil {
		t.Fatalf("complete service fixture: %v", err)
	}
	rec = it.raw("DELETE", "/v1/services/"+svc.UUID+"/intake-photos/"+angleKey, dealerTok, "", nil, nil)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "PHOTO_SERVICE_LOCKED") {
		t.Fatalf("delete completed = %d %s", rec.Code, rec.Body.String())
	}

	if code, env := it.do("GET", "/v1/platform/photo-standard/angles", hostOlex, centerTok, nil); code != http.StatusOK || !strings.Contains(string(env.Data), angleKey) {
		t.Fatalf("admin list angles = %d %s", code, errCode(env))
	}
}
