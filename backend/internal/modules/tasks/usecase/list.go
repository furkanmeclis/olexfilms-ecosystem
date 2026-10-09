package usecase

// TEC-379 (DT-BE-8): list contract of GET /v1/tasks (docs/list-contract.md).
// The list endpoint and the "select all matching" target of the task bulk
// actions read the same parameters with ParseListFilter, so both select the
// same rows.

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListSort is the sort contract of the task list. status and priority sort
// by their rank (open → cancelled, low → urgent), subject by the subject
// organization name; due_at keeps tasks without a deadline last.
var ListSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"title": "title", "subject": "subject", "status": "status", "priority": "priority",
		"due_at": "due_at", "created_at": "created_at", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// StatusActive is the list filter value for open + in_progress.
const StatusActive = "active"

// Statuses and Priorities are the task enums (filters and bulk params).
var (
	Statuses   = []string{StatusOpen, StatusInProgress, StatusDone, StatusCancelled}
	Priorities = []string{PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent}
	// Sources is the source filter enum (TEC-496: auto = performance rules).
	Sources = []string{SourceManual, SourceAuto}
)

// List query keys.
const (
	QueryStatus         = "status"
	QueryPriority       = "priority"
	QuerySource         = "source"
	QuerySubjectOrgUUID = "subject_organization_uuid"
	QueryAssigneeUUID   = "assignee_user_uuid"
	QueryMine           = "mine"
)

// ParseListFilter reads status (CSV, "active" = open + in_progress),
// priority (CSV), source (CSV: manual, auto), subject_organization_uuid (CSV), assignee_user_uuid,
// mine (true: assignee = me; me nil rejects it), due_after / due_before
// (RFC 3339) or due_from / due_to, created_from / created_to, q, sort,
// limit and offset. Errors are *apiquery.ValidationError (400).
func ParseListFilter(values url.Values, me *uuid.UUID) (Filter, error) {
	q := apiquery.Parse(values)
	f := Filter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	statuses, err := apiquery.EnumList(values, QueryStatus, append(slices.Clone(Statuses), StatusActive)...)
	if err != nil {
		return f, err
	}
	for _, s := range statuses {
		if s == StatusActive {
			f.Statuses = append(f.Statuses, StatusOpen, StatusInProgress)
		} else {
			f.Statuses = append(f.Statuses, s)
		}
	}
	slices.Sort(f.Statuses)
	f.Statuses = slices.Compact(f.Statuses)
	if f.Priorities, err = apiquery.EnumList(values, QueryPriority, Priorities...); err != nil {
		return f, err
	}
	if f.Sources, err = apiquery.EnumList(values, QuerySource, Sources...); err != nil {
		return f, err
	}
	for _, raw := range apiquery.CSVValues(values, QuerySubjectOrgUUID) {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, listInvalid(QuerySubjectOrgUUID, "must be a list of UUIDs")
		}
		f.SubjectOrgUUIDs = append(f.SubjectOrgUUIDs, id)
	}
	if raw := strings.TrimSpace(values.Get(QueryAssigneeUUID)); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, listInvalid(QueryAssigneeUUID, "must be a UUID")
		}
		f.AssigneeUUID = &id
	}
	mine, err := apiquery.Bool(values, QueryMine)
	if err != nil {
		return f, err
	}
	if mine != nil && *mine {
		if me == nil {
			return f, listInvalid(QueryMine, "is not supported here; use assignee_user_uuid")
		}
		id := *me
		f.AssigneeUUID = &id
	}
	if f.DueAfter, err = rfc3339(values, "due_after"); err != nil {
		return f, err
	}
	if f.DueBefore, err = rfc3339(values, "due_before"); err != nil {
		return f, err
	}
	due, err := apiquery.DateRange(values, "due")
	if err != nil {
		return f, err
	}
	if due.From != nil {
		if f.DueAfter != nil {
			return f, listInvalid("due_from", "cannot be combined with due_after")
		}
		f.DueAfter = due.From
	}
	if due.Before != nil {
		if f.DueBefore != nil {
			return f, listInvalid("due_to", "cannot be combined with due_before")
		}
		f.DueBefore = due.Before
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return f, err
	}
	f.CreatedFrom, f.CreatedBefore = created.From, created.Before
	if f.Sort, err = apiquery.ResolveSort(q.Sort, ListSort); err != nil {
		return f, err
	}
	return f, nil
}

func listInvalid(field, msg string) error {
	return &apiquery.ValidationError{Details: []apiquery.Detail{{Field: field, Message: msg}}}
}

// rfc3339 reads an optional RFC 3339 timestamp parameter.
func rfc3339(values url.Values, key string) (*time.Time, error) {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return nil, nil
	}
	v, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, listInvalid(key, "must be an RFC 3339 date-time")
	}
	return &v, nil
}

// escapeLike escapes the LIKE wildcards of a search term.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func textArg(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: escapeLike(s), Valid: true}
}
