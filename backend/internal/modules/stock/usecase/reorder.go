package usecase

// TEC-1229: quantity based min / reorder point / max per warehouse and
// product (stock_reorder_rules) and the critical stock list. A rule is
// evaluated on the warehouse balance (available = on hand - reserved -
// quarantine): below min_quantity is below_min, at or under reorder_point
// is reorder, and with warning_days and a forecast average (the latest
// stock_forecasts.avg_daily_30 of the organization) fewer days left than
// warning_days is days_low. The stock forecast day thresholds (warning /
// cover days) stay; a rule carries them per warehouse.
//
// The hourly check (ReorderAlerts) notifies organization owners and
// warehouse roles on the ok -> low edge only. No prices anywhere.
//
// TEC-1243 (K14): a rule has two thresholds that run apart. min_quantity,
// reorder_point and max_quantity count closed boxes (StockMeasures
// .AvailableBoxes; a roll_meter product has no boxes, its rolls count as
// before). min_meters is the meter threshold over the open rolls and cut
// pieces (StockMeasures.AvailableMeters, remnants excluded): MeterLevel is
// below_min under it. Each threshold keeps its own alert state and sends
// its own notification. The forecast days of a product sold by the meter
// use the box equivalent (boxes + meters / box meters), as the stock
// forecast does.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Reorder levels.
const (
	ReorderLevelOK       = "ok"
	ReorderLevelReorder  = "reorder"
	ReorderLevelBelowMin = "below_min"
	ReorderLevelDaysLow  = "days_low"
)

// MaxReorderQuantity bounds the rule quantities.
const MaxReorderQuantity = 1000000

// ErrReorderForbidden: the caller may not manage the organization's rules.
var ErrReorderForbidden = errors.New("stock: reorder rules need the active organization")

// ReorderSort is the sort contract of the rule and critical lists.
var ReorderSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"product": "product", "warehouse": "warehouse", "available": "available", "level": "level",
	},
	Default: apiquery.SortField{Field: "level"},
}

// ReorderWarehouse names a rule's warehouse.
type ReorderWarehouse struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// ReorderRule is one rule with its live evaluation.
type ReorderRule struct {
	UUID         uuid.UUID          `json:"uuid"`
	Warehouse    ReorderWarehouse   `json:"warehouse"`
	Product      model.StockProduct `json:"product"`
	MinQuantity  int32              `json:"min_quantity"`
	ReorderPoint *int32             `json:"reorder_point"`
	MaxQuantity  *int32             `json:"max_quantity"`
	WarningDays  *int32             `json:"warning_days"`
	CoverDays    *int32             `json:"cover_days"`
	Balance      model.StockBalance `json:"balance"`
	// Level is ok, reorder, below_min or days_low.
	Level string `json:"level"`
	// AvgDaily is the forecast 30-day average use (null: no forecast).
	AvgDaily *string `json:"avg_daily"`
	// DaysLeft is available / avg_daily (null without a forecast).
	DaysLeft *string `json:"days_left"`
	// SuggestedQuantity tops the warehouse up to max_quantity (else the
	// cover days, else the reorder point / minimum), net of incoming.
	SuggestedQuantity int64      `json:"suggested_quantity"`
	AlertState        string     `json:"alert_state"`
	AlertedAt         *time.Time `json:"alerted_at"`
	// MinMeters is the meter threshold (open rolls + cut pieces, TEC-1243).
	MinMeters *string `json:"min_meters"`
	// MeterLevel is ok or below_min (always ok without min_meters).
	MeterLevel      string     `json:"meter_level"`
	MeterAlertState string     `json:"meter_alert_state"`
	MeterAlertedAt  *time.Time `json:"meter_alerted_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	id              int64
	// boxMeters is the product's box length in centimeters (0: none).
	boxMeters int64
}

// Critical reports whether either threshold is crossed.
func (r ReorderRule) Critical() bool {
	return r.Level != ReorderLevelOK || r.MeterLevel != ReorderLevelOK
}

// ReorderFilter filters the rule lists.
type ReorderFilter struct {
	WarehouseUUID *uuid.UUID
	ProductUUID   *uuid.UUID
	Q             string
	CriticalOnly  bool
	Sort          apiquery.ResolvedSort
	Limit         int32
	Offset        int32
}

// ParseReorderFilter reads warehouse_uuid, product_uuid, q, sort, limit,
// offset.
func ParseReorderFilter(values url.Values) (ReorderFilter, error) {
	q := apiquery.Parse(values)
	f := ReorderFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	for name, dst := range map[string]**uuid.UUID{"warehouse_uuid": &f.WarehouseUUID, "product_uuid": &f.ProductUUID} {
		raw := strings.TrimSpace(values.Get(name))
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, movementInvalid(name, "is invalid")
		}
		*dst = &id
	}
	var err error
	f.Sort, err = apiquery.ResolveSort(q.Sort, ReorderSort)
	return f, err
}

// ReorderRuleInput creates or replaces the rule of a warehouse and product.
type ReorderRuleInput struct {
	WarehouseUUID string `json:"warehouse_uuid"`
	ProductUUID   string `json:"product_uuid"`
	MinQuantity   int32  `json:"min_quantity"`
	ReorderPoint  *int32 `json:"reorder_point"`
	MaxQuantity   *int32 `json:"max_quantity"`
	WarningDays   *int32 `json:"warning_days"`
	CoverDays     *int32 `json:"cover_days"`
	// MinMeters is the meter threshold ("25.50"; null or empty: none).
	MinMeters *string `json:"min_meters"`
}

// MaxReorderMeters bounds the meter threshold (centimeters).
const MaxReorderMeters = 100000 * 100

// minMetersCm parses the meter threshold (0: none).
func (in ReorderRuleInput) minMetersCm() (int64, error) {
	if in.MinMeters == nil || strings.TrimSpace(*in.MinMeters) == "" {
		return 0, nil
	}
	v, err := ledger.ParseMeters(*in.MinMeters)
	if err != nil || v <= 0 || v > MaxReorderMeters {
		return 0, movementInvalid("min_meters", "must be between 0.01 and 100000")
	}
	return v, nil
}

func i4(p *int32) pgtype.Int4 {
	if p == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *p, Valid: true}
}

func i4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	x := v.Int32
	return &x
}

func (in ReorderRuleInput) validate() error {
	bad := func(field, msg string) error { return movementInvalid(field, msg) }
	inRange := func(v int32) bool { return v >= 0 && v <= MaxReorderQuantity }
	if !inRange(in.MinQuantity) {
		return bad("min_quantity", "must be between 0 and 1000000")
	}
	if in.ReorderPoint != nil && (!inRange(*in.ReorderPoint) || *in.ReorderPoint < in.MinQuantity) {
		return bad("reorder_point", "must be at least min_quantity")
	}
	if in.MaxQuantity != nil {
		floor := in.MinQuantity
		if in.ReorderPoint != nil {
			floor = max(floor, *in.ReorderPoint)
		}
		if !inRange(*in.MaxQuantity) || *in.MaxQuantity < floor || *in.MaxQuantity == 0 {
			return bad("max_quantity", "must be above zero and at least the reorder point and min_quantity")
		}
	}
	for field, v := range map[string]*int32{"warning_days": in.WarningDays, "cover_days": in.CoverDays} {
		if v != nil && (*v < 1 || *v > 3650) {
			return bad(field, "must be between 1 and 3650")
		}
	}
	meters, err := in.minMetersCm()
	if err != nil {
		return err
	}
	if in.MinQuantity == 0 && in.ReorderPoint == nil && in.MaxQuantity == nil && in.WarningDays == nil && meters == 0 {
		return bad("min_quantity", "set min_quantity, reorder_point, max_quantity, warning_days or min_meters")
	}
	return nil
}

func (s *Service) callerOrg(c Caller) (int64, error) {
	if c.Org.InternalID == 0 || !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) || c.Filter.UserOnly() {
		return 0, ErrReorderForbidden
	}
	return c.Org.InternalID, nil
}

// UpsertReorderRule creates or replaces the rule of a warehouse (of the
// active organization) and a product of its brand.
func (s *Service) UpsertReorderRule(ctx context.Context, c Caller, in ReorderRuleInput) (ReorderRule, error) {
	org, err := s.callerOrg(c)
	if err != nil {
		return ReorderRule{}, err
	}
	if err := in.validate(); err != nil {
		return ReorderRule{}, err
	}
	whID, err := uuid.Parse(strings.TrimSpace(in.WarehouseUUID))
	if err != nil {
		return ReorderRule{}, movementInvalid("warehouse_uuid", "is invalid")
	}
	pID, err := uuid.Parse(strings.TrimSpace(in.ProductUUID))
	if err != nil {
		return ReorderRule{}, movementInvalid("product_uuid", "is invalid")
	}
	wh, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: whID, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReorderRule{}, movementInvalid("warehouse_uuid", "is not a warehouse of the organization")
	}
	if err != nil {
		return ReorderRule{}, fmt.Errorf("stock: reorder warehouse: %w", err)
	}
	p, err := s.q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: pID, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReorderRule{}, movementInvalid("product_uuid", "is not a product of the brand")
	}
	if err != nil {
		return ReorderRule{}, fmt.Errorf("stock: reorder product: %w", err)
	}
	user := pgtype.Int8{Int64: c.Principal.UserInternal, Valid: c.Principal.UserInternal > 0}
	metersCm, _ := in.minMetersCm()
	var minMeters pgtype.Numeric
	if metersCm > 0 {
		minMeters = cmNumeric(metersCm)
	}
	row, err := s.q.UpsertStockReorderRule(ctx, db.UpsertStockReorderRuleParams{
		OrganizationID: org, BrandID: p.BrandID, WarehouseID: wh.ID, ProductID: p.ID,
		MinQuantity: in.MinQuantity, ReorderPoint: i4(in.ReorderPoint), MaxQuantity: i4(in.MaxQuantity),
		WarningDays: i4(in.WarningDays), CoverDays: i4(in.CoverDays), MinMeters: minMeters, UserID: user,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return ReorderRule{}, movementInvalid("min_quantity", "the quantities are not consistent")
	}
	if err != nil {
		return ReorderRule{}, fmt.Errorf("stock: upsert reorder rule: %w", err)
	}
	rules, err := s.reorderRules(ctx, org, ReorderFilter{})
	if err != nil {
		return ReorderRule{}, err
	}
	for _, r := range rules {
		if r.UUID == row.Uuid {
			return r, nil
		}
	}
	return ReorderRule{}, ErrNotFound
}

// DeleteReorderRule removes a rule of the active organization.
func (s *Service) DeleteReorderRule(ctx context.Context, c Caller, id uuid.UUID) error {
	org, err := s.callerOrg(c)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteStockReorderRule(ctx, db.DeleteStockReorderRuleParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return fmt.Errorf("stock: delete reorder rule: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderRules lists the rules of the active organization with their
// evaluation; CriticalOnly keeps the rules whose level is not ok.
func (s *Service) ReorderRules(ctx context.Context, c Caller, f ReorderFilter) ([]ReorderRule, int64, error) {
	org, err := s.callerOrg(c)
	if err != nil {
		return nil, 0, err
	}
	all, err := s.reorderRules(ctx, org, f)
	if err != nil {
		return nil, 0, err
	}
	if f.CriticalOnly {
		all = slices.DeleteFunc(all, func(r ReorderRule) bool { return !r.Critical() })
	}
	if q := strings.ToLower(strings.TrimSpace(f.Q)); q != "" {
		all = slices.DeleteFunc(all, func(r ReorderRule) bool {
			return !strings.Contains(strings.ToLower(r.Product.Name), q) && !strings.Contains(strings.ToLower(r.Product.SKU), q) &&
				!strings.Contains(strings.ToLower(r.Warehouse.Name), q)
		})
	}
	sortReorder(all, f.Sort)
	page, total := apiquery.Slice(all, f.Limit, f.Offset)
	return page, total, nil
}

var levelRank = map[string]int{ReorderLevelBelowMin: 0, ReorderLevelReorder: 1, ReorderLevelDaysLow: 2, ReorderLevelOK: 3}

// rank orders a rule by its most urgent level (a meter threshold below its
// minimum ranks with below_min).
func (r ReorderRule) rank() int {
	return min(levelRank[r.Level], levelRank[r.MeterLevel])
}
func sortReorder(rows []ReorderRule, s apiquery.ResolvedSort) {
	key, desc := resolvedSort(s, ReorderSort)
	slices.SortStableFunc(rows, func(a, b ReorderRule) int {
		var c int
		switch key {
		case "product":
			c = strings.Compare(strings.ToLower(a.Product.Name), strings.ToLower(b.Product.Name))
		case "warehouse":
			c = strings.Compare(strings.ToLower(a.Warehouse.Name), strings.ToLower(b.Warehouse.Name))
		case "available":
			c = int(a.boxCount() - b.boxCount())
		default:
			c = a.rank() - b.rank()
		}
		if desc {
			c = -c
		}
		if c == 0 {
			c = strings.Compare(a.UUID.String(), b.UUID.String())
		}
		return c
	})
}

// reorderRules loads and evaluates the rules of an organization.
func (s *Service) reorderRules(ctx context.Context, org int64, f ReorderFilter) ([]ReorderRule, error) {
	var warehouse, product pgtype.Int8
	if f.WarehouseUUID != nil {
		wh, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: *f.WarehouseUUID, OrganizationID: org})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("stock: reorder warehouse: %w", err)
		}
		warehouse = pgtype.Int8{Int64: wh.ID, Valid: true}
	}
	if f.ProductUUID != nil {
		id, err := s.q.GetProductIDByUUID(ctx, *f.ProductUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return []ReorderRule{}, nil
		}
		if err != nil {
			return nil, fmt.Errorf("stock: reorder product: %w", err)
		}
		product = pgtype.Int8{Int64: id, Valid: true}
	}
	rows, err := s.q.ListStockReorderRules(ctx, db.ListStockReorderRulesParams{
		OrganizationID: org, WarehouseID: warehouse, ProductID: product,
	})
	if err != nil {
		return nil, fmt.Errorf("stock: reorder rules: %w", err)
	}
	if len(rows) == 0 {
		return []ReorderRule{}, nil
	}
	productIDs := map[int64]bool{}
	byWarehouse := map[int64][]int64{}
	for _, r := range rows {
		productIDs[r.ProductID] = true
		byWarehouse[r.WarehouseID] = append(byWarehouse[r.WarehouseID], r.ProductID)
	}
	ids := make([]int64, 0, len(productIDs))
	for id := range productIDs {
		ids = append(ids, id)
	}
	avg := map[int64]float64{}
	avgRaw := map[int64]string{}
	fc, err := s.q.ListLatestForecastAvgDaily(ctx, db.ListLatestForecastAvgDailyParams{OrganizationID: org, ProductIds: ids})
	if err != nil {
		return nil, fmt.Errorf("stock: reorder forecasts: %w", err)
	}
	for _, r := range fc {
		if v, err := strconv.ParseFloat(r.AvgDaily30, 64); err == nil {
			avg[r.ProductID], avgRaw[r.ProductID] = v, r.AvgDaily30
		}
	}
	balances := map[[2]int64]model.StockBalance{}
	for wh, pids := range byWarehouse {
		onRows, err := s.q.ListWarehouseOnHandByProduct(ctx, db.ListWarehouseOnHandByProductParams{
			WarehouseID: wh, OrganizationID: org, ProductIds: pids,
		})
		if err != nil {
			return nil, fmt.Errorf("stock: reorder on hand: %w", err)
		}
		onHand := make(map[int64]Amount, len(pids))
		for _, pid := range pids {
			onHand[pid] = Amount{}
		}
		for _, r := range onRows {
			onHand[r.ProductID] = Amount{Quantity: r.Quantity, Centimeters: cm(r.Meters)}
		}
		bal, err := s.Balances(ctx, BalanceScope{OrganizationID: org, WarehouseID: wh}, onHand)
		if err != nil {
			return nil, err
		}
		for pid, b := range bal {
			balances[[2]int64{wh, pid}] = b
		}
	}
	out := make([]ReorderRule, 0, len(rows))
	for _, r := range rows {
		rule := ReorderRule{
			UUID:        r.Uuid,
			Warehouse:   ReorderWarehouse{UUID: r.WarehouseUuid, Code: r.WarehouseCode, Name: r.WarehouseName},
			Product:     model.StockProduct{UUID: r.ProductUuid, SKU: r.Sku, Name: r.ProductName, UnitType: r.UnitType, Category: model.CategoryRef{UUID: r.CategoryUuid, Name: r.CategoryName}},
			MinQuantity: r.MinQuantity, ReorderPoint: i4Ptr(r.ReorderPoint), MaxQuantity: i4Ptr(r.MaxQuantity),
			WarningDays: i4Ptr(r.WarningDays), CoverDays: i4Ptr(r.CoverDays),
			Balance:    balances[[2]int64{r.WarehouseID, r.ProductID}],
			AlertState: r.AlertState, UpdatedAt: r.UpdatedAt.Time,
			MinMeters: numericPtr(r.MinMeters), MeterAlertState: r.MeterAlertState,
			id: r.ID,
		}
		if r.SellableByMeter && r.DefaultRollMeters != "" {
			rule.boxMeters = cm(r.DefaultRollMeters)
		}
		if r.MeterAlertedAt.Valid {
			t := r.MeterAlertedAt.Time
			rule.MeterAlertedAt = &t
		}
		rule.Product.Active = true
		if r.AlertedAt.Valid {
			t := r.AlertedAt.Time
			rule.AlertedAt = &t
		}
		var avgPtr *float64
		if v, ok := avg[r.ProductID]; ok {
			raw := avgRaw[r.ProductID]
			rule.AvgDaily, avgPtr = &raw, &v
		}
		EvaluateReorder(&rule, avgPtr)
		out = append(out, rule)
	}
	return out, nil
}

// boxCount is the quantity the box thresholds compare: the available
// closed boxes, or for a roll_meter product (no boxes) the available rolls.
func (r ReorderRule) boxCount() int64 {
	if r.Product.UnitType == "roll_meter" {
		return r.Balance.AvailableQuantity
	}
	return r.Balance.Measures.AvailableBoxes
}

// boxEquivalent is the available stock in boxes for the forecast days: a
// product sold by the meter adds its available meters / box meters.
func (r ReorderRule) boxEquivalent() float64 {
	n := float64(r.boxCount())
	if r.boxMeters > 0 {
		n += float64(cm(r.Balance.Measures.AvailableMeters)) / float64(r.boxMeters)
	}
	return n
}

// EvaluateReorder sets Level, MeterLevel, DaysLeft and SuggestedQuantity of
// a rule from its Balance and the forecast average daily use (nil: none).
func EvaluateReorder(r *ReorderRule, avgDaily *float64) {
	available := r.boxCount()
	r.Level = ReorderLevelOK
	r.MeterLevel = ReorderLevelOK
	if r.MinMeters != nil && cm(r.Balance.Measures.AvailableMeters) < cm(*r.MinMeters) {
		r.MeterLevel = ReorderLevelBelowMin
	}
	var days *float64
	if avgDaily != nil && *avgDaily > 0 {
		d := r.boxEquivalent() / *avgDaily
		days = &d
		s := strconv.FormatFloat(math.Floor(d*10)/10, 'f', 1, 64)
		r.DaysLeft = &s
	}
	switch {
	case r.MinQuantity > 0 && available < int64(r.MinQuantity):
		r.Level = ReorderLevelBelowMin
	case r.ReorderPoint != nil && available <= int64(*r.ReorderPoint):
		r.Level = ReorderLevelReorder
	case r.WarningDays != nil && days != nil && *days < float64(*r.WarningDays):
		r.Level = ReorderLevelDaysLow
	}
	r.SuggestedQuantity = 0
	if r.Level == ReorderLevelOK {
		return
	}
	var target int64
	switch {
	case r.MaxQuantity != nil:
		target = int64(*r.MaxQuantity)
	case r.CoverDays != nil && avgDaily != nil && *avgDaily > 0:
		target = int64(math.Ceil(*avgDaily * float64(*r.CoverDays)))
	case r.ReorderPoint != nil:
		target = int64(*r.ReorderPoint) + 1
	default:
		target = int64(r.MinQuantity)
	}
	r.SuggestedQuantity = max(target-available-r.Balance.IncomingQuantity, 0)
}

// threshold is the number the notification names.
func (r ReorderRule) threshold() int32 {
	if r.Level == ReorderLevelReorder && r.ReorderPoint != nil {
		return *r.ReorderPoint
	}
	if r.Level == ReorderLevelDaysLow && r.WarningDays != nil {
		return *r.WarningDays
	}
	return r.MinQuantity
}

// ReorderAlerts runs the hourly low stock check of every organization
// with rules (worker).
type ReorderAlerts struct {
	pool TxBeginner
	svc  *Service
	out  outbox.Enqueuer
}

// NewReorderAlerts builds the checker.
func NewReorderAlerts(pool TxBeginner, svc *Service, out outbox.Enqueuer) *ReorderAlerts {
	return &ReorderAlerts{pool: pool, svc: svc, out: out}
}

// Task checks every organization; one organization's failure does not stop
// the others (the first error is returned).
func (a *ReorderAlerts) Task(ctx context.Context) error {
	orgs, err := a.svc.q.ListStockReorderOrganizations(ctx)
	if err != nil {
		return fmt.Errorf("stock: reorder organizations: %w", err)
	}
	var first error
	for _, org := range orgs {
		if _, err := a.Check(ctx, org); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Check evaluates the rules of one organization, notifies the ok -> low
// edges and records the state. It returns the number of notifications.
func (a *ReorderAlerts) Check(ctx context.Context, orgID int64) (int, error) {
	rules, err := a.svc.reorderRules(ctx, orgID, ReorderFilter{})
	if err != nil {
		return 0, err
	}
	o, err := a.svc.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("stock: reorder organization: %w", err)
	}
	var recipients []int64
	sent := 0
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("stock: reorder begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := a.svc.q.WithTx(tx)
	notify := func(r ReorderRule, extra map[string]any) error {
		if recipients == nil {
			if recipients, err = q.ListStockForecastNotifyUserIDs(ctx, orgID); err != nil {
				return fmt.Errorf("stock: reorder recipients: %w", err)
			}
		}
		if len(recipients) == 0 || a.out == nil {
			return nil
		}
		payload := map[string]any{
			"rule_uuid": r.UUID.String(), "organization_id": orgID, "brand_id": o.BrandID,
			"product_uuid": r.Product.UUID.String(), "product_name": r.Product.Name,
			"warehouse_uuid": r.Warehouse.UUID.String(), "warehouse_name": r.Warehouse.Name,
			"notify_user_ids": recipients,
		}
		for k, v := range extra {
			payload[k] = v
		}
		ev := events.New(events.StockReorderLow).WithTenant(orgID).WithPayload(payload)
		if err := a.out.Enqueue(ctx, tx, ev); err != nil {
			return fmt.Errorf("stock: reorder event: %w", err)
		}
		sent++
		return nil
	}
	for _, r := range rules {
		low := r.Level != ReorderLevelOK
		state := ReorderLevelOK
		if low {
			state = "low"
		}
		boxNotify := low && r.AlertState != "low"
		if boxNotify {
			if err := notify(r, map[string]any{
				"measure":            "box",
				"available_quantity": strconv.FormatInt(r.boxCount(), 10),
				"threshold":          strconv.Itoa(int(r.threshold())), "level": r.Level,
			}); err != nil {
				return 0, err
			}
		}
		if err := q.SetStockReorderRuleAlert(ctx, db.SetStockReorderRuleAlertParams{
			AlertState: state, Alerted: boxNotify, ID: r.id,
		}); err != nil {
			return 0, fmt.Errorf("stock: reorder state: %w", err)
		}
		// TEC-1243 (K14): the meter threshold alerts on its own edge.
		meterLow := r.MeterLevel != ReorderLevelOK
		meterState := ReorderLevelOK
		if meterLow {
			meterState = "low"
		}
		meterNotify := meterLow && r.MeterAlertState != "low"
		if meterNotify {
			if err := notify(r, map[string]any{
				"measure":          "meter",
				"available_meters": r.Balance.Measures.AvailableMeters,
				"threshold":        *r.MinMeters, "level": r.MeterLevel,
			}); err != nil {
				return 0, err
			}
		}
		if meterState != r.MeterAlertState || meterNotify {
			if err := q.SetStockReorderRuleMeterAlert(ctx, db.SetStockReorderRuleMeterAlertParams{
				AlertState: meterState, Alerted: meterNotify, ID: r.id,
			}); err != nil {
				return 0, fmt.Errorf("stock: reorder meter state: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("stock: reorder commit: %w", err)
	}
	return sent, nil
}
