package usecase

// TEC-506 (F5-09b): recommended price publication (API and import), the
// version history, the price in force for a country / currency and the
// recommended block of the distributor and dealer price screens. Versions
// are append-only (F5-09a); a version effective today becomes current in the
// publishing transaction, a later one on its day through the pricing daily
// tick (recommended_jobs.go).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxPublishRows bounds one publication (API body or import file).
const MaxPublishRows = 2000

// Recommended price scopes: the price of the organization's country, or the
// currency-wide price it falls back to.
const (
	ScopeCountry  = "country"
	ScopeCurrency = "currency"
)

// Version statuses of the history.
const (
	VersionCurrent    = "current"
	VersionScheduled  = "scheduled"
	VersionSuperseded = "superseded"
)

// CountryNone is the country filter value of the currency-wide price.
const CountryNone = "none"

// Row error codes shared by the publish API (validation message) and the
// import (row code).
const (
	RowErrProductRequired = "PRICING_IMPORT_PRODUCT_REQUIRED"
	RowErrProductNotFound = "PRICING_IMPORT_PRODUCT_NOT_FOUND"
	RowErrCountryInvalid  = "PRICING_IMPORT_COUNTRY_INVALID"
	RowErrCurrencyInvalid = "PRICING_IMPORT_CURRENCY_INVALID"
	RowErrPriceInvalid    = "PRICING_IMPORT_PRICE_INVALID"
	RowErrDateInvalid     = "PRICING_IMPORT_EFFECTIVE_FROM_INVALID"
	RowErrDatePast        = "PRICING_IMPORT_EFFECTIVE_FROM_PAST"
	RowErrDuplicate       = "PRICING_IMPORT_DUPLICATE_IN_FILE"
)

var rowErrField = map[string]string{
	RowErrProductRequired: "product", RowErrProductNotFound: "product", RowErrCountryInvalid: "country",
	RowErrCurrencyInvalid: "currency", RowErrPriceInvalid: "price", RowErrDateInvalid: "effective_from",
	RowErrDatePast: "effective_from", RowErrDuplicate: "product",
}

var rowErrMessage = map[string]string{
	RowErrProductRequired: "product is required",
	RowErrProductNotFound: "product not found",
	RowErrCountryInvalid:  "must be an ISO-3166 alpha-2 country code or empty",
	RowErrCurrencyInvalid: "must be a three-letter ISO-4217 code",
	RowErrPriceInvalid:    "must be a non-negative decimal with at most 16 integer and 2 fractional digits",
	RowErrDateInvalid:     "must be a date (YYYY-MM-DD)",
	RowErrDatePast:        "must be today or later",
	RowErrDuplicate:       "the same product, country, currency and day appears twice",
}

// NUMERIC(18,2), never negative.
var recommendedPriceRe = regexp.MustCompile(`^[0-9]{1,16}(\.[0-9]{1,2})?$`)

// TxBeginner opens the publishing transaction.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Outbox writes events in a transaction.
type Outbox interface {
	Enqueue(ctx context.Context, tx pgx.Tx, ev events.Event) error
}

// RecommendedSettings are the pricing system settings (sysconfig).
type RecommendedSettings interface {
	PricingDeviationWarningPct(ctx context.Context) int
	PricingPriceListAutoPublish(ctx context.Context) bool
}

// PriceListTask names one price list PDF: a country (0 = currency-wide) and
// a currency of a brand. Token makes one publication's task unique.
type PriceListTask struct {
	BrandID   int64  `json:"brand_id"`
	CountryID int64  `json:"country_id"`
	Currency  string `json:"currency"`
	Token     string `json:"token"`
}

// PriceListQueue enqueues price list PDF publications (docs queue).
type PriceListQueue interface {
	EnqueuePriceList(ctx context.Context, task PriceListTask) error
}

// Recommended is the recommended price service.
type Recommended struct {
	pool     TxBeginner
	q        *db.Queries
	outbox   Outbox
	settings RecommendedSettings
	queue    PriceListQueue
	now      func() time.Time
	log      *slog.Logger
}

// NewRecommended creates the service. settings may be nil (defaults).
func NewRecommended(pool TxBeginner, q *db.Queries, box Outbox, settings RecommendedSettings, log *slog.Logger) *Recommended {
	if log == nil {
		log = slog.Default()
	}
	return &Recommended{pool: pool, q: q, outbox: box, settings: settings, now: time.Now, log: log}
}

// SetPriceListQueue wires the price list PDF queue (worker).
func (s *Recommended) SetPriceListQueue(q PriceListQueue) { s.queue = q }

// SetClock replaces the clock (tests).
func (s *Recommended) SetClock(now func() time.Time) { s.now = now }

func (s *Recommended) threshold(ctx context.Context) int {
	if s.settings == nil {
		return 15
	}
	return s.settings.PricingDeviationWarningPct(ctx)
}

// ThresholdPct is the price discipline threshold in percent
// (pricing.deviation_warning_pct): the price screens flag a deviation at or
// above it either way.
func (s *Recommended) ThresholdPct(ctx context.Context) int { return s.threshold(ctx) }

func (s *Recommended) autoPublish(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.PricingPriceListAutoPublish(ctx)
}

// LocalDay is the calendar day of at in tz (UTC midnight), the "today" of
// the brand center for effective dates and snapshots.
func LocalDay(at time.Time, tz string) time.Time {
	loc, err := time.LoadLocation(strings.TrimSpace(tz))
	if err != nil || strings.TrimSpace(tz) == "" {
		loc, _ = time.LoadLocation(i18n.DefaultTimezone)
		if loc == nil {
			loc = time.UTC
		}
	}
	l := at.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

func pgDate(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

func dateText(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format(time.DateOnly)
}

// money formats a NUMERIC(*,2) value with exactly 2 decimals ("" if NULL).
func money(n pgtype.Numeric) string {
	if p := fixed2(n); p != nil {
		return *p
	}
	return ""
}

// fixed2 formats a numeric with exactly 2 decimals (nil if NULL).
func fixed2(n pgtype.Numeric) *string {
	p := numericText(n)
	if p == nil {
		return nil
	}
	r, ok := new(big.Rat).SetString(*p)
	if !ok {
		return p
	}
	out := r.FloatString(2)
	if out == "-0.00" {
		out = "0.00"
	}
	return &out
}

// --- Publication ------------------------------------------------------------------

// PublishRow is one price of a publication. The product is named by uuid
// (API) or sku (import); Country is an ISO-3166 alpha-2 code, empty for the
// currency-wide price; EffectiveFrom (YYYY-MM-DD) defaults to the batch day.
type PublishRow struct {
	ProductUUID   *uuid.UUID
	SKU           string
	Country       string
	Currency      string
	Price         string
	EffectiveFrom string
}

// PublishInput is a publication: rows, the default effective day (empty =
// today in the center's timezone) and a note.
type PublishInput struct {
	EffectiveFrom string
	Note          string
	Rows          []PublishRow
}

// RecommendedVersion is one version of the history.
type RecommendedVersion struct {
	UUID            uuid.UUID  `json:"uuid"`
	ProductUUID     uuid.UUID  `json:"product_uuid"`
	ProductSKU      string     `json:"product_sku"`
	ProductName     string     `json:"product_name"`
	CountryISO2     string     `json:"country_iso2"`
	Currency        string     `json:"currency"`
	Price           string     `json:"price"`
	EffectiveFrom   string     `json:"effective_from"`
	Source          string     `json:"source"`
	BatchID         *uuid.UUID `json:"batch_id"`
	Note            string     `json:"note"`
	PublishedAt     time.Time  `json:"published_at"`
	PublishedByName string     `json:"published_by_name"`
	SupersededAt    *time.Time `json:"superseded_at"`
	Status          string     `json:"status"`
}

// PublishResult is the outcome of a publication.
type PublishResult struct {
	BatchID        uuid.UUID            `json:"batch_id"`
	PriceCount     int                  `json:"price_count"`
	AppliedCount   int                  `json:"applied_count"`
	ScheduledCount int                  `json:"scheduled_count"`
	Versions       []RecommendedVersion `json:"versions"`
}

// preparedRow is a validated publication row.
type preparedRow struct {
	productID   int64
	productUUID uuid.UUID
	sku, name   string
	countryID   int64
	countryISO2 string
	currency    string
	price       string
	effective   time.Time
}

func (r preparedRow) key() string {
	return fmt.Sprintf("%d|%d|%s|%s", r.productID, r.countryID, r.currency, r.effective.Format(time.DateOnly))
}

// rowCodeError is a row validation failure.
type rowCodeError struct {
	index int
	code  string
}

func (e *rowCodeError) Error() string { return e.code }

// prepare validates one row; code is set on a row error.
func (s *Recommended) prepare(ctx context.Context, q *db.Queries, brandID int64, today, batchDay time.Time, in PublishRow, countries map[string]int64) (preparedRow, string, error) {
	var out preparedRow
	var p db.Product
	var err error
	switch {
	case in.ProductUUID != nil:
		p, err = q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: *in.ProductUUID, BrandID: brandID})
	case strings.TrimSpace(in.SKU) != "":
		p, err = q.GetProductBySKU(ctx, db.GetProductBySKUParams{Sku: strings.TrimSpace(in.SKU), BrandID: brandID})
	default:
		return out, RowErrProductRequired, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return out, RowErrProductNotFound, nil
	}
	if err != nil {
		return out, "", fmt.Errorf("pricing: product: %w", err)
	}
	out.productID, out.productUUID, out.sku, out.name = p.ID, p.Uuid, p.Sku, p.Name
	if iso := strings.ToUpper(strings.TrimSpace(in.Country)); iso != "" {
		id, ok := countries[iso]
		if !ok {
			c, err := q.GetCountryByISO2(ctx, iso)
			if errors.Is(err, pgx.ErrNoRows) || len(iso) != 2 {
				return out, RowErrCountryInvalid, nil
			}
			if err != nil {
				return out, "", fmt.Errorf("pricing: country: %w", err)
			}
			id = c.ID
			countries[iso] = id
		}
		out.countryID, out.countryISO2 = id, iso
	}
	cur, err := NormalizeCurrency(in.Currency)
	if err != nil {
		return out, RowErrCurrencyInvalid, nil
	}
	out.currency = cur
	price := strings.TrimSpace(in.Price)
	if !recommendedPriceRe.MatchString(price) {
		return out, RowErrPriceInvalid, nil
	}
	out.price = price
	out.effective = batchDay
	if raw := strings.TrimSpace(in.EffectiveFrom); raw != "" {
		d, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			return out, RowErrDateInvalid, nil
		}
		out.effective = d
	}
	if out.effective.Before(today) {
		return out, RowErrDatePast, nil
	}
	return out, "", nil
}

// publishCenter is the brand center of a publishing viewer.
func (s *Recommended) publishCenter(ctx context.Context, v Viewer) (db.Organization, error) {
	if v.OrgType != OrgCenter || !v.RecommendedWrite {
		return db.Organization{}, ErrForbidden
	}
	center, err := s.q.GetOrganizationByID(ctx, v.OrgID)
	if err != nil {
		return db.Organization{}, fmt.Errorf("pricing: center: %w", err)
	}
	return center, nil
}

// batchDay parses the default effective day of a publication.
func batchDay(raw string, today time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return today, nil
	}
	d, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, invalid("effective_from", rowErrMessage[RowErrDateInvalid])
	}
	if d.Before(today) {
		return time.Time{}, invalid("effective_from", rowErrMessage[RowErrDatePast])
	}
	return d, nil
}

// Publish publishes a batch of recommended prices (center with
// pricing.recommended.write; the route requires a step-up). Every row is
// validated first; the versions, the projection of the rows in force today
// (and product_prices.recommended_sale_price for currency-wide rows) and the
// pricing.recommended_published event are written in one transaction.
func (s *Recommended) Publish(ctx context.Context, v Viewer, in PublishInput) (PublishResult, error) {
	center, err := s.publishCenter(ctx, v)
	if err != nil {
		return PublishResult{}, err
	}
	if len(in.Rows) == 0 {
		return PublishResult{}, invalid("rows", "at least one row is required")
	}
	if len(in.Rows) > MaxPublishRows {
		return PublishResult{}, invalid("rows", fmt.Sprintf("at most %d rows per publication", MaxPublishRows))
	}
	if len(in.Note) > 2000 {
		return PublishResult{}, invalid("note", "must be at most 2000 characters")
	}
	today := LocalDay(s.now(), center.Timezone)
	day, err := batchDay(in.EffectiveFrom, today)
	if err != nil {
		return PublishResult{}, err
	}
	rows, rowErr, err := s.prepareAll(ctx, s.q, center.BrandID, today, day, in.Rows)
	if err != nil {
		return PublishResult{}, err
	}
	if rowErr != nil {
		return PublishResult{}, invalid(fmt.Sprintf("rows[%d].%s", rowErr.index, rowErrField[rowErr.code]), rowErrMessage[rowErr.code])
	}
	var out PublishResult
	err = s.inTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		out, err = s.publishTx(ctx, tx, q, center, v.UserID, uuid.New(), model.SourcePublish, strings.TrimSpace(in.Note), rows, today)
		return err
	})
	return out, err
}

// prepareAll validates every row; the first failing row is returned.
func (s *Recommended) prepareAll(ctx context.Context, q *db.Queries, brandID int64, today, day time.Time, in []PublishRow) ([]preparedRow, *rowCodeError, error) {
	countries := map[string]int64{}
	seen := map[string]bool{}
	out := make([]preparedRow, 0, len(in))
	for i, r := range in {
		row, code, err := s.prepare(ctx, q, brandID, today, day, r, countries)
		if err != nil {
			return nil, nil, err
		}
		if code == "" && seen[row.key()] {
			code = RowErrDuplicate
		}
		if code != "" {
			return nil, &rowCodeError{index: i, code: code}, nil
		}
		seen[row.key()] = true
		out = append(out, row)
	}
	return out, nil, nil
}

func (s *Recommended) inTx(ctx context.Context, fn func(tx pgx.Tx, q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pricing: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx, s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// priceKey is a country (0 = currency-wide) and currency of a brand.
type priceKey struct {
	countryID   int64
	countryISO2 string
	currency    string
}

// publishTx writes the versions of a validated batch and the event.
func (s *Recommended) publishTx(ctx context.Context, tx pgx.Tx, q *db.Queries, center db.Organization, userID int64, batch uuid.UUID, source, note string, rows []preparedRow, today time.Time) (PublishResult, error) {
	store := repository.FromQueries(q)
	out := PublishResult{BatchID: batch, Versions: make([]RecommendedVersion, 0, len(rows))}
	keys := map[priceKey]bool{}
	currencies := map[string]bool{}
	for _, r := range rows {
		price, err := numericOf(r.price)
		if err != nil {
			return PublishResult{}, err
		}
		ver, err := store.Publish(ctx, db.InsertRecommendedPriceVersionParams{
			OrganizationID: center.ID, BrandID: center.BrandID, ProductID: r.productID,
			CountryID: pgtype.Int8{Int64: r.countryID, Valid: r.countryID != 0}, Currency: r.currency,
			Price: price, EffectiveFrom: pgDate(r.effective), Source: source,
			PublishedByUserID: pgtype.Int8{Int64: userID, Valid: userID != 0},
			BatchID:           pgtype.UUID{Bytes: batch, Valid: true}, Note: note,
		}, pgDate(today))
		if err != nil {
			return PublishResult{}, fmt.Errorf("pricing: publish: %w", err)
		}
		status := VersionScheduled
		if !r.effective.After(today) {
			status = VersionCurrent
			out.AppliedCount++
		} else {
			out.ScheduledCount++
		}
		b := batch
		out.Versions = append(out.Versions, RecommendedVersion{
			UUID: ver.Uuid, ProductUUID: r.productUUID, ProductSKU: r.sku, ProductName: r.name,
			CountryISO2: r.countryISO2, Currency: r.currency, Price: money(ver.Price),
			EffectiveFrom: dateText(ver.EffectiveFrom), Source: ver.Source, BatchID: &b, Note: ver.Note,
			PublishedAt: ver.PublishedAt.Time, Status: status,
		})
		keys[priceKey{countryID: r.countryID, countryISO2: r.countryISO2, currency: r.currency}] = true
		currencies[r.currency] = true
	}
	out.PriceCount = len(out.Versions)

	keyList := sortedKeys(keys)
	recipients := map[int64]bool{}
	payloadKeys := make([]map[string]any, 0, len(keyList))
	for _, k := range keyList {
		aud, err := q.ListRecommendedPriceListAudience(ctx, db.ListRecommendedPriceListAudienceParams{
			BrandID: center.BrandID, CountryID: pgtype.Int8{Int64: k.countryID, Valid: k.countryID != 0}, Currency: k.currency,
		})
		if err != nil {
			return PublishResult{}, fmt.Errorf("pricing: audience: %w", err)
		}
		for _, a := range aud {
			recipients[a.UserID] = true
		}
		payloadKeys = append(payloadKeys, map[string]any{"country_id": k.countryID, "country_iso2": k.countryISO2, "currency": k.currency})
	}
	ids := make([]int64, 0, len(recipients))
	for id := range recipients {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	curList := make([]string, 0, len(currencies))
	for c := range currencies {
		curList = append(curList, c)
	}
	sort.Strings(curList)
	effective := ""
	if len(rows) > 0 {
		first := rows[0].effective
		for _, r := range rows {
			if r.effective.Before(first) {
				first = r.effective
			}
		}
		effective = first.Format(time.DateOnly)
	}
	if s.outbox != nil {
		ev := events.New(events.PricingRecommendedPublished).WithTenant(center.ID).
			WithEntity("recommended_price_batch", nil, &batch).
			WithPayload(map[string]any{
				"brand_id": center.BrandID, "batch_id": batch.String(), "effective_from": effective,
				"applied": out.AppliedCount > 0, "price_count": out.PriceCount,
				"currencies": strings.Join(curList, ", "), "keys": payloadKeys, "notify_user_ids": ids,
			})
		if userID != 0 {
			ev = ev.WithActor(userID)
		}
		if err := s.outbox.Enqueue(ctx, tx, ev); err != nil {
			return PublishResult{}, fmt.Errorf("pricing: outbox: %w", err)
		}
	}
	return out, nil
}

func sortedKeys(m map[priceKey]bool) []priceKey {
	out := make([]priceKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].currency != out[j].currency {
			return out[i].currency < out[j].currency
		}
		return out[i].countryID < out[j].countryID
	})
	return out
}

func numericOf(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("pricing: numeric %q: %w", s, err)
	}
	return n, nil
}

// --- History --------------------------------------------------------------------

// VersionFilter narrows the version history (docs/list-contract.md).
type VersionFilter struct {
	ProductUUIDs  []uuid.UUID
	Countries     []string // ISO2 codes or CountryNone
	Currencies    []string
	Sources       []string
	BatchID       *uuid.UUID
	EffectiveFrom apiquery.TimeRange
	Q             string
	Sort          []apiquery.SortField
	Limit         int32
	Offset        int32
}

func (s *Recommended) requireRead(v Viewer) error {
	if !v.RecommendedRead {
		return ErrForbidden
	}
	return checkViewerType(v)
}

// productIDs resolves product uuids of the brand (unknown ones match
// nothing: -1).
func (s *Recommended) productIDs(ctx context.Context, brandID int64, ids []uuid.UUID) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.q.ListProductsByUUIDs(ctx, db.ListProductsByUUIDsParams{BrandID: brandID, Uuids: ids})
	if err != nil {
		return nil, fmt.Errorf("pricing: products: %w", err)
	}
	out := []int64{-1}
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out, nil
}

// countryIDs resolves ISO2 codes (CountryNone = currency-wide = 0; an
// unknown code matches nothing: -1).
func (s *Recommended) countryIDs(ctx context.Context, codes []string) ([]int64, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	out := []int64{-1}
	for _, raw := range codes {
		iso := strings.ToUpper(strings.TrimSpace(raw))
		if strings.EqualFold(iso, CountryNone) {
			out = append(out, model.CountryAll)
			continue
		}
		c, err := s.q.GetCountryByISO2(ctx, iso)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("pricing: country: %w", err)
		}
		out = append(out, c.ID)
	}
	return out, nil
}

func versionStatus(isCurrent bool, superseded pgtype.Timestamptz) string {
	switch {
	case isCurrent:
		return VersionCurrent
	case superseded.Valid:
		return VersionSuperseded
	default:
		return VersionScheduled
	}
}

// ListVersions returns the version history of the brand (center with
// pricing.recommended.read).
func (s *Recommended) ListVersions(ctx context.Context, v Viewer, f VersionFilter) ([]RecommendedVersion, int64, error) {
	if err := s.requireRead(v); err != nil {
		return nil, 0, err
	}
	if v.OrgType != OrgCenter {
		return nil, 0, ErrForbidden
	}
	products, err := s.productIDs(ctx, v.BrandID, f.ProductUUIDs)
	if err != nil {
		return nil, 0, err
	}
	countries, err := s.countryIDs(ctx, f.Countries)
	if err != nil {
		return nil, 0, err
	}
	arg := db.ListRecommendedPriceVersionsParams{
		BrandID: v.BrandID, ProductIds: products, CountryIds: countries, Currencies: f.Currencies,
		Sources: f.Sources, Q: pgtype.Text{String: f.Q, Valid: f.Q != ""},
		RowLimit: f.Limit, RowOffset: f.Offset,
	}
	if f.BatchID != nil {
		arg.BatchID = pgtype.UUID{Bytes: *f.BatchID, Valid: true}
	}
	if f.EffectiveFrom.From != nil {
		arg.EffectiveFromFrom = pgDate(*f.EffectiveFrom.From)
	}
	if f.EffectiveFrom.Before != nil {
		arg.EffectiveFromBefore = pgDate(*f.EffectiveFrom.Before)
	}
	rows, err := repository.FromQueries(s.q).ListVersions(ctx, arg, f.Sort)
	if err != nil {
		return nil, 0, err
	}
	out := make([]RecommendedVersion, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.TotalCount
		var batch *uuid.UUID
		if r.BatchID.Valid {
			b := uuid.UUID(r.BatchID.Bytes)
			batch = &b
		}
		var sup *time.Time
		if r.SupersededAt.Valid {
			t := r.SupersededAt.Time
			sup = &t
		}
		out = append(out, RecommendedVersion{
			UUID: r.Uuid, ProductUUID: r.ProductUuid, ProductSKU: r.ProductSku, ProductName: r.ProductName,
			CountryISO2: r.CountryIso2, Currency: strings.TrimSpace(r.Currency), Price: money(r.Price),
			EffectiveFrom: dateText(r.EffectiveFrom), Source: r.Source, BatchID: batch, Note: r.Note,
			PublishedAt: r.PublishedAt.Time, PublishedByName: r.PublishedByName, SupersededAt: sup,
			Status: versionStatus(r.IsCurrent, r.SupersededAt),
		})
	}
	if len(rows) == 0 && f.Offset > 0 {
		return out, 0, nil
	}
	return out, total, nil
}

// --- Price in force ------------------------------------------------------------------

// CurrentFilter narrows the price list in force. Country (ISO2) and
// Currency default to the caller organization's; distributors and dealers
// always read their own country.
type CurrentFilter struct {
	Country      string
	Currency     string
	ProductUUIDs []uuid.UUID
	Active       *bool
	Q            string
	Sort         []apiquery.SortField
	Limit        int32
	Offset       int32
}

// CurrentPrice is the recommended price in force of a product.
type CurrentPrice struct {
	ProductUUID   uuid.UUID  `json:"product_uuid"`
	ProductSKU    string     `json:"product_sku"`
	ProductName   string     `json:"product_name"`
	Price         string     `json:"price"`
	Currency      string     `json:"currency"`
	CountryISO2   string     `json:"country_iso2"`
	Scope         string     `json:"scope"`
	EffectiveFrom string     `json:"effective_from"`
	VersionUUID   uuid.UUID  `json:"version_uuid"`
	Source        string     `json:"source"`
	BatchID       *uuid.UUID `json:"batch_id"`
}

// CurrentSort is the sort contract of the price list in force.
var CurrentSort = repository.CurrentSort

// Current returns the recommended price list in force for a country and
// currency: the country's own price, else the currency-wide one.
func (s *Recommended) Current(ctx context.Context, v Viewer, f CurrentFilter) ([]CurrentPrice, int64, error) {
	if err := s.requireRead(v); err != nil {
		return nil, 0, err
	}
	org, err := s.q.GetOrganizationByID(ctx, v.OrgID)
	if err != nil {
		return nil, 0, fmt.Errorf("pricing: organization: %w", err)
	}
	country := org.CountryID
	if iso := strings.ToUpper(strings.TrimSpace(f.Country)); iso != "" && v.OrgType == OrgCenter {
		if strings.EqualFold(iso, CountryNone) {
			country = pgtype.Int8{}
		} else {
			c, err := s.q.GetCountryByISO2(ctx, iso)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, 0, invalid("country", rowErrMessage[RowErrCountryInvalid])
			}
			if err != nil {
				return nil, 0, fmt.Errorf("pricing: country: %w", err)
			}
			country = pgtype.Int8{Int64: c.ID, Valid: true}
		}
	} else if v.OrgType == OrgCenter && iso == "" {
		country = pgtype.Int8{}
	}
	cur := strings.TrimSpace(org.Currency)
	if strings.TrimSpace(f.Currency) != "" {
		if cur, err = NormalizeCurrency(f.Currency); err != nil {
			return nil, 0, err
		}
	}
	products, err := s.productIDs(ctx, v.BrandID, f.ProductUUIDs)
	if err != nil {
		return nil, 0, err
	}
	resolved, err := apiquery.ResolveSort(f.Sort, CurrentSort)
	if err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit == 0 {
		limit = apiquery.DefaultLimit
	}
	arg := db.ListApplicableRecommendedPricesPageParams{
		BrandID: v.BrandID, Currency: cur, CountryID: country, ProductIds: products,
		Q:       pgtype.Text{String: strings.TrimSpace(f.Q), Valid: strings.TrimSpace(f.Q) != ""},
		SortKey: resolved.Key, SortDesc: resolved.Desc, RowLimit: limit, RowOffset: f.Offset,
	}
	if f.Active != nil {
		arg.Active = pgtype.Bool{Bool: *f.Active, Valid: true}
	}
	rows, err := s.q.ListApplicableRecommendedPricesPage(ctx, arg)
	if err != nil {
		return nil, 0, fmt.Errorf("pricing: current: %w", err)
	}
	out := make([]CurrentPrice, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.TotalCount
		scope := ScopeCurrency
		if r.CountryID.Valid {
			scope = ScopeCountry
		}
		var batch *uuid.UUID
		if r.BatchID.Valid {
			b := uuid.UUID(r.BatchID.Bytes)
			batch = &b
		}
		out = append(out, CurrentPrice{
			ProductUUID: r.ProductUuid, ProductSKU: r.ProductSku, ProductName: r.ProductName,
			Price: money(r.Price), Currency: strings.TrimSpace(r.Currency), CountryISO2: r.CountryIso2,
			Scope: scope, EffectiveFrom: dateText(r.EffectiveFrom), VersionUUID: r.VersionUuid,
			Source: r.Source, BatchID: batch,
		})
	}
	return out, total, nil
}

// --- Recommended block of the price screens --------------------------------------

// RecommendedRef is the recommended price a distributor or dealer screen
// shows next to its own price: the country's price, else the currency-wide
// one.
type RecommendedRef struct {
	Price         string `json:"price"`
	Currency      string `json:"currency"`
	CountryISO2   string `json:"country_iso2"`
	Scope         string `json:"scope"`
	EffectiveFrom string `json:"effective_from"`
}

// RecommendedKey is a product and currency.
type RecommendedKey struct {
	ProductID int64
	Currency  string
}

// RecommendedReader reads the applicable recommended prices.
type RecommendedReader interface {
	ListApplicableRecommendedPrices(ctx context.Context, arg db.ListApplicableRecommendedPricesParams) ([]db.ListApplicableRecommendedPricesRow, error)
}

// ApplicableRecommended returns the recommended price in force per product
// and currency for an organization's country (currencies empty = all).
func ApplicableRecommended(ctx context.Context, q RecommendedReader, brandID int64, country pgtype.Int8, productIDs []int64, currencies []string) (map[RecommendedKey]RecommendedRef, error) {
	out := map[RecommendedKey]RecommendedRef{}
	if len(productIDs) == 0 {
		return out, nil
	}
	var curs []string
	if len(currencies) > 0 {
		curs = currencies
	}
	rows, err := q.ListApplicableRecommendedPrices(ctx, db.ListApplicableRecommendedPricesParams{
		BrandID: brandID, ProductIds: productIDs, Currencies: curs, CountryID: country,
	})
	if err != nil {
		return nil, fmt.Errorf("pricing: recommended prices: %w", err)
	}
	for _, r := range rows {
		scope := ScopeCurrency
		if r.CountryID.Valid {
			scope = ScopeCountry
		}
		cur := strings.TrimSpace(r.Currency)
		out[RecommendedKey{ProductID: r.ProductID, Currency: cur}] = RecommendedRef{
			Price: money(r.Price), Currency: cur, CountryISO2: r.CountryIso2, Scope: scope,
			EffectiveFrom: dateText(r.EffectiveFrom),
		}
	}
	return out, nil
}

// DeviationPct is (price - recommended) / recommended in percent, rounded
// half away from zero to 2 decimals (the snapshot's ROUND); nil when either
// is missing or the recommendation is zero.
func DeviationPct(price, recommended string) *string {
	p, ok1 := new(big.Rat).SetString(strings.TrimSpace(price))
	r, ok2 := new(big.Rat).SetString(strings.TrimSpace(recommended))
	if !ok1 || !ok2 || r.Sign() == 0 {
		return nil
	}
	d := new(big.Rat).Sub(p, r)
	d.Mul(d, big.NewRat(100, 1))
	d.Quo(d, r)
	out := d.FloatString(2)
	if out == "-0.00" {
		out = "0.00"
	}
	return &out
}

// VersionSort is the sort contract of the version history.
var VersionSort = repository.VersionSort

// IsVersionSource reports a known version source.
func IsVersionSource(s string) bool {
	for _, v := range model.Sources {
		if v == s {
			return true
		}
	}
	return false
}
