package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
)

// TEC-407 (F4-04d): campaign sending. campaigns:tick (every minute, the
// worker-core scheduler leader) starts due campaigns and finishes done ones;
// campaigns:send_recipient sends one campaign_recipients row on the
// campaigns queue of the low worker group.
const (
	TaskCampaignTick          = "campaigns:tick"
	TaskCampaignSendRecipient = "campaigns:send_recipient"

	QueueCampaigns = "campaigns"

	// CampaignSendMaxRetry: one try plus three retries, then failed.
	CampaignSendMaxRetry = 3

	campaignTickCron = "@every 1m"
)

// CampaignRecipientPayload names the recipient row of a send task.
type CampaignRecipientPayload struct {
	RecipientID int64 `json:"recipient_id"`
}

// CampaignTickFunc runs one scheduler tick.
type CampaignTickFunc func(ctx context.Context) error

// CampaignRecipientFunc sends one recipient; final is true on the last
// retry.
type CampaignRecipientFunc func(ctx context.Context, recipientID int64, final bool) error

// NewCampaignTickTask builds the periodic tick task.
func NewCampaignTickTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskCampaignTick, []byte("{}")), nil
}

// campaignTickOpts: no retry (the next minute runs again), short timeout
// and unique per minute so a slow tick is not stacked.
func campaignTickOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(0), asynq.Timeout(5 * time.Minute), asynq.Unique(55 * time.Second)}
}

// CampaignRecipientOpts: campaigns queue, task id = recipient row (one
// pending task per row, campaign + user + channel UNIQUE), three retries.
func CampaignRecipientOpts(recipientID int64) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueCampaigns),
		asynq.TaskID("campaign-rcpt-" + strconv.FormatInt(recipientID, 10)),
		asynq.MaxRetry(CampaignSendMaxRetry),
		asynq.Timeout(2 * time.Minute),
	}
}

// CampaignEnqueuer implements the campaign sender's RecipientQueue. A task
// id conflict (the task is already queued or deferred) is not an error.
type CampaignEnqueuer struct {
	Client interface {
		Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	}
}

// EnqueueRecipient enqueues campaigns:send_recipient for a row.
func (e CampaignEnqueuer) EnqueueRecipient(_ context.Context, recipientID int64) error {
	if e.Client == nil {
		return fmt.Errorf("queue: client is nil")
	}
	body, err := json.Marshal(CampaignRecipientPayload{RecipientID: recipientID})
	if err != nil {
		return err
	}
	_, err = e.Client.Enqueue(asynq.NewTask(TaskCampaignSendRecipient, body), CampaignRecipientOpts(recipientID)...)
	if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

// WithCampaigns sets the tick and recipient processors; a repeated call
// replaces them.
func (w *Worker) WithCampaigns(tick CampaignTickFunc, send CampaignRecipientFunc) *Worker {
	w.campaignTick, w.campaignSend = tick, send
	return w
}

func (w *Worker) handleCampaignTick(ctx context.Context, _ *asynq.Task) error {
	if w.campaignTick == nil {
		w.log.Warn("campaign_tick_handler_missing")
		return nil
	}
	return w.campaignTick(ctx)
}

func (w *Worker) handleCampaignSendRecipient(ctx context.Context, task *asynq.Task) error {
	var p CampaignRecipientPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("queue: unmarshal %s: %w: %w", task.Type(), err, asynq.SkipRetry)
	}
	if w.campaignSend == nil {
		w.log.Warn("campaign_send_handler_missing", "recipient_id", p.RecipientID)
		return nil
	}
	return w.campaignSend(ctx, p.RecipientID, finalAttempt(ctx))
}
