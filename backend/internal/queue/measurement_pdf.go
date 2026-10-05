package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskMeasurementPDF renders the timestamped PDF of one measurement
// (TEC-298, F3-02f). GET /v1/measurements/{uuid}/pdf enqueues it on the
// docs queue the first time; the handler is idempotent through
// measurement_results.pdf_key (a measurement is rendered once).
const TaskMeasurementPDF = "measurement:pdf"

// MeasurementPDFPayload names the measurement result to render.
type MeasurementPDFPayload struct {
	ResultID int64 `json:"result_id"`
}

// MeasurementPDFFunc renders the PDF of one measurement result.
type MeasurementPDFFunc func(ctx context.Context, resultID int64) error

// NewMeasurementPDFTask builds the measurement PDF task.
func NewMeasurementPDFTask(resultID int64) (*asynq.Task, error) {
	body, err := json.Marshal(MeasurementPDFPayload{ResultID: resultID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal measurement pdf: %w", err)
	}
	return asynq.NewTask(TaskMeasurementPDF, body), nil
}

// ParseMeasurementPDFPayload decodes a measurement PDF payload.
func ParseMeasurementPDFPayload(data []byte) (MeasurementPDFPayload, error) {
	var payload MeasurementPDFPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return MeasurementPDFPayload{}, fmt.Errorf("queue: unmarshal measurement pdf: %w", err)
	}
	return payload, nil
}

// MeasurementPDFTaskID is the Asynq task id of a measurement's PDF render:
// repeated requests while it is pending enqueue no second task.
func MeasurementPDFTaskID(resultID int64) string {
	return fmt.Sprintf("measurement-pdf-%d", resultID)
}

// MeasurementPDFOpts are the enqueue options: docs queue (worker-docs),
// deduplicated by task id, retried on Gotenberg/storage failures.
func MeasurementPDFOpts(resultID int64) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueDocs),
		asynq.TaskID(MeasurementPDFTaskID(resultID)),
		asynq.MaxRetry(5),
		asynq.Timeout(2 * time.Minute),
	}
}

// WithMeasurementPDF sets the measurement:pdf processor; a repeated call
// replaces it.
func (w *Worker) WithMeasurementPDF(fn MeasurementPDFFunc) *Worker {
	w.measurementPDF = fn
	return w
}

func (w *Worker) handleMeasurementPDF(ctx context.Context, task *asynq.Task) error {
	payload, err := ParseMeasurementPDFPayload(task.Payload())
	if err != nil {
		return err
	}
	if w.measurementPDF == nil {
		w.log.Warn("measurement_pdf_handler_missing", "result_id", payload.ResultID)
		return nil
	}
	return w.measurementPDF(ctx, payload.ResultID)
}
