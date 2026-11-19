package usecase

// TEC-1221 (kredili kod): the credit card of a unit and the super admin
// credit increase.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/credits"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ActionUnitCreditsIncreased is the activity action of a credit increase.
const ActionUnitCreditsIncreased = "stock.unit_credits_increased"

// MaxCreditReasonLength bounds the reason of a credit increase.
const MaxCreditReasonLength = 1000

// Credit errors.
var (
	// ErrCreditsNotApplicable: the unit carries no credits (roll, fixed).
	ErrCreditsNotApplicable = errors.New("stock: unit carries no credits")
)

// unitCredits builds the credit card of u; nil for a unit without
// credits. Services outside reach (another organization's) are left out.
func unitCredits(ctx context.Context, q *db.Queries, u db.Unit, reaches func(orgID, brandID int64) bool) (*model.UnitCredits, error) {
	b, ok, err := credits.Get(ctx, q, u)
	if err != nil || !ok {
		return nil, err
	}
	out := &model.UnitCredits{
		UnitUUID: u.Uuid, Total: b.Total, Remaining: b.Remaining, UsedCount: b.Used,
		Services: []model.CreditService{}, Movements: []model.CreditMovement{},
	}
	svcs, err := q.ListUnitCreditServices(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("stock: credit services: %w", err)
	}
	for _, sv := range svcs {
		if reaches != nil && !reaches(sv.OrganizationID, u.BrandID) {
			continue
		}
		cs := model.CreditService{
			UUID: sv.Uuid, ServiceNo: sv.ServiceNo, Status: sv.Status, FinishedProduct: sv.FinishedProduct,
			CreditsTaken: sv.CreditsTaken, CreatedAt: sv.CreatedAt.Time,
		}
		if sv.CompletedAt.Valid {
			t := sv.CompletedAt.Time
			cs.CompletedAt = &t
		}
		out.Services = append(out.Services, cs)
	}
	mvs, err := q.ListUnitCreditMovements(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("stock: credit movements: %w", err)
	}
	for _, m := range mvs {
		cm := model.CreditMovement{
			UUID: m.Uuid, Type: m.Type, TotalDelta: m.TotalDelta, RemainingDelta: m.RemainingDelta,
			CreatedAt: m.CreatedAt.Time,
		}
		if m.Reason.Valid {
			r := m.Reason.String
			cm.Reason = &r
		}
		if m.ServiceUuid.Valid {
			id := uuid.UUID(m.ServiceUuid.Bytes)
			cm.ServiceUUID = &id
		}
		if m.ServiceNo.Valid {
			n := m.ServiceNo.String
			cm.ServiceNo = &n
		}
		if m.ActorName.Valid {
			n := strings.TrimSpace(m.ActorName.String + " " + m.ActorSurname.String)
			cm.ActorName = &n
		}
		out.Movements = append(out.Movements, cm)
	}
	return out, nil
}

// UnitCredits is the super admin credit increase (TEC-1221).
type UnitCredits struct {
	pool TxBeginner
	q    *db.Queries
}

// NewUnitCredits builds the use case.
func NewUnitCredits(pool TxBeginner, q *db.Queries) *UnitCredits {
	return &UnitCredits{pool: pool, q: q}
}

// IncreaseInput adds Amount credits to the unit with a reason.
type IncreaseInput struct {
	Amount  int32
	Reason  string
	ActorID *int64
	Meta    activity.Meta
}

// Increase adds credits to the unit (platform super admin; the route
// checks it), as a credit ledger movement and an activity row.
func (s *UnitCredits) Increase(ctx context.Context, unitUUID uuid.UUID, in IncreaseInput) (*model.UnitCredits, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > MaxCreditReasonLength {
		return nil, &ValidationError{Field: "reason", Message: fmt.Sprintf("is required (at most %d characters)", MaxCreditReasonLength)}
	}
	if in.Amount <= 0 || in.Amount > credits.MaxCredits {
		return nil, &ValidationError{Field: "amount", Message: fmt.Sprintf("must be between 1 and %d", credits.MaxCredits)}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("stock: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	u, err := q.GetUnitByUUID(ctx, unitUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stock: unit: %w", err)
	}
	if u, err = q.LockUnit(ctx, u.ID); err != nil {
		return nil, fmt.Errorf("stock: lock unit: %w", err)
	}
	// TEC-1238: a cut piece of a meter line carries credits too.
	if ok, err := credits.Carries(ctx, q, u); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrCreditsNotApplicable
	}
	b, err := credits.Increase(ctx, q, u, credits.Actor{OrgID: u.OrganizationID, UserID: in.ActorID}, in.Amount, reason)
	if err != nil {
		return nil, err
	}
	uid := u.Uuid
	if err := activity.Write(ctx, q, in.ActorID, ActionUnitCreditsIncreased, "stock_units", &uid, map[string]any{
		"barcode": u.Barcode, "amount": in.Amount, "reason": reason,
		"credits_total": b.Total, "credits_remaining": b.Remaining,
		"organization_id": u.OrganizationID, "brand_id": u.BrandID,
	}, in.Meta); err != nil {
		return nil, fmt.Errorf("stock: audit: %w", err)
	}
	out, err := unitCredits(ctx, q, u, nil)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("stock: commit: %w", err)
	}
	return out, nil
}
