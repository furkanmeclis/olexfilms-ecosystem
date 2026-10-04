package queue

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Task type identifiers.
const (
	TaskPing                 = "app:ping"
	TaskNotificationDeliver  = "app:notification:deliver"
	TaskAnnouncementDispatch = "app:announcement:dispatch"
	TaskExportProcess        = "app:export:process"
	TaskImportProcess        = "app:import:process"
	TaskBulkProcess          = "app:bulk:process"
	TaskLogPurgeSweep        = "app:logs:purge_sweep"
	TaskSearchUpsert         = "app:search:upsert"
	TaskSearchDelete         = "app:search:delete"
	TaskSearchReindex        = "app:search:reindex"
	TaskDocsRender           = "app:docs:render"
	TaskRatesFetch           = "app:rates:fetch"
	QueueNotifications       = "notifications"
	QueueExports             = "exports"
	QueueImports             = "imports"
	QueueBulk                = "bulk"
	QueueSearch              = "search"
	QueueMaintenance         = "maintenance"
	QueueDocs                = "docs"
)

// PingPayload is the body for the sample ping job.
type PingPayload struct {
	Message    string    `json:"message"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

// NotificationDeliverPayload identifies a notification row to deliver.
type NotificationDeliverPayload struct {
	NotificationID int64 `json:"notification_id"`
}

// AnnouncementDispatchPayload carries one announcement fan-out batch.
type AnnouncementDispatchPayload struct {
	EventID          uuid.UUID `json:"event_id"`
	AnnouncementID   int64     `json:"announcement_id"`
	AnnouncementUUID uuid.UUID `json:"announcement_uuid"`
	BrandID          int64     `json:"brand_id"`
	OrganizationID   *int64    `json:"organization_id,omitempty"`
	Title            string    `json:"title"`
	Body             string    `json:"body"`
	UserIDs          []int64   `json:"user_ids"`
	Batch            int       `json:"batch"`
}

// NewPingTask builds a sample Asynq task used to verify the queue path.
func NewPingTask(message string) (*asynq.Task, error) {
	if message == "" {
		message = "pong"
	}
	body, err := json.Marshal(PingPayload{
		Message:    message,
		EnqueuedAt: time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal ping: %w", err)
	}
	return asynq.NewTask(TaskPing, body), nil
}

// ParsePingPayload decodes a ping task payload.
func ParsePingPayload(data []byte) (PingPayload, error) {
	var payload PingPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return PingPayload{}, fmt.Errorf("queue: unmarshal ping: %w", err)
	}
	return payload, nil
}

// NewNotificationDeliverTask builds a delivery task.
func NewNotificationDeliverTask(notificationID int64) (*asynq.Task, error) {
	body, err := json.Marshal(NotificationDeliverPayload{NotificationID: notificationID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal notification deliver: %w", err)
	}
	return asynq.NewTask(TaskNotificationDeliver, body), nil
}

// ParseNotificationDeliverPayload decodes a deliver task payload.
func ParseNotificationDeliverPayload(data []byte) (NotificationDeliverPayload, error) {
	var payload NotificationDeliverPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return NotificationDeliverPayload{}, fmt.Errorf("queue: unmarshal notification deliver: %w", err)
	}
	return payload, nil
}

// NewAnnouncementDispatchTask builds an announcement notification fan-out task.
func NewAnnouncementDispatchTask(payload AnnouncementDispatchPayload) (*asynq.Task, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("queue: marshal announcement dispatch: %w", err)
	}
	return asynq.NewTask(TaskAnnouncementDispatch, body), nil
}

// ParseAnnouncementDispatchPayload decodes an announcement dispatch task payload.
func ParseAnnouncementDispatchPayload(data []byte) (AnnouncementDispatchPayload, error) {
	var payload AnnouncementDispatchPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return AnnouncementDispatchPayload{}, fmt.Errorf("queue: unmarshal announcement dispatch: %w", err)
	}
	return payload, nil
}

// ExportProcessPayload identifies an export job row to process.
type ExportProcessPayload struct {
	ExportJobID int64 `json:"export_job_id"`
}

// NewExportProcessTask builds an export processing task.
func NewExportProcessTask(exportJobID int64) (*asynq.Task, error) {
	body, err := json.Marshal(ExportProcessPayload{ExportJobID: exportJobID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal export process: %w", err)
	}
	return asynq.NewTask(TaskExportProcess, body), nil
}

// ParseExportProcessPayload decodes export process payload.
func ParseExportProcessPayload(data []byte) (ExportProcessPayload, error) {
	var payload ExportProcessPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return ExportProcessPayload{}, fmt.Errorf("queue: unmarshal export process: %w", err)
	}
	return payload, nil
}

// ImportProcessPayload identifies an import job row to process.
type ImportProcessPayload struct {
	ImportJobID int64 `json:"import_job_id"`
}

// NewImportProcessTask builds an import processing task.
func NewImportProcessTask(importJobID int64) (*asynq.Task, error) {
	body, err := json.Marshal(ImportProcessPayload{ImportJobID: importJobID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal import process: %w", err)
	}
	return asynq.NewTask(TaskImportProcess, body), nil
}

// ParseImportProcessPayload decodes import process payload.
func ParseImportProcessPayload(data []byte) (ImportProcessPayload, error) {
	var payload ImportProcessPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return ImportProcessPayload{}, fmt.Errorf("queue: unmarshal import process: %w", err)
	}
	return payload, nil
}

// BulkProcessPayload identifies a bulk job row to process.
type BulkProcessPayload struct {
	BulkJobID int64 `json:"bulk_job_id"`
}

// NewBulkProcessTask builds a bulk processing task.
func NewBulkProcessTask(bulkJobID int64) (*asynq.Task, error) {
	body, err := json.Marshal(BulkProcessPayload{BulkJobID: bulkJobID})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal bulk process: %w", err)
	}
	return asynq.NewTask(TaskBulkProcess, body), nil
}

// ParseBulkProcessPayload decodes bulk process payload.
func ParseBulkProcessPayload(data []byte) (BulkProcessPayload, error) {
	var payload BulkProcessPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return BulkProcessPayload{}, fmt.Errorf("queue: unmarshal bulk process: %w", err)
	}
	return payload, nil
}

// NewLogPurgeSweepTask enqueues due log retention rule processing.
func NewLogPurgeSweepTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskLogPurgeSweep, []byte("{}")), nil
}

// NewRatesFetchTask fetches the daily TCMB and ECB exchange rates.
func NewRatesFetchTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskRatesFetch, []byte("{}")), nil
}

// SearchUpsertPayload identifies one entity to upsert into search indexes.
type SearchUpsertPayload struct {
	Spec string `json:"spec"`
	ID   string `json:"id"`
}

// SearchDeletePayload identifies one entity to remove from search indexes.
type SearchDeletePayload struct {
	Spec string `json:"spec"`
	ID   string `json:"id"`
}

// SearchReindexPayload triggers a full reindex for one spec (empty = all).
type SearchReindexPayload struct {
	Spec string `json:"spec,omitempty"`
}

// NewSearchUpsertTask builds a search upsert task.
func NewSearchUpsertTask(spec, id string) (*asynq.Task, error) {
	body, err := json.Marshal(SearchUpsertPayload{Spec: spec, ID: id})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal search upsert: %w", err)
	}
	return asynq.NewTask(TaskSearchUpsert, body), nil
}

// ParseSearchUpsertPayload decodes search upsert payload.
func ParseSearchUpsertPayload(data []byte) (SearchUpsertPayload, error) {
	var payload SearchUpsertPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return SearchUpsertPayload{}, fmt.Errorf("queue: unmarshal search upsert: %w", err)
	}
	return payload, nil
}

// NewSearchDeleteTask builds a search delete task.
func NewSearchDeleteTask(spec, id string) (*asynq.Task, error) {
	body, err := json.Marshal(SearchDeletePayload{Spec: spec, ID: id})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal search delete: %w", err)
	}
	return asynq.NewTask(TaskSearchDelete, body), nil
}

// ParseSearchDeletePayload decodes search delete payload.
func ParseSearchDeletePayload(data []byte) (SearchDeletePayload, error) {
	var payload SearchDeletePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return SearchDeletePayload{}, fmt.Errorf("queue: unmarshal search delete: %w", err)
	}
	return payload, nil
}

// NewSearchReindexTask builds a search reindex task.
func NewSearchReindexTask(spec string) (*asynq.Task, error) {
	body, err := json.Marshal(SearchReindexPayload{Spec: spec})
	if err != nil {
		return nil, fmt.Errorf("queue: marshal search reindex: %w", err)
	}
	return asynq.NewTask(TaskSearchReindex, body), nil
}

// ParseSearchReindexPayload decodes search reindex payload.
func ParseSearchReindexPayload(data []byte) (SearchReindexPayload, error) {
	var payload SearchReindexPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return SearchReindexPayload{}, fmt.Errorf("queue: unmarshal search reindex: %w", err)
	}
	return payload, nil
}

// DocsRenderPayload identifies a document_renders row to render.
type DocsRenderPayload struct {
	RenderID int64 `json:"render_id"`
}

// NewDocsRenderTask builds a PDF render task. The task id is derived from
// the render cache key and attempt, so enqueuing the same document twice
// yields a single task (asynq.ErrTaskIDConflict on the second call).
func NewDocsRenderTask(renderID int64, cacheKey string, attempt int32) (*asynq.Task, []asynq.Option, error) {
	body, err := json.Marshal(DocsRenderPayload{RenderID: renderID})
	if err != nil {
		return nil, nil, fmt.Errorf("queue: marshal docs render: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDocs),
		asynq.TaskID(fmt.Sprintf("docs:%s:%d", cacheKey, attempt)),
		asynq.MaxRetry(5),
		asynq.Timeout(2 * time.Minute),
	}
	return asynq.NewTask(TaskDocsRender, body), opts, nil
}

// ParseDocsRenderPayload decodes a docs render payload.
func ParseDocsRenderPayload(data []byte) (DocsRenderPayload, error) {
	var payload DocsRenderPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return DocsRenderPayload{}, fmt.Errorf("queue: unmarshal docs render: %w", err)
	}
	return payload, nil
}
