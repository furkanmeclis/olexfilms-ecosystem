// Package usecase implements the F3 lead API (TEC-313).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	tasksuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusNew       = "new"
	StatusContacted = "contacted"
	StatusQuoted    = "quoted"
	StatusWon       = "won"
	StatusLost      = "lost"

	EventNote        = "note"
	EventMessage     = "message"
	EventAssigned    = "assigned"
	EventFollowUpSet = "follow_up_set"
	EventConverted   = "converted"
	EventTaskCreated = "task_created"

	CodeInvalidTransition = "LEAD_INVALID_TRANSITION"

	maxName  = 200
	maxEmail = 255
	maxNotes = 20000
)

var (
	ErrNotFound          = errors.New("leads: not found")
	ErrForbidden         = errors.New("leads: forbidden")
	ErrInvalidTransition = errors.New("leads: invalid transition")

	e164Re = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
)

// TxBeginner opens a transaction.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// TaskCreator is the center task use case.
type TaskCreator interface {
	Create(ctx context.Context, c tasksuc.Caller, in tasksuc.CreateInput) (tasksuc.Task, error)
}

// Caller is the authenticated member in an active organization.
type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

func (c Caller) actor() pgtype.Int8 {
	return pgtype.Int8{Int64: c.Principal.UserInternal, Valid: c.Principal.UserInternal != 0}
}

// Service is the lead use case.
type Service struct {
	pool    TxBeginner
	q       *db.Queries
	tasks   TaskCreator
	finder  searchengine.ListFinder
	nowFunc func() time.Time
}

// New creates the service.
func New(pool TxBeginner, q *db.Queries, tasks TaskCreator) *Service {
	return &Service{pool: pool, q: q, tasks: tasks, nowFunc: time.Now}
}

// SetFinder enables Meilisearch-backed list search.
func (s *Service) SetFinder(f searchengine.ListFinder) { s.finder = f }

// SetClock is used by tests.
func (s *Service) SetClock(fn func() time.Time) {
	if fn != nil {
		s.nowFunc = fn
	}
}

// ValidationError is one invalid input field.
type ValidationError struct {
	Field   string
	Message string
	Code    string
}

func (e *ValidationError) Error() string { return "leads: invalid " + e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// Lead is the API shape of a lead.
type Lead struct {
	UUID                 uuid.UUID  `json:"uuid"`
	OrganizationUUID     uuid.UUID  `json:"organization_uuid"`
	TargetType           string     `json:"target_type"`
	CustomerUserID       *int64     `json:"customer_user_id,omitempty"`
	VehicleID            *int64     `json:"vehicle_id,omitempty"`
	CandidateCompanyName *string    `json:"candidate_company_name,omitempty"`
	CandidateContactName *string    `json:"candidate_contact_name,omitempty"`
	CandidatePhoneE164   *string    `json:"candidate_phone_e164,omitempty"`
	CandidateEmail       *string    `json:"candidate_email,omitempty"`
	CountryID            *int64     `json:"country_id,omitempty"`
	ProvinceID           *int64     `json:"province_id,omitempty"`
	DistrictID           *int64     `json:"district_id,omitempty"`
	Source               string     `json:"source"`
	Temperature          string     `json:"temperature"`
	Status               string     `json:"status"`
	LostReason           *string    `json:"lost_reason,omitempty"`
	FollowUpDate         *time.Time `json:"follow_up_date,omitempty"`
	AssigneeUserID       *int64     `json:"assignee_user_id,omitempty"`
	Notes                string     `json:"notes"`
	WonRefType           *string    `json:"won_ref_type,omitempty"`
	WonRefID             *int64     `json:"won_ref_id,omitempty"`
	CreatedByUserID      *int64     `json:"created_by_user_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// Event is one timeline entry.
type Event struct {
	UUID        uuid.UUID      `json:"uuid"`
	EventType   string         `json:"event_type"`
	Payload     map[string]any `json:"payload"`
	ActorUserID *int64         `json:"actor_user_id,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// ListFilter narrows List.
type ListFilter struct {
	Status, TargetType string
	FollowUp           string
	Q                  string
	Limit, Offset      int32
}

type leadInput struct {
	TargetType           string
	CustomerUserID       *int64
	VehicleID            *int64
	CandidateCompanyName *string
	CandidateContactName *string
	CandidatePhoneE164   *string
	CandidateEmail       *string
	CountryID            *int64
	ProvinceID           *int64
	DistrictID           *int64
	Source               string
	Temperature          string
	FollowUpDate         *time.Time
	AssigneeUserID       *int64
	Notes                *string
}

type CreateInput = leadInput

// PatchInput changes a lead; nil fields stay unchanged.
type PatchInput struct {
	TargetType           *string
	CustomerUserID       Field[int64]
	VehicleID            Field[int64]
	CandidateCompanyName Field[string]
	CandidateContactName Field[string]
	CandidatePhoneE164   Field[string]
	CandidateEmail       Field[string]
	CountryID            Field[int64]
	ProvinceID           Field[int64]
	DistrictID           Field[int64]
	Source               *string
	Temperature          *string
	FollowUpDate         Field[time.Time]
	AssigneeUserID       Field[int64]
	Notes                *string
}

// Field distinguishes absent from explicit null in PATCH.
type Field[T any] struct {
	Set   bool
	Value *T
}

type StatusInput struct {
	Status     string
	LostReason *string
	WonRefType *string
	WonRefID   *int64
}

type TaskInput struct {
	Title        string
	Description  string
	AssigneeUUID *uuid.UUID
	Priority     string
	DueAt        *time.Time
}

type FollowUpCount struct {
	Overdue int64 `json:"overdue"`
	Today   int64 `json:"today"`
}

func validTargetType(v string) bool {
	switch v {
	case "customer", "dealer_candidate", "distributor_candidate":
		return true
	}
	return false
}

func validSource(v string) bool {
	switch v {
	case "incoming_call", "outgoing_call", "walk_in", "whatsapp", "social", "referral", "website", "application_form", "other":
		return true
	}
	return false
}

func validTemperature(v string) bool {
	switch v {
	case "cold", "warm", "hot":
		return true
	}
	return false
}

func validStatus(v string) bool {
	switch v {
	case StatusNew, StatusContacted, StatusQuoted, StatusWon, StatusLost:
		return true
	}
	return false
}

func cleanOptText(field string, v *string, max int) (pgtype.Text, error) {
	if v == nil {
		return pgtype.Text{}, nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return pgtype.Text{}, nil
	}
	if utf8.RuneCountInString(s) > max {
		return pgtype.Text{}, invalid(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return pgtype.Text{String: s, Valid: true}, nil
}

func textPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func intPtr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	i := v.Int64
	return &i
}

func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func int8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func tstz(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *v, Valid: true}
}

func leadOf(row db.Lead, org uuid.UUID) Lead {
	return Lead{
		UUID: row.Uuid, OrganizationUUID: org, TargetType: row.TargetType,
		CustomerUserID: intPtr(row.CustomerUserID), VehicleID: intPtr(row.VehicleID),
		CandidateCompanyName: textPtr(row.CandidateCompanyName), CandidateContactName: textPtr(row.CandidateContactName),
		CandidatePhoneE164: textPtr(row.CandidatePhoneE164), CandidateEmail: textPtr(row.CandidateEmail),
		CountryID: intPtr(row.CountryID), ProvinceID: intPtr(row.ProvinceID), DistrictID: intPtr(row.DistrictID),
		Source: row.Source, Temperature: row.Temperature, Status: row.Status, LostReason: textPtr(row.LostReason),
		FollowUpDate: timePtr(row.FollowUpDate), AssigneeUserID: intPtr(row.AssigneeUserID),
		Notes: row.Notes, WonRefType: textPtr(row.WonRefType), WonRefID: intPtr(row.WonRefID),
		CreatedByUserID: intPtr(row.CreatedByUserID), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func (s *Service) orgUUID(ctx context.Context, orgID int64) (uuid.UUID, error) {
	if orgID == 0 {
		return uuid.Nil, nil
	}
	o, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("leads: organization: %w", err)
	}
	return o.Uuid, nil
}

func (s *Service) toLead(ctx context.Context, row db.Lead) (Lead, error) {
	id, err := s.orgUUID(ctx, row.OrganizationID)
	if err != nil {
		return Lead{}, err
	}
	return leadOf(row, id), nil
}

func (s *Service) params(c Caller, f ListFilter) (db.ListLeadsInScopeParams, error) {
	p := db.ListLeadsInScopeParams{
		BrandID: c.Org.BrandID, OrganizationIds: c.Filter.OrgIDsArg(),
		ActorUserID: c.Principal.UserInternal, PageLimit: f.Limit, PageOffset: f.Offset,
	}
	if f.Status != "" {
		if !validStatus(f.Status) {
			return p, invalid("status", "must be new, contacted, quoted, won or lost")
		}
		p.Status = pgtype.Text{String: f.Status, Valid: true}
	}
	if f.TargetType != "" {
		if !validTargetType(f.TargetType) {
			return p, invalid("target_type", "must be customer, dealer_candidate or distributor_candidate")
		}
		p.TargetType = pgtype.Text{String: f.TargetType, Valid: true}
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	now := s.nowFunc().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch f.FollowUp {
	case "":
	case "overdue":
		p.FollowUpOnly = true
		p.FollowUpTo = pgtype.Timestamptz{Time: start, Valid: true}
	case "today":
		p.FollowUpOnly = true
		p.FollowUpFrom = pgtype.Timestamptz{Time: start, Valid: true}
		p.FollowUpTo = pgtype.Timestamptz{Time: start.Add(24 * time.Hour), Valid: true}
	default:
		return p, invalid("follow_up", "must be overdue or today")
	}
	return p, nil
}

// List returns leads in the caller's resolved scope.
func (s *Service) List(ctx context.Context, c Caller, f ListFilter) ([]Lead, int64, error) {
	p, err := s.params(c, f)
	if err != nil {
		return nil, 0, err
	}
	rows, total, indexed := []db.Lead(nil), int64(0), false
	if p.Q.Valid && !p.FollowUpOnly && s.finder != nil && s.finder.Enabled() {
		rows, total, indexed = s.searchIndexed(ctx, c, p)
	}
	if !indexed {
		rows, err = s.q.ListLeadsInScope(ctx, p)
		if err != nil {
			return nil, 0, fmt.Errorf("leads: list: %w", err)
		}
		total, err = s.q.CountLeadsInScope(ctx, db.CountLeadsInScopeParams{
			BrandID: p.BrandID, OrganizationIds: p.OrganizationIds, Status: p.Status, TargetType: p.TargetType,
			Q: p.Q, FollowUpOnly: p.FollowUpOnly, ActorUserID: p.ActorUserID,
			FollowUpFrom: p.FollowUpFrom, FollowUpTo: p.FollowUpTo,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("leads: count: %w", err)
		}
	}
	out := make([]Lead, 0, len(rows))
	for _, r := range rows {
		v, err := s.toLead(ctx, r)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}

func (s *Service) getRow(ctx context.Context, c Caller, id uuid.UUID) (db.Lead, error) {
	row, err := s.q.GetLeadByUUID(ctx, db.GetLeadByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Lead{}, ErrNotFound
	}
	if err != nil {
		return db.Lead{}, fmt.Errorf("leads: get: %w", err)
	}
	if !c.Filter.AllowsOrg(row.OrganizationID, row.BrandID) {
		return db.Lead{}, ErrNotFound
	}
	return row, nil
}

// Get returns one lead.
func (s *Service) Get(ctx context.Context, c Caller, id uuid.UUID) (Lead, error) {
	row, err := s.getRow(ctx, c, id)
	if err != nil {
		return Lead{}, err
	}
	return s.toLead(ctx, row)
}

func (s *Service) validateInput(in leadInput, create bool) (db.CreateLeadParams, error) {
	target := strings.TrimSpace(in.TargetType)
	if target == "" && !create {
		target = "customer"
	}
	if !validTargetType(target) {
		return db.CreateLeadParams{}, invalid("target_type", "must be customer, dealer_candidate or distributor_candidate")
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "other"
	}
	if !validSource(source) {
		return db.CreateLeadParams{}, invalid("source", "is invalid")
	}
	temp := strings.TrimSpace(in.Temperature)
	if temp == "" {
		temp = "cold"
	}
	if !validTemperature(temp) {
		return db.CreateLeadParams{}, invalid("temperature", "must be cold, warm or hot")
	}
	company, err := cleanOptText("candidate_company_name", in.CandidateCompanyName, maxName)
	if err != nil {
		return db.CreateLeadParams{}, err
	}
	contact, err := cleanOptText("candidate_contact_name", in.CandidateContactName, maxName)
	if err != nil {
		return db.CreateLeadParams{}, err
	}
	phone, err := cleanOptText("candidate_phone_e164", in.CandidatePhoneE164, 16)
	if err != nil {
		return db.CreateLeadParams{}, err
	}
	if phone.Valid && !e164Re.MatchString(phone.String) {
		return db.CreateLeadParams{}, invalid("candidate_phone_e164", "must be E.164")
	}
	email, err := cleanOptText("candidate_email", in.CandidateEmail, maxEmail)
	if err != nil {
		return db.CreateLeadParams{}, err
	}
	notes := ""
	if in.Notes != nil {
		notes = strings.TrimSpace(*in.Notes)
	}
	if utf8.RuneCountInString(notes) > maxNotes {
		return db.CreateLeadParams{}, invalid("notes", fmt.Sprintf("must be at most %d characters", maxNotes))
	}
	return db.CreateLeadParams{
		TargetType: target, CustomerUserID: int8(in.CustomerUserID), VehicleID: int8(in.VehicleID),
		CandidateCompanyName: company, CandidateContactName: contact, CandidatePhoneE164: phone, CandidateEmail: email,
		CountryID: int8(in.CountryID), ProvinceID: int8(in.ProvinceID), DistrictID: int8(in.DistrictID),
		Source: source, Temperature: temp, Status: StatusNew, FollowUpDate: tstz(in.FollowUpDate),
		AssigneeUserID: int8(in.AssigneeUserID), Notes: notes,
	}, nil
}

// Create opens a new lead in the active organization.
func (s *Service) Create(ctx context.Context, c Caller, in CreateInput) (Lead, error) {
	p, err := s.validateInput(in, true)
	if err != nil {
		return Lead{}, err
	}
	p.OrganizationID, p.BrandID, p.CreatedByUserID = c.Org.InternalID, c.Org.BrandID, c.actor()
	row, err := s.q.CreateLead(ctx, p)
	if err != nil {
		return Lead{}, fmt.Errorf("leads: create: %w", err)
	}
	return s.toLead(ctx, row)
}

func applyPatch(cur db.Lead, in PatchInput) (leadInput, []string) {
	out := leadInput{
		TargetType: cur.TargetType, CustomerUserID: intPtr(cur.CustomerUserID), VehicleID: intPtr(cur.VehicleID),
		CandidateCompanyName: textPtr(cur.CandidateCompanyName), CandidateContactName: textPtr(cur.CandidateContactName),
		CandidatePhoneE164: textPtr(cur.CandidatePhoneE164), CandidateEmail: textPtr(cur.CandidateEmail),
		CountryID: intPtr(cur.CountryID), ProvinceID: intPtr(cur.ProvinceID), DistrictID: intPtr(cur.DistrictID),
		Source: cur.Source, Temperature: cur.Temperature, FollowUpDate: timePtr(cur.FollowUpDate),
		AssigneeUserID: intPtr(cur.AssigneeUserID), Notes: &cur.Notes,
	}
	var fields []string
	if in.TargetType != nil {
		out.TargetType = *in.TargetType
		fields = append(fields, "target_type")
	}
	if in.CustomerUserID.Set {
		out.CustomerUserID = in.CustomerUserID.Value
		fields = append(fields, "customer_user_id")
	}
	if in.VehicleID.Set {
		out.VehicleID = in.VehicleID.Value
		fields = append(fields, "vehicle_id")
	}
	if in.CandidateCompanyName.Set {
		out.CandidateCompanyName = in.CandidateCompanyName.Value
		fields = append(fields, "candidate_company_name")
	}
	if in.CandidateContactName.Set {
		out.CandidateContactName = in.CandidateContactName.Value
		fields = append(fields, "candidate_contact_name")
	}
	if in.CandidatePhoneE164.Set {
		out.CandidatePhoneE164 = in.CandidatePhoneE164.Value
		fields = append(fields, "candidate_phone_e164")
	}
	if in.CandidateEmail.Set {
		out.CandidateEmail = in.CandidateEmail.Value
		fields = append(fields, "candidate_email")
	}
	if in.CountryID.Set {
		out.CountryID = in.CountryID.Value
		fields = append(fields, "country_id")
	}
	if in.ProvinceID.Set {
		out.ProvinceID = in.ProvinceID.Value
		fields = append(fields, "province_id")
	}
	if in.DistrictID.Set {
		out.DistrictID = in.DistrictID.Value
		fields = append(fields, "district_id")
	}
	if in.Source != nil {
		out.Source = *in.Source
		fields = append(fields, "source")
	}
	if in.Temperature != nil {
		out.Temperature = *in.Temperature
		fields = append(fields, "temperature")
	}
	if in.FollowUpDate.Set {
		out.FollowUpDate = in.FollowUpDate.Value
		fields = append(fields, "follow_up_date")
	}
	if in.AssigneeUserID.Set {
		out.AssigneeUserID = in.AssigneeUserID.Value
		fields = append(fields, "assignee_user_id")
	}
	if in.Notes != nil {
		out.Notes = in.Notes
		fields = append(fields, "notes")
	}
	return out, fields
}

func (s *Service) inTx(ctx context.Context, fn func(*db.Queries) error) error {
	if s.pool == nil {
		return fn(s.q)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func addEvent(ctx context.Context, q *db.Queries, row db.Lead, typ string, payload map[string]any, actor pgtype.Int8) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = q.AddLeadEvent(ctx, db.AddLeadEventParams{
		LeadID: row.ID, OrganizationID: row.OrganizationID, BrandID: row.BrandID,
		EventType: typ, Payload: b, ActorUserID: actor,
	})
	return err
}

// Patch changes mutable lead fields and appends one timeline event.
func (s *Service) Patch(ctx context.Context, c Caller, id uuid.UUID, in PatchInput) (Lead, error) {
	cur, err := s.getRow(ctx, c, id)
	if err != nil {
		return Lead{}, err
	}
	next, fields := applyPatch(cur, in)
	if len(fields) == 0 {
		return s.toLead(ctx, cur)
	}
	p, err := s.validateInput(next, false)
	if err != nil {
		return Lead{}, err
	}
	var row db.Lead
	err = s.inTx(ctx, func(q *db.Queries) error {
		row, err = q.UpdateLead(ctx, db.UpdateLeadParams{
			ID: cur.ID, OrganizationID: cur.OrganizationID, TargetType: p.TargetType,
			CustomerUserID: p.CustomerUserID, VehicleID: p.VehicleID,
			CandidateCompanyName: p.CandidateCompanyName, CandidateContactName: p.CandidateContactName,
			CandidatePhoneE164: p.CandidatePhoneE164, CandidateEmail: p.CandidateEmail,
			CountryID: p.CountryID, ProvinceID: p.ProvinceID, DistrictID: p.DistrictID,
			Source: p.Source, Temperature: p.Temperature, LostReason: cur.LostReason,
			FollowUpDate: p.FollowUpDate, AssigneeUserID: p.AssigneeUserID, Notes: p.Notes,
		})
		if err != nil {
			return fmt.Errorf("leads: patch: %w", err)
		}
		typ := EventMessage
		if len(fields) == 1 && fields[0] == "follow_up_date" {
			typ = EventFollowUpSet
		}
		return addEvent(ctx, q, row, typ, map[string]any{"kind": "updated", "fields": fields}, c.actor())
	})
	if err != nil {
		return Lead{}, err
	}
	return s.toLead(ctx, row)
}

func allowedTransition(from, to string) bool {
	switch from {
	case StatusNew:
		return to == StatusContacted
	case StatusContacted:
		return to == StatusQuoted
	case StatusQuoted:
		return to == StatusWon || to == StatusLost
	case StatusLost:
		return to == StatusContacted
	default:
		return false
	}
}

// SetStatus moves a lead through the allowed pipeline.
func (s *Service) SetStatus(ctx context.Context, c Caller, id uuid.UUID, in StatusInput) (Lead, error) {
	cur, err := s.getRow(ctx, c, id)
	if err != nil {
		return Lead{}, err
	}
	to := strings.TrimSpace(in.Status)
	if !validStatus(to) {
		return Lead{}, invalid("status", "must be new, contacted, quoted, won or lost")
	}
	if cur.Status == to {
		return s.toLead(ctx, cur)
	}
	if !allowedTransition(cur.Status, to) {
		return Lead{}, ErrInvalidTransition
	}
	var lost pgtype.Text
	if to == StatusLost {
		if in.LostReason == nil || strings.TrimSpace(*in.LostReason) == "" {
			return Lead{}, invalid("lost_reason", "is required when status is lost")
		}
		lost = pgtype.Text{String: strings.TrimSpace(*in.LostReason), Valid: true}
	}
	row, err := s.q.SetLeadStatus(ctx, db.SetLeadStatusParams{
		ID: cur.ID, OrganizationID: cur.OrganizationID, Status: to, LostReason: lost,
		WonRefType: func() pgtype.Text {
			if in.WonRefType != nil {
				return pgtype.Text{String: strings.TrimSpace(*in.WonRefType), Valid: true}
			}
			return pgtype.Text{}
		}(),
		WonRefID: int8(in.WonRefID),
	})
	if err != nil {
		return Lead{}, fmt.Errorf("leads: status: %w", err)
	}
	return s.toLead(ctx, row)
}

// AddNote appends a timeline note.
func (s *Service) AddNote(ctx context.Context, c Caller, id uuid.UUID, body string) (Event, error) {
	row, err := s.getRow(ctx, c, id)
	if err != nil {
		return Event{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return Event{}, invalid("body", "is required")
	}
	ev, err := s.add(ctx, row, EventNote, map[string]any{"body": body}, c.actor())
	if err != nil {
		return Event{}, err
	}
	return ev, nil
}

// Assign changes assignee and appends a timeline event.
func (s *Service) Assign(ctx context.Context, c Caller, id uuid.UUID, assignee *int64) (Lead, error) {
	cur, err := s.getRow(ctx, c, id)
	if err != nil {
		return Lead{}, err
	}
	var row db.Lead
	err = s.inTx(ctx, func(q *db.Queries) error {
		row, err = q.AssignLead(ctx, db.AssignLeadParams{ID: cur.ID, OrganizationID: cur.OrganizationID, AssigneeUserID: int8(assignee)})
		if err != nil {
			return fmt.Errorf("leads: assign: %w", err)
		}
		return addEvent(ctx, q, row, EventAssigned, map[string]any{"assignee_user_id": assignee}, c.actor())
	})
	if err != nil {
		return Lead{}, err
	}
	return s.toLead(ctx, row)
}

func (s *Service) add(ctx context.Context, row db.Lead, typ string, payload map[string]any, actor pgtype.Int8) (Event, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	ev, err := s.q.AddLeadEvent(ctx, db.AddLeadEventParams{
		LeadID: row.ID, OrganizationID: row.OrganizationID, BrandID: row.BrandID,
		EventType: typ, Payload: b, ActorUserID: actor,
	})
	if err != nil {
		return Event{}, fmt.Errorf("leads: event: %w", err)
	}
	return eventOf(ev), nil
}

func eventOf(row db.LeadEvent) Event {
	var payload map[string]any
	if len(row.Payload) > 0 {
		_ = json.Unmarshal(row.Payload, &payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return Event{UUID: row.Uuid, EventType: row.EventType, Payload: payload, ActorUserID: intPtr(row.ActorUserID), CreatedAt: row.CreatedAt.Time}
}

// Events returns the lead timeline.
func (s *Service) Events(ctx context.Context, c Caller, id uuid.UUID) ([]Event, error) {
	row, err := s.getRow(ctx, c, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListLeadEvents(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("leads: events: %w", err)
	}
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		out = append(out, eventOf(r))
	}
	return out, nil
}

// FollowUpCount returns nav badge counters for the caller's queue.
func (s *Service) FollowUpCount(ctx context.Context, c Caller) (FollowUpCount, error) {
	overdue, err := s.countFollowUp(ctx, c, "overdue")
	if err != nil {
		return FollowUpCount{}, err
	}
	today, err := s.countFollowUp(ctx, c, "today")
	if err != nil {
		return FollowUpCount{}, err
	}
	return FollowUpCount{Overdue: overdue, Today: today}, nil
}

func (s *Service) countFollowUp(ctx context.Context, c Caller, mode string) (int64, error) {
	p, err := s.params(c, ListFilter{FollowUp: mode, Limit: 1})
	if err != nil {
		return 0, err
	}
	return s.q.CountLeadsInScope(ctx, db.CountLeadsInScopeParams{
		BrandID: p.BrandID, OrganizationIds: p.OrganizationIds, FollowUpOnly: p.FollowUpOnly,
		ActorUserID: p.ActorUserID, FollowUpFrom: p.FollowUpFrom, FollowUpTo: p.FollowUpTo,
	})
}

// CreateTask opens a center task about the lead's owner organization.
func (s *Service) CreateTask(ctx context.Context, c Caller, id uuid.UUID, in TaskInput) (tasksuc.Task, error) {
	if c.Org.OrgType != rbac.OrgTypeCenter {
		return tasksuc.Task{}, ErrForbidden
	}
	if s.tasks == nil {
		return tasksuc.Task{}, fmt.Errorf("leads: tasks service is not configured")
	}
	row, err := s.getRow(ctx, c, id)
	if err != nil {
		return tasksuc.Task{}, err
	}
	subject, err := s.orgUUID(ctx, row.OrganizationID)
	if err != nil {
		return tasksuc.Task{}, err
	}
	t, err := s.tasks.Create(ctx, tasksuc.Caller{UserID: c.Principal.UserInternal, Org: c.Org}, tasksuc.CreateInput{
		SubjectOrgUUID: subject, Title: in.Title, Description: in.Description,
		AssigneeUUID: in.AssigneeUUID, Priority: in.Priority, DueAt: in.DueAt,
	})
	if err != nil {
		return tasksuc.Task{}, err
	}
	_, err = s.add(ctx, row, EventTaskCreated, map[string]any{"task_uuid": t.UUID.String()}, c.actor())
	if err != nil {
		return tasksuc.Task{}, err
	}
	return t, nil
}
