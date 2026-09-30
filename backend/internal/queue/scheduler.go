package queue

import (
	"fmt"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/hibiken/asynq"
)

const logPurgeCron = "@every 5m"

// StartLogPurgeScheduler registers a periodic sweep for due log retention rules.
func StartLogPurgeScheduler(cfg config.Config, log *slog.Logger) (*asynq.Scheduler, error) {
	if log == nil {
		log = slog.Default()
	}
	scheduler := asynq.NewScheduler(RedisOpt(cfg.Redis), nil)
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
