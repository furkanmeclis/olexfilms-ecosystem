package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskServiceSubscriptionsExpire expires service subscriptions past ends_on
// and closes the modules of expired module bundles (TEC-508).
const TaskServiceSubscriptionsExpire = "service_subscriptions:expire"

// Hourly and idempotent: a subscription already expired is not touched.
const serviceSubscriptionsExpireCron = "41 * * * *"

type ServiceSubscriptionsExpireFunc func(ctx context.Context) error

func NewServiceSubscriptionsExpireTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskServiceSubscriptionsExpire, []byte("{}")), nil
}

func serviceSubscriptionsExpireOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(10 * time.Minute), asynq.Unique(50 * time.Minute)}
}

func (w *Worker) WithServiceSubscriptionsExpire(fn ServiceSubscriptionsExpireFunc) *Worker {
	w.serviceSubscriptionsExpire = fn
	return w
}

func (w *Worker) handleServiceSubscriptionsExpire(ctx context.Context, _ *asynq.Task) error {
	if w.serviceSubscriptionsExpire == nil {
		w.log.Warn("service_subscriptions_expire_handler_missing")
		return nil
	}
	return w.serviceSubscriptionsExpire(ctx)
}
