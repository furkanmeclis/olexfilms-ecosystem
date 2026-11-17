package usecase

// TEC-1228: the warehouse work queue. An order carries a priority (normal /
// urgent) and a due date; the warehouse lists sort urgent first, then by
// due date (sort=queue) and flag the overdue ones. A warehouse user takes
// a job ("işi üstlen"); someone else sees who holds it, and a seller
// manager (orders.write or orders.approve) takes it over. Shipping records
// the package count, desi and weight, printed on the shipping note.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Order priorities.
const (
	PriorityNormal = "normal"
	PriorityUrgent = "urgent"
)

// Priorities lists the order priorities.
var Priorities = []string{PriorityNormal, PriorityUrgent}

// Queue activity actions (timeline: the action without "order.").
const (
	ActionScheduleUpdated = "order.schedule_updated"
	ActionClaimed         = "order.claimed"
	ActionClaimTakenOver  = "order.claim_taken_over"
	ActionClaimReleased   = "order.claim_released"
)

// Shipment detail bounds.
const (
	MaxPackageCount = 9999
	MaxDesi         = 99999999.99
)

var (
	// ErrNotClaimable: only an approved order not shipped yet is a job.
	ErrNotClaimable = errors.New("orders: the order is not in the warehouse queue")
	// ErrScheduleLocked: the priority and due date change only before
	// shipping (seller) or in a draft / submitted order (buyer).
	ErrScheduleLocked = errors.New("orders: the order schedule cannot change in this status")
	// ErrAlreadyClaimed: another user holds the job (see ClaimedError).
	ErrAlreadyClaimed = errors.New("orders: another user has taken the job")
)

// ClaimedError is ErrAlreadyClaimed with the user holding the job.
type ClaimedError struct {
	Picker    UserRef
	ClaimedAt time.Time
}

func (e *ClaimedError) Error() string { return ErrAlreadyClaimed.Error() + ": " + e.Picker.Name }

// Is makes errors.Is(err, ErrAlreadyClaimed) hold.
func (e *ClaimedError) Is(target error) bool { return target == ErrAlreadyClaimed }

// queueStatuses are the statuses of a warehouse job.
var queueStatuses = []string{StatusApproved, StatusProcessing, StatusPreparing, StatusReady}

// closedForSchedule: after shipping the priority and due date are history.
var closedForSchedule = []string{StatusShipped, StatusDelivered, StatusReceived, StatusCancelling, StatusCancelled}

// ScheduleInput is the priority and the due date (YYYY-MM-DD, nil or ""
// clears it) of an order.
type ScheduleInput struct {
	Priority string
	DueDate  *string
}

type schedule struct {
	priority string
	due      pgtype.Date
}

func (in ScheduleInput) parse() (schedule, error) {
	out := schedule{priority: strings.TrimSpace(in.Priority)}
	if out.priority == "" {
		out.priority = PriorityNormal
	}
	if !slices.Contains(Priorities, out.priority) {
		return out, invalid("priority", "must be normal or urgent")
	}
	if in.DueDate != nil && strings.TrimSpace(*in.DueDate) != "" {
		d, err := time.Parse(time.DateOnly, strings.TrimSpace(*in.DueDate))
		if err != nil {
			return out, invalid("due_date", "must be a date (YYYY-MM-DD)")
		}
		if d.Year() < 2000 || d.Year() > 2100 {
			return out, invalid("due_date", "is out of range")
		}
		out.due = pgtype.Date{Time: d, Valid: true}
	}
	return out, nil
}

func (s schedule) isDefault() bool { return s.priority == PriorityNormal && !s.due.Valid }

// applySchedule stores the schedule a new order was created with.
func applySchedule(ctx context.Context, q *db.Queries, o db.Order, sc schedule) (db.Order, error) {
	if sc.isDefault() {
		return o, nil
	}
	o, err := q.SetOrderSchedule(ctx, db.SetOrderScheduleParams{ID: o.ID, Priority: sc.priority, DueDate: sc.due})
	if err != nil {
		return o, fmt.Errorf("orders: schedule: %w", err)
	}
	return o, nil
}

// canEditSchedule: the buyer (orders.write) while the order is a draft or
// submitted; the seller (orders.write or orders.approve) until it ships.
func canEditSchedule(c Caller, o db.Order) bool {
	switch partyOf(c, o) {
	case PartyBuyer:
		return c.can(rbac.PermOrdersWrite) && (o.Status == StatusDraft || o.Status == StatusSubmitted)
	case PartySeller:
		return (c.can(rbac.PermOrdersWrite) || c.can(rbac.PermOrdersApprove)) && !slices.Contains(closedForSchedule, o.Status)
	}
	return false
}

// canTakeOver: a seller manager may take over or release someone else's job.
func canTakeOver(c Caller) bool {
	return c.can(rbac.PermOrdersWrite) || c.can(rbac.PermOrdersApprove)
}

// canClaim: a seller warehouse user (orders.ship) or manager on an open job.
func canClaim(c Caller, o db.Order) bool {
	return partyOf(c, o) == PartySeller && (c.can(rbac.PermOrdersShip) || canTakeOver(c)) &&
		slices.Contains(queueStatuses, o.Status)
}

func dateText(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	v := d.Time.Format(time.DateOnly)
	return &v
}

// overdue: the due date passed (in the seller's time zone) and the order
// has not shipped.
func overdue(o db.Order, now time.Time, zone *time.Location) bool {
	if !o.DueDate.Valid || slices.Contains(closedForSchedule, o.Status) || o.Status == StatusDraft {
		return false
	}
	today := now.In(zone).Format(time.DateOnly)
	return o.DueDate.Time.Format(time.DateOnly) < today
}

// queueFields fills the queue and shipment fields of a view.
func (s *Service) queueFields(c Caller, o db.Order, zone string, v *OrderView) {
	v.Priority = o.Priority
	v.DueDate = dateText(o.DueDate)
	v.IsOverdue = overdue(o, s.now(), pdfrender.Zone(zone))
	v.ClaimedAt = tsPtr(o.ClaimedAt)
	v.ClaimedByMe = o.PickerUserID.Valid && c.Principal.UserInternal != 0 && o.PickerUserID.Int64 == c.Principal.UserInternal
	v.CanClaim = canClaim(c, o)
	v.CanTakeOver = v.CanClaim && canTakeOver(c)
	v.CanEditSchedule = canEditSchedule(c, o)
	v.PackageCount = int4Ptr(o.PackageCount)
	v.Desi = numericTextPtr(o.Desi, 2)
	v.WeightKg = numericTextPtr(o.WeightKg, 2)
}

// SetSchedule changes the priority and the due date of an order.
func (s *Service) SetSchedule(ctx context.Context, c Caller, orderUUID uuid.UUID, in ScheduleInput) (OrderView, error) {
	sc, err := in.parse()
	if err != nil {
		return OrderView{}, err
	}
	var result db.Order
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		party := partyOf(c, o)
		if party == "" || (party == PartyBuyer && !c.can(rbac.PermOrdersWrite)) ||
			(party == PartySeller && !canTakeOver(c)) {
			return ErrForbidden
		}
		if !canEditSchedule(c, o) {
			return ErrScheduleLocked
		}
		result = o
		if o.Priority == sc.priority && o.DueDate == sc.due {
			return nil
		}
		if result, err = q.SetOrderSchedule(ctx, db.SetOrderScheduleParams{ID: o.ID, Priority: sc.priority, DueDate: sc.due}); err != nil {
			return fmt.Errorf("orders: schedule: %w", err)
		}
		pr := sc.priority
		p := activityPayload{Priority: &pr, DueDate: dateText(sc.due)}
		return s.recordActivity(ctx, q, c, result, ActionScheduleUpdated, p)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, result)
}

// Claim lets the caller take the warehouse job. A job held by someone else
// fails with *ClaimedError unless takeover is set and the caller is a
// seller manager (canTakeOver). Claiming one's own job is a no-op.
func (s *Service) Claim(ctx context.Context, c Caller, orderUUID uuid.UUID, takeover bool) (OrderView, error) {
	if c.Principal.UserInternal == 0 {
		return OrderView{}, ErrForbidden
	}
	var result db.Order
	err := s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		if partyOf(c, o) != PartySeller || (!c.can(rbac.PermOrdersShip) && !canTakeOver(c)) {
			return ErrForbidden
		}
		if !slices.Contains(queueStatuses, o.Status) {
			return ErrNotClaimable
		}
		result = o
		me := c.Principal.UserInternal
		if o.PickerUserID.Valid && o.PickerUserID.Int64 == me {
			return nil
		}
		action, p := ActionClaimed, activityPayload{}
		if o.PickerUserID.Valid {
			holder, err := pickerRef(ctx, q, o.PickerUserID.Int64)
			if err != nil {
				return err
			}
			if !takeover {
				return &ClaimedError{Picker: holder, ClaimedAt: o.ClaimedAt.Time}
			}
			if !canTakeOver(c) {
				return ErrForbidden
			}
			name := holder.Name
			action, p.PreviousPicker = ActionClaimTakenOver, &name
		}
		if result, err = q.SetOrderPicker(ctx, db.SetOrderPickerParams{ID: o.ID, PickerUserID: pgtype.Int8{Int64: me, Valid: true}}); err != nil {
			return fmt.Errorf("orders: claim: %w", err)
		}
		return s.recordActivity(ctx, q, c, result, action, p)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, result)
}

// ReleaseClaim gives the job back: the holder, or a seller manager for
// someone else's job. Releasing an unclaimed job is a no-op.
func (s *Service) ReleaseClaim(ctx context.Context, c Caller, orderUUID uuid.UUID) (OrderView, error) {
	var result db.Order
	err := s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		if partyOf(c, o) != PartySeller || (!c.can(rbac.PermOrdersShip) && !canTakeOver(c)) {
			return ErrForbidden
		}
		result = o
		if !o.PickerUserID.Valid {
			return nil
		}
		p := activityPayload{}
		if o.PickerUserID.Int64 != c.Principal.UserInternal {
			if !canTakeOver(c) {
				return ErrForbidden
			}
			holder, err := pickerRef(ctx, q, o.PickerUserID.Int64)
			if err != nil {
				return err
			}
			name := holder.Name
			p.PreviousPicker = &name
		}
		if result, err = q.SetOrderPicker(ctx, db.SetOrderPickerParams{ID: o.ID}); err != nil {
			return fmt.Errorf("orders: release claim: %w", err)
		}
		return s.recordActivity(ctx, q, c, result, ActionClaimReleased, p)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, result)
}

func pickerRef(ctx context.Context, q *db.Queries, id int64) (UserRef, error) {
	users, err := q.ListUsersByIDs(ctx, []int64{id})
	if err != nil {
		return UserRef{}, fmt.Errorf("orders: picker: %w", err)
	}
	if len(users) == 0 {
		return UserRef{}, nil
	}
	return UserRef{UUID: users[0].Uuid, Name: userName(users[0].Name, users[0].Surname)}, nil
}

// PackagesInput is the shipment detail of a ship transition.
type PackagesInput struct {
	PackageCount *int32
	Desi         *float64
	WeightKg     *float64
}

type packages struct {
	count        pgtype.Int4
	desi, weight pgtype.Numeric
}

func (p packages) any() bool { return p.count.Valid || p.desi.Valid || p.weight.Valid }

func (in PackagesInput) parse() (packages, error) {
	var out packages
	if in.PackageCount != nil {
		if *in.PackageCount < 1 || *in.PackageCount > MaxPackageCount {
			return out, invalid("package_count", fmt.Sprintf("must be between 1 and %d", MaxPackageCount))
		}
		out.count = pgtype.Int4{Int32: *in.PackageCount, Valid: true}
	}
	num := func(field string, v *float64) (pgtype.Numeric, error) {
		if v == nil {
			return pgtype.Numeric{}, nil
		}
		if *v < 0 || *v > MaxDesi {
			return pgtype.Numeric{}, invalid(field, "must be zero or more")
		}
		return numArg(v), nil
	}
	var err error
	if out.desi, err = num("desi", in.Desi); err != nil {
		return out, err
	}
	if out.weight, err = num("weight_kg", in.WeightKg); err != nil {
		return out, err
	}
	return out, nil
}

func setPackages(ctx context.Context, q *db.Queries, o db.Order, p packages) (db.Order, error) {
	if !p.any() {
		return o, nil
	}
	o, err := q.SetOrderPackages(ctx, db.SetOrderPackagesParams{ID: o.ID, PackageCount: p.count, Desi: p.desi, WeightKg: p.weight})
	if err != nil {
		return o, fmt.Errorf("orders: packages: %w", err)
	}
	return o, nil
}

// pickerArgs resolves the picker filter of the list.
func (s *Service) pickerArgs(ctx context.Context, c Caller, f ListFilter, p *db.ListOrdersFilteredParams) error {
	p.Priorities = f.Priorities
	switch f.Picker {
	case "":
	case PickerNone:
		p.PickerUnassigned = pgtype.Bool{Bool: true, Valid: true}
	case PickerMe:
		// A caller without a user (an export job) matches nobody.
		p.PickerUserID = pgtype.Int8{Int64: c.Principal.UserInternal, Valid: true}
	default:
		id, err := uuid.Parse(f.Picker)
		if err != nil {
			return invalid(QueryPicker, "must be me, none or a user uuid")
		}
		u, err := s.q.GetUserByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			p.PickerUserID = pgtype.Int8{Int64: -1, Valid: true}
			return nil
		}
		if err != nil {
			return fmt.Errorf("orders: picker: %w", err)
		}
		p.PickerUserID = pgtype.Int8{Int64: u.ID, Valid: true}
	}
	return nil
}

// createSchedule stores the schedule of a new order (CreateInput).
func createSchedule(ctx context.Context, q *db.Queries, o db.Order, in CreateInput) (db.Order, error) {
	sc, err := ScheduleInput{Priority: in.Priority, DueDate: in.DueDate}.parse()
	if err != nil {
		return o, err
	}
	return applySchedule(ctx, q, o, sc)
}
