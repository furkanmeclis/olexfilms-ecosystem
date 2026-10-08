package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
)

type captureEnqueue struct {
	tasks []*asynq.Task
	opts  [][]asynq.Option
	err   error
}

func (c *captureEnqueue) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	c.tasks = append(c.tasks, task)
	c.opts = append(c.opts, opts)
	return &asynq.TaskInfo{}, c.err
}

// Acceptance (TEC-476): a report generation runs on the docs queue
// (worker-docs) with three retries, one queued task per report.
func TestFleetReportEnqueueOptions(t *testing.T) {
	c := &captureEnqueue{}
	if err := (FleetReportEnqueuer{Client: c}).EnqueueFleetReport(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	if len(c.tasks) != 1 || c.tasks[0].Type() != TaskFleetReportGenerate || string(c.tasks[0].Payload()) != `{"report_id":42}` {
		t.Fatalf("task = %+v", c.tasks)
	}
	got := map[asynq.OptionType]any{}
	for _, o := range c.opts[0] {
		got[o.Type()] = o.Value()
	}
	if got[asynq.QueueOpt] != QueueDocs || got[asynq.MaxRetryOpt] != 3 || got[asynq.TaskIDOpt] != "fleet-report-42" {
		t.Fatalf("options = %v", got)
	}
	// A task already queued for the report is not an error.
	c.err = asynq.ErrTaskIDConflict
	if err := (FleetReportEnqueuer{Client: c}).EnqueueFleetReport(context.Background(), 42); err != nil {
		t.Fatalf("conflict: %v", err)
	}
	c.err = errors.New("redis down")
	if err := (FleetReportEnqueuer{Client: c}).EnqueueFleetReport(context.Background(), 42); err == nil {
		t.Fatal("enqueue error must surface")
	}
}

// The schedule tick runs hourly on worker-core (maintenance queue).
func TestFleetReportsScheduleIsHourly(t *testing.T) {
	for _, p := range Schedules() {
		if p.Type != TaskFleetReportsSchedule {
			continue
		}
		if p.Cron != "7 * * * *" || p.Queue != QueueMaintenance {
			t.Fatalf("schedule = %+v", p)
		}
		return
	}
	t.Fatal("fleet report schedule missing")
}

// The generation handler tells the use case when the attempt is the last.
func TestFleetReportHandlerPassesFinalAttempt(t *testing.T) {
	w := &Worker{}
	var finals []bool
	w.WithFleetReports(nil, func(_ context.Context, id int64, final bool) error {
		if id != 7 {
			t.Fatalf("id = %d", id)
		}
		finals = append(finals, final)
		return nil
	})
	task := asynq.NewTask(TaskFleetReportGenerate, []byte(`{"report_id":7}`))
	if err := w.handleFleetReportGenerate(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(finals) != 1 || finals[0] {
		t.Fatalf("outside asynq the attempt is not final: %v", finals)
	}
}
