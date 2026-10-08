package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-476 (F5-02e): periodic fleet reports. An hourly cron (worker-core)
// creates, for every fleet with a report frequency, the report of the last
// closed month / quarter once the fleet's local time has passed 07:00 on
// the first day of the new period (fleet_reports UNIQUE (fleet, period):
// two ticks create one report) and enqueues its generation on the docs
// queue. worker-docs renders the fleet_report document template (Gotenberg),
// stores the PDF under fleet-reports/{fleet}/{period}.pdf, marks it ready
// and e-mails it to the fleet users and the billing address in the report
// language (attached up to 10 MB, else a portal link). Only the dealers
// with the fleet module count; a fleet without one gets no report.

// MaxReportAttachment is the largest PDF sent as an e-mail attachment.
const MaxReportAttachment = 10 << 20

// reportDueHour is the local hour of the period's first day from which the
// previous period is reported.
const reportDueHour = 7

// upcomingExpiryDays is the window of the "upcoming warranty expirations"
// table after the period end.
const upcomingExpiryDays = 90

// ReportRenderer renders a document of a registered kind as the system
// (documents.Service.RenderSource).
type ReportRenderer interface {
	RenderSource(ctx context.Context, kind, sourceID, locale string) ([]byte, error)
}

// ReportStorage stores and reads the report PDFs (storage driver).
type ReportStorage interface {
	Upload(ctx context.Context, f storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// ReportQueue enqueues the generation of a report (docs queue; a task
// already queued for the report is not an error).
type ReportQueue interface {
	EnqueueFleetReport(ctx context.Context, reportID int64) error
}

// MailBrand is the e-mail frame of a brand (name, logo, color).
type MailBrand struct{ Name, LogoURL, Color string }

// ReportConfig wires the report pipeline. Every part is optional: the
// scheduler needs Queue, the generation Renderer + Storage, the e-mail
// Mail (nil: no e-mail).
type ReportConfig struct {
	Renderer ReportRenderer
	Storage  ReportStorage
	Queue    ReportQueue
	Mail     mail.Sender
	Brand    func(ctx context.Context, brandID int64) MailBrand
	// PortalURL is the portal reports page linked when the PDF is too
	// large to attach.
	PortalURL string
	Log       *slog.Logger
}

// SetReports wires the report pipeline.
func (s *Service) SetReports(cfg ReportConfig) {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	s.reports = cfg
}

// ErrNoReportDealer: no dealer of the fleet has the fleet module; the
// report is not produced.
var ErrNoReportDealer = errors.New("fleet: no dealer with the fleet module")

// ErrReportQueueUnavailable: no queue is wired for the generation (503).
var ErrReportQueueUnavailable = errors.New("fleet: report queue unavailable")

// --- Periods -----------------------------------------------------------------------

// ReportPeriod is a closed report period: inclusive dates (UTC midnight).
type ReportPeriod struct {
	Kind       string
	Start, End time.Time
}

// Key is the period in file names: 2026-09 or 2026-Q3.
func (p ReportPeriod) Key() string {
	if p.Kind == model.ReportQuarterly {
		return fmt.Sprintf("%d-Q%d", p.Start.Year(), (int(p.Start.Month())-1)/3+1)
	}
	return p.Start.Format("2006-01")
}

// periodContaining is the month / quarter of the date d.
func periodContaining(kind string, d time.Time) ReportPeriod {
	m := d.Month()
	if kind == model.ReportQuarterly {
		m = time.Month((int(m)-1)/3*3 + 1)
	}
	start := time.Date(d.Year(), m, 1, 0, 0, 0, 0, time.UTC)
	months := 1
	if kind == model.ReportQuarterly {
		months = 3
	}
	return ReportPeriod{Kind: kind, Start: start, End: start.AddDate(0, months, -1)}
}

// ParseReportPeriod reads a period of POST /v1/fleets/{uuid}/reports:
// monthly YYYY-MM, quarterly YYYY-Qn.
func ParseReportPeriod(kind, raw string) (ReportPeriod, error) {
	raw = strings.TrimSpace(raw)
	switch kind {
	case model.ReportMonthly:
		t, err := time.Parse("2006-01", raw)
		if err != nil {
			return ReportPeriod{}, &ValidationError{Field: "period", Message: "monthly period must be YYYY-MM"}
		}
		return periodContaining(kind, t), nil
	case model.ReportQuarterly:
		y, q, ok := strings.Cut(strings.ToUpper(raw), "-Q")
		year, err1 := strconv.Atoi(y)
		n, err2 := strconv.Atoi(q)
		if !ok || err1 != nil || err2 != nil || len(y) != 4 || n < 1 || n > 4 {
			return ReportPeriod{}, &ValidationError{Field: "period", Message: "quarterly period must be YYYY-Qn"}
		}
		return periodContaining(kind, time.Date(year, time.Month((n-1)*3+1), 1, 0, 0, 0, 0, time.UTC)), nil
	}
	return ReportPeriod{}, &ValidationError{Field: "period_kind", Message: "monthly or quarterly"}
}

func location(tz string) *time.Location {
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	loc, err := time.LoadLocation(i18n.DefaultTimezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// localDate is the calendar date of t in loc (UTC midnight).
func localDate(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

// dueReportPeriod is the period to report at now: the previous month /
// quarter once the current one is past its first day 07:00 local.
func dueReportPeriod(kind string, now time.Time, loc *time.Location) (ReportPeriod, bool) {
	cur := periodContaining(kind, localDate(now, loc))
	due := time.Date(cur.Start.Year(), cur.Start.Month(), cur.Start.Day(), reportDueHour, 0, 0, 0, loc)
	if now.Before(due) {
		return ReportPeriod{}, false
	}
	return periodContaining(kind, cur.Start.AddDate(0, 0, -1)), true
}

// reportObjectKey is the storage key of a report PDF.
func reportObjectKey(fleet uuid.UUID, p ReportPeriod) string {
	return "fleet-reports/" + fleet.String() + "/" + p.Key() + ".pdf"
}

func reportPeriodOf(r db.FleetReport) ReportPeriod {
	return ReportPeriod{Kind: r.PeriodKind, Start: r.PeriodStart.Time, End: r.PeriodEnd.Time}
}

func pgDate(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

// --- Scheduler (worker-core) ----------------------------------------------------

// reportBatch is the scheduler's page size.
const reportBatch = 200

// ScheduleReportsTask is the hourly cron: it creates the due report of every
// fleet with a report frequency (idempotent) and enqueues its generation,
// then enqueues pending reports whose task was lost. A fleet's error does
// not stop the others; the last error is returned (Asynq retries).
func (s *Service) ScheduleReportsTask(ctx context.Context) error {
	if s.reports.Queue == nil {
		return errors.New("fleet: report queue is not configured")
	}
	q := db.New(s.conn)
	now := s.now()
	var lastErr error
	var after int64
	for {
		rows, err := q.ListFleetsDueForReport(ctx, db.ListFleetsDueForReportParams{AfterID: after, LimitCount: reportBatch})
		if err != nil {
			return fmt.Errorf("fleet: due fleets: %w", err)
		}
		for _, r := range rows {
			after = r.ID
			if err := s.scheduleFleetReport(ctx, q, r, now); err != nil {
				s.reports.Log.Error("fleet_report_schedule_failed", "fleet", r.Uuid, "error", err)
				lastErr = err
			}
		}
		if len(rows) < reportBatch {
			break
		}
	}
	stale, err := q.ListStalePendingFleetReports(ctx, reportBatch)
	if err != nil {
		return fmt.Errorf("fleet: stale reports: %w", err)
	}
	for _, id := range stale {
		if err := s.reports.Queue.EnqueueFleetReport(ctx, id); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (s *Service) scheduleFleetReport(ctx context.Context, q *db.Queries, f db.ListFleetsDueForReportRow, now time.Time) error {
	loc := location(f.Timezone)
	p, ok := dueReportPeriod(f.ReportFrequency, now, loc)
	if !ok {
		return nil
	}
	// A fleet opened after the period has nothing to report for it.
	if !localDate(f.CreatedAt.Time, loc).Before(p.End.AddDate(0, 0, 1)) {
		return nil
	}
	dealers, _, _, err := s.visibleDealers(ctx, q, f.ID)
	if err != nil {
		return err
	}
	if len(dealers) == 0 {
		return nil // no dealer with the module: no report
	}
	r, err := q.CreateFleetReportIfAbsent(ctx, db.CreateFleetReportIfAbsentParams{
		FleetOrgID: f.ID, BrandID: f.BrandID, PeriodKind: p.Kind,
		PeriodStart: pgDate(p.Start), PeriodEnd: pgDate(p.End), Locale: f.ReportLocale,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already created by an earlier tick
	}
	if err != nil {
		return fmt.Errorf("fleet: create report: %w", err)
	}
	return s.reports.Queue.EnqueueFleetReport(ctx, r.ID)
}

// --- Generation (worker-docs) ---------------------------------------------------

// GenerateReport is the docs task of one report: render, store, mark ready,
// then e-mail. It is idempotent: a ready report is not rendered again and
// an e-mailed one is not sent again. A render or storage error is returned
// for a retry; on the final attempt the report is marked failed. An e-mail
// error is returned for a retry too (the report stays ready).
func (s *Service) GenerateReport(ctx context.Context, reportID int64, final bool) error {
	q := db.New(s.conn)
	r, err := q.GetFleetReportByID(ctx, reportID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("fleet: report: %w", err)
	}
	var pdf []byte
	switch r.Status {
	case model.ReportFailed:
		return nil
	case model.ReportPending:
		var key string
		pdf, key, err = s.renderReport(ctx, q, r)
		if errors.Is(err, ErrNoReportDealer) {
			return s.failReport(ctx, q, r, err)
		}
		if err != nil {
			if final {
				if ferr := s.failReport(ctx, q, r, err); ferr != nil {
					return ferr
				}
			}
			return err
		}
		if r, err = q.MarkFleetReportReady(ctx, db.MarkFleetReportReadyParams{
			ID: r.ID, StorageKey: pgtype.Text{String: key, Valid: true},
		}); err != nil {
			return fmt.Errorf("fleet: report ready: %w", err)
		}
	case model.ReportReady:
		if r.EmailedAt.Valid {
			return nil
		}
	}
	return s.mailReport(ctx, q, r, pdf)
}

func (s *Service) failReport(ctx context.Context, q *db.Queries, r db.FleetReport, cause error) error {
	msg := cause.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	if _, err := q.MarkFleetReportFailed(ctx, db.MarkFleetReportFailedParams{
		ID: r.ID, Error: pgtype.Text{String: msg, Valid: true},
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("fleet: report failed: %w", err)
	}
	s.reports.Log.Warn("fleet_report_failed", "report", r.Uuid, "error", msg)
	return nil
}

// renderReport renders and stores the PDF of a pending report; it returns
// the PDF and its storage key.
func (s *Service) renderReport(ctx context.Context, q *db.Queries, r db.FleetReport) ([]byte, string, error) {
	if s.reports.Renderer == nil || s.reports.Storage == nil {
		return nil, "", errors.New("fleet: report renderer is not configured")
	}
	f, err := q.GetOrganizationByID(ctx, r.FleetOrgID)
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report fleet: %w", err)
	}
	dealers, _, _, err := s.visibleDealers(ctx, q, r.FleetOrgID)
	if err != nil {
		return nil, "", err
	}
	if len(dealers) == 0 {
		return nil, "", ErrNoReportDealer
	}
	pdf, err := s.reports.Renderer.RenderSource(ctx, docmodel.KindFleetReport, r.Uuid.String(), r.Locale)
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report render: %w", err)
	}
	key := reportObjectKey(f.Uuid, reportPeriodOf(r))
	if err := s.reports.Storage.Upload(ctx, storage.File{
		Body: bytes.NewReader(pdf), Size: int64(len(pdf)), ContentType: "application/pdf",
		Filename: reportFileName(r),
	}, key); err != nil {
		return nil, "", fmt.Errorf("fleet: report upload: %w", err)
	}
	return pdf, key, nil
}

func reportFileName(r db.FleetReport) string {
	return "fleet-report-" + reportPeriodOf(r).Key() + ".pdf"
}

// mailReport sends a ready report to the fleet users and the billing
// address in the report language, then marks it e-mailed.
func (s *Service) mailReport(ctx context.Context, q *db.Queries, r db.FleetReport, pdf []byte) error {
	if s.reports.Mail == nil {
		return nil
	}
	f, err := q.GetOrganizationByID(ctx, r.FleetOrgID)
	if err != nil {
		return fmt.Errorf("fleet: report fleet: %w", err)
	}
	profile, err := q.GetFleetProfileByOrg(ctx, r.FleetOrgID)
	if err != nil {
		return fmt.Errorf("fleet: report profile: %w", err)
	}
	to, err := reportRecipients(ctx, q, r.FleetOrgID, profile.BillingEmail)
	if err != nil {
		return err
	}
	if len(to) == 0 {
		s.reports.Log.Info("fleet_report_no_recipient", "report", r.Uuid)
		return nil
	}
	if pdf == nil && r.StorageKey.Valid && s.reports.Storage != nil {
		if pdf, err = s.readStored(ctx, r.StorageKey.String); err != nil {
			return err
		}
	}
	msg, err := s.reportMessage(ctx, f, r, pdf)
	if err != nil {
		return err
	}
	msg.To = to
	if err := s.reports.Mail.Send(ctx, msg); err != nil {
		return fmt.Errorf("fleet: report mail: %w", err)
	}
	if _, err := q.MarkFleetReportEmailed(ctx, r.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("fleet: report emailed: %w", err)
	}
	return nil
}

func (s *Service) readStored(ctx context.Context, key string) ([]byte, error) {
	rc, size, err := s.reports.Storage.Download(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("fleet: report download: %w", err)
	}
	defer func() { _ = rc.Close() }()
	if size > MaxReportAttachment {
		return nil, nil // too large to attach: the e-mail links the portal
	}
	b, err := io.ReadAll(io.LimitReader(rc, MaxReportAttachment+1))
	if err != nil {
		return nil, fmt.Errorf("fleet: report download: %w", err)
	}
	return b, nil
}

// reportRecipients are the active fleet users' and the billing e-mail
// addresses, without duplicates (case-insensitive).
func reportRecipients(ctx context.Context, q *db.Queries, fleetOrgID int64, billing pgtype.Text) ([]string, error) {
	emails, err := q.ListActiveFleetUserEmails(ctx, fleetOrgID)
	if err != nil {
		return nil, fmt.Errorf("fleet: report recipients: %w", err)
	}
	if billing.Valid {
		emails = append(emails, billing.String)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(emails))
	for _, e := range emails {
		e = strings.TrimSpace(e)
		k := strings.ToLower(e)
		if e == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out, nil
}

// reportMessage builds the e-mail in the report language: the PDF attached
// up to MaxReportAttachment, else a link to the portal reports page.
func (s *Service) reportMessage(ctx context.Context, f db.Organization, r db.FleetReport, pdf []byte) (mail.Message, error) {
	t, lang := reportLabelsFor(r.Locale)
	vars := map[string]string{"fleet": f.Name, "period": periodLabel(t, reportPeriodOf(r)), "url": s.reports.PortalURL}
	subject := fill(t.MailSubject, vars)
	body := fill(t.MailBody, vars)
	attach := len(pdf) > 0 && len(pdf) <= MaxReportAttachment
	if !attach {
		body += "\n\n" + fill(t.MailLink, vars)
	}
	brand := MailBrand{Name: "Olex Films"}
	if s.reports.Brand != nil {
		if b := s.reports.Brand(ctx, r.BrandID); b.Name != "" {
			brand = b
		}
	}
	htmlBody, err := mail.Layout{
		Lang: string(lang), Dir: i18n.Dir(lang), Title: subject,
		BrandName: brand.Name, LogoURL: brand.LogoURL, Color: brand.Color,
		BodyHTML: msgtemplate.MarkdownToHTML(body),
	}.RenderHTML()
	if err != nil {
		return mail.Message{}, fmt.Errorf("fleet: report mail layout: %w", err)
	}
	msg := mail.Message{Subject: subject, Body: msgtemplate.MarkdownToText(body), HTMLBody: htmlBody}
	if attach {
		msg.Attachments = []mail.Attachment{{Filename: reportFileName(r), ContentType: "application/pdf", Data: pdf}}
	}
	return msg, nil
}

// --- Manual request (panel) --------------------------------------------------------

// RequestReportInput is POST /v1/fleets/{uuid}/reports.
type RequestReportInput struct {
	PeriodKind string `json:"period_kind"`
	Period     string `json:"period"`
}

// ReportView is a fleet report.
type ReportView struct {
	UUID        uuid.UUID  `json:"uuid"`
	PeriodKind  string     `json:"period_kind"`
	PeriodStart string     `json:"period_start"`
	PeriodEnd   string     `json:"period_end"`
	Locale      string     `json:"locale"`
	Status      string     `json:"status"`
	EmailedAt   *time.Time `json:"emailed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func reportView(r db.FleetReport) ReportView {
	return ReportView{
		UUID: r.Uuid, PeriodKind: r.PeriodKind,
		PeriodStart: r.PeriodStart.Time.Format(time.DateOnly), PeriodEnd: r.PeriodEnd.Time.Format(time.DateOnly),
		Locale: r.Locale, Status: r.Status, EmailedAt: timePtr(r.EmailedAt), CreatedAt: r.CreatedAt.Time,
	}
}

// RequestReport is POST /v1/fleets/{uuid}/reports: a linked dealer (or the
// brand) asks for the report of a closed period. A failed report runs
// again, a pending one is enqueued again (task id dedupe), a ready one is
// returned as is. The period must have ended in the fleet's timezone.
func (s *Service) RequestReport(ctx context.Context, c Caller, fleetUUID uuid.UUID, in RequestReportInput) (ReportView, error) {
	p, err := ParseReportPeriod(strings.TrimSpace(in.PeriodKind), in.Period)
	if err != nil {
		return ReportView{}, err
	}
	q := db.New(s.conn)
	a, err := s.resolve(ctx, q, c, fleetUUID)
	if err != nil {
		return ReportView{}, err
	}
	fleet := a.fleet.Organization
	if !p.End.Before(localDate(s.now(), location(fleet.Timezone))) {
		return ReportView{}, &ValidationError{Field: "period", Message: "the period has not ended yet"}
	}
	dealers, _, _, err := s.visibleDealers(ctx, q, fleet.ID)
	if err != nil {
		return ReportView{}, err
	}
	if len(dealers) == 0 {
		return ReportView{}, ErrPortalClosed
	}
	if s.reports.Queue == nil {
		return ReportView{}, ErrReportQueueUnavailable
	}
	r, err := q.UpsertFleetReport(ctx, db.UpsertFleetReportParams{
		FleetOrgID: fleet.ID, BrandID: fleet.BrandID, PeriodKind: p.Kind,
		PeriodStart: pgDate(p.Start), PeriodEnd: pgDate(p.End), Locale: a.fleet.FleetProfile.ReportLocale,
	})
	if err != nil {
		return ReportView{}, fmt.Errorf("fleet: request report: %w", err)
	}
	if r.Status == model.ReportPending {
		if err := s.reports.Queue.EnqueueFleetReport(ctx, r.ID); err != nil {
			return ReportView{}, fmt.Errorf("fleet: enqueue report: %w", err)
		}
	}
	return reportView(r), nil
}

// --- Document source (documents.RegisterLoader) ------------------------------------

// ReportLoader is the fleet_report document source: the source id is the
// report uuid. Only the system (worker) or the fleet organization loads it.
type ReportLoader struct{ s *Service }

// ReportDocumentLoader returns the fleet_report source loader.
func (s *Service) ReportDocumentLoader() ReportLoader { return ReportLoader{s: s} }

// SourceType implements docmodel.SourceLoader.
func (ReportLoader) SourceType() string { return "fleet_report" }

// Load implements docmodel.SourceLoader.
func (l ReportLoader) Load(ctx context.Context, viewer docmodel.Viewer, sourceID, locale string) (docmodel.Source, error) {
	id, err := uuid.Parse(sourceID)
	if err != nil {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	q := db.New(l.s.conn)
	r, err := q.GetFleetReportByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	if err != nil {
		return docmodel.Source{}, fmt.Errorf("fleet: report source: %w", err)
	}
	if !viewer.System && viewer.OrganizationID != r.FleetOrgID {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	vars, title, err := l.s.reportVars(ctx, q, r)
	if err != nil {
		return docmodel.Source{}, err
	}
	return docmodel.Source{
		OrganizationID: r.FleetOrgID, BrandID: r.BrandID,
		Version: strconv.FormatInt(r.UpdatedAt.Time.UnixNano(), 10), Vars: vars, Title: title,
	}, nil
}

// periodLabel is "September 2026" or "Q3 2026" in the labels' language.
func periodLabel(t reportLabels, p ReportPeriod) string {
	year := strconv.Itoa(p.Start.Year())
	if p.Kind == model.ReportQuarterly {
		return fill(t.Quarter, map[string]string{"q": strconv.Itoa((int(p.Start.Month())-1)/3 + 1), "year": year})
	}
	return t.Months[p.Start.Month()-1] + " " + year
}

// reportVars builds the template variables of a report: counts, tables and
// the accounts of the dealers that have the fleet module.
func (s *Service) reportVars(ctx context.Context, q *db.Queries, r db.FleetReport) (map[string]string, string, error) {
	fleet, err := q.GetOrganizationByID(ctx, r.FleetOrgID)
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report fleet: %w", err)
	}
	profile, err := q.GetFleetProfileByOrg(ctx, r.FleetOrgID)
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report profile: %w", err)
	}
	dealers, _, _, err := s.visibleDealers(ctx, q, fleet.ID)
	if err != nil {
		return nil, "", err
	}
	if len(dealers) == 0 {
		return nil, "", ErrNoReportDealer
	}
	ids := make([]int64, 0, len(dealers))
	for _, d := range dealers {
		ids = append(ids, d.id)
	}
	t, lang := reportLabelsFor(r.Locale)
	p := reportPeriodOf(r)
	loc := location(fleet.Timezone)
	from := time.Date(p.Start.Year(), p.Start.Month(), p.Start.Day(), 0, 0, 0, 0, loc)
	to := time.Date(p.End.Year(), p.End.Month(), p.End.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)

	vehicles, err := q.CountFleetReportVehicles(ctx, fleet.ID)
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report vehicles: %w", err)
	}
	byDealer, err := q.FleetReportServicesByDealer(ctx, db.FleetReportServicesByDealerParams{
		FleetOrgID: fleet.ID, DealerIds: ids, PeriodFrom: ts(from), PeriodTo: ts(to),
	})
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report services: %w", err)
	}
	byVehicle, err := q.FleetReportServicesByVehicle(ctx, db.FleetReportServicesByVehicleParams{
		FleetOrgID: fleet.ID, DealerIds: ids, PeriodFrom: ts(from), PeriodTo: ts(to),
	})
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report services: %w", err)
	}
	parts, err := q.FleetReportParts(ctx, db.FleetReportPartsParams{
		FleetOrgID: fleet.ID, DealerIds: ids, PeriodFrom: ts(from), PeriodTo: ts(to),
	})
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report parts: %w", err)
	}
	products, err := q.FleetReportProducts(ctx, db.FleetReportProductsParams{
		FleetOrgID: fleet.ID, DealerIds: ids, PeriodFrom: ts(from), PeriodTo: ts(to),
	})
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report products: %w", err)
	}
	wc, err := q.FleetReportWarrantyCounts(ctx, db.FleetReportWarrantyCountsParams{
		FleetOrgID: fleet.ID, DealerIds: ids, PeriodFrom: ts(from), PeriodTo: ts(to),
	})
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report warranties: %w", err)
	}
	upcoming, err := q.FleetReportUpcomingExpirations(ctx, db.FleetReportUpcomingExpirationsParams{
		FleetOrgID: fleet.ID, DealerIds: ids, PeriodTo: ts(to), Until: ts(to.AddDate(0, 0, upcomingExpiryDays)), LimitCount: 50,
	})
	if err != nil {
		return nil, "", fmt.Errorf("fleet: report expirations: %w", err)
	}

	var services int64
	dealerRows := make([][]string, 0, len(byDealer))
	for _, d := range byDealer {
		services += d.ServiceCount
		dealerRows = append(dealerRows, []string{d.OrganizationName, strconv.FormatInt(d.ServiceCount, 10)})
	}
	vehicleRows := make([][]string, 0, len(byVehicle))
	for _, v := range byVehicle {
		vehicleRows = append(vehicleRows, []string{v.Plate, strings.TrimSpace(v.CarBrandName + " " + v.CarModelName), strconv.FormatInt(v.ServiceCount, 10)})
	}
	partRows := make([][]string, 0, len(parts))
	for _, pr := range parts {
		partRows = append(partRows, []string{partLabel(lang, pr.PartKey), strconv.FormatInt(pr.PartCount, 10)})
	}
	productRows := make([][]string, 0, len(products))
	for _, pr := range products {
		productRows = append(productRows, []string{
			pr.ProductName, strconv.FormatInt(pr.ItemCount, 10), strconv.FormatInt(pr.Quantity, 10), rat(pr.Meters).FloatString(2),
		})
	}
	upcomingRows := make([][]string, 0, len(upcoming))
	for _, u := range upcoming {
		upcomingRows = append(upcomingRows, []string{u.Plate, u.ProductName, u.EndAt.Time.In(loc).Format(t.DateLayout)})
	}
	accountRows := make([][]string, 0, len(dealers))
	for _, d := range dealers {
		acc, err := portalDealerAccount(ctx, q, d, StatementPeriod{From: p.Start, To: p.End})
		if err != nil {
			return nil, "", err
		}
		accountRows = append(accountRows, []string{
			acc.Dealer.Name, money2(acc.ServiceIncomeTotal, acc.Currency), money2(acc.CollectionTotal, acc.Currency),
			money2(acc.ClosingBalance, acc.Currency),
		})
	}

	label := periodLabel(t, p)
	vars := map[string]string{
		"fleet_name": fleet.Name, "fleet_legal_name": profile.LegalName, "fleet_tax_number": profile.TaxNumber,
		"document_number": "FR-" + p.Key(), "document_date": s.now().In(loc).Format(t.DateLayout),
		"period_label": label, "period_start": p.Start.Format(t.DateLayout), "period_end": p.End.Format(t.DateLayout),
		"vehicle_count": strconv.FormatInt(vehicles, 10), "service_count": strconv.FormatInt(services, 10),
		"dealer_count":          strconv.Itoa(len(dealers)),
		"warranty_active_count": strconv.FormatInt(wc.ActiveCount, 10), "warranty_expired_count": strconv.FormatInt(wc.ExpiredCount, 10),
		"warranty_started_count": strconv.FormatInt(wc.StartedCount, 10),
		"dealer_services_table":  reportTable(t, []pdfrender.Column{{Label: t.Dealer}, {Label: t.Services, Numeric: true}}, dealerRows),
		"vehicle_services_table": reportTable(t, []pdfrender.Column{{Label: t.Plate}, {Label: t.Vehicle}, {Label: t.Services, Numeric: true}}, vehicleRows),
		"parts_table":            reportTable(t, []pdfrender.Column{{Label: t.Part}, {Label: t.Count, Numeric: true}}, partRows),
		"products_table": reportTable(t, []pdfrender.Column{
			{Label: t.Product}, {Label: t.Items, Numeric: true}, {Label: t.Pieces, Numeric: true}, {Label: t.Meters, Numeric: true},
		}, productRows),
		"upcoming_expirations_table": reportTable(t, []pdfrender.Column{{Label: t.Plate}, {Label: t.Product}, {Label: t.Ends}}, upcomingRows),
		"accounts_table": reportTable(t, []pdfrender.Column{
			{Label: t.Dealer}, {Label: t.ServiceAmount, Numeric: true}, {Label: t.Payments, Numeric: true}, {Label: t.Balance, Numeric: true},
		}, accountRows),
	}
	return vars, fleet.Name + " · " + label, nil
}

// reportTable is a table, or the "no records" line when empty.
func reportTable(t reportLabels, cols []pdfrender.Column, rows [][]string) string {
	if len(rows) == 0 {
		return `<p class="doc-empty">` + html.EscapeString(t.Empty) + `</p>`
	}
	return pdfrender.Table(cols, rows)
}

// partLabel translates an applied part key (services.parts.<key>); an
// unknown key is printed as is.
func partLabel(lang i18n.Locale, key string) string {
	k := "services.parts." + key
	if l := i18n.Translate(lang, k); l != k {
		return l
	}
	return key
}

// money2 is "1234.50 TRY".
func money2(amount, currency string) string {
	if _, ok := new(big.Rat).SetString(amount); !ok {
		amount = "0.00"
	}
	return amount + " " + currency
}
