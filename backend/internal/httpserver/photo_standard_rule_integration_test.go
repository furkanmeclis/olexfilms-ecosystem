package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// TEC-499 (F5-07b): with photo_standard on, a missing required angle blocks
// leaving draft and the intake contract (422 PHOTO_STANDARD_INCOMPLETE with
// data.missing_angles); the last upload writes
// service.intake_photos_completed once; EXIF location reaches only the
// dealer owner and the center; KVKK anonymization clears it.
func TestIntegrationPhotoStandardRule(t *testing.T) {
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	ctx := context.Background()
	exifJPEG, err := os.ReadFile("../modules/photostandard/handler/testdata/exif.jpg")
	if err != nil {
		t.Fatal(err)
	}
	angleKey := "t499-" + it.suffix[len(it.suffix)-6:]
	center := it.brandCenter("olex")
	dealer := it.org("t499-dealer", "dealer", center)
	centerUser, centerPW := it.user("t499-center")
	it.member(center, centerUser, "staff", rbac.RoleCenterStaff)
	ownerUser, ownerPW := it.user("t499-owner")
	it.member(dealer, ownerUser, "owner")
	staffUser, staffPW := it.user("t499-staff")
	it.member(dealer, staffUser, "staff")
	centerTok := it.loginOrg(centerUser, centerPW, center)
	ownerTok := it.loginOrg(ownerUser, ownerPW, dealer)
	staffTok := it.loginOrg(staffUser, staffPW, dealer)

	admin, _ := it.user("t499-admin", rbac.RoleSuperAdmin)
	orgs := []int64{center.ID, dealer.ID}
	for _, org := range orgs {
		for _, key := range []string{features.ModulePhotoStandard, features.ModuleIntakeContracts} {
			if _, err := it.srv.features.SetByAdmin(ctx, admin.ID, org, key, true); err != nil {
				t.Fatalf("enable %s: %v", key, err)
			}
		}
	}
	t.Cleanup(func() {
		for _, org := range orgs {
			_, _ = it.srv.features.ClearByAdmin(context.Background(), org, features.ModulePhotoStandard)
			_, _ = it.srv.features.ClearByAdmin(context.Background(), org, features.ModuleIntakeContracts)
		}
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM intake_photos WHERE organization_id = $1`, dealer.ID)
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM photo_angles WHERE key = $1`, angleKey)
	})
	if code, env := it.do("POST", "/v1/platform/photo-standard/angles", hostOlex, centerTok, map[string]any{
		"key": angleKey, "name": map[string]string{"tr": "Ön", "en": "Front"}, "required": true, "sort_order": -500,
	}); code != http.StatusCreated {
		t.Fatalf("create angle = %d %s", code, errCode(env))
	}

	cust, veh := it.svcCustomer(dealer, "t499-cust", "34T499"+it.suffix[len(it.suffix)-4:])
	svc, _ := it.svcCall("POST", "/v1/services", ownerTok, map[string]any{
		"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String(), "km": 1000, "package": "PPF",
	}, http.StatusCreated)

	missingOf := func(code int, env envelope, what string) []string {
		t.Helper()
		if code != http.StatusUnprocessableEntity || errCode(env) != "PHOTO_STANDARD_INCOMPLETE" {
			t.Fatalf("%s = %d %s, want 422 PHOTO_STANDARD_INCOMPLETE", what, code, errCode(env))
		}
		var data struct {
			MissingAngles []string `json:"missing_angles"`
		}
		if err := json.Unmarshal(env.Data, &data); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, k := range data.MissingAngles {
			found = found || k == angleKey
		}
		if !found {
			t.Fatalf("%s missing_angles = %v, want %s", what, data.MissingAngles, angleKey)
		}
		return data.MissingAngles
	}

	// Leaving draft and creating the intake contract are refused.
	code, env := it.do("POST", "/v1/services/"+svc.UUID+"/transitions", hostOlex, ownerTok, map[string]any{"status": "pending"})
	missing := missingOf(code, env, "draft -> pending")
	code, env = it.do("POST", "/v1/services/"+svc.UUID+"/contract", hostOlex, ownerTok, map[string]any{})
	missingOf(code, env, "create contract")

	// Photograph every missing angle (JPEG with EXIF GPS).
	for _, key := range missing {
		if rec := it.postIntakePhoto(svc.UUID, key, ownerTok, "image/jpeg", exifJPEG); rec.Code != http.StatusCreated {
			t.Fatalf("upload %s = %d %s", key, rec.Code, rec.Body.String())
		}
	}
	// A replacement upload does not repeat the event.
	if rec := it.postIntakePhoto(svc.UUID, angleKey, ownerTok, "image/jpeg", exifJPEG); rec.Code != http.StatusCreated {
		t.Fatalf("replace upload = %d %s", rec.Code, rec.Body.String())
	}
	var events int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = 'service.intake_photos_completed' AND payload->>'entity_uuid' = $1`, svc.UUID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("intake_photos_completed events = %d, want 1", events)
	}

	// EXIF location: dealer owner and center see it, dealer staff does not.
	exifOf := func(tok string) (lat, device bool) {
		t.Helper()
		code, env := it.do("GET", "/v1/services/"+svc.UUID+"/intake-photos", hostOlex, tok, nil)
		if code != http.StatusOK {
			t.Fatalf("intake = %d %s", code, errCode(env))
		}
		body := string(env.Data)
		return strings.Contains(body, `"exif_lat"`), strings.Contains(body, `"exif_device"`)
	}
	if lat, _ := exifOf(ownerTok); !lat {
		t.Fatal("dealer owner should see exif_lat")
	}
	if lat, _ := exifOf(centerTok); !lat {
		t.Fatal("center should see exif_lat")
	}
	if lat, device := exifOf(staffTok); lat || device {
		t.Fatal("dealer staff must not see EXIF location / device")
	}

	// All angles photographed: the service leaves draft.
	it.svcCall("POST", "/v1/services/"+svc.UUID+"/transitions", ownerTok, map[string]any{"status": "pending"}, http.StatusOK)

	// KVKK anonymization clears EXIF location and device.
	it.stepUp(centerUser.Uuid)
	if code, env := it.do("POST", "/v1/customers/"+cust.Uuid.String()+"/anonymize", hostOlex, centerTok, nil); code != http.StatusOK {
		t.Fatalf("anonymize = %d %s", code, errCode(env))
	}
	var withLocation int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM intake_photos ip JOIN services s ON s.id = ip.service_id
		WHERE s.uuid = $1 AND (ip.exif_lat IS NOT NULL OR ip.exif_lng IS NOT NULL OR ip.exif_device IS NOT NULL)`, svc.UUID).Scan(&withLocation); err != nil {
		t.Fatal(err)
	}
	if withLocation != 0 {
		t.Fatalf("intake photos with EXIF location after anonymization = %d", withLocation)
	}
}
