package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

// TaskVehicleTransferExpire marks pending vehicle transfers past expires_at
// as expired and writes vehicle.transfer_expired events (TEC-190). The
// transfer endpoints also expire lazily, so this only keeps the status and
// the events current; a second run writes nothing.
const TaskVehicleTransferExpire = "vehicle_transfer:expire"

// Transfers live 15 minutes; every 5 minutes keeps the delay small.
const vehicleTransferExpireCron = "*/5 * * * *"

// VehicleTransferTaskFunc runs the transfer expiry.
type VehicleTransferTaskFunc func(ctx context.Context) error

// NewVehicleTransferExpireTask builds the expiry task.
func NewVehicleTransferExpireTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskVehicleTransferExpire, []byte("{}")), nil
}

func vehicleTransferTaskOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(2 * time.Minute)}
}

// WithVehicleTransferExpire sets the transfer expiry processor; a repeated
// call replaces it.
func (w *Worker) WithVehicleTransferExpire(fn VehicleTransferTaskFunc) *Worker {
	w.vehicleTransferExpire = fn
	return w
}

func (w *Worker) handleVehicleTransferExpire(ctx context.Context, _ *asynq.Task) error {
	if w.vehicleTransferExpire == nil {
		w.log.Warn("vehicle_transfer_expire_handler_missing")
		return nil
	}
	return w.vehicleTransferExpire(ctx)
}
