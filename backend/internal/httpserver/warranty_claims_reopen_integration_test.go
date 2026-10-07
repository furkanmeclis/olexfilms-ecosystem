package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// TEC-462: only a platform super admin can reopen a closed warranty claim.
// Reopen moves closed -> approved, writes the reopened timeline/audit/outbox
// notification event, does not create accounting rows, and refuses center
// staff, dealers and already-open claims.
func TestIntegrationWarrantyClaimReopen(t *testing.T) {
	var serverErr error
	prevHook := response.ServerErrorHook
	response.ServerErrorHook = func(_ *http.Request, err error) { serverErr = err }
	t.Cleanup(func() { response.ServerErrorHook = prevHook })

	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t462-dealer", "dealer", center)
	product := it.product(center, "T462R")
	it.setWarrantyMonths(product, 12)
	cust, veh := it.svcCustomer(dealer, "t462-cust", "34R462"+it.suffix[len(it.suffix)-4:])

	makeClaim := func(seq int) db.WarrantyClaim {
		t.Helper()
		w := it.warrantyFor(dealer, center, product, cust, veh, seq)
		claim, err := it.q.CreateWarrantyClaim(ctx, db.CreateWarrantyClaimParams{
			OrganizationID: w.OrganizationID, BrandID: w.BrandID, WarrantyID: w.ID,
			ServiceID: w.ServiceID, VehicleID: w.VehicleID, CustomerUserID: w.HolderUserID,
			Description: "TEC-462 yeniden açma", Status: "open",
			CoverageCheck: []byte(`{"ok":true,"reasons":[],"checked_at":"2026-10-07T00:00:00Z"}`),
		})
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		return claim
	}

	closedClaim := makeClaim(46201)
	closedClaim, err := it.q.SetWarrantyClaimStatus(ctx, db.SetWarrantyClaimStatusParams{
		ID: closedClaim.ID, BrandID: closedClaim.BrandID, FromStatus: "open", Status: "closed",
	})
	if err != nil {
		t.Fatalf("close claim: %v", err)
	}
	openClaim := makeClaim(46202)

	dealerOwner, dealerPW := it.user("t462-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	dealerTok := it.loginOrg(dealerOwner, dealerPW, dealer)
	centerStaff, centerPW := it.user("t462-center")
	it.member(center, centerStaff, "staff", rbac.RoleCenterStaff)
	centerTok := it.loginOrg(centerStaff, centerPW, center)
	admin, adminPW := it.user("t462-admin", rbac.RoleSuperAdmin)
	it.member(center, admin, "staff", rbac.RoleCenterStaff)
	adminTok := it.loginOrg(admin, adminPW, center)

	path := "/v1/warranty-claims/" + closedClaim.Uuid.String() + "/reopen"
	for name, tok := range map[string]string{"dealer": dealerTok, "center": centerTok} {
		if code, env := it.do("POST", path, hostOlex, tok, map[string]any{"reason": "yetki testi"}); code != http.StatusForbidden {
			t.Fatalf("%s reopen = %d %s, want 403", name, code, errCode(env))
		}
	}
	if code, env := it.do("POST", path, hostOlex, adminTok, map[string]any{"reason": "   "}); code != http.StatusBadRequest {
		t.Fatalf("empty reason = %d %s, want 400", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/warranty-claims/"+openClaim.Uuid.String()+"/reopen", hostOlex, adminTok,
		map[string]any{"reason": "açık talep"}); code != http.StatusConflict {
		t.Fatalf("open reopen = %d %s, want 409", code, errCode(env))
	}

	before := it.countRows(`SELECT COUNT(*) FROM finance_entries WHERE source_type = 'warranty_claim' AND source_uuid = $1`, closedClaim.Uuid)
	code, env := it.do("POST", path, hostOlex, adminTok, map[string]any{"reason": "Tamamlanmış iptal ters kaydı düzeltildi"})
	if code != http.StatusOK {
		t.Fatalf("admin reopen = %d %s (%v)", code, errCode(env), serverErr)
	}
	var view struct {
		Status string `json:"status"`
		Events []struct {
			EventType string  `json:"event_type"`
			Note      *string `json:"note"`
		} `json:"events"`
	}
	if err := json.Unmarshal(env.Data, &view); err != nil {
		t.Fatalf("reopen payload: %s", env.Data)
	}
	if view.Status != "approved" {
		t.Fatalf("status = %s, want approved", view.Status)
	}
	var reopened bool
	for _, ev := range view.Events {
		if ev.EventType == "reopened" && ev.Note != nil && *ev.Note == "Tamamlanmış iptal ters kaydı düzeltildi" {
			reopened = true
		}
	}
	if !reopened {
		t.Fatalf("reopened event missing: %+v", view.Events)
	}
	after := it.countRows(`SELECT COUNT(*) FROM finance_entries WHERE source_type = 'warranty_claim' AND source_uuid = $1`, closedClaim.Uuid)
	if before != after {
		t.Fatalf("finance rows changed: %d -> %d", before, after)
	}
	var audits int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_events
WHERE action = 'warranty_claim.reopened' AND resource_uuid = $1 AND actor_user_id = $2`,
		closedClaim.Uuid, admin.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("activity = %d, %v", audits, err)
	}
	var outboxed int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events
WHERE event_name = 'warranty_claim.status_changed'
  AND payload->'data'->>'claim_uuid' = $1
  AND payload->'data'->>'action' = 'reopened'
  AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(payload->'data'->'notify_user_ids') v WHERE v = $2)
  AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(payload->'data'->'notify_user_ids') v WHERE v = $3)`,
		closedClaim.Uuid.String(), strconv.FormatInt(dealerOwner.ID, 10), strconv.FormatInt(centerStaff.ID, 10)).Scan(&outboxed); err != nil || outboxed != 1 {
		t.Fatalf("reopen notification outbox = %d, %v", outboxed, err)
	}
	if err := it.srv.events.Publish(ctx, it.warrantyClaimOutboxEvent("warranty_claim.status_changed", closedClaim.Uuid.String())); err != nil {
		t.Fatalf("publish warranty_claim.status_changed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if n := it.countRows(`SELECT COUNT(*) FROM notification_deliveries
WHERE event_code = 'WARRANTY_CLAIM_REOPENED' AND user_id IN ($1, $2)`, dealerOwner.ID, centerStaff.ID); n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("WARRANTY_CLAIM_REOPENED notifications were not delivered")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (it *itest) warrantyClaimOutboxEvent(name, claimUUID string) events.Event {
	it.t.Helper()
	var raw []byte
	if err := it.pool.QueryRow(context.Background(), `SELECT payload FROM outbox_events
WHERE event_name = $1 AND payload->'data'->>'claim_uuid' = $2 ORDER BY id DESC LIMIT 1`,
		name, claimUUID).Scan(&raw); err != nil {
		it.t.Fatalf("outbox %s: %v", name, err)
	}
	var env struct {
		EventID     uuid.UUID      `json:"event_id"`
		TenantID    *int64         `json:"tenant_id"`
		ActorUserID *int64         `json:"actor_user_id"`
		EntityType  string         `json:"entity_type"`
		EntityID    *int64         `json:"entity_id"`
		EntityUUID  *uuid.UUID     `json:"entity_uuid"`
		Data        map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		it.t.Fatal(err)
	}
	return events.Event{
		Name: name, EventID: env.EventID, TenantID: env.TenantID, ActorUserID: env.ActorUserID,
		EntityType: env.EntityType, EntityID: env.EntityID, EntityUUID: env.EntityUUID,
		Payload: env.Data, OccurredAt: time.Now().UTC(),
	}
}
