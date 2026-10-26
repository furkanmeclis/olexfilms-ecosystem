// Package usecase implements the bulk barcode operations of TEC-1008
// (super_admin): select units (filters and/or a barcode list), preview what
// an action would touch and what it leaves out (with the reason), run it in
// the background after a step-up, undo it.
//
// Every stock change is a ledger movement written forward (history is
// never rewritten): a piece box becomes a roll with measure_conversion,
// a product change is a reclassification, a move a placement, a void a
// void; lot and note are unit fields. Units that left the center (to a
// dealer with an order or a transfer), units used in a service, in transit,
// reserved by an order or not in stock yet are excluded.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
)

// Actions.
const (
	ActionConvertToRoll = "convert_to_roll"
	ActionChangeProduct = "change_product"
	ActionMoveLocation  = "move_location"
	ActionVoid          = "void"
	ActionSetLot        = "set_lot"
	ActionSetNote       = "set_note"
)

// Actions lists the actions in display order.
var Actions = []string{ActionConvertToRoll, ActionChangeProduct, ActionMoveLocation, ActionVoid, ActionSetLot, ActionSetNote}

// Reversible reports whether a job of the action can be undone.
func Reversible(action string) bool { return action != ActionVoid }

// Job statuses.
const (
	StatusPreview   = "preview"
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusUndoing   = "undoing"
	StatusUndone    = "undone"
	StatusDiscarded = "discarded"
)

// Item statuses.
const (
	ItemEligible      = "eligible"
	ItemExcluded      = "excluded"
	ItemApplied       = "applied"
	ItemFailed        = "failed"
	ItemReverted      = "reverted"
	ItemRevertSkipped = "revert_skipped"
)

// Exclusion and skip reasons (API values; the UI translates them).
const (
	ReasonAtOtherOrg     = "at_other_organization" // went to a dealer / distributor
	ReasonUsedInService  = "used_in_service"
	ReasonConsumed       = "consumed" // used or void
	ReasonInTransit      = "in_transit"
	ReasonReserved       = "reserved_by_order"
	ReasonNotInStock     = "not_in_stock" // label not entered yet
	ReasonFixedBarcode   = "fixed_barcode"
	ReasonAlreadyRoll    = "already_roll"
	ReasonNoMeters       = "no_meters"
	ReasonSameProduct    = "same_product"
	ReasonUnitType       = "unit_type_mismatch"
	ReasonAlreadyThere   = "already_there"
	ReasonChangedSince   = "changed_since"
	ReasonPrevUnplaced   = "previously_unplaced"
	ReasonRollCut        = "roll_cut"
	ReasonNotFoundInList = "not_found"
)

// Limits.
const (
	MaxUnitsPerJob = 5000
	MaxBarcodes    = 5000
	MaxMeters      = 100000 // per roll
	MaxLotLength   = 64
	MaxNoteLength  = 2000
	sampleSize     = 20
	batchSize      = 100
	keySource      = "bulk"
	keyUndoSource  = "bulk-undo"
	keyRefType     = "stock_bulk_job_item"
)

// Errors.
var (
	ErrNotFound      = errors.New("stockbulk: not found")
	ErrInvalidStatus = errors.New("stockbulk: the job is not in a status for this")
	ErrBusy          = errors.New("stockbulk: another job of the organization is running")
	ErrTooMany       = errors.New("stockbulk: too many units selected")
	ErrNothingToRun  = errors.New("stockbulk: no eligible unit")
	ErrNotReversible = errors.New("stockbulk: the action cannot be undone")
)

// ValidationError is an input error of one field.
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// TxBeginner begins a transaction (*pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Enqueuer starts a job in the background (queue: stock:bulk).
type Enqueuer interface {
	EnqueueStockBulk(ctx context.Context, jobID int64, undo bool) error
}

// Service implements the use cases.
type Service struct {
	pool  TxBeginner
	q     *db.Queries
	out   outbox.Enqueuer
	queue Enqueuer
}

// New creates the service.
func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, queue Enqueuer) *Service {
	return &Service{pool: pool, q: q, out: out, queue: queue}
}

// Caller is the super admin in the active (center) organization.
type Caller struct {
	UserID int64
	OrgID  int64
}

// --- Selection and preview -------------------------------------------------

// Selection picks the units: filters, a barcode list or both (AND).
type Selection struct {
	ProductUUIDs  []uuid.UUID `json:"product_uuids,omitempty"`
	WarehouseUUID *uuid.UUID  `json:"warehouse_uuid,omitempty"`
	LocationUUIDs []uuid.UUID `json:"location_uuids,omitempty"`
	Statuses      []string    `json:"statuses,omitempty"`
	CreatedFrom   *time.Time  `json:"created_from,omitempty"`
	CreatedTo     *time.Time  `json:"created_to,omitempty"`
	Lot           string      `json:"lot,omitempty"`
	// EntryUUID is a stock entry (a container delivery, TEC-1007).
	EntryUUID *uuid.UUID `json:"entry_uuid,omitempty"`
	// Barcodes is the pasted / Excel list; a line "BARCODE;12.5" (or a tab
	// or comma) overrides the roll length of that barcode.
	Barcodes []string `json:"barcodes,omitempty"`
}

// Params are the action inputs.
type Params struct {
	// Meters: default roll length of convert_to_roll (else the product's).
	Meters      string     `json:"meters,omitempty"`
	ProductUUID *uuid.UUID `json:"product_uuid,omitempty"`
	// LocationUUID: target bin of move_location.
	LocationUUID *uuid.UUID `json:"location_uuid,omitempty"`
	Lot          *string    `json:"lot,omitempty"`
	Note         *string    `json:"note,omitempty"`
	Reason       string     `json:"reason,omitempty"`
}

// PreviewInput creates a preview job.
type PreviewInput struct {
	Action    string
	Params    Params
	Selection Selection
}

// resolved holds the ids the action and the selection name.
type resolved struct {
	products   []int64
	warehouse  pgtype.Int8
	locations  []int64
	entry      pgtype.Int8
	toProduct  *db.Product
	toLocation *db.WarehouseLocation
	meters     int64 // cm, 0: product default
	lot, note  pgtype.Text
	barcodes   []string
	overrides  map[string]int64 // barcode -> cm
}

// Preview resolves the selection, classifies every unit and stores the job
// with its items (status preview). Nothing changes in stock.
func (s *Service) Preview(ctx context.Context, c Caller, in PreviewInput) (JobView, error) {
	if !slices.Contains(Actions, in.Action) {
		return JobView{}, invalid("action", "unknown action")
	}
	r, err := s.resolve(ctx, c, in)
	if err != nil {
		return JobView{}, err
	}
	filtered := len(r.products) > 0 || r.warehouse.Valid || len(r.locations) > 0 || len(in.Selection.Statuses) > 0 ||
		in.Selection.CreatedFrom != nil || in.Selection.CreatedTo != nil || r.lot.Valid && in.Selection.Lot != "" ||
		r.entry.Valid || len(r.barcodes) > 0
	if !filtered {
		return JobView{}, invalid("selection", "pick units with a filter or a barcode list")
	}
	arg := db.ListStockBulkCandidatesParams{
		OrganizationID: c.OrgID, Barcodes: r.barcodes, ProductIds: r.products, WarehouseID: r.warehouse,
		LocationIds: r.locations, Statuses: in.Selection.Statuses, EntryID: r.entry,
		RowLimit: MaxUnitsPerJob + 1,
	}
	if in.Selection.CreatedFrom != nil {
		arg.CreatedFrom = pgtype.Timestamptz{Time: *in.Selection.CreatedFrom, Valid: true}
	}
	if in.Selection.CreatedTo != nil {
		arg.CreatedBefore = pgtype.Timestamptz{Time: in.Selection.CreatedTo.AddDate(0, 0, 1), Valid: true}
	}
	if l := strings.TrimSpace(in.Selection.Lot); l != "" {
		arg.Lot = pgtype.Text{String: l, Valid: true}
	}
	rows, err := s.q.ListStockBulkCandidates(ctx, arg)
	if err != nil {
		return JobView{}, fmt.Errorf("stockbulk: candidates: %w", err)
	}
	if len(rows) > MaxUnitsPerJob {
		return JobView{}, fmt.Errorf("%w: more than %d", ErrTooMany, MaxUnitsPerJob)
	}

	type item struct {
		UnitID int64   `json:"unit_id"`
		Status string  `json:"status"`
		Reason string  `json:"reason"`
		Meters *string `json:"meters"`
	}
	items := make([]item, 0, len(rows))
	found := map[string]bool{}
	eligible, excluded := 0, 0
	for _, u := range rows {
		found[u.Barcode] = true
		reason, cm := classify(in.Action, c.OrgID, u, r)
		it := item{UnitID: u.ID, Status: ItemEligible, Reason: reason}
		if reason != "" {
			it.Status = ItemExcluded
			excluded++
		} else {
			eligible++
		}
		if cm > 0 {
			m := ledger.FormatMeters(cm)
			it.Meters = &m
		}
		items = append(items, it)
	}
	var missing []string
	for _, b := range r.barcodes {
		if !found[b] {
			missing = append(missing, b)
		}
	}
	params, _ := json.Marshal(in.Params)
	selection := in.Selection
	selection.Barcodes = nil // the units are the items; keep the job row small
	sel, _ := json.Marshal(struct {
		Selection
		BarcodeCount int      `json:"barcode_count,omitempty"`
		Missing      []string `json:"missing,omitempty"`
	}{selection, len(r.barcodes), missing})
	var job db.StockBulkJob
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		job, err = q.CreateStockBulkJob(ctx, db.CreateStockBulkJobParams{
			OrganizationID: c.OrgID, Action: in.Action, Params: params, Selection: sel,
			Total: int32(len(rows)), Eligible: int32(eligible), Excluded: int32(excluded), CreatedBy: c.UserID,
		})
		if err != nil {
			return fmt.Errorf("stockbulk: create job: %w", err)
		}
		if len(items) == 0 {
			return nil
		}
		body, _ := json.Marshal(items)
		if err := q.InsertStockBulkJobItems(ctx, db.InsertStockBulkJobItemsParams{
			JobID: job.ID, OrganizationID: c.OrgID, Items: body,
		}); err != nil {
			return fmt.Errorf("stockbulk: items: %w", err)
		}
		return nil
	})
	if err != nil {
		return JobView{}, err
	}
	return s.view(ctx, job, true)
}

// classify returns why the unit is left out ("" : eligible) and the roll
// length a conversion would give it (cm).
func classify(action string, orgID int64, u db.ListStockBulkCandidatesRow, r resolved) (string, int64) {
	status := u.StateStatus
	if status == "" {
		status = u.UnitStatus
	}
	lotOrNote := action == ActionSetLot || action == ActionSetNote
	switch {
	case u.InService:
		return ReasonUsedInService, 0
	case status == "used" || status == "void":
		return ReasonConsumed, 0
	case u.HolderOrgID != 0 && u.HolderOrgID != orgID:
		return ReasonAtOtherOrg, 0
	case status == "in_transit":
		return ReasonInTransit, 0
	case u.Reserved:
		return ReasonReserved, 0
	case u.OwnerType == "" && !lotOrNote:
		return ReasonNotInStock, 0
	case u.UnitKind == ledger.KindFixed && !lotOrNote:
		return ReasonFixedBarcode, 0
	}
	switch action {
	case ActionConvertToRoll:
		if u.InitialMeters.Valid {
			return ReasonAlreadyRoll, 0
		}
		cm := r.overrides[u.Barcode]
		if cm == 0 {
			cm = r.meters
		}
		if cm == 0 && u.DefaultRollMeters.Valid {
			cm, _ = ledger.NumericToCentimeters(u.DefaultRollMeters)
		}
		if cm <= 0 {
			return ReasonNoMeters, 0
		}
		return "", cm
	case ActionChangeProduct:
		if r.toProduct == nil {
			return ReasonUnitType, 0
		}
		if u.ProductID == r.toProduct.ID {
			return ReasonSameProduct, 0
		}
		if u.UsesFixedBarcode != r.toProduct.UsesFixedBarcode || u.BrandID != r.toProduct.BrandID {
			return ReasonUnitType, 0
		}
	case ActionMoveLocation:
		if u.OwnerType == string(ledger.OwnerWarehouseLocation) && r.toLocation != nil && u.OwnerID == r.toLocation.ID {
			return ReasonAlreadyThere, 0
		}
	}
	return "", 0
}

// resolve validates the action inputs and turns uuids into ids.
func (s *Service) resolve(ctx context.Context, c Caller, in PreviewInput) (resolved, error) {
	r := resolved{overrides: map[string]int64{}}
	sel, p := in.Selection, in.Params
	for _, id := range sel.ProductUUIDs {
		pid, err := s.q.GetProductIDByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return r, invalid("selection.product_uuids", "product not found")
		}
		if err != nil {
			return r, fmt.Errorf("stockbulk: product: %w", err)
		}
		r.products = append(r.products, pid)
	}
	if sel.WarehouseUUID != nil {
		w, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: *sel.WarehouseUUID, OrganizationID: c.OrgID})
		if errors.Is(err, pgx.ErrNoRows) {
			return r, invalid("selection.warehouse_uuid", "warehouse not found")
		}
		if err != nil {
			return r, fmt.Errorf("stockbulk: warehouse: %w", err)
		}
		r.warehouse = pgtype.Int8{Int64: w.ID, Valid: true}
	}
	for _, id := range sel.LocationUUIDs {
		l, err := s.q.GetWarehouseLocationByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && l.OrganizationID != c.OrgID) {
			return r, invalid("selection.location_uuids", "location not found")
		}
		if err != nil {
			return r, fmt.Errorf("stockbulk: location: %w", err)
		}
		r.locations = append(r.locations, l.ID)
	}
	for _, st := range sel.Statuses {
		if !slices.Contains([]string{"reserved", "printed", "available", "placed", "in_transit", "used", "void"}, st) {
			return r, invalid("selection.statuses", "unknown status")
		}
	}
	if sel.EntryUUID != nil {
		e, err := s.q.GetStockEntryByUUID(ctx, db.GetStockEntryByUUIDParams{Uuid: *sel.EntryUUID, OrganizationID: c.OrgID})
		if errors.Is(err, pgx.ErrNoRows) {
			return r, invalid("selection.entry_uuid", "stock entry not found")
		}
		if err != nil {
			return r, fmt.Errorf("stockbulk: entry: %w", err)
		}
		r.entry = pgtype.Int8{Int64: e.ID, Valid: true}
	}
	if len(sel.Barcodes) > MaxBarcodes {
		return r, invalid("selection.barcodes", fmt.Sprintf("at most %d barcodes", MaxBarcodes))
	}
	seen := map[string]bool{}
	for i, line := range sel.Barcodes {
		code, meters := splitBarcodeLine(line)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		r.barcodes = append(r.barcodes, code)
		if meters != "" {
			cm, err := parseMeters(meters)
			if err != nil {
				return r, invalid(fmt.Sprintf("selection.barcodes[%d]", i), "meters must be a positive number")
			}
			r.overrides[code] = cm
		}
	}

	switch in.Action {
	case ActionConvertToRoll:
		if strings.TrimSpace(p.Meters) != "" {
			cm, err := parseMeters(p.Meters)
			if err != nil {
				return r, invalid("params.meters", "must be a positive number")
			}
			r.meters = cm
		}
	case ActionChangeProduct:
		if p.ProductUUID == nil {
			return r, invalid("params.product_uuid", "required")
		}
		id, err := s.q.GetProductIDByUUID(ctx, *p.ProductUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return r, invalid("params.product_uuid", "product not found")
		}
		if err != nil {
			return r, fmt.Errorf("stockbulk: product: %w", err)
		}
		prod, err := s.q.GetProductByIDAnyBrand(ctx, id)
		if err != nil {
			return r, fmt.Errorf("stockbulk: product: %w", err)
		}
		if !prod.Active {
			return r, invalid("params.product_uuid", "the product is inactive")
		}
		r.toProduct = &prod
	case ActionMoveLocation:
		if p.LocationUUID == nil {
			return r, invalid("params.location_uuid", "required")
		}
		l, err := s.q.GetWarehouseLocationByUUID(ctx, *p.LocationUUID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && l.OrganizationID != c.OrgID) {
			return r, invalid("params.location_uuid", "location not found")
		}
		if err != nil {
			return r, fmt.Errorf("stockbulk: location: %w", err)
		}
		if !l.Active {
			return r, invalid("params.location_uuid", "the location is inactive")
		}
		r.toLocation = &l
	case ActionSetLot:
		if p.Lot != nil {
			v := strings.TrimSpace(*p.Lot)
			if len([]rune(v)) > MaxLotLength {
				return r, invalid("params.lot", fmt.Sprintf("at most %d characters", MaxLotLength))
			}
			r.lot = pgtype.Text{String: v, Valid: v != ""}
		}
	case ActionSetNote:
		if p.Note != nil {
			v := strings.TrimSpace(*p.Note)
			if len([]rune(v)) > MaxNoteLength {
				return r, invalid("params.note", fmt.Sprintf("at most %d characters", MaxNoteLength))
			}
			r.note = pgtype.Text{String: v, Valid: v != ""}
		}
	}
	return r, nil
}

// splitBarcodeLine reads "BARCODE", "BARCODE;12.5", "BARCODE\t12,5" or
// "BARCODE,12.5".
func splitBarcodeLine(line string) (string, string) {
	line = strings.TrimSpace(line)
	for _, sep := range []string{";", "\t", " "} {
		if a, b, ok := strings.Cut(line, sep); ok {
			return strings.TrimSpace(a), strings.TrimSpace(b)
		}
	}
	if a, b, ok := strings.Cut(line, ","); ok && strings.ContainsAny(b, "0123456789") && !strings.Contains(b, ",") {
		return strings.TrimSpace(a), strings.TrimSpace(b)
	}
	return line, ""
}

// parseMeters reads a positive meters value (comma or dot) into cm.
func parseMeters(s string) (int64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 || r.Cmp(big.NewRat(MaxMeters, 1)) > 0 {
		return 0, errors.New("invalid meters")
	}
	cm := new(big.Rat).Mul(r, big.NewRat(100, 1))
	if !cm.IsInt() {
		return 0, errors.New("at most 2 decimals")
	}
	return cm.Num().Int64(), nil
}

// --- Run / undo ------------------------------------------------------------

// Run queues a previewed job (the handler requires a recent step-up).
func (s *Service) Run(ctx context.Context, c Caller, id uuid.UUID) (JobView, error) {
	job, err := s.q.GetStockBulkJob(ctx, db.GetStockBulkJobParams{Uuid: id, OrganizationID: c.OrgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return JobView{}, ErrNotFound
	}
	if err != nil {
		return JobView{}, fmt.Errorf("stockbulk: job: %w", err)
	}
	if job.Status != StatusPreview {
		return JobView{}, ErrInvalidStatus
	}
	if job.Eligible == 0 {
		return JobView{}, ErrNothingToRun
	}
	job, err = s.transition(ctx, job.ID, StatusQueued, []string{StatusPreview}, &c.UserID, nil)
	if err != nil {
		return JobView{}, err
	}
	if err := s.enqueue(ctx, job.ID, false); err != nil {
		_, _ = s.transition(ctx, job.ID, StatusFailed, []string{StatusQueued}, nil, nil, err.Error())
		return JobView{}, fmt.Errorf("stockbulk: enqueue: %w", err)
	}
	return s.Get(ctx, c, id)
}

// Undo queues the undo of a completed job.
func (s *Service) Undo(ctx context.Context, c Caller, id uuid.UUID) (JobView, error) {
	job, err := s.q.GetStockBulkJob(ctx, db.GetStockBulkJobParams{Uuid: id, OrganizationID: c.OrgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return JobView{}, ErrNotFound
	}
	if err != nil {
		return JobView{}, fmt.Errorf("stockbulk: job: %w", err)
	}
	if !Reversible(job.Action) {
		return JobView{}, ErrNotReversible
	}
	if job.Status != StatusCompleted {
		return JobView{}, ErrInvalidStatus
	}
	job, err = s.transition(ctx, job.ID, StatusUndoing, []string{StatusCompleted}, nil, &c.UserID)
	if err != nil {
		return JobView{}, err
	}
	if err := s.enqueue(ctx, job.ID, true); err != nil {
		_, _ = s.transition(ctx, job.ID, StatusCompleted, []string{StatusUndoing}, nil, nil, err.Error())
		return JobView{}, fmt.Errorf("stockbulk: enqueue: %w", err)
	}
	return s.Get(ctx, c, id)
}

// enqueue hands the job to the queue; without one (tests, a single
// process without Redis) it is processed here and now.
func (s *Service) enqueue(ctx context.Context, jobID int64, undo bool) error {
	if s.queue == nil {
		return s.Process(ctx, jobID, undo)
	}
	return s.queue.EnqueueStockBulk(ctx, jobID, undo)
}

// Discard drops a preview.
func (s *Service) Discard(ctx context.Context, c Caller, id uuid.UUID) error {
	job, err := s.q.GetStockBulkJob(ctx, db.GetStockBulkJobParams{Uuid: id, OrganizationID: c.OrgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("stockbulk: job: %w", err)
	}
	_, err = s.transition(ctx, job.ID, StatusDiscarded, []string{StatusPreview}, nil, nil)
	return err
}

func (s *Service) transition(ctx context.Context, id int64, to string, from []string, runBy, undoneBy *int64, errMsg ...string) (db.StockBulkJob, error) {
	arg := db.SetStockBulkJobStatusParams{ID: id, Status: to, FromStatuses: from}
	if runBy != nil {
		arg.RunBy = pgtype.Int8{Int64: *runBy, Valid: true}
	}
	if undoneBy != nil {
		arg.UndoneBy = pgtype.Int8{Int64: *undoneBy, Valid: true}
	}
	if len(errMsg) > 0 && errMsg[0] != "" {
		arg.Error = pgtype.Text{String: truncate(errMsg[0], 1000), Valid: true}
	}
	job, err := s.q.SetStockBulkJobStatus(ctx, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		return job, ErrInvalidStatus
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return job, ErrBusy
	}
	if err != nil {
		return job, fmt.Errorf("stockbulk: job status: %w", err)
	}
	return job, nil
}

// Process runs (or undoes) a queued job in batches; each batch is one
// transaction and every item a savepoint, so a failing unit is reported
// and the rest go on. It is the stock:bulk task handler.
func (s *Service) Process(ctx context.Context, jobID int64, undo bool) error {
	job, err := s.q.GetStockBulkJobByID(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stockbulk: job: %w", err)
	}
	if undo {
		if job.Status != StatusUndoing {
			return nil
		}
	} else {
		if job.Status == StatusQueued {
			if job, err = s.transition(ctx, job.ID, StatusRunning, []string{StatusQueued}, nil, nil); err != nil {
				return err
			}
		}
		if job.Status != StatusRunning {
			return nil
		}
	}
	var params Params
	_ = json.Unmarshal(job.Params, &params)
	actor := job.RunBy.Int64
	if undo {
		actor = job.UndoneBy.Int64
	}
	from := ItemEligible
	if undo {
		from = ItemApplied
	}
	for {
		n, err := s.batch(ctx, job, params, actor, from, undo)
		if err != nil {
			if undo {
				_, _ = s.transition(ctx, job.ID, StatusCompleted, []string{StatusUndoing}, nil, nil, err.Error())
			} else {
				_, _ = s.transition(ctx, job.ID, StatusFailed, []string{StatusRunning}, nil, nil, err.Error())
			}
			return err
		}
		if n == 0 {
			break
		}
	}
	if undo {
		_, err = s.transition(ctx, job.ID, StatusUndone, []string{StatusUndoing}, nil, nil)
	} else {
		_, err = s.transition(ctx, job.ID, StatusCompleted, []string{StatusRunning}, nil, nil)
	}
	return err
}

// batch handles up to batchSize items of the job in one transaction.
func (s *Service) batch(ctx context.Context, job db.StockBulkJob, params Params, actor int64, from string, undo bool) (int, error) {
	var n int
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		items, err := q.ListStockBulkJobItemsToRun(ctx, db.ListStockBulkJobItemsToRunParams{
			JobID: job.ID, Status: from, RowLimit: batchSize,
		})
		if err != nil {
			return fmt.Errorf("stockbulk: items: %w", err)
		}
		n = len(items)
		led := ledger.New(q, s.out)
		for _, it := range items {
			sp, err := tx.Begin(ctx)
			if err != nil {
				return err
			}
			var res itemResult
			if undo {
				res, err = s.revertItem(ctx, sp, led, q.WithTx(sp), job, it, actor)
			} else {
				res, err = s.applyItem(ctx, sp, led, q.WithTx(sp), job, params, it, actor)
			}
			if err != nil {
				_ = sp.Rollback(ctx)
				status := ItemFailed
				if undo {
					status = ItemRevertSkipped
				}
				res = itemResult{status: status, reason: errorReason(err), err: err.Error()}
			} else if err := sp.Commit(ctx); err != nil {
				return err
			}
			if err := q.UpdateStockBulkJobItem(ctx, res.params(it.ID)); err != nil {
				return fmt.Errorf("stockbulk: item: %w", err)
			}
		}
		return q.RefreshStockBulkJobCounts(ctx, job.ID)
	})
	return n, err
}

type itemResult struct {
	status, reason, err string
	previous            map[string]any
	movementID          int64
}

func (r itemResult) params(id int64) db.UpdateStockBulkJobItemParams {
	p := db.UpdateStockBulkJobItemParams{ID: id, Status: r.status}
	if r.reason != "" {
		p.Reason = pgtype.Text{String: r.reason, Valid: true}
	}
	if r.err != "" {
		p.Error = pgtype.Text{String: truncate(r.err, 1000), Valid: true}
	}
	if r.previous != nil {
		p.Previous, _ = json.Marshal(r.previous)
	}
	if r.movementID != 0 {
		p.MovementID = pgtype.Int8{Int64: r.movementID, Valid: true}
	}
	return p
}

// errorReason maps ledger refusals to item reasons.
func errorReason(err error) string {
	switch {
	case errors.Is(err, ledger.ErrConvertAlreadyRoll):
		return ReasonAlreadyRoll
	case errors.Is(err, ledger.ErrConvertRollUsed):
		return ReasonRollCut
	case errors.Is(err, ledger.ErrConvertNotOnHand), errors.Is(err, ledger.ErrReclassifyUnitElsewhere):
		return ReasonChangedSince
	case errors.Is(err, ledger.ErrReclassifySameProduct):
		return ReasonSameProduct
	case errors.Is(err, ledger.ErrReclassifyUnitTypeMismatch):
		return ReasonUnitType
	}
	return ""
}

func (s *Service) applyItem(ctx context.Context, tx pgx.Tx, led *ledger.Ledger, q *db.Queries, job db.StockBulkJob,
	params Params, it db.ListStockBulkJobItemsToRunRow, actor int64) (itemResult, error) {
	u, err := q.GetUnitLotNote(ctx, it.UnitID)
	if err != nil {
		return itemResult{}, fmt.Errorf("stockbulk: unit: %w", err)
	}
	reason := params.Reason
	if reason == "" {
		reason = "bulk " + job.Action
	}
	meta := map[string]any{"bulk_job_uuid": job.Uuid.String()}
	switch job.Action {
	case ActionConvertToRoll:
		cm := int64(0)
		if it.Meters.Valid {
			cm, _ = ledger.NumericToCentimeters(it.Meters)
		}
		res, err := led.ConvertMeasure(ctx, tx, ledger.MeasureConversion{
			UnitID: it.UnitID, Centimeters: cm, HolderOrgID: job.OrganizationID,
			Source: keySource, RefType: keyRefType, RefID: it.ID, ActorUserID: &actor, Reason: reason, Metadata: meta,
		})
		if err != nil {
			return itemResult{}, err
		}
		return itemResult{status: ItemApplied, previous: map[string]any{"meters": nil}, movementID: res.Movement.ID}, nil
	case ActionChangeProduct:
		to, err := q.GetProductIDByUUID(ctx, *params.ProductUUID)
		if err != nil {
			return itemResult{}, fmt.Errorf("stockbulk: product: %w", err)
		}
		res, err := led.Reclassify(ctx, tx, ledger.Reclassification{
			RequestID: it.ID, UnitID: it.UnitID, FromProductID: u.ProductID, ToProductID: to,
			HolderOrgID: job.OrganizationID, ActorUserID: &actor, Reason: reason, Metadata: meta,
			Source: keySource, RefType: keyRefType,
		})
		if err != nil {
			return itemResult{}, err
		}
		return itemResult{status: ItemApplied, previous: map[string]any{"product_id": u.ProductID}, movementID: res.Movement.ID}, nil
	case ActionMoveLocation:
		loc, err := q.GetWarehouseLocationByUUID(ctx, *params.LocationUUID)
		if err != nil {
			return itemResult{}, fmt.Errorf("stockbulk: location: %w", err)
		}
		res, err := led.Post(ctx, tx, ledger.Movement{
			Type: ledger.TypePlacement, UnitID: it.UnitID,
			To:     &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: loc.OrganizationID},
			Source: keySource, RefType: keyRefType, RefID: it.ID, ActorUserID: &actor, Reason: reason, Metadata: meta,
		})
		if err != nil {
			return itemResult{}, err
		}
		return itemResult{status: ItemApplied, previous: map[string]any{"owner_type": u.OwnerType, "owner_id": u.OwnerID},
			movementID: res.Movement.ID}, nil
	case ActionVoid:
		res, err := led.Post(ctx, tx, ledger.Movement{
			Type: ledger.TypeVoid, UnitID: it.UnitID,
			Source: keySource, RefType: keyRefType, RefID: it.ID, ActorUserID: &actor, Reason: reason, Metadata: meta,
		})
		if err != nil {
			return itemResult{}, err
		}
		return itemResult{status: ItemApplied, movementID: res.Movement.ID}, nil
	case ActionSetLot:
		v := textParam(params.Lot)
		if err := q.SetUnitLot(ctx, db.SetUnitLotParams{ID: it.UnitID, Lot: v}); err != nil {
			return itemResult{}, fmt.Errorf("stockbulk: lot: %w", err)
		}
		return itemResult{status: ItemApplied, previous: map[string]any{"lot": textValue(u.Lot), "applied": textValue(v)}}, nil
	case ActionSetNote:
		v := textParam(params.Note)
		if err := q.SetUnitNote(ctx, db.SetUnitNoteParams{ID: it.UnitID, Note: v}); err != nil {
			return itemResult{}, fmt.Errorf("stockbulk: note: %w", err)
		}
		return itemResult{status: ItemApplied, previous: map[string]any{"note": textValue(u.Note), "applied": textValue(v)}}, nil
	}
	return itemResult{}, fmt.Errorf("stockbulk: action %q", job.Action)
}

// revertItem undoes one applied item when the unit has not moved since.
func (s *Service) revertItem(ctx context.Context, tx pgx.Tx, led *ledger.Ledger, q *db.Queries, job db.StockBulkJob,
	it db.ListStockBulkJobItemsToRunRow, actor int64) (itemResult, error) {
	u, err := q.GetUnitLotNote(ctx, it.UnitID)
	if err != nil {
		return itemResult{}, fmt.Errorf("stockbulk: unit: %w", err)
	}
	skip := func(reason string) (itemResult, error) {
		return itemResult{status: ItemRevertSkipped, reason: reason}, nil
	}
	var prev map[string]any
	_ = json.Unmarshal(it.Previous, &prev)
	ledgerAction := job.Action == ActionConvertToRoll || job.Action == ActionChangeProduct || job.Action == ActionMoveLocation
	if ledgerAction && (!it.MovementID.Valid || u.LastMovementID != it.MovementID.Int64) {
		return skip(ReasonChangedSince)
	}
	meta := map[string]any{"bulk_job_uuid": job.Uuid.String(), "undo": true}
	reason := "bulk undo " + job.Action
	switch job.Action {
	case ActionConvertToRoll:
		res, err := led.ConvertMeasure(ctx, tx, ledger.MeasureConversion{
			UnitID: it.UnitID, Centimeters: 0, HolderOrgID: job.OrganizationID,
			Source: keyUndoSource, RefType: keyRefType, RefID: it.ID, ActorUserID: &actor, Reason: reason, Metadata: meta,
		})
		if err != nil {
			return itemResult{}, err
		}
		return itemResult{status: ItemReverted, movementID: res.Movement.ID}, nil
	case ActionChangeProduct:
		back, ok := prev["product_id"].(float64)
		if !ok {
			return skip(ReasonChangedSince)
		}
		res, err := led.Reclassify(ctx, tx, ledger.Reclassification{
			RequestID: it.ID, UnitID: it.UnitID, FromProductID: u.ProductID, ToProductID: int64(back),
			HolderOrgID: job.OrganizationID, ActorUserID: &actor, Reason: reason, Metadata: meta,
			Source: keyUndoSource, RefType: keyRefType,
		})
		if err != nil {
			return itemResult{}, err
		}
		return itemResult{status: ItemReverted, movementID: res.Movement.ID}, nil
	case ActionMoveLocation:
		if prev["owner_type"] != string(ledger.OwnerWarehouseLocation) {
			return skip(ReasonPrevUnplaced)
		}
		locID, _ := prev["owner_id"].(float64)
		loc, err := q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: int64(locID), OrganizationID: job.OrganizationID})
		if err != nil {
			return skip(ReasonChangedSince)
		}
		res, err := led.Post(ctx, tx, ledger.Movement{
			Type: ledger.TypePlacement, UnitID: it.UnitID,
			To:     &ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: loc.OrganizationID},
			Source: keyUndoSource, RefType: keyRefType, RefID: it.ID, ActorUserID: &actor, Reason: reason, Metadata: meta,
		})
		if err != nil {
			return itemResult{}, err
		}
		return itemResult{status: ItemReverted, movementID: res.Movement.ID}, nil
	case ActionSetLot:
		if textValue(u.Lot) != prev["applied"] {
			return skip(ReasonChangedSince)
		}
		if err := q.SetUnitLot(ctx, db.SetUnitLotParams{ID: it.UnitID, Lot: anyText(prev["lot"])}); err != nil {
			return itemResult{}, err
		}
		return itemResult{status: ItemReverted}, nil
	case ActionSetNote:
		if textValue(u.Note) != prev["applied"] {
			return skip(ReasonChangedSince)
		}
		if err := q.SetUnitNote(ctx, db.SetUnitNoteParams{ID: it.UnitID, Note: anyText(prev["note"])}); err != nil {
			return itemResult{}, err
		}
		return itemResult{status: ItemReverted}, nil
	}
	return skip(ReasonChangedSince)
}

func textParam(p *string) pgtype.Text {
	if p == nil {
		return pgtype.Text{}
	}
	v := strings.TrimSpace(*p)
	return pgtype.Text{String: v, Valid: v != ""}
}

// textValue is the JSON value of a nullable text (nil or the string).
func textValue(t pgtype.Text) any {
	if !t.Valid {
		return nil
	}
	return t.String
}

func anyText(v any) pgtype.Text {
	s, ok := v.(string)
	return pgtype.Text{String: s, Valid: ok && s != ""}
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("stockbulk: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("stockbulk: commit: %w", err)
	}
	return nil
}
