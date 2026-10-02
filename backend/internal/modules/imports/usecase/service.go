package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrForbidden      = errors.New("forbidden")
	ErrInvalidRequest = errors.New("invalid request")
)

type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Service orchestrates import jobs.
type Service struct {
	q        *db.Queries
	storage  storage.Driver
	registry *ioengine.Registry
	queue    Enqueuer
	notifier Notifier
	activity *activity.Recorder
	log      *slog.Logger
	syncMode bool
}

func New(
	q *db.Queries,
	store storage.Driver,
	reg *ioengine.Registry,
	enq Enqueuer,
	notifier Notifier,
	rec *activity.Recorder,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		q: q, storage: store, registry: reg, queue: enq, notifier: notifier,
		activity: rec, log: log, syncMode: enq == nil,
	}
}

type ImportJobView struct {
	UUID           uuid.UUID               `json:"uuid"`
	Resource       string                  `json:"resource"`
	Format         string                  `json:"format"`
	Status         string                  `json:"status"`
	Mapping        map[string]string       `json:"mapping,omitempty"`
	Defaults       map[string]string       `json:"defaults,omitempty"`
	PreviewSummary ioengine.PreviewSummary `json:"preview_summary,omitempty"`
	Error          *string                 `json:"error,omitempty"`
	RollbackUntil  *time.Time              `json:"rollback_until,omitempty"`
	AppliedAt      *time.Time              `json:"applied_at,omitempty"`
	// SourceFilename is the uploaded file name (TEC-211).
	SourceFilename string `json:"source_filename"`
	// Actor is the user who uploaded the job; set on the organization
	// list (TEC-211).
	Actor     *ActorView `json:"actor,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ActorView names the user who uploaded a job.
type ActorView struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// sourceFilenameMax is import_jobs.source_filename VARCHAR(255).
const sourceFilenameMax = 255

func cleanSourceFilename(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "." || name == "/" {
		name = ""
	}
	if len(name) > sourceFilenameMax {
		name = name[:sourceFilenameMax]
	}
	return name
}

// Upload creates a job from file bytes.
func (s *Service) Upload(ctx context.Context, actorID int64, organizationID *int64, resource string, format ioengine.ImportFormat, locale string, filename string, r io.Reader) (ImportJobView, error) {
	if _, err := s.registry.Get(resource); err != nil {
		return ImportJobView{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return ImportJobView{}, err
	}
	row, err := s.q.CreateImportJob(ctx, db.CreateImportJobParams{
		Resource: resource, ActorID: actorID, Format: string(format), Locale: locale,
		FileKey: pgtype.Text{}, OrganizationID: int8Arg(organizationID),
		SourceFilename: cleanSourceFilename(filename),
	})
	if err != nil {
		return ImportJobView{}, err
	}
	ext := string(format)
	if format == ioengine.ImportJSON {
		ext = "json"
	}
	key := storage.ImportSourceObjectKey(row.Uuid.String(), ext)
	if err := s.storage.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), Filename: filename,
	}, key); err != nil {
		return ImportJobView{}, err
	}
	updated, err := s.q.UpdateImportJobFileKey(ctx, db.UpdateImportJobFileKeyParams{
		Uuid: row.Uuid, FileKey: pgtype.Text{String: key, Valid: true},
	})
	if err != nil {
		return ImportJobView{}, err
	}
	if adapter, err := s.registry.Get(resource); err == nil {
		if headers, _, parseErr := ioengine.ParseUpload(format, bytes.NewReader(data)); parseErr == nil && len(headers) > 0 {
			suggested := ioengine.SuggestMapping(headers, adapter.ImportSchema(), i18n.Normalize(locale))
			if len(suggested) > 0 {
				mb, _ := json.Marshal(suggested)
				if mapped, mapErr := s.q.UpdateImportJobMapping(ctx, db.UpdateImportJobMappingParams{
					Uuid: updated.Uuid, MappingJson: mb, DefaultsJson: []byte("{}"),
				}); mapErr == nil {
					updated = mapped
				}
			}
		}
	}
	if s.activity != nil {
		uid := actorID
		s.activity.Record(ctx, &uid, "import.uploaded", resource, &updated.Uuid, map[string]any{
			"format": format,
		}, nil)
	}
	return mapImportJob(updated), nil
}

// SampleFile builds localized sample document.
func (s *Service) Sample(ctx context.Context, resource string, format ioengine.ImportFormat, locale string) ([]byte, string, error) {
	adapter, err := s.registry.Get(resource)
	if err != nil {
		return nil, "", err
	}
	fields := adapter.ImportSchema()
	if len(fields) == 0 {
		return nil, "", ErrInvalidRequest
	}
	rows := adapters.SampleRows(resource)
	if sp, ok := adapter.(interface{ SampleRows() []map[string]any }); ok {
		rows = sp.SampleRows()
	}
	return ioengine.EncodeSample(format, locale, fields, rows)
}

// UpdateMapping saves column mapping and defaults.
func (s *Service) UpdateMapping(ctx context.Context, jobUUID uuid.UUID, actorID int64, orgID *int64, mapping, defaults map[string]string) (ImportJobView, error) {
	job, err := s.getAccessible(ctx, jobUUID, actorID, orgID)
	if err != nil {
		return ImportJobView{}, err
	}
	mb, _ := json.Marshal(mapping)
	dbb, _ := json.Marshal(defaults)
	row, err := s.q.UpdateImportJobMapping(ctx, db.UpdateImportJobMappingParams{
		Uuid: jobUUID, MappingJson: mb, DefaultsJson: dbb,
	})
	if err != nil {
		return ImportJobView{}, err
	}
	_ = job
	return mapImportJob(row), nil
}

// Preview dry-runs mapped rows.
func (s *Service) Preview(ctx context.Context, jobUUID uuid.UUID, actorID int64, orgID *int64) (ImportJobView, error) {
	job, err := s.getAccessible(ctx, jobUUID, actorID, orgID)
	if err != nil {
		return ImportJobView{}, err
	}
	summary, err := s.buildPreview(ctx, job)
	if err != nil {
		return ImportJobView{}, mapStagedErr(err)
	}
	pb, _ := json.Marshal(summary)
	row, err := s.q.UpdateImportJobPreview(ctx, db.UpdateImportJobPreviewParams{Uuid: jobUUID, PreviewJson: pb})
	if err != nil {
		return ImportJobView{}, err
	}
	return mapImportJob(row), nil
}

// Confirm queues apply task.
func (s *Service) Confirm(ctx context.Context, jobUUID uuid.UUID, actorID int64, orgID *int64) (ImportJobView, error) {
	job, err := s.getAccessible(ctx, jobUUID, actorID, orgID)
	if err != nil {
		return ImportJobView{}, err
	}
	// TEC-158: confirming a staged batch again (queued, applying or
	// applied) writes nothing new; the job is returned as it is.
	if _, staged := s.staged(job.Resource); staged && confirmedStatus(job.Status) {
		return mapImportJob(job), nil
	}
	row, err := s.q.QueueImportJob(ctx, jobUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportJobView{}, fmt.Errorf("%w: import job is not previewed", ErrInvalidRequest)
	}
	if err != nil {
		return ImportJobView{}, err
	}
	if s.syncMode {
		_ = s.ProcessImport(ctx, row.ID)
		updated, _ := s.q.GetImportJobByID(ctx, row.ID)
		return mapImportJob(updated), nil
	}
	task, err := queue.NewImportProcessTask(row.ID)
	if err != nil {
		return ImportJobView{}, err
	}
	if _, err := s.queue.Enqueue(task, asynq.Queue(queue.QueueImports)); err != nil {
		return ImportJobView{}, err
	}
	return mapImportJob(row), nil
}

// ProcessImport applies rows in worker.
func (s *Service) ProcessImport(ctx context.Context, jobID int64) error {
	job, err := s.q.MarkImportJobApplying(ctx, jobID)
	if err != nil {
		return err
	}
	ctx, err = s.withJobOrganization(ctx, job)
	if err != nil {
		return s.failImport(ctx, jobID, err.Error())
	}
	adapter, err := s.registry.Get(job.Resource)
	if err != nil {
		return s.failImport(ctx, jobID, err.Error())
	}
	if st, ok := adapter.(ioengine.StagedImporter); ok {
		return s.processStaged(ctx, job, st)
	}
	rows, mapping, defaults, err := s.loadMappedRows(ctx, job)
	if err != nil {
		return s.failImport(ctx, jobID, err.Error())
	}
	created, updated, failed := 0, 0, 0
	var applyErrs []ioengine.RowError
	for i, row := range rows {
		res, err := adapter.ApplyRow(ctx, row, defaults)
		if err != nil {
			failed++
			applyErrs = append(applyErrs, ioengine.RowError{Index: i + 1, Error: err.Error()})
			continue
		}
		if !res.OK {
			failed++
			applyErrs = append(applyErrs, ioengine.RowError{Index: i + 1, Error: res.Error})
			continue
		}
		if res.Op == "create" {
			created++
		} else {
			updated++
		}
		prev, _ := json.Marshal(res.Previous)
		_, _ = s.q.InsertImportChange(ctx, db.InsertImportChangeParams{
			JobID: jobID, EntityType: res.EntityType,
			EntityUuid: uuid.MustParse(res.EntityUUID), Op: res.Op,
			PreviousJson: prev,
		})
	}
	summary := ioengine.PreviewSummary{Total: len(rows), Valid: created + updated, Invalid: failed, Errors: applyErrs}
	pb, _ := json.Marshal(summary)
	applied, err := s.q.MarkImportJobApplied(ctx, db.MarkImportJobAppliedParams{ID: jobID, PreviewJson: pb})
	if err != nil {
		return err
	}
	if s.notifier != nil {
		uid := job.ActorID
		loc := i18n.Normalize(job.Locale)
		actionURL := fmt.Sprintf("/v1/platform/imports/%s", job.Uuid.String())
		in := notifmodel.EnqueueInput{
			UserID: &uid, Channels: []string{notifmodel.ChannelInapp},
			TemplateCode: "imports.applied", Language: job.Locale,
			ActionURL: &actionURL,
			TemplateVars: map[string]string{
				"resource": i18n.ResourceLabel(loc, job.Resource),
				"created":  fmt.Sprintf("%d", created),
				"updated":  fmt.Sprintf("%d", updated),
				"failed":   fmt.Sprintf("%d", failed),
			},
			SourceEvent: "imports.applied",
		}
		if job.OrganizationID.Valid {
			actionURL = fmt.Sprintf("/v1/tenant/imports/%s", job.Uuid.String())
			in.ActionURL = &actionURL
			if slug := organizationSlug(ctx, s.q, job.OrganizationID); slug != "" {
				in.Payload = map[string]any{"organization_slug": slug}
			}
		}
		_, _ = s.notifier.Enqueue(ctx, in)
	}
	if s.activity != nil {
		uid := job.ActorID
		s.activity.Record(ctx, &uid, "import.applied", job.Resource, &job.Uuid, map[string]any{
			"created": created, "updated": updated, "failed": failed,
		}, nil)
	}
	_ = applied
	_ = mapping
	return nil
}

// Rollback reverts an applied import within window.
func (s *Service) Rollback(ctx context.Context, jobUUID uuid.UUID, actorID int64, orgID *int64) (ImportJobView, error) {
	job, err := s.getAccessible(ctx, jobUUID, actorID, orgID)
	if err != nil {
		return ImportJobView{}, err
	}
	ctx, err = s.withJobOrganization(ctx, job)
	if err != nil {
		return ImportJobView{}, err
	}
	if st, ok := s.staged(job.Resource); ok {
		return s.undoStaged(ctx, job, actorID, st)
	}
	if job.Status != "applied" {
		return ImportJobView{}, ErrInvalidRequest
	}
	if job.RollbackUntil.Valid && time.Now().After(job.RollbackUntil.Time) {
		return ImportJobView{}, ErrInvalidRequest
	}
	adapter, err := s.registry.Get(job.Resource)
	if err != nil {
		return ImportJobView{}, err
	}
	changes, err := s.q.ListImportChangesForJob(ctx, job.ID)
	if err != nil {
		return ImportJobView{}, err
	}
	for i := len(changes) - 1; i >= 0; i-- {
		ch := changes[i]
		var prev map[string]any
		_ = json.Unmarshal(ch.PreviousJson, &prev)
		_ = adapter.RevertRow(ctx, ch.EntityType, ch.EntityUuid.String(), prev)
	}
	row, err := s.q.MarkImportJobRolledBack(ctx, job.ID)
	if err != nil {
		return ImportJobView{}, err
	}
	if s.activity != nil {
		uid := actorID
		s.activity.Record(ctx, &uid, "import.rolled_back", job.Resource, &job.Uuid, map[string]any{}, nil)
	}
	return mapImportJob(row), nil
}

func (s *Service) ListJobs(ctx context.Context, actorID int64, admin bool, orgID *int64, limit, offset int32) ([]ImportJobView, int64, error) {
	var rows []db.ImportJob
	var total int64
	var err error
	if orgID != nil {
		orgRows, err := s.q.ListImportJobsForOrganization(ctx, db.ListImportJobsForOrganizationParams{
			OrganizationID: *orgID, LimitCount: limit, OffsetCount: offset,
		})
		if err != nil {
			return nil, 0, err
		}
		total, _ = s.q.CountImportJobsForOrganization(ctx, *orgID)
		out := make([]ImportJobView, 0, len(orgRows))
		for _, r := range orgRows {
			v := mapImportJob(r.ImportJob)
			v.Actor = &ActorView{UUID: r.ActorUuid, Name: strings.TrimSpace(r.ActorName + " " + r.ActorSurname)}
			out = append(out, v)
		}
		return out, total, nil
	} else if admin {
		rows, err = s.q.ListAllImportJobs(ctx, db.ListAllImportJobsParams{LimitCount: limit, OffsetCount: offset})
		total, _ = s.q.CountAllImportJobs(ctx)
	} else {
		rows, err = s.q.ListImportJobsForActor(ctx, db.ListImportJobsForActorParams{
			ActorID: actorID, LimitCount: limit, OffsetCount: offset,
		})
		total, _ = s.q.CountImportJobsForActor(ctx, actorID)
	}
	if err != nil {
		return nil, 0, err
	}
	out := make([]ImportJobView, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapImportJob(r))
	}
	return out, total, nil
}

func (s *Service) GetJob(ctx context.Context, jobUUID uuid.UUID, actorID int64, admin bool, orgID *int64) (ImportJobView, error) {
	row, err := s.q.GetImportJobByUUID(ctx, jobUUID)
	if err != nil {
		return ImportJobView{}, ErrNotFound
	}
	if orgID != nil {
		if !row.OrganizationID.Valid || row.OrganizationID.Int64 != *orgID {
			return ImportJobView{}, ErrForbidden
		}
		return mapImportJob(row), nil
	}
	if row.OrganizationID.Valid {
		return ImportJobView{}, ErrForbidden
	}
	if !admin && row.ActorID != actorID {
		return ImportJobView{}, ErrForbidden
	}
	return mapImportJob(row), nil
}

func (s *Service) buildPreview(ctx context.Context, job db.ImportJob) (ioengine.PreviewSummary, error) {
	rows, _, defaults, err := s.loadMappedRows(ctx, job)
	if err != nil {
		return ioengine.PreviewSummary{}, err
	}
	if st, ok := s.staged(job.Resource); ok {
		ctx, err = s.withJobOrganization(ctx, job)
		if err != nil {
			return ioengine.PreviewSummary{}, err
		}
		return st.Stage(ctx, jobRef(job), rows, defaults)
	}
	summary := ioengine.PreviewSummary{Total: len(rows)}
	previews := make([]ioengine.RowPreview, 0, min(len(rows), 50))
	for i, row := range rows {
		ok := true
		for k, v := range row {
			if v == nil || fmt.Sprint(v) == "" {
				_ = k
			}
		}
		if ok {
			summary.Valid++
			if len(previews) < 50 {
				previews = append(previews, ioengine.RowPreview{Index: i + 1, Data: row})
			}
		} else {
			summary.Invalid++
		}
	}
	summary.Rows = previews
	return summary, nil
}

func (s *Service) loadMappedRows(ctx context.Context, job db.ImportJob) ([]map[string]any, map[string]string, map[string]any, error) {
	if !job.FileKey.Valid || job.FileKey.String == "" {
		ext := string(job.Format)
		key := storage.ImportSourceObjectKey(job.Uuid.String(), ext)
		job.FileKey = pgtype.Text{String: key, Valid: true}
	}
	rc, _, err := s.storage.Download(ctx, job.FileKey.String)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, nil, err
	}
	headers, rawRows, err := ioengine.ParseUpload(ioengine.ImportFormat(job.Format), bytes.NewReader(data))
	if err != nil {
		return nil, nil, nil, err
	}
	var mapping map[string]string
	_ = json.Unmarshal(job.MappingJson, &mapping)
	if len(mapping) == 0 {
		adapter, _ := s.registry.Get(job.Resource)
		mapping = ioengine.SuggestMapping(headers, adapter.ImportSchema(), i18n.Normalize(job.Locale))
	}
	var defaults map[string]string
	_ = json.Unmarshal(job.DefaultsJson, &defaults)
	defAny := map[string]any{}
	for k, v := range defaults {
		defAny[k] = v
	}
	rows := ioengine.ApplyMapping(rawRows, mapping, defaults)
	return rows, mapping, defAny, nil
}

func (s *Service) getAccessible(ctx context.Context, jobUUID uuid.UUID, actorID int64, orgID *int64) (db.ImportJob, error) {
	row, err := s.q.GetImportJobByUUID(ctx, jobUUID)
	if err != nil {
		return db.ImportJob{}, ErrNotFound
	}
	if orgID != nil {
		if !row.OrganizationID.Valid || row.OrganizationID.Int64 != *orgID {
			return db.ImportJob{}, ErrForbidden
		}
		return row, nil
	}
	if row.OrganizationID.Valid {
		return db.ImportJob{}, ErrForbidden
	}
	if row.ActorID != actorID {
		return db.ImportJob{}, ErrForbidden
	}
	return row, nil
}

func (s *Service) withJobOrganization(ctx context.Context, job db.ImportJob) (context.Context, error) {
	if !job.OrganizationID.Valid {
		return ctx, nil
	}
	org, err := s.q.GetOrganizationByID(ctx, job.OrganizationID.Int64)
	if err != nil {
		return ctx, err
	}
	return orgctx.WithScope(ctx, orgctx.Scope{
		InternalID: org.ID, UUID: org.Uuid, Slug: org.Slug, Name: org.Name,
		Status: org.Status, OrgType: org.Type, BrandID: org.BrandID,
	}), nil
}

func int8Arg(id *int64) pgtype.Int8 {
	if id == nil || *id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *id, Valid: true}
}

func organizationSlug(ctx context.Context, q *db.Queries, orgID pgtype.Int8) string {
	if !orgID.Valid {
		return ""
	}
	org, err := q.GetOrganizationByID(ctx, orgID.Int64)
	if err != nil {
		return ""
	}
	return org.Slug
}

func (s *Service) failImport(ctx context.Context, jobID int64, msg string) error {
	_, _ = s.q.MarkImportJobFailed(ctx, db.MarkImportJobFailedParams{ID: jobID, Error: pgtype.Text{String: msg, Valid: true}})
	return errors.New(msg)
}

func mapImportJob(row db.ImportJob) ImportJobView {
	var mapping map[string]string
	_ = json.Unmarshal(row.MappingJson, &mapping)
	var defaults map[string]string
	_ = json.Unmarshal(row.DefaultsJson, &defaults)
	var preview ioengine.PreviewSummary
	_ = json.Unmarshal(row.PreviewJson, &preview)
	var errMsg *string
	if row.Error.Valid {
		errMsg = &row.Error.String
	}
	var rb *time.Time
	if row.RollbackUntil.Valid {
		t := row.RollbackUntil.Time
		rb = &t
	}
	var ap *time.Time
	if row.AppliedAt.Valid {
		t := row.AppliedAt.Time
		ap = &t
	}
	return ImportJobView{
		UUID: row.Uuid, Resource: row.Resource, Format: row.Format, Status: row.Status,
		Mapping: mapping, Defaults: defaults, PreviewSummary: preview, Error: errMsg,
		RollbackUntil: rb, AppliedAt: ap, SourceFilename: row.SourceFilename, CreatedAt: row.CreatedAt.Time,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
