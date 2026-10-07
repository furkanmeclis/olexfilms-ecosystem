package usecase

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusReapplied = "reapplied"
	StatusClosed    = "closed"

	reapplyPackage  = "Garanti yeniden uygulama"
	serviceDraft    = "draft"
	serviceNoPrefix = "DS"
	serviceNoLength = 8
	serviceNoTries  = 10
	serviceNoChars  = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

var ErrReapplyOpen = errors.New("warranty claims: reapply service is still open")

type ReapplyServiceView = model.ServiceRef

func RegisterEventHandlers(bus events.Bus, pool txBeginner, q *db.Queries, out outbox.Enqueuer, log *slog.Logger) {
	if bus == nil || pool == nil || q == nil {
		return
	}
	svc := &Service{q: txStore{Queries: q, pool: pool}, out: out, now: func() time.Time { return time.Now().UTC() }}
	if log == nil {
		log = slog.Default()
	}
	bus.Subscribe(events.WarrantyClaimStatusChanged, func(ctx context.Context, ev events.Event) error {
		to, _ := ev.Payload["to"].(string)
		action, _ := ev.Payload["action"].(string)
		if to != StatusApproved || action == EventActionReopened {
			return nil
		}
		claimID, ok := eventID(ev)
		if !ok {
			return nil
		}
		brandID, ok := payloadInt64(ev.Payload["brand_id"])
		if !ok {
			return nil
		}
		_, err := svc.createReapplyServiceByClaimID(ctx, claimID, brandID, ev.ActorUserID, false)
		if err != nil {
			log.Error("warranty_claim_reapply_create_failed", "claim_id", claimID, "error", err)
		}
		return err
	})
	bus.Subscribe(events.ServiceCompleted, func(ctx context.Context, ev events.Event) error {
		return svc.handleReapplyServiceFinal(ctx, ev, StatusClosed)
	})
	bus.Subscribe(events.ServiceCancelled, func(ctx context.Context, ev events.Event) error {
		return svc.handleReapplyServiceFinal(ctx, ev, StatusApproved)
	})
}

func (s *Service) ReapplyService(ctx context.Context, c Caller, id uuid.UUID) (ReapplyServiceView, error) {
	if c.OrgType == "customer" || c.OrganizationID <= 0 || !has(c, rbac.PermWarrantyClaimsWrite) {
		return ReapplyServiceView{}, ErrForbidden
	}
	tx, err := s.q.Begin(ctx)
	if err != nil {
		return ReapplyServiceView{}, fmt.Errorf("warranty claims: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := db.New(tx)
	claim, err := qtx.GetWarrantyClaimByUUIDForUpdate(ctx, db.GetWarrantyClaimByUUIDForUpdateParams{
		Uuid: id, BrandID: c.BrandID, OrganizationIds: c.Filter.OrgIDsArg(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReapplyServiceView{}, ErrNotFound
	}
	if err != nil {
		return ReapplyServiceView{}, err
	}
	if !c.Filter.AllowsOrg(claim.OrganizationID, claim.BrandID) {
		return ReapplyServiceView{}, ErrForbidden
	}
	out, err := s.createReapplyServiceLocked(ctx, qtx, tx, claim, int8(c.UserID), true)
	if err != nil {
		return ReapplyServiceView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReapplyServiceView{}, fmt.Errorf("warranty claims: commit: %w", err)
	}
	return serviceRef(out), nil
}

func (s *Service) createReapplyServiceByClaimID(ctx context.Context, claimID, brandID int64, actor *int64, api bool) (db.Service, error) {
	tx, err := s.q.Begin(ctx)
	if err != nil {
		return db.Service{}, fmt.Errorf("warranty claims: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := db.New(tx)
	claim, err := qtx.GetWarrantyClaimByIDForUpdate(ctx, db.GetWarrantyClaimByIDForUpdateParams{ID: claimID, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Service{}, nil
	}
	if err != nil {
		return db.Service{}, err
	}
	var actorID pgtype.Int8
	if actor != nil {
		actorID = int8(*actor)
	}
	out, err := s.createReapplyServiceLocked(ctx, qtx, tx, claim, actorID, api)
	if err != nil {
		return db.Service{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Service{}, fmt.Errorf("warranty claims: commit: %w", err)
	}
	return out, nil
}

func (s *Service) createReapplyServiceLocked(
	ctx context.Context,
	q *db.Queries,
	tx pgx.Tx,
	claim db.WarrantyClaim,
	actor pgtype.Int8,
	api bool,
) (db.Service, error) {
	if claim.Status == StatusReapplied && claim.ReapplyServiceID.Valid {
		existing, err := q.GetWarrantyClaimReapplyService(ctx, db.GetWarrantyClaimReapplyServiceParams{
			ReapplyServiceID: claim.ReapplyServiceID.Int64, BrandID: claim.BrandID, ClaimID: int8(claim.ID),
		})
		if err == nil {
			if existing.Status == "cancelled" {
				claim, err = q.ClearWarrantyClaimReapplyService(ctx, db.ClearWarrantyClaimReapplyServiceParams{
					ID: claim.ID, BrandID: claim.BrandID, ReapplyServiceID: int8(existing.ID), ActorUserID: actor,
				})
				if err != nil {
					return db.Service{}, fmt.Errorf("warranty claims: clear cancelled reapply service: %w", err)
				}
				if err := s.emitStatus(ctx, tx, claim, StatusReapplied, StatusApproved); err != nil {
					return db.Service{}, err
				}
			} else {
				if api {
					return db.Service{}, ErrReapplyOpen
				}
				return existing, nil
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.Service{}, err
		}
	}
	if claim.Status != StatusApproved {
		if api {
			return db.Service{}, ErrUnsupportedFlow
		}
		return db.Service{}, nil
	}
	base, err := q.GetService(ctx, db.GetServiceParams{ID: claim.ServiceID, BrandID: claim.BrandID})
	if err != nil {
		return db.Service{}, fmt.Errorf("warranty claims: claimed service: %w", err)
	}
	no, err := allocateServiceNo(ctx, q)
	if err != nil {
		return db.Service{}, err
	}
	row, err := q.CreateService(ctx, db.CreateServiceParams{
		ServiceNo: no, OrganizationID: claim.OrganizationID, BrandID: claim.BrandID,
		CustomerUserID: claim.CustomerUserID, VehicleID: claim.VehicleID,
		CarBrandID: base.CarBrandID, CarModelID: base.CarModelID,
		ModelYear: base.ModelYear, Plate: base.Plate, PlateCountry: base.PlateCountry, Vin: base.Vin,
		Package: text(reapplyPackage), HasMeasurement: false,
		Status: serviceDraft, CreatedByUserID: actor, WarrantyClaimID: int8(claim.ID),
	})
	if err != nil {
		return db.Service{}, fmt.Errorf("warranty claims: create reapply service: %w", err)
	}
	items, err := q.ListWarrantyClaimReapplyItems(ctx, claim.ID)
	if err != nil {
		return db.Service{}, fmt.Errorf("warranty claims: reapply items: %w", err)
	}
	for _, item := range items {
		parts, err := json.Marshal([]string{item.PartKey})
		if err != nil {
			return db.Service{}, err
		}
		if _, err := q.CreateServiceItem(ctx, db.CreateServiceItemParams{
			ServiceID: row.ID, ProductID: item.ProductID, UnitID: item.UnitID, Kind: item.Kind,
			Quantity: item.Quantity, Meters: item.Meters, AppliedParts: parts,
		}); err != nil {
			return db.Service{}, fmt.Errorf("warranty claims: create reapply item: %w", err)
		}
	}
	linked, err := q.LinkWarrantyClaimReapplyService(ctx, db.LinkWarrantyClaimReapplyServiceParams{
		ID: claim.ID, BrandID: claim.BrandID, ReapplyServiceID: int8(row.ID), ActorUserID: actor,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Service{}, ErrReapplyOpen
	}
	if err != nil {
		return db.Service{}, fmt.Errorf("warranty claims: link reapply service: %w", err)
	}
	if err := s.emitStatus(ctx, tx, linked, StatusApproved, StatusReapplied); err != nil {
		return db.Service{}, err
	}
	return row, nil
}

func (s *Service) handleReapplyServiceFinal(ctx context.Context, ev events.Event, target string) error {
	serviceID, ok := payloadInt64(ev.Payload["service_id"])
	if !ok && ev.EntityID != nil {
		serviceID, ok = *ev.EntityID, *ev.EntityID > 0
	}
	if !ok {
		return nil
	}
	brandID, ok := payloadInt64(ev.Payload["brand_id"])
	if !ok {
		return nil
	}
	tx, err := s.q.Begin(ctx)
	if err != nil {
		return fmt.Errorf("warranty claims: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := db.New(tx)
	svc, err := qtx.GetService(ctx, db.GetServiceParams{ID: serviceID, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !svc.WarrantyClaimID.Valid) {
		return nil
	}
	if err != nil {
		return err
	}
	claim, err := qtx.GetWarrantyClaimByIDForUpdate(ctx, db.GetWarrantyClaimByIDForUpdateParams{
		ID: svc.WarrantyClaimID.Int64, BrandID: brandID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var actor pgtype.Int8
	if ev.ActorUserID != nil {
		actor = int8(*ev.ActorUserID)
	}
	switch target {
	case StatusClosed:
		if claim.Status != StatusReapplied || !claim.ReapplyServiceID.Valid || claim.ReapplyServiceID.Int64 != svc.ID {
			return nil
		}
		updated, err := qtx.SetWarrantyClaimStatus(ctx, db.SetWarrantyClaimStatusParams{
			ID: claim.ID, BrandID: claim.BrandID, FromStatus: StatusReapplied, Status: StatusClosed, ActorUserID: actor,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.emitStatus(ctx, tx, updated, StatusReapplied, StatusClosed); err != nil {
			return err
		}
	case StatusApproved:
		if claim.Status != StatusReapplied || !claim.ReapplyServiceID.Valid || claim.ReapplyServiceID.Int64 != svc.ID {
			return nil
		}
		updated, err := qtx.ClearWarrantyClaimReapplyService(ctx, db.ClearWarrantyClaimReapplyServiceParams{
			ID: claim.ID, BrandID: claim.BrandID, ReapplyServiceID: int8(svc.ID), ActorUserID: actor,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.emitStatus(ctx, tx, updated, StatusReapplied, StatusApproved); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func allocateServiceNo(ctx context.Context, q *db.Queries) (string, error) {
	for i := 0; i < serviceNoTries; i++ {
		no, err := newServiceNo()
		if err != nil {
			return "", err
		}
		exists, err := q.ServiceNoExists(ctx, no)
		if err != nil {
			return "", fmt.Errorf("warranty claims: service number check: %w", err)
		}
		if !exists {
			return no, nil
		}
	}
	return "", errors.New("warranty claims: could not allocate service number")
}

func newServiceNo() (string, error) {
	var b strings.Builder
	b.WriteString(serviceNoPrefix)
	max := big.NewInt(int64(len(serviceNoChars)))
	for i := 0; i < serviceNoLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("warranty claims: service number: %w", err)
		}
		b.WriteByte(serviceNoChars[n.Int64()])
	}
	return b.String(), nil
}

func eventID(ev events.Event) (int64, bool) {
	if ev.EntityID != nil && *ev.EntityID > 0 {
		return *ev.EntityID, true
	}
	return payloadInt64(ev.Payload["claim_id"])
}

func payloadInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, n > 0
	case int:
		return int64(n), n > 0
	case int32:
		return int64(n), n > 0
	case float64:
		return int64(n), n > 0
	case json.Number:
		i, err := n.Int64()
		return i, err == nil && i > 0
	default:
		return 0, false
	}
}

func serviceRef(row db.Service) ReapplyServiceView {
	return ReapplyServiceView{UUID: row.Uuid, ServiceNo: row.ServiceNo, Status: row.Status}
}
