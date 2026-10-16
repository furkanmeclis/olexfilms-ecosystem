//go:build integration

package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	leadsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestIntegrationLeadUUIDReferences(t *testing.T) {
	it := crmIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t773-dealer", "dealer", center)
	owner, pw := it.user("t773-owner")
	it.member(dealer, owner, "owner")
	outsider, _ := it.user("t773-outsider")
	customer, _ := it.user("t773-customer")
	if _, err := it.pool.Exec(ctx, `INSERT INTO customer_organizations (organization_id,brand_id,user_id) VALUES ($1,$2,$3)`, dealer.ID, dealer.BrandID, customer.ID); err != nil {
		t.Fatal(err)
	}
	vehicle, err := it.q.CreateVehicle(ctx, db.CreateVehicleParams{UserID: customer.ID, OrganizationID: pgtype.Int8{Int64: dealer.ID, Valid: true}, BrandID: dealer.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(ctx, `DELETE FROM lead_events WHERE organization_id=$1`, dealer.ID)
		_, _ = it.pool.Exec(ctx, `DELETE FROM leads WHERE organization_id=$1`, dealer.ID)
		_, _ = it.pool.Exec(ctx, `DELETE FROM vehicles WHERE id=$1`, vehicle.ID)
		_, _ = it.pool.Exec(ctx, `DELETE FROM customer_organizations WHERE user_id=$1`, customer.ID)
	})
	tok := it.loginOrg(owner, pw, dealer)
	lead := decodeData[leadsuc.Lead](t, it.custDo("POST", "/v1/leads", tok, map[string]any{"target_type": "customer", "source": "walk_in", "assignee_user_uuid": owner.Uuid, "assignee_user_id": -1, "customer_uuid": customer.Uuid, "customer_user_id": -1, "vehicle_uuid": vehicle.Uuid, "vehicle_id": -1}, http.StatusCreated))
	if lead.AssigneeUserUUID == nil || *lead.AssigneeUserUUID != owner.Uuid || lead.CustomerUUID == nil || *lead.CustomerUUID != customer.Uuid || lead.VehicleUUID == nil || *lead.VehicleUUID != vehicle.Uuid {
		t.Fatalf("lead=%+v", lead)
	}
	path := "/v1/leads/" + lead.UUID.String()
	patched := decodeData[leadsuc.Lead](t, it.custDo("PATCH", path, tok, map[string]any{"assignee_user_uuid": owner.Uuid, "assignee_user_id": -1, "customer_uuid": customer.Uuid, "customer_user_id": -1, "vehicle_uuid": vehicle.Uuid, "vehicle_id": -1}, http.StatusOK))
	if patched.VehicleUUID == nil || *patched.VehicleUUID != vehicle.Uuid {
		t.Fatalf("patched=%+v", patched)
	}
	assigned := decodeData[leadsuc.Lead](t, it.custDo("POST", path+"/assign", tok, map[string]any{"assignee_user_uuid": owner.Uuid, "assignee_user_id": -1}, http.StatusOK))
	if assigned.AssigneeUserUUID == nil || *assigned.AssigneeUserUUID != owner.Uuid {
		t.Fatalf("assigned=%+v", assigned)
	}
	for _, uid := range []uuid.UUID{outsider.Uuid, uuid.New()} {
		it.custDo("POST", path+"/assign", tok, map[string]any{"assignee_user_uuid": uid, "assignee_user_id": owner.ID}, http.StatusBadRequest)
		it.custDo("PATCH", path, tok, map[string]any{"customer_uuid": uid, "customer_user_id": customer.ID}, http.StatusBadRequest)
		it.custDo("PATCH", path, tok, map[string]any{"vehicle_uuid": uid, "vehicle_id": vehicle.ID}, http.StatusBadRequest)
	}
	directory := decodeData[struct {
		Items []leadsuc.AssigneeOption `json:"items"`
	}](t, it.custDo("GET", "/v1/leads/assignees?q=t773-owner", tok, nil, http.StatusOK))
	if len(directory.Items) != 1 || directory.Items[0].UUID != owner.Uuid {
		t.Fatalf("directory=%+v", directory)
	}
	cleared := decodeData[leadsuc.Lead](t, it.custDo("POST", path+"/assign", tok, map[string]any{"assignee_user_uuid": nil, "assignee_user_id": owner.ID}, http.StatusOK))
	if cleared.AssigneeUserID != nil {
		t.Fatalf("cleared=%+v", cleared)
	}
}
