package usecase

// Roll split (TEC-184): a user with stock.write in the organization that
// holds a roll on hand cuts N meters off it as a new unit (same product,
// barcode <roll>-S<n>). ledger.Split writes both movements, the new unit
// and the stock_splits row in one transaction. The caller's idempotency key
// makes a retry return the earlier split (replayed) without writing again.
// A roll on an open service or reserved for an order line is not split
// here (the order flow splits before it reserves). Printing the new label
// is TEC-95.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AuditSplit is the activity action of a roll split.
const (
	AuditResourceSplit = "stock.split"
	AuditSplitCreated  = "stock.split.created"
)

// maxSplitKeyLen bounds the caller's idempotency key.
const maxSplitKeyLen = 100

// Splits implements the roll split use case.
type Splits struct {
	pool   TxBeginner
	q      *db.Queries
	ledger *ledger.Ledger
}

// NewSplits builds the use case; stock.* events go to out.
func NewSplits(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *Splits {
	return &Splits{pool: pool, q: q, ledger: ledger.New(q, out)}
}

// SplitInput names the roll (barcode or unit uuid), the meters to cut and
// the caller's idempotency key.
type SplitInput struct {
	Barcode        string
	UnitUUID       string
	Meters         string
	IdempotencyKey string
	Reason         string
}

// Split cuts the meters off a roll held on hand by the caller's
// organization. replayed is true when the key was already used.
func (s *Splits) Split(ctx context.Context, c Caller, in SplitInput) (model.Split, bool, error) {
	key := strings.TrimSpace(in.IdempotencyKey)
	switch {
	case key == "":
		return model.Split{}, false, invalid("idempotency_key", "idempotency_key is required")
	case len(key) > maxSplitKeyLen:
		return model.Split{}, false, invalid("idempotency_key", "idempotency_key is too long")
	}
	reason := strings.TrimSpace(in.Reason)
	if len(reason) > maxReasonLen {
		return model.Split{}, false, invalid("reason", "reason is too long")
	}
	cm, err := ledger.ParseMeters(in.Meters)
	if err != nil || cm <= 0 {
		return model.Split{}, false, invalid("meters", "meters must be a positive number with at most two decimals")
	}
	unitID, err := (&Reclassifications{q: s.q}).findUnit(ctx, c, RequestInput{Barcode: in.Barcode, UnitUUID: in.UnitUUID})
	if err != nil {
		return model.Split{}, false, err
	}
	if err := checkWriteReach(ctx, s.q, c, unitID); err != nil {
		return model.Split{}, false, err
	}

	var res ledger.SplitResult
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		// A retry returns the earlier split even if the roll has been
		// reserved or moved since.
		prev, err := ledger.FindSplit(ctx, q, c.Org.InternalID, key)
		if err != nil {
			return err
		}
		if prev == nil {
			if _, err := q.LockUnit(ctx, unitID); err != nil {
				return fmt.Errorf("stock: lock unit: %w", err)
			}
			if err := checkNotInUse(ctx, q, unitID); err != nil {
				return err
			}
		}
		res, err = s.ledger.Split(ctx, tx, ledger.SplitCommand{
			SourceUnitID: unitID, Centimeters: cm, HolderOrgID: c.Org.InternalID,
			IdempotencyKey: key, ActorUserID: c.actorPtr(), Reason: reason,
		})
		if err != nil || res.Replayed {
			return err
		}
		return splitAudit(ctx, q, c, res)
	})
	if err != nil {
		return model.Split{}, false, err
	}
	return SplitView(res), res.Replayed, nil
}

func (s *Splits) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	return (&Reclassifications{pool: s.pool, q: s.q}).inTx(ctx, fn)
}

func splitAudit(ctx context.Context, q *db.Queries, c Caller, res ledger.SplitResult) error {
	payload, err := json.Marshal(map[string]any{
		"split_id": res.Split.ID, "source_unit_id": res.Split.SourceUnitID, "new_unit_id": res.Split.NewUnitID,
		"organization_id": res.Split.OrganizationID, "brand_id": res.Split.BrandID,
		"meters": meterString(res.Split.Meters), "barcode": res.Source.Barcode, "new_barcode": res.New.Barcode,
	})
	if err != nil {
		return fmt.Errorf("stock: audit payload: %w", err)
	}
	if _, err := q.InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID: c.actor(), Action: AuditSplitCreated, Resource: AuditResourceSplit,
		ResourceUuid: pgtype.UUID{Bytes: res.Split.Uuid, Valid: true}, Payload: payload,
		IpAddress: c.IP, UserAgent: pgText(c.UserAgent),
	}); err != nil {
		return fmt.Errorf("stock: audit: %w", err)
	}
	return nil
}

func meterString(n pgtype.Numeric) string {
	cm, err := ledger.NumericToCentimeters(n)
	if err != nil {
		return ""
	}
	return ledger.FormatMeters(cm)
}

// SplitView renders a split result for the API.
func SplitView(r ledger.SplitResult) model.Split {
	return model.Split{
		UUID:   r.Split.Uuid,
		Meters: meterString(r.Split.Meters),
		Source: model.SplitUnit{
			UUID: r.Source.Uuid, Barcode: r.Source.Barcode, Status: r.Source.Status,
			RemainingMeters: meterString(r.Source.RemainingMeters),
		},
		NewUnit: model.SplitUnit{
			UUID: r.New.Uuid, Barcode: r.New.Barcode, Status: r.New.Status,
			RemainingMeters: meterString(r.New.RemainingMeters),
		},
		Replayed:  r.Replayed,
		CreatedAt: r.Split.CreatedAt.Time,
	}
}
