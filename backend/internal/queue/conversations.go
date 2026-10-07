package queue

import (
	"context"

	"github.com/hibiken/asynq"
)

// TaskConversationAIRunPurge deletes WhatsApp conversation AI runs older than
// the 90-day retention (TEC-393, QUESTIONS #15). Messages are kept.
const TaskConversationAIRunPurge = "app:conversations:ai_runs_purge"

const conversationAIRunPurgeCron = "@every 6h"

// ConversationAIRunPurgeFunc runs one retention sweep.
type ConversationAIRunPurgeFunc func(ctx context.Context) (int64, error)

// NewConversationAIRunPurgeTask builds the periodic sweep task. The sweep
// deletes in batches and is idempotent, so a failed run is retried once.
func NewConversationAIRunPurgeTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskConversationAIRunPurge, []byte("{}"), asynq.MaxRetry(1)), nil
}

// WithConversationAIRunPurge sets the sweep processor. The task handler is
// registered once in NewWorkerWithQueues, so a repeated call replaces fn.
func (w *Worker) WithConversationAIRunPurge(fn ConversationAIRunPurgeFunc) *Worker {
	w.purgeConversationAIRuns = fn
	return w
}

func (w *Worker) handleConversationAIRunPurge(ctx context.Context, _ *asynq.Task) error {
	if w.purgeConversationAIRuns == nil {
		w.log.Warn("conversation_ai_run_purge_handler_missing")
		return nil
	}
	n, err := w.purgeConversationAIRuns(ctx)
	if err == nil && n > 0 {
		w.log.Info("conversation_ai_runs_purged", "deleted", n)
	}
	return err
}
