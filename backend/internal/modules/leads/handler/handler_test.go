package handler

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestLeadUUIDBodies(t *testing.T) {
	assignee, customer, vehicle := uuid.New(), uuid.New(), uuid.New()
	payload, err := json.Marshal(map[string]any{"assignee_user_uuid": assignee, "customer_uuid": customer, "vehicle_uuid": vehicle, "assignee_user_id": -1, "customer_user_id": -1, "vehicle_id": -1})
	if err != nil {
		t.Fatal(err)
	}
	var create leadBody
	if err := json.Unmarshal(payload, &create); err != nil {
		t.Fatal(err)
	}
	input := create.input()
	if input.AssigneeUserUUID == nil || *input.AssigneeUserUUID != assignee || input.CustomerUUID == nil || *input.CustomerUUID != customer || input.VehicleUUID == nil || *input.VehicleUUID != vehicle {
		t.Fatalf("create=%+v", input)
	}
	var patch patchBody
	if err := json.Unmarshal(payload, &patch); err != nil {
		t.Fatal(err)
	}
	if !patch.AssigneeUserUUID.Set || patch.AssigneeUserUUID.Value == nil || *patch.AssigneeUserUUID.Value != assignee || !patch.CustomerUUID.Set || patch.CustomerUUID.Value == nil || *patch.CustomerUUID.Value != customer || !patch.VehicleUUID.Set || patch.VehicleUUID.Value == nil || *patch.VehicleUUID.Value != vehicle {
		t.Fatalf("patch=%+v", patch)
	}
	if err := json.Unmarshal([]byte(`{"assignee_user_uuid":null,"customer_uuid":null,"vehicle_uuid":null}`), &patch); err != nil {
		t.Fatal(err)
	}
	if !patch.AssigneeUserUUID.Set || patch.AssigneeUserUUID.Value != nil || !patch.CustomerUUID.Set || patch.CustomerUUID.Value != nil || !patch.VehicleUUID.Set || patch.VehicleUUID.Value != nil {
		t.Fatalf("null patch=%+v", patch)
	}
	var assign assignBody
	data, _ := json.Marshal(map[string]any{"assignee_user_uuid": assignee, "assignee_user_id": -1})
	if err := json.Unmarshal(data, &assign); err != nil {
		t.Fatal(err)
	}
	if assign.AssigneeUserUUID.Value == nil || *assign.AssigneeUserUUID.Value != assignee {
		t.Fatalf("assign=%+v", assign)
	}
}

func TestLeadCreateNullUUIDOverridesNumericID(t *testing.T) {
	var body leadBody
	if err := json.Unmarshal([]byte(`{"assignee_user_uuid":null,"assignee_user_id":42,"customer_uuid":null,"customer_user_id":42,"vehicle_uuid":null,"vehicle_id":42}`), &body); err != nil {
		t.Fatal(err)
	}
	input := body.input()
	if input.AssigneeUserID != nil || input.CustomerUserID != nil || input.VehicleID != nil {
		t.Fatalf("input=%+v", input)
	}
}
