package httpserver

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
)

// TEC-339: the claim screens read the opening organization, warranty code,
// product and service on list rows, the event timeline on the detail, the
// photos through GET /v1/warranty-claims/{uuid}/photos/{photo} (read scope)
// and the warranty of each portal claim status.
func TestIntegrationWarrantyClaimScreens(t *testing.T) {
	store := storage.NewMemory()
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.Storage = store })
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t339-dist", "distributor", center)
	dealerA := it.org("t339-a", "dealer", dist)
	dealerB := it.org("t339-b", "dealer", dist)
	p := it.product(center, "T339")
	it.setWarrantyMonths(p, 12)
	tag := dt7Tag(it)
	cust, veh := it.svcCustomer(dealerA, "t339-cust", "34T"+tag[4:])
	w := it.warrantyFor(dealerA, center, p, cust, veh, 33901)
	c, err := it.q.CreateWarrantyClaim(ctx, db.CreateWarrantyClaimParams{
		OrganizationID: w.OrganizationID, BrandID: w.BrandID, WarrantyID: w.ID, ServiceID: w.ServiceID,
		VehicleID: w.VehicleID, CustomerUserID: w.HolderUserID, Description: "Kaput " + tag, Status: "open",
		CoverageCheck: []byte(`{"ok":true,"reasons":[],"checked_at":"2026-10-01T00:00:00Z"}`),
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	ua, pwa := it.user("t339-dealer-a")
	it.member(dealerA, ua, "owner")
	dealerTok := it.loginOrg(ua, pwa, dealerA)
	ub, pwb := it.user("t339-dealer-b")
	it.member(dealerB, ub, "owner")
	otherTok := it.loginOrg(ub, pwb, dealerB)
	claimPath := "/v1/warranty-claims/" + c.Uuid.String()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="hasar.png"`)
	h.Set("Content-Type", "image/png")
	part, err := mw.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(pngBytes)
	_ = mw.Close()
	rec := it.raw("POST", claimPath+"/photos", dealerTok, mw.FormDataContentType(), buf.Bytes(), nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	photo := decodeEnv[struct {
		UUID string `json:"uuid"`
	}](t, rec)

	type claimView struct {
		OrganizationUUID string `json:"organization_uuid"`
		OrganizationName string `json:"organization_name"`
		WarrantyNo       string `json:"warranty_no"`
		ProductName      string `json:"product_name"`
		ServiceUUID      string `json:"service_uuid"`
		ServiceNo        string `json:"service_no"`
		Events           []struct {
			EventType string  `json:"event_type"`
			ToStatus  *string `json:"to_status"`
		} `json:"events"`
	}
	rec = it.raw("GET", claimPath, dealerTok, "", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body.String())
	}
	got := decodeEnv[claimView](t, rec)
	if got.OrganizationUUID != dealerA.Uuid.String() || got.OrganizationName != dealerA.Name ||
		got.WarrantyNo != w.PublicCode || got.ProductName == "" || got.ServiceUUID == "" || got.ServiceNo == "" {
		t.Fatalf("detail context = %+v", got)
	}
	if len(got.Events) != 1 || got.Events[0].EventType != "created" || got.Events[0].ToStatus == nil ||
		*got.Events[0].ToStatus != "open" {
		t.Fatalf("events = %+v", got.Events)
	}

	rec = it.raw("GET", "/v1/warranty-claims?limit=100", dealerTok, "", nil, nil)
	list := decodeEnv[struct {
		Items []claimView `json:"items"`
	}](t, rec)
	if len(list.Items) != 1 || list.Items[0].OrganizationName != dealerA.Name || list.Items[0].WarrantyNo != w.PublicCode ||
		list.Items[0].Events != nil {
		t.Fatalf("list = %+v", list.Items)
	}

	photoPath := claimPath + "/photos/" + photo.UUID
	rec = it.raw("GET", photoPath, dealerTok, "", nil, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" ||
		!bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Fatalf("photo: %d %v", rec.Code, rec.Header())
	}
	if rec := it.raw("GET", claimPath+"/photos/"+uuid.NewString(), dealerTok, "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown photo = %d", rec.Code)
	}
	if rec := it.raw("GET", photoPath, otherTok, "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("sibling dealer photo = %d", rec.Code)
	}

	portalTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{
		UserID: cust.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal,
	})
	if err != nil {
		t.Fatal(err)
	}
	code, env := it.do("GET", "/v1/portal/warranty-claims", hostOlex, portalTok, nil)
	if code != http.StatusOK {
		t.Fatalf("portal = %d", code)
	}
	portal := decodeData[struct {
		Items []map[string]any `json:"items"`
	}](t, env)
	if len(portal.Items) != 1 || portal.Items[0]["warranty_uuid"] != w.Uuid.String() {
		t.Fatalf("portal = %+v", portal.Items)
	}
	for _, hidden := range []string{"description", "photos", "organization_name"} {
		if _, ok := portal.Items[0][hidden]; ok {
			t.Fatalf("portal leaks %s", hidden)
		}
	}
}
