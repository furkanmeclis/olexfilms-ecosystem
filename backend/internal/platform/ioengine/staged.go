package ioengine

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrImportRejected is wrapped by staged importers when the job cannot be
// staged, applied or undone in its current state (the request is invalid,
// not the server).
var ErrImportRejected = errors.New("ioengine: import rejected")

// ImportJob identifies the import job a staged importer works on.
type ImportJob struct {
	ID             int64
	UUID           uuid.UUID
	ActorID        int64
	OrganizationID int64
	Locale         string
}

// StagedImporter is implemented by adapters that keep their own staging
// rows (TEC-158 stock import) instead of applying row by row:
//
//   - Stage (preview / dry run) classifies the mapped rows, replaces the
//     staging rows of the job and writes nothing else.
//   - Apply (confirm, import queue) writes the staged rows in one
//     transaction. It is idempotent: a job that was applied before returns
//     its report and writes nothing new.
//   - Undo reverses the applied rows that nothing touched since, in one
//     transaction, and reports the refused rows.
//
// The import service calls these instead of ApplyRow / RevertRow.
type StagedImporter interface {
	Stage(ctx context.Context, job ImportJob, rows []map[string]any, defaults map[string]any) (PreviewSummary, error)
	Apply(ctx context.Context, job ImportJob) (PreviewSummary, error)
	Undo(ctx context.Context, job ImportJob) (PreviewSummary, error)
}
