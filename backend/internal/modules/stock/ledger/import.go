package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// ImportKeyPrefix starts the idempotency key of every imported movement, so
// imported history is told apart from movements Post wrote.
const ImportKeyPrefix = "legacy:"

// Recorded is a movement that already happened elsewhere (the legacy
// systems, TEC-258) and is appended as recorded.
//
// Unlike Post, Import does not plan the movement: the caller gives the
// owners, statuses and quantity the history says, the transition table is
// not applied (legacy history does not follow it), no projection is
// touched and no outbox event is written (an import must not trigger
// accounting or notifications). The caller rebuilds the projections from
// the ledger in the same transaction afterwards (rebuild.ApplyTx), which
// keeps them derived from stock_movements like every Post.
type Recorded struct {
	// UUID is the movement's uuid (the migration_map target); zero: new.
	UUID   uuid.UUID
	UnitID int64
	Type   MovementType
	// OrganizationID is the holder the movement is recorded on (see
	// Movement): before the movement, or after it when the unit had no
	// owner (an opening); for fixed barcodes the touched owner's holder.
	OrganizationID int64
	// From and To are the owners before and after (nil: none / unknown).
	From, To *Owner
	// FromStatus and ToStatus ("" : unknown).
	FromStatus, ToStatus Status
	// QuantityDelta: serial units +1 entering and -1 leaving counted stock;
	// fixed barcodes the signed quantity at the touched owner.
	QuantityDelta int32
	// Key is the idempotency key; it must start with ImportKeyPrefix.
	Key       string
	Reason    string
	Metadata  map[string]any
	CreatedAt time.Time // zero: now
}

// importableTypes are the movement types an import may record.
var importableTypes = append(slices.Clone(PostableTypes), TypeReclassification)

// Import appends r. A repeated key writes nothing and returns the earlier
// row (Replayed).
func (l *Ledger) Import(ctx context.Context, tx pgx.Tx, r Recorded) (Result, error) {
	if tx == nil {
		return Result{}, errors.New("ledger: transaction required")
	}
	if !slices.Contains(importableTypes, r.Type) {
		return Result{}, fmt.Errorf("%w: type %q", ErrInvalidMovement, r.Type)
	}
	if r.UnitID <= 0 || r.OrganizationID <= 0 {
		return Result{}, fmt.Errorf("%w: unit and organization required", ErrInvalidMovement)
	}
	if !strings.HasPrefix(r.Key, ImportKeyPrefix) || len(r.Key) > maxKeyLen || strings.TrimSpace(r.Key) == ImportKeyPrefix {
		return Result{}, fmt.Errorf("%w: import key %q", ErrInvalidMovement, r.Key)
	}
	for _, o := range []*Owner{r.From, r.To} {
		if o != nil && (o.ID <= 0 || !slices.Contains([]OwnerType{OwnerWarehouseLocation, OwnerOrganization, OwnerService, OwnerTrash}, o.Type)) {
			return Result{}, fmt.Errorf("%w: owner %s %d", ErrInvalidMovement, o.Type, o.ID)
		}
	}
	q := l.q.WithTx(tx)
	// Same first lock as Post: the unit's movements stay serialised.
	unit, err := q.LockUnit(ctx, r.UnitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrUnitNotFound
	}
	if err != nil {
		return Result{}, fmt.Errorf("ledger: lock unit: %w", err)
	}
	m := Movement{Type: r.Type}
	if res, done, err := replay(ctx, q, r.Key, unit, m); done || err != nil {
		return res, err
	}

	meta := r.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return Result{}, fmt.Errorf("%w: metadata: %v", ErrInvalidMovement, err)
	}
	arg := db.ImportStockMovementParams{
		OrganizationID: r.OrganizationID, BrandID: unit.BrandID, UnitID: unit.ID, ProductID: unit.ProductID,
		Type: string(r.Type), QuantityDelta: r.QuantityDelta, MetersDelta: cmToNumeric(0),
		FromStatus: text(string(r.FromStatus)), ToStatus: text(string(r.ToStatus)),
		Reason: text(r.Reason), Metadata: body, IdempotencyKey: r.Key,
	}
	if r.UUID != uuid.Nil {
		arg.Uuid = pgtype.UUID{Bytes: r.UUID, Valid: true}
	}
	if !r.CreatedAt.IsZero() {
		arg.CreatedAt = pgtype.Timestamptz{Time: r.CreatedAt, Valid: true}
	}
	if r.From != nil {
		arg.FromOwnerType, arg.FromOwnerID = text(string(r.From.Type)), i8(r.From.ID)
	}
	if r.To != nil {
		arg.ToOwnerType, arg.ToOwnerID = text(string(r.To.Type)), i8(r.To.ID)
	}
	mv, err := q.ImportStockMovement(ctx, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		res, _, err := replay(ctx, q, r.Key, unit, m)
		return res, err
	}
	if err != nil {
		return Result{}, dbErr("import movement", err)
	}
	return Result{Movement: mv}, nil
}
