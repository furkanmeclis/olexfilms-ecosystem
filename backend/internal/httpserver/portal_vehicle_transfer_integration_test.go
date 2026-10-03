package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-243 acceptance: from the portal the owner starts a transfer of their
// own vehicle, enters their code and the new owner's code, and the vehicle
// moves to the new owner. Another customer gets 404 on the vehicle and the
// transfer; a cancelled transfer cannot be verified (existing
// VEHICLE_TRANSFER_NOT_PENDING).
func TestIntegrationPortalVehicleTransfer(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t243-dist", "distributor", center)
	dealer := it.org("t243-dealer", "dealer", dist)

	tail := it.suffix[len(it.suffix)-4:]
	cust, veh := it.svcCustomer(dealer, "t243-cust", "34P243"+tail)
	stranger, _ := it.svcCustomer(dealer, "t243-stranger", "34S243"+tail)
	custPhone, newPhone := itPhone(), itPhone()
	for newPhone == custPhone {
		newPhone = itPhone()
	}
	if _, err := it.pool.Exec(ctx, `UPDATE users SET phone_e164 = $2 WHERE id = $1`, cust.ID, custPhone); err != nil {
		t.Fatalf("owner phone: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = it.pool.Exec(bg, `DELETE FROM vehicle_transfers WHERE organization_id = $1`, dealer.ID)
		_, _ = it.pool.Exec(bg, `DELETE FROM customer_organizations WHERE user_id IN (SELECT id FROM users WHERE phone_e164 = $1)`, newPhone)
	})

	portalTok := func(u db.User) string {
		tok, _, err := it.tokens.IssueAccess(jwt.AccessInput{
			UserID: u.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal,
		})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	tok, strangerTok := portalTok(cust), portalTok(stranger)
	refused := func(method, path, bearer string, body any, wantStatus int, wantCode string) {
		t.Helper()
		if code, env := it.do(method, path, hostOlex, bearer, body); code != wantStatus || (wantCode != "" && errCode(env) != wantCode) {
			t.Fatalf("%s %s = %d %s, want %d %s", method, path, code, errCode(env), wantStatus, wantCode)
		}
	}
	vPath := "/v1/portal/vehicles/" + veh.Uuid.String() + "/transfers"

	// 1. Not the owner: 404 on the list and the start.
	refused("GET", vPath, strangerTok, nil, http.StatusNotFound, "")
	refused("POST", vPath, strangerTok, map[string]any{"phone": newPhone}, http.StatusNotFound, "")

	// 2. Start + cancel; the cancelled transfer cannot be verified and the
	// stranger cannot touch it.
	tr := decodeData[transferView](t, it.custDo("POST", vPath, tok, map[string]any{"phone": newPhone}, http.StatusCreated))
	if tr.Status != "pending" || tr.VehicleUUID != veh.Uuid.String() {
		t.Fatalf("started = %+v", tr)
	}
	from1, to1 := transferCodeFor(t, fw, custPhone), transferCodeFor(t, fw, newPhone)
	t1 := "/v1/portal/vehicle-transfers/" + tr.UUID
	refused("POST", t1+"/cancel", strangerTok, nil, http.StatusNotFound, "")
	refused("POST", t1+"/verify", strangerTok, map[string]any{"from_code": from1}, http.StatusNotFound, "")
	if c := decodeData[transferView](t, it.custDo("POST", t1+"/cancel", tok, nil, http.StatusOK)); c.Status != "cancelled" {
		t.Fatalf("cancelled = %+v", c)
	}
	refused("POST", t1+"/verify", tok, map[string]any{"from_code": from1, "to_code": to1, "new_owner_name": "Yeni"},
		http.StatusConflict, "VEHICLE_TRANSFER_NOT_PENDING")

	// 3. Start again; a wrong code is the existing 422; the owner's code,
	// then the new owner's code completes the transfer.
	tr2 := decodeData[transferView](t, it.custDo("POST", vPath, tok, map[string]any{"phone": newPhone}, http.StatusCreated))
	from2, to2 := transferCodeFor(t, fw, custPhone), transferCodeFor(t, fw, newPhone)
	t2 := "/v1/portal/vehicle-transfers/" + tr2.UUID
	refused("POST", t2+"/verify", tok, map[string]any{"to_code": wrongCode(to2)},
		http.StatusUnprocessableEntity, "VEHICLE_TRANSFER_INVALID_CODE")
	half := decodeData[transferView](t, it.custDo("POST", t2+"/verify", tok, map[string]any{"from_code": from2}, http.StatusOK))
	if half.Status != "pending" || !half.FromVerified || half.ToVerified {
		t.Fatalf("after the owner's code = %+v", half)
	}
	done := decodeData[transferView](t, it.custDo("POST", t2+"/verify", tok, map[string]any{
		"to_code": to2, "new_owner_name": "Yeni", "new_owner_surname": "Sahip",
	}, http.StatusOK))
	if done.Status != "completed" || !done.NewOwnerKnown {
		t.Fatalf("completed = %+v", done)
	}

	// 4. The vehicle belongs to the new owner, who is linked to the
	// organization that registered the vehicle; the old owner no longer
	// sees it in the portal.
	var newUserID, owner, orgID int64
	var linked bool
	if err := it.pool.QueryRow(ctx, `SELECT
		(SELECT id FROM users WHERE phone_e164 = $1),
		(SELECT user_id FROM vehicles WHERE id = $2),
		(SELECT organization_id FROM vehicle_transfers WHERE uuid = $3::uuid),
		EXISTS (SELECT 1 FROM customer_organizations co JOIN users u ON u.id = co.user_id
		        WHERE u.phone_e164 = $1 AND co.organization_id = $4)`,
		newPhone, veh.ID, tr2.UUID, dealer.ID).Scan(&newUserID, &owner, &orgID, &linked); err != nil {
		t.Fatalf("after transfer: %v", err)
	}
	if owner != newUserID || orgID != dealer.ID || !linked {
		t.Fatalf("after transfer: owner %d new %d org %d (want %d) linked %v", owner, newUserID, orgID, dealer.ID, linked)
	}
	refused("GET", "/v1/portal/vehicles/"+veh.Uuid.String(), tok, nil, http.StatusNotFound, "")
	refused("GET", vPath, tok, nil, http.StatusNotFound, "")

	// 5. A panel token is refused on the portal realm.
	staff, pw := it.user("t243-staff")
	it.member(dealer, staff, "owner")
	if code, _ := it.do("GET", vPath, hostOlex, it.loginOrg(staff, pw, dealer), nil); code == http.StatusOK {
		t.Fatal("panel token reached the portal transfer list")
	}
}
