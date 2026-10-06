package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-207 (F1-03g): end-of-day reports. A report summarizes the ledger
// movements (stock_movements) of one local day of an organization: per
// warehouse, or for the whole system (warehouse NULL: every warehouse plus
// the organization-level movements). Stock entries (TEC-204), transfers,
// order shipments / receipts, service consumption, returns, count
// adjustments and disposals all come from the same ledger, grouped by
// movement type and product.
//
// The day is the organization's calendar day in its time zone (K10). The
// hourly cron writes the previous local day of every center / distributor
// once (kind auto); POST /v1/warehouse/eod-reports regenerates a day on
// demand (kind manual). The PDF is an export job rendered on worker-docs
// from the stored summary (eod_pdf.go).

// Report kinds (eod_reports.kind).
const (
	EODKindAuto   = "auto"
	EODKindManual = "manual"
)

// List scopes.
const (
	EODScopeSystem    = "system"
	EODScopeWarehouse = "warehouse"
)

// ErrEODReportNotFound: no report with that uuid in the organization.
var ErrEODReportNotFound = errors.New("warehouse: end-of-day report not found")

// EOD movement groups: every ledger movement type falls in exactly one.
const (
	EODGroupEntry       = "entry"
	EODGroupPlacement   = "placement"
	EODGroupTransfer    = "transfer"
	EODGroupOrder       = "order"
	EODGroupConsumption = "consumption"
	EODGroupReturn      = "return"
	EODGroupAdjustment  = "adjustment"
	EODGroupDisposal    = "disposal"
)

// EODGroups lists the groups in report order.
var EODGroups = []string{
	EODGroupEntry, EODGroupPlacement, EODGroupTransfer, EODGroupOrder,
	EODGroupConsumption, EODGroupReturn, EODGroupAdjustment, EODGroupDisposal,
}

var eodGroupOf = map[string]string{
	"entry":                   EODGroupEntry,
	"placement":               EODGroupPlacement,
	"transfer_out":            EODGroupTransfer,
	"transfer_in":             EODGroupTransfer,
	"transfer_cancel_restore": EODGroupTransfer,
	"order_out":               EODGroupOrder,
	"received":                EODGroupOrder,
	"order_cancel_restore":    EODGroupOrder,
	"consumption":             EODGroupConsumption,
	"partial_consumption":     EODGroupConsumption,
	"sale":                    EODGroupConsumption,
	"return":                  EODGroupReturn,
	"count_adjustment":        EODGroupAdjustment,
	"reclassification":        EODGroupAdjustment,
	"void":                    EODGroupDisposal,
	"external_outbound":       EODGroupDisposal,
}

// EODGroupOf returns the group of a movement type (unknown types fall in
// adjustment, so a new ledger type is never dropped from the totals).
func EODGroupOf(movementType string) string {
	if g, ok := eodGroupOf[movementType]; ok {
		return g
	}
	return EODGroupAdjustment
}

// EODTotals is one summary line: movements, distinct units, counted
// quantity in / out (serial units: on-hand delta; fixed barcodes: the
// quantity) and roll meters in / out ("12.50").
type EODTotals struct {
	MovementCount int64  `json:"movement_count"`
	UnitCount     int64  `json:"unit_count"`
	QuantityIn    int64  `json:"quantity_in"`
	QuantityOut   int64  `json:"quantity_out"`
	MetersIn      string `json:"meters_in"`
	MetersOut     string `json:"meters_out"`
}

// EODGroupTotal is the total of one movement group.
type EODGroupTotal struct {
	Group string `json:"group"`
	EODTotals
}

// EODTypeTotal is the total of one movement type.
type EODTypeTotal struct {
	Type  string `json:"type"`
	Group string `json:"group"`
	EODTotals
}

// EODProductLine is one (movement type, product) line.
type EODProductLine struct {
	Type        string    `json:"type"`
	Group       string    `json:"group"`
	ProductUUID uuid.UUID `json:"product_uuid"`
	SKU         string    `json:"sku"`
	ProductName string    `json:"product_name"`
	EODTotals
}

// EODSummary is the stored report body (eod_reports.summary).
type EODSummary struct {
	Totals   EODTotals        `json:"totals"`
	Groups   []EODGroupTotal  `json:"groups"`
	Types    []EODTypeTotal   `json:"types"`
	Products []EODProductLine `json:"products"`
}

// EODWarehouseRef names the report's warehouse.
type EODWarehouseRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// EODReport is the API view of a stored report. Warehouse nil: system
// report.
type EODReport struct {
	UUID        uuid.UUID        `json:"uuid"`
	ReportDate  string           `json:"report_date"`
	Timezone    string           `json:"timezone"`
	PeriodStart time.Time        `json:"period_start"`
	PeriodEnd   time.Time        `json:"period_end"`
	Kind        string           `json:"kind"`
	Warehouse   *EODWarehouseRef `json:"warehouse"`
	Summary     EODSummary       `json:"summary"`
	GeneratedAt time.Time        `json:"generated_at"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// EODCaller is the request principal (generated_by of a manual report).
type EODCaller struct {
	Caller
	Principal authctx.Principal
}

// EODGenerateInput is a manual run. Date (YYYY-MM-DD, the organization's
// calendar day) defaults to today; WarehouseUUID nil is the system report.
type EODGenerateInput struct {
	Date          string
	WarehouseUUID *string
}

// EODListInput filters the stored reports.
type EODListInput struct {
	WarehouseUUID *string
	Scope         string
	DateFrom      string
	DateTo        string
	// TEC-375: kind filter (auto, manual) and sort (EODSort; zero value
	// means the default -report_date).
	Kinds  []string
	Sort   apiquery.ResolvedSort
	Limit  int32
	Offset int32
}

// EOD implements the end-of-day report use cases.
type EOD struct {
	pool TxBeginner
	q    *db.Queries
	now  func() time.Time
}

// NewEOD builds the end-of-day report use case.
func NewEOD(pool TxBeginner, q *db.Queries) *EOD {
	return &EOD{pool: pool, q: q, now: func() time.Time { return time.Now().UTC() }}
}

// SetClock overrides the clock (tests).
func (s *EOD) SetClock(now func() time.Time) { s.now = now }

const eodDateLayout = "2006-01-02"

// zoneOf loads an organization time zone (UTC when empty or unknown).
func zoneOf(tz string) (*time.Location, string) {
	if tz = strings.TrimSpace(tz); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc, tz
		}
	}
	return time.UTC, "UTC"
}

// EODDay returns the [start, end) instants of a calendar day in loc.
func EODDay(day time.Time, loc *time.Location) (time.Time, time.Time) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 0, 1)
}

func pgDate(day time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func parseDay(field, raw string) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	d, err := time.Parse(eodDateLayout, raw)
	if err != nil {
		return time.Time{}, false, invalid(field, "must be a date (YYYY-MM-DD)")
	}
	return d, true, nil
}

// Summarize builds the summary of the organization's movements in
// [start, end); warehouseID 0 is the system report.
func (s *EOD) Summarize(ctx context.Context, orgID, warehouseID int64, start, end time.Time) (EODSummary, error) {
	arg := db.SummarizeEODMovementsParams{
		OrganizationID: orgID,
		PeriodStart:    pgtype.Timestamptz{Time: start, Valid: true},
		PeriodEnd:      pgtype.Timestamptz{Time: end, Valid: true},
	}
	if warehouseID > 0 {
		arg.WarehouseID = pgtype.Int8{Int64: warehouseID, Valid: true}
	}
	rows, err := s.q.SummarizeEODMovements(ctx, arg)
	if err != nil {
		return EODSummary{}, fmt.Errorf("warehouse eod: summarize: %w", err)
	}
	return BuildEODSummary(rows)
}

// eodAcc accumulates totals; meters are kept in hundredths.
type eodAcc struct {
	t       EODTotals
	cmIn    int64
	cmOut   int64
	present bool
}

func (a *eodAcc) add(movements, units, qIn, qOut, cmIn, cmOut int64) {
	a.present = true
	a.t.MovementCount += movements
	a.t.UnitCount += units
	a.t.QuantityIn += qIn
	a.t.QuantityOut += qOut
	a.cmIn += cmIn
	a.cmOut += cmOut
}

func (a *eodAcc) totals() EODTotals {
	t := a.t
	t.MetersIn, t.MetersOut = formatCm(a.cmIn), formatCm(a.cmOut)
	return t
}

// BuildEODSummary folds the grouped query rows into the summary. Units
// add up within a type (a unit has one product); across types a unit
// moved twice counts twice.
func BuildEODSummary(rows []db.SummarizeEODMovementsRow) (EODSummary, error) {
	var total eodAcc
	groups := map[string]*eodAcc{}
	types := map[string]*eodAcc{}
	var typeOrder []string
	out := EODSummary{Groups: []EODGroupTotal{}, Types: []EODTypeTotal{}, Products: make([]EODProductLine, 0, len(rows))}
	for _, r := range rows {
		cmIn, err := parseCm(r.MetersIn)
		if err != nil {
			return EODSummary{}, err
		}
		cmOut, err := parseCm(r.MetersOut)
		if err != nil {
			return EODSummary{}, err
		}
		g := EODGroupOf(r.Type)
		if groups[g] == nil {
			groups[g] = &eodAcc{}
		}
		if types[r.Type] == nil {
			types[r.Type] = &eodAcc{}
			typeOrder = append(typeOrder, r.Type)
		}
		for _, a := range []*eodAcc{&total, groups[g], types[r.Type]} {
			a.add(r.MovementCount, r.UnitCount, r.QuantityIn, r.QuantityOut, cmIn, cmOut)
		}
		line := eodAcc{}
		line.add(r.MovementCount, r.UnitCount, r.QuantityIn, r.QuantityOut, cmIn, cmOut)
		out.Products = append(out.Products, EODProductLine{
			Type: r.Type, Group: g, ProductUUID: r.ProductUuid, SKU: r.Sku, ProductName: r.ProductName,
			EODTotals: line.totals(),
		})
	}
	out.Totals = total.totals()
	for _, g := range EODGroups {
		a := groups[g]
		if a == nil {
			a = &eodAcc{}
		}
		out.Groups = append(out.Groups, EODGroupTotal{Group: g, EODTotals: a.totals()})
	}
	for _, t := range typeOrder {
		out.Types = append(out.Types, EODTypeTotal{Type: t, Group: EODGroupOf(t), EODTotals: types[t].totals()})
	}
	return out, nil
}

// parseCm parses a NUMERIC(14,2) text ("12.50", "-0.25", "3") into
// hundredths.
func parseCm(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	neg := strings.HasPrefix(raw, "-")
	raw = strings.TrimPrefix(raw, "-")
	whole, frac, _ := strings.Cut(raw, ".")
	if len(frac) > 2 {
		frac = frac[:2]
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("warehouse eod: meters %q: %w", raw, err)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("warehouse eod: meters %q: %w", raw, err)
	}
	v := w*100 + f
	if neg {
		v = -v
	}
	return v, nil
}

func formatCm(cm int64) string {
	sign := ""
	if cm < 0 {
		sign, cm = "-", -cm
	}
	return fmt.Sprintf("%s%d.%02d", sign, cm/100, cm%100)
}

// view maps a stored report.
func (s *EOD) view(ctx context.Context, q *db.Queries, r db.EodReport) (EODReport, error) {
	v := EODReport{
		UUID: r.Uuid, ReportDate: r.ReportDate.Time.Format(eodDateLayout), Timezone: r.Timezone,
		PeriodStart: r.PeriodStart.Time, PeriodEnd: r.PeriodEnd.Time, Kind: r.Kind,
		GeneratedAt: r.GeneratedAt.Time, CreatedAt: ts(r.CreatedAt), UpdatedAt: ts(r.UpdatedAt),
	}
	if err := json.Unmarshal(r.Summary, &v.Summary); err != nil {
		return EODReport{}, fmt.Errorf("warehouse eod: summary: %w", err)
	}
	if r.WarehouseID.Valid {
		w, err := q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: r.WarehouseID.Int64, OrganizationID: r.OrganizationID})
		if err != nil {
			return EODReport{}, fmt.Errorf("warehouse eod: warehouse: %w", err)
		}
		v.Warehouse = &EODWarehouseRef{UUID: w.Uuid, Code: w.Code, Name: w.Name}
	}
	return v, nil
}

// warehouseID resolves an optional warehouse uuid of the organization (0:
// system report).
func (s *EOD) warehouseID(ctx context.Context, org int64, raw *string) (int64, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return 0, nil
	}
	id, err := uuid.Parse(strings.TrimSpace(*raw))
	if err != nil {
		return 0, invalid("warehouse_uuid", "must be a UUID")
	}
	w, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return 0, notFound(err, ErrWarehouseNotFound)
	}
	return w.ID, nil
}

type eodWrite struct {
	org         db.Organization
	warehouseID int64
	day         time.Time
	kind        string
	actor       pgtype.Int8
}

func (s *EOD) params(ctx context.Context, w eodWrite) (db.UpsertEODReportParams, error) {
	loc, tz := zoneOf(w.org.Timezone)
	start, end := EODDay(w.day, loc)
	sum, err := s.Summarize(ctx, w.org.ID, w.warehouseID, start, end)
	if err != nil {
		return db.UpsertEODReportParams{}, err
	}
	body, err := json.Marshal(sum)
	if err != nil {
		return db.UpsertEODReportParams{}, err
	}
	p := db.UpsertEODReportParams{
		OrganizationID: w.org.ID, BrandID: w.org.BrandID, ReportDate: pgDate(w.day), Timezone: tz,
		PeriodStart: pgtype.Timestamptz{Time: start, Valid: true}, PeriodEnd: pgtype.Timestamptz{Time: end, Valid: true},
		Kind: w.kind, Summary: body, GeneratedByUserID: w.actor,
	}
	if w.warehouseID > 0 {
		p.WarehouseID = pgtype.Int8{Int64: w.warehouseID, Valid: true}
	}
	return p, nil
}

// Generate (re)writes the report of a day for the active organization
// (manual run). A future day is refused; today reports the day so far.
func (s *EOD) Generate(ctx context.Context, c EODCaller, in EODGenerateInput) (EODReport, error) {
	orgID, err := guard(c.Caller)
	if err != nil {
		return EODReport{}, err
	}
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return EODReport{}, fmt.Errorf("warehouse eod: organization: %w", err)
	}
	loc, _ := zoneOf(org.Timezone)
	today := s.now().In(loc)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	day, ok, err := parseDay("date", in.Date)
	if err != nil {
		return EODReport{}, err
	}
	if !ok {
		day = today
	}
	if day.After(today) {
		return EODReport{}, invalid("date", "must not be in the future")
	}
	whID, err := s.warehouseID(ctx, orgID, in.WarehouseUUID)
	if err != nil {
		return EODReport{}, err
	}
	actor := pgtype.Int8{Int64: c.Principal.UserInternal, Valid: c.Principal.UserInternal > 0}
	p, err := s.params(ctx, eodWrite{org: org, warehouseID: whID, day: day, kind: EODKindManual, actor: actor})
	if err != nil {
		return EODReport{}, err
	}
	row, err := s.q.UpsertEODReport(ctx, p)
	if err != nil {
		return EODReport{}, mapDBError(err)
	}
	return s.view(ctx, s.q, row)
}

// Get returns a stored report of the active organization.
func (s *EOD) Get(ctx context.Context, c Caller, id uuid.UUID) (EODReport, error) {
	orgID, err := guard(c)
	if err != nil {
		return EODReport{}, err
	}
	row, err := s.q.GetEODReportByUUID(ctx, db.GetEODReportByUUIDParams{Uuid: id, OrganizationID: orgID})
	if err != nil {
		return EODReport{}, notFound(err, ErrEODReportNotFound)
	}
	return s.view(ctx, s.q, row)
}

// List lists the stored reports of the active organization, newest day
// first (system report before the warehouse reports of a day).
func (s *EOD) List(ctx context.Context, c Caller, in EODListInput) ([]EODReport, int64, error) {
	orgID, err := guard(c)
	if err != nil {
		return nil, 0, err
	}
	scope := strings.TrimSpace(in.Scope)
	if scope != "" && scope != EODScopeSystem && scope != EODScopeWarehouse {
		return nil, 0, invalid("scope", "must be system or warehouse")
	}
	whID, err := s.warehouseID(ctx, orgID, in.WarehouseUUID)
	if err != nil {
		return nil, 0, err
	}
	sort := orDefault(in.Sort, EODSort)
	arg := db.ListEODReportsParams{
		OrganizationID: orgID, Scope: scope, Kinds: in.Kinds,
		SortKey: sort.Key, SortDesc: sort.Desc, RowLimit: in.Limit, RowOffset: in.Offset,
	}
	if whID > 0 {
		arg.WarehouseID = pgtype.Int8{Int64: whID, Valid: true}
	}
	if d, ok, err := parseDay("date_from", in.DateFrom); err != nil {
		return nil, 0, err
	} else if ok {
		arg.DateFrom = pgDate(d)
	}
	if d, ok, err := parseDay("date_to", in.DateTo); err != nil {
		return nil, 0, err
	} else if ok {
		arg.DateTo = pgDate(d)
	}
	if arg.RowLimit <= 0 {
		arg.RowLimit = 20
	}
	rows, err := s.q.ListEODReports(ctx, arg)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountEODReports(ctx, db.CountEODReportsParams{
		OrganizationID: orgID, WarehouseID: arg.WarehouseID, Scope: scope, DateFrom: arg.DateFrom, DateTo: arg.DateTo,
		Kinds: in.Kinds,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]EODReport, 0, len(rows))
	for _, r := range rows {
		v, err := s.view(ctx, s.q, r)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}

// RunDaily is the cron run: for every active center / distributor with a
// warehouse it writes the previous local day's system report and one
// report per active warehouse, unless that report already exists (a rerun
// or an earlier manual report wins). It returns the number of reports
// written; one organization's failure does not stop the others.
func (s *EOD) RunDaily(ctx context.Context) (int, error) {
	orgs, err := s.q.ListEODReportOrganizations(ctx)
	if err != nil {
		return 0, fmt.Errorf("warehouse eod: organizations: %w", err)
	}
	written := 0
	var errs []error
	for _, o := range orgs {
		n, err := s.RunOrganization(ctx, o.ID)
		written += n
		if err != nil {
			errs = append(errs, fmt.Errorf("organization %d: %w", o.ID, err))
		}
	}
	return written, errors.Join(errs...)
}

// RunOrganization writes the missing auto reports of the previous local
// day of one organization (system + every active warehouse).
func (s *EOD) RunOrganization(ctx context.Context, orgID int64) (int, error) {
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return 0, err
	}
	loc, _ := zoneOf(org.Timezone)
	day := s.now().In(loc).AddDate(0, 0, -1)
	whs, err := s.q.ListWarehouses(ctx, db.ListWarehousesParams{OrganizationID: orgID, Active: pgtype.Bool{Bool: true, Valid: true}})
	if err != nil {
		return 0, err
	}
	scopes := make([]int64, 0, len(whs)+1)
	scopes = append(scopes, 0)
	for _, w := range whs {
		scopes = append(scopes, w.ID)
	}
	written := 0
	for _, whID := range scopes {
		ok, err := s.writeIfMissing(ctx, org, whID, day)
		if err != nil {
			return written, err
		}
		if ok {
			written++
		}
	}
	return written, nil
}

func (s *EOD) writeIfMissing(ctx context.Context, org db.Organization, whID int64, day time.Time) (bool, error) {
	exists := db.EODReportExistsParams{OrganizationID: org.ID, ReportDate: pgDate(day)}
	if whID > 0 {
		exists.WarehouseID = pgtype.Int8{Int64: whID, Valid: true}
	}
	found, err := s.q.EODReportExists(ctx, exists)
	if err != nil || found {
		return false, err
	}
	p, err := s.params(ctx, eodWrite{org: org, warehouseID: whID, day: day, kind: EODKindAuto})
	if err != nil {
		return false, err
	}
	_, err = s.q.InsertEODReportIfMissing(ctx, db.InsertEODReportIfMissingParams(p))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// EODGuard checks that the caller may use the warehouse module in the
// active organization (the PDF job routes, which load no report).
func EODGuard(c Caller) (int64, error) { return guard(c) }

// DailyTask is the queue processor of the cron (logs the written count).
func (s *EOD) DailyTask(log *slog.Logger) func(ctx context.Context) error {
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context) error {
		n, err := s.RunDaily(ctx)
		log.Info("warehouse_eod_reports_written", "count", n)
		return err
	}
}
