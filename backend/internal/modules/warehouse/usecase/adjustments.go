package usecase

// TEC-1226: stock adjustment documents (waste, damage, scrap, loss, found,
// other).
//
// A document belongs to the active organization (center or distributor,
// K12) and holds lines of units the organization holds:
//   - whole:    a serial unit (piece or roll) as a whole. Posting voids it
//               (a quarantined unit too); a "found" document brings a voided
//               unit back from the organization's trash (ledger found).
//   - meters:   part of a roll in stock: count_adjustment of -meters (found:
//               +meters, never above the roll's initial length).
//   - quantity: a fixed barcode quantity: count_adjustment from one holding
//               of the organization (found: credited to a bin or the
//               organization).
//
// Flow: draft -> pending_approval (submit) -> posted (approve), or
// cancelled. Submit freezes the totals: pieces (whole pieces and fixed
// quantities) and meters (roll meters and whole rolls). Above the
// organization's threshold (stock_adjustment_settings, pieces and meters
// separately) a second person approves: the creator and the submitter cannot.
// Approving needs stock.adjust; a damage document needs at least one photo.
// The ledger key of a line is stock_adjustment:stock_adjustment_line:<id>:<type>:<barcode>.
// No price or amount is part of any view (warehouse roles, K8).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Adjustment types.
const (
	AdjTypeWaste  = "waste"
	AdjTypeDamage = "damage"
	AdjTypeScrap  = "scrap"
	AdjTypeLoss   = "loss"
	AdjTypeFound  = "found"
	AdjTypeOther  = "other"
)

// AdjustmentTypes lists the adjustment types.
var AdjustmentTypes = []string{AdjTypeWaste, AdjTypeDamage, AdjTypeScrap, AdjTypeLoss, AdjTypeFound, AdjTypeOther}

// Adjustment statuses.
const (
	AdjStatusDraft     = "draft"
	AdjStatusPending   = "pending_approval"
	AdjStatusPosted    = "posted"
	AdjStatusCancelled = "cancelled"
)

// Adjustment sources.
const (
	AdjSourceManual = "manual"
	AdjSourceReturn = "return_inspection"
)

// Adjustment line kinds.
const (
	AdjLineWhole    = "whole"
	AdjLineMeters   = "meters"
	AdjLineQuantity = "quantity"
)

// Ledger reference of adjustment movements.
const (
	adjustmentSource  = "stock_adjustment"
	adjustmentRefType = "stock_adjustment_line"
)

// Limits and defaults.
const (
	MaxAdjustmentLines      = 500
	MaxAdjustmentPhotos     = 10
	MaxAdjustmentPhotoBytes = 10 << 20
	MaxAdjustmentQuantity   = 100000
	maxAdjustmentNote       = 1000
	maxAdjustmentReason     = 64
	// DefaultThresholdQuantity / DefaultThresholdMeters apply while the
	// organization has saved no settings.
	DefaultThresholdQuantity = 5
	DefaultThresholdMeters   = "25.00"
)

// Adjustment errors.
var (
	ErrAdjustmentNotFound      = errors.New("warehouse: stock adjustment not found")
	ErrAdjustmentPhotoNotFound = errors.New("warehouse: adjustment photo not found")
	ErrAdjustmentStatus        = errors.New("warehouse: stock adjustment status does not allow this")
	// ErrAdjustmentForbidden: approving (or the settings) needs stock.adjust.
	ErrAdjustmentForbidden = errors.New("warehouse: stock adjustment needs stock.adjust")
	// ErrAdjustmentSelfApproval: above the threshold the creator and the
	// submitter cannot approve.
	ErrAdjustmentSelfApproval = errors.New("warehouse: an adjustment above the threshold needs a second person")
	ErrAdjustmentEmpty        = errors.New("warehouse: the adjustment has no line")
	ErrAdjustmentPhotoNeeded  = errors.New("warehouse: a damage adjustment needs a photo")
	// ErrAdjustmentUnit: the unit cannot be adjusted (message says why).
	ErrAdjustmentUnit = errors.New("warehouse: unit cannot be adjusted")
	// ErrAdjustmentUnitBusy: another open document, an order reservation or
	// an open warehouse transfer holds the unit.
	ErrAdjustmentUnitBusy = errors.New("warehouse: unit is held by another document")
	// ErrAdjustmentStale: the stock changed since the line was added.
	ErrAdjustmentStale = errors.New("warehouse: stock changed since the line was added")
	// ErrStorageUnavailable: photos need the object storage.
	ErrStorageUnavailable = errors.New("warehouse: file storage is not configured")
)

func unitErr(msg string) error { return fmt.Errorf("%w: %s", ErrAdjustmentUnit, msg) }

// Adjustments implements the stock adjustment use cases.
type Adjustments struct {
	pool   TxBeginner
	q      *db.Queries
	ledger *ledger.Ledger
	store  storage.Driver
}

// NewAdjustments builds the service; store keeps the photos (nil: photos
// unavailable).
func NewAdjustments(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, store storage.Driver) *Adjustments {
	return &Adjustments{pool: pool, q: q, ledger: ledger.New(q, out), store: store}
}

// ---------------------------------------------------------------------------
// Views.

// AdjustmentPerson is a user on the document.
type AdjustmentPerson struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// AdjustmentLocation is a bin of a line.
type AdjustmentLocation struct {
	UUID     uuid.UUID `json:"uuid"`
	FullCode string    `json:"full_code"`
}

// AdjustmentUnit is the unit of a line.
type AdjustmentUnit struct {
	UUID     uuid.UUID `json:"uuid"`
	Barcode  string    `json:"barcode"`
	UnitKind string    `json:"unit_kind"`
	IsRoll   bool      `json:"is_roll"`
}

// AdjustmentLine is one line (no price).
type AdjustmentLine struct {
	UUID       uuid.UUID           `json:"uuid"`
	Kind       string              `json:"kind"`
	Unit       AdjustmentUnit      `json:"unit"`
	Product    CountProduct        `json:"product"`
	Quantity   *int32              `json:"quantity"`
	Meters     *string             `json:"meters"`
	RollMeters *string             `json:"roll_meters"`
	Location   *AdjustmentLocation `json:"location"`
	Posted     bool                `json:"posted"`
}

// AdjustmentPhoto is a photo of the document.
type AdjustmentPhoto struct {
	UUID        uuid.UUID `json:"uuid"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

// StockAdjustment is the API view of a document.
type StockAdjustment struct {
	UUID                   uuid.UUID         `json:"uuid"`
	AdjustmentNo           string            `json:"adjustment_no"`
	Type                   string            `json:"type"`
	ReasonCode             *string           `json:"reason_code"`
	Note                   *string           `json:"note"`
	Source                 string            `json:"source"`
	Status                 string            `json:"status"`
	Warehouse              *CountWarehouse   `json:"warehouse"`
	TotalQuantity          int32             `json:"total_quantity"`
	TotalMeters            string            `json:"total_meters"`
	RequiresSecondApproval bool              `json:"requires_second_approval"`
	CreatedBy              *AdjustmentPerson `json:"created_by"`
	SubmittedBy            *AdjustmentPerson `json:"submitted_by"`
	ApprovedBy             *AdjustmentPerson `json:"approved_by"`
	CancelledBy            *AdjustmentPerson `json:"cancelled_by"`
	SubmittedAt            *time.Time        `json:"submitted_at"`
	PostedAt               *time.Time        `json:"posted_at"`
	CancelledAt            *time.Time        `json:"cancelled_at"`
	CancelReason           *string           `json:"cancel_reason"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
	// Detail only.
	Lines  []AdjustmentLine  `json:"lines,omitempty"`
	Photos []AdjustmentPhoto `json:"photos,omitempty"`
	// CanApprove: the caller may approve it now (detail only).
	CanApprove *bool `json:"can_approve,omitempty"`
}

// AdjustmentSettings are the organization's second approval thresholds.
type AdjustmentSettings struct {
	ThresholdQuantity int32  `json:"threshold_quantity"`
	ThresholdMeters   string `json:"threshold_meters"`
}

// AdjustmentInput creates or edits a draft.
type AdjustmentInput struct {
	Type          string
	ReasonCode    *string
	Note          *string
	WarehouseUUID *string
}

// AdjustmentLineInput adds a line: a barcode, with quantity (fixed
// barcode) or meters (part of a roll); location_uuid is the bin a found unit
// goes to or the bin a fixed quantity is taken from.
type AdjustmentLineInput struct {
	Barcode      string
	Quantity     *int32
	Meters       *string
	LocationUUID *string
}

// AdjustmentPhotoInput is an uploaded photo.
type AdjustmentPhotoInput struct {
	Body io.Reader
	Size int64
}

// AdjustmentSort is the sort contract of GET /v1/warehouse/adjustments.
var AdjustmentSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "adjustment_no": "adjustment_no", "type": "type", "status": "status",
		"total_quantity": "total_quantity", "total_meters": "total_meters",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// AdjustmentListFilter is the parsed list query.
type AdjustmentListFilter struct {
	Statuses       []string
	Types          []string
	Sources        []string
	WarehouseUUIDs []uuid.UUID
	UUIDs          []uuid.UUID
	CreatedFrom    *time.Time
	CreatedBefore  *time.Time
	Q              string
	Sort           apiquery.ResolvedSort
	Limit, Offset  int32
}

// ParseAdjustmentListFilter reads status, type, source, warehouse_uuid,
// uuid (CSV), created_from / created_to, q, sort, limit and offset.
func ParseAdjustmentListFilter(values map[string][]string) (AdjustmentListFilter, error) {
	q := apiquery.Parse(values)
	f := AdjustmentListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", AdjStatusDraft, AdjStatusPending, AdjStatusPosted, AdjStatusCancelled); err != nil {
		return f, err
	}
	if f.Types, err = apiquery.EnumList(values, "type", AdjustmentTypes...); err != nil {
		return f, err
	}
	if f.Sources, err = apiquery.EnumList(values, "source", AdjSourceManual, AdjSourceReturn); err != nil {
		return f, err
	}
	if f.WarehouseUUIDs, err = uuidList(values, "warehouse_uuid"); err != nil {
		return f, err
	}
	if f.UUIDs, err = uuidList(values, QueryExportUUIDs); err != nil {
		return f, err
	}
	if f.CreatedFrom, f.CreatedBefore, err = createdRange(values); err != nil {
		return f, err
	}
	f.Sort, err = apiquery.ResolveSort(q.Sort, AdjustmentSort)
	return f, err
}

// ---------------------------------------------------------------------------
// Helpers.

func (s *Adjustments) inTx(ctx context.Context, fn func(tx pgx.Tx, q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx, s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// canAdjust reports whether the caller holds stock.adjust for org.
func (s *Adjustments) canAdjust(ctx context.Context, c ScanCaller, org int64) bool {
	f, err := scopefilter.Resolve(ctx, s.q, c.Principal, &c.Org, rbac.PermStockAdjust)
	return err == nil && !f.UserOnly() && f.AllowsOrg(org, c.Org.BrandID)
}

func (s *Adjustments) lock(ctx context.Context, q *db.Queries, c ScanCaller, org int64, id uuid.UUID) (db.StockAdjustment, error) {
	a, err := q.LockStockAdjustmentByUUID(ctx, db.LockStockAdjustmentByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return db.StockAdjustment{}, notFound(err, ErrAdjustmentNotFound)
	}
	if a.WarehouseID.Valid {
		if err := requireMember(ctx, q, c.Principal, org, a.WarehouseID.Int64); err != nil {
			return db.StockAdjustment{}, err
		}
	}
	return a, nil
}

func optText(field string, raw *string, maxLen int) (pgtype.Text, error) {
	if raw == nil {
		return pgtype.Text{}, nil
	}
	v := strings.TrimSpace(*raw)
	if v == "" {
		return pgtype.Text{}, nil
	}
	if utf8.RuneCountInString(v) > maxLen {
		return pgtype.Text{}, invalid(field, fmt.Sprintf("must be at most %d characters", maxLen))
	}
	return pgtype.Text{String: v, Valid: true}, nil
}

func (s *Adjustments) header(ctx context.Context, q *db.Queries, c ScanCaller, org int64, in AdjustmentInput) (string, pgtype.Text, pgtype.Text, pgtype.Int8, error) {
	typ := strings.TrimSpace(in.Type)
	if !slices.Contains(AdjustmentTypes, typ) {
		return "", pgtype.Text{}, pgtype.Text{}, pgtype.Int8{}, invalid("type", "must be one of "+strings.Join(AdjustmentTypes, ", "))
	}
	reason, err := optText("reason_code", in.ReasonCode, maxAdjustmentReason)
	if err != nil {
		return "", pgtype.Text{}, pgtype.Text{}, pgtype.Int8{}, err
	}
	note, err := optText("note", in.Note, maxAdjustmentNote)
	if err != nil {
		return "", pgtype.Text{}, pgtype.Text{}, pgtype.Int8{}, err
	}
	var wh pgtype.Int8
	if in.WarehouseUUID != nil && strings.TrimSpace(*in.WarehouseUUID) != "" {
		id, err := parseUUIDField("warehouse_uuid", *in.WarehouseUUID)
		if err != nil {
			return "", pgtype.Text{}, pgtype.Text{}, pgtype.Int8{}, err
		}
		w, err := q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: id, OrganizationID: org})
		if err != nil {
			return "", pgtype.Text{}, pgtype.Text{}, pgtype.Int8{}, notFound(err, ErrWarehouseNotFound)
		}
		if err := requireMember(ctx, q, c.Principal, org, w.ID); err != nil {
			return "", pgtype.Text{}, pgtype.Text{}, pgtype.Int8{}, err
		}
		wh = i8(w.ID)
	}
	return typ, reason, note, wh, nil
}

// settings returns the thresholds of org (defaults while none are saved).
func (s *Adjustments) settings(ctx context.Context, q *db.Queries, org int64) (int32, int64, error) {
	st, err := q.GetStockAdjustmentSettings(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		cm, _ := ledger.ParseMeters(DefaultThresholdMeters)
		return DefaultThresholdQuantity, cm, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("warehouse: adjustment settings: %w", err)
	}
	cm, _ := numToCm(st.ThresholdMeters)
	return st.ThresholdQuantity, cm, nil
}

// totals sums the lines: pieces (whole pieces and fixed quantities) and
// centimeters (roll meters and whole rolls).
func totals(lines []db.ListStockAdjustmentLinesRow) (int32, int64) {
	var qty int32
	var cm int64
	for _, l := range lines {
		switch l.Kind {
		case AdjLineQuantity:
			qty += l.Quantity.Int32
		case AdjLineMeters:
			v, _ := numToCm(l.Meters)
			cm += v
		case AdjLineWhole:
			if l.IsRoll {
				v, _ := numToCm(l.RollMeters)
				cm += v
			} else {
				qty++
			}
		}
	}
	return qty, cm
}

func (s *Adjustments) refreshTotals(ctx context.Context, q *db.Queries, a db.StockAdjustment) error {
	lines, err := q.ListStockAdjustmentLines(ctx, a.ID)
	if err != nil {
		return fmt.Errorf("warehouse: adjustment lines: %w", err)
	}
	qty, cm := totals(lines)
	_, err = q.UpdateStockAdjustmentTotals(ctx, db.UpdateStockAdjustmentTotalsParams{
		ID: a.ID, TotalQuantity: qty, TotalMeters: cmToNum(cm),
	})
	return err
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func metersPtr(n pgtype.Numeric) *string {
	cm, ok := numToCm(n)
	if !ok {
		return nil
	}
	s := ledger.FormatMeters(cm)
	return &s
}

func (s *Adjustments) view(ctx context.Context, q *db.Queries, a db.StockAdjustment) (StockAdjustment, error) {
	v := StockAdjustment{
		UUID: a.Uuid, AdjustmentNo: a.AdjustmentNo, Type: a.Type, ReasonCode: textPtr(a.ReasonCode), Note: textPtr(a.Note),
		Source: a.Source, Status: a.Status, TotalQuantity: a.TotalQuantity, RequiresSecondApproval: a.RequiresSecondApproval,
		SubmittedAt: timePtr(a.SubmittedAt), PostedAt: timePtr(a.PostedAt), CancelledAt: timePtr(a.CancelledAt),
		CancelReason: textPtr(a.CancelReason), CreatedAt: a.CreatedAt.Time, UpdatedAt: a.UpdatedAt.Time,
		TotalMeters: "0.00",
	}
	if m := metersPtr(a.TotalMeters); m != nil {
		v.TotalMeters = *m
	}
	if a.WarehouseID.Valid {
		w, err := q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: a.WarehouseID.Int64, OrganizationID: a.OrganizationID})
		if err != nil {
			return v, fmt.Errorf("warehouse: adjustment warehouse: %w", err)
		}
		v.Warehouse = &CountWarehouse{UUID: w.Uuid, Code: w.Code, Name: w.Name}
	}
	ids := make([]int64, 0, 4)
	for _, u := range []pgtype.Int8{a.CreatedByUserID, a.SubmittedByUserID, a.ApprovedByUserID, a.CancelledByUserID} {
		if u.Valid {
			ids = append(ids, u.Int64)
		}
	}
	if len(ids) > 0 {
		users, err := q.ListUsersByIDs(ctx, ids)
		if err != nil {
			return v, fmt.Errorf("warehouse: adjustment users: %w", err)
		}
		person := func(id pgtype.Int8) *AdjustmentPerson {
			if !id.Valid {
				return nil
			}
			for _, u := range users {
				if u.ID == id.Int64 {
					return &AdjustmentPerson{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}
				}
			}
			return nil
		}
		v.CreatedBy, v.SubmittedBy = person(a.CreatedByUserID), person(a.SubmittedByUserID)
		v.ApprovedBy, v.CancelledBy = person(a.ApprovedByUserID), person(a.CancelledByUserID)
	}
	return v, nil
}

func (s *Adjustments) detail(ctx context.Context, q *db.Queries, c ScanCaller, a db.StockAdjustment) (StockAdjustment, error) {
	v, err := s.view(ctx, q, a)
	if err != nil {
		return v, err
	}
	lines, err := q.ListStockAdjustmentLines(ctx, a.ID)
	if err != nil {
		return v, fmt.Errorf("warehouse: adjustment lines: %w", err)
	}
	v.Lines = make([]AdjustmentLine, 0, len(lines))
	for _, l := range lines {
		line := AdjustmentLine{
			UUID: l.Uuid, Kind: l.Kind,
			Unit:    AdjustmentUnit{UUID: l.UnitUuid, Barcode: l.Barcode, UnitKind: l.UnitKind, IsRoll: l.IsRoll},
			Product: CountProduct{UUID: l.ProductUuid, Name: l.ProductName, SKU: l.ProductSku},
			Meters:  metersPtr(l.Meters), RollMeters: metersPtr(l.RollMeters), Posted: l.MovementID.Valid,
		}
		if l.Quantity.Valid {
			qty := l.Quantity.Int32
			line.Quantity = &qty
		}
		if l.LocationUuid.Valid {
			line.Location = &AdjustmentLocation{UUID: l.LocationUuid.Bytes, FullCode: l.LocationFullCode.String}
		}
		v.Lines = append(v.Lines, line)
	}
	photos, err := q.ListStockAdjustmentPhotos(ctx, a.ID)
	if err != nil {
		return v, fmt.Errorf("warehouse: adjustment photos: %w", err)
	}
	v.Photos = make([]AdjustmentPhoto, 0, len(photos))
	for _, p := range photos {
		v.Photos = append(v.Photos, AdjustmentPhoto{UUID: p.Uuid, ContentType: p.ContentType, SizeBytes: p.SizeBytes, CreatedAt: p.CreatedAt.Time})
	}
	can := a.Status == AdjStatusPending && s.canAdjust(ctx, c, a.OrganizationID) && !selfApproval(a, c.Principal.UserInternal)
	v.CanApprove = &can
	return v, nil
}

// selfApproval: above the threshold the creator and the submitter may not
// approve.
func selfApproval(a db.StockAdjustment, actor int64) bool {
	if !a.RequiresSecondApproval || actor <= 0 {
		return false
	}
	return (a.CreatedByUserID.Valid && a.CreatedByUserID.Int64 == actor) ||
		(a.SubmittedByUserID.Valid && a.SubmittedByUserID.Int64 == actor)
}

// ---------------------------------------------------------------------------
// Settings.

// Settings returns the second approval thresholds of the organization.
func (s *Adjustments) Settings(ctx context.Context, c ScanCaller) (AdjustmentSettings, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return AdjustmentSettings{}, err
	}
	qty, cm, err := s.settings(ctx, s.q, org)
	if err != nil {
		return AdjustmentSettings{}, err
	}
	return AdjustmentSettings{ThresholdQuantity: qty, ThresholdMeters: ledger.FormatMeters(cm)}, nil
}

// UpdateSettings writes the thresholds (stock.adjust).
func (s *Adjustments) UpdateSettings(ctx context.Context, c ScanCaller, in AdjustmentSettings) (AdjustmentSettings, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return AdjustmentSettings{}, err
	}
	if !s.canAdjust(ctx, c, org) {
		return AdjustmentSettings{}, ErrAdjustmentForbidden
	}
	if in.ThresholdQuantity < 0 || in.ThresholdQuantity > MaxAdjustmentQuantity {
		return AdjustmentSettings{}, invalid("threshold_quantity", fmt.Sprintf("must be between 0 and %d", MaxAdjustmentQuantity))
	}
	cm, err := ledger.ParseMeters(strings.TrimSpace(in.ThresholdMeters))
	if err != nil || cm < 0 || cm > 99999999 {
		return AdjustmentSettings{}, invalid("threshold_meters", "must be meters >= 0 (2 decimals)")
	}
	if _, err := s.q.UpsertStockAdjustmentSettings(ctx, db.UpsertStockAdjustmentSettingsParams{
		OrganizationID: org, ThresholdQuantity: in.ThresholdQuantity, ThresholdMeters: cmToNum(cm), UserID: userArg(c),
	}); err != nil {
		return AdjustmentSettings{}, fmt.Errorf("warehouse: adjustment settings: %w", err)
	}
	return s.Settings(ctx, c)
}

// ---------------------------------------------------------------------------
// Documents.

// Create opens a draft.
func (s *Adjustments) Create(ctx context.Context, c ScanCaller, in AdjustmentInput) (StockAdjustment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockAdjustment{}, err
	}
	typ, reason, note, wh, err := s.header(ctx, s.q, c, org, in)
	if err != nil {
		return StockAdjustment{}, err
	}
	a, err := s.q.CreateStockAdjustment(ctx, db.CreateStockAdjustmentParams{
		OrganizationID: org, BrandID: c.Org.BrandID, WarehouseID: wh, Type: typ, ReasonCode: reason, Note: note,
		Source: AdjSourceManual, UserID: userArg(c),
	})
	if err != nil {
		return StockAdjustment{}, fmt.Errorf("warehouse: create adjustment: %w", err)
	}
	return s.detail(ctx, s.q, c, a)
}

// Update edits a draft's header.
func (s *Adjustments) Update(ctx context.Context, c ScanCaller, id uuid.UUID, in AdjustmentInput) (StockAdjustment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockAdjustment{}, err
	}
	var out db.StockAdjustment
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		a, err := s.lock(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		if a.Status != AdjStatusDraft {
			return ErrAdjustmentStatus
		}
		typ, reason, note, wh, err := s.header(ctx, q, c, org, in)
		if err != nil {
			return err
		}
		if (typ == AdjTypeFound) != (a.Type == AdjTypeFound) {
			n, err := q.CountStockAdjustmentLines(ctx, a.ID)
			if err != nil {
				return err
			}
			if n > 0 {
				return invalid("type", "remove the lines before switching between found and a loss type")
			}
		}
		out, err = q.UpdateStockAdjustmentDraft(ctx, db.UpdateStockAdjustmentDraftParams{
			ID: a.ID, Type: typ, ReasonCode: reason, Note: note, WarehouseID: wh,
		})
		return err
	})
	if err != nil {
		return StockAdjustment{}, err
	}
	return s.detail(ctx, s.q, c, out)
}

// List lists the organization's documents.
func (s *Adjustments) List(ctx context.Context, c ScanCaller, f AdjustmentListFilter) ([]StockAdjustment, int64, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return nil, 0, err
	}
	f.Sort = orDefault(f.Sort, AdjustmentSort)
	cp := db.CountStockAdjustmentsParams{
		OrganizationID: org, Statuses: f.Statuses, Types: f.Types, Sources: f.Sources, WarehouseUuids: f.WarehouseUUIDs,
		Uuids: f.UUIDs, CreatedFrom: listTS(f.CreatedFrom), CreatedBefore: listTS(f.CreatedBefore), Q: listQ(f.Q),
	}
	rows, err := s.q.ListStockAdjustments(ctx, db.ListStockAdjustmentsParams{
		OrganizationID: cp.OrganizationID, Statuses: cp.Statuses, Types: cp.Types, Sources: cp.Sources,
		WarehouseUuids: cp.WarehouseUuids, Uuids: cp.Uuids, CreatedFrom: cp.CreatedFrom, CreatedBefore: cp.CreatedBefore,
		Q: cp.Q, SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("warehouse: list adjustments: %w", err)
	}
	total, err := s.q.CountStockAdjustments(ctx, cp)
	if err != nil {
		return nil, 0, fmt.Errorf("warehouse: count adjustments: %w", err)
	}
	out := make([]StockAdjustment, 0, len(rows))
	for _, r := range rows {
		v, err := s.view(ctx, s.q, r)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, nil
}

// Get returns a document with its lines and photos.
func (s *Adjustments) Get(ctx context.Context, c ScanCaller, id uuid.UUID) (StockAdjustment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockAdjustment{}, err
	}
	a, err := s.q.GetStockAdjustmentByUUID(ctx, db.GetStockAdjustmentByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return StockAdjustment{}, notFound(err, ErrAdjustmentNotFound)
	}
	return s.detail(ctx, s.q, c, a)
}

// AddLine adds a unit to a draft.
func (s *Adjustments) AddLine(ctx context.Context, c ScanCaller, id uuid.UUID, in AdjustmentLineInput) (StockAdjustment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockAdjustment{}, err
	}
	var out db.StockAdjustment
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		a, err := s.lock(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		if a.Status != AdjStatusDraft {
			return ErrAdjustmentStatus
		}
		n, err := q.CountStockAdjustmentLines(ctx, a.ID)
		if err != nil {
			return err
		}
		if n >= MaxAdjustmentLines {
			return invalid("barcode", fmt.Sprintf("a document holds at most %d lines", MaxAdjustmentLines))
		}
		arg, err := s.buildLine(ctx, q, c, a, in)
		if err != nil {
			return err
		}
		if _, err := q.InsertStockAdjustmentLine(ctx, arg); err != nil {
			if isUnique(err) {
				return invalid("barcode", "the unit is already on this document")
			}
			return fmt.Errorf("warehouse: adjustment line: %w", err)
		}
		if err := s.refreshTotals(ctx, q, a); err != nil {
			return err
		}
		out, err = q.GetStockAdjustmentByUUID(ctx, db.GetStockAdjustmentByUUIDParams{Uuid: a.Uuid, OrganizationID: org})
		return err
	})
	if err != nil {
		return StockAdjustment{}, err
	}
	return s.detail(ctx, s.q, c, out)
}

// heldUnit finds the unit of barcode the organization holds (found: the
// unit voided into the organization's trash).
func heldUnit(ctx context.Context, q *db.Queries, org int64, barcode string, found bool) (db.Unit, *db.UnitCurrentState, error) {
	units, err := q.ListUnitsByBarcode(ctx, barcode)
	if err != nil {
		return db.Unit{}, nil, fmt.Errorf("warehouse: unit: %w", err)
	}
	if len(units) == 0 {
		return db.Unit{}, nil, unitErr("no unit has this barcode")
	}
	for _, u := range units {
		if u.UnitKind == ledger.KindFixed {
			holdings, err := q.ListFixedBarcodeHoldingsByUnit(ctx, u.ID)
			if err != nil {
				return db.Unit{}, nil, fmt.Errorf("warehouse: holdings: %w", err)
			}
			for _, h := range holdings {
				if h.HolderOrgID == org {
					return u, nil, nil
				}
			}
			continue
		}
		st, err := q.GetUnitCurrentState(ctx, u.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return db.Unit{}, nil, fmt.Errorf("warehouse: unit state: %w", err)
		}
		if st.HolderOrgID == org {
			return u, &st, nil
		}
	}
	if found {
		return db.Unit{}, nil, unitErr("the unit was not voided by this organization")
	}
	return db.Unit{}, nil, unitErr("the unit is not in this organization's stock")
}

// lineLocation resolves a bin of the organization.
func lineLocation(ctx context.Context, q *db.Queries, c ScanCaller, org int64, raw *string) (*db.WarehouseLocation, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	id, err := parseUUIDField("location_uuid", *raw)
	if err != nil {
		return nil, err
	}
	l, err := q.GetWarehouseLocationByUUID(ctx, id)
	if err != nil || l.OrganizationID != org || !l.Active {
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrLocationNotFound
		}
		return nil, err
	}
	if l.WarehouseID.Valid {
		if err := requireMember(ctx, q, c.Principal, org, l.WarehouseID.Int64); err != nil {
			return nil, err
		}
	}
	return &l, nil
}

func (s *Adjustments) buildLine(ctx context.Context, q *db.Queries, c ScanCaller, a db.StockAdjustment, in AdjustmentLineInput) (db.InsertStockAdjustmentLineParams, error) {
	org := a.OrganizationID
	found := a.Type == AdjTypeFound
	barcode := strings.TrimSpace(in.Barcode)
	if barcode == "" {
		return db.InsertStockAdjustmentLineParams{}, invalid("barcode", "is required")
	}
	u, st, err := heldUnit(ctx, q, org, barcode, found)
	if err != nil {
		return db.InsertStockAdjustmentLineParams{}, err
	}
	if u, err = q.LockUnit(ctx, u.ID); err != nil {
		return db.InsertStockAdjustmentLineParams{}, fmt.Errorf("warehouse: lock unit: %w", err)
	}
	busy, err := q.CountOpenAdjustmentLinesByUnit(ctx, db.CountOpenAdjustmentLinesByUnitParams{UnitID: u.ID, ExcludeAdjustmentID: a.ID})
	if err != nil {
		return db.InsertStockAdjustmentLineParams{}, err
	}
	if busy > 0 {
		return db.InsertStockAdjustmentLineParams{}, fmt.Errorf("%w: another open adjustment holds %s", ErrAdjustmentUnitBusy, barcode)
	}
	loc, err := lineLocation(ctx, q, c, org, in.LocationUUID)
	if err != nil {
		return db.InsertStockAdjustmentLineParams{}, err
	}
	arg := db.InsertStockAdjustmentLineParams{AdjustmentID: a.ID, OrganizationID: org, UnitID: u.ID, ProductID: u.ProductID}
	if loc != nil {
		arg.LocationID = i8(loc.ID)
	}

	if u.UnitKind == ledger.KindFixed {
		if in.Meters != nil {
			return arg, invalid("meters", "a fixed barcode has no meters")
		}
		if in.Quantity == nil || *in.Quantity <= 0 || *in.Quantity > MaxAdjustmentQuantity {
			return arg, invalid("quantity", fmt.Sprintf("must be between 1 and %d", MaxAdjustmentQuantity))
		}
		if !found {
			holdings, err := q.ListFixedBarcodeHoldingsByUnit(ctx, u.ID)
			if err != nil {
				return arg, err
			}
			var have int32
			for _, h := range holdings {
				if h.HolderOrgID != org {
					continue
				}
				if loc == nil || (h.OwnerType == string(ledger.OwnerWarehouseLocation) && h.OwnerID == loc.ID) {
					have = max(have, h.QuantityOnHand)
				}
			}
			if *in.Quantity > have {
				return arg, unitErr(fmt.Sprintf("only %d on hand in one place for this barcode", have))
			}
		}
		arg.Kind, arg.Quantity = AdjLineQuantity, pgtype.Int4{Int32: *in.Quantity, Valid: true}
		return arg, nil
	}

	if in.Quantity != nil && *in.Quantity != 1 {
		return arg, invalid("quantity", "a serial unit is adjusted whole or by meters")
	}
	isRoll := u.InitialMeters.Valid
	if in.Meters != nil && strings.TrimSpace(*in.Meters) != "" {
		if !isRoll {
			return arg, invalid("meters", "only a roll is adjusted by meters")
		}
		cm, err := ledger.ParseMeters(strings.TrimSpace(*in.Meters))
		if err != nil || cm <= 0 {
			return arg, invalid("meters", "must be meters > 0 (2 decimals)")
		}
		if st == nil || !stocked(st.Status) {
			return arg, unitErr("the roll is not in stock")
		}
		remaining, _ := numToCm(u.RemainingMeters)
		initial, _ := numToCm(u.InitialMeters)
		if !found && cm >= remaining {
			return arg, invalid("meters", fmt.Sprintf("the roll has %s m; adjust the whole roll instead", ledger.FormatMeters(remaining)))
		}
		if found && remaining+cm > initial {
			return arg, invalid("meters", fmt.Sprintf("the roll cannot exceed its initial %s m", ledger.FormatMeters(initial)))
		}
		if loc != nil {
			return arg, invalid("location_uuid", "a meter adjustment keeps the roll where it is")
		}
		arg.Kind, arg.Meters = AdjLineMeters, cmToNum(cm)
		return arg, nil
	}

	if st == nil {
		return arg, unitErr("the unit is not in this organization's stock")
	}
	if found {
		if st.Status != string(ledger.StatusVoid) || st.OwnerType != string(ledger.OwnerTrash) {
			return arg, unitErr("only a voided unit can be found")
		}
	} else {
		if !stocked(st.Status) && st.Status != string(ledger.StatusQuarantine) {
			return arg, unitErr("the unit is not in stock (status " + st.Status + ")")
		}
		if loc != nil {
			return arg, invalid("location_uuid", "only a found unit takes a bin")
		}
		res, err := q.LockActiveReservationsByUnit(ctx, u.ID)
		if err != nil {
			return arg, err
		}
		if len(res) > 0 {
			return arg, fmt.Errorf("%w: an order reserves %s", ErrAdjustmentUnitBusy, barcode)
		}
		open, err := q.CountOpenWarehouseTransferLinesByUnit(ctx, u.ID)
		if err != nil {
			return arg, err
		}
		if open > 0 {
			return arg, fmt.Errorf("%w: a warehouse transfer holds %s", ErrAdjustmentUnitBusy, barcode)
		}
	}
	arg.Kind = AdjLineWhole
	if isRoll {
		arg.RollMeters = u.RemainingMeters
	}
	return arg, nil
}

// DeleteLine removes a line of a draft.
func (s *Adjustments) DeleteLine(ctx context.Context, c ScanCaller, id, lineID uuid.UUID) (StockAdjustment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockAdjustment{}, err
	}
	var out db.StockAdjustment
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		a, err := s.lock(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		if a.Status != AdjStatusDraft {
			return ErrAdjustmentStatus
		}
		n, err := q.DeleteStockAdjustmentLine(ctx, db.DeleteStockAdjustmentLineParams{Uuid: lineID, AdjustmentID: a.ID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrAdjustmentNotFound
		}
		if err := s.refreshTotals(ctx, q, a); err != nil {
			return err
		}
		out, err = q.GetStockAdjustmentByUUID(ctx, db.GetStockAdjustmentByUUIDParams{Uuid: a.Uuid, OrganizationID: org})
		return err
	})
	if err != nil {
		return StockAdjustment{}, err
	}
	return s.detail(ctx, s.q, c, out)
}

// Submit sends a draft for approval and freezes its totals.
func (s *Adjustments) Submit(ctx context.Context, c ScanCaller, id uuid.UUID) (StockAdjustment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockAdjustment{}, err
	}
	var out db.StockAdjustment
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		a, err := s.lock(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		out, err = s.submit(ctx, q, c, a)
		return err
	})
	if err != nil {
		return StockAdjustment{}, err
	}
	return s.detail(ctx, s.q, c, out)
}

func (s *Adjustments) submit(ctx context.Context, q *db.Queries, c ScanCaller, a db.StockAdjustment) (db.StockAdjustment, error) {
	if a.Status != AdjStatusDraft {
		return db.StockAdjustment{}, ErrAdjustmentStatus
	}
	lines, err := q.ListStockAdjustmentLines(ctx, a.ID)
	if err != nil {
		return db.StockAdjustment{}, err
	}
	if len(lines) == 0 {
		return db.StockAdjustment{}, ErrAdjustmentEmpty
	}
	if a.Type == AdjTypeDamage {
		n, err := q.CountStockAdjustmentPhotos(ctx, a.ID)
		if err != nil {
			return db.StockAdjustment{}, err
		}
		if n == 0 {
			return db.StockAdjustment{}, ErrAdjustmentPhotoNeeded
		}
	}
	qty, cm := totals(lines)
	thrQ, thrCm, err := s.settings(ctx, q, a.OrganizationID)
	if err != nil {
		return db.StockAdjustment{}, err
	}
	return q.SubmitStockAdjustment(ctx, db.SubmitStockAdjustmentParams{
		ID: a.ID, UserID: userArg(c), TotalQuantity: qty, TotalMeters: cmToNum(cm),
		RequiresSecondApproval: qty > thrQ || cm > thrCm,
	})
}

// Approve posts a pending document to the ledger (stock.adjust; above the
// threshold not by its creator or submitter).
func (s *Adjustments) Approve(ctx context.Context, c ScanCaller, id uuid.UUID) (StockAdjustment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockAdjustment{}, err
	}
	if !s.canAdjust(ctx, c, org) {
		return StockAdjustment{}, ErrAdjustmentForbidden
	}
	var out db.StockAdjustment
	err = s.inTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		a, err := s.lock(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		if a.Status != AdjStatusPending {
			return ErrAdjustmentStatus
		}
		if selfApproval(a, c.Principal.UserInternal) {
			return ErrAdjustmentSelfApproval
		}
		lines, err := q.ListStockAdjustmentLines(ctx, a.ID)
		if err != nil {
			return err
		}
		for _, l := range lines {
			if err := s.post(ctx, tx, q, c, a, l); err != nil {
				return err
			}
		}
		out, err = q.PostStockAdjustment(ctx, db.PostStockAdjustmentParams{ID: a.ID, UserID: userArg(c)})
		return err
	})
	if err != nil {
		return StockAdjustment{}, err
	}
	return s.detail(ctx, s.q, c, out)
}

func (s *Adjustments) post(ctx context.Context, tx pgx.Tx, q *db.Queries, c ScanCaller, a db.StockAdjustment, l db.ListStockAdjustmentLinesRow) error {
	org := a.OrganizationID
	reason := a.Type
	if a.ReasonCode.Valid {
		reason += ": " + a.ReasonCode.String
	}
	m := ledger.Movement{
		UnitID: l.UnitID, Source: adjustmentSource, RefType: adjustmentRefType, RefID: l.ID,
		ActorUserID: actorPtr(EntryCaller(c)), Reason: reason,
		Metadata: map[string]any{
			"adjustment_uuid": a.Uuid.String(), "adjustment_no": a.AdjustmentNo, "adjustment_type": a.Type,
		},
	}
	target := countOwner(org, l.LocationID)
	found := a.Type == AdjTypeFound
	switch l.Kind {
	case AdjLineWhole:
		if found {
			m.Type, m.To = ledger.TypeFound, &target
		} else {
			m.Type = ledger.TypeVoid
		}
	case AdjLineMeters:
		cm, _ := numToCm(l.Meters)
		if !found {
			cm = -cm
		}
		m.Type, m.Centimeters = ledger.TypeCountAdjustment, cm
	case AdjLineQuantity:
		m.Type = ledger.TypeCountAdjustment
		if found {
			m.To, m.Quantity = &target, l.Quantity.Int32
		} else {
			src, err := adjustmentFixedSource(ctx, q, l.UnitID, org, l.LocationID, l.Quantity.Int32)
			if err != nil {
				return err
			}
			m.From, m.Quantity = &src, -l.Quantity.Int32
		}
	default:
		return fmt.Errorf("warehouse: adjustment line kind %q", l.Kind)
	}
	res, err := s.ledger.Post(ctx, tx, m)
	if err != nil {
		if isLedgerConflict(err) {
			return fmt.Errorf("%w: %s: %v", ErrAdjustmentStale, l.Barcode, err)
		}
		return fmt.Errorf("warehouse: adjustment %s: %w", l.Barcode, err)
	}
	return q.SetStockAdjustmentLineMovement(ctx, db.SetStockAdjustmentLineMovementParams{
		ID: l.ID, MovementID: pgtype.Int8{Int64: res.Movement.ID, Valid: true},
	})
}

// adjustmentFixedSource picks the holding a fixed quantity is taken from:
// the line's bin, else the organization's largest holding of the quantity.
func adjustmentFixedSource(ctx context.Context, q *db.Queries, unitID, org int64, loc pgtype.Int8, qty int32) (ledger.Owner, error) {
	holdings, err := q.ListFixedBarcodeHoldingsByUnit(ctx, unitID)
	if err != nil {
		return ledger.Owner{}, err
	}
	var best *db.FixedBarcodeHolding
	for i := range holdings {
		h := &holdings[i]
		if h.HolderOrgID != org || h.QuantityOnHand < qty {
			continue
		}
		if loc.Valid && (h.OwnerType != string(ledger.OwnerWarehouseLocation) || h.OwnerID != loc.Int64) {
			continue
		}
		if best == nil || h.QuantityOnHand > best.QuantityOnHand {
			best = h
		}
	}
	if best == nil {
		return ledger.Owner{}, fmt.Errorf("%w: no holding of the organization has %d", ErrAdjustmentStale, qty)
	}
	return ledger.Owner{Type: ledger.OwnerType(best.OwnerType), ID: best.OwnerID, OrgID: best.HolderOrgID}, nil
}

// Cancel cancels a draft or pending document (nothing was posted).
func (s *Adjustments) Cancel(ctx context.Context, c ScanCaller, id uuid.UUID, reason *string) (StockAdjustment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockAdjustment{}, err
	}
	why, err := optText("reason", reason, 500)
	if err != nil {
		return StockAdjustment{}, err
	}
	var out db.StockAdjustment
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		a, err := s.lock(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		if a.Status != AdjStatusDraft && a.Status != AdjStatusPending {
			return ErrAdjustmentStatus
		}
		out, err = q.CancelStockAdjustment(ctx, db.CancelStockAdjustmentParams{ID: a.ID, UserID: userArg(c), Reason: why})
		return err
	})
	if err != nil {
		return StockAdjustment{}, err
	}
	return s.detail(ctx, s.q, c, out)
}

// ---------------------------------------------------------------------------
// Photos.

var adjustmentPhotoTypes = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

// AdjustmentPhotoObjectKey is the storage key of a photo.
func AdjustmentPhotoObjectKey(orgUUID, adjUUID, photoUUID uuid.UUID, ext string) string {
	return fmt.Sprintf("stock-adjustments/%s/%s/%s.%s", orgUUID, adjUUID, photoUUID, ext)
}

// AddPhoto stores a photo of a draft or pending document.
func (s *Adjustments) AddPhoto(ctx context.Context, c ScanCaller, id uuid.UUID, in AdjustmentPhotoInput) (StockAdjustment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockAdjustment{}, err
	}
	if s.store == nil {
		return StockAdjustment{}, ErrStorageUnavailable
	}
	if in.Size <= 0 || in.Size > MaxAdjustmentPhotoBytes {
		return StockAdjustment{}, invalid("file", fmt.Sprintf("the photo must be 1 byte to %d MB", MaxAdjustmentPhotoBytes>>20))
	}
	raw, err := io.ReadAll(io.LimitReader(in.Body, MaxAdjustmentPhotoBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > MaxAdjustmentPhotoBytes {
		return StockAdjustment{}, invalid("file", "the photo is empty or too large")
	}
	ct := http.DetectContentType(raw)
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ext, ok := adjustmentPhotoTypes[ct]
	if !ok {
		return StockAdjustment{}, invalid("file", "JPEG, PNG or WebP only")
	}
	var out db.StockAdjustment
	var key string
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		a, err := s.lock(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		if a.Status != AdjStatusDraft && a.Status != AdjStatusPending {
			return ErrAdjustmentStatus
		}
		n, err := q.CountStockAdjustmentPhotos(ctx, a.ID)
		if err != nil {
			return err
		}
		if n >= MaxAdjustmentPhotos {
			return invalid("file", fmt.Sprintf("a document holds at most %d photos", MaxAdjustmentPhotos))
		}
		key = AdjustmentPhotoObjectKey(c.Org.UUID, a.Uuid, uuid.New(), ext)
		if err := s.store.Upload(ctx, storage.File{
			Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: ct,
			Filename: a.AdjustmentNo + "." + ext, Metadata: map[string]string{"adjustment_uuid": a.Uuid.String()},
		}, key); err != nil {
			return fmt.Errorf("warehouse: upload photo: %w", err)
		}
		if _, err := q.InsertStockAdjustmentPhoto(ctx, db.InsertStockAdjustmentPhotoParams{
			AdjustmentID: a.ID, OrganizationID: org, ObjectKey: key, ContentType: ct, SizeBytes: int64(len(raw)), UserID: userArg(c),
		}); err != nil {
			return fmt.Errorf("warehouse: photo row: %w", err)
		}
		out = a
		return nil
	})
	if err != nil {
		if key != "" {
			_ = s.store.Delete(context.WithoutCancel(ctx), key)
		}
		return StockAdjustment{}, err
	}
	return s.detail(ctx, s.q, c, out)
}

// DeletePhoto removes a photo of a draft.
func (s *Adjustments) DeletePhoto(ctx context.Context, c ScanCaller, id, photoID uuid.UUID) (StockAdjustment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockAdjustment{}, err
	}
	var out db.StockAdjustment
	var key string
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		a, err := s.lock(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		if a.Status != AdjStatusDraft {
			return ErrAdjustmentStatus
		}
		p, err := q.GetStockAdjustmentPhoto(ctx, db.GetStockAdjustmentPhotoParams{Uuid: photoID, AdjustmentID: a.ID})
		if err != nil {
			return notFound(err, ErrAdjustmentPhotoNotFound)
		}
		if _, err := q.DeleteStockAdjustmentPhoto(ctx, p.ID); err != nil {
			return err
		}
		key, out = p.ObjectKey, a
		return nil
	})
	if err != nil {
		return StockAdjustment{}, err
	}
	if s.store != nil && key != "" {
		_ = s.store.Delete(ctx, key)
	}
	return s.detail(ctx, s.q, c, out)
}

// PhotoFile is a photo download.
type PhotoFile struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	Filename    string
}

// Photo opens a photo of a document.
func (s *Adjustments) Photo(ctx context.Context, c ScanCaller, id, photoID uuid.UUID) (PhotoFile, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return PhotoFile{}, err
	}
	if s.store == nil {
		return PhotoFile{}, ErrStorageUnavailable
	}
	a, err := s.q.GetStockAdjustmentByUUID(ctx, db.GetStockAdjustmentByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return PhotoFile{}, notFound(err, ErrAdjustmentNotFound)
	}
	p, err := s.q.GetStockAdjustmentPhoto(ctx, db.GetStockAdjustmentPhotoParams{Uuid: photoID, AdjustmentID: a.ID})
	if err != nil {
		return PhotoFile{}, notFound(err, ErrAdjustmentPhotoNotFound)
	}
	body, size, err := s.store.Download(ctx, p.ObjectKey)
	if err != nil {
		return PhotoFile{}, fmt.Errorf("warehouse: photo download: %w", err)
	}
	ext := adjustmentPhotoTypes[p.ContentType]
	return PhotoFile{Body: body, Size: size, ContentType: p.ContentType, Filename: a.AdjustmentNo + "-" + p.Uuid.String()[:8] + "." + ext}, nil
}
