package queue

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
)

const TaskCertificateExpiryScan = "certificates:expiry_scan"

const certificateExpiryScanCron = "23 * * * *"

type CertificateExpiryScanFunc func(ctx context.Context) error

func NewCertificateExpiryScanTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskCertificateExpiryScan, []byte("{}")), nil
}

func certificateExpiryScanOpts() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(3), asynq.Timeout(10 * time.Minute)}
}

func (w *Worker) WithCertificateExpiryScan(fn CertificateExpiryScanFunc) *Worker {
	w.certificateExpiryScan = fn
	return w
}

func (w *Worker) handleCertificateExpiryScan(ctx context.Context, _ *asynq.Task) error {
	if w.certificateExpiryScan == nil {
		w.log.Warn("certificate_expiry_scan_handler_missing")
		return nil
	}
	return w.certificateExpiryScan(ctx)
}
