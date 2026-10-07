package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// warranty_claim:ai_triage runs on the default queue with the claim uuid
// as task id; a task already queued for the claim is not an error.
func TestWarrantyClaimTriageTaskID(t *testing.T) {
	c := &recordingClient{}
	claim := uuid.New()
	if err := (WarrantyTriageEnqueuer{Client: c}).EnqueueClaimTriage(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	if len(c.tasks) != 1 || c.tasks[0].Type() != TaskWarrantyClaimTriage {
		t.Fatalf("tasks = %+v", c.tasks)
	}
	var p WarrantyClaimTriagePayload
	if err := json.Unmarshal(c.tasks[0].Payload(), &p); err != nil || p.ClaimUUID != claim {
		t.Fatalf("payload %s %v", c.tasks[0].Payload(), err)
	}
	var queue, id string
	for _, o := range c.opts[0] {
		switch o.Type() {
		case asynq.QueueOpt:
			queue = o.Value().(string)
		case asynq.TaskIDOpt:
			id = o.Value().(string)
		}
	}
	if queue != "default" || id != TaskWarrantyClaimTriage+":"+claim.String() {
		t.Fatalf("queue %q id %q", queue, id)
	}

	c.err = asynq.ErrTaskIDConflict
	if err := (WarrantyTriageEnqueuer{Client: c}).EnqueueClaimTriage(context.Background(), claim); err != nil {
		t.Fatalf("conflicting task id: %v", err)
	}
}
