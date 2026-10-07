package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskOAuthCleanup deletes expired MCP OAuth requests, codes, tokens and
// abandoned client registrations (TEC-400).
const TaskOAuthCleanup = "oauth:cleanup"

// Hourly at minute 23, away from the other maintenance jobs.
const oauthCleanupCron = "23 * * * *"

// OAuthCleanupFunc runs one cleanup pass.
type OAuthCleanupFunc func(ctx context.Context) error

// NewOAuthCleanupTask builds the cleanup task.
func NewOAuthCleanupTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskOAuthCleanup, []byte("{}")), nil
}

func oauthCleanupOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(2), asynq.Timeout(5 * time.Minute)}
}

// WithOAuthCleanup sets the OAuth cleanup processor.
func (w *Worker) WithOAuthCleanup(fn OAuthCleanupFunc) *Worker {
	w.oauthCleanup = fn
	return w
}

func (w *Worker) handleOAuthCleanup(ctx context.Context, _ *asynq.Task) error {
	if w.oauthCleanup == nil {
		w.log.Warn("oauth_cleanup_handler_missing")
		return nil
	}
	return w.oauthCleanup(ctx)
}
