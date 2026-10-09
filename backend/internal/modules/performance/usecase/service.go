package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	accountingmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	perfrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/repository"
	tasksuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	EventPerformanceComputed    = events.PerformanceComputed
	EventPerformanceWeakDealer  = events.PerformanceWeakDealer
	EventPerformanceBelowTarget = events.PerformanceBelowTarget
	EventBonusCalculated        = events.PerformanceBonusCalculated
	EventBonusApproved          = events.PerformanceBonusApproved

	ScopeOrg     = "org"
	ScopeSubtree = "subtree"
)

var (
	ErrNotFound  = errors.New("performance: not found")
	ErrForbidden = errors.New("performance: forbidden")
)

type ValidationError struct {
	Field   string
	Message string
	Code    string
}

func (e *ValidationError) Error() string { return "performance: invalid " + e.Field + ": " + e.Message }

func invalid(field, msg string) error {
	return &ValidationError{Field: field, Message: msg, Code: "invalid"}
}

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

type Service struct {
	pool     TxBeginner
	q        *db.Queries
	store    *perfrepo.Store
	out      outbox.Enqueuer
	poster   *posting.Poster
	features FeatureChecker
	log      *slog.Logger
	now      func() time.Time
	panelURL string
}

func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, opts ...any) *Service {
	s := &Service{pool: pool, q: q, store: perfrepo.FromQueries(q), out: out, log: slog.Default(), now: time.Now}
	for _, opt := range opts {
		switch v := opt.(type) {
		case FeatureChecker:
			s.features = v
		case *posting.Poster:
			s.poster = v
		case *slog.Logger:
			if v != nil {
				s.log = v
			}
		}
	}
	return s
}

func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *Service) WithPanelURL(url string) *Service {
	s.panelURL = strings.TrimRight(url, "/")
	return s
}

type MetricValue struct {
	Value      string     `json:"value"`
	Currency   *string    `json:"currency,omitempty"`
	ComputedAt *time.Time `json:"computed_at,omitempty"`
}

type MetricDelta struct {
	Current  *MetricValue `json:"current,omitempty"`
	Previous *MetricValue `json:"previous,omitempty"`
	Delta    *string      `json:"delta,omitempty"`
	DeltaPct *string      `json:"delta_pct,omitempty"`
}

type TrendPoint struct {
	Period  string                  `json:"period"`
	Metrics map[string]*MetricValue `json:"metrics"`
}

type TargetView struct {
	UUID                   uuid.UUID `json:"uuid"`
	OwnerOrganizationUUID  uuid.UUID `json:"owner_organization_uuid,omitempty"`
	TargetOrganizationUUID uuid.UUID `json:"target_organization_uuid,omitempty"`
	TargetName             string    `json:"target_name,omitempty"`
	TargetType             string    `json:"target_type,omitempty"`
	Metric                 string    `json:"metric"`
	PeriodKind             string    `json:"period_kind"`
	PeriodStart            string    `json:"period_start"`
	PeriodEnd              string    `json:"period_end"`
	Value                  string    `json:"value"`
	Currency               *string   `json:"currency,omitempty"`
	ContractRef            *string   `json:"contract_ref,omitempty"`
	Note                   *string   `json:"note,omitempty"`
	Actual                 *string   `json:"actual,omitempty"`
	AchievementPct         *string   `json:"achievement_pct,omitempty"`
	CreatedAt              time.Time `json:"created_at,omitempty"`
	UpdatedAt              time.Time `json:"updated_at,omitempty"`
}

type Dashboard struct {
	Period           string                 `json:"period"`
	OrganizationUUID uuid.UUID              `json:"organization_uuid"`
	OrganizationName string                 `json:"organization_name"`
	Metrics          map[string]MetricDelta `json:"metrics"`
	Trend            []TrendPoint           `json:"trend"`
	Targets          []TargetView           `json:"targets"`
	Subtree          []SubtreeSummary       `json:"subtree"`
}

type SubtreeSummary struct {
	OrganizationUUID uuid.UUID               `json:"organization_uuid"`
	Name             string                  `json:"name"`
	Type             string                  `json:"type"`
	Metrics          map[string]*MetricValue `json:"metrics"`
}

// RankingOrg is the distributor of a ranking row (TEC-496).
type RankingOrg struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

type RankingRow struct {
	Rank             int                     `json:"rank"`
	OrganizationUUID uuid.UUID               `json:"organization_uuid,omitempty"`
	Name             string                  `json:"name,omitempty"`
	Type             string                  `json:"type"`
	Distributor      *RankingOrg             `json:"distributor,omitempty"`
	ProvinceID       *int64                  `json:"province_id,omitempty"`
	ProvinceName     *string                 `json:"province_name,omitempty"`
	Currency         string                  `json:"currency"`
	Metrics          map[string]*MetricValue `json:"metrics"`
	ComputedAt       *time.Time              `json:"computed_at,omitempty"`
}

type Benchmark struct {
	Period         string                  `json:"period"`
	Own            RankingRow              `json:"own"`
	NetworkAverage map[string]*MetricValue `json:"network_average"`
}

type StaffTargetView struct {
	UUID      uuid.UUID `json:"uuid"`
	UserID    int64     `json:"user_id"`
	UserName  string    `json:"user_name"`
	Period    string    `json:"period"`
	Metric    string    `json:"metric"`
	Value     string    `json:"value"`
	Currency  *string   `json:"currency,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type RuleView struct {
	UUID                  uuid.UUID `json:"uuid"`
	OwnerOrganizationUUID uuid.UUID `json:"owner_organization_uuid,omitempty"`
	OwnerName             string    `json:"owner_name,omitempty"`
	OwnerType             string    `json:"owner_type,omitempty"`
	Name                  string    `json:"name"`
	Metric                string    `json:"metric"`
	Operator              string    `json:"operator"`
	Threshold             string    `json:"threshold"`
	CreateTask            bool      `json:"create_task"`
	Notify                bool      `json:"notify"`
	AssigneeUserID        *int64    `json:"assignee_user_id,omitempty"`
	Active                bool      `json:"active"`
	CreatedAt             time.Time `json:"created_at,omitempty"`
	UpdatedAt             time.Time `json:"updated_at,omitempty"`
}

type RankingFilter struct {
	Period         string
	Q              string
	OrgTypes       []string
	DistributorIDs []int64
	// DistributorUUIDs keeps distributors and dealers of these distributors.
	DistributorUUIDs []uuid.UUID
	ProvinceIDs      []int64
	Sort             []apiquery.SortField
	Limit, Offset    int32
}

func ParseRankingFilter(values url.Values, now time.Time) (RankingFilter, error) {
	q := apiquery.Parse(values)
	f := RankingFilter{Period: monthParam(values.Get("period"), now), Q: q.Q, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.OrgTypes, err = apiquery.EnumList(values, "type", "distributor", "dealer"); err != nil {
		return f, err
	}
	for _, raw := range apiquery.CSVValues(values, "distributor_id") {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return f, invalid("distributor_id", "must be an integer")
		}
		f.DistributorIDs = append(f.DistributorIDs, id)
	}
	for _, raw := range apiquery.CSVValues(values, "distributor_uuid") {
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, invalid("distributor_uuid", "must be a list of UUIDs")
		}
		f.DistributorUUIDs = append(f.DistributorUUIDs, id)
	}
	for _, raw := range apiquery.CSVValues(values, "province_id") {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return f, invalid("province_id", "must be an integer")
		}
		f.ProvinceIDs = append(f.ProvinceIDs, id)
	}
	return f, nil
}

type TargetFilter struct {
	Q                        string
	Metrics                  []string
	PeriodKinds              []string
	PeriodFrom, PeriodBefore *time.Time
	Sort                     []apiquery.SortField
	Limit, Offset            int32
}

func ParseTargetFilter(values url.Values) (TargetFilter, error) {
	q := apiquery.Parse(values)
	f := TargetFilter{Q: q.Q, Sort: q.Sort, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Metrics, err = apiquery.EnumList(values, "metric", model.TargetMetrics...); err != nil {
		return f, err
	}
	if f.PeriodKinds, err = apiquery.EnumList(values, "period_kind", model.PeriodKinds...); err != nil {
		return f, err
	}
	dr, err := apiquery.DateRange(values, "period")
	if err != nil {
		return f, err
	}
	f.PeriodFrom, f.PeriodBefore = dr.From, dr.Before
	return f, nil
}

type StaffTargetFilter struct {
	PeriodFrom, PeriodTo string
	UserID               *int64
}

type RuleFilter struct{ Active *bool }

type TargetInput struct {
	TargetOrganizationUUID uuid.UUID `json:"target_organization_uuid"`
	Metric                 string    `json:"metric"`
	PeriodKind             string    `json:"period_kind"`
	PeriodStart            string    `json:"period_start"`
	Value                  string    `json:"value"`
	Currency               *string   `json:"currency"`
	ContractRef            *string   `json:"contract_ref"`
	Note                   *string   `json:"note"`
}

type StaffTargetInput struct {
	UserUUID uuid.UUID `json:"user_uuid"`
	Period   string    `json:"period"`
	Metric   string    `json:"metric"`
	Value    string    `json:"value"`
	Currency *string   `json:"currency"`
}

type RuleInput struct {
	Name           string `json:"name"`
	Metric         string `json:"metric"`
	Operator       string `json:"operator"`
	Threshold      string `json:"threshold"`
	CreateTask     bool   `json:"create_task"`
	Notify         bool   `json:"notify"`
	AssigneeUserID *int64 `json:"assignee_user_id"`
	Active         bool   `json:"active"`
}

type RuleRunResult struct {
	Period        string `json:"period"`
	Evaluated     int    `json:"evaluated"`
	Matched       int    `json:"matched"`
	TasksCreated  int    `json:"tasks_created"`
	Notifications int    `json:"notifications"`
}

type BonusFilter struct {
	Period        string
	UserID        *int64
	Statuses      []string
	Limit, Offset int32
}

type BonusAccrualView struct {
	UUID             uuid.UUID  `json:"uuid"`
	UserID           int64      `json:"user_id"`
	UserName         string     `json:"user_name"`
	Period           string     `json:"period"`
	RuleName         string     `json:"rule_name"`
	AchievementPct   string     `json:"achievement_pct"`
	Amount           string     `json:"amount"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	StaffPaymentID   *int64     `json:"staff_payment_id,omitempty"`
	ApprovedByUserID *int64     `json:"approved_by_user_id,omitempty"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`
	CancelledAt      *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type BonusApprovalInput struct {
	Amount *string `json:"amount"`
	Note   *string `json:"note"`
}

type BonusRunResult struct {
	Period        string `json:"period"`
	Evaluated     int    `json:"evaluated"`
	Accrued       int    `json:"accrued"`
	Notifications int    `json:"notifications"`
}

func (s *Service) Dashboard(ctx context.Context, c Caller, period string) (Dashboard, error) {
	period = monthParam(period, s.now())
	orgIDs, err := s.readOrgIDs(ctx, c)
	if err != nil {
		return Dashboard{}, err
	}
	metrics, err := s.q.ListPerformanceMetrics(ctx, db.ListPerformanceMetricsParams{
		BrandID: c.Org.BrandID, OrgIds: []int64{c.Org.InternalID}, PeriodFrom: addMonths(period, -1), PeriodTo: period, Scope: textArg(ScopeOrg),
	})
	if err != nil {
		return Dashboard{}, fmt.Errorf("performance: dashboard metrics: %w", err)
	}
	trendFrom := addMonths(period, -11)
	trendRows, err := s.q.ListPerformanceMetrics(ctx, db.ListPerformanceMetricsParams{
		BrandID: c.Org.BrandID, OrgIds: []int64{c.Org.InternalID}, PeriodFrom: trendFrom, PeriodTo: period, Scope: textArg(ScopeOrg),
	})
	if err != nil {
		return Dashboard{}, fmt.Errorf("performance: dashboard trend: %w", err)
	}
	targets, _, err := s.ListTargets(ctx, c, TargetFilter{Limit: 10, Offset: 0})
	if err != nil {
		return Dashboard{}, err
	}
	out := Dashboard{Period: period, OrganizationUUID: c.Org.UUID, OrganizationName: c.Org.Name, Metrics: map[string]MetricDelta{}, Targets: targets}
	cur := metricMap(metrics, period)
	prev := metricMap(metrics, addMonths(period, -1))
	for _, key := range model.Metrics {
		cv, pv := cur[key], prev[key]
		out.Metrics[key] = MetricDelta{Current: cv, Previous: pv, Delta: diff(cv, pv), DeltaPct: diffPct(cv, pv)}
	}
	byPeriod := map[string]map[string]*MetricValue{}
	for _, r := range trendRows {
		if byPeriod[r.Period] == nil {
			byPeriod[r.Period] = map[string]*MetricValue{}
		}
		byPeriod[r.Period][r.Metric] = metricValue(r.Value, r.Currency, r.ComputedAt)
	}
	for i := 11; i >= 0; i-- {
		p := addMonths(period, -i)
		out.Trend = append(out.Trend, TrendPoint{Period: p, Metrics: byPeriod[p]})
	}
	if len(orgIDs) > 0 && (c.Org.OrgType == "center" || c.Org.OrgType == "distributor") {
		rows, err := s.store.ListRanking(ctx, db.ListPerformanceRankingParams{
			BrandID: c.Org.BrandID, OrgIds: orgIDs, Period: period, Scope: ScopeOrg, RowLimit: 20,
		}, nil)
		if err != nil {
			return Dashboard{}, err
		}
		for _, r := range rows {
			out.Subtree = append(out.Subtree, SubtreeSummary{OrganizationUUID: r.OrganizationUuid, Name: r.Name, Type: r.Type, Metrics: rankingMetrics(r)})
		}
	}
	return out, nil
}

func (s *Service) ListRanking(ctx context.Context, c Caller, f RankingFilter) ([]RankingRow, int64, error) {
	if c.Org.OrgType == "dealer" {
		return nil, 0, ErrForbidden
	}
	f.Period = monthParam(f.Period, s.now())
	orgIDs, err := s.readOrgIDs(ctx, c)
	if err != nil {
		return nil, 0, err
	}
	if c.Org.OrgType == "distributor" && len(f.OrgTypes) == 0 {
		f.OrgTypes = []string{"dealer"}
	}
	rows, err := s.store.ListRanking(ctx, db.ListPerformanceRankingParams{
		BrandID: c.Org.BrandID, OrgIds: orgIDs, Period: f.Period, Scope: ScopeOrg, Q: textArg(f.Q), OrgTypes: f.OrgTypes,
		DistributorIds: f.DistributorIDs, DistributorUuids: f.DistributorUUIDs, ProvinceIds: f.ProvinceIDs, RowLimit: f.Limit, RowOffset: f.Offset,
	}, f.Sort)
	if err != nil {
		return nil, 0, err
	}
	out := make([]RankingRow, 0, len(rows))
	for i, r := range rows {
		row := RankingRow{Rank: int(f.Offset) + i + 1, OrganizationUUID: r.OrganizationUuid, Name: r.Name, Type: r.Type, Currency: r.Currency, Metrics: rankingMetrics(r), ComputedAt: timePtr(r.ComputedAt), ProvinceID: int64Ptr(r.ProvinceID), ProvinceName: textPtr(r.ProvinceName)}
		if r.DistributorUuid.Valid {
			row.Distributor = &RankingOrg{UUID: r.DistributorUuid.Bytes, Name: r.DistributorName.String}
		}
		out = append(out, row)
	}
	return out, totalRanking(rows), nil
}

func (s *Service) Benchmark(ctx context.Context, c Caller, period string) (Benchmark, error) {
	if c.Org.OrgType != "dealer" {
		return Benchmark{}, ErrForbidden
	}
	period = monthParam(period, s.now())
	rows, err := s.store.ListRanking(ctx, db.ListPerformanceRankingParams{
		BrandID: c.Org.BrandID, OrgIds: []int64{c.Org.InternalID}, Period: period, Scope: ScopeOrg, RowLimit: 1,
	}, nil)
	if err != nil {
		return Benchmark{}, err
	}
	var own RankingRow
	if len(rows) > 0 {
		own = RankingRow{Rank: 1, OrganizationUUID: c.Org.UUID, Type: c.Org.OrgType, Currency: rows[0].Currency, Metrics: rankingMetrics(rows[0]), ComputedAt: timePtr(rows[0].ComputedAt)}
	}
	all, err := s.store.ListRanking(ctx, db.ListPerformanceRankingParams{
		BrandID: c.Org.BrandID, OrgTypes: []string{"dealer"}, Period: period, Scope: ScopeOrg, RowLimit: 100,
	}, nil)
	if err != nil {
		return Benchmark{}, err
	}
	return Benchmark{Period: period, Own: own, NetworkAverage: averageMetrics(all)}, nil
}

func (s *Service) ListTargets(ctx context.Context, c Caller, f TargetFilter) ([]TargetView, int64, error) {
	ownerIDs, targetIDs, err := s.targetScopeIDs(ctx, c)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.store.ListTargets(ctx, db.ListPerformanceTargetsParams{
		BrandID: c.Org.BrandID, OwnerOrgIds: ownerIDs, TargetOrgIds: targetIDs, Metrics: f.Metrics, PeriodKinds: f.PeriodKinds,
		PeriodFrom: datePtr(f.PeriodFrom), PeriodBefore: datePtr(f.PeriodBefore), Q: textArg(f.Q), RowLimit: f.Limit, RowOffset: f.Offset,
	}, f.Sort)
	if err != nil {
		return nil, 0, err
	}
	out := make([]TargetView, 0, len(rows))
	for _, r := range rows {
		out = append(out, targetRowView(r))
	}
	return out, totalTargets(rows), nil
}

func (s *Service) CreateTarget(ctx context.Context, c Caller, in TargetInput) (TargetView, error) {
	target, err := s.validTargetOrg(ctx, c, in.TargetOrganizationUUID)
	if err != nil {
		return TargetView{}, err
	}
	arg, err := targetInput(c, target.ID, in)
	if err != nil {
		return TargetView{}, err
	}
	row, err := s.q.CreatePerformanceTarget(ctx, arg)
	if err != nil {
		return TargetView{}, mapPG(err)
	}
	return s.targetByIDView(ctx, c, row.Uuid)
}

func (s *Service) UpdateTarget(ctx context.Context, c Caller, id uuid.UUID, in TargetInput) (TargetView, error) {
	cur, err := s.q.GetPerformanceTarget(ctx, db.GetPerformanceTargetParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return TargetView{}, ErrNotFound
	}
	if err != nil {
		return TargetView{}, err
	}
	if err := s.canOwn(ctx, c, cur.OrganizationID, cur.TargetOrgID); err != nil {
		return TargetView{}, err
	}
	val, err := numeric(in.Value)
	if err != nil {
		return TargetView{}, invalid("value", "must be a positive decimal")
	}
	row, err := s.q.UpdatePerformanceTarget(ctx, db.UpdatePerformanceTargetParams{ID: cur.ID, BrandID: c.Org.BrandID, Value: val, Currency: currencyArg(in.Currency), ContractRef: dateStringArg(in.ContractRef), Note: textPtrArg(in.Note)})
	if err != nil {
		return TargetView{}, mapPG(err)
	}
	return s.targetByIDView(ctx, c, row.Uuid)
}

func (s *Service) DeleteTarget(ctx context.Context, c Caller, id uuid.UUID) error {
	cur, err := s.q.GetPerformanceTarget(ctx, db.GetPerformanceTargetParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := s.canOwn(ctx, c, cur.OrganizationID, cur.TargetOrgID); err != nil {
		return err
	}
	n, err := s.q.DeletePerformanceTarget(ctx, db.DeletePerformanceTargetParams{ID: cur.ID, BrandID: c.Org.BrandID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListStaffTargets(ctx context.Context, c Caller, f StaffTargetFilter) ([]StaffTargetView, error) {
	from, to := f.PeriodFrom, f.PeriodTo
	if from == "" {
		from = addMonths(monthParam("", s.now()), -11)
	}
	if to == "" {
		to = monthParam("", s.now())
	}
	rows, err := s.q.ListStaffTargets(ctx, db.ListStaffTargetsParams{OrganizationID: c.Org.InternalID, PeriodFrom: from, PeriodTo: to, UserID: int8Ptr(f.UserID)})
	if err != nil {
		return nil, err
	}
	out := make([]StaffTargetView, 0, len(rows))
	for _, r := range rows {
		out = append(out, StaffTargetView{UUID: r.Uuid, UserID: r.UserID, UserName: strings.TrimSpace(r.UserName + " " + r.UserSurname), Period: r.Period, Metric: r.Metric, Value: numText(r.Value), Currency: textPtr(r.Currency), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time})
	}
	return out, nil
}

func (s *Service) UpsertStaffTarget(ctx context.Context, c Caller, in StaffTargetInput) (StaffTargetView, error) {
	u, err := s.q.GetOrganizationMemberByUserUUID(ctx, db.GetOrganizationMemberByUserUUIDParams{OrganizationID: c.Org.InternalID, Uuid: in.UserUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return StaffTargetView{}, ErrNotFound
	}
	if err != nil {
		return StaffTargetView{}, err
	}
	val, err := numeric(in.Value)
	if err != nil {
		return StaffTargetView{}, invalid("value", "must be a positive decimal")
	}
	if !contains(model.StaffMetrics, in.Metric) {
		return StaffTargetView{}, invalid("metric", "invalid")
	}
	row, err := s.q.UpsertStaffTarget(ctx, db.UpsertStaffTargetParams{OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, UserID: u.UserID, Period: in.Period, Metric: in.Metric, Value: val, Currency: currencyArg(in.Currency), CreatedByUserID: userArg(c)})
	if err != nil {
		return StaffTargetView{}, mapPG(err)
	}
	return StaffTargetView{UUID: row.Uuid, UserID: row.UserID, UserName: strings.TrimSpace(u.Name + " " + u.Surname), Period: row.Period, Metric: row.Metric, Value: numText(row.Value), Currency: textPtr(row.Currency), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

func (s *Service) DeleteStaffTarget(ctx context.Context, c Caller, id uuid.UUID) error {
	cur, err := s.q.GetStaffTarget(ctx, db.GetStaffTargetParams{Uuid: id, OrganizationID: c.Org.InternalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	n, err := s.q.DeleteStaffTarget(ctx, db.DeleteStaffTargetParams{ID: cur.ID, OrganizationID: c.Org.InternalID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func ParseBonusFilter(values url.Values, now time.Time) (BonusFilter, error) {
	q := apiquery.Parse(values)
	f := BonusFilter{Period: values.Get("period"), Limit: q.Limit, Offset: q.Offset}
	if f.Period == "" {
		f.Period = monthParam("", now)
	}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", model.AccrualStatuses...); err != nil {
		return f, err
	}
	if raw := strings.TrimSpace(values.Get("user_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return f, invalid("user_id", "must be an integer")
		}
		f.UserID = &id
	}
	return f, nil
}

func (s *Service) ListBonuses(ctx context.Context, c Caller, f BonusFilter) ([]BonusAccrualView, int64, error) {
	if c.Org.OrgType != "dealer" {
		return nil, 0, ErrForbidden
	}
	if err := s.ensureBonusFeatures(ctx, c.Org.InternalID, true); err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListBonusAccruals(ctx, db.ListBonusAccrualsParams{
		OrganizationID: c.Org.InternalID,
		Period:         textArg(f.Period),
		Statuses:       f.Statuses,
		UserID:         int8Ptr(f.UserID),
		RowLimit:       f.Limit,
		RowOffset:      f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]BonusAccrualView, 0, len(rows))
	for _, r := range rows {
		out = append(out, bonusAccrualRowView(r))
	}
	return out, totalBonuses(rows), nil
}

func (s *Service) CalculateBonuses(ctx context.Context, organizationID int64, period string) (BonusRunResult, error) {
	org, err := s.q.GetOrganizationByID(ctx, organizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BonusRunResult{}, ErrNotFound
	}
	if err != nil {
		return BonusRunResult{}, err
	}
	return s.calculateBonuses(ctx, org, monthParam(period, s.now()), true)
}

func (s *Service) calculateBonuses(ctx context.Context, org db.Organization, period string, strict bool) (BonusRunResult, error) {
	if org.Type != "dealer" {
		if strict {
			return BonusRunResult{}, ErrForbidden
		}
		return BonusRunResult{Period: period}, nil
	}
	if err := s.ensureBonusFeatures(ctx, org.ID, strict); err != nil {
		if strict {
			return BonusRunResult{}, err
		}
		return BonusRunResult{Period: period}, nil
	}
	from, to, err := periodBounds(period)
	if err != nil {
		return BonusRunResult{}, err
	}
	rows, err := s.q.ListBonusCalculationCandidates(ctx, db.ListBonusCalculationCandidatesParams{
		OrganizationID: org.ID,
		BrandID:        org.BrandID,
		Period:         period,
		PeriodFrom:     ts(from),
		PeriodTo:       ts(to),
	})
	if err != nil {
		return BonusRunResult{}, err
	}
	res := BonusRunResult{Period: period, Evaluated: len(rows)}
	var calculated []db.BonusAccrual
	for _, r := range rows {
		achievement := pct(r.ActualValue, r.TargetValue)
		if numFloat(achievement) < numFloat(r.ThresholdPct) {
			continue
		}
		amount, currency, ok := bonusAmount(org.Currency, r)
		if !ok {
			continue
		}
		accrual, err := s.q.UpsertBonusAccrual(ctx, db.UpsertBonusAccrualParams{
			OrganizationID: org.ID,
			BrandID:        org.BrandID,
			UserID:         r.UserID,
			Period:         period,
			RuleID:         r.RuleID,
			AchievementPct: achievement,
			Amount:         amount,
			Currency:       currency,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return res, mapPG(err)
		}
		res.Accrued++
		calculated = append(calculated, accrual)
	}
	n, err := s.notifyBonusesCalculated(ctx, org, period, calculated)
	if err != nil {
		return res, err
	}
	res.Notifications = n
	return res, nil
}

func (s *Service) ApproveBonus(ctx context.Context, c Caller, id uuid.UUID, in BonusApprovalInput) (BonusAccrualView, error) {
	if c.Org.OrgType != "dealer" {
		return BonusAccrualView{}, ErrForbidden
	}
	if err := s.ensureBonusFeatures(ctx, c.Org.InternalID, true); err != nil {
		return BonusAccrualView{}, err
	}
	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}
	overrideAmount := pgtype.Numeric{}
	if in.Amount != nil && strings.TrimSpace(*in.Amount) != "" {
		if note == "" {
			return BonusAccrualView{}, invalid("note", "is required when amount is adjusted")
		}
		n, err := numeric(*in.Amount)
		if err != nil {
			return BonusAccrualView{}, invalid("amount", "must be a positive decimal")
		}
		overrideAmount = n
	}
	org, err := s.q.GetOrganizationByID(ctx, c.Org.InternalID)
	if err != nil {
		return BonusAccrualView{}, err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		qtx := s.q.WithTx(tx)
		cur, err := qtx.LockBonusAccrual(ctx, db.LockBonusAccrualParams{Uuid: id, OrganizationID: c.Org.InternalID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if cur.Status != model.AccrualCalculated {
			return ErrNotFound
		}
		if overrideAmount.Valid {
			cur.Amount = overrideAmount
		}
		approved, err := qtx.ApproveBonusAccrual(ctx, db.ApproveBonusAccrualParams{ID: cur.ID, OrganizationID: c.Org.InternalID, Amount: cur.Amount, ApprovedByUserID: userArg(c)})
		if err != nil {
			return err
		}
		staff, err := qtx.GetStaffProfileByUserID(ctx, db.GetStaffProfileByUserIDParams{OrganizationID: c.Org.InternalID, UserID: pgtype.Int8{Int64: approved.UserID, Valid: true}})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		paidOn, err := s.bonusPayoutDate(ctx, qtx, org, approved.Period)
		if err != nil {
			return err
		}
		status := "posted"
		if paidOn.After(bookToday(org, s.now())) {
			status = "planned"
		}
		desc := "Prim tahakkuku"
		if note != "" {
			desc += ": " + note
		}
		payment, err := qtx.CreateStaffPayment(ctx, db.CreateStaffPaymentParams{
			OrganizationID:  c.Org.InternalID,
			BrandID:         c.Org.BrandID,
			StaffID:         staff.ID,
			Type:            "bonus",
			Period:          approved.Period,
			Amount:          cur.Amount,
			Currency:        approved.Currency,
			PaidOn:          pgtype.Date{Time: paidOn, Valid: true},
			Description:     textArg(desc),
			CreatedByUserID: userArg(c),
			Status:          status,
		})
		if err != nil {
			return mapPG(err)
		}
		if status == "posted" {
			if err := s.postBonusStaffPayment(ctx, tx, payment, userArg(c)); err != nil {
				return err
			}
		}
		if _, err := qtx.MarkBonusAccrualPosted(ctx, db.MarkBonusAccrualPostedParams{
			ID:             approved.ID,
			OrganizationID: approved.OrganizationID,
			StaffPaymentID: pgtype.Int8{Int64: payment.ID, Valid: true},
		}); err != nil {
			return err
		}
		return s.notifyBonusApproved(ctx, tx, org, approved, payment)
	})
	if err != nil {
		return BonusAccrualView{}, err
	}
	return s.bonusByIDView(ctx, c, id)
}

func (s *Service) CancelBonus(ctx context.Context, c Caller, id uuid.UUID) (BonusAccrualView, error) {
	if c.Org.OrgType != "dealer" {
		return BonusAccrualView{}, ErrForbidden
	}
	if err := s.ensureBonusFeatures(ctx, c.Org.InternalID, true); err != nil {
		return BonusAccrualView{}, err
	}
	cur, err := s.q.LockBonusAccrual(ctx, db.LockBonusAccrualParams{Uuid: id, OrganizationID: c.Org.InternalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return BonusAccrualView{}, ErrNotFound
	}
	if err != nil {
		return BonusAccrualView{}, err
	}
	row, err := s.q.CancelBonusAccrual(ctx, db.CancelBonusAccrualParams{ID: cur.ID, OrganizationID: c.Org.InternalID})
	if errors.Is(err, pgx.ErrNoRows) {
		return BonusAccrualView{}, ErrNotFound
	}
	if err != nil {
		return BonusAccrualView{}, err
	}
	return s.bonusByIDView(ctx, c, row.Uuid)
}

func (s *Service) ListRules(ctx context.Context, c Caller, f RuleFilter) ([]RuleView, error) {
	owners, err := s.ruleOwnerIDs(ctx, c)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListWeakDealerRules(ctx, db.ListWeakDealerRulesParams{BrandID: c.Org.BrandID, OwnerOrgIds: owners, Active: boolArg(f.Active)})
	if err != nil {
		return nil, err
	}
	out := make([]RuleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, ruleRowView(r))
	}
	return out, nil
}

func (s *Service) CreateRule(ctx context.Context, c Caller, in RuleInput) (RuleView, error) {
	arg, err := ruleInput(c, in)
	if err != nil {
		return RuleView{}, err
	}
	row, err := s.q.CreateWeakDealerRule(ctx, arg)
	if err != nil {
		return RuleView{}, mapPG(err)
	}
	return ruleView(row), nil
}

func (s *Service) UpdateRule(ctx context.Context, c Caller, id uuid.UUID, in RuleInput) (RuleView, error) {
	cur, err := s.q.GetWeakDealerRule(ctx, db.GetWeakDealerRuleParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return RuleView{}, ErrNotFound
	}
	if err != nil {
		return RuleView{}, err
	}
	if cur.OrganizationID != c.Org.InternalID {
		return RuleView{}, ErrNotFound
	}
	arg, err := ruleUpdateInput(cur.ID, c.Org.BrandID, in)
	if err != nil {
		return RuleView{}, err
	}
	row, err := s.q.UpdateWeakDealerRule(ctx, arg)
	if err != nil {
		return RuleView{}, mapPG(err)
	}
	return ruleView(row), nil
}

func (s *Service) DeleteRule(ctx context.Context, c Caller, id uuid.UUID) error {
	cur, err := s.q.GetWeakDealerRule(ctx, db.GetWeakDealerRuleParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if cur.OrganizationID != c.Org.InternalID {
		return ErrNotFound
	}
	n, err := s.q.DeleteWeakDealerRule(ctx, db.DeleteWeakDealerRuleParams{ID: cur.ID, BrandID: c.Org.BrandID})
	if err == nil && n > 0 {
		return nil
	}
	_, err = s.q.UpdateWeakDealerRule(ctx, db.UpdateWeakDealerRuleParams{ID: cur.ID, BrandID: c.Org.BrandID, Name: cur.Name, Metric: cur.Metric, Operator: cur.Operator, Threshold: cur.Threshold, CreateTask: cur.CreateTask, Notify: cur.Notify, AssigneeUserID: cur.AssigneeUserID, Active: false})
	return err
}

func (s *Service) RunRules(ctx context.Context, brandID int64, ownerOrgIDs []int64, period string) (RuleRunResult, error) {
	if period == "" {
		period = monthParam("", s.now())
	}
	rows, err := s.q.ListPerformanceRuleEvaluations(ctx, db.ListPerformanceRuleEvaluationsParams{BrandID: brandID, OwnerOrgIds: ownerOrgIDs, Period: period})
	if err != nil {
		return RuleRunResult{}, err
	}
	center, err := s.q.GetBrandCenterOrganization(ctx, brandID)
	if err != nil {
		return RuleRunResult{}, err
	}
	res := RuleRunResult{Period: period, Evaluated: len(rows)}
	for _, r := range rows {
		if !ruleMatches(r) {
			continue
		}
		res.Matched++
		if r.CreateTask && r.RuleOwnerType == "center" {
			created, err := s.createAutoTask(ctx, center, r, period)
			if err != nil {
				return res, err
			}
			if created {
				res.TasksCreated++
			}
		}
		if r.Notify {
			n, err := s.notifyRule(ctx, center, r, period)
			if err != nil {
				return res, err
			}
			res.Notifications += n
		}
	}
	return res, nil
}

func (s *Service) createAutoTask(ctx context.Context, center db.Organization, r db.ListPerformanceRuleEvaluationsRow, period string) (bool, error) {
	assignee := r.AssigneeUserID
	if !assignee.Valid {
		owners, err := s.q.ListOrganizationOwnerUserIDs(ctx, center.ID)
		if err != nil {
			return false, err
		}
		if len(owners) > 0 {
			assignee = pgtype.Int8{Int64: owners[0], Valid: true}
		}
	}
	title := "Zayıf bayi takibi: " + r.DealerName
	desc := fmt.Sprintf("Kural: %s\nDönem: %s\nMetrik: %s = %s\nEşik: %s %s\nPanel: %s", r.RuleName, period, r.Metric, numText(r.MetricValue), r.Operator, numText(r.Threshold), s.performanceLink(r.DealerUuid, period))
	_, err := s.q.InsertAutoPerformanceTask(ctx, db.InsertAutoPerformanceTaskParams{OrganizationID: center.ID, BrandID: center.BrandID, SubjectOrgID: r.DealerOrgID, Title: title, Description: desc, AssigneeUserID: assignee, Priority: tasksuc.PriorityHigh, AutoRuleID: pgtype.Int8{Int64: r.RuleID, Valid: true}, AutoPeriod: pgtype.Text{String: period, Valid: true}})
	if err == nil {
		return true, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return false, nil
	}
	return false, err
}

func (s *Service) notifyRule(ctx context.Context, center db.Organization, r db.ListPerformanceRuleEvaluationsRow, period string) (int, error) {
	if s.out == nil {
		return 0, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	centerOwners, err := s.q.ListOrganizationOwnerUserIDs(ctx, center.ID)
	if err != nil {
		return 0, err
	}
	var distributorOwners []int64
	if r.DistributorOrgID.Valid {
		distributorOwners, err = s.q.ListOrganizationOwnerUserIDs(ctx, r.DistributorOrgID.Int64)
		if err != nil {
			return 0, err
		}
	}
	dealerOwners, err := s.q.ListOrganizationOwnerUserIDs(ctx, r.DealerOrgID)
	if err != nil {
		return 0, err
	}
	payload := map[string]any{
		"brand_id": r.BrandID, "rule_uuid": r.RuleUuid.String(), "rule_name": r.RuleName,
		"dealer_uuid": r.DealerUuid.String(), "dealer_name": r.DealerName, "period": period,
		"metric": r.Metric, "metric_value": numText(r.MetricValue), "threshold": numText(r.Threshold),
		"panel_url": s.performanceLink(r.DealerUuid, period),
	}
	if len(centerOwners)+len(distributorOwners) > 0 {
		payload["notify_user_ids"] = append(centerOwners, distributorOwners...)
		ev := events.New(EventPerformanceWeakDealer).WithTenant(r.RuleOwnerOrgID).WithPayload(payload)
		if err := s.out.Enqueue(ctx, tx, ev); err != nil {
			return 0, err
		}
	}
	if len(dealerOwners) > 0 {
		payload2 := clonePayload(payload)
		payload2["notify_user_ids"] = dealerOwners
		ev := events.New(EventPerformanceBelowTarget).WithTenant(r.DealerOrgID).WithPayload(payload2)
		if err := s.out.Enqueue(ctx, tx, ev); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(centerOwners) + len(distributorOwners) + len(dealerOwners), nil
}

func (s *Service) performanceLink(dealer uuid.UUID, period string) string {
	if s.panelURL == "" {
		return "/panel/performance?dealer=" + dealer.String() + "&period=" + period
	}
	return s.panelURL + "/panel/performance?dealer=" + dealer.String() + "&period=" + period
}

func (s *Service) readOrgIDs(ctx context.Context, c Caller) ([]int64, error) {
	switch c.Org.OrgType {
	case "center":
		return c.Filter.OrgIDsArg(), nil
	case "distributor":
		rows, err := s.q.Descendants(ctx, c.Org.InternalID)
		if err != nil {
			return nil, err
		}
		ids := make([]int64, 0, len(rows))
		for _, o := range rows {
			ids = append(ids, o.ID)
		}
		return ids, nil
	case "dealer":
		return []int64{c.Org.InternalID}, nil
	default:
		return nil, ErrForbidden
	}
}

func (s *Service) targetScopeIDs(ctx context.Context, c Caller) ([]int64, []int64, error) {
	switch c.Org.OrgType {
	case "center":
		return []int64{c.Org.InternalID}, nil, nil
	case "distributor":
		desc, err := s.q.Descendants(ctx, c.Org.InternalID)
		if err != nil {
			return nil, nil, err
		}
		targets := make([]int64, 0, len(desc))
		for _, o := range desc {
			if o.Type == "dealer" {
				targets = append(targets, o.ID)
			}
		}
		return []int64{c.Org.InternalID}, targets, nil
	case "dealer":
		return nil, []int64{c.Org.InternalID}, nil
	default:
		return nil, nil, ErrForbidden
	}
}

func (s *Service) ruleOwnerIDs(ctx context.Context, c Caller) ([]int64, error) {
	if c.Org.OrgType == "center" || c.Org.OrgType == "distributor" {
		return []int64{c.Org.InternalID}, nil
	}
	return nil, ErrForbidden
}

func (s *Service) validTargetOrg(ctx context.Context, c Caller, id uuid.UUID) (db.Organization, error) {
	if c.Org.OrgType != "center" && c.Org.OrgType != "distributor" {
		return db.Organization{}, ErrForbidden
	}
	target, err := s.q.GetOrganizationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrNotFound
	}
	if err != nil {
		return db.Organization{}, err
	}
	if target.BrandID != c.Org.BrandID || (target.Type != "distributor" && target.Type != "dealer") {
		return db.Organization{}, ErrNotFound
	}
	if c.Org.OrgType == "distributor" && (target.Type != "dealer" || !target.ParentID.Valid || target.ParentID.Int64 != c.Org.InternalID) {
		return db.Organization{}, ErrNotFound
	}
	return target, nil
}

func (s *Service) canOwn(ctx context.Context, c Caller, ownerID, targetID int64) error {
	if ownerID != c.Org.InternalID {
		return ErrNotFound
	}
	if c.Org.OrgType == "distributor" {
		target, err := s.q.GetOrganizationByID(ctx, targetID)
		if err != nil {
			return ErrNotFound
		}
		if !target.ParentID.Valid || target.ParentID.Int64 != c.Org.InternalID {
			return ErrNotFound
		}
	}
	return nil
}

func targetInput(c Caller, targetID int64, in TargetInput) (db.CreatePerformanceTargetParams, error) {
	if !contains(model.TargetMetrics, in.Metric) {
		return db.CreatePerformanceTargetParams{}, invalid("metric", "invalid")
	}
	if !contains(model.PeriodKinds, in.PeriodKind) {
		return db.CreatePerformanceTargetParams{}, invalid("period_kind", "invalid")
	}
	val, err := numeric(in.Value)
	if err != nil {
		return db.CreatePerformanceTargetParams{}, invalid("value", "must be a positive decimal")
	}
	start, err := time.Parse(time.DateOnly, in.PeriodStart)
	if err != nil {
		return db.CreatePerformanceTargetParams{}, invalid("period_start", "must be YYYY-MM-DD")
	}
	return db.CreatePerformanceTargetParams{OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, TargetOrgID: targetID, Metric: in.Metric, PeriodKind: in.PeriodKind, PeriodStart: pgtype.Date{Time: start, Valid: true}, Value: val, Currency: currencyArg(in.Currency), ContractRef: dateStringArg(in.ContractRef), Note: textPtrArg(in.Note), CreatedByUserID: userArg(c)}, nil
}

func ruleInput(c Caller, in RuleInput) (db.CreateWeakDealerRuleParams, error) {
	up, err := ruleUpdateInput(0, c.Org.BrandID, in)
	if err != nil {
		return db.CreateWeakDealerRuleParams{}, err
	}
	return db.CreateWeakDealerRuleParams{OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, Name: up.Name, Metric: up.Metric, Operator: up.Operator, Threshold: up.Threshold, CreateTask: up.CreateTask, Notify: up.Notify, AssigneeUserID: up.AssigneeUserID, Active: up.Active, CreatedByUserID: userArg(c)}, nil
}

func ruleUpdateInput(id, brandID int64, in RuleInput) (db.UpdateWeakDealerRuleParams, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return db.UpdateWeakDealerRuleParams{}, invalid("name", "is required")
	}
	if !contains(model.RuleMetrics(), in.Metric) {
		return db.UpdateWeakDealerRuleParams{}, invalid("metric", "invalid")
	}
	if !contains(model.RuleOperators, in.Operator) {
		return db.UpdateWeakDealerRuleParams{}, invalid("operator", "invalid")
	}
	if !in.CreateTask && !in.Notify {
		return db.UpdateWeakDealerRuleParams{}, invalid("action", "create_task or notify is required")
	}
	thr, err := numeric(in.Threshold)
	if err != nil {
		return db.UpdateWeakDealerRuleParams{}, invalid("threshold", "must be a decimal")
	}
	return db.UpdateWeakDealerRuleParams{ID: id, BrandID: brandID, Name: name, Metric: in.Metric, Operator: in.Operator, Threshold: thr, CreateTask: in.CreateTask, Notify: in.Notify, AssigneeUserID: int8Ptr(in.AssigneeUserID), Active: in.Active}, nil
}

func ruleMatches(r db.ListPerformanceRuleEvaluationsRow) bool {
	v, threshold := numFloat(r.MetricValue), numFloat(r.Threshold)
	switch r.Operator {
	case model.OpLT:
		return v < threshold
	case model.OpLTE:
		return v <= threshold
	case model.OpGT:
		return v > threshold
	case model.OpGTE:
		return v >= threshold
	case model.OpBelowMedianPct:
		m := numFloat(r.MedianValue)
		return m > 0 && v <= m*(1-threshold/100)
	default:
		return false
	}
}

func monthParam(raw string, now time.Time) string {
	raw = strings.TrimSpace(raw)
	if len(raw) == 7 {
		return raw
	}
	if now.IsZero() {
		now = time.Now()
	}
	return now.Format("2006-01")
}

func addMonths(period string, n int) string {
	t, err := time.Parse("2006-01-02", period+"-01")
	if err != nil {
		return period
	}
	return t.AddDate(0, n, 0).Format("2006-01")
}

func metricMap(rows []db.PerformanceMetricsMonthly, period string) map[string]*MetricValue {
	out := map[string]*MetricValue{}
	for _, r := range rows {
		if r.Period == period {
			out[r.Metric] = metricValue(r.Value, r.Currency, r.ComputedAt)
		}
	}
	return out
}

func metricValue(v pgtype.Numeric, c pgtype.Text, t pgtype.Timestamptz) *MetricValue {
	return &MetricValue{Value: numText(v), Currency: textPtr(c), ComputedAt: timePtr(t)}
}

func rankingMetrics(r db.ListPerformanceRankingRow) map[string]*MetricValue {
	return map[string]*MetricValue{
		model.MetricServicesCount:       metricValue(r.ServicesCount, pgtype.Text{}, r.ComputedAt),
		model.MetricWarrantyStartRate:   metricValue(r.WarrantyStartRate, pgtype.Text{}, r.ComputedAt),
		model.MetricMeasurementRate:     metricValue(r.MeasurementRate, pgtype.Text{}, r.ComputedAt),
		model.MetricReviewAvg:           metricValue(r.ReviewAvg, pgtype.Text{}, r.ComputedAt),
		model.MetricStockTurnover:       metricValue(r.StockTurnover, pgtype.Text{}, r.ComputedAt),
		model.MetricContractDaysLeft:    metricValue(r.ContractDaysLeft, pgtype.Text{}, r.ComputedAt),
		model.MetricCariOverdueAmount:   metricValue(r.CariOverdueAmount, pgtype.Text{String: r.Currency, Valid: r.CariOverdueAmount.Valid}, r.ComputedAt),
		model.MetricCariOverdueDays:     metricValue(r.CariOverdueDays, pgtype.Text{}, r.ComputedAt),
		model.MetricCertificateCoverage: metricValue(r.CertificateCoverage, pgtype.Text{}, r.ComputedAt),
		model.MetricLeadConversionRate:  metricValue(r.LeadConversionRate, pgtype.Text{}, r.ComputedAt),
		model.MetricWasteRatio:          metricValue(r.WasteRatio, pgtype.Text{}, r.ComputedAt),
		model.MetricOrderVolume:         metricValue(r.OrderVolume, pgtype.Text{String: r.Currency, Valid: r.OrderVolume.Valid}, r.ComputedAt),
	}
}

func averageMetrics(rows []db.ListPerformanceRankingRow) map[string]*MetricValue {
	sums := map[string]float64{}
	counts := map[string]int{}
	for _, r := range rows {
		for k, v := range rankingMetrics(r) {
			if v == nil || v.Value == "" {
				continue
			}
			f, _ := strconv.ParseFloat(v.Value, 64)
			sums[k] += f
			counts[k]++
		}
	}
	out := map[string]*MetricValue{}
	for k, sum := range sums {
		out[k] = &MetricValue{Value: fmt.Sprintf("%.2f", sum/float64(counts[k]))}
	}
	return out
}

func targetRowView(r db.ListPerformanceTargetsRow) TargetView {
	return TargetView{UUID: r.Uuid, TargetOrganizationUUID: r.TargetOrgUuid, TargetName: r.TargetName, TargetType: r.TargetType, Metric: r.Metric, PeriodKind: r.PeriodKind, PeriodStart: dateText(r.PeriodStart), PeriodEnd: dateText(r.PeriodEnd), Value: numText(r.Value), Currency: textPtr(r.Currency), ContractRef: datePtrText(r.ContractRef), Note: textPtr(r.Note), Actual: numPtr(r.Actual), AchievementPct: numPtr(r.AchievementPct), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}

func (s *Service) targetByIDView(ctx context.Context, c Caller, id uuid.UUID) (TargetView, error) {
	rows, _, err := s.ListTargets(ctx, c, TargetFilter{Limit: 100, Offset: 0})
	if err != nil {
		return TargetView{}, err
	}
	for _, r := range rows {
		if r.UUID == id {
			return r, nil
		}
	}
	return TargetView{}, ErrNotFound
}

func ruleRowView(r db.ListWeakDealerRulesRow) RuleView {
	v := RuleView{UUID: r.Uuid, OwnerName: r.OwnerName, OwnerType: r.OwnerType, Name: r.Name, Metric: r.Metric, Operator: r.Operator, Threshold: numText(r.Threshold), CreateTask: r.CreateTask, Notify: r.Notify, AssigneeUserID: int64Ptr(r.AssigneeUserID), Active: r.Active, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	return v
}

func ruleView(r db.WeakDealerRule) RuleView {
	return RuleView{UUID: r.Uuid, Name: r.Name, Metric: r.Metric, Operator: r.Operator, Threshold: numText(r.Threshold), CreateTask: r.CreateTask, Notify: r.Notify, AssigneeUserID: int64Ptr(r.AssigneeUserID), Active: r.Active, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}

func bonusAccrualRowView(r db.ListBonusAccrualsRow) BonusAccrualView {
	return BonusAccrualView{
		UUID:             r.Uuid,
		UserID:           r.UserID,
		UserName:         strings.TrimSpace(r.UserName + " " + r.UserSurname),
		Period:           r.Period,
		RuleName:         r.RuleName,
		AchievementPct:   numText(r.AchievementPct),
		Amount:           numText(r.Amount),
		Currency:         r.Currency,
		Status:           r.Status,
		StaffPaymentID:   int64Ptr(r.StaffPaymentID),
		ApprovedByUserID: int64Ptr(r.ApprovedByUserID),
		ApprovedAt:       timePtr(r.ApprovedAt),
		CancelledAt:      timePtr(r.CancelledAt),
		CreatedAt:        r.CreatedAt.Time,
		UpdatedAt:        r.UpdatedAt.Time,
	}
}

func totalBonuses(rows []db.ListBonusAccrualsRow) int64 {
	if len(rows) == 0 {
		return 0
	}
	return rows[0].TotalCount
}

func (s *Service) bonusByIDView(ctx context.Context, c Caller, id uuid.UUID) (BonusAccrualView, error) {
	rows, err := s.q.ListBonusAccruals(ctx, db.ListBonusAccrualsParams{
		OrganizationID: c.Org.InternalID,
		Statuses:       model.AccrualStatuses,
		RowLimit:       1000,
	})
	if err != nil {
		return BonusAccrualView{}, err
	}
	for _, r := range rows {
		if r.Uuid == id {
			return bonusAccrualRowView(r), nil
		}
	}
	return BonusAccrualView{}, ErrNotFound
}

func diff(a, b *MetricValue) *string {
	if a == nil || b == nil {
		return nil
	}
	v := numString(numFloatString(a.Value) - numFloatString(b.Value))
	return &v
}

func diffPct(a, b *MetricValue) *string {
	if a == nil || b == nil {
		return nil
	}
	prev := numFloatString(b.Value)
	if prev == 0 {
		return nil
	}
	v := numString((numFloatString(a.Value) - prev) / math.Abs(prev) * 100)
	return &v
}

func totalRanking(rows []db.ListPerformanceRankingRow) int64 {
	if len(rows) == 0 {
		return 0
	}
	return rows[0].TotalCount
}
func totalTargets(rows []db.ListPerformanceTargetsRow) int64 {
	if len(rows) == 0 {
		return 0
	}
	return rows[0].TotalCount
}

func textArg(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}
func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}
func textPtrArg(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return textArg(*s)
}
func currencyArg(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.ToUpper(strings.TrimSpace(*s)), Valid: strings.TrimSpace(*s) != ""}
}
func userArg(c Caller) pgtype.Int8 {
	return pgtype.Int8{Int64: c.Principal.UserInternal, Valid: c.Principal.UserInternal != 0}
}
func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
func int64Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}
func datePtr(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}
func dateStringArg(s *string) pgtype.Date {
	if s == nil || strings.TrimSpace(*s) == "" {
		return pgtype.Date{}
	}
	t, _ := time.Parse(time.DateOnly, *s)
	return pgtype.Date{Time: t, Valid: true}
}
func dateText(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format(time.DateOnly)
}
func datePtrText(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format(time.DateOnly)
	return &s
}
func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
func numText(n pgtype.Numeric) string {
	if !n.Valid {
		return ""
	}
	return posting.FormatNumeric(n)
}
func numPtr(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	s := numText(n)
	return &s
}
func numFloat(n pgtype.Numeric) float64 { f, _ := strconv.ParseFloat(numText(n), 64); return f }
func numFloatString(s string) float64   { f, _ := strconv.ParseFloat(s, 64); return f }
func numString(v float64) string        { return fmt.Sprintf("%.2f", v) }

func numeric(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(strings.TrimSpace(s)); err != nil || !n.Valid || numFloat(n) <= 0 {
		return n, errors.New("invalid numeric")
	}
	return n, nil
}

func pct(actual, target pgtype.Numeric) pgtype.Numeric {
	v := 0.0
	if t := numFloat(target); t > 0 {
		v = numFloat(actual) / t * 100
	}
	n, _ := numeric(fmt.Sprintf("%.2f", v))
	if !n.Valid {
		_ = n.Scan("0")
	}
	return n
}

func bonusAmount(currency string, r db.ListBonusCalculationCandidatesRow) (pgtype.Numeric, string, bool) {
	switch r.Kind {
	case model.BonusFixed:
		if !r.Amount.Valid || !r.Currency.Valid || numFloat(r.Amount) <= 0 {
			return pgtype.Numeric{}, "", false
		}
		return r.Amount, r.Currency.String, true
	case model.BonusPercentOfRevenue:
		amount := numFloat(r.ActualRevenue) * numFloat(r.Percent) / 100
		if amount <= 0 {
			return pgtype.Numeric{}, "", false
		}
		n, err := numeric(fmt.Sprintf("%.2f", amount))
		return n, currency, err == nil
	default:
		return pgtype.Numeric{}, "", false
	}
}

func periodBounds(period string) (time.Time, time.Time, error) {
	period = strings.TrimSpace(period)
	if len(period) != 7 {
		return time.Time{}, time.Time{}, invalid("period", "must be YYYY-MM")
	}
	start, err := time.Parse("2006-01-02", period+"-01")
	if err != nil {
		return time.Time{}, time.Time{}, invalid("period", "must be YYYY-MM")
	}
	return start, start.AddDate(0, 1, 0), nil
}

func (s *Service) ensureBonusFeatures(ctx context.Context, orgID int64, strict bool) error {
	on, err := s.featureOn(ctx, orgID, features.ModulePerformance)
	if err != nil {
		return err
	}
	if !on {
		return ErrForbidden
	}
	on, err = s.featureOn(ctx, orgID, features.ModuleDealerAccounting)
	if err != nil {
		return err
	}
	if !on {
		return ErrForbidden
	}
	_ = strict
	return nil
}

func (s *Service) bonusPayoutDate(ctx context.Context, q *db.Queries, org db.Organization, period string) (time.Time, error) {
	day := int16(5)
	settings, err := q.GetBonusSettings(ctx, org.ID)
	if err == nil {
		day = settings.PayoutDay
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, err
	}
	start, _, err := periodBounds(period)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(start.Year(), start.Month()+1, int(day), 0, 0, 0, 0, time.UTC), nil
}

func bookToday(org db.Organization, now time.Time) time.Time {
	loc := loadLocation(org.Timezone)
	return dayStart(now.In(loc))
}

func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func ledgerTime(paidOn, now time.Time) time.Time {
	day := dayStart(paidOn)
	if dayStart(now.UTC()).Equal(day) {
		return now
	}
	return day
}

func (s *Service) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) postBonusStaffPayment(ctx context.Context, tx pgx.Tx, p db.StaffPayment, actor pgtype.Int8) error {
	if s.poster == nil {
		return errors.New("performance: accounting poster not configured")
	}
	var actorPtr *int64
	if actor.Valid {
		actorPtr = &actor.Int64
	}
	desc := ""
	if p.Description.Valid {
		desc = p.Description.String
	}
	res, err := s.poster.PostExpense(ctx, tx, posting.Entry{
		OrganizationID: p.OrganizationID,
		Source:         posting.Source{Type: "staff_payment", UUID: p.Uuid},
		Category:       accountingmodel.CategoryStaffBonus,
		Amount:         posting.FormatNumeric(p.Amount),
		Currency:       p.Currency,
		AllowNoTarget:  true,
		Description:    desc,
		ActorUserID:    actorPtr,
		PostedAt:       ledgerTime(p.PaidOn.Time, s.now()),
	})
	if err != nil {
		return err
	}
	if _, err := s.q.WithTx(tx).SetStaffPaymentFinanceEntry(ctx, db.SetStaffPaymentFinanceEntryParams{
		FinanceEntryID: pgtype.Int8{Int64: res.Entry.ID, Valid: true},
		ID:             p.ID,
		OrganizationID: p.OrganizationID,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) notifyBonusesCalculated(ctx context.Context, org db.Organization, period string, accruals []db.BonusAccrual) (int, error) {
	if s.out == nil || len(accruals) == 0 {
		return 0, nil
	}
	owners, err := s.q.ListOrganizationOwnerUserIDs(ctx, org.ID)
	if err != nil || len(owners) == 0 {
		return 0, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	payload := map[string]any{
		"brand_id": org.BrandID, "period": period, "count": strconv.Itoa(len(accruals)),
		"notify_user_ids": owners,
	}
	if err := s.out.Enqueue(ctx, tx, events.New(EventBonusCalculated).WithTenant(org.ID).WithPayload(payload)); err != nil {
		return 0, err
	}
	return len(owners), tx.Commit(ctx)
}

func (s *Service) notifyBonusApproved(ctx context.Context, tx pgx.Tx, org db.Organization, accrual db.BonusAccrual, payment db.StaffPayment) error {
	if s.out == nil {
		return nil
	}
	payload := map[string]any{
		"brand_id": org.BrandID, "period": accrual.Period, "amount": numText(payment.Amount),
		"currency": payment.Currency, "paid_on": payment.PaidOn.Time.Format(time.DateOnly),
		"status": payment.Status, "notify_user_ids": []int64{accrual.UserID},
	}
	return s.out.Enqueue(ctx, tx, events.New(EventBonusApproved).WithTenant(org.ID).WithEntity("staff_payment", &payment.ID, &payment.Uuid).WithPayload(payload))
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func mapPG(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return invalid("unique", "already exists")
		case "23514", "23503":
			return invalid("constraint", "invalid value")
		}
	}
	return err
}

func clonePayload(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
