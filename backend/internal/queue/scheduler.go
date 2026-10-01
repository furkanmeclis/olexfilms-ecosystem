package queue

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/hibiken/asynq"
)

const logPurgeCron = "@every 5m"

// StartLogPurgeScheduler registers a periodic sweep for due log retention rules.
func StartLogPurgeScheduler(cfg config.Config, log *slog.Logger) (*asynq.Scheduler, error) {
	if log == nil {
		log = slog.Default()
	}
	scheduler := asynq.NewScheduler(RedisOpt(cfg.Redis), &asynq.SchedulerOpts{
		// PostEnqueueFunc gets a nil TaskInfo on failure (no task type), so
		// the deprecated error handler stays the only hook with task + error.
		EnqueueErrorHandler: SchedulerErrorHandler(log), //nolint:staticcheck // see above
	})
	task, err := NewLogPurgeSweepTask()
	if err != nil {
		return nil, fmt.Errorf("queue: log purge task: %w", err)
	}
	if _, err := scheduler.Register(logPurgeCron, task, asynq.Queue(QueueMaintenance)); err != nil {
		return nil, fmt.Errorf("queue: register log purge schedule: %w", err)
	}
	log.Info("queue_log_purge_scheduler_registered", "cron", logPurgeCron)
	return scheduler, nil
}

// SchedulerErrorHandler logs and reports periodic task enqueue failures.
func SchedulerErrorHandler(log *slog.Logger) func(task *asynq.Task, opts []asynq.Option, err error) {
	if log == nil {
		log = slog.Default()
	}
	return func(task *asynq.Task, opts []asynq.Option, err error) {
		taskType := ""
		if task != nil {
			taskType = task.Type()
		}
		queueName := "default"
		for _, o := range opts {
			if o.Type() == asynq.QueueOpt {
				if q, ok := o.Value().(string); ok {
					queueName = q
				}
			}
		}
		log.Error("queue_schedule_enqueue_failed", "type", taskType, "error", err)
		errtrack.CaptureTask(context.Background(), errtrack.TaskInfo{
			Type: taskType, Queue: queueName, Scheduler: true,
		}, err)
	}
}
