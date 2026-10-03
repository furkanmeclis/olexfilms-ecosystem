package db_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	tasksusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/google/uuid"
)

// TEC-221 (F1-11b): task notifications against the real schema. The use
// case and the cron run in savepoints of the fixture transaction, so
// everything is rolled back at the end.

func (f *orderFixture) centerMember(t *testing.T, name string) db.User {
	t.Helper()
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: name, Surname: "T221", Status: "active",
		Email: text(fmt.Sprintf("t221-%s-%d@example.test", name, time.Now().UnixNano())),
	})
	if err != nil {
		t.Fatalf("user %s: %v", name, err)
	}
	if _, err := f.tx.Exec(f.ctx, `INSERT INTO organization_members (organization_id, user_id, role) VALUES ($1, $2, 'staff')`,
		f.centerID, u.ID); err != nil {
		t.Fatalf("member %s: %v", name, err)
	}
	return u
}

// taskEvents returns the notify_user_ids of every outbox row of an event
// for one task.
func (f *orderFixture) taskEvents(t *testing.T, name string, taskUUID uuid.UUID) [][]int64 {
	t.Helper()
	rows, err := f.tx.Query(f.ctx, `SELECT COALESCE(payload->'data'->'notify_user_ids', '[]'::jsonb)
		FROM outbox_events WHERE event_name = $1 AND payload->'data'->>'task_uuid' = $2 ORDER BY id`,
		name, taskUUID.String())
	if err != nil {
		t.Fatalf("outbox: %v", err)
	}
	defer rows.Close()
	var out [][]int64
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var ids []int64
		if err := json.Unmarshal(raw, &ids); err != nil {
			t.Fatalf("notify_user_ids %s: %v", raw, err)
		}
		out = append(out, ids)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTaskNotifications(t *testing.T) {
	f := newOrderFixture(t)
	creator := f.centerMember(t, "creator")
	assignee := f.centerMember(t, "assignee")
	svc := tasksusecase.New(f.tx, f.q, outbox.NewStore(nil, f.q))
	cron := tasksusecase.NewCron(f.tx, f.q, outbox.NewStore(nil, f.q))
	c := tasksusecase.Caller{UserID: creator.ID, Org: orgctx.Scope{InternalID: f.centerID, OrgType: "center", BrandID: f.brandID}}
	now := time.Now()

	open := func(t *testing.T, title string, assigneeUUID *uuid.UUID, due *time.Time) tasksusecase.Task {
		t.Helper()
		task, err := svc.Create(f.ctx, c, tasksusecase.CreateInput{
			SubjectOrgUUID: f.dealer.Uuid, Title: title, AssigneeUUID: assigneeUUID, DueAt: due,
		})
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		return task
	}

	t.Run("assignment queues a notification for the assignee", func(t *testing.T) {
		task := open(t, "Assigned", &assignee.Uuid, nil)
		got := f.taskEvents(t, events.TasksAssigned, task.UUID)
		if len(got) != 1 || !slices.Equal(got[0], []int64{assignee.ID}) {
			t.Fatalf("tasks.assigned notify = %v, want [[%d]]", got, assignee.ID)
		}
		var subject string
		if err := f.tx.QueryRow(f.ctx, `SELECT payload->'data'->>'subject_org_name' FROM outbox_events
			WHERE event_name = $1 AND payload->'data'->>'task_uuid' = $2`, events.TasksAssigned, task.UUID.String()).Scan(&subject); err != nil {
			t.Fatal(err)
		}
		if subject != f.dealer.Name {
			t.Fatalf("subject_org_name = %q, want %q", subject, f.dealer.Name)
		}
		// Self-assignment notifies nobody.
		self := open(t, "Self", &creator.Uuid, nil)
		if got := f.taskEvents(t, events.TasksAssigned, self.UUID); len(got) != 1 || len(got[0]) != 0 {
			t.Fatalf("self assignment notify = %v, want one empty list", got)
		}
		// Reassigning through Update notifies the new assignee.
		reassign := open(t, "Reassign", nil, nil)
		if _, err := svc.Update(f.ctx, c, reassign.UUID, tasksusecase.UpdateInput{AssigneeSet: true, AssigneeUUID: &assignee.Uuid}); err != nil {
			t.Fatalf("assign: %v", err)
		}
		if got := f.taskEvents(t, events.TasksAssigned, reassign.UUID); len(got) != 1 || !slices.Equal(got[0], []int64{assignee.ID}) {
			t.Fatalf("update tasks.assigned notify = %v", got)
		}
	})

	t.Run("cron reminds due soon and overdue tasks once", func(t *testing.T) {
		overdueAt := now.Add(-2 * time.Hour)
		soonAt := now.Add(3 * time.Hour)
		farAt := now.Add(72 * time.Hour)
		overdue := open(t, "Overdue", &assignee.Uuid, &overdueAt)
		soon := open(t, "Soon", nil, &soonAt) // unassigned: the creator is reminded
		far := open(t, "Far", &assignee.Uuid, &farAt)
		done := open(t, "Done", &assignee.Uuid, &overdueAt)
		st := "done"
		if _, err := svc.Update(f.ctx, c, done.UUID, tasksusecase.UpdateInput{Status: &st}); err != nil {
			t.Fatalf("close: %v", err)
		}

		if _, err := cron.NotifyDue(f.ctx, now); err != nil {
			t.Fatalf("scan: %v", err)
		}
		check := func(t *testing.T, name string, task tasksusecase.Task, want [][]int64) {
			t.Helper()
			got := f.taskEvents(t, name, task.UUID)
			if len(got) != len(want) {
				t.Fatalf("%s %s rows = %v, want %v", task.Title, name, got, want)
			}
			for i := range want {
				if !slices.Equal(got[i], want[i]) {
					t.Fatalf("%s %s rows = %v, want %v", task.Title, name, got, want)
				}
			}
		}
		check(t, events.TasksOverdue, overdue, [][]int64{{assignee.ID}})
		check(t, events.TasksDueSoon, overdue, nil)
		check(t, events.TasksDueSoon, soon, [][]int64{{creator.ID}})
		check(t, events.TasksOverdue, soon, nil)
		check(t, events.TasksDueSoon, far, nil)
		check(t, events.TasksOverdue, done, nil)

		// Second run: nothing new.
		if _, err := cron.NotifyDue(f.ctx, now.Add(time.Minute)); err != nil {
			t.Fatalf("second scan: %v", err)
		}
		check(t, events.TasksOverdue, overdue, [][]int64{{assignee.ID}})
		check(t, events.TasksDueSoon, soon, [][]int64{{creator.ID}})

		// Past its deadline the due soon task gets its overdue reminder once.
		later := soonAt.Add(time.Hour)
		if _, err := cron.NotifyDue(f.ctx, later); err != nil {
			t.Fatalf("later scan: %v", err)
		}
		if _, err := cron.NotifyDue(f.ctx, later.Add(time.Minute)); err != nil {
			t.Fatalf("later second scan: %v", err)
		}
		check(t, events.TasksOverdue, soon, [][]int64{{creator.ID}})
		check(t, events.TasksDueSoon, soon, [][]int64{{creator.ID}})

		// A new deadline clears the stamps: it is reminded again.
		newDue := now.Add(2 * time.Hour)
		if _, err := svc.Update(f.ctx, c, overdue.UUID, tasksusecase.UpdateInput{DueAtSet: true, DueAt: &newDue}); err != nil {
			t.Fatalf("move due: %v", err)
		}
		if _, err := cron.NotifyDue(f.ctx, now.Add(2*time.Minute)); err != nil {
			t.Fatalf("scan after move: %v", err)
		}
		check(t, events.TasksDueSoon, overdue, [][]int64{{assignee.ID}})

		// The payload carries the template variables.
		var title, subject, dueDate string
		if err := f.tx.QueryRow(f.ctx, `SELECT payload->'data'->>'title', payload->'data'->>'subject_org_name',
				payload->'data'->>'due_date'
			FROM outbox_events WHERE event_name = $1 AND payload->'data'->>'task_uuid' = $2`,
			events.TasksOverdue, overdue.UUID.String()).Scan(&title, &subject, &dueDate); err != nil {
			t.Fatal(err)
		}
		if title != "Overdue" || subject != f.dealer.Name || dueDate == "" {
			t.Fatalf("payload title=%q subject=%q due=%q", title, subject, dueDate)
		}
	})

	t.Run("list filters by due date", func(t *testing.T) {
		from, to := now.Add(-time.Minute), now.Add(48*time.Hour)
		items, _, err := svc.List(f.ctx, c, tasksusecase.Filter{DueAfter: &from, DueBefore: &to, Limit: 100})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, it := range items {
			if it.DueAt == nil || it.DueAt.Before(from) || !it.DueAt.Before(to) {
				t.Fatalf("task %s due %v outside [%v, %v)", it.Title, it.DueAt, from, to)
			}
		}
		if len(items) == 0 {
			t.Fatal("want the tasks due within two days")
		}
	})

	t.Run("assignees lists the center members", func(t *testing.T) {
		members, err := svc.Assignees(f.ctx, c)
		if err != nil {
			t.Fatalf("assignees: %v", err)
		}
		var seen int
		for _, m := range members {
			if m.UUID == creator.Uuid || m.UUID == assignee.Uuid {
				seen++
			}
		}
		if seen != 2 {
			t.Fatalf("assignees = %+v, want both members", members)
		}
	})
}
