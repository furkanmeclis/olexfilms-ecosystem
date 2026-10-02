package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrForbidden      = errors.New("forbidden")
	ErrInvalidRequest = errors.New("invalid request")
)

// Notifier enqueues in-app notifications.
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// Enqueuer schedules background tasks.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Service orchestrates export jobs.
type Service struct {
	q        *db.Queries
	storage  storage.Driver
	registry *ioengine.Registry
	queue    Enqueuer
	notifier Notifier
	activity *activity.Recorder
	log      *slog.Logger
	syncMode bool
	pdf      HTMLToPDF
}

// HTMLToPDF converts HTML to PDF (satisfied by *pdfrender.Client).
type HTMLToPDF interface {
	HTMLToPDF(ctx context.Context, html string) ([]byte, error)
}

// SetDocumentPDF enables styled PDF documents for adapters implementing
// ioengine.DocumentRenderer (nil keeps the generic table PDF).
func (s *Service) SetDocumentPDF(r HTMLToPDF) { s.pdf = r }

// New creates an export service.
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

// ExportJobView is API projection.
type ExportJobView struct {
	UUID      uuid.UUID `json:"uuid"`
	Resource  string    `json:"resource"`
	Format    string    `json:"format"`
	Status    string    `json:"status"`
	RowCount  int32     `json:"row_count"`
	Error     *string   `json:"error,omitempty"`
	Download  *string   `json:"download_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// RequestExport queues an export job.
func (s *Service) RequestExport(
	ctx context.Context,
	actorID int64,
	organizationID *int64,
	resource string,
	format ioengine.ExportFormat,
	query ioengine.ExportQuery,
	locale string,
) (ExportJobView, error) {
	adapter, err := s.registry.Get(resource)
	if err != nil {
		return ExportJobView{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	_ = adapter
	if format != ioengine.ExportPDF && format != ioengine.ExportXLSX && format != ioengine.ExportCSV && format != ioengine.ExportJSON {
		return ExportJobView{}, fmt.Errorf("%w: invalid format", ErrInvalidRequest)
	}
	if query == nil {
		query = ioengine.ExportQuery{}
	}
	delete(query, ioengine.QueryOrganizationID)
	qb, _ := json.Marshal(query)
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	params := db.CreateExportJobParams{
		Resource: resource, ActorID: actorID, Format: string(format),
		QueryJson: qb, Locale: locale, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	}
	if organizationID != nil && *organizationID > 0 {
		params.OrganizationID = pgtype.Int8{Int64: *organizationID, Valid: true}
	}
	row, err := s.q.CreateExportJob(ctx, params)
	if err != nil {
		return ExportJobView{}, err
	}
	if s.activity != nil {
		uid := actorID
		s.activity.Record(ctx, &uid, "export.requested", resource, &row.Uuid, map[string]any{
			"format": format, "query": query,
		}, nil)
	}
	if s.syncMode {
		if err := s.ProcessExport(ctx, row.ID); err != nil {
			return ExportJobView{}, err
		}
		updated, _ := s.q.GetExportJobByID(ctx, row.ID)
		return mapExportJob(updated), nil
	}
	task, err := queue.NewExportProcessTask(row.ID)
	if err != nil {
		return ExportJobView{}, err
	}
	if _, err := s.queue.Enqueue(task, asynq.Queue(queue.QueueExports)); err != nil {
		return ExportJobView{}, err
	}
	return mapExportJob(row), nil
}

// ProcessExport runs export worker logic.
func (s *Service) ProcessExport(ctx context.Context, jobID int64) error {
	job, err := s.q.MarkExportJobProcessing(ctx, jobID)
	if err != nil {
		return err
	}
	adapter, err := s.registry.Get(job.Resource)
	if err != nil {
		return s.fail(ctx, jobID, err.Error())
	}
	var query ioengine.ExportQuery
	_ = json.Unmarshal(job.QueryJson, &query)
	if query == nil {
		query = ioengine.ExportQuery{}
	}
	delete(query, ioengine.QueryOrganizationID)
	if job.OrganizationID.Valid {
		query[ioengine.QueryOrganizationID] = strconv.FormatInt(job.OrganizationID.Int64, 10)
	}
	ds, err := adapter.Export(ctx, query, i18n.Normalize(job.Locale))
	if err != nil {
		return s.fail(ctx, jobID, err.Error())
	}
	lh, err := s.letterheadForJob(ctx, job)
	if err != nil {
		return s.fail(ctx, jobID, err.Error())
	}
	title := ioengine.ExportTitle(job.Locale, job.Resource)
	data, err := s.renderDocument(ctx, adapter, job.Format, ds, job.Locale, &lh, title)
	if data == nil {
		data, err = ioengine.EncodeExport(ioengine.ExportFormat(job.Format), ds, job.Locale, &lh, title)
	}
	if err != nil {
		return s.fail(ctx, jobID, err.Error())
	}
	ext := ioengine.FileExtForExport(ioengine.ExportFormat(job.Format))
	key := storage.ExportObjectKey(job.Uuid.String(), ext)
	filename := ioengine.ExportDownloadFilename(job.Resource, ioengine.ExportFormat(job.Format), job.CreatedAt.Time)
	if err := s.storage.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)),
		ContentType: ioengine.ContentTypeForExport(ioengine.ExportFormat(job.Format)),
		Filename:    filename,
	}, key); err != nil {
		return s.fail(ctx, jobID, err.Error())
	}
	completed, err := s.q.MarkExportJobCompleted(ctx, db.MarkExportJobCompletedParams{
		ID: jobID, FileKey: pgtype.Text{String: key, Valid: true}, RowCount: int32(len(ds.Rows)),
	})
	if err != nil {
		return err
	}
	if s.notifier != nil {
		dl := exportDownloadPath(job)
		uid := job.ActorID
		loc := i18n.Normalize(job.Locale)
		in := notifmodel.EnqueueInput{
			UserID: &uid, Channels: []string{notifmodel.ChannelInapp},
			TemplateCode: "exports.ready", Language: job.Locale,
			ActionURL: &dl,
			TemplateVars: map[string]string{
				"resource": i18n.ResourceLabel(loc, job.Resource),
				"format":   i18n.ExportFormatLabel(loc, job.Format),
			},
			SourceEvent: "exports.ready",
		}
		if slug := organizationSlug(ctx, s.q, job.OrganizationID); slug != "" {
			in.Payload = map[string]any{"organization_slug": slug}
		}
		_, _ = s.notifier.Enqueue(ctx, in)
	}
	_ = completed
	return nil
}

func (s *Service) fail(ctx context.Context, jobID int64, msg string) error {
	_, _ = s.q.MarkExportJobFailed(ctx, db.MarkExportJobFailedParams{ID: jobID, Error: pgtype.Text{String: msg, Valid: true}})
	return errors.New(msg)
}

func (s *Service) letterheadForJob(ctx context.Context, job db.ExportJob) (ioengine.Letterhead, error) {
	settings, err := s.q.GetAppSettings(ctx)
	if err != nil {
		return ioengine.Letterhead{}, err
	}
	if job.OrganizationID.Valid {
		org, err := s.q.GetOrganizationByID(ctx, job.OrganizationID.Int64)
		if err != nil {
			return ioengine.Letterhead{}, err
		}
		lh, err := ioengine.LoadOrganizationLetterhead(ctx, s.storage, org, settings)
		if err != nil {
			s.log.Warn("export_org_letterhead_logo_failed", "error", err)
			return ioengine.LetterheadFromOrganization(org, settings), nil
		}
		return lh, nil
	}
	lh, err := ioengine.LoadLetterheadLogo(ctx, s.storage, settings)
	if err != nil {
		s.log.Warn("export_letterhead_logo_failed", "error", err)
		return ioengine.LetterheadFromSettings(settings), nil
	}
	return lh, nil
}

func jobBelongsToOrg(row db.ExportJob, orgID int64) bool {
	return row.OrganizationID.Valid && row.OrganizationID.Int64 == orgID
}

// GetJob returns a job if actor may access it.
func (s *Service) GetJob(ctx context.Context, jobUUID uuid.UUID, actorID int64, admin bool) (ExportJobView, error) {
	row, err := s.q.GetExportJobByUUID(ctx, jobUUID)
	if err != nil {
		return ExportJobView{}, ErrNotFound
	}
	if !admin && row.ActorID != actorID {
		return ExportJobView{}, ErrForbidden
	}
	return mapExportJob(row), nil
}

// GetPortalJob returns a portal export job owned by the actor (other jobs
// answer ErrNotFound, never ErrForbidden, so their existence does not leak).
func (s *Service) GetPortalJob(ctx context.Context, jobUUID uuid.UUID, actorID int64) (ExportJobView, error) {
	row, err := s.q.GetExportJobByUUID(ctx, jobUUID)
	if err != nil || !portalJobOf(row, actorID) {
		return ExportJobView{}, ErrNotFound
	}
	return mapExportJob(row), nil
}

// DownloadPortal opens the file of a portal export job owned by the actor.
func (s *Service) DownloadPortal(ctx context.Context, jobUUID uuid.UUID, actorID int64) (io.ReadCloser, string, string, error) {
	row, err := s.q.GetExportJobByUUID(ctx, jobUUID)
	if err != nil || !portalJobOf(row, actorID) {
		return nil, "", "", ErrNotFound
	}
	return s.openExportFile(ctx, row)
}

func portalJobOf(row db.ExportJob, actorID int64) bool {
	return strings.HasPrefix(row.Resource, PortalResourcePrefix) && row.ActorID == actorID && !row.OrganizationID.Valid
}

// GetOrgJob returns a tenant-scoped job.
func (s *Service) GetOrgJob(ctx context.Context, jobUUID uuid.UUID, orgID int64) (ExportJobView, error) {
	row, err := s.q.GetExportJobByUUID(ctx, jobUUID)
	if err != nil {
		return ExportJobView{}, ErrNotFound
	}
	if !jobBelongsToOrg(row, orgID) {
		return ExportJobView{}, ErrNotFound
	}
	return mapExportJob(row), nil
}

// ListJobs lists export jobs for actor or all if admin.
func (s *Service) ListJobs(ctx context.Context, actorID int64, admin bool, limit, offset int32) ([]ExportJobView, int64, error) {
	var rows []db.ExportJob
	var total int64
	var err error
	if admin {
		rows, err = s.q.ListAllExportJobs(ctx, db.ListAllExportJobsParams{LimitCount: limit, OffsetCount: offset})
		total, _ = s.q.CountAllExportJobs(ctx)
	} else {
		rows, err = s.q.ListExportJobsForActor(ctx, db.ListExportJobsForActorParams{
			ActorID: actorID, LimitCount: limit, OffsetCount: offset,
		})
		total, _ = s.q.CountExportJobsForActor(ctx, actorID)
	}
	if err != nil {
		return nil, 0, err
	}
	out := make([]ExportJobView, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapExportJob(r))
	}
	return out, total, nil
}

// ListOrgJobs lists export jobs for one organization.
func (s *Service) ListOrgJobs(ctx context.Context, orgID int64, limit, offset int32) ([]ExportJobView, int64, error) {
	rows, err := s.q.ListExportJobsForOrganization(ctx, db.ListExportJobsForOrganizationParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true},
		LimitCount:     limit,
		OffsetCount:    offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountExportJobsForOrganization(ctx, pgtype.Int8{Int64: orgID, Valid: true})
	if err != nil {
		return nil, 0, err
	}
	out := make([]ExportJobView, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapExportJob(r))
	}
	return out, total, nil
}

// Download opens export file stream.
func (s *Service) Download(ctx context.Context, jobUUID uuid.UUID, actorID int64, admin bool) (io.ReadCloser, string, string, error) {
	row, err := s.q.GetExportJobByUUID(ctx, jobUUID)
	if err != nil {
		return nil, "", "", ErrNotFound
	}
	if !admin && row.ActorID != actorID {
		return nil, "", "", ErrForbidden
	}
	return s.openExportFile(ctx, row)
}

// DownloadOrg opens a tenant-scoped export file.
func (s *Service) DownloadOrg(ctx context.Context, jobUUID uuid.UUID, orgID int64) (io.ReadCloser, string, string, error) {
	row, err := s.q.GetExportJobByUUID(ctx, jobUUID)
	if err != nil {
		return nil, "", "", ErrNotFound
	}
	if !jobBelongsToOrg(row, orgID) {
		return nil, "", "", ErrNotFound
	}
	return s.openExportFile(ctx, row)
}

func (s *Service) openExportFile(ctx context.Context, row db.ExportJob) (io.ReadCloser, string, string, error) {
	if !row.FileKey.Valid || row.Status != "completed" {
		return nil, "", "", ErrNotFound
	}
	rc, _, err := s.storage.Download(ctx, row.FileKey.String)
	if err != nil {
		return nil, "", "", err
	}
	format := ioengine.ExportFormat(row.Format)
	filename := ioengine.ExportDownloadFilename(row.Resource, format, row.CreatedAt.Time)
	return rc, ioengine.ContentTypeForExport(format), filename, nil
}

// PortalResourcePrefix marks export resources requested from the customer
// portal (TEC-161): their files are downloaded through /v1/portal/exports.
const PortalResourcePrefix = "portal."

func exportDownloadPath(row db.ExportJob) string {
	if strings.HasPrefix(row.Resource, PortalResourcePrefix) {
		return fmt.Sprintf("/v1/portal/exports/%s/download", row.Uuid.String())
	}
	if row.OrganizationID.Valid {
		return fmt.Sprintf("/v1/tenant/exports/%s/download", row.Uuid.String())
	}
	return fmt.Sprintf("/v1/platform/exports/%s/download", row.Uuid.String())
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

func mapExportJob(row db.ExportJob) ExportJobView {
	var errMsg *string
	if row.Error.Valid {
		errMsg = &row.Error.String
	}
	var dl *string
	if row.Status == "completed" {
		u := exportDownloadPath(row)
		dl = &u
	}
	return ExportJobView{
		UUID: row.Uuid, Resource: row.Resource, Format: row.Format, Status: row.Status,
		RowCount: row.RowCount, Error: errMsg, Download: dl, CreatedAt: row.CreatedAt.Time,
	}
}

// renderDocument returns a structured JSON document or a styled PDF for
// document adapters, or nil to use the generic encoder (other formats, no
// renderer, or Gotenberg failure).
func (s *Service) renderDocument(
	ctx context.Context,
	adapter ioengine.ResourceAdapter,
	format string,
	ds ioengine.Dataset,
	locale string,
	lh *ioengine.Letterhead,
	title string,
) ([]byte, error) {
	if jd, ok := adapter.(ioengine.JSONDocumentRenderer); ok && ioengine.ExportFormat(format) == ioengine.ExportJSON {
		return jd.DocumentJSON(ds, locale)
	}
	doc, ok := adapter.(ioengine.DocumentRenderer)
	if !ok || s.pdf == nil || ioengine.ExportFormat(format) != ioengine.ExportPDF {
		return nil, nil
	}
	html, err := doc.DocumentHTML(ds, locale, lh, title)
	if err != nil {
		s.log.Warn("export_document_html_failed", "resource", adapter.Resource(), "error", err)
		return nil, nil
	}
	data, err := s.pdf.HTMLToPDF(ctx, html)
	if err != nil {
		s.log.Warn("export_document_pdf_failed", "resource", adapter.Resource(), "error", err)
		return nil, nil
	}
	return data, nil
}
