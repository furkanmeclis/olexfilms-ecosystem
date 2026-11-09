package usecase

// TEC-1054 (RV-5): the order list shows the seller warehouse, the ordered
// amount, the total in Turkish lira at today's rate and the user who last
// handled the order. ListOrderFacts reads them for a page in one query.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
)

// UserRef names a user on an order.
type UserRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// LastActivity is the latest recorded step of an order and who took it.
// Action is "created" or the status the order moved to; User is null when
// neither the step nor the order has a user (system or migrated rows).
type LastActivity struct {
	Action string    `json:"action"`
	At     time.Time `json:"at"`
	User   *UserRef  `json:"user"`
}

// ActivityCreated is the LastActivity action of an order's first step.
const ActivityCreated = "created"

// liveRates resolves today's rate from an order currency to TRY once per
// currency of a page. A missing rate leaves the TRY amount empty.
type liveRates struct {
	s     *Service
	day   time.Time
	cache map[string]*string
}

func (s *Service) newLiveRates() *liveRates {
	return &liveRates{s: s, day: s.now().UTC(), cache: map[string]*string{}}
}

func (l *liveRates) rate(ctx context.Context, currency string) *string {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if r, ok := l.cache[cur]; ok {
		return r
	}
	var out *string
	if cur == TryCurrency {
		one := "1"
		out = &one
	} else if l.s.rates != nil {
		snap, err := l.s.rates.ResolveRate(ctx, l.day, cur, TryCurrency)
		if err == nil {
			out = &snap.Rate
		} else {
			l.s.log.DebugContext(ctx, "orders: live rate", "currency", cur, "err", err)
		}
	}
	l.cache[cur] = out
	return out
}

// liveTotal is total × rate rounded to cents; nil without a rate.
func liveTotal(total string, rate *string) *string {
	if rate == nil || total == "" {
		return nil
	}
	t, err := parseRat(total)
	if err != nil {
		return nil
	}
	r, err := parseRat(*rate)
	if err != nil {
		return nil
	}
	v := t.Mul(t, r).FloatString(2)
	return &v
}

// withFacts fills the list facts of views (same order as orders): the
// amounts, the last activity, today's TRY rate and total, and on the
// seller side the seller warehouse.
func (s *Service) withFacts(ctx context.Context, q *db.Queries, c Caller, orders []db.Order, views []OrderView) error {
	if len(orders) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(orders))
	for _, o := range orders {
		ids = append(ids, o.ID)
	}
	rows, err := q.ListOrderFacts(ctx, ids)
	if err != nil {
		return fmt.Errorf("orders: list facts: %w", err)
	}
	byID := make(map[int64]db.ListOrderFactsRow, len(rows))
	for _, r := range rows {
		byID[r.OrderID] = r
	}
	rates := s.newLiveRates()
	for i, o := range orders {
		v := &views[i]
		if r, ok := byID[o.ID]; ok {
			applyFacts(v, r, partyOf(c, o) == PartySeller)
		}
		// TEC-1050: a caller without a price grant has no total (hidePrices
		// already ran), so it gets no live rate either.
		if v.Total == nil {
			continue
		}
		v.LiveTryRate = rates.rate(ctx, o.Currency)
		v.LiveTotalTry = liveTotal(*v.Total, v.LiveTryRate)
	}
	return nil
}

func applyFacts(v *OrderView, r db.ListOrderFactsRow, seller bool) {
	v.TotalQuantity = r.TotalQuantity
	v.TotalMeters = numericTextPtr(r.TotalMeters, 2)
	v.LineCount = r.LineCount
	// The seller warehouse is the seller's own business (TEC-1028).
	if seller && r.WarehouseUuid.Valid && v.SellerWarehouse == nil {
		v.SellerWarehouse = &WarehouseRef{
			UUID: uuid.UUID(r.WarehouseUuid.Bytes), Code: r.WarehouseCode.String,
			Name: r.WarehouseName.String, IsPrimary: r.WarehouseIsPrimary.Bool,
		}
	}
	la := &LastActivity{Action: r.LastAction, At: r.LastAt.Time}
	if r.ActorUuid.Valid {
		la.User = &UserRef{UUID: uuid.UUID(r.ActorUuid.Bytes), Name: fullName(r.ActorName.String, r.ActorSurname.String)}
	}
	v.LastActivity = la
}

func fullName(name, surname string) string {
	return strings.TrimSpace(strings.TrimSpace(name) + " " + strings.TrimSpace(surname))
}
