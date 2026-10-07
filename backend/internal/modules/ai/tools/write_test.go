package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appointmentsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	catalogmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	customersuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	leadsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	ordersuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	tasksuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// Write fakes: reads answer canned rows, every write is counted.

var (
	leadUUID        = uuid.MustParse("00000000-0000-0000-0000-000000000040")
	appointmentUUID = uuid.MustParse("00000000-0000-0000-0000-000000000050")
	customerUUID    = uuid.MustParse("00000000-0000-0000-0000-000000000060")
)

type writeLog struct{ calls []string }

func (w *writeLog) add(s string) { w.calls = append(w.calls, s) }

type fakeTasksW struct{ log *writeLog }

func (fakeTasksW) Subject(_ context.Context, _ tasksuc.Caller, id uuid.UUID) (tasksuc.SubjectRef, error) {
	if id != orgUUID {
		return tasksuc.SubjectRef{}, &tasksuc.ValidationError{Field: "subject_organization_uuid", Message: "must be a distributor or dealer"}
	}
	return tasksuc.SubjectRef{UUID: id, Name: "Bayi A", Type: "dealer"}, nil
}

func (f fakeTasksW) Create(_ context.Context, _ tasksuc.Caller, in tasksuc.CreateInput) (tasksuc.Task, error) {
	f.log.add("task:" + in.Title)
	return tasksuc.Task{UUID: uuid.New(), Title: in.Title}, nil
}

type fakeLeadsW struct{ log *writeLog }

func (fakeLeadsW) Get(_ context.Context, _ leadsuc.Caller, id uuid.UUID) (leadsuc.Lead, error) {
	if id != leadUUID {
		return leadsuc.Lead{}, leadsuc.ErrNotFound
	}
	name := "Ayşe"
	return leadsuc.Lead{UUID: id, CandidateContactName: &name}, nil
}

func (f fakeLeadsW) Create(_ context.Context, _ leadsuc.Caller, in leadsuc.CreateInput) (leadsuc.Lead, error) {
	f.log.add("lead:create")
	return leadsuc.Lead{UUID: uuid.New()}, nil
}

func (f fakeLeadsW) Patch(_ context.Context, _ leadsuc.Caller, id uuid.UUID, in leadsuc.PatchInput) (leadsuc.Lead, error) {
	f.log.add("lead:follow_up:" + in.FollowUpDate.Value.Format(time.DateOnly))
	return leadsuc.Lead{UUID: id}, nil
}

type fakeAppointmentsW struct {
	log    *writeLog
	status string
}

func (f fakeAppointmentsW) Get(_ context.Context, _ appointmentsuc.Caller, id uuid.UUID) (appointmentsuc.Appointment, error) {
	if id != appointmentUUID {
		return appointmentsuc.Appointment{}, appointmentsuc.ErrNotFound
	}
	return appointmentsuc.Appointment{UUID: id, Status: f.status, CustomerName: "Ali",
		StartsAt: time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)}, nil
}

func (f fakeAppointmentsW) Create(_ context.Context, _ appointmentsuc.Caller, in appointmentsuc.CreateInput) (appointmentsuc.Appointment, error) {
	f.log.add("appointment:create:" + in.StartsAt.UTC().Format(time.RFC3339) + ":" + in.Source)
	return appointmentsuc.Appointment{UUID: uuid.New()}, nil
}

func (f fakeAppointmentsW) SetStatus(_ context.Context, _ appointmentsuc.Caller, id uuid.UUID, in appointmentsuc.StatusInput) (appointmentsuc.Appointment, error) {
	f.log.add("appointment:" + in.Status + ":" + *in.CancelReason)
	return appointmentsuc.Appointment{UUID: id}, nil
}

type fakeCustomerGetter struct{}

func (fakeCustomerGetter) GetCustomer(_ context.Context, _ customersuc.Caller, id uuid.UUID) (customersuc.CustomerDetail, error) {
	if id != customerUUID {
		return customersuc.CustomerDetail{}, customersuc.ErrCustomerNotFound
	}
	return customersuc.CustomerDetail{CustomerSummary: customersuc.CustomerSummary{UUID: id, Name: "Ali", Surname: "Veli"}}, nil
}

type fakeOrdersW struct{ log *writeLog }

func (f fakeOrdersW) Create(_ context.Context, _ ordersuc.Caller, in ordersuc.CreateInput) (ordersuc.OrderView, error) {
	f.log.add("order:draft")
	return ordersuc.OrderView{UUID: uuid.New(), OrderNo: "SP-1", Status: "draft"}, nil
}

type fakeProducts struct{}

func (fakeProducts) GetProduct(_ context.Context, _ orgctx.Scope, id uuid.UUID) (catalogmodel.Product, error) {
	if id != productUUID {
		return catalogmodel.Product{}, catalogusecase.ErrNotFound
	}
	return catalogmodel.Product{UUID: id, Name: "PPF Film", Active: true}, nil
}

type fakeServicesW struct{ log *writeLog }

func (fakeServicesW) Get(_ context.Context, _ svcuc.Caller, id uuid.UUID) (svcuc.ServiceView, error) {
	if id != serviceUUID {
		return svcuc.ServiceView{}, svcuc.ErrNotFound
	}
	notes := "old"
	return svcuc.ServiceView{UUID: id, ServiceNo: "DSAB12CD34", Notes: &notes}, nil
}

func (f fakeServicesW) Update(_ context.Context, _ svcuc.Caller, id uuid.UUID, in svcuc.UpdateInput) (svcuc.ServiceView, error) {
	f.log.add("service:notes:" + *in.Notes.Value)
	return svcuc.ServiceView{UUID: id, ServiceNo: "DSAB12CD34"}, nil
}

func writeRegistry(log *writeLog, appointmentStatus string) *Registry {
	r := NewRegistry(nil).WithClock(func() time.Time { return time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC) }, nil)
	RegisterPanelWrite(r, WriteDeps{
		Tree: fakeTree{}, Tasks: fakeTasksW{log}, Leads: fakeLeadsW{log},
		Appointments: fakeAppointmentsW{log: log, status: appointmentStatus}, Customers: fakeCustomerGetter{},
		Orders: fakeOrdersW{log}, Products: fakeProducts{}, Services: fakeServicesW{log},
	})
	return r
}

func superAdminOf(orgType string) Principal {
	p := principal(orgType, nil)
	p.Auth.IsSuperAdmin = true
	return p
}

// validWrites is one valid input per write tool, with the principal's
// organization type.
var validWrites = []struct {
	name, orgType, input, wantWrite string
}{
	{ToolCreateTask, OrgCenter, `{"subject_organization_uuid":"` + orgUUID.String() + `","title":"Ziyaret","due_date":"2026-10-10"}`, "task:Ziyaret"},
	{ToolCreateLead, OrgDealer, `{"contact_name":"Ayşe","phone":"+905321234567","follow_up_date":"2026-10-08"}`, "lead:create"},
	{ToolSetLeadFollowUp, OrgDealer, `{"lead_uuid":"` + leadUUID.String() + `","follow_up_date":"2026-10-12"}`, "lead:follow_up:2026-10-12"},
	{ToolCreateAppointment, OrgDealer, `{"customer_uuid":"` + customerUUID.String() + `","date":"2026-10-09","time":"14:30"}`, "appointment:create:2026-10-09T11:30:00Z:assistant"},
	{ToolCancelAppointment, OrgDealer, `{"appointment_uuid":"` + appointmentUUID.String() + `","reason":"Müşteri aradı"}`, "appointment:cancelled:Müşteri aradı"},
	{ToolCreateOrderDraft, OrgDealer, `{"items":[{"product_uuid":"` + productUUID.String() + `","quantity":2}]}`, "order:draft"},
	{ToolAddServiceNote, OrgDealer, `{"service_uuid":"` + serviceUUID.String() + `","note":"Cam filmi kontrol"}`, "service:notes:old\n\nCam filmi kontrol"},
}

// TEC-387: the first write set is exactly the conservative list; there is
// no money, accounting, stock, warranty or bulk write tool.
func TestWriteToolSetIsConservative(t *testing.T) {
	r := writeRegistry(&writeLog{}, appointmentsuc.StatusScheduled)
	want := []string{ToolAddServiceNote, ToolCancelAppointment, ToolCreateAppointment, ToolCreateLead,
		ToolCreateOrderDraft, ToolCreateTask, ToolSetLeadFollowUp}
	if got := names(r.All()); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("write tools = %v, want %v", got, want)
	}
	for _, tool := range r.All() {
		spec := tool.Spec()
		if _, ok := tool.(ActionTool); !ok || spec.Kind != KindWrite || spec.Realm != RealmPanel || spec.Feature == "" {
			t.Fatalf("%s: not a gated panel write tool: %+v", spec.Name, spec)
		}
		hasWrite := false
		for _, p := range spec.Permissions {
			hasWrite = hasWrite || strings.HasSuffix(p, ".write")
		}
		if !hasWrite || spec.InputSchema["additionalProperties"] != false {
			t.Fatalf("%s: needs a write permission and a closed schema", spec.Name)
		}
		for _, banned := range []string{"payment", "income", "accounting", "stock", "warranty", "bulk", "submit"} {
			if strings.Contains(spec.Name, banned) {
				t.Fatalf("%s: out of the conservative set", spec.Name)
			}
		}
	}
	if !contains(r.All()[4].Spec().OrgTypes, OrgDealer) || contains(r.All()[4].Spec().OrgTypes, OrgCenter) {
		t.Fatalf("order drafts are for distributors and dealers only")
	}
}

// TEC-387: a write tool never runs from Call; Propose previews it without
// writing; only RunConfirmed writes, once, after the same checks.
func TestWriteToolsProposeThenRunConfirmed(t *testing.T) {
	ctx := context.Background()
	for _, tc := range validWrites {
		t.Run(tc.name, func(t *testing.T) {
			log := &writeLog{}
			r := writeRegistry(log, appointmentsuc.StatusScheduled)
			p := superAdminOf(tc.orgType)

			res, err := r.Call(ctx, p, tc.name, json.RawMessage(tc.input))
			if !errors.Is(err, ErrConfirmationRequired) || res.Code != CodeConfirmationRequired {
				t.Fatalf("Call: %+v %v", res, err)
			}
			prop, bad, err := r.Propose(ctx, p, tc.name, json.RawMessage(tc.input))
			if err != nil || bad != nil || prop == nil {
				t.Fatalf("Propose: %+v %+v %v", prop, bad, err)
			}
			if prop.Preview.Action != tc.name || prop.Preview.Summary == "" || len(prop.Preview.Fields) == 0 || len(prop.Preview.Edit) == 0 {
				t.Fatalf("preview: %+v", prop.Preview)
			}
			if err := Validate(r.byName[tc.name].Spec().InputSchema, prop.Input); err != nil {
				t.Fatalf("normalized input does not fit the schema: %s %v", prop.Input, err)
			}
			if len(log.calls) != 0 {
				t.Fatalf("writes before confirmation: %v", log.calls)
			}

			denied := principal(tc.orgType, map[string]rbac.Scope{})
			if _, err := r.RunConfirmed(ctx, denied, tc.name, prop.Input); !errors.Is(err, ErrToolNotAllowed) || len(log.calls) != 0 {
				t.Fatalf("RunConfirmed without permission: %v %v", err, log.calls)
			}

			out, err := r.RunConfirmed(ctx, p, tc.name, prop.Input)
			if err != nil || out.IsError || out.Link == nil {
				t.Fatalf("RunConfirmed: %+v %v", out, err)
			}
			if len(log.calls) != 1 || log.calls[0] != tc.wantWrite {
				t.Fatalf("writes = %v, want [%s]", log.calls, tc.wantWrite)
			}
		})
	}
}

// Propose rejects inputs the model must fix as results, without writing.
func TestWriteToolsRejectBadProposals(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, orgType, input, status, want string
	}{
		{ToolCreateTask, OrgCenter, `{"subject_organization_uuid":"` + uuid.NewString() + `","title":"x"}`, "", "subject_organization_uuid"},
		{ToolCreateTask, OrgCenter, `{"subject_organization_uuid":"` + orgUUID.String() + `","title":"x","due_date":"2026-10-01"}`, "", "today or later"},
		{ToolCreateLead, OrgDealer, `{"notes":"only notes"}`, "", "contact name, company name or phone"},
		{ToolSetLeadFollowUp, OrgDealer, `{"lead_uuid":"` + uuid.NewString() + `","follow_up_date":"2026-10-12"}`, "", "not found"},
		{ToolCreateAppointment, OrgDealer, `{"customer_uuid":"` + customerUUID.String() + `","date":"2026-10-09","time":"25:00"}`, "", "HH:MM"},
		{ToolCreateAppointment, OrgDealer, `{"customer_uuid":"` + customerUUID.String() + `","date":"2026-10-07","time":"08:00"}`, "", "future"},
		{ToolCancelAppointment, OrgDealer, `{"appointment_uuid":"` + appointmentUUID.String() + `","reason":"x"}`, appointmentsuc.StatusArrived, "only scheduled or confirmed"},
		{ToolCreateOrderDraft, OrgDealer, `{"items":[{"product_uuid":"` + productUUID.String() + `","quantity":2,"meters":1.5}]}`, "", "exactly one of quantity or meters"},
		{ToolCreateOrderDraft, OrgDealer, `{"items":[{"product_uuid":"` + uuid.NewString() + `","quantity":2}]}`, "", "not found"},
		{ToolAddServiceNote, OrgDealer, `{"service_uuid":"` + serviceUUID.String() + `","note":"   "}`, "", "note is required"},
	}
	for _, tc := range cases {
		log := &writeLog{}
		status := tc.status
		if status == "" {
			status = appointmentsuc.StatusScheduled
		}
		r := writeRegistry(log, status)
		prop, bad, err := r.Propose(ctx, superAdminOf(tc.orgType), tc.name, json.RawMessage(tc.input))
		if err != nil || prop != nil || bad == nil || !bad.IsError || !strings.Contains(bad.Content, tc.want) {
			t.Fatalf("%s %s: want an error result containing %q, got %+v %+v %v", tc.name, tc.input, tc.want, prop, bad, err)
		}
		if len(log.calls) != 0 {
			t.Fatalf("%s: wrote %v", tc.name, log.calls)
		}
	}
	// Organization type gate: a center cannot draft orders, a dealer cannot
	// create center tasks.
	r := writeRegistry(&writeLog{}, appointmentsuc.StatusScheduled)
	for name, orgType := range map[string]string{ToolCreateOrderDraft: OrgCenter, ToolCreateTask: OrgDealer} {
		if _, bad, err := r.Propose(ctx, superAdminOf(orgType), name, json.RawMessage(`{}`)); !errors.Is(err, ErrToolNotAllowed) || bad.Code != CodeToolNotAllowed {
			t.Fatalf("%s as %s: %+v %v", name, orgType, bad, err)
		}
	}
}
