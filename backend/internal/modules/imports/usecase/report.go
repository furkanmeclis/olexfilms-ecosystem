package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/google/uuid"
)

// ErrorReportFile is a downloadable import error report.
type ErrorReportFile struct {
	Data     []byte
	Filename string
}

// reportStatuses are the job statuses with a preview to report on.
var reportStatuses = []string{"previewed", "queued", "applying", "applied", "failed", "rolled_back"}

// ErrorReport builds the XLSX error report of a job (TEC-757): the uploaded
// rows with their source headers plus row, status and error columns in the
// job locale. Staged importers report their staged row statuses and error
// codes; row-by-row importers the preview check (or, once applied, the
// apply result). Access follows the job detail (GetJob).
func (s *Service) ErrorReport(ctx context.Context, jobUUID uuid.UUID, actorID int64, admin bool, orgID *int64) (ErrorReportFile, error) {
	job, err := s.getReadable(ctx, jobUUID, actorID, admin, orgID)
	if err != nil {
		return ErrorReportFile{}, err
	}
	if !slices.Contains(reportStatuses, job.Status) {
		return ErrorReportFile{}, fmt.Errorf("%w: import job is not previewed", ErrInvalidRequest)
	}
	headers, rawRows, err := s.loadSource(ctx, job)
	if err != nil {
		return ErrorReportFile{}, err
	}
	var mapping map[string]string
	_ = json.Unmarshal(job.MappingJson, &mapping)
	if len(mapping) > 0 {
		mapped := make([]string, 0, len(mapping))
		for _, h := range headers {
			if _, ok := mapping[h]; ok {
				mapped = append(mapped, h)
			}
		}
		headers = mapped
	}
	statuses, err := s.reportStatuses(ctx, job)
	if err != nil {
		return ErrorReportFile{}, err
	}
	report := ioengine.ErrorReport{Headers: headers, Lines: make([]ioengine.ReportLine, 0, len(rawRows))}
	for i, raw := range rawRows {
		line := ioengine.ReportLine{Index: i + 1, Cells: raw}
		if st, ok := statuses[i+1]; ok {
			line.Status, line.Errors, line.Failed = st.StatusLabel, st.Errors, st.Failed
		}
		report.Lines = append(report.Lines, line)
	}
	data, err := ioengine.EncodeErrorReport(report, job.Locale)
	if err != nil {
		return ErrorReportFile{}, err
	}
	return ErrorReportFile{Data: data, Filename: ioengine.ErrorReportFilename(job.SourceFilename)}, nil
}

// reportStatuses returns the report status of every row (by row index).
func (s *Service) reportStatuses(ctx context.Context, job db.ImportJob) (map[int]ioengine.RowReport, error) {
	out := map[int]ioengine.RowReport{}
	if adapter, err := s.registry.Get(job.Resource); err == nil {
		if rr, ok := adapter.(ioengine.RowReporter); ok {
			ctx, err := s.withJobOrganization(ctx, job)
			if err != nil {
				return nil, err
			}
			rows, err := rr.ReportRows(ctx, jobRef(job))
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				out[r.Index] = r
			}
			return out, nil
		}
	}
	// From the stored summary: errors cover every row, statuses only the
	// rows the summary kept.
	var summary ioengine.PreviewSummary
	_ = json.Unmarshal(job.PreviewJson, &summary)
	loc := i18n.Normalize(job.Locale)
	_, staged := s.staged(job.Resource)
	applied := !staged && (job.Status == "applied" || job.Status == "rolled_back")
	for _, e := range summary.Errors {
		r := out[e.Index]
		r.Index, r.Failed = e.Index, true
		r.Errors = append(r.Errors, e.Error)
		out[e.Index] = r
	}
	for _, p := range summary.Rows {
		r := out[p.Index]
		r.Index, r.Status, r.StatusLabel = p.Index, p.Status, p.StatusLabel
		out[p.Index] = r
	}
	if staged {
		return out, nil
	}
	for i := 1; i <= summary.Total; i++ {
		r := out[i]
		r.Index = i
		switch {
		case applied && r.Failed:
			r.Status = "failed"
		case applied:
			r.Status = "applied"
		case r.Failed:
			r.Status = previewInvalid
		default:
			r.Status = previewValid
		}
		r.StatusLabel = i18n.Translate(loc, "imports.report.status."+r.Status)
		out[i] = r
	}
	return out, nil
}

// loadSource parses the uploaded file of a job (source headers and rows).
func (s *Service) loadSource(ctx context.Context, job db.ImportJob) ([]string, []map[string]string, error) {
	rc, _, err := s.storage.Download(ctx, s.sourceKey(job))
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, err
	}
	return ioengine.ParseUpload(ioengine.ImportFormat(job.Format), bytes.NewReader(data))
}
