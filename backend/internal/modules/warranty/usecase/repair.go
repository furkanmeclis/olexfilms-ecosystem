package usecase

// TEC-194 (F1-06h): the warranty repair scan (warranty:repair_scan). The
// event bus swallows handler errors and does not retry, so a transient
// failure of the service.completed listener can leave a completed service
// without its warranties. The scan lists completed services of the last N
// days that still have an item without a warranty, applies the shared
// write-free rules (SkipReason) and, for every service with at least one
// eligible missing item, runs the listener's own CreateForService: the
// same transaction, the same idempotent insert and the same
// warranty.created events. A second run finds nothing to create.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// Repair scan defaults.
const (
	// DefaultRepairScanDays is the look-back window (by completed_at).
	DefaultRepairScanDays = 30
	// RepairScanBatch is how many services one candidate page holds.
	RepairScanBatch = 100
	// RepairScanGrace leaves just completed services to the listener: their
	// service.completed outbox row may not be drained yet.
	RepairScanGrace = 15 * time.Minute
)

// RepairResult reports one scan.
type RepairResult struct {
	// ServicesScanned is the number of completed services with at least
	// one item without a warranty.
	ServicesScanned int `json:"services_scanned"`
	// ServicesExcluded had only excluded items (Glorian, external, no
	// warranty period): nothing to repair.
	ServicesExcluded int `json:"services_excluded"`
	// ServicesRetried went through CreateForService.
	ServicesRetried int `json:"services_retried"`
	// ServicesRepaired got at least one new warranty.
	ServicesRepaired int `json:"services_repaired"`
	// WarrantiesCreated is the number of new warranties (= warranty.created
	// events).
	WarrantiesCreated int `json:"warranties_created"`
	// ServicesFailed returned an error; the scan goes on and reports it.
	ServicesFailed int `json:"services_failed"`
}

// RepairScanner runs the warranty repair scan.
type RepairScanner struct {
	l     *Listener
	days  int
	batch int
	grace time.Duration
	log   *slog.Logger
	now   func() time.Time
}

// NewRepairScanner builds the scan over the listener. days <= 0 uses
// DefaultRepairScanDays; log may be nil.
func NewRepairScanner(l *Listener, days int, log *slog.Logger) *RepairScanner {
	if days <= 0 {
		days = DefaultRepairScanDays
	}
	if log == nil {
		log = slog.Default()
	}
	return &RepairScanner{l: l, days: days, batch: RepairScanBatch, grace: RepairScanGrace, log: log, now: time.Now}
}

// Days is the look-back window in days.
func (s *RepairScanner) Days() int { return s.days }

// Task is the warranty:repair_scan handler (every organization).
func (s *RepairScanner) Task(ctx context.Context) error {
	_, err := s.Scan(ctx, s.now(), 0)
	return err
}

// Scan repairs the services completed in [now - days, now - grace];
// organizationID 0 scans every organization. A failing service does not
// stop the scan; the first error is returned after the whole pass so the
// task is retried (every step is idempotent).
func (s *RepairScanner) Scan(ctx context.Context, now time.Time, organizationID int64) (RepairResult, error) {
	var res RepairResult
	since := now.AddDate(0, 0, -s.days)
	until := now.Add(-s.grace)
	var after int64
	var firstErr error
	for {
		rows, err := s.l.q.ListWarrantyRepairCandidates(ctx, db.ListWarrantyRepairCandidatesParams{
			Since: ts(since), Until: ts(until), AfterServiceID: after,
			OrganizationID: organizationID, ServiceLimit: int32(s.batch),
		})
		if err != nil {
			s.logResult(res, organizationID, err)
			return res, fmt.Errorf("warranty: repair candidates: %w", err)
		}
		services, eligible := groupRepairCandidates(rows)
		for _, sid := range services {
			res.ServicesScanned++
			after = sid
			if !eligible[sid] {
				res.ServicesExcluded++
				continue
			}
			res.ServicesRetried++
			cr, err := s.l.CreateForService(ctx, sid)
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					s.logResult(res, organizationID, ctxErr)
					return res, ctxErr
				}
				res.ServicesFailed++
				firstErr = errors.Join(firstErr, err)
				s.log.Warn("warranty_repair_service_failed", "service_id", sid, "error", err)
				continue
			}
			if n := len(cr.Created); n > 0 {
				res.ServicesRepaired++
				res.WarrantiesCreated += n
				s.log.Warn("warranty_repaired", "service_id", sid, "created", n)
			}
		}
		if len(services) < s.batch {
			break
		}
	}
	s.logResult(res, organizationID, firstErr)
	if firstErr != nil {
		return res, fmt.Errorf("warranty: repair scan: %d services failed: %w", res.ServicesFailed, firstErr)
	}
	return res, nil
}

// groupRepairCandidates returns the services of a page in order and whether
// each has at least one item that the shared rules leave eligible.
func groupRepairCandidates(rows []db.ListWarrantyRepairCandidatesRow) ([]int64, map[int64]bool) {
	var services []int64
	eligible := map[int64]bool{}
	for _, r := range rows {
		if _, seen := eligible[r.ServiceID]; !seen {
			services = append(services, r.ServiceID)
			eligible[r.ServiceID] = false
		}
		if SkipReason(Eligibility{
			ServiceBrandSlug:       r.ServiceBrandSlug,
			UnitBrandSlug:          r.UnitBrandSlug,
			UnitSource:             r.UnitSource,
			UnitConnectionID:       r.UnitConnectionID,
			ExternalOutbound:       r.ExternalOutbound,
			WarrantyDurationMonths: r.WarrantyDurationMonths,
		}) == "" {
			eligible[r.ServiceID] = true
		}
	}
	return services, eligible
}

// logResult writes the scan summary. There is no metrics backend yet, so
// the structured log line is the metric (warn when something was
// repaired or failed: the listener missed it).
func (s *RepairScanner) logResult(res RepairResult, organizationID int64, err error) {
	attrs := []any{
		"organization_id", organizationID, "days", s.days,
		"services_scanned", res.ServicesScanned, "services_excluded", res.ServicesExcluded,
		"services_retried", res.ServicesRetried, "services_repaired", res.ServicesRepaired,
		"warranties_created", res.WarrantiesCreated, "services_failed", res.ServicesFailed,
	}
	switch {
	case err != nil:
		s.log.Error("warranty_repair_scan_done", append(attrs, "error", err)...)
	case res.WarrantiesCreated > 0:
		s.log.Warn("warranty_repair_scan_done", attrs...)
	default:
		s.log.Info("warranty_repair_scan_done", attrs...)
	}
}
