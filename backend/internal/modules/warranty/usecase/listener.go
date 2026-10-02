package usecase

// TEC-186 (F1-06b): the service.completed consumer. One warranty per
// service item, opened in one transaction together with one
// warranty.created outbox event per new warranty.
//
// Rules (TEC-98 decisions 3 and 4, K2):
//   - no warranty when the product has no warranty period (NULL or 0);
//   - no warranty for Glorian (brand slug glorian: the service, or the unit)
//     and for units that left the system or came from outside: any
//     external_outbound movement of the unit, source external or an
//     external connection;
//   - full items: one active warranty per (vehicle, unit); a second one is
//     skipped (uq_warranties_active_full_unit is the final barrier, its
//     violation is swallowed and logged). Partial cuts are exempt;
//   - idempotent: UNIQUE (service_item_id) + ON CONFLICT DO NOTHING; an item
//     that already has a warranty writes neither a row nor an event.
//
// start_at is services.completed_at; end_at is EndAt(start, months, org
// zone).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// GlorianBrandSlug is the brand without warranties (K2: warranty, service
// and customers are closed to Glorian).
const GlorianBrandSlug = "glorian"

// activeFullUnitIndex is the partial unique index of decision 3.
const activeFullUnitIndex = "uq_warranties_active_full_unit"

// Reasons an item gets no new warranty.
const (
	SkipExists         = "exists"
	SkipNoPeriod       = "no_warranty_period"
	SkipGlorian        = "glorian"
	SkipExternal       = "external_outbound"
	SkipActiveFull     = "active_full_warranty"
	SkipServiceNotDone = "service_not_completed"
)

// Item kinds (service_items.kind).
const (
	KindFull    = "full"
	KindPartial = "partial"
)

// CreateResult reports one run over a service.
type CreateResult struct {
	Created []db.Warranty
	// Skipped maps a service item id to the reason it got no new warranty.
	Skipped map[int64]string
}

// Listener opens warranties from service.completed.
type Listener struct {
	pool          TxBeginner
	q             *db.Queries
	out           outbox.Enqueuer
	verifyBaseURL string
	log           *slog.Logger
}

// NewListener builds the consumer. verifyBaseURL is the public frontend
// origin of the /garanti/{public_code} link; log may be nil.
func NewListener(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, verifyBaseURL string, log *slog.Logger) *Listener {
	if log == nil {
		log = slog.Default()
	}
	return &Listener{
		pool: pool, q: q, out: out,
		verifyBaseURL: strings.TrimRight(verifyBaseURL, "/"),
		log:           log,
	}
}

// HandleServiceCompleted is the bus handler of service.completed. The
// payload's service_id (or the event's entity id) names the service; every
// other value is read from the database.
func (l *Listener) HandleServiceCompleted(ctx context.Context, ev events.Event) error {
	serviceID, ok := payloadInt64(ev.Payload, "service_id")
	if !ok && ev.EntityID != nil {
		serviceID, ok = *ev.EntityID, true
	}
	if !ok || serviceID <= 0 {
		l.log.Warn("warranty_listener_no_service", "event_id", ev.EventID.String())
		return nil
	}
	res, err := l.CreateForService(ctx, serviceID)
	if err != nil {
		return fmt.Errorf("warranty: service %d: %w", serviceID, err)
	}
	l.log.Info("warranty_listener_done", "event_id", ev.EventID.String(), "service_id", serviceID,
		"created", len(res.Created), "skipped", len(res.Skipped))
	return nil
}

// CreateForService opens the warranties of a completed service in one
// transaction. Running it again creates and emits nothing.
func (l *Listener) CreateForService(ctx context.Context, serviceID int64) (CreateResult, error) {
	res := CreateResult{Skipped: map[int64]string{}}
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("warranty: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := l.q.WithTx(tx)

	svc, err := q.GetWarrantyServiceContext(ctx, serviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		l.log.Warn("warranty_listener_service_missing", "service_id", serviceID)
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("warranty: service: %w", err)
	}
	items, err := q.ListWarrantyCandidatesByService(ctx, serviceID)
	if err != nil {
		return res, fmt.Errorf("warranty: items: %w", err)
	}
	if svc.Status != "completed" || !svc.CompletedAt.Valid {
		for _, it := range items {
			res.Skipped[it.ID] = SkipServiceNotDone
		}
		l.log.Warn("warranty_listener_service_not_completed", "service_id", serviceID, "status", svc.Status)
		return res, nil
	}

	start := svc.CompletedAt.Time
	loc := orgLocation(svc.Timezone)
	for _, it := range items {
		if reason := skipReason(svc, it); reason != "" {
			res.Skipped[it.ID] = reason
			l.log.Info("warranty_skipped", "service_id", serviceID, "service_item_id", it.ID,
				"unit_id", it.UnitID, "product_id", it.ProductID, "reason", reason)
			continue
		}
		w, reason, err := l.createOne(ctx, tx, q, svc, it, start, loc)
		if err != nil {
			return CreateResult{}, err
		}
		if reason != "" {
			res.Skipped[it.ID] = reason
			l.log.Info("warranty_skipped", "service_id", serviceID, "service_item_id", it.ID,
				"unit_id", it.UnitID, "vehicle_id", svc.VehicleID, "reason", reason)
			continue
		}
		res.Created = append(res.Created, w)
	}

	if len(res.Created) > 0 {
		if err := l.emitCreated(ctx, tx, q, svc, res.Created); err != nil {
			return CreateResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, fmt.Errorf("warranty: commit: %w", err)
	}
	return res, nil
}

// skipReason applies the rules that need no write: Glorian, external
// units and the warranty period of the product.
func skipReason(svc db.GetWarrantyServiceContextRow, it db.ListWarrantyCandidatesByServiceRow) string {
	switch {
	case strings.EqualFold(svc.BrandSlug, GlorianBrandSlug) || strings.EqualFold(it.UnitBrandSlug, GlorianBrandSlug):
		return SkipGlorian
	case it.ExternalOutbound || it.UnitSource == "external" || it.UnitConnectionID.Valid:
		return SkipExternal
	case !it.WarrantyDurationMonths.Valid || it.WarrantyDurationMonths.Int32 <= 0:
		return SkipNoPeriod
	}
	return ""
}

// createOne inserts the warranty of one item. It returns a skip reason
// instead of a row when the item already has a warranty or, for a full
// unit, the vehicle already holds an active full warranty on that unit.
func (l *Listener) createOne(ctx context.Context, tx pgx.Tx, q *db.Queries, svc db.GetWarrantyServiceContextRow,
	it db.ListWarrantyCandidatesByServiceRow, start time.Time, loc *time.Location) (db.Warranty, string, error) {
	if _, err := q.GetWarrantyByServiceItem(ctx, it.ID); err == nil {
		return db.Warranty{}, SkipExists, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.Warranty{}, "", fmt.Errorf("warranty: existing: %w", err)
	}
	if it.Kind == KindFull {
		if _, err := q.GetActiveFullWarrantyByVehicleUnit(ctx, db.GetActiveFullWarrantyByVehicleUnitParams{
			VehicleID: svc.VehicleID, UnitID: it.UnitID,
		}); err == nil {
			return db.Warranty{}, SkipActiveFull, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.Warranty{}, "", fmt.Errorf("warranty: active full: %w", err)
		}
	}

	// A savepoint keeps the transaction usable when a concurrent run wins
	// the active full unit index.
	sp, err := tx.Begin(ctx)
	if err != nil {
		return db.Warranty{}, "", fmt.Errorf("warranty: savepoint: %w", err)
	}
	w, err := q.WithTx(sp).CreateWarrantyForServiceItem(ctx, db.CreateWarrantyForServiceItemParams{
		ServiceItemID: it.ID,
		HolderUserID:  svc.CustomerUserID,
		StartAt:       ts(start),
		EndAt:         ts(EndAt(start, int(it.WarrantyDurationMonths.Int32), loc)),
	})
	switch {
	case err == nil:
		if err := sp.Commit(ctx); err != nil {
			return db.Warranty{}, "", fmt.Errorf("warranty: release savepoint: %w", err)
		}
		return w, "", nil
	case errors.Is(err, pgx.ErrNoRows):
		// ON CONFLICT (service_item_id): a concurrent run created it.
		_ = sp.Rollback(ctx)
		return db.Warranty{}, SkipExists, nil
	case isActiveFullViolation(err):
		_ = sp.Rollback(ctx)
		return db.Warranty{}, SkipActiveFull, nil
	default:
		_ = sp.Rollback(ctx)
		return db.Warranty{}, "", fmt.Errorf("warranty: create item %d: %w", it.ID, err)
	}
}

// emitCreated writes one warranty.created event per new warranty.
func (l *Listener) emitCreated(ctx context.Context, tx pgx.Tx, q *db.Queries, svc db.GetWarrantyServiceContextRow,
	created []db.Warranty) error {
	ids := make([]int64, len(created))
	for i, w := range created {
		ids[i] = w.ID
	}
	list, err := q.ListWarrantyNoticeContexts(ctx, ids)
	if err != nil {
		return fmt.Errorf("warranty: notice context: %w", err)
	}
	ctxs := make(map[int64]db.ListWarrantyNoticeContextsRow, len(list))
	for _, r := range list {
		ctxs[r.ID] = r
	}
	for _, w := range created {
		extra := map[string]any{
			"service_uuid":    svc.Uuid.String(),
			"service_no":      svc.ServiceNo,
			"service_item_id": w.ServiceItemID,
			"unit_id":         w.UnitID,
			"item_kind":       w.ItemKind,
			"start_at":        w.StartAt.Time.UTC().Format(time.RFC3339),
		}
		ev := warrantyEvent(events.WarrantyCreated, w, ctxs[w.ID], verifyURL(l.verifyBaseURL, w.PublicCode), 0, extra)
		if err := l.out.Enqueue(ctx, tx, ev); err != nil {
			return fmt.Errorf("warranty: created event: %w", err)
		}
	}
	return nil
}

func isActiveFullViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == activeFullUnitIndex
}

// payloadInt64 reads a numeric payload value (int64 after the outbox round
// trip, float64 or a json.Number from other decoders).
func payloadInt64(p map[string]any, key string) (int64, bool) {
	switch v := p[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		if v != float64(int64(v)) {
			return 0, false
		}
		return int64(v), true
	case interface{ Int64() (int64, error) }:
		n, err := v.Int64()
		return n, err == nil
	}
	return 0, false
}
