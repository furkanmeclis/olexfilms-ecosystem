package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1075: summary cards above the service list, after the legacy hub's
// ServiceOverviewStatsWidget (total, this month, by status, warranties).

// ListStatusCounts counts the services per status.
type ListStatusCounts struct {
	Draft      int64 `json:"draft"`
	Pending    int64 `json:"pending"`
	Processing int64 `json:"processing"`
	Ready      int64 `json:"ready"`
	Completed  int64 `json:"completed"`
	Cancelled  int64 `json:"cancelled"`
}

// ListStats is the response of GET /v1/services/stats.
type ListStats struct {
	Total            int64            `json:"total"`
	ThisMonth        int64            `json:"this_month"`
	MonthStart       time.Time        `json:"month_start"`
	ByStatus         ListStatusCounts `json:"by_status"`
	Warranties       int64            `json:"warranties"`
	ActiveWarranties int64            `json:"active_warranties"`
}

// MonthStart is the first instant of now's calendar month in tz (an
// unknown or empty zone falls back to UTC).
func MonthStart(now time.Time, tz string) time.Time {
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
}

// ListStats counts the services of the caller's services.read scope (the
// list's scope), optionally narrowed to organizations (the list's
// organization_uuid filter: the center's own / network switch). monthStart
// bounds this_month.
func (s *Service) ListStats(ctx context.Context, c Caller, organizations []uuid.UUID, monthStart time.Time) (ListStats, error) {
	if c.Org.BrandID == 0 || !c.Principal.HasPermission(rbac.PermServicesRead) {
		return ListStats{}, ErrForbidden
	}
	var createdBy pgtype.Int8
	if c.Filter.UserOnly() {
		createdBy = pgtype.Int8{Int64: c.Filter.UserID, Valid: true}
	}
	counts, err := s.q.ServiceListStats(ctx, db.ServiceListStatsParams{
		MonthStart: pgtype.Timestamptz{Time: monthStart, Valid: true},
		BrandID:    c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), OrganizationUuids: organizations,
		CreatedByUserID: createdBy,
	})
	if err != nil {
		return ListStats{}, fmt.Errorf("services: list stats: %w", err)
	}
	w, err := s.q.ServiceListWarrantyStats(ctx, db.ServiceListWarrantyStatsParams{
		BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), OrganizationUuids: organizations,
		CreatedByUserID: createdBy,
	})
	if err != nil {
		return ListStats{}, fmt.Errorf("services: list warranty stats: %w", err)
	}
	return ListStats{
		Total: counts.Total, ThisMonth: counts.ThisMonth, MonthStart: monthStart,
		ByStatus: ListStatusCounts{
			Draft: counts.Draft, Pending: counts.Pending, Processing: counts.Processing,
			Ready: counts.Ready, Completed: counts.Completed, Cancelled: counts.Cancelled,
		},
		Warranties: w.Total, ActiveWarranties: w.Active,
	}, nil
}
