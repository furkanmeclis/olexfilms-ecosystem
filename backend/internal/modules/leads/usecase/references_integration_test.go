//go:build integration

package usecase

import (
	"errors"
	"strconv"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

func (f *fixture) referenceFixtures() (uuid.UUID, int64, uuid.UUID, int64, uuid.UUID, int64) {
	f.t.Helper()
	member := f.userRow("assignee")
	customer := f.userRow("customer")
	vehicle := f.vehicle(customer, f.dealer)
	if _, err := f.tx.Exec(f.ctx, `INSERT INTO organization_members (organization_id,user_id) VALUES ($1,$2)`, f.dealer.ID, member.ID); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.tx.Exec(f.ctx, `INSERT INTO customer_organizations (organization_id,brand_id,user_id) VALUES ($1,$2,$3)`, f.dealer.ID, f.brand, customer.ID); err != nil {
		f.t.Fatal(err)
	}
	return member.Uuid, member.ID, customer.Uuid, customer.ID, vehicle.Uuid, vehicle.ID
}

func TestLeadUUIDReferences(t *testing.T) {
	f := newFixture(t)
	assignee, assigneeID, customer, customerID, vehicle, vehicleID := f.referenceFixtures()
	c := f.caller(f.dealer, rbac.ScopeManaged)
	invalidID := int64(-1)
	lead, err := f.svc.Create(f.ctx, c, CreateInput{TargetType: "customer", Source: "walk_in", AssigneeUserUUID: &assignee, AssigneeUserID: &invalidID, CustomerUUID: &customer, CustomerUserID: &invalidID, VehicleUUID: &vehicle, VehicleID: &invalidID})
	if err != nil {
		t.Fatal(err)
	}
	check := func(l Lead) {
		t.Helper()
		if l.AssigneeUserID == nil || *l.AssigneeUserID != assigneeID || l.AssigneeUserUUID == nil || *l.AssigneeUserUUID != assignee || l.CustomerUserID == nil || *l.CustomerUserID != customerID || l.CustomerUUID == nil || *l.CustomerUUID != customer || l.VehicleID == nil || *l.VehicleID != vehicleID || l.VehicleUUID == nil || *l.VehicleUUID != vehicle {
			t.Fatalf("references = %+v", l)
		}
	}
	check(lead)
	got, err := f.svc.Get(f.ctx, c, lead.UUID)
	if err != nil {
		t.Fatal(err)
	}
	check(got)
	patched, err := f.svc.Patch(f.ctx, c, lead.UUID, PatchInput{AssigneeUserUUID: Field[uuid.UUID]{Set: true, Value: &assignee}, AssigneeUserID: Field[int64]{Set: true, Value: &invalidID}, CustomerUUID: Field[uuid.UUID]{Set: true, Value: &customer}, CustomerUserID: Field[int64]{Set: true, Value: &invalidID}, VehicleUUID: Field[uuid.UUID]{Set: true, Value: &vehicle}, VehicleID: Field[int64]{Set: true, Value: &invalidID}})
	if err != nil {
		t.Fatal(err)
	}
	check(patched)
	assigned, err := f.svc.Assign(f.ctx, c, lead.UUID, &invalidID, &assignee)
	if err != nil {
		t.Fatal(err)
	}
	check(assigned)
	cleared, err := f.svc.Patch(f.ctx, c, lead.UUID, PatchInput{AssigneeUserUUID: Field[uuid.UUID]{Set: true}, AssigneeUserID: Field[int64]{Set: true, Value: &assigneeID}, CustomerUUID: Field[uuid.UUID]{Set: true}, VehicleUUID: Field[uuid.UUID]{Set: true}})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.AssigneeUserID != nil || cleared.CustomerUUID != nil || cleared.VehicleUUID != nil {
		t.Fatalf("null UUID did not clear: %+v", cleared)
	}
}

func TestLeadUUIDReferencesRejectUnknownAndOutOfScope(t *testing.T) {
	f := newFixture(t)
	assignee, assigneeID, customer, customerID, vehicle, vehicleID := f.referenceFixtures()
	c := f.caller(f.other, rbac.ScopeManaged)
	for _, tc := range []struct {
		name, field string
		uid         uuid.UUID
		id          int64
	}{
		{"assignee", "assignee_user_id", assignee, assigneeID}, {"customer", "customer_user_id", customer, customerID}, {"vehicle", "vehicle_id", vehicle, vehicleID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, unknown := range []bool{false, true} {
				uid := tc.uid
				id := tc.id
				if unknown {
					uid = uuid.New()
					id = -1
				}
				makeInput := func(byUUID bool) CreateInput {
					in := CreateInput{TargetType: "customer", Source: "walk_in"}
					switch tc.name {
					case "assignee":
						if byUUID {
							in.AssigneeUserUUID = &uid
						} else {
							in.AssigneeUserID = &id
						}
					case "customer":
						if byUUID {
							in.CustomerUUID = &uid
						} else {
							in.CustomerUserID = &id
						}
					case "vehicle":
						if byUUID {
							in.VehicleUUID = &uid
						} else {
							in.VehicleID = &id
						}
					}
					return in
				}
				_, numericErr := f.svc.Create(f.ctx, c, makeInput(false))
				_, uuidErr := f.svc.Create(f.ctx, c, makeInput(true))
				var validation *ValidationError
				if !errors.As(uuidErr, &validation) || validation.Field != tc.field || numericErr == nil || numericErr.Error() != uuidErr.Error() {
					t.Fatalf("numeric=%v uuid=%v", numericErr, uuidErr)
				}
				lead := f.lead(f.other, StatusNew, nil)
				patch := PatchInput{}
				switch tc.name {
				case "assignee":
					patch.AssigneeUserUUID = Field[uuid.UUID]{Set: true, Value: &uid}
				case "customer":
					patch.CustomerUUID = Field[uuid.UUID]{Set: true, Value: &uid}
				case "vehicle":
					patch.VehicleUUID = Field[uuid.UUID]{Set: true, Value: &uid}
				}
				if _, err := f.svc.Patch(f.ctx, c, lead.Uuid, patch); !errors.As(err, &validation) {
					t.Fatalf("patch=%v", err)
				}
				if tc.name == "assignee" {
					if _, err := f.svc.Assign(f.ctx, c, lead.Uuid, nil, &uid); !errors.As(err, &validation) {
						t.Fatalf("assign=%v", err)
					}
				}
			}
		})
	}
}

func TestLeadAssigneeDirectoryScope(t *testing.T) {
	f := newFixture(t)
	assignee, _, _, _, _, _ := f.referenceFixtures()
	c := f.caller(f.dealer, rbac.ScopeManaged)
	options, err := f.svc.Assignees(f.ctx, c, nil, "assignee")
	if err != nil || len(options) != 1 || options[0].UUID != assignee {
		t.Fatalf("options=%+v err=%v", options, err)
	}
	if _, err := f.svc.Assignees(f.ctx, c, &f.other.Uuid, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("scope err=%v", err)
	}
}

func TestBulkAssigneeUUIDPrecedenceAndScope(t *testing.T) {
	f := newFixture(t)
	assignee, assigneeID, _, _, _, _ := f.referenceFixtures()
	lead := f.lead(f.dealer, StatusNew, nil)
	adapter := NewBulkAdapter(f.q)
	run := bulkengine.Run{Query: map[string]string{queryOrganizationKey: f.dealer.Uuid.String(), bulkengine.QueryScope: strconv.FormatInt(f.dealer.ID, 10)}, Params: map[string]string{ParamAssigneeUserUUID: assignee.String(), ParamAssigneeUserID: "-1"}}
	ctx := bulkengine.WithRun(f.ctx, run)
	res, err := adapter.ApplyItem(ctx, BulkAssign, lead.Uuid.String())
	if err != nil || !res.OK {
		t.Fatalf("result=%+v err=%v", res, err)
	}
	got, err := f.svc.Get(f.ctx, f.caller(f.dealer, rbac.ScopeManaged), lead.Uuid)
	if err != nil || got.AssigneeUserID == nil || *got.AssigneeUserID != assigneeID {
		t.Fatalf("lead=%+v err=%v", got, err)
	}
	for _, uid := range []uuid.UUID{f.user.Uuid, uuid.New()} {
		run.Params[ParamAssigneeUserUUID] = uid.String()
		res, err = adapter.ApplyItem(bulkengine.WithRun(f.ctx, run), BulkAssign, lead.Uuid.String())
		if err != nil || res.OK {
			t.Fatalf("invalid result=%+v err=%v", res, err)
		}
	}
}
