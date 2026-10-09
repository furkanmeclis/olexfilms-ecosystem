package usecase

// TEC-506 (F5-09b): the pricing daily tick of worker-core. Hourly, per
// brand, in the brand center's timezone: due recommended versions become
// current (idempotent: a second tick finds nothing due), the day's price
// discipline snapshot is written once (and with the Monday snapshot the
// weekly digest events), old snapshots are purged.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// dueBatch bounds the versions one tick applies per brand.
	dueBatch = 5000
	// snapshotRetentionDays keeps a little more than a year of snapshots.
	snapshotRetentionDays = 400
	// salesWindowDays is the realised average sale price window.
	salesWindowDays = 30
	// digestNames bounds the organization names listed in a digest.
	digestNames = 5
)

// DailyTask is the hourly pricing tick (pricing:daily).
func (s *Recommended) DailyTask(ctx context.Context) error {
	brands, err := s.q.ListBrands(ctx)
	if err != nil {
		return fmt.Errorf("pricing daily: brands: %w", err)
	}
	var errs []error
	for _, b := range brands {
		center, err := s.q.GetBrandCenter(ctx, b.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("pricing daily: brand %d center: %w", b.ID, err))
			continue
		}
		today := LocalDay(s.now(), center.Timezone)
		if _, err := s.ApplyDue(ctx, center.BrandID, today); err != nil {
			errs = append(errs, err)
			continue
		}
		if _, err := s.SnapshotDiscipline(ctx, center, today); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ApplyDue makes the versions whose day has come current (one transaction)
// and enqueues the price lists they change. Returns the applied count.
func (s *Recommended) ApplyDue(ctx context.Context, brandID int64, today time.Time) (int, error) {
	keys := map[priceKey]bool{}
	n := 0
	err := s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		due, err := q.ListDueRecommendedPriceVersionsForBrand(ctx, db.ListDueRecommendedPriceVersionsForBrandParams{
			BrandID: brandID, AsOf: pgDate(today), RowLimit: dueBatch,
		})
		if err != nil {
			return fmt.Errorf("pricing: due versions: %w", err)
		}
		for _, v := range due {
			if err := q.MakeRecommendedPriceCurrent(ctx, v.ID); err != nil {
				return fmt.Errorf("pricing: make current %d: %w", v.ID, err)
			}
			keys[priceKey{countryID: v.CountryID.Int64, currency: strings.TrimSpace(v.Currency)}] = true
			n++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if n > 0 {
		s.log.Info("pricing_recommended_applied", "brand_id", brandID, "count", n, "day", today.Format(time.DateOnly))
		s.enqueuePriceLists(ctx, brandID, sortedKeys(keys), "eff-"+today.Format("20060102"))
	}
	return n, nil
}

// EnqueuePublished enqueues the price lists of a published batch
// (pricing.recommended_published listener).
func (s *Recommended) EnqueuePublished(ctx context.Context, ev events.Event) error {
	brandID, _ := payloadInt(ev.Payload, "brand_id")
	if brandID <= 0 {
		return nil
	}
	token, _ := ev.Payload["batch_id"].(string)
	if token == "" {
		token = ev.EventID.String()
	}
	var keys []priceKey
	switch raw := ev.Payload["keys"].(type) {
	case []any:
		for _, it := range raw {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			country, _ := payloadInt(m, "country_id")
			cur, _ := m["currency"].(string)
			if cur != "" {
				keys = append(keys, priceKey{countryID: country, currency: cur})
			}
		}
	case []map[string]any:
		for _, m := range raw {
			country, _ := payloadInt(m, "country_id")
			cur, _ := m["currency"].(string)
			if cur != "" {
				keys = append(keys, priceKey{countryID: country, currency: cur})
			}
		}
	}
	s.enqueuePriceLists(ctx, brandID, keys, token)
	return nil
}

// enqueuePriceLists enqueues one price list task per key; a currency-wide
// key also refreshes the country lists of that currency (they fall back to
// it). Skipped when the auto publication is off or no queue is wired.
func (s *Recommended) enqueuePriceLists(ctx context.Context, brandID int64, keys []priceKey, token string) {
	if s.queue == nil || len(keys) == 0 {
		return
	}
	if !s.autoPublish(ctx) {
		s.log.Info("pricing_price_list_auto_publish_off", "brand_id", brandID)
		return
	}
	tasks := map[PriceListTask]bool{}
	for _, k := range keys {
		tasks[PriceListTask{BrandID: brandID, CountryID: k.countryID, Currency: k.currency, Token: token}] = true
		if k.countryID != 0 {
			continue
		}
		countries, err := s.q.ListRecommendedPriceCountries(ctx, db.ListRecommendedPriceCountriesParams{BrandID: brandID, Currency: k.currency})
		if err != nil {
			s.log.Error("pricing_price_list_countries_failed", "brand_id", brandID, "error", err)
			continue
		}
		for _, c := range countries {
			tasks[PriceListTask{BrandID: brandID, CountryID: c, Currency: k.currency, Token: token}] = true
		}
	}
	list := make([]PriceListTask, 0, len(tasks))
	for t := range tasks {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Currency != list[j].Currency {
			return list[i].Currency < list[j].Currency
		}
		return list[i].CountryID < list[j].CountryID
	})
	for _, t := range list {
		if err := s.queue.EnqueuePriceList(ctx, t); err != nil {
			s.log.Error("pricing_price_list_enqueue_failed", "brand_id", brandID, "country_id", t.CountryID,
				"currency", t.Currency, "error", err)
		}
	}
}

// SnapshotDiscipline writes the price discipline snapshot of a brand for
// today once (false when it already exists). The Monday snapshot also
// writes the weekly digest events in the same transaction.
func (s *Recommended) SnapshotDiscipline(ctx context.Context, center db.Organization, today time.Time) (bool, error) {
	brandID := center.BrandID
	latest, err := s.q.GetLatestPriceDisciplineSnapshotDate(ctx, brandID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("pricing: latest snapshot: %w", err)
	}
	if latest.Valid && !latest.Time.Before(today) {
		return false, nil
	}
	threshold := s.threshold(ctx)
	err = s.inTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if _, err := q.DeletePriceDisciplineSnapshotsOn(ctx, db.DeletePriceDisciplineSnapshotsOnParams{
			BrandID: brandID, SnapshotDate: pgDate(today),
		}); err != nil {
			return fmt.Errorf("pricing: clear snapshot: %w", err)
		}
		until := s.now()
		if _, err := q.RefreshPriceDisciplineSnapshots(ctx, db.RefreshPriceDisciplineSnapshotsParams{
			SnapshotDate: pgDate(today), BrandID: brandID,
			SalesFrom: pgtype.Timestamptz{Time: until.AddDate(0, 0, -salesWindowDays), Valid: true},
			SalesTo:   pgtype.Timestamptz{Time: until, Valid: true},
		}); err != nil {
			return fmt.Errorf("pricing: snapshot: %w", err)
		}
		if _, err := q.DeletePriceDisciplineSnapshotsBefore(ctx, pgDate(today.AddDate(0, 0, -snapshotRetentionDays))); err != nil {
			return fmt.Errorf("pricing: purge snapshots: %w", err)
		}
		if today.Weekday() == time.Monday {
			return s.digest(ctx, tx, q, center, today, threshold)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// digestGroup is one digest recipient group.
type digestGroup struct {
	orgID int64
	names []string
	count int
}

// digest writes one pricing.discipline_digest event for the center
// (pricing.discipline.read holders, every organization over the threshold)
// and one per distributor (its holders, the distributor and its dealers).
func (s *Recommended) digest(ctx context.Context, tx pgx.Tx, q *db.Queries, center db.Organization, day time.Time, threshold int) error {
	over, err := q.ListPriceDisciplineOverThresholdOrgs(ctx, db.ListPriceDisciplineOverThresholdOrgsParams{
		BrandID: center.BrandID, SnapshotDate: pgDate(day), ThresholdPct: numericInt(threshold),
	})
	if err != nil {
		return fmt.Errorf("pricing: digest: %w", err)
	}
	if len(over) == 0 {
		return nil
	}
	groups := map[int64]*digestGroup{center.ID: {orgID: center.ID}}
	order := []int64{center.ID}
	add := func(orgID int64, name string) {
		g, ok := groups[orgID]
		if !ok {
			g = &digestGroup{orgID: orgID}
			groups[orgID] = g
			order = append(order, orgID)
		}
		g.count++
		if len(g.names) < digestNames {
			g.names = append(g.names, name)
		}
	}
	for _, o := range over {
		add(center.ID, o.OrgName)
		switch {
		case o.OrgType == OrgDistributor:
			add(o.OrganizationID, o.OrgName)
		case o.OrgParentID.Valid && o.OrgParentID.Int64 != center.ID:
			add(o.OrgParentID.Int64, o.OrgName)
		}
	}
	for _, id := range order {
		g := groups[id]
		ids, err := q.ListTransferNotifyUserIDs(ctx, db.ListTransferNotifyUserIDsParams{
			OrganizationID: g.orgID, PermissionSlug: rbac.PermPricingDisciplineRead,
		})
		if err != nil {
			return fmt.Errorf("pricing: digest recipients: %w", err)
		}
		if len(ids) == 0 || s.outbox == nil {
			continue
		}
		names := strings.Join(g.names, ", ")
		if g.count > len(g.names) {
			names += ", …"
		}
		ev := events.New(events.PricingDisciplineDigest).WithTenant(g.orgID).WithPayload(map[string]any{
			"brand_id": center.BrandID, "organization_id": g.orgID, "snapshot_date": day.Format(time.DateOnly),
			"threshold_pct": strconv.Itoa(threshold), "org_count": strconv.Itoa(g.count), "org_names": names,
			"notify_user_ids": ids,
		})
		if err := s.outbox.Enqueue(ctx, tx, ev); err != nil {
			return fmt.Errorf("pricing: digest outbox: %w", err)
		}
	}
	return nil
}

func numericInt(n int) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(int64(n)), Exp: 0, Valid: true}
}

func payloadInt(p map[string]any, key string) (int64, bool) {
	switch v := p[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
