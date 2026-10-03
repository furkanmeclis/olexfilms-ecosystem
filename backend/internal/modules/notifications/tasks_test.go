package notifications

import (
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

// TEC-221: tasks.assigned / tasks.due_soon / tasks.overdue map to
// registered catalog events; the dispatch goes to notify_user_ids (as JSON
// decodes them) with the task variables; no recipient sends nothing.
func TestTaskDispatch(t *testing.T) {
	if len(TaskEventCodes) != 3 {
		t.Fatalf("mapped events = %d, want 3", len(TaskEventCodes))
	}
	for name, code := range TaskEventCodes {
		if _, ok := catalog.Lookup(code); !ok {
			t.Fatalf("%s -> %s: not in the catalog", name, code)
		}
	}
	assigned := events.New(events.TasksAssigned).WithPayload(map[string]any{
		"notify_user_ids": []any{float64(12)}, "brand_id": float64(2),
		"task_uuid": "t-1", "title": "Ziyaret", "subject_org_name": "Kuzey Oto", "priority": "high",
	})
	in, ok := taskDispatcher(catalog.EventTaskAssigned)(assigned)
	if !ok {
		t.Fatal("want a dispatch")
	}
	if in.EventCode != catalog.EventTaskAssigned || !slices.Equal(in.UserIDs, []int64{12}) {
		t.Fatalf("dispatch = %+v", in)
	}
	if in.BrandID == nil || *in.BrandID != 2 {
		t.Fatalf("brand = %v", in.BrandID)
	}
	if in.Vars["task_title"] != "Ziyaret" || in.Vars["subject_name"] != "Kuzey Oto" {
		t.Fatalf("vars = %v", in.Vars)
	}
	if _, has := in.Vars["due_date"]; has {
		t.Fatalf("assignment has no due_date variable: %v", in.Vars)
	}

	overdue := events.New(events.TasksOverdue).WithPayload(map[string]any{
		"notify_user_ids": []int64{5}, "title": "Ziyaret", "subject_org_name": "Kuzey Oto",
		"due_date": "2026-10-03 17:00",
	})
	in, ok = taskDispatcher(catalog.EventTaskOverdue)(overdue)
	if !ok || in.Vars["due_date"] != "2026-10-03 17:00" || in.Priority != notifmodel.PriorityHigh {
		t.Fatalf("overdue dispatch = %+v (%v)", in, ok)
	}

	if _, ok := taskDispatcher(catalog.EventTaskAssigned)(events.New(events.TasksAssigned).WithPayload(map[string]any{
		"notify_user_ids": []any{},
	})); ok {
		t.Fatal("no recipient must send nothing")
	}
}
