package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	appointmentsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	catalogmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	customersuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	leadsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	ordersuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	tasksuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// The first, conservative write tool set (TEC-387, F4-01e): center task,
// lead and its follow-up date, appointment create / cancel, order draft
// (never submitted) and a service note. There is deliberately no tool that
// moves money, posts accounting, moves stock, decides a warranty or works
// in bulk.

// Write tool names.
const (
	ToolCreateTask        = "create_task"
	ToolCreateLead        = "create_lead"
	ToolSetLeadFollowUp   = "set_lead_follow_up"
	ToolCreateAppointment = "create_appointment"
	ToolCancelAppointment = "cancel_appointment"
	ToolCreateOrderDraft  = "create_order_draft"
	ToolAddServiceNote    = "add_service_note"
)

// Use case surfaces of the write tools.
type (
	TasksWriter interface {
		Subject(ctx context.Context, c tasksuc.Caller, id uuid.UUID) (tasksuc.SubjectRef, error)
		Create(ctx context.Context, c tasksuc.Caller, in tasksuc.CreateInput) (tasksuc.Task, error)
	}
	LeadsWriter interface {
		Get(ctx context.Context, c leadsuc.Caller, id uuid.UUID) (leadsuc.Lead, error)
		Create(ctx context.Context, c leadsuc.Caller, in leadsuc.CreateInput) (leadsuc.Lead, error)
		Patch(ctx context.Context, c leadsuc.Caller, id uuid.UUID, in leadsuc.PatchInput) (leadsuc.Lead, error)
	}
	AppointmentsWriter interface {
		Get(ctx context.Context, c appointmentsuc.Caller, id uuid.UUID) (appointmentsuc.Appointment, error)
		Create(ctx context.Context, c appointmentsuc.Caller, in appointmentsuc.CreateInput) (appointmentsuc.Appointment, error)
		SetStatus(ctx context.Context, c appointmentsuc.Caller, id uuid.UUID, in appointmentsuc.StatusInput) (appointmentsuc.Appointment, error)
	}
	CustomerGetter interface {
		GetCustomer(ctx context.Context, c customersuc.Caller, id uuid.UUID) (customersuc.CustomerDetail, error)
	}
	OrdersWriter interface {
		Create(ctx context.Context, c ordersuc.Caller, in ordersuc.CreateInput) (ordersuc.OrderView, error)
	}
	ProductGetter interface {
		GetProduct(ctx context.Context, org orgctx.Scope, id uuid.UUID) (catalogmodel.Product, error)
	}
	ServicesWriter interface {
		Get(ctx context.Context, c svcuc.Caller, id uuid.UUID) (svcuc.ServiceView, error)
		Update(ctx context.Context, c svcuc.Caller, id uuid.UUID, in svcuc.UpdateInput) (svcuc.ServiceView, error)
	}
)

// WriteDeps wires the write tools; a nil dependency leaves its tools out.
type WriteDeps struct {
	Tree         scopefilter.TreeReader
	Tasks        TasksWriter
	Leads        LeadsWriter
	Appointments AppointmentsWriter
	Customers    CustomerGetter
	Orders       OrdersWriter
	Products     ProductGetter
	Services     ServicesWriter
}

// RegisterPanelWrite adds the panel write tools whose use cases are wired.
func RegisterPanelWrite(r *Registry, d WriteDeps) {
	if d.Tasks != nil {
		r.Register(CreateTask{tasks: d.Tasks})
	}
	if d.Leads != nil {
		r.Register(CreateLead{leads: d.Leads, tree: d.Tree})
		r.Register(SetLeadFollowUp{leads: d.Leads, tree: d.Tree})
	}
	if d.Appointments != nil && d.Customers != nil {
		r.Register(CreateAppointment{appointments: d.Appointments, customers: d.Customers, tree: d.Tree})
	}
	if d.Appointments != nil {
		r.Register(CancelAppointment{appointments: d.Appointments, tree: d.Tree})
	}
	if d.Orders != nil && d.Products != nil {
		r.Register(CreateOrderDraft{orders: d.Orders, products: d.Products, tree: d.Tree})
	}
	if d.Services != nil {
		r.Register(AddServiceNote{services: d.Services, tree: d.Tree})
	}
}

// proposal builds a proposal from the normalized input.
func proposal(in any, pv Preview) (Proposal, *Result, error) {
	raw, err := marshalInput(in)
	if err != nil {
		return Proposal{}, nil, err
	}
	return Proposal{Input: raw, Preview: pv}, nil, nil
}

// rejected turns a use case error into a model result for Propose.
func rejected(errs errCases, err error) (Proposal, *Result, error) {
	res, err := errs.result(err)
	if err != nil {
		return Proposal{}, nil, err
	}
	return Proposal{}, &res, nil
}

func badInput(msg string) *Result {
	r := ErrorResult(CodeInvalidInput, "invalid input: "+msg+". Fix the arguments (or ask the user) and call the tool again.")
	return &r
}

// done is the result of a confirmed write.
func done(msg, kind string, id uuid.UUID) (Result, error) {
	res, err := JSONResult(map[string]any{"ok": true, "message": msg, kind + "_uuid": id})
	if err != nil {
		return Result{}, err
	}
	res.Link = &Link{Kind: kind, UUID: id.String()}
	return res, nil
}

// futureDate parses a YYYY-MM-DD date that must not be in the past.
func futureDate(env Env, v string) (time.Time, error) {
	loc := env.Loc()
	d, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(v), loc)
	if err != nil {
		return time.Time{}, errors.New("date must be YYYY-MM-DD")
	}
	now := env.Now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if d.Before(today) {
		return time.Time{}, errors.New("date must be today or later")
	}
	if d.After(today.AddDate(2, 0, 0)) {
		return time.Time{}, errors.New("date must be within two years")
	}
	return d, nil
}

func fieldIf(fields []Field, key, value string) []Field {
	if strings.TrimSpace(value) == "" {
		return fields
	}
	return append(fields, Field{Key: key, Value: value})
}

func strPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// --- create_task -------------------------------------------------------------

// taskDueHour is the local due time of a task created with a due date.
const taskDueHour = 18

// CreateTask: merkez görevi oluştur (konu bayi / distribütör, kullanıcıya atanır).
type CreateTask struct{ tasks TasksWriter }

type createTaskInput struct {
	SubjectOrganizationUUID string `json:"subject_organization_uuid"`
	Title                   string `json:"title"`
	Description             string `json:"description,omitempty"`
	Priority                string `json:"priority,omitempty"`
	DueDate                 string `json:"due_date,omitempty"`
}

// Spec implements Tool.
func (CreateTask) Spec() Spec {
	return Spec{
		Name: ToolCreateTask,
		Description: "Propose a center task about a distributor or dealer, assigned to the user. The user confirms it " +
			"on a card before anything is saved. Get the organization uuid from list_sub_organizations.",
		InputSchema: object(map[string]any{
			"subject_organization_uuid": map[string]any{"type": "string", "format": "uuid", "description": "Distributor or dealer the task is about."},
			"title":                     strMin("Short task title.", 1, 200),
			"description":               str("Optional details.", 2000),
			"priority":                  enum("Default normal.", tasksuc.PriorityLow, tasksuc.PriorityNormal, tasksuc.PriorityHigh, tasksuc.PriorityUrgent),
			"due_date":                  date("Optional due date, YYYY-MM-DD (today or later)."),
		}, "subject_organization_uuid", "title"),
		Kind: KindWrite, Realm: RealmPanel, OrgTypes: []string{OrgCenter}, Feature: features.ModuleTasks,
		Permissions: []string{rbac.PermTasksRead, rbac.PermTasksWrite},
	}
}

func (t CreateTask) errs() errCases {
	return errCases{tool: ToolCreateTask, what: "organization", forbidden: []error{tasksuc.ErrCenterOnly},
		notFound: []error{tasksuc.ErrNotFound}, invalid: asError[*tasksuc.ValidationError]}
}

// Propose implements ActionTool.
func (t CreateTask) Propose(ctx context.Context, env Env, raw json.RawMessage) (Proposal, *Result, error) {
	var in createTaskInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return Proposal{}, r, nil
	}
	in.Title, in.Description = trimmed(in.Title, 200), trimmed(in.Description, 2000)
	if in.Title == "" {
		return Proposal{}, badInput("title is required"), nil
	}
	if in.Priority == "" {
		in.Priority = tasksuc.PriorityNormal
	}
	if in.DueDate != "" {
		if _, err := futureDate(env, in.DueDate); err != nil {
			return Proposal{}, badInput(err.Error()), nil
		}
	}
	id, _ := parseID(in.SubjectOrganizationUUID)
	p := env.Principal
	subject, err := t.tasks.Subject(ctx, tasksuc.Caller{UserID: p.Auth.UserInternal, Org: *p.Org}, id)
	if err != nil {
		return rejected(t.errs(), err)
	}
	fields := []Field{{Key: "subject_organization", Value: subject.Name}, {Key: "title", Value: in.Title}}
	fields = fieldIf(fields, "description", in.Description)
	fields = append(fields, Field{Key: "priority", Value: in.Priority})
	fields = fieldIf(fields, "due_date", in.DueDate)
	return proposal(in, Preview{
		SummaryArgs: map[string]string{"title": in.Title, "subject": subject.Name},
		Fields:      fields,
		Edit: []EditField{
			{Key: "title", Type: EditText, Value: in.Title, Required: true},
			{Key: "description", Type: EditTextarea, Value: in.Description},
			{Key: "priority", Type: EditSelect, Value: in.Priority, Required: true,
				Options: []string{tasksuc.PriorityLow, tasksuc.PriorityNormal, tasksuc.PriorityHigh, tasksuc.PriorityUrgent}},
			{Key: "due_date", Type: EditDate, Value: in.DueDate},
		},
	})
}

// Run implements Tool.
func (t CreateTask) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in createTaskInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	id, _ := parseID(in.SubjectOrganizationUUID)
	p := env.Principal
	me := p.Auth.UserID
	ci := tasksuc.CreateInput{SubjectOrgUUID: id, Title: in.Title, Description: in.Description,
		AssigneeUUID: &me, Priority: in.Priority}
	if in.DueDate != "" {
		d, err := futureDate(env, in.DueDate)
		if err != nil {
			return *badInput(err.Error()), nil
		}
		due := d.Add(taskDueHour * time.Hour)
		ci.DueAt = &due
	}
	task, err := t.tasks.Create(ctx, tasksuc.Caller{UserID: p.Auth.UserInternal, Org: *p.Org}, ci)
	if err != nil {
		return t.errs().result(err)
	}
	return done("Task created.", "task", task.UUID)
}

// --- create_lead -------------------------------------------------------------

// CreateLead: lead oluştur.
type CreateLead struct {
	leads LeadsWriter
	tree  scopefilter.TreeReader
}

type createLeadInput struct {
	TargetType   string `json:"target_type,omitempty"`
	ContactName  string `json:"contact_name,omitempty"`
	CompanyName  string `json:"company_name,omitempty"`
	Phone        string `json:"phone,omitempty"`
	Email        string `json:"email,omitempty"`
	Source       string `json:"source,omitempty"`
	Temperature  string `json:"temperature,omitempty"`
	FollowUpDate string `json:"follow_up_date,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

// Spec implements Tool.
func (CreateLead) Spec() Spec {
	return Spec{
		Name: ToolCreateLead,
		Description: "Propose a new sales lead (potential customer, dealer or distributor) for the user's organization. " +
			"Needs a contact name, company name or phone. The user confirms it on a card before anything is saved.",
		InputSchema: object(map[string]any{
			"target_type":    enum("Default customer.", leadsuc.LeadTargetTypes...),
			"contact_name":   str("Contact person.", 200),
			"company_name":   str("Company.", 200),
			"phone":          str("Phone in E.164 format, e.g. +905321234567.", 16),
			"email":          str("E-mail.", 254),
			"source":         enum("Where the lead came from. Default other.", leadsuc.LeadSources...),
			"temperature":    enum("Default cold.", leadsuc.LeadTemperatures...),
			"follow_up_date": date("Optional follow-up date, YYYY-MM-DD (today or later)."),
			"notes":          str("Optional notes.", 2000),
		}),
		Kind: KindWrite, Realm: RealmPanel, Feature: features.ModuleLeads,
		Permissions: []string{rbac.PermLeadsRead, rbac.PermLeadsWrite},
	}
}

func leadErrs(tool string) errCases {
	return errCases{tool: tool, what: "lead", forbidden: []error{leadsuc.ErrForbidden},
		notFound: []error{leadsuc.ErrNotFound}, invalid: asError[*leadsuc.ValidationError]}
}

// Propose implements ActionTool.
func (t CreateLead) Propose(_ context.Context, env Env, raw json.RawMessage) (Proposal, *Result, error) {
	var in createLeadInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return Proposal{}, r, nil
	}
	in.ContactName, in.CompanyName = trimmed(in.ContactName, 200), trimmed(in.CompanyName, 200)
	in.Phone, in.Email, in.Notes = strings.ReplaceAll(strings.TrimSpace(in.Phone), " ", ""), strings.TrimSpace(in.Email), trimmed(in.Notes, 2000)
	if in.ContactName == "" && in.CompanyName == "" && in.Phone == "" {
		return Proposal{}, badInput("a contact name, company name or phone is required"), nil
	}
	if in.TargetType == "" {
		in.TargetType = "customer"
	}
	if in.Source == "" {
		in.Source = "other"
	}
	if in.Temperature == "" {
		in.Temperature = "cold"
	}
	if in.FollowUpDate != "" {
		if _, err := futureDate(env, in.FollowUpDate); err != nil {
			return Proposal{}, badInput(err.Error()), nil
		}
	}
	name := in.ContactName
	if in.CompanyName != "" {
		name = strings.TrimSpace(in.CompanyName + " " + in.ContactName)
	}
	if name == "" {
		name = in.Phone
	}
	fields := []Field{{Key: "target_type", Value: in.TargetType}}
	fields = fieldIf(fields, "contact_name", in.ContactName)
	fields = fieldIf(fields, "company_name", in.CompanyName)
	fields = fieldIf(fields, "phone", in.Phone)
	fields = fieldIf(fields, "email", in.Email)
	fields = append(fields, Field{Key: "source", Value: in.Source}, Field{Key: "temperature", Value: in.Temperature})
	fields = fieldIf(fields, "follow_up_date", in.FollowUpDate)
	fields = fieldIf(fields, "notes", in.Notes)
	return proposal(in, Preview{
		SummaryArgs: map[string]string{"name": name},
		Fields:      fields,
		Edit: []EditField{
			{Key: "contact_name", Type: EditText, Value: in.ContactName},
			{Key: "company_name", Type: EditText, Value: in.CompanyName},
			{Key: "phone", Type: EditText, Value: in.Phone},
			{Key: "temperature", Type: EditSelect, Value: in.Temperature, Options: leadsuc.LeadTemperatures, Required: true},
			{Key: "follow_up_date", Type: EditDate, Value: in.FollowUpDate},
			{Key: "notes", Type: EditTextarea, Value: in.Notes},
		},
	})
}

// Run implements Tool.
func (t CreateLead) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in createLeadInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := leadErrs(ToolCreateLead)
	p := env.Principal
	f, err := resolveScope(ctx, t.tree, p, rbac.PermLeadsRead)
	if err != nil {
		return errs.result(err)
	}
	ci := leadsuc.CreateInput{
		TargetType: in.TargetType, CandidateContactName: strPtr(in.ContactName), CandidateCompanyName: strPtr(in.CompanyName),
		CandidatePhoneE164: strPtr(in.Phone), CandidateEmail: strPtr(in.Email), Source: in.Source,
		Temperature: in.Temperature, Notes: strPtr(in.Notes),
	}
	if in.FollowUpDate != "" {
		d, err := futureDate(env, in.FollowUpDate)
		if err != nil {
			return *badInput(err.Error()), nil
		}
		ci.FollowUpDate = &d
	}
	lead, err := t.leads.Create(ctx, leadsuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}, ci)
	if err != nil {
		return errs.result(err)
	}
	return done("Lead created.", "lead", lead.UUID)
}

// --- set_lead_follow_up ---------------------------------------------------------

// SetLeadFollowUp: lead takip tarihini güncelle.
type SetLeadFollowUp struct {
	leads LeadsWriter
	tree  scopefilter.TreeReader
}

type setFollowUpInput struct {
	LeadUUID     string `json:"lead_uuid"`
	FollowUpDate string `json:"follow_up_date"`
}

// Spec implements Tool.
func (SetLeadFollowUp) Spec() Spec {
	return Spec{
		Name: ToolSetLeadFollowUp,
		Description: "Propose a new follow-up date for a lead within your access (uuid from list_leads). The user " +
			"confirms it on a card before anything is saved.",
		InputSchema: object(map[string]any{
			"lead_uuid":      map[string]any{"type": "string", "format": "uuid", "description": "Lead uuid."},
			"follow_up_date": date("New follow-up date, YYYY-MM-DD (today or later)."),
		}, "lead_uuid", "follow_up_date"),
		Kind: KindWrite, Realm: RealmPanel, Feature: features.ModuleLeads,
		Permissions: []string{rbac.PermLeadsRead, rbac.PermLeadsWrite},
	}
}

func (t SetLeadFollowUp) lead(ctx context.Context, p Principal, id uuid.UUID) (leadsuc.Lead, leadsuc.Caller, error) {
	f, err := resolveScope(ctx, t.tree, p, rbac.PermLeadsRead)
	if err != nil {
		return leadsuc.Lead{}, leadsuc.Caller{}, err
	}
	c := leadsuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}
	l, err := t.leads.Get(ctx, c, id)
	return l, c, err
}

// Propose implements ActionTool.
func (t SetLeadFollowUp) Propose(ctx context.Context, env Env, raw json.RawMessage) (Proposal, *Result, error) {
	var in setFollowUpInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return Proposal{}, r, nil
	}
	if _, err := futureDate(env, in.FollowUpDate); err != nil {
		return Proposal{}, badInput(err.Error()), nil
	}
	id, _ := parseID(in.LeadUUID)
	l, _, err := t.lead(ctx, env.Principal, id)
	if err != nil {
		return rejected(leadErrs(ToolSetLeadFollowUp), err)
	}
	name := leadName(l)
	current := ""
	if l.FollowUpDate != nil {
		current = l.FollowUpDate.In(env.Loc()).Format(time.DateOnly)
	}
	fields := []Field{{Key: "lead", Value: name}}
	fields = fieldIf(fields, "current_follow_up_date", current)
	fields = append(fields, Field{Key: "follow_up_date", Value: in.FollowUpDate})
	return proposal(in, Preview{
		SummaryArgs: map[string]string{"name": name, "date": in.FollowUpDate},
		Fields:      fields,
		Edit:        []EditField{{Key: "follow_up_date", Type: EditDate, Value: in.FollowUpDate, Required: true}},
	})
}

// Run implements Tool.
func (t SetLeadFollowUp) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in setFollowUpInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	d, err := futureDate(env, in.FollowUpDate)
	if err != nil {
		return *badInput(err.Error()), nil
	}
	errs := leadErrs(ToolSetLeadFollowUp)
	id, _ := parseID(in.LeadUUID)
	_, c, err := t.lead(ctx, env.Principal, id)
	if err != nil {
		return errs.result(err)
	}
	l, err := t.leads.Patch(ctx, c, id, leadsuc.PatchInput{FollowUpDate: leadsuc.Field[time.Time]{Set: true, Value: &d}})
	if err != nil {
		return errs.result(err)
	}
	return done("Follow-up date updated.", "lead", l.UUID)
}

func leadName(l leadsuc.Lead) string {
	var parts []string
	for _, p := range []*string{l.CandidateCompanyName, l.CandidateContactName} {
		if p != nil && strings.TrimSpace(*p) != "" {
			parts = append(parts, strings.TrimSpace(*p))
		}
	}
	if len(parts) == 0 && l.CandidatePhoneE164 != nil {
		parts = append(parts, *l.CandidatePhoneE164)
	}
	return dataText(strings.Join(parts, " / "), maxNameChars)
}

// --- create_appointment ---------------------------------------------------------

// CreateAppointment: randevu oluştur.
type CreateAppointment struct {
	appointments AppointmentsWriter
	customers    CustomerGetter
	tree         scopefilter.TreeReader
}

type createAppointmentInput struct {
	CustomerUUID     string `json:"customer_uuid"`
	Date             string `json:"date"`
	Time             string `json:"time"`
	EstimatedMinutes int    `json:"estimated_minutes,omitempty"`
	Note             string `json:"note,omitempty"`
}

// Spec implements Tool.
func (CreateAppointment) Spec() Spec {
	return Spec{
		Name: ToolCreateAppointment,
		Description: "Propose a service appointment for a customer of your organization (uuid from search_customers). " +
			"The user confirms it on a card before anything is saved.",
		InputSchema: object(map[string]any{
			"customer_uuid":     map[string]any{"type": "string", "format": "uuid", "description": "Customer uuid."},
			"date":              date("Appointment day, YYYY-MM-DD (today or later)."),
			"time":              strMin("Start time HH:MM (24h, business time zone).", 5, 5),
			"estimated_minutes": map[string]any{"type": "integer", "minimum": 15, "maximum": 1440, "description": "Optional duration in minutes."},
			"note":              str("Optional note.", 1000),
		}, "customer_uuid", "date", "time"),
		Kind: KindWrite, Realm: RealmPanel, Feature: features.ModuleAppointments,
		Permissions: []string{rbac.PermAppointmentsRead, rbac.PermAppointmentsWrite, rbac.PermCustomersRead},
	}
}

func appointmentErrs(tool string) errCases {
	return errCases{tool: tool, what: "appointment", forbidden: []error{appointmentsuc.ErrForbidden},
		notFound: []error{appointmentsuc.ErrNotFound}, invalid: func(err error) (string, bool) {
			switch {
			case errors.Is(err, appointmentsuc.ErrCapacityFull):
				return "the day is fully booked; propose another day or time", true
			case errors.Is(err, appointmentsuc.ErrDayClosed):
				return "the business is closed that day; propose another day", true
			case errors.Is(err, appointmentsuc.ErrInvalidTransition):
				return "the appointment can no longer be changed in its current status", true
			}
			return asError[*appointmentsuc.ValidationError](err)
		}}
}

func (t CreateAppointment) startsAt(env Env, in createAppointmentInput) (time.Time, error) {
	d, err := futureDate(env, in.Date)
	if err != nil {
		return time.Time{}, err
	}
	hm, err := time.Parse("15:04", strings.TrimSpace(in.Time))
	if err != nil {
		return time.Time{}, errors.New("time must be HH:MM")
	}
	at := d.Add(time.Duration(hm.Hour())*time.Hour + time.Duration(hm.Minute())*time.Minute)
	if at.Before(env.Now) {
		return time.Time{}, errors.New("the appointment must start in the future")
	}
	return at, nil
}

// Propose implements ActionTool.
func (t CreateAppointment) Propose(ctx context.Context, env Env, raw json.RawMessage) (Proposal, *Result, error) {
	var in createAppointmentInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return Proposal{}, r, nil
	}
	in.Note = trimmed(in.Note, 1000)
	if _, err := t.startsAt(env, in); err != nil {
		return Proposal{}, badInput(err.Error()), nil
	}
	p := env.Principal
	errs := errCases{tool: ToolCreateAppointment, what: "customer", notFound: []error{customersuc.ErrCustomerNotFound},
		forbidden: []error{customersuc.ErrForbidden}, invalid: asError[*customersuc.ValidationError]}
	f, err := resolveScope(ctx, t.tree, p, rbac.PermCustomersRead)
	if err != nil {
		return rejected(errs, err)
	}
	id, _ := parseID(in.CustomerUUID)
	cust, err := t.customers.GetCustomer(ctx, customersuc.Caller{UserID: p.Auth.UserInternal, Org: *p.Org, Filter: f,
		Locale: i18n.FromContext(ctx).Locale}, id)
	if err != nil {
		return rejected(errs, err)
	}
	name := dataText(strings.TrimSpace(cust.Name+" "+cust.Surname), maxNameChars)
	fields := []Field{{Key: "customer", Value: name}, {Key: "date", Value: in.Date}, {Key: "time", Value: in.Time}}
	if in.EstimatedMinutes > 0 {
		fields = append(fields, Field{Key: "estimated_minutes", Value: strconv.Itoa(in.EstimatedMinutes)})
	}
	fields = fieldIf(fields, "note", in.Note)
	minutes := ""
	if in.EstimatedMinutes > 0 {
		minutes = strconv.Itoa(in.EstimatedMinutes)
	}
	return proposal(in, Preview{
		SummaryArgs: map[string]string{"name": name, "date": in.Date, "time": in.Time},
		Fields:      fields,
		Edit: []EditField{
			{Key: "date", Type: EditDate, Value: in.Date, Required: true},
			{Key: "time", Type: EditTime, Value: in.Time, Required: true},
			{Key: "estimated_minutes", Type: EditNumber, Value: minutes},
			{Key: "note", Type: EditTextarea, Value: in.Note},
		},
	})
}

// Run implements Tool.
func (t CreateAppointment) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in createAppointmentInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	at, err := t.startsAt(env, in)
	if err != nil {
		return *badInput(err.Error()), nil
	}
	errs := appointmentErrs(ToolCreateAppointment)
	p := env.Principal
	f, err := resolveScope(ctx, t.tree, p, rbac.PermAppointmentsRead)
	if err != nil {
		return errs.result(err)
	}
	id, _ := parseID(in.CustomerUUID)
	ci := appointmentsuc.CreateInput{CustomerUUID: &id, StartsAt: at, Source: "assistant", Note: in.Note}
	if in.EstimatedMinutes > 0 {
		m := int32(in.EstimatedMinutes) //nolint:gosec // schema caps it at 1440
		ci.EstimatedMinutes = &m
	}
	a, err := t.appointments.Create(ctx, appointmentsuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}, ci)
	if err != nil {
		return errs.result(err)
	}
	return done("Appointment created.", "appointment", a.UUID)
}

// --- cancel_appointment ---------------------------------------------------------

// CancelAppointment: randevu iptal et.
type CancelAppointment struct {
	appointments AppointmentsWriter
	tree         scopefilter.TreeReader
}

type cancelAppointmentInput struct {
	AppointmentUUID string `json:"appointment_uuid"`
	Reason          string `json:"reason"`
}

// Spec implements Tool.
func (CancelAppointment) Spec() Spec {
	return Spec{
		Name: ToolCancelAppointment,
		Description: "Propose cancelling a scheduled or confirmed appointment within your access (uuid from " +
			"list_appointments) with a reason. The user confirms it on a card before anything changes.",
		InputSchema: object(map[string]any{
			"appointment_uuid": map[string]any{"type": "string", "format": "uuid", "description": "Appointment uuid."},
			"reason":           strMin("Cancellation reason.", 1, 500),
		}, "appointment_uuid", "reason"),
		Kind: KindWrite, Realm: RealmPanel, Feature: features.ModuleAppointments,
		Permissions: []string{rbac.PermAppointmentsRead, rbac.PermAppointmentsWrite},
	}
}

func (t CancelAppointment) caller(ctx context.Context, p Principal) (appointmentsuc.Caller, error) {
	f, err := resolveScope(ctx, t.tree, p, rbac.PermAppointmentsRead)
	return appointmentsuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}, err
}

// Propose implements ActionTool.
func (t CancelAppointment) Propose(ctx context.Context, env Env, raw json.RawMessage) (Proposal, *Result, error) {
	var in cancelAppointmentInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return Proposal{}, r, nil
	}
	in.Reason = trimmed(in.Reason, 500)
	if in.Reason == "" {
		return Proposal{}, badInput("reason is required"), nil
	}
	errs := appointmentErrs(ToolCancelAppointment)
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return rejected(errs, err)
	}
	id, _ := parseID(in.AppointmentUUID)
	a, err := t.appointments.Get(ctx, c, id)
	if err != nil {
		return rejected(errs, err)
	}
	if a.Status != appointmentsuc.StatusScheduled && a.Status != appointmentsuc.StatusConfirmed {
		return Proposal{}, badInput("only scheduled or confirmed appointments can be cancelled (status: " + a.Status + ")"), nil
	}
	starts := a.StartsAt.In(env.Loc()).Format("2006-01-02 15:04")
	customer := dataText(a.CustomerName, maxNameChars)
	fields := []Field{{Key: "customer", Value: customer}, {Key: "starts_at", Value: starts}}
	if a.VehiclePlate != nil {
		fields = append(fields, Field{Key: "plate", Value: *a.VehiclePlate})
	}
	fields = append(fields, Field{Key: "reason", Value: in.Reason})
	return proposal(in, Preview{
		SummaryArgs: map[string]string{"name": customer, "starts_at": starts},
		Fields:      fields,
		Edit:        []EditField{{Key: "reason", Type: EditTextarea, Value: in.Reason, Required: true}},
	})
}

// Run implements Tool.
func (t CancelAppointment) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in cancelAppointmentInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := appointmentErrs(ToolCancelAppointment)
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return errs.result(err)
	}
	id, _ := parseID(in.AppointmentUUID)
	reason := strings.TrimSpace(in.Reason)
	a, err := t.appointments.SetStatus(ctx, c, id, appointmentsuc.StatusInput{Status: appointmentsuc.StatusCancelled, CancelReason: &reason})
	if err != nil {
		return errs.result(err)
	}
	return done("Appointment cancelled.", "appointment", a.UUID)
}

// --- create_order_draft ---------------------------------------------------------

// maxOrderDraftItems caps the lines of an assistant order draft.
const maxOrderDraftItems = 30

// CreateOrderDraft: sipariş taslağı oluştur (gönderilmez).
type CreateOrderDraft struct {
	orders   OrdersWriter
	products ProductGetter
	tree     scopefilter.TreeReader
}

type orderDraftItem struct {
	ProductUUID string   `json:"product_uuid"`
	Quantity    int64    `json:"quantity,omitempty"`
	Meters      *float64 `json:"meters,omitempty"`
}

type orderDraftInput struct {
	Items []orderDraftItem `json:"items"`
	Note  string           `json:"note,omitempty"`
}

// Spec implements Tool.
func (CreateOrderDraft) Spec() Spec {
	return Spec{
		Name: ToolCreateOrderDraft,
		Description: "Propose a DRAFT order of your organization to its supplier (product uuids from search_products; " +
			"quantity for piece products, meters for roll products). The draft is never submitted: the user reviews " +
			"and submits it in the orders screen. The user confirms the draft on a card before it is saved.",
		InputSchema: object(map[string]any{
			"items": map[string]any{"type": "array", "minItems": 1, "maxItems": maxOrderDraftItems, "items": object(map[string]any{
				"product_uuid": map[string]any{"type": "string", "format": "uuid"},
				"quantity":     map[string]any{"type": "integer", "minimum": 1, "maximum": 100000},
				"meters":       map[string]any{"type": "number", "minimum": 0.01, "maximum": 100000},
			}, "product_uuid")},
			"note": str("Optional order note.", 1000),
		}, "items"),
		Kind: KindWrite, Realm: RealmPanel, OrgTypes: []string{OrgDistributor, OrgDealer}, Feature: features.ModuleOrders,
		Permissions: []string{rbac.PermOrdersRead, rbac.PermOrdersWrite},
	}
}

func orderErrs() errCases {
	return errCases{tool: ToolCreateOrderDraft, what: "product", forbidden: []error{ordersuc.ErrForbidden, ordersuc.ErrNoSupplier},
		notFound: []error{ordersuc.ErrNotFound, catalogusecase.ErrNotFound}, invalid: func(err error) (string, bool) {
			if errors.Is(err, ordersuc.ErrRateNotFound) {
				return "no exchange rate is available for the supplier currency; try again later", true
			}
			return asError[*ordersuc.ValidationError](err)
		}}
}

// Propose implements ActionTool.
func (t CreateOrderDraft) Propose(ctx context.Context, env Env, raw json.RawMessage) (Proposal, *Result, error) {
	var in orderDraftInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return Proposal{}, r, nil
	}
	in.Note = trimmed(in.Note, 1000)
	fields := make([]Field, 0, len(in.Items)+1)
	for i, it := range in.Items {
		if (it.Quantity > 0) == (it.Meters != nil) {
			return Proposal{}, badInput(fmt.Sprintf("items[%d] needs exactly one of quantity or meters", i)), nil
		}
		id, _ := parseID(it.ProductUUID)
		prod, err := t.products.GetProduct(ctx, *env.Principal.Org, id)
		if err != nil {
			return rejected(orderErrs(), err)
		}
		if !prod.Active {
			return Proposal{}, badInput(fmt.Sprintf("items[%d]: the product %s is not active", i, prod.Name)), nil
		}
		amount := strconv.FormatInt(it.Quantity, 10)
		if it.Meters != nil {
			amount = strconv.FormatFloat(*it.Meters, 'f', 2, 64) + " m"
		}
		fields = append(fields, Field{Key: "item", Value: dataText(prod.Name, maxNameChars) + " × " + amount})
	}
	fields = fieldIf(fields, "note", in.Note)
	return proposal(in, Preview{
		SummaryArgs: map[string]string{"count": strconv.Itoa(len(in.Items))},
		Fields:      fields,
		Edit:        []EditField{{Key: "note", Type: EditTextarea, Value: in.Note}},
		Warnings:    []string{"order_draft_not_submitted"},
	})
}

// Run implements Tool.
func (t CreateOrderDraft) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in orderDraftInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := orderErrs()
	p := env.Principal
	f, err := resolveScope(ctx, t.tree, p, rbac.PermOrdersRead)
	if err != nil {
		return errs.result(err)
	}
	ci := ordersuc.CreateInput{Note: strPtr(in.Note), Items: make([]ordersuc.ItemInput, 0, len(in.Items))}
	for _, it := range in.Items {
		item := ordersuc.ItemInput{ProductUUID: it.ProductUUID}
		if it.Meters != nil {
			m := strconv.FormatFloat(*it.Meters, 'f', 2, 64)
			item.Meters = &m
		} else {
			q := it.Quantity
			item.Quantity = &q
		}
		ci.Items = append(ci.Items, item)
	}
	o, err := t.orders.Create(ctx, ordersuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}, ci)
	if err != nil {
		return errs.result(err)
	}
	return done("Draft order "+o.OrderNo+" created; it is not submitted.", "order", o.UUID)
}

// --- add_service_note -----------------------------------------------------------

// AddServiceNote: hizmete not ekle (hizmet notlarının sonuna eklenir).
type AddServiceNote struct {
	services ServicesWriter
	tree     scopefilter.TreeReader
}

type serviceNoteInput struct {
	ServiceUUID string `json:"service_uuid"`
	Note        string `json:"note"`
}

// Spec implements Tool.
func (AddServiceNote) Spec() Spec {
	return Spec{
		Name: ToolAddServiceNote,
		Description: "Propose appending a note to a service record within your access (uuid from search_services). " +
			"The user confirms it on a card before anything is saved.",
		InputSchema: object(map[string]any{
			"service_uuid": map[string]any{"type": "string", "format": "uuid", "description": "Service uuid."},
			"note":         strMin("Note to append.", 1, 1000),
		}, "service_uuid", "note"),
		Kind: KindWrite, Realm: RealmPanel, Feature: features.ModuleServices,
		Permissions: []string{rbac.PermServicesRead, rbac.PermServicesWrite},
	}
}

func serviceErrs() errCases {
	return errCases{tool: ToolAddServiceNote, what: "service", forbidden: []error{svcuc.ErrForbidden},
		notFound: []error{svcuc.ErrNotFound}, invalid: func(err error) (string, bool) {
			if errors.Is(err, svcuc.ErrNotEditable) {
				return "the service is closed and its notes can no longer be changed", true
			}
			return asError[*svcuc.ValidationError](err)
		}}
}

func (t AddServiceNote) load(ctx context.Context, p Principal, id uuid.UUID) (svcuc.ServiceView, svcuc.Caller, error) {
	f, err := resolveScope(ctx, t.tree, p, rbac.PermServicesRead)
	if err != nil {
		return svcuc.ServiceView{}, svcuc.Caller{}, err
	}
	c := svcuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}
	v, err := t.services.Get(ctx, c, id)
	return v, c, err
}

// Propose implements ActionTool.
func (t AddServiceNote) Propose(ctx context.Context, env Env, raw json.RawMessage) (Proposal, *Result, error) {
	var in serviceNoteInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return Proposal{}, r, nil
	}
	in.Note = trimmed(in.Note, 1000)
	if in.Note == "" {
		return Proposal{}, badInput("note is required"), nil
	}
	id, _ := parseID(in.ServiceUUID)
	v, _, err := t.load(ctx, env.Principal, id)
	if err != nil {
		return rejected(serviceErrs(), err)
	}
	return proposal(in, Preview{
		SummaryArgs: map[string]string{"service_no": v.ServiceNo},
		Fields:      []Field{{Key: "service_no", Value: v.ServiceNo}, {Key: "note", Value: in.Note}},
		Edit:        []EditField{{Key: "note", Type: EditTextarea, Value: in.Note, Required: true}},
	})
}

// Run implements Tool.
func (t AddServiceNote) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in serviceNoteInput
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := serviceErrs()
	id, _ := parseID(in.ServiceUUID)
	v, c, err := t.load(ctx, env.Principal, id)
	if err != nil {
		return errs.result(err)
	}
	notes := strings.TrimSpace(in.Note)
	if v.Notes != nil && strings.TrimSpace(*v.Notes) != "" {
		notes = strings.TrimSpace(*v.Notes) + "\n\n" + notes
	}
	if len([]rune(notes)) > svcuc.MaxNoteLength {
		return *badInput("the service notes are full; shorten the note"), nil
	}
	updated, err := t.services.Update(ctx, c, id, svcuc.UpdateInput{Notes: svcuc.Optional[string]{Set: true, Value: &notes}})
	if err != nil {
		return errs.result(err)
	}
	return done("Note added to service "+updated.ServiceNo+".", "service", updated.UUID)
}
