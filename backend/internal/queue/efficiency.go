package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

const TaskEfficiencyNetworkRefresh = "efficiency:network_refresh"
const efficiencyNetworkRefreshCron = "23 4 * * 1"

type EfficiencyNetworkRefreshFunc func(ctx context.Context) error

func NewEfficiencyNetworkRefreshTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskEfficiencyNetworkRefresh, []byte("{}")), nil
}

func efficiencyNetworkRefreshOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(2), asynq.Timeout(20 * time.Minute), asynq.Unique(23 * time.Hour)}
}

func (w *Worker) WithEfficiencyNetworkRefresh(fn EfficiencyNetworkRefreshFunc) *Worker {
	w.efficiencyNetworkRefresh = fn
	return w
}

func (w *Worker) handleEfficiencyNetworkRefresh(ctx context.Context, _ *asynq.Task) error {
	if w.efficiencyNetworkRefresh == nil {
		w.log.Warn("efficiency_network_refresh_handler_missing")
		return nil
	}
	return w.efficiencyNetworkRefresh(ctx)
}
