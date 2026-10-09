package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/hibiken/asynq"
)

// TEC-506 (F5-09b): the hourly pricing tick on worker-core (maintenance
// queue: due recommended versions become current, the daily price
// discipline snapshot and the Monday digest, all in each brand center's
// timezone and idempotent) and the price list PDF publication on the docs
// queue (worker-docs: Gotenberg, document center).
const (
	TaskPricingDaily     = "pricing:daily"
	TaskPricingPriceList = "pricing:price_list"
)

const pricingDailyCron = "11 * * * *"

// PricingPriceListRetries is the retry count of a price list publication.
const PricingPriceListRetries = 3

// PricingDailyFunc runs one pricing tick.
type PricingDailyFunc func(ctx context.Context) error

// PricingPriceListFunc publishes one price list.
type PricingPriceListFunc func(ctx context.Context, task pricingusecase.PriceListTask) error

// NewPricingDailyTask builds the hourly tick.
func NewPricingDailyTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskPricingDaily, []byte("{}")), nil
}

func pricingDailyOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(15 * time.Minute), asynq.Unique(50 * time.Minute)}
}

// NewPricingPriceListTask builds a price list publication task.
func NewPricingPriceListTask(t pricingusecase.PriceListTask) (*asynq.Task, error) {
	body, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("queue: marshal %s: %w", TaskPricingPriceList, err)
	}
	return asynq.NewTask(TaskPricingPriceList, body), nil
}

// PricingPriceListOpts: docs queue, one task per list and publication.
func PricingPriceListOpts(t pricingusecase.PriceListTask) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueDocs),
		asynq.TaskID(fmt.Sprintf("price-list-%d-%d-%s-%s", t.BrandID, t.CountryID, t.Currency, t.Token)),
		asynq.MaxRetry(PricingPriceListRetries),
		asynq.Timeout(10 * time.Minute),
	}
}

// PriceListEnqueuer implements the pricing use case's PriceListQueue; a
// task already queued for the same publication is not an error.
type PriceListEnqueuer struct {
	Client interface {
		Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	}
}

// EnqueuePriceList enqueues pricing:price_list.
func (e PriceListEnqueuer) EnqueuePriceList(_ context.Context, t pricingusecase.PriceListTask) error {
	if e.Client == nil {
		return errors.New("queue: client is nil")
	}
	task, err := NewPricingPriceListTask(t)
	if err != nil {
		return err
	}
	if _, err := e.Client.Enqueue(task, PricingPriceListOpts(t)...); err != nil &&
		!errors.Is(err, asynq.ErrDuplicateTask) && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

// WithPricing sets the pricing tick and the price list processors.
func (w *Worker) WithPricing(daily PricingDailyFunc, priceList PricingPriceListFunc) *Worker {
	w.pricingDaily = daily
	w.pricingPriceList = priceList
	return w
}

func (w *Worker) handlePricingDaily(ctx context.Context, _ *asynq.Task) error {
	if w.pricingDaily == nil {
		w.log.Warn("pricing_daily_handler_missing")
		return nil
	}
	return w.pricingDaily(ctx)
}

func (w *Worker) handlePricingPriceList(ctx context.Context, task *asynq.Task) error {
	var t pricingusecase.PriceListTask
	if err := json.Unmarshal(task.Payload(), &t); err != nil {
		return fmt.Errorf("queue: unmarshal %s: %w: %w", TaskPricingPriceList, err, asynq.SkipRetry)
	}
	if w.pricingPriceList == nil {
		w.log.Warn("pricing_price_list_handler_missing", "brand_id", t.BrandID)
		return nil
	}
	return w.pricingPriceList(ctx, t)
}
