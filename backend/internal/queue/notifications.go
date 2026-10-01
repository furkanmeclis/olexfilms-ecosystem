package queue

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// TaskNotificationPurge sweeps notifications and deliveries older than the
// 90-day retention (TEC-87).
const TaskNotificationPurge = "app:notifications:purge"

const notificationPurgeCron = "@every 1h"

// NotificationPurgeFunc runs one retention sweep.
type NotificationPurgeFunc func(ctx context.Context) (int64, error)

// NewNotificationPurgeTask builds the periodic sweep task. The sweep deletes
// in batches and is idempotent, so a failed run is retried once at most.
func NewNotificationPurgeTask() *asynq.Task {
	return asynq.NewTask(TaskNotificationPurge, []byte("{}"), asynq.MaxRetry(1))
}

// WithNotificationPurge sets the sweep processor. The task handler itself is
// registered once in NewWorkerWithQueues, so a repeated call replaces fn.
func (w *Worker) WithNotificationPurge(fn NotificationPurgeFunc) *Worker {
	w.purgeNotifications = fn
	return w
}

func (w *Worker) handleNotificationPurge(ctx context.Context, _ *asynq.Task) error {
	if w.purgeNotifications == nil {
		w.log.Warn("notification_purge_handler_missing")
		return nil
	}
	_, err := w.purgeNotifications(ctx)
	return err
}

// RegisterNotificationPurge adds the hourly sweep to a scheduler.
func RegisterNotificationPurge(s *asynq.Scheduler) error {
	if _, err := s.Register(notificationPurgeCron, NewNotificationPurgeTask(), asynq.Queue(QueueMaintenance)); err != nil {
		return fmt.Errorf("queue: register notification purge: %w", err)
	}
	return nil
}
