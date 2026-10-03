package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// FindPortalServiceJob returns the newest reusable portal PDF job of the
// actor for one service (TEC-239): a completed job created at or after
// notBefore, or a queued / processing one created at or after
// pendingAfter, in the same locale. found is false when nothing can be
// reused; the caller then queues a new job. Only portal resources are
// looked up (they never carry an organization).
func (s *Service) FindPortalServiceJob(ctx context.Context, actorID int64, resource, serviceUUID, locale string,
	notBefore, pendingAfter time.Time,
) (ExportJobView, bool, error) {
	if !strings.HasPrefix(resource, PortalResourcePrefix) || actorID <= 0 || serviceUUID == "" {
		return ExportJobView{}, false, nil
	}
	row, err := s.q.GetReusablePortalServiceJob(ctx, db.GetReusablePortalServiceJobParams{
		ActorID: actorID, Resource: resource, Locale: locale, ServiceUuid: serviceUUID,
		NotBefore:    pgtype.Timestamptz{Time: notBefore, Valid: true},
		PendingAfter: pgtype.Timestamptz{Time: pendingAfter, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ExportJobView{}, false, nil
	}
	if err != nil {
		return ExportJobView{}, false, err
	}
	return mapExportJob(row), true, nil
}
