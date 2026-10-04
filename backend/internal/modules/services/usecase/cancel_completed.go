package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Completed service cancellation (TEC-356): a completed service may be
// cancelled by privileged service managers. In one transaction it voids
// warranties, returns every whole consumption to stock, reverses open
// accounting rows sourced by the service and appends the service status log,
// activity row and outbox events.

const (
	// PermServicesCancelCompleted is seeded by migration 000088 and kept in
	// the rbac catalog. It is intentionally separate from services.cancel:
	// dealer_staff can complete services but must not cancel completed ones.
	PermServicesCancelCompleted = rbac.PermServicesCancelCompleted

	// Source type of service accounting rows (finance_entries.source_type).
	AccountingSourceType = ledgerSource

	// ActionServiceCompletedCancelled is the activity log action of this
	// composite cancellation.
	ActionServiceCompletedCancelled = "service.completed_cancelled"

	maxCompletedCancelReasonRunes = 1000
)

// CompletedCancelPoster is the accounting API needed by service cancellation.
type CompletedCancelPoster interface {
	VoidBySourceTx(ctx context.Context, tx pgx.Tx, src posting.Source, reason string, actorUserID *int64) (posting.VoidResult, error)
}

// WithCompletedCancelAccounting wires the accounting ledger reversal hook.
func (s *Service) WithCompletedCancelAccounting(p CompletedCancelPoster) *Service {
	s.poster = p
	return s
}

// CancelCompletedInput is the body of POST /v1/services/{uuid}/cancel-completed.
type CancelCompletedInput struct {
	Reason string
	Meta   activity.Meta
}

// CancelCompleted cancels a completed service. Already cancelled services
// return ErrInvalidTransition; non-completed services return ErrNotEditable.
func (s *Service) CancelCompleted(ctx context.Context, c Caller, id uuid.UUID, in CancelCompletedInput) (ServiceView, error) {
	reason, err := normalizeCompletedCancelReason(in.Reason)
	if err != nil {
		return ServiceView{}, err
	}
	var result db.Service
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := s.lockVisible(ctx, q, c, id)
		if err != nil {
			return err
		}
		if !c.allows(PermServicesCancelCompleted, svc) {
			return ErrForbidden
		}
		switch svc.Status {
		case StatusCompleted:
		case StatusCancelled:
			return ErrInvalidTransition
		default:
			return ErrNotEditable
		}

		items, err := q.LockServiceItems(ctx, svc.ID)
		if err != nil {
			return fmt.Errorf("services: lock items: %w", err)
		}
		l := ledger.New(s.q, s.out)
		returnedIDs := make([]int64, 0, len(items))
		for _, item := range items {
			mv, err := s.returnCompletedItem(ctx, q, tx, l, svc, item, c, reason)
			if err != nil {
				return err
			}
			returnedIDs = append(returnedIDs, mv.ID)
		}

		voided, err := q.VoidWarrantiesByService(ctx, db.VoidWarrantiesByServiceParams{
			ServiceID: svc.ID, BrandID: svc.BrandID, ActorUserID: c.actor(), VoidReason: pgtype.Text{String: "service_cancelled", Valid: true},
		})
		if err != nil {
			return fmt.Errorf("services: void warranties: %w", err)
		}
		if err := s.emitWarrantyVoids(ctx, tx, svc, voided, c, reason); err != nil {
			return err
		}

		var reversals []int64
		if s.poster != nil {
			vr, err := s.poster.VoidBySourceTx(ctx, tx, posting.Source{Type: AccountingSourceType, UUID: svc.Uuid}, reason, actorPtr(c))
			if err != nil {
				return fmt.Errorf("services: accounting reversal: %w", err)
			}
			for _, r := range vr.Reversals {
				reversals = append(reversals, r.ID)
			}
		}

		from := svc.Status
		result, err = q.CancelCompletedService(ctx, db.CancelCompletedServiceParams{
			ID: svc.ID, ActorUserID: c.actor(), CancelReason: pgtype.Text{String: reason, Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidTransition
		}
		if err != nil {
			return fmt.Errorf("services: cancel completed: %w", err)
		}
		meta := map[string]any{
			"reason": reason, "warranty_void_reason": "service_cancelled",
			"return_movement_ids": returnedIDs, "voided_warranty_ids": warrantyIDs(voided),
			"accounting_reversal_ids": reversals,
		}
		if err := s.log(ctx, q, result, from, StatusCancelled, c, &reason, meta); err != nil {
			return err
		}
		if err := s.auditCancelCompleted(ctx, q, result, c, reason, meta, in.Meta); err != nil {
			return err
		}
		return s.emit(ctx, tx, events.ServiceCancelled, result, from, c, meta)
	})
	if err != nil {
		return ServiceView{}, err
	}
	return s.view(ctx, s.q, c, result)
}

func normalizeCompletedCancelReason(raw string) (string, error) {
	reason := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(reason)
	if n == 0 {
		return "", invalid("reason", "is required")
	}
	if n > maxCompletedCancelReasonRunes {
		return "", invalid("reason", fmt.Sprintf("must be at most %d characters", maxCompletedCancelReasonRunes))
	}
	return reason, nil
}

func (s *Service) returnCompletedItem(ctx context.Context, q *db.Queries, tx pgx.Tx, l *ledger.Ledger, svc db.Service,
	item db.ServiceItem, c Caller, reason string) (db.StockMovement, error) {
	if !item.StockMovementID.Valid {
		return db.StockMovement{}, ErrConsumptionNotReversible
	}
	consumed, err := q.GetStockMovement(ctx, item.StockMovementID.Int64)
	if err != nil {
		return db.StockMovement{}, fmt.Errorf("services: item movement: %w", err)
	}
	if consumed.Type != string(ledger.TypeConsumption) || consumed.UnitID != item.UnitID {
		return db.StockMovement{}, ErrConsumptionNotReversible
	}
	return s.returnConsumption(ctx, q, tx, l, svc, item, consumed, c, reason)
}

func (s *Service) emitWarrantyVoids(ctx context.Context, tx pgx.Tx, svc db.Service, ws []db.Warranty, c Caller, reason string) error {
	if s.out == nil {
		return nil
	}
	for _, w := range ws {
		wid, wuid := w.ID, w.Uuid
		ev := events.New(events.WarrantyVoided).
			WithTenant(w.OrganizationID).
			WithEntity("warranty", &wid, &wuid).
			WithPayload(map[string]any{
				"warranty_uuid": w.Uuid.String(), "public_code": w.PublicCode,
				"service_uuid": svc.Uuid.String(), "service_no": svc.ServiceNo,
				"service_cancel_reason": reason, "void_reason": "service_cancelled",
				"organization_id": w.OrganizationID, "brand_id": w.BrandID,
			})
		if c.Principal.UserInternal != 0 {
			ev = ev.WithActor(c.Principal.UserInternal)
		}
		if err := s.out.Enqueue(ctx, tx, ev); err != nil {
			return fmt.Errorf("services: warranty void event: %w", err)
		}
	}
	return nil
}

func (s *Service) auditCancelCompleted(ctx context.Context, q *db.Queries, svc db.Service, c Caller, reason string,
	meta map[string]any, origin activity.Meta) error {
	payload := map[string]any{
		"service_no": svc.ServiceNo, "reason": reason, "organization_id": svc.OrganizationID, "brand_id": svc.BrandID,
	}
	for k, v := range meta {
		payload[k] = v
	}
	if c.Org.UUID != uuid.Nil {
		payload["actor_organization_uuid"] = c.Org.UUID.String()
	}
	sid := svc.Uuid
	if err := activity.Write(ctx, q, actorPtr(c), ActionServiceCompletedCancelled, "services", &sid, payload, origin); err != nil {
		return fmt.Errorf("services: audit: %w", err)
	}
	return nil
}

func actorPtr(c Caller) *int64 {
	if c.Principal.UserInternal == 0 {
		return nil
	}
	uid := c.Principal.UserInternal
	return &uid
}

func warrantyIDs(ws []db.Warranty) []int64 {
	out := make([]int64, len(ws))
	for i, w := range ws {
		out[i] = w.ID
	}
	return out
}
