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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
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

// TEC-461: the card summary is written in the principal's locale for every
// write tool and every catalog locale; an unknown or missing locale is en.
func TestWriteCardSummaryLocalized(t *testing.T) {
	ctx := context.Background()
	summary := func(t *testing.T, name, input string, locale i18n.Locale) Preview {
		t.Helper()
		p := superAdminOf(OrgCenter)
		for _, tc := range validWrites {
			if tc.name == name {
				p = superAdminOf(tc.orgType)
			}
		}
		p.Locale = locale
		prop, bad, err := writeRegistry(&writeLog{}, appointmentsuc.StatusScheduled).Propose(ctx, p, name, json.RawMessage(input))
		if err != nil || bad != nil {
			t.Fatalf("%s/%s: %+v %v", name, locale, bad, err)
		}
		return prop.Preview
	}
	for _, tc := range validWrites {
		en := summary(t, tc.name, tc.input, i18n.LocaleEN).Summary
		for _, l := range i18n.Supported {
			pv := summary(t, tc.name, tc.input, l)
			if pv.Summary == "" || strings.Contains(pv.Summary, "{{") || len(pv.SummaryArgs) == 0 {
				t.Errorf("%s/%s: summary %q args %v", tc.name, l, pv.Summary, pv.SummaryArgs)
			}
			if l != i18n.LocaleEN && pv.Summary == en {
				t.Errorf("%s/%s: summary is the English one: %q", tc.name, l, en)
			}
		}
		for _, l := range []i18n.Locale{"", "ja", "pt-BR"} {
			if got := summary(t, tc.name, tc.input, l).Summary; got != en {
				t.Errorf("%s/%q: %q, want en %q", tc.name, l, got, en)
			}
		}
	}

	task := validWrites[0]
	want := map[i18n.Locale]string{
		i18n.LocaleTR: `Bayi A için "Ziyaret" görevi oluşturulsun.`,
		i18n.LocaleAR: `إنشاء المهمة "Ziyaret" بخصوص Bayi A.`,
		i18n.LocaleEN: `Create the task "Ziyaret" about Bayi A.`,
	}
	for l, w := range want {
		pv := summary(t, task.name, task.input, l)
		if pv.Summary != w {
			t.Errorf("%s: %q, want %q", l, pv.Summary, w)
		}
		// A channel can render the stored card in another language.
		if got := pv.LocalizedSummary(i18n.LocaleTR); got != want[i18n.LocaleTR] {
			t.Errorf("re-render from %s: %q", l, got)
		}
	}
}

// TEC-648: enum values on the card (task priority, lead target type,
// source and temperature) and the select options are shown in the
// principal's locale; the raw value stays in Code and in the input.
func TestWriteCardEnumValuesLocalized(t *testing.T) {
	ctx := context.Background()
	propose := func(t *testing.T, name, orgType, input string, locale i18n.Locale) Proposal {
		t.Helper()
		p := superAdminOf(orgType)
		p.Locale = locale
		prop, bad, err := writeRegistry(&writeLog{}, appointmentsuc.StatusScheduled).Propose(ctx, p, name, json.RawMessage(input))
		if err != nil || bad != nil {
			t.Fatalf("%s/%s: %+v %v", name, locale, bad, err)
		}
		return *prop
	}
	fields := func(pv Preview) map[string]Field {
		m := map[string]Field{}
		for _, f := range pv.Fields {
			m[f.Key] = f
		}
		return m
	}
	task := `{"subject_organization_uuid":"` + orgUUID.String() + `","title":"Ziyaret","priority":"low"}`
	lead := validWrites[1].input
	cases := []struct {
		name, orgType, input string
		want                 map[i18n.Locale]map[string]string
	}{
		{ToolCreateTask, OrgCenter, task, map[i18n.Locale]map[string]string{
			i18n.LocaleTR: {"priority": "Düşük"},
			i18n.LocaleEN: {"priority": "Low"},
		}},
		{ToolCreateLead, OrgDealer, lead, map[i18n.Locale]map[string]string{
			i18n.LocaleTR: {"target_type": "Müşteri", "source": "Diğer", "temperature": "Soğuk"},
			i18n.LocaleEN: {"target_type": "Customer", "source": "Other", "temperature": "Cold"},
		}},
	}
	codes := map[string]string{"priority": "low", "target_type": "customer", "source": "other", "temperature": "cold"}
	for _, tc := range cases {
		for l, want := range tc.want {
			prop := propose(t, tc.name, tc.orgType, tc.input, l)
			got := fields(prop.Preview)
			for k, v := range want {
				if got[k].Value != v || got[k].Code != codes[k] {
					t.Errorf("%s/%s %s: %+v, want %q (code %q)", tc.name, l, k, got[k], v, codes[k])
				}
				// A channel can render the stored card in another language.
				if r := got[k].LocalizedValue(i18n.LocaleTR); r != tc.want[i18n.LocaleTR][k] {
					t.Errorf("%s/%s %s re-render: %q", tc.name, l, k, r)
				}
				if !strings.Contains(string(prop.Input), `"`+codes[k]+`"`) {
					t.Errorf("%s/%s: input lost the raw %s: %s", tc.name, l, k, prop.Input)
				}
			}
			for _, e := range prop.Preview.Edit {
				if e.Type != EditSelect {
					continue
				}
				for _, o := range e.Options {
					if lbl := e.OptionLabels[o]; lbl == "" || (l == i18n.LocaleTR && lbl == o) {
						t.Errorf("%s/%s %s option %q label %q", tc.name, l, e.Key, o, lbl)
					}
				}
			}
		}
		// Every catalog locale has a label for the enum values.
		for _, l := range i18n.Supported {
			for k, f := range fields(propose(t, tc.name, tc.orgType, tc.input, l).Preview) {
				if f.Code != "" && f.Value == f.Code {
					t.Errorf("%s/%s %s: raw value %q", tc.name, l, k, f.Value)
				}
			}
		}
	}
	// Non-enum fields keep their value and carry no code.
	for _, tc := range validWrites {
		for _, f := range propose(t, tc.name, tc.orgType, tc.input, i18n.LocaleTR).Preview.Fields {
			if _, enum := enumValueKeys[f.Key]; !enum && f.Code != "" {
				t.Errorf("%s %s: unexpected code %q", tc.name, f.Key, f.Code)
			}
		}
	}
}

// TEC-648: applied parts carry the localized part name next to the key.
func TestAppliedPartsLabeled(t *testing.T) {
	got := appliedParts(i18n.LocaleTR, []string{"body_kaput", "window_on_cam", "unknown_part"})
	want := []appliedPart{{"body_kaput", "Kaput"}, {"window_on_cam", "Ön cam"}, {"unknown_part", "unknown_part"}}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if en := appliedParts(i18n.LocaleEN, []string{"body_kaput"}); en[0].Label != "Hood" {
		t.Errorf("en: %+v", en)
	}
	if appliedParts(i18n.LocaleTR, nil) != nil {
		t.Error("no parts must stay omitted")
	}
}
