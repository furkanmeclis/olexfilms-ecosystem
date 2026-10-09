package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TEC-308 (F3-08d): the daily subscription job. It activates scheduled
// subscriptions on starts_on, writes every due period of a running
// subscription to service_subscription_periods and books it on both ledgers
// through accounting/posting, expires subscriptions past ends_on (closing
// their module bundles) and sends the 30 / 7 day expiry reminders. An
// approved early cancellation books its frozen fee on both ledgers from the
// service_subscription.cancelled event. Every step is idempotent.
//
// Ledger rows (K9 bridge, one row per side, see accounting/posting):
//   - seller (the brand's center): income on the receiver's cari;
//   - receiver (distributor or dealer): expense on the center's cari.
//
// The amount is the subscription's frozen price in its currency; each side
// converts it to its own currency at the subscription's rate_snapshot (K7).
// The receiver cannot delete the rows; it disputes them (K24).
const (
	// SourceSubscriptionPeriod keys a posted period (period uuid).
	SourceSubscriptionPeriod = "service_subscription_period"
	// SourceSubscriptionCancel keys a cancellation fee (cancel request uuid).
	SourceSubscriptionCancel = "service_subscription_cancel"
)

// Expiry reminder thresholds in days before ends_on, largest first.
var ReminderDays = []int{30, 7}

// Poster is the part of accounting/posting the job uses.
type Poster interface {
	PostIncome(ctx context.Context, tx pgx.Tx, e posting.Entry) (posting.Result, error)
	PostExpense(ctx context.Context, tx pgx.Tx, e posting.Entry) (posting.Result, error)
}

// WithAccounting wires the ledger writer of periods and cancellation fees.
// Without it the job still activates, expires and reminds but posts nothing.
func (s *Service) WithAccounting(p Poster) *Service {
	s.poster = p
	return s
}

// WithClock replaces time.Now (tests).
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// jobLocation is the zone of the business day (queue.SchedulerTimezone).
var jobLocation = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// today is the current business day as a UTC midnight date.
func (s *Service) today() time.Time {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	return dateArg(now().In(jobLocation)).Time
}

// DailyResult counts what one run of the daily job did.
type DailyResult struct {
	Activated int
	Posted    int
	Expired   int
	Reminded  int
}

// RunDaily is the service_subscriptions:post_periods task: activate, post,
// expire, remind, in this order, for the current business day.
func (s *Service) RunDaily(ctx context.Context) (DailyResult, error) {
	return s.RunDailyOn(ctx, s.today())
}

// RunDailyOn runs the daily job for the given day. Every step runs even
// when an earlier one failed; the errors are joined.
func (s *Service) RunDailyOn(ctx context.Context, today time.Time) (DailyResult, error) {
	var (
		res  DailyResult
		errs []error
		err  error
	)
	if res.Activated, err = s.ActivateDue(ctx, today); err != nil {
		errs = append(errs, err)
	}
	if res.Posted, err = s.PostDuePeriods(ctx, today); err != nil {
		errs = append(errs, err)
	}
	if res.Expired, err = s.ExpireDue(ctx, today); err != nil {
		errs = append(errs, err)
	}
	if res.Reminded, err = s.SendExpiryReminders(ctx, today); err != nil {
		errs = append(errs, err)
	}
	return res, errors.Join(errs...)
}

// ActivateDue starts every scheduled subscription whose starts_on has come:
// status active, module bundle flags granted, cache dropped after commit.
// A module closed upstream keeps the subscription scheduled (retried the
// next day). Returns the activated count.
func (s *Service) ActivateDue(ctx context.Context, today time.Time) (int, error) {
	if s.pool == nil || s.queries == nil {
		return 0, ErrNotConfigured
	}
	due, err := s.queries.ListDueScheduledServiceSubscriptionIDs(ctx, dateArg(today))
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, row := range due {
		var (
			sub  db.ServiceSubscription
			item db.ServiceCatalogItem
		)
		err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
			var err error
			sub, err = q.ActivateScheduledServiceSubscription(ctx, row.ID)
			if errors.Is(err, pgx.ErrNoRows) {
				return errSkip
			}
			if err != nil {
				return err
			}
			item, err = q.GetServiceCatalogItem(ctx, db.GetServiceCatalogItemParams{ID: sub.ItemID, BrandID: sub.BrandID})
			if err != nil {
				return err
			}
			if err := s.openModules(ctx, q, sub, item, 0); err != nil {
				return err
			}
			return s.emit(ctx, tx, events.ServiceSubscriptionActivated, sub, item, 0, nil)
		})
		switch {
		case errors.Is(err, errSkip):
			continue
		case err != nil:
			errs = append(errs, fmt.Errorf("service subscriptions: activate %d: %w", row.ID, err))
			continue
		}
		s.invalidateModules(ctx, sub, item)
		n++
	}
	return n, errors.Join(errs...)
}

// PostDuePeriods writes and books every due, unposted period of every
// running subscription. Returns the number of periods booked now.
func (s *Service) PostDuePeriods(ctx context.Context, today time.Time) (int, error) {
	if s.pool == nil || s.queries == nil {
		return 0, ErrNotConfigured
	}
	if s.poster == nil {
		return 0, nil
	}
	rows, err := s.queries.ListPostableServiceSubscriptionIDs(ctx, dateArg(today))
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, row := range rows {
		posted := 0
		err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
			sub, err := q.LockServiceSubscription(ctx, db.LockServiceSubscriptionParams(row))
			if err != nil {
				return err
			}
			if sub.Status != StatusActive && sub.Status != StatusCancelRequested {
				return nil // cancelled or expired meanwhile
			}
			posted, err = s.postPeriods(ctx, q, tx, sub, today)
			return err
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("service subscriptions: post periods of %d: %w", row.ID, err))
			continue
		}
		n += posted
	}
	return n, errors.Join(errs...)
}

// postPeriods books the due periods of a locked, running subscription in
// tx. A period already posted is skipped (idempotent).
func (s *Service) postPeriods(ctx context.Context, q *db.Queries, tx pgx.Tx, sub db.ServiceSubscription, today time.Time) (int, error) {
	if s.poster == nil {
		return 0, nil
	}
	n := 0
	for _, p := range DuePeriods(sub.Recurrence, sub.StartsOn.Time, sub.EndsOn.Time, today) {
		period, err := q.InsertServiceSubscriptionPeriod(ctx, db.InsertServiceSubscriptionPeriodParams{
			SubscriptionID: sub.ID, OrganizationID: sub.OrganizationID, BrandID: sub.BrandID,
			PeriodStart: dateArg(p.Start), PeriodEnd: dateArg(p.End),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			period, err = q.GetServiceSubscriptionPeriodByStart(ctx, db.GetServiceSubscriptionPeriodByStartParams{
				SubscriptionID: sub.ID, PeriodStart: dateArg(p.Start),
			})
		}
		if err != nil {
			return n, err
		}
		if period.PostedAt.Valid {
			continue
		}
		desc := fmt.Sprintf("%s %s / %s", SourceSubscriptionPeriod, p.Start.Format(time.DateOnly), p.End.Format(time.DateOnly))
		if err := s.postBothSides(ctx, tx, sub, posting.Source{Type: SourceSubscriptionPeriod, UUID: period.Uuid},
			numText(sub.Price), sub.Currency, accounting.CategoryServiceSale, accounting.CategoryServicePurchase, desc, nil); err != nil {
			return n, err
		}
		if _, err := q.MarkServiceSubscriptionPeriodPosted(ctx, period.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// postBothSides books amount on the seller's (income) and the receiver's
// (expense) ledger, each on the other's cari. A zero amount writes nothing.
func (s *Service) postBothSides(ctx context.Context, tx pgx.Tx, sub db.ServiceSubscription, src posting.Source,
	amount, currency, sellerCategory, buyerCategory, desc string, actor *int64) error {
	if v, ok := new(big.Rat).SetString(amount); !ok || v.Sign() <= 0 {
		return nil
	}
	rate, day := frozenRate(sub)
	base := posting.Entry{
		Source: src, Amount: amount, Currency: currency, RateDate: day, Rate: rate,
		Description: desc, ActorUserID: actor,
	}
	seller := base
	seller.OrganizationID, seller.CounterpartyOrgID = sub.SellerOrgID, sub.OrganizationID
	seller.Role, seller.Category = posting.RoleSale, sellerCategory
	if _, err := s.poster.PostIncome(ctx, tx, seller); err != nil {
		return fmt.Errorf("seller side: %w", err)
	}
	buyer := base
	buyer.OrganizationID, buyer.CounterpartyOrgID = sub.OrganizationID, sub.SellerOrgID
	buyer.Role, buyer.Category = posting.RolePurchase, buyerCategory
	if _, err := s.poster.PostExpense(ctx, tx, buyer); err != nil {
		return fmt.Errorf("buyer side: %w", err)
	}
	return nil
}

// frozenRate returns the subscription's rate snapshot and its day (the
// start day when the snapshot carries none).
func frozenRate(sub db.ServiceSubscription) (*fxrates.Snapshot, time.Time) {
	day := sub.StartsOn.Time
	var snap fxrates.Snapshot
	if err := json.Unmarshal(sub.RateSnapshot, &snap); err != nil || snap.Base == "" {
		return nil, day
	}
	if d, err := time.Parse(time.DateOnly, snap.RateDate); err == nil {
		day = d
	}
	return &snap, day
}

// Period is one accounting period of a subscription (both days inclusive).
type Period struct {
	Start time.Time
	End   time.Time
}

// DuePeriods lists the periods of a subscription that start on or before
// today. one_time is a single period over the whole term; monthly and
// yearly step from starts_on (a day missing in a short month is clamped to
// its last day). ends_on is the last served day: a period starting on it is
// not written and the last period is cut at ends_on (full price, no
// proration).
func DuePeriods(recurrence string, startsOn, endsOn, today time.Time) []Period {
	start, end, today := dateArg(startsOn).Time, dateArg(endsOn).Time, dateArg(today).Time
	if start.After(today) {
		return nil
	}
	if recurrence == "one_time" {
		return []Period{{Start: start, End: end}}
	}
	step := 1
	if recurrence == "yearly" {
		step = 12
	}
	var out []Period
	for k := 0; ; k++ {
		ps := addMonths(start, k*step)
		if !ps.Before(end) && k > 0 || ps.After(today) {
			break
		}
		pe := addMonths(start, (k+1)*step).AddDate(0, 0, -1)
		if pe.After(end) {
			pe = end
		}
		out = append(out, Period{Start: ps, End: pe})
		if !pe.Before(end) {
			break
		}
	}
	return out
}

// addMonths adds n months to d keeping its day, clamped to the month end.
func addMonths(d time.Time, n int) time.Time {
	y, m, day := d.Date()
	first := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC)
}

// ExpireDue expires every open subscription whose ends_on is before today:
// its remaining due periods are booked first, then the status turns
// expired, the module bundle closes (unless another running subscription
// still covers a module) and the receiver is notified; the feature cache is
// dropped after commit. Run hourly and by the daily job; returns the
// expired count.
func (s *Service) ExpireDue(ctx context.Context, today time.Time) (int, error) {
	if s.pool == nil || s.queries == nil {
		return 0, ErrNotConfigured
	}
	due, err := s.queries.ListExpiringServiceSubscriptionIDs(ctx, dateArg(today))
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, row := range due {
		var (
			sub  db.ServiceSubscription
			item db.ServiceCatalogItem
		)
		err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
			cur, err := q.LockServiceSubscription(ctx, db.LockServiceSubscriptionParams(row))
			if err != nil {
				return err
			}
			started := cur.Status == StatusActive || cur.Status == StatusCancelRequested
			if started {
				if _, err := s.postPeriods(ctx, q, tx, cur, today); err != nil {
					return err
				}
			}
			sub, err = q.ExpireServiceSubscription(ctx, cur.ID)
			if errors.Is(err, pgx.ErrNoRows) {
				return errSkip
			}
			if err != nil {
				return err
			}
			item, err = q.GetServiceCatalogItem(ctx, db.GetServiceCatalogItemParams{ID: sub.ItemID, BrandID: sub.BrandID})
			if err != nil {
				return err
			}
			if started {
				if err := s.closeModules(ctx, q, sub, item); err != nil {
					return err
				}
			}
			return s.emit(ctx, tx, events.ServiceSubscriptionExpired, sub, item, 0, nil)
		})
		switch {
		case errors.Is(err, errSkip):
			continue
		case err != nil:
			errs = append(errs, fmt.Errorf("service subscriptions: expire %d: %w", row.ID, err))
			continue
		}
		s.invalidateModules(ctx, sub, item)
		n++
	}
	return n, errors.Join(errs...)
}

// SendExpiryReminders sends the expiry reminder of every running
// subscription ending within the largest threshold: the smallest threshold
// that covers the days left (7 when 7 or fewer days remain, else 30). A
// reminder is recorded in service_subscription_reminders and goes out once.
func (s *Service) SendExpiryReminders(ctx context.Context, today time.Time) (int, error) {
	if s.pool == nil || s.queries == nil {
		return 0, ErrNotConfigured
	}
	today = dateArg(today).Time
	subs, err := s.queries.ListServiceSubscriptionsEndingBetween(ctx, db.ListServiceSubscriptionsEndingBetweenParams{
		FromDay: dateArg(today), ToDay: dateArg(today.AddDate(0, 0, ReminderDays[0])),
	})
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, sub := range subs {
		left := int(sub.EndsOn.Time.Sub(today).Hours() / 24)
		days := ReminderDays[0]
		for _, d := range ReminderDays {
			if left <= d {
				days = d
			}
		}
		sent := false
		err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
			_, err := q.InsertServiceSubscriptionReminder(ctx, db.InsertServiceSubscriptionReminderParams{
				SubscriptionID: sub.ID, OrganizationID: sub.OrganizationID, BrandID: sub.BrandID, DaysBefore: int16(days),
			})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // already sent
			}
			if err != nil {
				return err
			}
			item, err := q.GetServiceCatalogItem(ctx, db.GetServiceCatalogItemParams{ID: sub.ItemID, BrandID: sub.BrandID})
			if err != nil {
				return err
			}
			sent = true
			return s.emit(ctx, tx, events.ServiceSubscriptionExpiring, sub, item, 0, map[string]any{
				"days_before": strconv.Itoa(days), "days_left": strconv.Itoa(left),
			})
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("service subscriptions: remind %d: %w", sub.ID, err))
			continue
		}
		if sent {
			n++
		}
	}
	return n, errors.Join(errs...)
}

// PostCancellationFee books the frozen fee of an approved cancel request on
// both ledgers (category service_cancellation_fee, source
// service_subscription_cancel + request uuid). Nothing is written for a
// zero fee or a request that is not approved; a repeated call writes
// nothing. Returns whether rows were (or had been) booked.
func (s *Service) PostCancellationFee(ctx context.Context, brandID int64, requestUUID uuid.UUID) (bool, error) {
	if s.pool == nil || s.queries == nil {
		return false, ErrNotConfigured
	}
	if s.poster == nil {
		return false, nil
	}
	booked := false
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		req, err := q.GetServiceSubscriptionCancelRequestByUUID(ctx, db.GetServiceSubscriptionCancelRequestByUUIDParams{Uuid: requestUUID, BrandID: brandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		fee := numText(req.CancellationFee)
		if v, ok := new(big.Rat).SetString(fee); req.Status != StatusApproved || !ok || v.Sign() <= 0 {
			return nil
		}
		sub, err := q.LockServiceSubscription(ctx, db.LockServiceSubscriptionParams{ID: req.SubscriptionID, BrandID: brandID})
		if err != nil {
			return err
		}
		var actor *int64
		if req.DecidedByUserID.Valid {
			actor = &req.DecidedByUserID.Int64
		}
		if err := s.postBothSides(ctx, tx, sub, posting.Source{Type: SourceSubscriptionCancel, UUID: req.Uuid},
			fee, req.Currency, accounting.CategoryServiceCancellationFee, accounting.CategoryServiceCancellationFee,
			SourceSubscriptionCancel, actor); err != nil {
			return err
		}
		booked = true
		return nil
	})
	return booked, err
}

// RegisterAccountingHandlers books the cancellation fee when the outbox
// publishes service_subscription.cancelled (worker).
func RegisterAccountingHandlers(bus events.Bus, s *Service, log *slog.Logger) {
	if bus == nil || s == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	bus.Subscribe(events.ServiceSubscriptionCancelled, func(ctx context.Context, ev events.Event) error {
		err := s.HandleSubscriptionCancelled(ctx, ev)
		if err != nil {
			log.Error("service_subscription_cancel_fee_failed", "event_id", ev.EventID, "error", err)
		}
		return err
	})
}

// HandleSubscriptionCancelled is the bus handler of
// service_subscription.cancelled.
func (s *Service) HandleSubscriptionCancelled(ctx context.Context, ev events.Event) error {
	raw, _ := ev.Payload["cancel_request_uuid"].(string)
	reqUUID, err := uuid.Parse(raw)
	if err != nil {
		return nil // emitted before TEC-308: no request reference
	}
	brandID, ok := payloadInt64(ev.Payload["brand_id"])
	if !ok {
		return nil
	}
	_, err = s.PostCancellationFee(ctx, brandID, reqUUID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func payloadInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

// errSkip ends a per-row transaction without error (row already handled).
var errSkip = errors.New("service subscriptions: skip")
