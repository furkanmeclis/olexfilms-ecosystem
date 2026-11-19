// Package credits is the TEC-1221 unit usage credit ledger (kredili kod,
// karar TEC-1084).
//
// A serial piece unit (a barcode without meters) can be used in as many
// services as it has credits; rolls and fixed barcodes carry none. A cut
// piece shipped on a meter line carries that line's credits (TEC-1238,
// K3/K4) and is used the same way: one credit per service, its meters are
// never cut (TEC-1242). Every
// change is an append-only unit_credit_movements row and the current value
// is the unit_credit_balances projection, written in the caller's
// transaction:
//
//   - opening: the first credit event of a unit opens its balance with
//     unit_credit_default (the plotter credits of the line it was shipped
//     on, else the product's usage_credits);
//   - service_use: selecting the unit in a service takes one credit;
//   - service_finish: "ürünü bitirdim" takes all the remaining credits;
//   - service_refund: removing the item or cancelling the service gives
//     back what the item took;
//   - increase: the platform super admin adds credits (reason required).
//
// The caller locks the unit row first (LockUnit); every function here
// expects it, so two services cannot take the last credit at once.
package credits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Movement types (chk_unit_credit_movements_type).
const (
	TypeOpening       = "opening"
	TypeMigration     = "migration"
	TypeServiceUse    = "service_use"
	TypeServiceFinish = "service_finish"
	TypeServiceRefund = "service_refund"
	TypeIncrease      = "increase"
)

// MaxCredits bounds a product default, a plotter override and an increase.
const MaxCredits = 1000

// Errors.
var (
	// ErrExhausted: the unit has no credits left (UNIT_CREDITS_EXHAUSTED).
	ErrExhausted = errors.New("credits: unit credits exhausted")
	// ErrNotApplicable: the unit carries no credits (roll or fixed barcode).
	ErrNotApplicable = errors.New("credits: unit carries no credits")
	// ErrInvalidAmount: an increase outside 1..MaxCredits.
	ErrInvalidAmount = errors.New("credits: invalid amount")
)

// Applies reports whether the unit carries credits by its shape alone: a
// serial unit without meters. A cut piece (a serial unit with meters) may
// also carry credits — those of the meter line it was shipped on (TEC-1238,
// K3/K4) — which only the database knows: see Carries.
func Applies(u db.Unit) bool {
	return u.UnitKind == "serial" && !u.InitialMeters.Valid
}

// mayCarry: a serial unit; whether one with meters carries credits is
// decided by unit_credit_default (its meter line's line_credits).
func mayCarry(u db.Unit) bool { return u.UnitKind == "serial" }

// Carries reports whether the unit carries credits: a credit piece
// (Applies) or a cut piece shipped on a meter line with line credits
// (TEC-1238). Rolls and cut pieces without such a line keep their meter
// consumption.
func Carries(ctx context.Context, q *db.Queries, u db.Unit) (bool, error) {
	if Applies(u) {
		return true, nil
	}
	if !mayCarry(u) {
		return false, nil
	}
	if _, err := q.GetUnitCreditBalance(ctx, u.ID); err == nil {
		return true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("credits: balance: %w", err)
	}
	def, err := q.GetUnitCreditDefault(ctx, u.ID)
	if err != nil {
		return false, fmt.Errorf("credits: default: %w", err)
	}
	return def >= 0, nil
}

// Balance is the credit state of a unit. Opened is false while the unit has
// had no credit event (the values are then the default).
type Balance struct {
	Total     int32 `json:"credits_total"`
	Remaining int32 `json:"credits_remaining"`
	Used      int32 `json:"used_count"`
	Opened    bool  `json:"-"`
}

// Exhausted reports whether no credit is left.
func (b Balance) Exhausted() bool { return b.Remaining <= 0 }

// Get reads the credit state of a unit without opening it. ok is false
// for a unit that carries no credits.
func Get(ctx context.Context, q *db.Queries, u db.Unit) (Balance, bool, error) {
	if !mayCarry(u) {
		return Balance{}, false, nil
	}
	b, err := q.GetUnitCreditBalance(ctx, u.ID)
	if err == nil {
		return Balance{Total: b.CreditsTotal, Remaining: b.CreditsRemaining, Used: b.UsedCount, Opened: true}, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Balance{}, false, fmt.Errorf("credits: balance: %w", err)
	}
	def, err := q.GetUnitCreditDefault(ctx, u.ID)
	if err != nil {
		return Balance{}, false, fmt.Errorf("credits: default: %w", err)
	}
	if def < 0 {
		return Balance{}, false, nil
	}
	return Balance{Total: def, Remaining: def}, true, nil
}

// Actor is who writes a movement and for which organization.
type Actor struct {
	OrgID  int64
	UserID *int64
}

// Ref ties a movement to a service item.
type Ref struct {
	ServiceID     int64
	ServiceItemID int64
	ServiceNo     string
}

type entry struct {
	typ                   string
	total, remaining, use int32
	ref                   *Ref
	reason                string
	key                   string
	meta                  map[string]any
}

// open locks (or opens) the balance of a credit unit.
func open(ctx context.Context, q *db.Queries, u db.Unit, a Actor) (db.UnitCreditBalance, error) {
	if !mayCarry(u) {
		return db.UnitCreditBalance{}, ErrNotApplicable
	}
	b, err := q.LockUnitCreditBalance(ctx, u.ID)
	if err == nil {
		return b, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.UnitCreditBalance{}, fmt.Errorf("credits: lock balance: %w", err)
	}
	def, err := q.GetUnitCreditDefault(ctx, u.ID)
	if err != nil {
		return db.UnitCreditBalance{}, fmt.Errorf("credits: default: %w", err)
	}
	if def < 0 {
		return db.UnitCreditBalance{}, ErrNotApplicable
	}
	mv, err := insert(ctx, q, u, a, entry{
		typ: TypeOpening, total: def, remaining: def,
		key: "credit:unit:" + strconv.FormatInt(u.ID, 10) + ":opening",
	})
	if err != nil {
		return db.UnitCreditBalance{}, err
	}
	b, err = q.InsertUnitCreditBalance(ctx, db.InsertUnitCreditBalanceParams{
		UnitID: u.ID, OrganizationID: u.OrganizationID, BrandID: u.BrandID,
		CreditsTotal: def, CreditsRemaining: def, LastMovementID: pgtype.Int8{Int64: mv.ID, Valid: true},
	})
	if err != nil {
		return db.UnitCreditBalance{}, fmt.Errorf("credits: open balance: %w", err)
	}
	return b, nil
}

func insert(ctx context.Context, q *db.Queries, u db.Unit, a Actor, e entry) (db.UnitCreditMovement, error) {
	meta := e.meta
	if meta == nil {
		meta = map[string]any{}
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return db.UnitCreditMovement{}, fmt.Errorf("credits: metadata: %w", err)
	}
	org := a.OrgID
	if org == 0 {
		org = u.OrganizationID
	}
	p := db.InsertUnitCreditMovementParams{
		OrganizationID: org, BrandID: u.BrandID, UnitID: u.ID, Type: e.typ,
		TotalDelta: e.total, RemainingDelta: e.remaining, UsesDelta: e.use,
		Metadata: raw, IdempotencyKey: e.key,
	}
	if e.ref != nil {
		p.ServiceID = pgtype.Int8{Int64: e.ref.ServiceID, Valid: true}
		p.ServiceItemID = pgtype.Int8{Int64: e.ref.ServiceItemID, Valid: true}
	}
	if a.UserID != nil {
		p.ActorUserID = pgtype.Int8{Int64: *a.UserID, Valid: true}
	}
	if e.reason != "" {
		p.Reason = pgtype.Text{String: e.reason, Valid: true}
	}
	mv, err := q.InsertUnitCreditMovement(ctx, p)
	if err != nil {
		return db.UnitCreditMovement{}, fmt.Errorf("credits: movement %s: %w", e.key, err)
	}
	return mv, nil
}

// post writes one movement and applies it to the locked balance.
func post(ctx context.Context, q *db.Queries, u db.Unit, a Actor, e entry) (db.UnitCreditBalance, error) {
	mv, err := insert(ctx, q, u, a, e)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already written (idempotency key): nothing to apply.
		return q.GetUnitCreditBalance(ctx, u.ID)
	}
	if err != nil {
		return db.UnitCreditBalance{}, err
	}
	b, err := q.ApplyUnitCreditMovement(ctx, db.ApplyUnitCreditMovementParams{
		TotalDelta: e.total, RemainingDelta: e.remaining, UsesDelta: e.use,
		MovementID: pgtype.Int8{Int64: mv.ID, Valid: true}, UnitID: u.ID,
	})
	if err != nil {
		return db.UnitCreditBalance{}, fmt.Errorf("credits: apply: %w", err)
	}
	return b, nil
}

func itemKey(ref Ref, kind string) string {
	return "credit:service_item:" + strconv.FormatInt(ref.ServiceItemID, 10) + ":" + kind
}

func refMeta(ref Ref) map[string]any {
	return map[string]any{"service_no": ref.ServiceNo}
}

// UseForItem takes one credit of the unit for a new service item and, when
// finish is set ("ürünü bitirdim"), all the credits left after it. A unit
// with no credit left answers ErrExhausted.
func UseForItem(ctx context.Context, q *db.Queries, u db.Unit, a Actor, ref Ref, finish bool) (Balance, error) {
	b, err := open(ctx, q, u, a)
	if err != nil {
		return Balance{}, err
	}
	if b.CreditsRemaining <= 0 {
		return Balance{}, ErrExhausted
	}
	if b, err = post(ctx, q, u, a, entry{
		typ: TypeServiceUse, remaining: -1, use: 1, ref: &ref,
		key: itemKey(ref, "use"), meta: refMeta(ref),
	}); err != nil {
		return Balance{}, err
	}
	if finish && b.CreditsRemaining > 0 {
		if b, err = post(ctx, q, u, a, entry{
			typ: TypeServiceFinish, remaining: -b.CreditsRemaining, ref: &ref,
			key: itemKey(ref, "finish"), meta: refMeta(ref),
		}); err != nil {
			return Balance{}, err
		}
	}
	return view(b), nil
}

// RefundItem gives back what the service item took (its use and finish).
// An item that took nothing, or was already refunded, is a no-op.
func RefundItem(ctx context.Context, q *db.Queries, u db.Unit, a Actor, ref Ref, reason string) (Balance, bool, error) {
	// TEC-1242: a cut piece with line credits refunds too; a roll's items
	// took nothing, so the sum below answers the no-op.
	if !mayCarry(u) {
		return Balance{}, false, nil
	}
	sum, err := q.SumUnitCreditsByItem(ctx, pgtype.Int8{Int64: ref.ServiceItemID, Valid: true})
	if err != nil {
		return Balance{}, false, fmt.Errorf("credits: item sum: %w", err)
	}
	if sum.RemainingDelta >= 0 && sum.UsesDelta <= 0 {
		bal, _, err := Get(ctx, q, u)
		return bal, false, err
	}
	if _, err := open(ctx, q, u, a); err != nil {
		return Balance{}, false, err
	}
	meta := refMeta(ref)
	b, err := post(ctx, q, u, a, entry{
		typ: TypeServiceRefund, remaining: -sum.RemainingDelta, use: -sum.UsesDelta, ref: &ref,
		reason: reason, key: itemKey(ref, "refund"), meta: meta,
	})
	if err != nil {
		return Balance{}, false, err
	}
	return view(b), true, nil
}

// TookCredits reports whether the item holds credits (it was selected
// through the credit rule and not refunded).
func TookCredits(ctx context.Context, q *db.Queries, itemID int64) (bool, error) {
	sum, err := q.SumUnitCreditsByItem(ctx, pgtype.Int8{Int64: itemID, Valid: true})
	if err != nil {
		return false, fmt.Errorf("credits: item sum: %w", err)
	}
	return sum.UsesDelta > 0, nil
}

// Increase adds amount credits to the unit's total and remaining (super
// admin; the reason is required by the caller).
func Increase(ctx context.Context, q *db.Queries, u db.Unit, a Actor, amount int32, reason string) (Balance, error) {
	if amount <= 0 || amount > MaxCredits {
		return Balance{}, ErrInvalidAmount
	}
	if _, err := open(ctx, q, u, a); err != nil {
		return Balance{}, err
	}
	b, err := post(ctx, q, u, a, entry{
		typ: TypeIncrease, total: amount, remaining: amount, reason: reason,
		key: "credit:unit:" + strconv.FormatInt(u.ID, 10) + ":increase:" + uuid.NewString(),
	})
	if err != nil {
		return Balance{}, err
	}
	return view(b), nil
}

// Remaining reads the locked balance's remaining credits (opened or not).
func Remaining(ctx context.Context, q *db.Queries, u db.Unit) (int32, error) {
	b, ok, err := Get(ctx, q, u)
	if err != nil || !ok {
		return 0, err
	}
	return b.Remaining, nil
}

func view(b db.UnitCreditBalance) Balance {
	return Balance{Total: b.CreditsTotal, Remaining: b.CreditsRemaining, Used: b.UsedCount, Opened: true}
}
