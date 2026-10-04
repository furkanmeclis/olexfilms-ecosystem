package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/google/uuid"
)

func (it *itest) cancelCompleted(token, svcUUID string, body map[string]any, want int) (serviceView, string) {
	it.t.Helper()
	return it.svcCall("POST", "/v1/services/"+svcUUID+"/cancel-completed", token, body, want)
}

// TEC-356 acceptance: a completed service can be cancelled by an authorized
// manager, but not dealer_staff. The transaction voids warranties, returns
// whole consumptions to stock, reverses service-sourced accounting rows,
// writes audit/activity and moves the service to cancelled. Repeating the
// operation is a 409 and writes nothing new.
func TestIntegrationCancelCompletedService(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t356-dist", "distributor", center)
	dealer := it.org("t356-dealer", "dealer", dist)

	owner, opw := it.user("t356-owner")
	it.member(dealer, owner, "owner")
	staff, spw := it.user("t356-staff")
	it.member(dealer, staff, "staff")
	cust, veh := it.svcCustomer(dealer, "t356-cust", "34T356"+it.suffix[len(it.suffix)-4:])

	product := it.product(center, "T356P")
	it.setWarrantyMonths(product, 12)
	chain := it.stockChain()
	centerLoc := chain.location(center, "C356")
	distLoc := chain.location(dist, "D356")
	dealerOwner := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID}
	unit := chain.unit(center, product, 3560)
	chain.post(ledger.TypeEntry, unit, chain.nextRef(), centerLoc)
	chain.ship(unit, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, distLoc)
	chain.ship(unit, ledger.TypeOrderOut, ledger.TypeReceived, dealer, dealerOwner)

	ownerTok := it.loginOrg(owner, opw, dealer)
	staffTok := it.loginOrg(staff, spw, dealer)
	svc, _ := it.svcCall("POST", "/v1/services", ownerTok,
		map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}, http.StatusCreated)
	it.svcCall("POST", "/v1/services/"+svc.UUID+"/items", ownerTok,
		map[string]any{"barcode": unit.Barcode, "kind": "full"}, http.StatusCreated)
	done, _ := it.svcCall("POST", "/v1/services/"+svc.UUID+"/transitions", ownerTok,
		map[string]string{"status": "completed"}, http.StatusOK)
	if done.Status != "completed" {
		t.Fatalf("completed status = %s", done.Status)
	}
	var svcID int64
	if err := it.pool.QueryRow(ctx, `SELECT id FROM services WHERE uuid = $1`, svc.UUID).Scan(&svcID); err != nil {
		t.Fatal(err)
	}
	if err := it.srv.events.Publish(ctx, it.outboxEvent(events.ServiceCompleted, svc.UUID)); err != nil {
		t.Fatalf("publish service.completed: %v", err)
	}
	ws := it.serviceWarranties(svcID, dealer.BrandID)
	if len(ws) != 1 || ws[0].Status != "active" {
		t.Fatalf("warranties before cancel = %+v", ws)
	}

	p := posting.New(it.q, outbox.NewStore(it.pool, it.q), nil)
	err := func() error {
		tx, err := it.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = p.PostIncome(ctx, tx, posting.Entry{
			OrganizationID:    dealer.ID,
			Source:            posting.Source{Type: "service", UUID: uuid.MustParse(svc.UUID)},
			Role:              posting.DefaultRole,
			Category:          "service",
			Amount:            "120.00",
			Currency:          dealer.Currency,
			CounterpartyOrgID: dist.ID,
			Description:       "Service " + done.ServiceNo,
			ActorUserID:       &owner.ID,
		})
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}()
	if err != nil {
		t.Fatalf("service accounting seed: %v", err)
	}
	if got := it.cariBalance(dealer.ID, dist.ID); got != "120.00" {
		t.Fatalf("dealer cari before cancel = %s", got)
	}

	if _, ec := it.cancelCompleted(staffTok, svc.UUID, map[string]any{"reason": "müşteri iptal istedi"}, http.StatusForbidden); ec == "" {
		t.Fatal("dealer_staff cancel-completed returned no error code")
	}
	cancelled, _ := it.cancelCompleted(ownerTok, svc.UUID, map[string]any{"reason": "müşteri iptal istedi"}, http.StatusOK)
	if cancelled.Status != "cancelled" || cancelled.CancelReason != "müşteri iptal istedi" {
		t.Fatalf("cancelled service = %+v", cancelled)
	}
	if _, ec := it.cancelCompleted(ownerTok, svc.UUID, map[string]any{"reason": "tekrar"}, http.StatusConflict); ec != "SERVICE_INVALID_TRANSITION" {
		t.Fatalf("second cancel = %s", ec)
	}

	ws = it.serviceWarranties(svcID, dealer.BrandID)
	if len(ws) != 1 || ws[0].Status != "void" || !ws[0].VoidReason.Valid || ws[0].VoidReason.String != "service_cancelled" {
		t.Fatalf("warranties after cancel = %+v", ws)
	}
	var status, ownerType, types string
	var ownerID int64
	if err := it.pool.QueryRow(ctx, `SELECT u.status, s.owner_type, s.owner_id,
		(SELECT string_agg(m.type, ',' ORDER BY m.id) FROM stock_movements m WHERE m.unit_id = u.id)
		FROM units u JOIN unit_current_state s ON s.unit_id = u.id WHERE u.id = $1`, unit.ID).
		Scan(&status, &ownerType, &ownerID, &types); err != nil {
		t.Fatal(err)
	}
	if status != "available" || ownerType != "organization" || ownerID != dealer.ID ||
		types != "entry,transfer_out,transfer_in,order_out,received,consumption,return" {
		t.Fatalf("unit after cancel = %s %s %d %s", status, ownerType, ownerID, types)
	}
	rep, err := rebuild.New(it.pool, it.q).Check(ctx, dealer.ID)
	if err != nil {
		t.Fatalf("rebuild check: %v", err)
	}
	if rep.DiffCount != 0 || len(rep.Anomalies) != 0 {
		t.Fatalf("rebuild check = %d diffs %v anomalies", rep.DiffCount, rep.Anomalies)
	}
	if got := it.cariBalance(dealer.ID, dist.ID); got != "0.00" {
		t.Fatalf("dealer cari after cancel = %s", got)
	}
	var financeRows, reversals, auditRows int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*), COUNT(reversal_of_id)
		FROM finance_entries WHERE source_type = 'service' AND source_uuid = $1::uuid`, svc.UUID).Scan(&financeRows, &reversals); err != nil {
		t.Fatal(err)
	}
	if financeRows != 2 || reversals != 1 {
		t.Fatalf("finance rows = %d reversals = %d, want 2/1", financeRows, reversals)
	}
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_events
		WHERE action = 'service.completed_cancelled' AND resource_uuid = $1::uuid`, svc.UUID).Scan(&auditRows); err != nil {
		t.Fatal(err)
	}
	if auditRows != 1 {
		t.Fatalf("activity rows = %d, want 1", auditRows)
	}
}
