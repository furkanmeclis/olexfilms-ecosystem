package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type transferView struct {
	UUID           string `json:"uuid"`
	VehicleUUID    string `json:"vehicle_uuid"`
	Status         string `json:"status"`
	ToPhoneMasked  string `json:"to_phone_masked"`
	NewOwnerKnown  bool   `json:"new_owner_known"`
	FromVerified   bool   `json:"from_verified"`
	ToVerified     bool   `json:"to_verified"`
	Attempts       int32  `json:"attempts"`
	MaxAttempts    int32  `json:"max_attempts"`
	WarrantiesMove int    `json:"warranties_moved"`
}

// transferCodeFor returns the 6 digit code of the latest text sent to e164.
func transferCodeFor(t *testing.T, fw *fakeWuzapi, e164 string) string {
	t.Helper()
	fw.mu.Lock()
	defer fw.mu.Unlock()
	digits := strings.TrimPrefix(e164, "+")
	for i := len(fw.sends) - 1; i >= 0; i-- {
		if fw.sends[i]["Phone"] != digits {
			continue
		}
		body, _ := fw.sends[i]["Body"].(string)
		if m := itCodeRe.FindStringSubmatch(body); m != nil {
			return m[1]
		}
	}
	t.Fatalf("no code sent to %s", e164)
	return ""
}

func wrongCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

// TEC-190 acceptance: start -> two right codes -> completed: the vehicle
// owner and the active warranty holder change, the service keeps its
// customer, the events and audit rows are written. A wrong code counts an
// attempt and the limit cancels the transfer; an expired transfer cannot
// complete; a second pending transfer of the same vehicle is refused.
func TestIntegrationVehicleTransfer(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t190-dist", "distributor", center)
	dealer := it.org("t190-dealer", "dealer", dist)
	owner, pw := it.user("t190-owner")
	it.member(dealer, owner, "owner")
	tok := it.loginOrg(owner, pw, dealer)

	tail := it.suffix[len(it.suffix)-4:]
	cust, veh := it.svcCustomer(dealer, "t190-cust", "34V190"+tail)
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

	// A completed service with an active warranty (piece unit shipped
	// center -> distributor -> dealer).
	piece := it.product(center, "T190P")
	c := it.stockChain()
	cLoc := c.location(center, "C190")
	dLoc := c.location(dist, "D190")
	u := c.unit(center, piece, 1900)
	c.post(ledger.TypeEntry, u, c.nextRef(), cLoc)
	c.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	c.ship(u, ledger.TypeOrderOut, ledger.TypeReceived, dealer,
		ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID})
	svc, _ := it.svcCall("POST", "/v1/services", tok,
		map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}, http.StatusCreated)
	it.svcCall("POST", "/v1/services/"+svc.UUID+"/items", tok, map[string]any{"barcode": u.Barcode, "kind": "full"}, http.StatusCreated)
	if done, _ := it.svcCall("POST", "/v1/services/"+svc.UUID+"/transitions", tok,
		map[string]string{"status": "completed"}, http.StatusOK); done.Status != "completed" {
		t.Fatalf("service = %+v", done)
	}
	var itemID int64
	if err := it.pool.QueryRow(ctx, `SELECT si.id FROM service_items si JOIN services s ON s.id = si.service_id
		WHERE s.uuid = $1`, svc.UUID).Scan(&itemID); err != nil {
		t.Fatalf("service item: %v", err)
	}
	start := time.Now().Add(-time.Hour)
	if _, err := it.q.CreateWarrantyForServiceItem(ctx, db.CreateWarrantyForServiceItemParams{
		ServiceItemID: itemID, HolderUserID: cust.ID,
		StartAt: pgtype.Timestamptz{Time: start, Valid: true}, EndAt: pgtype.Timestamptz{Time: start.AddDate(1, 0, 0), Valid: true},
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("warranty: %v", err)
	}
	var warrantyUUID string
	if err := it.pool.QueryRow(ctx, `SELECT uuid::text FROM warranties WHERE service_item_id = $1`, itemID).Scan(&warrantyUUID); err != nil {
		t.Fatalf("warranty uuid: %v", err)
	}

	refused := func(method, path string, body any, wantStatus int, wantCode string) {
		t.Helper()
		if code, env := it.do(method, path, hostOlex, tok, body); code != wantStatus || errCode(env) != wantCode {
			t.Fatalf("%s %s = %d %s, want %d %s", method, path, code, errCode(env), wantStatus, wantCode)
		}
	}
	vPath := "/v1/vehicles/" + veh.Uuid.String() + "/transfers"

	// 1. Validation: same owner, bad phone.
	refused("POST", vPath, map[string]any{"phone": custPhone}, http.StatusConflict, "VEHICLE_TRANSFER_SAME_OWNER")
	refused("POST", vPath, map[string]any{"phone": "12"}, http.StatusBadRequest, "VALIDATION_ERROR")

	// 2. Start: two codes over WhatsApp, only hashes stored.
	tr := decodeData[transferView](t, it.custDo("POST", vPath, tok, map[string]any{"phone": newPhone}, http.StatusCreated))
	if tr.Status != "pending" || tr.NewOwnerKnown || tr.FromVerified || tr.ToVerified || tr.MaxAttempts != 5 {
		t.Fatalf("started = %+v", tr)
	}
	fromCode := transferCodeFor(t, fw, custPhone)
	toCode := transferCodeFor(t, fw, newPhone)
	var fromHash, toHash string
	if err := it.pool.QueryRow(ctx, `SELECT from_code_hash, to_code_hash FROM vehicle_transfers WHERE uuid = $1`, tr.UUID).
		Scan(&fromHash, &toHash); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fromHash, fromCode) || strings.Contains(toHash, toCode) || fromHash == toHash {
		t.Fatalf("codes stored in clear: %s %s", fromHash, toHash)
	}

	// 3. A second pending transfer of the same vehicle is refused.
	refused("POST", vPath, map[string]any{"phone": itPhone()}, http.StatusConflict, "VEHICLE_TRANSFER_PENDING")

	// 4. A wrong code counts one attempt; a new owner without an account
	// needs a name (refused before an attempt is spent).
	tPath := "/v1/vehicle-transfers/" + tr.UUID
	refused("POST", tPath+"/verify", map[string]any{"from_code": wrongCode(fromCode)},
		http.StatusUnprocessableEntity, "VEHICLE_TRANSFER_INVALID_CODE")
	refused("POST", tPath+"/verify", map[string]any{"from_code": fromCode, "to_code": toCode},
		http.StatusBadRequest, "VALIDATION_ERROR")
	var attempts int32
	if err := it.pool.QueryRow(ctx, `SELECT attempts FROM vehicle_transfers WHERE uuid = $1`, tr.UUID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("attempts = %d (%v), want 1", attempts, err)
	}

	// 5. Codes one by one: the old owner's first, then the new owner's.
	half := decodeData[transferView](t, it.custDo("POST", tPath+"/verify", tok, map[string]any{
		"from_code": fromCode, "new_owner_name": "Yeni", "new_owner_surname": "Sahip",
	}, http.StatusOK))
	if half.Status != "pending" || !half.FromVerified || half.ToVerified {
		t.Fatalf("after first code = %+v", half)
	}
	done := decodeData[transferView](t, it.custDo("POST", tPath+"/verify", tok, map[string]any{
		"to_code": toCode, "new_owner_name": "Yeni", "new_owner_surname": "Sahip",
	}, http.StatusOK))
	if done.Status != "completed" || done.WarrantiesMove != 1 || !done.NewOwnerKnown {
		t.Fatalf("completed = %+v", done)
	}

	// 6. Owner and warranty holder moved; the service keeps its customer.
	var newUserID, vehicleOwner, holder, svcCustomer int64
	var linked bool
	if err := it.pool.QueryRow(ctx, `SELECT
		(SELECT id FROM users WHERE phone_e164 = $1),
		(SELECT user_id FROM vehicles WHERE id = $2),
		(SELECT holder_user_id FROM warranties WHERE service_item_id = $3),
		(SELECT customer_user_id FROM services WHERE uuid = $4),
		EXISTS (SELECT 1 FROM customer_organizations co JOIN users u ON u.id = co.user_id
		        WHERE u.phone_e164 = $1 AND co.organization_id = $5)`,
		newPhone, veh.ID, itemID, svc.UUID, dealer.ID).Scan(&newUserID, &vehicleOwner, &holder, &svcCustomer, &linked); err != nil {
		t.Fatalf("after transfer: %v", err)
	}
	if vehicleOwner != newUserID || holder != newUserID || svcCustomer != cust.ID || !linked {
		t.Fatalf("after transfer: new %d vehicle %d holder %d service customer %d (want %d) linked %v",
			newUserID, vehicleOwner, holder, svcCustomer, cust.ID, linked)
	}
	var started, completed, holderChanged, audits int
	if err := it.pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM outbox_events WHERE event_name = $1 AND payload->>'entity_uuid' = $3::text),
		(SELECT COUNT(*) FROM outbox_events WHERE event_name = $2 AND payload->>'entity_uuid' = $3::text),
		(SELECT COUNT(*) FROM outbox_events WHERE event_name = $4 AND payload->>'entity_uuid' = $5),
		(SELECT COUNT(*) FROM activity_events WHERE action = $6 AND resource_uuid = $3::uuid)`,
		events.VehicleTransferStarted, events.VehicleTransferCompleted, tr.UUID,
		events.WarrantyHolderChanged, warrantyUUID, customersusecase.ActionTransferCompleted).
		Scan(&started, &completed, &holderChanged, &audits); err != nil {
		t.Fatalf("events: %v", err)
	}
	if started != 1 || completed != 1 || holderChanged != 1 || audits != 1 {
		t.Fatalf("events: started %d completed %d holder_changed %d audits %d", started, completed, holderChanged, audits)
	}
	refused("POST", tPath+"/verify", map[string]any{"to_code": toCode}, http.StatusConflict, "VEHICLE_TRANSFER_NOT_PENDING")
	refused("POST", tPath+"/cancel", nil, http.StatusConflict, "VEHICLE_TRANSFER_NOT_PENDING")

	// 7. Attempt limit: five wrong codes cancel the transfer.
	cust2, veh2 := it.svcCustomer(dealer, "t190-cust2", "34W190"+tail)
	cust2Phone := itPhone()
	if _, err := it.pool.Exec(ctx, `UPDATE users SET phone_e164 = $2 WHERE id = $1`, cust2.ID, cust2Phone); err != nil {
		t.Fatal(err)
	}
	// The new owner already has an account (cust): to_user_id set at start.
	tr2 := decodeData[transferView](t, it.custDo("POST", "/v1/vehicles/"+veh2.Uuid.String()+"/transfers", tok,
		map[string]any{"phone": custPhone}, http.StatusCreated))
	if !tr2.NewOwnerKnown {
		t.Fatalf("known owner = %+v", tr2)
	}
	from2 := transferCodeFor(t, fw, cust2Phone)
	t2Path := "/v1/vehicle-transfers/" + tr2.UUID
	for i := 1; i < 5; i++ {
		refused("POST", t2Path+"/verify", map[string]any{"from_code": wrongCode(from2)},
			http.StatusUnprocessableEntity, "VEHICLE_TRANSFER_INVALID_CODE")
	}
	refused("POST", t2Path+"/verify", map[string]any{"from_code": wrongCode(from2)}, http.StatusConflict, "VEHICLE_TRANSFER_LOCKED")
	refused("POST", t2Path+"/verify", map[string]any{"from_code": from2}, http.StatusConflict, "VEHICLE_TRANSFER_NOT_PENDING")
	var status2 string
	var owner2 int64
	if err := it.pool.QueryRow(ctx, `SELECT t.status, v.user_id FROM vehicle_transfers t JOIN vehicles v ON v.id = t.vehicle_id
		WHERE t.uuid = $1`, tr2.UUID).Scan(&status2, &owner2); err != nil || status2 != "cancelled" || owner2 != cust2.ID {
		t.Fatalf("locked transfer: %s owner %d (%v)", status2, owner2, err)
	}

	// 8. Expired: the right codes no longer complete the transfer.
	tr3 := decodeData[transferView](t, it.custDo("POST", "/v1/vehicles/"+veh2.Uuid.String()+"/transfers", tok,
		map[string]any{"phone": newPhone}, http.StatusCreated))
	from3, to3 := transferCodeFor(t, fw, cust2Phone), transferCodeFor(t, fw, newPhone)
	if _, err := it.pool.Exec(ctx, `UPDATE vehicle_transfers
		SET created_at = NOW() - interval '1 hour', expires_at = NOW() - interval '1 minute' WHERE uuid = $1`, tr3.UUID); err != nil {
		t.Fatal(err)
	}
	t3Path := "/v1/vehicle-transfers/" + tr3.UUID
	refused("POST", t3Path+"/verify", map[string]any{"from_code": from3, "to_code": to3}, http.StatusConflict, "VEHICLE_TRANSFER_EXPIRED")
	var status3 string
	var expiredEvents int
	if err := it.pool.QueryRow(ctx, `SELECT status,
		(SELECT COUNT(*) FROM outbox_events WHERE event_name = $2 AND payload->>'entity_uuid' = $1::text)
		FROM vehicle_transfers WHERE uuid = $1::uuid`, tr3.UUID, events.VehicleTransferExpired).Scan(&status3, &expiredEvents); err != nil ||
		status3 != "expired" || expiredEvents != 1 {
		t.Fatalf("expired transfer: %s events %d (%v)", status3, expiredEvents, err)
	}
	if err := it.pool.QueryRow(ctx, `SELECT user_id FROM vehicles WHERE id = $1`, veh2.ID).Scan(&owner2); err != nil || owner2 != cust2.ID {
		t.Fatalf("owner after expiry = %d (%v)", owner2, err)
	}

	// 9. Cancel: a new transfer can be cancelled; the list shows them all.
	tr4 := decodeData[transferView](t, it.custDo("POST", "/v1/vehicles/"+veh2.Uuid.String()+"/transfers", tok,
		map[string]any{"phone": newPhone}, http.StatusCreated))
	cancelled := decodeData[transferView](t, it.custDo("POST", "/v1/vehicle-transfers/"+tr4.UUID+"/cancel", tok, nil, http.StatusOK))
	if cancelled.Status != "cancelled" {
		t.Fatalf("cancelled = %+v", cancelled)
	}
	list := decodeData[struct {
		Items []transferView `json:"items"`
	}](t, it.custDo("GET", "/v1/vehicles/"+veh2.Uuid.String()+"/transfers", tok, nil, http.StatusOK))
	if len(list.Items) != 3 || list.Items[0].UUID != tr4.UUID {
		t.Fatalf("list = %+v", list.Items)
	}

	// 10. Another dealer does not see the transfer.
	other := it.org("t190-other", "dealer", dist)
	otherOwner, opw := it.user("t190-other-owner")
	it.member(other, otherOwner, "owner")
	otherTok := it.loginOrg(otherOwner, opw, other)
	if code, env := it.do("POST", "/v1/vehicle-transfers/"+tr4.UUID+"/cancel", hostOlex, otherTok, nil); code != http.StatusNotFound {
		t.Fatalf("other dealer cancel = %d %s", code, errCode(env))
	}
}
