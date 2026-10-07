// Package usecase implements center tasks (TEC-214, F1-11a). The center of
// a brand opens tasks about one of its distributors or dealers, assigns them
// to a center member, sets a priority and a due date, moves them through
// open -> in_progress -> done/cancelled and comments on them.
//
// tasks.* are center-only grants; on top of the permission every call
// requires the active organization to be the brand center, and every read
// is bounded by its brand (K1/K20). Each write records an outbox event and
// an audit row in its transaction. source=auto tasks (TEC-130) are not
// opened here.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Task statuses (tasks.status).
const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusDone       = "done"
	StatusCancelled  = "cancelled"
)

// Task priorities (tasks.priority).
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

// SourceManual marks tasks opened through the API; "auto" is reserved for
// the F5 performance panel (TEC-130).
const SourceManual = "manual"

const (
	orgTypeCenter      = "center"
	orgTypeDistributor = "distributor"
	orgTypeDealer      = "dealer"

	taskResource    = "task"
	commentResource = "task_comment"

	maxTitle       = 200
	maxDescription = 10000
	maxComment     = 4000
)

var (
	// ErrCenterOnly: the active organization is not the brand center.
	ErrCenterOnly = errors.New("tasks: only the center organization manages tasks")
	// ErrNotFound: no such task in the active brand.
	ErrNotFound = errors.New("tasks: task not found")
)

// ValidationError is one invalid input field (422).
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return "tasks: invalid " + e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// TxBeginner opens a transaction (pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Caller is the authenticated member and its active organization.
type Caller struct {
	UserID int64 // internal user id; 0 when unknown
	Org    orgctx.Scope
}

func (c Caller) actor() pgtype.Int8 {
	return pgtype.Int8{Int64: c.UserID, Valid: c.UserID != 0}
}

// Service is the task use case.
type Service struct {
	pool TxBeginner
	q    *db.Queries
	out  outbox.Enqueuer
}

// New creates the service. out may be nil (no events).
func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *Service {
	return &Service{pool: pool, q: q, out: out}
}

func requireCenter(c Caller) error {
	if c.Org.OrgType != orgTypeCenter || c.Org.BrandID == 0 {
		return ErrCenterOnly
	}
	return nil
}

// Ref is an organization or user reference.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// SubjectRef is the distributor or dealer a task is about.
type SubjectRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// Task is one task as the API shows it.
type Task struct {
	UUID         uuid.UUID  `json:"uuid"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Subject      SubjectRef `json:"subject_organization"`
	Assignee     *Ref       `json:"assignee"`
	CreatedBy    *Ref       `json:"created_by"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	Source       string     `json:"source"`
	DueAt        *time.Time `json:"due_at"`
	ClosedAt     *time.Time `json:"closed_at"`
	CommentCount int64      `json:"comment_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func userRef(id pgtype.UUID, name, surname pgtype.Text) *Ref {
	if !id.Valid {
		return nil
	}
	return &Ref{UUID: uuid.UUID(id.Bytes), Name: strings.TrimSpace(name.String + " " + surname.String)}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func taskOf(r db.ListTasksRow) Task {
	return Task{
		UUID: r.Uuid, Title: r.Title, Description: r.Description,
		Subject:   SubjectRef{UUID: r.SubjectUuid, Name: r.SubjectName, Type: r.SubjectType},
		Assignee:  userRef(r.AssigneeUuid, r.AssigneeName, r.AssigneeSurname),
		CreatedBy: userRef(r.CreatorUuid, r.CreatorName, r.CreatorSurname),
		Priority:  r.Priority, Status: r.Status, Source: r.Source,
		DueAt: timePtr(r.DueAt), ClosedAt: timePtr(r.ClosedAt),
		CommentCount: r.CommentCount,
		CreatedAt:    r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// Comment is one task comment.
type Comment struct {
	UUID      uuid.UUID `json:"uuid"`
	Body      string    `json:"body"`
	Author    *Ref      `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

func validPriority(p string) bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}

func validStatus(s string) bool {
	switch s {
	case StatusOpen, StatusInProgress, StatusDone, StatusCancelled:
		return true
	}
	return false
}

func closedStatus(s string) bool { return s == StatusDone || s == StatusCancelled }

func cleanTitle(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", invalid("title", "is required")
	}
	if utf8.RuneCountInString(v) > maxTitle {
		return "", invalid("title", fmt.Sprintf("must be at most %d characters", maxTitle))
	}
	return v, nil
}

func cleanDescription(v string) (string, error) {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) > maxDescription {
		return "", invalid("description", fmt.Sprintf("must be at most %d characters", maxDescription))
	}
	return v, nil
}

// Filter narrows List (ParseListFilter builds it from the list query).
type Filter struct {
	Statuses        []string // validated; "active" already expanded
	Priorities      []string // validated
	SubjectOrgUUIDs []uuid.UUID
	AssigneeUUID    *uuid.UUID
	// DueAfter / DueBefore bound due_at (inclusive / exclusive, TEC-221);
	// either one leaves tasks without a due date out.
	DueAfter, DueBefore *time.Time
	// CreatedFrom / CreatedBefore bound created_at (inclusive / exclusive).
	CreatedFrom, CreatedBefore *time.Time
	Q                          string
	Sort                       apiquery.ResolvedSort
	Limit, Offset              int32
}

// listParams validates the filter and builds the query arguments. ok=false
// means the assignee is not a member of the center (an empty page).
func (s *Service) listParams(ctx context.Context, c Caller, f Filter) (db.ListTasksParams, bool, error) {
	arg := db.ListTasksParams{
		BrandID: c.Org.BrandID, Statuses: f.Statuses, Priorities: f.Priorities,
		SubjectOrgUuids: f.SubjectOrgUUIDs, Q: textArg(f.Q),
		CreatedFrom: tstz(f.CreatedFrom), CreatedBefore: tstz(f.CreatedBefore),
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	}
	for _, st := range f.Statuses {
		if !validStatus(st) {
			return arg, false, invalid("status", "must be open, in_progress, done, cancelled or active")
		}
	}
	for _, p := range f.Priorities {
		if !validPriority(p) {
			return arg, false, invalid("priority", "must be low, normal, high or urgent")
		}
	}
	if arg.SortKey == "" {
		arg.SortKey, arg.SortDesc = "created_at", true
	}
	if f.AssigneeUUID != nil {
		u, err := s.q.GetCenterMemberByUUID(ctx, db.GetCenterMemberByUUIDParams{Uuid: *f.AssigneeUUID, OrganizationID: c.Org.InternalID})
		if errors.Is(err, pgx.ErrNoRows) {
			return arg, false, nil
		}
		if err != nil {
			return arg, false, fmt.Errorf("tasks: assignee: %w", err)
		}
		arg.AssigneeUserID = pgtype.Int8{Int64: u.ID, Valid: true}
	}
	if f.DueAfter != nil && f.DueBefore != nil && !f.DueBefore.After(*f.DueAfter) {
		return arg, false, invalid("due_before", "must be after due_after")
	}
	arg.DueAfter, arg.DueBefore = tstz(f.DueAfter), tstz(f.DueBefore)
	if arg.RowLimit <= 0 {
		arg.RowLimit = 20
	}
	return arg, true, nil
}

// List lists the tasks of the active brand (default newest first).
func (s *Service) List(ctx context.Context, c Caller, f Filter) ([]Task, int64, error) {
	if err := requireCenter(c); err != nil {
		return nil, 0, err
	}
	arg, ok, err := s.listParams(ctx, c, f)
	if err != nil {
		return nil, 0, err
	}
	if !ok {
		return []Task{}, 0, nil
	}
	rows, err := s.q.ListTasks(ctx, arg)
	if err != nil {
		return nil, 0, fmt.Errorf("tasks: list: %w", err)
	}
	total, err := s.q.CountTasks(ctx, db.CountTasksParams{
		BrandID: arg.BrandID, Statuses: arg.Statuses, Priorities: arg.Priorities,
		SubjectOrgUuids: arg.SubjectOrgUuids, AssigneeUserID: arg.AssigneeUserID,
		DueAfter: arg.DueAfter, DueBefore: arg.DueBefore,
		CreatedFrom: arg.CreatedFrom, CreatedBefore: arg.CreatedBefore, Q: arg.Q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("tasks: count: %w", err)
	}
	out := make([]Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, taskOf(r))
	}
	return out, total, nil
}

// MatchingUUIDs returns up to max tasks of the active center that match the
// list filter (bulk "select all matching", TEC-379).
func (s *Service) MatchingUUIDs(ctx context.Context, c Caller, f Filter, max int32) ([]string, error) {
	if err := requireCenter(c); err != nil {
		return nil, err
	}
	arg, ok, err := s.listParams(ctx, c, f)
	if err != nil || !ok {
		return nil, err
	}
	ids, err := s.q.ListTaskUUIDsFiltered(ctx, db.ListTaskUUIDsFilteredParams{
		BrandID: arg.BrandID, Statuses: arg.Statuses, Priorities: arg.Priorities,
		SubjectOrgUuids: arg.SubjectOrgUuids, AssigneeUserID: arg.AssigneeUserID,
		DueAfter: arg.DueAfter, DueBefore: arg.DueBefore,
		CreatedFrom: arg.CreatedFrom, CreatedBefore: arg.CreatedBefore, Q: arg.Q,
		OrganizationID: c.Org.InternalID, RowLimit: max,
	})
	if err != nil {
		return nil, fmt.Errorf("tasks: matching: %w", err)
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out, nil
}

// Get returns one task of the active brand.
func (s *Service) Get(ctx context.Context, c Caller, id uuid.UUID) (Task, error) {
	if err := requireCenter(c); err != nil {
		return Task{}, err
	}
	return s.view(ctx, s.q, c.Org.BrandID, id)
}

func (s *Service) view(ctx context.Context, q *db.Queries, brandID int64, id uuid.UUID) (Task, error) {
	r, err := q.GetTaskView(ctx, db.GetTaskViewParams{Uuid: id, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("tasks: get: %w", err)
	}
	return taskOf(db.ListTasksRow(r)), nil
}

// subject resolves the distributor or dealer of the active brand a task is
// about; anything else (center, other brand, unknown) is a validation error.
func (s *Service) subject(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (int64, error) {
	o, err := q.GetTaskSubjectOrg(ctx, db.GetTaskSubjectOrgParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, invalid("subject_organization_uuid", "must be a distributor or dealer of the brand")
	}
	if err != nil {
		return 0, fmt.Errorf("tasks: subject: %w", err)
	}
	if o.Type != orgTypeDistributor && o.Type != orgTypeDealer {
		return 0, invalid("subject_organization_uuid", "must be a distributor or dealer of the brand")
	}
	return o.ID, nil
}

// Subject resolves a task subject organization (a distributor or dealer of
// the active center's brand) for a preview (TEC-387, AI confirmation card).
func (s *Service) Subject(ctx context.Context, c Caller, id uuid.UUID) (SubjectRef, error) {
	if err := requireCenter(c); err != nil {
		return SubjectRef{}, err
	}
	o, err := s.q.GetTaskSubjectOrg(ctx, db.GetTaskSubjectOrgParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && o.Type != orgTypeDistributor && o.Type != orgTypeDealer) {
		return SubjectRef{}, invalid("subject_organization_uuid", "must be a distributor or dealer of the brand")
	}
	if err != nil {
		return SubjectRef{}, fmt.Errorf("tasks: subject: %w", err)
	}
	return SubjectRef{UUID: o.Uuid, Name: o.Name, Type: o.Type}, nil
}

// assignee resolves a member of the active center.
func (s *Service) assignee(ctx context.Context, q *db.Queries, c Caller, id *uuid.UUID) (pgtype.Int8, error) {
	if id == nil {
		return pgtype.Int8{}, nil
	}
	u, err := q.GetCenterMemberByUUID(ctx, db.GetCenterMemberByUUIDParams{Uuid: *id, OrganizationID: c.Org.InternalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.Int8{}, invalid("assignee_user_uuid", "must be a member of the center organization")
	}
	if err != nil {
		return pgtype.Int8{}, fmt.Errorf("tasks: assignee: %w", err)
	}
	return pgtype.Int8{Int64: u.ID, Valid: true}, nil
}

func tstz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// CreateInput opens a manual task.
type CreateInput struct {
	SubjectOrgUUID uuid.UUID
	Title          string
	Description    string
	AssigneeUUID   *uuid.UUID
	Priority       string // default normal
	DueAt          *time.Time
}

// Create opens a manual task of the active center.
func (s *Service) Create(ctx context.Context, c Caller, in CreateInput) (Task, error) {
	if err := requireCenter(c); err != nil {
		return Task{}, err
	}
	title, err := cleanTitle(in.Title)
	if err != nil {
		return Task{}, err
	}
	desc, err := cleanDescription(in.Description)
	if err != nil {
		return Task{}, err
	}
	prio := strings.TrimSpace(in.Priority)
	if prio == "" {
		prio = PriorityNormal
	}
	if !validPriority(prio) {
		return Task{}, invalid("priority", "must be low, normal, high or urgent")
	}
	subjectID, err := s.subject(ctx, s.q, c, in.SubjectOrgUUID)
	if err != nil {
		return Task{}, err
	}
	assignee, err := s.assignee(ctx, s.q, c, in.AssigneeUUID)
	if err != nil {
		return Task{}, err
	}

	var out Task
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		t, err := q.InsertTask(ctx, db.InsertTaskParams{
			OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, SubjectOrgID: subjectID,
			Title: title, Description: desc, AssigneeUserID: assignee, Priority: prio,
			DueAt: tstz(in.DueAt), Source: SourceManual, CreatedByUserID: c.actor(),
		})
		if err != nil {
			return fmt.Errorf("tasks: create: %w", err)
		}
		if err := s.record(ctx, tx, events.TasksCreated, c, t, nil); err != nil {
			return err
		}
		if out, err = s.view(ctx, q, c.Org.BrandID, t.Uuid); err != nil {
			return err
		}
		if t.AssigneeUserID.Valid {
			return s.recordAssigned(ctx, tx, c, t, nil, out)
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return out, nil
}

// UpdateInput changes a task; nil fields stay. AssigneeSet/DueAtSet with a
// nil value clear the assignee or the due date.
type UpdateInput struct {
	Title          *string
	Description    *string
	SubjectOrgUUID *uuid.UUID
	Priority       *string
	Status         *string
	AssigneeSet    bool
	AssigneeUUID   *uuid.UUID
	DueAtSet       bool
	DueAt          *time.Time
}

// Update changes a task of the active brand. Moving it to done or cancelled
// closes it (closed_at, closed_by); moving it back reopens it.
func (s *Service) Update(ctx context.Context, c Caller, id uuid.UUID, in UpdateInput) (Task, error) {
	if err := requireCenter(c); err != nil {
		return Task{}, err
	}
	var out Task
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		t, err := q.LockTask(ctx, db.LockTaskParams{Uuid: id, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("tasks: lock: %w", err)
		}
		arg := db.UpdateTaskParams{
			ID: t.ID, SubjectOrgID: t.SubjectOrgID, Title: t.Title, Description: t.Description,
			AssigneeUserID: t.AssigneeUserID, Priority: t.Priority, DueAt: t.DueAt, Status: t.Status,
			ClosedAt: t.ClosedAt, ClosedByUserID: t.ClosedByUserID,
		}
		if in.Title != nil {
			if arg.Title, err = cleanTitle(*in.Title); err != nil {
				return err
			}
		}
		if in.Description != nil {
			if arg.Description, err = cleanDescription(*in.Description); err != nil {
				return err
			}
		}
		if in.SubjectOrgUUID != nil {
			if arg.SubjectOrgID, err = s.subject(ctx, q, c, *in.SubjectOrgUUID); err != nil {
				return err
			}
		}
		if in.Priority != nil {
			p := strings.TrimSpace(*in.Priority)
			if !validPriority(p) {
				return invalid("priority", "must be low, normal, high or urgent")
			}
			arg.Priority = p
		}
		if in.AssigneeSet {
			if arg.AssigneeUserID, err = s.assignee(ctx, q, c, in.AssigneeUUID); err != nil {
				return err
			}
		}
		if in.DueAtSet {
			arg.DueAt = tstz(in.DueAt)
		}
		if in.Status != nil {
			st := strings.TrimSpace(*in.Status)
			if !validStatus(st) {
				return invalid("status", "must be open, in_progress, done or cancelled")
			}
			arg.Status = st
		}
		statusChanged := arg.Status != t.Status
		if statusChanged {
			switch {
			case closedStatus(arg.Status) && !closedStatus(t.Status):
				arg.ClosedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
				arg.ClosedByUserID = c.actor()
			case !closedStatus(arg.Status):
				arg.ClosedAt, arg.ClosedByUserID = pgtype.Timestamptz{}, pgtype.Int8{}
			}
		}
		assigned := arg.AssigneeUserID.Valid && arg.AssigneeUserID != t.AssigneeUserID
		changed := arg.Title != t.Title || arg.Description != t.Description ||
			arg.SubjectOrgID != t.SubjectOrgID || arg.Priority != t.Priority ||
			arg.AssigneeUserID != t.AssigneeUserID || !sameTime(arg.DueAt, t.DueAt)

		if !changed && !statusChanged {
			out, err = s.view(ctx, q, c.Org.BrandID, t.Uuid)
			return err
		}
		updated, err := q.UpdateTask(ctx, arg)
		if err != nil {
			return fmt.Errorf("tasks: update: %w", err)
		}
		prev := &t
		if changed {
			if err := s.record(ctx, tx, events.TasksUpdated, c, updated, prev); err != nil {
				return err
			}
		}
		if statusChanged {
			if err := s.record(ctx, tx, events.TasksStatusChanged, c, updated, prev); err != nil {
				return err
			}
		}
		if out, err = s.view(ctx, q, c.Org.BrandID, updated.Uuid); err != nil {
			return err
		}
		if assigned {
			return s.recordAssigned(ctx, tx, c, updated, prev, out)
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	return out, nil
}

func sameTime(a, b pgtype.Timestamptz) bool {
	if a.Valid != b.Valid {
		return false
	}
	return !a.Valid || a.Time.Equal(b.Time)
}

// ListComments lists the comments of a task, oldest first.
func (s *Service) ListComments(ctx context.Context, c Caller, id uuid.UUID, limit, offset int32) ([]Comment, int64, error) {
	if err := requireCenter(c); err != nil {
		return nil, 0, err
	}
	t, err := s.q.GetTaskByUUID(ctx, db.GetTaskByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("tasks: get: %w", err)
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q.ListTaskComments(ctx, db.ListTaskCommentsParams{TaskID: t.ID, RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("tasks: comments: %w", err)
	}
	total, err := s.q.CountTaskComments(ctx, t.ID)
	if err != nil {
		return nil, 0, fmt.Errorf("tasks: count comments: %w", err)
	}
	out := make([]Comment, 0, len(rows))
	for _, r := range rows {
		out = append(out, Comment{
			UUID: r.Uuid, Body: r.Body, CreatedAt: r.CreatedAt.Time,
			Author: userRef(r.AuthorUuid, r.AuthorName, r.AuthorSurname),
		})
	}
	return out, total, nil
}

// AddComment appends a comment to a task of the active brand.
func (s *Service) AddComment(ctx context.Context, c Caller, id uuid.UUID, body string) (Comment, error) {
	if err := requireCenter(c); err != nil {
		return Comment{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return Comment{}, invalid("body", "is required")
	}
	if utf8.RuneCountInString(body) > maxComment {
		return Comment{}, invalid("body", fmt.Sprintf("must be at most %d characters", maxComment))
	}
	var out Comment
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		t, err := q.GetTaskByUUID(ctx, db.GetTaskByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("tasks: get: %w", err)
		}
		cm, err := q.InsertTaskComment(ctx, db.InsertTaskCommentParams{
			TaskID: t.ID, OrganizationID: t.OrganizationID, BrandID: t.BrandID,
			AuthorUserID: c.actor(), Body: body,
		})
		if err != nil {
			return fmt.Errorf("tasks: comment: %w", err)
		}
		if err := s.recordComment(ctx, tx, c, t, cm); err != nil {
			return err
		}
		out = Comment{UUID: cm.Uuid, Body: cm.Body, CreatedAt: cm.CreatedAt.Time}
		if c.UserID != 0 {
			u, err := q.GetUserByID(ctx, c.UserID)
			if err != nil {
				return fmt.Errorf("tasks: author: %w", err)
			}
			out.Author = &Ref{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}
		}
		return nil
	})
	if err != nil {
		return Comment{}, err
	}
	return out, nil
}

func taskPayload(t db.Task) map[string]any {
	p := map[string]any{
		"task_id":         t.ID,
		"task_uuid":       t.Uuid.String(),
		"organization_id": t.OrganizationID,
		"brand_id":        t.BrandID,
		"subject_org_id":  t.SubjectOrgID,
		"title":           t.Title,
		"priority":        t.Priority,
		"status":          t.Status,
		"source":          t.Source,
	}
	if t.AssigneeUserID.Valid {
		p["assignee_user_id"] = t.AssigneeUserID.Int64
	}
	if t.DueAt.Valid {
		p["due_at"] = t.DueAt.Time.UTC().Format(time.RFC3339)
	}
	return p
}

// record writes a task event (outbox) and its audit row in tx. prev is the
// row before an update (nil on create).
func (s *Service) record(ctx context.Context, tx pgx.Tx, name string, c Caller, t db.Task, prev *db.Task) error {
	payload := taskPayload(t)
	if prev != nil {
		if prev.Status != t.Status {
			payload["previous_status"] = prev.Status
		}
		if prev.AssigneeUserID != t.AssigneeUserID && prev.AssigneeUserID.Valid {
			payload["previous_assignee_user_id"] = prev.AssigneeUserID.Int64
		}
	}
	id, uid := t.ID, t.Uuid
	return s.write(ctx, tx, name, c, taskResource, id, uid, payload)
}

// recordAssigned writes tasks.assigned with what the assignment
// notification shows (TEC-221): the subject organization name next to the
// title, and notify_user_ids (the assignee, unless they assigned
// themselves).
func (s *Service) recordAssigned(ctx context.Context, tx pgx.Tx, c Caller, t db.Task, prev *db.Task, view Task) error {
	payload := taskPayload(t)
	if prev != nil && prev.AssigneeUserID.Valid && prev.AssigneeUserID != t.AssigneeUserID {
		payload["previous_assignee_user_id"] = prev.AssigneeUserID.Int64
	}
	payload["subject_org_name"] = view.Subject.Name
	notify := []int64{}
	if t.AssigneeUserID.Valid && t.AssigneeUserID.Int64 != c.UserID {
		notify = append(notify, t.AssigneeUserID.Int64)
	}
	payload["notify_user_ids"] = notify
	return s.write(ctx, tx, events.TasksAssigned, c, taskResource, t.ID, t.Uuid, payload)
}

// Member is a member of the center (assignee picker).
type Member struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Assignees lists the members of the active center (TEC-221).
func (s *Service) Assignees(ctx context.Context, c Caller) ([]Member, error) {
	if err := requireCenter(c); err != nil {
		return nil, err
	}
	rows, err := s.q.ListCenterMembers(ctx, c.Org.InternalID)
	if err != nil {
		return nil, fmt.Errorf("tasks: members: %w", err)
	}
	out := make([]Member, 0, len(rows))
	for _, r := range rows {
		out = append(out, Member{UUID: r.Uuid, Name: strings.TrimSpace(r.Name + " " + r.Surname)})
	}
	return out, nil
}

func (s *Service) recordComment(ctx context.Context, tx pgx.Tx, c Caller, t db.Task, cm db.TaskComment) error {
	payload := taskPayload(t)
	payload["comment_id"] = cm.ID
	payload["comment_uuid"] = cm.Uuid.String()
	return s.write(ctx, tx, events.TasksCommentAdded, c, commentResource, cm.ID, cm.Uuid, payload)
}

func (s *Service) write(ctx context.Context, tx pgx.Tx, name string, c Caller, resource string,
	id int64, uid uuid.UUID, payload map[string]any) error {
	if s.out != nil {
		ev := events.New(name).WithTenant(c.Org.InternalID).WithEntity(resource, &id, &uid).WithPayload(payload)
		if c.UserID != 0 {
			ev = ev.WithActor(c.UserID)
		}
		if err := s.out.Enqueue(ctx, tx, ev); err != nil {
			return fmt.Errorf("tasks: outbox: %w", err)
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("tasks: audit payload: %w", err)
	}
	if _, err := s.q.WithTx(tx).InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID: c.actor(), Action: name, Resource: resource,
		ResourceUuid: pgtype.UUID{Bytes: uid, Valid: true}, Payload: body,
	}); err != nil {
		return fmt.Errorf("tasks: audit: %w", err)
	}
	return nil
}

func (s *Service) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tasks: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("tasks: commit: %w", err)
	}
	return nil
}
