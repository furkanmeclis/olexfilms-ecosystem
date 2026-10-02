package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
)

// TEC-158: staged importers (ioengine.StagedImporter) keep their own staging
// rows. Preview stages and classifies, the import queue applies the batch in
// one transaction (idempotent) and rollback runs the controlled undo.

// staged returns the staged importer of a resource.
func (s *Service) staged(resource string) (ioengine.StagedImporter, bool) {
	adapter, err := s.registry.Get(resource)
	if err != nil {
		return nil, false
	}
	st, ok := adapter.(ioengine.StagedImporter)
	return st, ok
}

// confirmedStatus reports whether the job was confirmed before.
func confirmedStatus(status string) bool {
	switch status {
	case "queued", "applying", "applied", "rolled_back":
		return true
	}
	return false
}

func jobRef(job db.ImportJob) ioengine.ImportJob {
	return ioengine.ImportJob{
		ID: job.ID, UUID: job.Uuid, ActorID: job.ActorID,
		OrganizationID: job.OrganizationID.Int64, Locale: job.Locale,
	}
}

// mapStagedErr turns a staged importer rejection into ErrInvalidRequest.
func mapStagedErr(err error) error {
	if errors.Is(err, ioengine.ErrImportRejected) {
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	return err
}

// processStaged applies a staged batch (worker, import queue).
func (s *Service) processStaged(ctx context.Context, job db.ImportJob, st ioengine.StagedImporter) error {
	summary, err := st.Apply(ctx, jobRef(job))
	if err != nil {
		return s.failImport(ctx, job.ID, err.Error())
	}
	pb, _ := json.Marshal(summary)
	if _, err := s.q.MarkImportJobApplied(ctx, db.MarkImportJobAppliedParams{ID: job.ID, PreviewJson: pb}); err != nil {
		return err
	}
	applied := summary.Counts["applied"]
	failed := summary.Total - applied
	if s.notifier != nil {
		uid := job.ActorID
		loc := i18n.Normalize(job.Locale)
		actionURL := fmt.Sprintf("/v1/tenant/imports/%s", job.Uuid.String())
		in := notifmodel.EnqueueInput{
			UserID: &uid, Channels: []string{notifmodel.ChannelInapp},
			TemplateCode: "imports.applied", Language: job.Locale,
			ActionURL: &actionURL,
			TemplateVars: map[string]string{
				"resource": i18n.ResourceLabel(loc, job.Resource),
				"created":  fmt.Sprintf("%d", applied),
				"updated":  "0",
				"failed":   fmt.Sprintf("%d", failed),
			},
			SourceEvent: "imports.applied",
		}
		if slug := organizationSlug(ctx, s.q, job.OrganizationID); slug != "" {
			in.Payload = map[string]any{"organization_slug": slug}
		}
		_, _ = s.notifier.Enqueue(ctx, in)
	}
	if s.activity != nil {
		uid := job.ActorID
		s.activity.Record(ctx, &uid, "import.applied", job.Resource, &job.Uuid, map[string]any{
			"batch_uuid": summary.BatchUUID, "counts": summary.Counts,
		}, nil)
	}
	return nil
}

// undoStaged runs the controlled undo of an applied staged batch. A second
// undo returns the job as it is.
func (s *Service) undoStaged(ctx context.Context, job db.ImportJob, actorID int64, st ioengine.StagedImporter) (ImportJobView, error) {
	if job.Status == "rolled_back" {
		return mapImportJob(job), nil
	}
	if job.Status != "applied" {
		return ImportJobView{}, fmt.Errorf("%w: import job is not applied", ErrInvalidRequest)
	}
	if job.RollbackUntil.Valid && time.Now().After(job.RollbackUntil.Time) {
		return ImportJobView{}, fmt.Errorf("%w: undo window has passed", ErrInvalidRequest)
	}
	// The undo movements are recorded on the user who runs the undo.
	ref := jobRef(job)
	ref.ActorID = actorID
	summary, err := st.Undo(ctx, ref)
	if err != nil {
		return ImportJobView{}, mapStagedErr(err)
	}
	pb, _ := json.Marshal(summary)
	if _, err := s.q.SetImportJobPreview(ctx, db.SetImportJobPreviewParams{ID: job.ID, PreviewJson: pb}); err != nil {
		return ImportJobView{}, err
	}
	row, err := s.q.MarkImportJobRolledBack(ctx, job.ID)
	if err != nil {
		return ImportJobView{}, err
	}
	if s.activity != nil {
		uid := actorID
		s.activity.Record(ctx, &uid, "import.rolled_back", job.Resource, &job.Uuid, map[string]any{
			"batch_uuid": summary.BatchUUID, "counts": summary.Counts,
		}, nil)
	}
	return mapImportJob(row), nil
}
