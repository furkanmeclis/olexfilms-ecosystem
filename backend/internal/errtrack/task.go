package errtrack

import (
	"context"
	"strconv"

	"github.com/getsentry/sentry-go"
)

// TaskInfo describes a background task for error reports.
type TaskInfo struct {
	Type     string
	Queue    string
	Retry    int
	MaxRetry int
	// Scheduler marks errors raised while enqueueing periodic tasks.
	Scheduler bool
}

// CaptureTask reports a failed background task (asynq ErrorHandler,
// scheduler enqueue errors). The module comes from the task type
// (app:notification:deliver → notification) unless pinned on ctx.
func CaptureTask(ctx context.Context, task TaskInfo, err error) *sentry.EventID {
	if !Enabled() || err == nil {
		return nil
	}
	tags := Tags{
		TagTaskType: task.Type,
		TagQueue:    task.Queue,
	}
	if task.Scheduler {
		tags[TagComponent] = "scheduler"
	} else {
		tags["retry"] = strconv.Itoa(task.Retry)
		tags["max_retry"] = strconv.Itoa(task.MaxRetry)
	}
	return Capture(ctx, ModuleForTask(task.Type), err, tags)
}
