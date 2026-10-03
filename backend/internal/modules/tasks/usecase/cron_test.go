package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-221: the reminder goes to the assignee, or to the creator of an
// unassigned task; the payload carries the template variables and the due
// date in the center's zone.
func TestDueEventRecipientsAndPayload(t *testing.T) {
	due := time.Date(2026, 10, 3, 14, 0, 0, 0, time.UTC)
	row := db.ClaimTasksOverdueRow{
		ID: 9, Uuid: uuid.New(), OrganizationID: 1, BrandID: 2, Title: "Ziyaret", Priority: "high",
		DueAt:           pgtype.Timestamptz{Time: due, Valid: true},
		AssigneeUserID:  pgtype.Int8{Int64: 5, Valid: true},
		CreatedByUserID: pgtype.Int8{Int64: 3, Valid: true},
		Timezone:        "Europe/Istanbul", SubjectName: "Kuzey Oto",
	}
	ev := DueEvent(events.TasksOverdue, row)
	if ev.Name != events.TasksOverdue || ev.TenantID == nil || *ev.TenantID != 1 {
		t.Fatalf("event = %s tenant %v", ev.Name, ev.TenantID)
	}
	if ids, _ := ev.Payload["notify_user_ids"].([]int64); !slices.Equal(ids, []int64{5}) {
		t.Fatalf("assigned notify = %v", ev.Payload["notify_user_ids"])
	}
	if ev.Payload["subject_org_name"] != "Kuzey Oto" || ev.Payload["title"] != "Ziyaret" || ev.Payload["brand_id"] != int64(2) {
		t.Fatalf("payload = %v", ev.Payload)
	}
	if ev.Payload["due_date"] != "2026-10-03 17:00" {
		t.Fatalf("due_date = %v", ev.Payload["due_date"])
	}

	row.AssigneeUserID = pgtype.Int8{}
	if ids, _ := DueEvent(events.TasksDueSoon, row).Payload["notify_user_ids"].([]int64); !slices.Equal(ids, []int64{3}) {
		t.Fatalf("unassigned notify = %v", ids)
	}
}

func TestDueDateFallsBackToUTC(t *testing.T) {
	due := time.Date(2026, 10, 3, 14, 5, 0, 0, time.UTC)
	if got := DueDate(due, "Mars/Base"); got != "2026-10-03 14:05" {
		t.Fatalf("fallback = %s", got)
	}
	if got := DueDate(due, ""); got != "2026-10-03 14:05" {
		t.Fatalf("empty zone = %s", got)
	}
}

func TestListRejectsAnEmptyDueRange(t *testing.T) {
	s := New(nil, nil, nil)
	c := Caller{UserID: 1, Org: orgctx.Scope{InternalID: 7, OrgType: "center", BrandID: 1}}
	from := time.Now()
	to := from.Add(-time.Hour)
	_, _, err := s.List(context.Background(), c, Filter{DueAfter: &from, DueBefore: &to})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "due_before" {
		t.Fatalf("got %v", err)
	}
}

func TestAssigneesNeedsTheCenter(t *testing.T) {
	s := New(nil, nil, nil)
	c := Caller{UserID: 1, Org: orgctx.Scope{InternalID: 7, OrgType: "dealer", BrandID: 1}}
	if _, err := s.Assignees(context.Background(), c); !errors.Is(err, ErrCenterOnly) {
		t.Fatalf("got %v", err)
	}
}
