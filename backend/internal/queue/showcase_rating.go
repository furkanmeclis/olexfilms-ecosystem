package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskShowcaseGoogleRating refreshes dealer showcase Google ratings from
// the Places API (TEC-469). A no-op without GOOGLE_PLACES_API_KEY.
const TaskShowcaseGoogleRating = "app:showcase:google_rating_refresh"

// Daily at 04:40 Europe/Istanbul on the low (maintenance) queue.
const showcaseGoogleRatingCron = "40 4 * * *"

// ShowcaseGoogleRatingFunc runs one refresh.
type ShowcaseGoogleRatingFunc func(ctx context.Context) error

// NewShowcaseGoogleRatingTask builds the refresh task.
func NewShowcaseGoogleRatingTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskShowcaseGoogleRating, []byte("{}")), nil
}

// Failures are backed off per organization inside the run, so the task
// itself is retried only once (database errors).
func showcaseGoogleRatingOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(1), asynq.Timeout(30 * time.Minute)}
}

// WithShowcaseGoogleRating sets the refresh processor.
func (w *Worker) WithShowcaseGoogleRating(fn ShowcaseGoogleRatingFunc) *Worker {
	w.showcaseGoogleRating = fn
	return w
}

func (w *Worker) handleShowcaseGoogleRating(ctx context.Context, _ *asynq.Task) error {
	if w.showcaseGoogleRating == nil {
		w.log.Warn("showcase_google_rating_handler_missing")
		return nil
	}
	return w.showcaseGoogleRating(ctx)
}
