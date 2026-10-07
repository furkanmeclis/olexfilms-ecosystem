package tools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	appointmentsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	leadsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	tasksuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// AppointmentsReader is the appointments use case surface the tools use.
type AppointmentsReader interface {
	List(ctx context.Context, c appointmentsuc.Caller, f appointmentsuc.ListFilter) ([]appointmentsuc.Appointment, int64, error)
}

// ListAppointments: randevular (gün / hafta).
type ListAppointments struct {
	appointments AppointmentsReader
	tree         scopefilter.TreeReader
}

// Spec implements Tool.
func (ListAppointments) Spec() Spec {
	return Spec{
		Name: "list_appointments",
		Description: "List service appointments of a day or of the week (Monday to Sunday) containing the date, " +
			"within your access: time, customer, vehicle, status. Defaults to today.",
		InputSchema: object(map[string]any{
			"date":   date("Day to list, YYYY-MM-DD. Default: today."),
			"range":  enum("day (default) or week.", "day", "week"),
			"status": enum("Optional status filter.", appointmentsuc.StatusScheduled, appointmentsuc.StatusConfirmed, appointmentsuc.StatusArrived, appointmentsuc.StatusNoShow, appointmentsuc.StatusCancelled),
			"limit":  limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleAppointments,
		Permissions: []string{rbac.PermAppointmentsRead},
	}
}

type appointmentRow struct {
	UUID     uuid.UUID `json:"uuid"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Status   string    `json:"status"`
	Customer string    `json:"customer"`
	Plate    *string   `json:"plate,omitempty"`
	Vehicle  *string   `json:"vehicle,omitempty"`
	Source   string    `json:"source"`
	Note     string    `json:"note,omitempty"`
}

// Run implements Tool.
func (t ListAppointments) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Date   string `json:"date"`
		Range  string `json:"range"`
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := errCases{tool: t.Spec().Name, what: "appointment", forbidden: []error{appointmentsuc.ErrForbidden},
		notFound: []error{appointmentsuc.ErrNotFound}, invalid: asError[*appointmentsuc.ValidationError]}
	loc := env.Loc()
	now := env.Now.In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if in.Date != "" {
		day, _ = time.ParseInLocation(time.DateOnly, in.Date, loc)
	}
	from, to := day, day.AddDate(0, 0, 1)
	if in.Range == "week" {
		offset := (int(day.Weekday()) + 6) % 7 // Monday = 0
		from = day.AddDate(0, 0, -offset)
		to = from.AddDate(0, 0, 7)
	}
	p := env.Principal
	f, err := resolveScope(ctx, t.tree, p, rbac.PermAppointmentsRead)
	if err != nil {
		return errs.result(err)
	}
	rows, total, err := t.appointments.List(ctx, appointmentsuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f},
		appointmentsuc.ListFilter{From: from, To: to, Status: in.Status, Limit: limitArg(in.Limit)})
	if err != nil {
		return errs.result(err)
	}
	out := make([]appointmentRow, 0, len(rows))
	for _, a := range rows {
		out = append(out, appointmentRow{
			UUID: a.UUID, StartsAt: a.StartsAt.In(loc), EndsAt: a.EndsAt.In(loc), Status: a.Status,
			Customer: dataText(a.CustomerName, maxNameChars), Plate: a.VehiclePlate,
			Vehicle: textPtr(a.VehicleLabel, maxNameChars), Source: a.Source, Note: dataText(a.Note, maxTextChars),
		})
	}
	type result struct {
		From string `json:"from"`
		To   string `json:"to"`
		List[appointmentRow]
	}
	return JSONResult(result{From: from.Format(time.DateOnly), To: to.AddDate(0, 0, -1).Format(time.DateOnly),
		List: NewList(out, total)})
}

// LeadsReader is the leads use case surface the tools use.
type LeadsReader interface {
	List(ctx context.Context, c leadsuc.Caller, f leadsuc.ListFilter) ([]leadsuc.Lead, int64, error)
}

// ListLeads: lead listesi.
type ListLeads struct {
	leads LeadsReader
	tree  scopefilter.TreeReader
}

// Spec implements Tool.
func (ListLeads) Spec() Spec {
	return Spec{
		Name: "list_leads",
		Description: "List sales leads (potential customers or dealers) within your access: status, temperature, " +
			"source and follow-up date. Filter by text, status, your own leads or due follow-ups.",
		InputSchema: object(map[string]any{
			"query":     str("Optional name, company, phone or e-mail.", 100),
			"status":    enum("Optional status filter.", leadsuc.StatusNew, leadsuc.StatusContacted, leadsuc.StatusQuoted, leadsuc.StatusWon, leadsuc.StatusLost),
			"mine":      map[string]any{"type": "boolean", "description": "Only leads assigned to the user."},
			"follow_up": enum("Optional: overdue or today (leads whose follow-up date is due).", "overdue", "today"),
			"limit":     limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleLeads,
		Permissions: []string{rbac.PermLeadsRead},
	}
}

type leadRow struct {
	UUID         uuid.UUID  `json:"uuid"`
	TargetType   string     `json:"target_type"`
	Company      *string    `json:"company,omitempty"`
	Contact      *string    `json:"contact,omitempty"`
	Phone        *string    `json:"phone,omitempty"`
	Email        *string    `json:"email,omitempty"`
	Status       string     `json:"status"`
	Temperature  string     `json:"temperature"`
	Source       string     `json:"source"`
	FollowUpDate *time.Time `json:"follow_up_date,omitempty"`
	Notes        string     `json:"notes,omitempty"`
	AssignedToMe bool       `json:"assigned_to_me"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Run implements Tool.
func (t ListLeads) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Query    string `json:"query"`
		Status   string `json:"status"`
		Mine     bool   `json:"mine"`
		FollowUp string `json:"follow_up"`
		Limit    int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := errCases{tool: t.Spec().Name, what: "lead", invalid: asError[*leadsuc.ValidationError]}
	p := env.Principal
	f, err := resolveScope(ctx, t.tree, p, rbac.PermLeadsRead)
	if err != nil {
		return errs.result(err)
	}
	lf := leadsuc.ListFilter{Q: strings.TrimSpace(in.Query), FollowUp: in.FollowUp, Limit: limitArg(in.Limit)}
	if in.Status != "" {
		lf.Statuses = []string{in.Status}
	}
	if in.Mine {
		lf.AssigneeIDs = []int64{p.Auth.UserInternal}
	}
	rows, total, err := t.leads.List(ctx, leadsuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}, lf)
	if err != nil {
		return errs.result(err)
	}
	out := make([]leadRow, 0, len(rows))
	for _, l := range rows {
		out = append(out, leadRow{
			UUID: l.UUID, TargetType: l.TargetType, Company: textPtr(l.CandidateCompanyName, maxNameChars),
			Contact: textPtr(l.CandidateContactName, maxNameChars), Phone: l.CandidatePhoneE164, Email: l.CandidateEmail,
			Status: l.Status, Temperature: l.Temperature, Source: l.Source, FollowUpDate: l.FollowUpDate,
			Notes: dataText(l.Notes, maxTextChars), CreatedAt: l.CreatedAt,
			AssignedToMe: l.AssigneeUserID != nil && *l.AssigneeUserID == p.Auth.UserInternal,
		})
	}
	return JSONResult(NewList(out, total))
}

// TasksReader is the tasks use case surface the tools use.
type TasksReader interface {
	List(ctx context.Context, c tasksuc.Caller, f tasksuc.Filter) ([]tasksuc.Task, int64, error)
}

// MyTasks: görevlerim (merkez görevleri; tasks modülü yalnız merkezde).
type MyTasks struct{ tasks TasksReader }

// Spec implements Tool.
func (MyTasks) Spec() Spec {
	return Spec{
		Name: "my_tasks",
		Description: "List the center tasks assigned to the user (about distributors and dealers), open ones by " +
			"default: title, subject organization, priority, due date.",
		InputSchema: object(map[string]any{
			"status": enum("Optional status filter (default: open and in_progress).", tasksuc.StatusOpen, tasksuc.StatusInProgress, tasksuc.StatusDone, tasksuc.StatusCancelled),
			"query":  str("Optional text filter.", 100),
			"limit":  limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, OrgTypes: []string{OrgCenter}, Feature: features.ModuleTasks,
		Permissions: []string{rbac.PermTasksRead},
	}
}

type taskRow struct {
	UUID         uuid.UUID  `json:"uuid"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	Organization string     `json:"subject_organization"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	DueAt        *time.Time `json:"due_at,omitempty"`
	Overdue      bool       `json:"overdue,omitempty"`
}

// Run implements Tool.
func (t MyTasks) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Status string `json:"status"`
		Query  string `json:"query"`
		Limit  int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := errCases{tool: t.Spec().Name, what: "task", forbidden: []error{tasksuc.ErrCenterOnly},
		notFound: []error{tasksuc.ErrNotFound}, invalid: asError[*tasksuc.ValidationError]}
	p := env.Principal
	me := p.Auth.UserID
	tf := tasksuc.Filter{AssigneeUUID: &me, Q: strings.TrimSpace(in.Query), Limit: limitArg(in.Limit),
		Statuses: []string{tasksuc.StatusOpen, tasksuc.StatusInProgress}}
	if in.Status != "" {
		tf.Statuses = []string{in.Status}
	}
	rows, total, err := t.tasks.List(ctx, tasksuc.Caller{UserID: p.Auth.UserInternal, Org: *p.Org}, tf)
	if err != nil {
		return errs.result(err)
	}
	out := make([]taskRow, 0, len(rows))
	for _, tk := range rows {
		r := taskRow{UUID: tk.UUID, Title: dataText(tk.Title, maxNameChars), Description: dataText(tk.Description, maxTextChars),
			Organization: dataText(tk.Subject.Name, maxNameChars), Priority: tk.Priority, Status: tk.Status, DueAt: tk.DueAt}
		if tk.DueAt != nil && tk.DueAt.Before(env.Now) && (tk.Status == tasksuc.StatusOpen || tk.Status == tasksuc.StatusInProgress) {
			r.Overdue = true
		}
		out = append(out, r)
	}
	return JSONResult(NewList(out, total))
}
