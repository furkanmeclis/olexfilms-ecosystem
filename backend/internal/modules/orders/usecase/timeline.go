package usecase

// TEC-1060 (RV-5.4.1): who did what on an order and when. The status steps
// come from order_status_history (actor_user_id); unit assignments,
// unassignments and draft edits are written to activity_events (resource
// "order", resource_uuid = order uuid) in the same transaction as the
// change. Assignments made before those events are read from
// order_item_units (assigned_by_user_id, assigned_at).

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Order activity actions (activity_events.action) and their timeline names
// (the action without the "order." prefix).
const (
	ActivityResource      = "order"
	ActionUnitAssigned    = "order.unit_assigned"
	ActionUnitUnassigned  = "order.unit_unassigned"
	ActionUpdated         = "order.updated"
	activityActionPrefix  = "order."
	timelineUnitAssigned  = "unit_assigned"
	maxTimelineDetailText = 200
)

// TimelineEntry is one step of an order: "created", a status the order
// moved to, "unit_assigned", "unit_unassigned" or "updated". The unit
// fields are set on the unit steps (Quantity or Meters is the amount of
// that step), Reason on a status step that carries one, Lines on an edit.
type TimelineEntry struct {
	Action     string    `json:"action"`
	At         time.Time `json:"at"`
	User       *UserRef  `json:"user"`
	FromStatus *string   `json:"from_status,omitempty"`
	Reason     *string   `json:"reason,omitempty"`
	Product    *string   `json:"product,omitempty"`
	Barcode    *string   `json:"barcode,omitempty"`
	Quantity   *int64    `json:"quantity,omitempty"`
	Meters     *string   `json:"meters,omitempty"`
	Lines      *int64    `json:"lines,omitempty"`
	// Changes: the lines an edit added, changed or removed (TEC-1056).
	Changes []LineChange `json:"changes,omitempty"`
}

// activityPayload is the payload of the order activity events.
type activityPayload struct {
	ItemUUID string  `json:"item_uuid,omitempty"`
	Product  string  `json:"product,omitempty"`
	UnitUUID string  `json:"unit_uuid,omitempty"`
	Barcode  string  `json:"barcode,omitempty"`
	Quantity *int64  `json:"quantity,omitempty"`
	Meters   *string `json:"meters,omitempty"`
	Lines    *int64  `json:"lines,omitempty"`
	// Changes of an edit (TEC-1056): before/after amount by product.
	Changes []LineChange `json:"changes,omitempty"`
}

func (p activityPayload) mapped() map[string]any {
	raw, _ := json.Marshal(p)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func (s *Service) recordActivity(ctx context.Context, q *db.Queries, c Caller, o db.Order, action string, p activityPayload) error {
	var actor *int64
	if c.Principal.UserInternal != 0 {
		id := c.Principal.UserInternal
		actor = &id
	}
	id := o.Uuid
	if err := activity.Write(ctx, q, actor, action, ActivityResource, &id, p.mapped(), activity.Meta{}); err != nil {
		return fmt.Errorf("orders: activity: %w", err)
	}
	return nil
}

// lineUnits snapshots the assignments of a line by unit.
func lineUnits(ctx context.Context, q *db.Queries, o db.Order, it db.OrderItem) (map[int64]db.ListOrderItemUnitsByOrderRow, error) {
	rows, err := q.ListOrderItemUnitsByOrder(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("orders: assigned units: %w", err)
	}
	out := map[int64]db.ListOrderItemUnitsByOrderRow{}
	for _, r := range rows {
		if r.OrderItemID == it.ID {
			out[r.UnitID] = r
		}
	}
	return out, nil
}

// recordAssigned writes one unit_assigned event per unit that is new on the
// line or grew since before (the amount is the growth).
func (s *Service) recordAssigned(ctx context.Context, q *db.Queries, c Caller, o db.Order, it db.OrderItem,
	before map[int64]db.ListOrderItemUnitsByOrderRow) error {
	after, err := lineUnits(ctx, q, o, it)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(after))
	for id := range after {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return after[ids[i]].ID < after[ids[j]].ID })
	var product string
	for _, id := range ids {
		a := after[id]
		p := activityPayload{ItemUUID: it.Uuid.String(), UnitUUID: a.UnitUuid.String(), Barcode: a.Barcode}
		prev, had := before[id]
		switch {
		case a.Quantity.Valid:
			n := int64(a.Quantity.Int32)
			if had && prev.Quantity.Valid {
				n -= int64(prev.Quantity.Int32)
			}
			if n <= 0 {
				continue
			}
			p.Quantity = &n
		default:
			m := numericRat(a.Meters)
			if m == nil {
				continue
			}
			if had {
				if pm := numericRat(prev.Meters); pm != nil {
					m = new(big.Rat).Sub(m, pm)
				}
			}
			if m.Sign() <= 0 {
				continue
			}
			v := m.FloatString(2)
			p.Meters = &v
		}
		if product == "" {
			if pr, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: o.BrandID}); err == nil {
				product = pr.Name
			}
		}
		p.Product = product
		if err := s.recordActivity(ctx, q, c, o, ActionUnitAssigned, p); err != nil {
			return err
		}
	}
	return nil
}

// recordUnassigned writes the unit_unassigned event of a removed assignment.
func (s *Service) recordUnassigned(ctx context.Context, q *db.Queries, c Caller, o db.Order, it db.OrderItem,
	u db.Unit, a db.OrderItemUnit) error {
	p := activityPayload{ItemUUID: it.Uuid.String(), UnitUUID: u.Uuid.String(), Barcode: u.Barcode}
	if a.Quantity.Valid {
		n := int64(a.Quantity.Int32)
		p.Quantity = &n
	} else {
		p.Meters = numericTextPtr(a.Meters, 2)
	}
	if pr, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: o.BrandID}); err == nil {
		p.Product = pr.Name
	}
	return s.recordActivity(ctx, q, c, o, ActionUnitUnassigned, p)
}

// recordUpdated writes the updated event of a line edit with the line
// count after it and the before/after amounts of the changed lines.
func (s *Service) recordUpdated(ctx context.Context, q *db.Queries, c Caller, o db.Order, lines int, changes []LineChange) error {
	n := int64(lines)
	return s.recordActivity(ctx, q, c, o, ActionUpdated, activityPayload{Lines: &n, Changes: changes})
}

func userRef(id pgtype.UUID, name, surname pgtype.Text) *UserRef {
	if !id.Valid {
		return nil
	}
	return &UserRef{UUID: uuid.UUID(id.Bytes), Name: fullName(name.String, surname.String)}
}

func strRef(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if len(s) > maxTimelineDetailText {
		s = s[:maxTimelineDetailText]
	}
	return &s
}

// timeline merges the status history, the order activity events and the
// assignments older than those events, oldest first.
func (s *Service) timeline(ctx context.Context, q *db.Queries, o db.Order, hist []db.OrderStatusHistory,
	assigned []db.ListOrderItemUnitsByOrderRow, products map[int64]string) ([]TimelineEntry, error) {
	userIDs := map[int64]struct{}{}
	for _, h := range hist {
		if h.ActorUserID.Valid {
			userIDs[h.ActorUserID.Int64] = struct{}{}
		}
	}
	for _, a := range assigned {
		if a.AssignedByUserID.Valid {
			userIDs[a.AssignedByUserID.Int64] = struct{}{}
		}
	}
	users := map[int64]*UserRef{}
	if len(userIDs) > 0 {
		ids := make([]int64, 0, len(userIDs))
		for id := range userIDs {
			ids = append(ids, id)
		}
		rows, err := q.ListUsersByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("orders: timeline users: %w", err)
		}
		for _, u := range rows {
			users[u.ID] = &UserRef{UUID: u.Uuid, Name: fullName(u.Name, u.Surname)}
		}
	}
	userOf := func(id pgtype.Int8) *UserRef {
		if !id.Valid {
			return nil
		}
		return users[id.Int64]
	}

	type keyed struct {
		e   TimelineEntry
		src int
		seq int64
	}
	var all []keyed
	for _, h := range hist {
		e := TimelineEntry{Action: h.ToStatus, At: h.CreatedAt.Time, User: userOf(h.ActorUserID), Reason: textPtr(h.Reason)}
		if !h.FromStatus.Valid {
			e.Action = ActivityCreated
		} else {
			e.FromStatus = textPtr(h.FromStatus)
		}
		all = append(all, keyed{e, 1, h.ID})
	}
	events, err := q.ListResourceActivityEvents(ctx, db.ListResourceActivityEventsParams{
		Resource: ActivityResource, ResourceUuid: o.Uuid,
	})
	if err != nil {
		return nil, fmt.Errorf("orders: timeline events: %w", err)
	}
	recorded := map[string]struct{}{}
	for _, ev := range events {
		var p activityPayload
		_ = json.Unmarshal(ev.Payload, &p)
		e := TimelineEntry{
			Action: strings.TrimPrefix(ev.Action, activityActionPrefix), At: ev.CreatedAt.Time,
			User:    userRef(ev.ActorUuid, ev.ActorName, ev.ActorSurname),
			Product: strRef(p.Product), Barcode: strRef(p.Barcode), Quantity: p.Quantity, Meters: p.Meters, Lines: p.Lines,
			Changes: p.Changes,
		}
		if ev.Action == ActionUnitAssigned {
			recorded[p.ItemUUID+"/"+p.UnitUUID] = struct{}{}
		}
		all = append(all, keyed{e, 2, ev.ID})
	}
	// Assignments written before the activity events (TEC-1060).
	itemUUIDs := map[int64]string{}
	if len(assigned) > 0 {
		items, err := q.ListOrderItems(ctx, o.ID)
		if err != nil {
			return nil, fmt.Errorf("orders: lines: %w", err)
		}
		for _, it := range items {
			itemUUIDs[it.ID] = it.Uuid.String()
		}
	}
	for _, a := range assigned {
		if _, ok := recorded[itemUUIDs[a.OrderItemID]+"/"+a.UnitUuid.String()]; ok {
			continue
		}
		e := TimelineEntry{
			Action: timelineUnitAssigned, At: a.AssignedAt.Time, User: userOf(a.AssignedByUserID),
			Barcode: strRef(a.Barcode), Meters: numericTextPtr(a.Meters, 2),
		}
		if name, ok := products[a.OrderItemID]; ok {
			e.Product = strRef(name)
		}
		if a.Quantity.Valid {
			n := int64(a.Quantity.Int32)
			e.Quantity = &n
		}
		all = append(all, keyed{e, 3, a.ID})
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].e.At.Equal(all[j].e.At) {
			return all[i].e.At.Before(all[j].e.At)
		}
		if all[i].src != all[j].src {
			return all[i].src < all[j].src
		}
		return all[i].seq < all[j].seq
	})
	out := make([]TimelineEntry, 0, len(all))
	for _, k := range all {
		out = append(out, k.e)
	}
	return out, nil
}
