package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskServiceReviewRequest sends the Google review request of one completed
// service (TEC-192, F1-06h). It is enqueued with ProcessIn when
// service.completed is consumed (default 24 hours); the handler re-checks
// every condition and stamps services.review_request_sent_at, so a second
// run sends nothing.
const TaskServiceReviewRequest = "service:review_request"

// ServiceReviewRequestPayload names the service of a review request task.
type ServiceReviewRequestPayload struct {
	ServiceID int64 `json:"service_id"`
}

// ServiceReviewRequestFunc runs the review request of one service.
type ServiceReviewRequestFunc func(ctx context.Context, serviceID int64) error

// NewServiceReviewRequestTask builds the review request task of a service.
func NewServiceReviewRequestTask(serviceID int64) (*asynq.Task, error) {
	body, err := json.Marshal(ServiceReviewRequestPayload{ServiceID: serviceID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal service review request: %w", err)
	}
	return asynq.NewTask(TaskServiceReviewRequest, body), nil
}

// ParseServiceReviewRequestPayload decodes a review request payload.
func ParseServiceReviewRequestPayload(data []byte) (ServiceReviewRequestPayload, error) {
	var payload ServiceReviewRequestPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return ServiceReviewRequestPayload{}, fmt.Errorf("queue: unmarshal service review request: %w", err)
	}
	return payload, nil
}

// ServiceReviewRequestTaskID is the Asynq task id of a service's review
// request: a redelivered service.completed event finds the pending task and
// enqueues no second one (ErrTaskIDConflict).
func ServiceReviewRequestTaskID(serviceID int64) string {
	return fmt.Sprintf("service-review-request-%d", serviceID)
}

// ServiceReviewRequestOpts are the enqueue options: notifications queue,
// delayed by delay, deduplicated by task id.
func ServiceReviewRequestOpts(serviceID int64, delay time.Duration) []asynq.Option {
	if delay < 0 {
		delay = 0
	}
	return []asynq.Option{
		asynq.Queue(QueueNotifications),
		asynq.ProcessIn(delay),
		asynq.TaskID(ServiceReviewRequestTaskID(serviceID)),
		asynq.MaxRetry(5),
		asynq.Timeout(time.Minute),
	}
}

// WithServiceReviewRequest sets the service:review_request processor; a
// repeated call replaces it.
func (w *Worker) WithServiceReviewRequest(fn ServiceReviewRequestFunc) *Worker {
	w.serviceReviewRequest = fn
	return w
}

func (w *Worker) handleServiceReviewRequest(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseServiceReviewRequestPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.serviceReviewRequest == nil {
		w.log.Warn("service_review_request_handler_missing", "service_id", payload.ServiceID)
		return nil
	}
	return w.serviceReviewRequest(ctx, payload.ServiceID)
}
