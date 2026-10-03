package usecase

// Barcode batches (TEC-202): the center reserves N barcodes for a product
// (K14). The barcodes are allocated from the brand's counter under its row
// lock as <PREFIX>-<8 digits> (the roll split, TEC-184, derives
// <barcode>-S<n>, so the formats never collide) and the units are created
// in status printed, like the import path (TEC-158); they enter stock later
// through ledger.Post (entry, TEC-95d). Nothing is written to the ledger
// here: a label without stock is not a movement.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/labels"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Barcode errors.
var (
	// ErrBarcodesCenterOnly: only the center issues barcodes (K14).
	ErrBarcodesCenterOnly = errors.New("stock: only the center generates barcodes")
	// ErrProductInactive: the product is not active.
	ErrProductInactive = errors.New("stock: product is inactive")
)

// Audit actions of barcode batches.
const (
	AuditResourceBarcodeBatch = "stock.barcode_batch"
	AuditBarcodeBatchCreated  = "stock.barcode_batch.created"
	AuditBarcodeBatchPrinted  = "stock.barcode_batch.printed"
)

// MaxBatchQuantity is the largest batch (barcode_batches CHECK).
const MaxBatchQuantity = 1000

var prefixPattern = regexp.MustCompile(`^[A-Z0-9]{2,8}$`)

// Barcodes implements the batch use case.
type Barcodes struct {
	pool TxBeginner
	q    *db.Queries
}

// NewBarcodes builds the use case.
func NewBarcodes(pool TxBeginner, q *db.Queries) *Barcodes {
	return &Barcodes{pool: pool, q: q}
}

// BatchInput reserves Quantity barcodes for the product.
type BatchInput struct {
	ProductUUID string
	Quantity    int
	// Meters is the initial length of each roll (roll_meter products only).
	Meters string
	// Prefix overrides the brand prefix (A-Z0-9, 2-8 characters).
	Prefix string
	// TemplateUUID is the label template printed by default.
	TemplateUUID string
}

// centerOrg returns the active organization when it is the center and the
// stock.write grant reaches it.
func centerOrg(c Caller) (int64, error) {
	if c.Org.OrgType != rbac.OrgTypeCenter {
		return 0, ErrBarcodesCenterOnly
	}
	if c.Org.InternalID == 0 || !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) || c.Filter.UserOnly() {
		return 0, ErrBarcodesCenterOnly
	}
	return c.Org.InternalID, nil
}

// Barcode formats the n-th barcode of a prefix.
func Barcode(prefix string, seq int64) string {
	return fmt.Sprintf("%s-%08d", prefix, seq)
}

// DefaultPrefix derives the barcode prefix of a brand from its slug
// (upper-case letters and digits, 2-8 characters; "OF" when too short).
func DefaultPrefix(brandSlug string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(brandSlug) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
		if b.Len() == 8 {
			break
		}
	}
	if b.Len() < 2 {
		return "OF"
	}
	return b.String()
}

// Create reserves the batch: the units exist in status printed when it
// returns.
func (s *Barcodes) Create(ctx context.Context, c Caller, in BatchInput) (model.BarcodeBatch, error) {
	prep, err := s.prepare(ctx, s.q, c, in)
	if err != nil {
		return model.BarcodeBatch{}, err
	}
	var batch db.BarcodeBatch
	var units []db.Unit
	err = s.inTx(ctx, func(q *db.Queries) error {
		batch, units, err = s.reserve(ctx, q, c, prep)
		return err
	})
	if err != nil {
		return model.BarcodeBatch{}, err
	}
	return s.view(ctx, batch, prep.product, units), nil
}

// ReserveInTx reserves a batch inside the caller's transaction q (the stock
// entry generate_new mode, TEC-204) with the same rules as Create. It
// returns the batch view and the new printed units.
func (s *Barcodes) ReserveInTx(ctx context.Context, q *db.Queries, c Caller, in BatchInput) (model.BarcodeBatch, []db.Unit, error) {
	prep, err := s.prepare(ctx, q, c, in)
	if err != nil {
		return model.BarcodeBatch{}, nil, err
	}
	batch, units, err := s.reserve(ctx, q, c, prep)
	if err != nil {
		return model.BarcodeBatch{}, nil, err
	}
	return s.view(ctx, batch, prep.product, units), units, nil
}

// batchPrep is a validated batch request.
type batchPrep struct {
	org      int64
	quantity int
	prefix   string
	product  db.Product
	meters   pgtype.Numeric
	template pgtype.Int8
	kind     string
}

// prepare validates a batch request (no writes).
func (s *Barcodes) prepare(ctx context.Context, q *db.Queries, c Caller, in BatchInput) (batchPrep, error) {
	org, err := centerOrg(c)
	if err != nil {
		return batchPrep{}, err
	}
	if in.Quantity < 1 || in.Quantity > MaxBatchQuantity {
		return batchPrep{}, invalid("quantity", fmt.Sprintf("must be between 1 and %d", MaxBatchQuantity))
	}
	pid, err := uuid.Parse(strings.TrimSpace(in.ProductUUID))
	if err != nil {
		return batchPrep{}, invalid("product_uuid", "must be a uuid")
	}
	prefix := strings.ToUpper(strings.TrimSpace(in.Prefix))
	if prefix == "" {
		prefix = DefaultPrefix(c.Org.BrandSlug)
	}
	if !prefixPattern.MatchString(prefix) {
		return batchPrep{}, invalid("prefix", "must be 2-8 characters: A-Z or 0-9")
	}
	p, err := q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: pid, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return batchPrep{}, invalid("product_uuid", "product not found")
	}
	if err != nil {
		return batchPrep{}, fmt.Errorf("stock: product: %w", err)
	}
	if !p.Active {
		return batchPrep{}, ErrProductInactive
	}
	var meters pgtype.Numeric
	isRoll := p.UnitType == "roll_meter" && !p.UsesFixedBarcode
	switch {
	case isRoll:
		cm, err := ledger.ParseMeters(in.Meters)
		if err != nil || cm <= 0 {
			return batchPrep{}, invalid("meters", "roll products need the length of each roll (positive, at most two decimals)")
		}
		meters = ledger.CentimetersToNumeric(cm)
	case strings.TrimSpace(in.Meters) != "":
		return batchPrep{}, invalid("meters", "only roll products carry meters")
	}
	var template pgtype.Int8
	if t := strings.TrimSpace(in.TemplateUUID); t != "" {
		tid, err := uuid.Parse(t)
		if err != nil {
			return batchPrep{}, invalid("template_uuid", "must be a uuid")
		}
		tpl, err := q.GetLabelTemplateByUUID(ctx, db.GetLabelTemplateByUUIDParams{Uuid: tid, OrganizationID: org})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && tpl.Kind != labels.KindUnit) {
			return batchPrep{}, invalid("template_uuid", "unit label template not found")
		}
		if err != nil {
			return batchPrep{}, fmt.Errorf("stock: template: %w", err)
		}
		template = i8(tpl.ID)
	}
	kind := ledger.KindSerial
	if p.UsesFixedBarcode {
		kind = ledger.KindFixed
	}
	return batchPrep{org: org, quantity: in.Quantity, prefix: prefix, product: p, meters: meters, template: template, kind: kind}, nil
}

// reserve allocates the barcodes and creates the printed units in q.
func (s *Barcodes) reserve(ctx context.Context, q *db.Queries, c Caller, b batchPrep) (db.BarcodeBatch, []db.Unit, error) {
	counter, err := q.LockBarcodeCounter(ctx, db.LockBarcodeCounterParams{BrandID: c.Org.BrandID, Prefix: b.prefix})
	if err != nil {
		return db.BarcodeBatch{}, nil, fmt.Errorf("stock: barcode counter: %w", err)
	}
	seqs, err := freeSequences(ctx, q, c.Org.BrandID, b.prefix, counter.NextSeq, b.quantity)
	if err != nil {
		return db.BarcodeBatch{}, nil, err
	}
	batch, err := q.CreateBarcodeBatch(ctx, db.CreateBarcodeBatchParams{
		OrganizationID: b.org, BrandID: c.Org.BrandID, ProductID: b.product.ID, Quantity: int32(b.quantity),
		Prefix: b.prefix, FirstSeq: seqs[0], LastSeq: seqs[len(seqs)-1], Meters: b.meters,
		TemplateID: b.template, CreatedByUserID: c.actor(),
	})
	if err != nil {
		return db.BarcodeBatch{}, nil, fmt.Errorf("stock: batch: %w", err)
	}
	units := make([]db.Unit, 0, len(seqs))
	for _, seq := range seqs {
		u, err := q.CreateBatchUnit(ctx, db.CreateBatchUnitParams{
			OrganizationID: b.org, BrandID: c.Org.BrandID, ProductID: b.product.ID, Barcode: Barcode(b.prefix, seq),
			UnitKind: b.kind, Status: string(ledger.StatusPrinted),
			InitialMeters: b.meters, RemainingMeters: b.meters, BatchID: i8(batch.ID),
		})
		if err != nil {
			return db.BarcodeBatch{}, nil, fmt.Errorf("stock: batch unit: %w", err)
		}
		units = append(units, u)
	}
	if err := q.SetBarcodeCounter(ctx, db.SetBarcodeCounterParams{
		NextSeq: seqs[len(seqs)-1] + 1, BrandID: c.Org.BrandID, Prefix: b.prefix,
	}); err != nil {
		return db.BarcodeBatch{}, nil, fmt.Errorf("stock: barcode counter: %w", err)
	}
	err = s.audit(ctx, q, c, AuditBarcodeBatchCreated, batch, map[string]any{
		"product_id": b.product.ID, "quantity": b.quantity, "prefix": b.prefix,
		"first_barcode": units[0].Barcode, "last_barcode": units[len(units)-1].Barcode,
	})
	return batch, units, err
}

// freeSequences returns n consecutive-by-allocation sequence numbers from
// next whose barcodes are unused (imported barcodes may occupy a number).
func freeSequences(ctx context.Context, q *db.Queries, brand int64, prefix string, next int64, n int) ([]int64, error) {
	seqs := make([]int64, 0, n)
	for seq := next; len(seqs) < n; seq++ {
		if seq-next > int64(n)+10_000 {
			return nil, fmt.Errorf("stock: no free barcodes after %s", Barcode(prefix, next))
		}
		_, err := q.GetUnitByBarcode(ctx, db.GetUnitByBarcodeParams{BrandID: brand, Barcode: Barcode(prefix, seq)})
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("stock: barcode lookup: %w", err)
		}
		seqs = append(seqs, seq)
	}
	return seqs, nil
}

// List returns the batches of the center, newest first.
func (s *Barcodes) List(ctx context.Context, c Caller, limit, offset int32) ([]model.BarcodeBatch, int64, error) {
	org, err := centerOrg(c)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListBarcodeBatches(ctx, db.ListBarcodeBatchesParams{OrganizationID: org, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: batches: %w", err)
	}
	total, err := s.q.CountBarcodeBatches(ctx, org)
	if err != nil {
		return nil, 0, fmt.Errorf("stock: batches: %w", err)
	}
	out := make([]model.BarcodeBatch, 0, len(rows))
	for _, b := range rows {
		p, err := s.q.GetProductByIDAnyBrand(ctx, b.ProductID)
		if err != nil {
			return nil, 0, fmt.Errorf("stock: product: %w", err)
		}
		out = append(out, s.view(ctx, b, p, nil))
	}
	return out, total, nil
}

// Get returns one batch with its units.
func (s *Barcodes) Get(ctx context.Context, c Caller, id uuid.UUID) (model.BarcodeBatch, error) {
	org, err := centerOrg(c)
	if err != nil {
		return model.BarcodeBatch{}, err
	}
	b, err := s.q.GetBarcodeBatchByUUID(ctx, db.GetBarcodeBatchByUUIDParams{Uuid: id, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.BarcodeBatch{}, ErrNotFound
	}
	if err != nil {
		return model.BarcodeBatch{}, fmt.Errorf("stock: batch: %w", err)
	}
	p, err := s.q.GetProductByIDAnyBrand(ctx, b.ProductID)
	if err != nil {
		return model.BarcodeBatch{}, fmt.Errorf("stock: product: %w", err)
	}
	units, err := s.q.ListUnitsByBatch(ctx, i8(b.ID))
	if err != nil {
		return model.BarcodeBatch{}, fmt.Errorf("stock: batch units: %w", err)
	}
	return s.view(ctx, b, p, units), nil
}

func (s *Barcodes) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("stock: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("stock: commit: %w", err)
	}
	return nil
}

func (s *Barcodes) audit(ctx context.Context, q *db.Queries, c Caller, action string, b db.BarcodeBatch, extra map[string]any) error {
	m := map[string]any{"batch_id": b.ID, "organization_id": b.OrganizationID, "brand_id": b.BrandID}
	for k, v := range extra {
		m[k] = v
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("stock: audit payload: %w", err)
	}
	if _, err := q.InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID: c.actor(), Action: action, Resource: AuditResourceBarcodeBatch,
		ResourceUuid: pgtype.UUID{Bytes: b.Uuid, Valid: true}, Payload: payload,
		IpAddress: c.IP, UserAgent: pgText(c.UserAgent),
	}); err != nil {
		return fmt.Errorf("stock: audit: %w", err)
	}
	return nil
}

// BatchLabelsPath is the print endpoint of a batch.
func BatchLabelsPath(id uuid.UUID) string { return "/v1/stock/barcodes/" + id.String() + "/labels.pdf" }

// UnitLabelPath is the print endpoint of one unit label (the split's new
// barcode links here).
func UnitLabelPath(barcode string) string {
	return "/v1/stock/labels/units.pdf?barcode=" + url.QueryEscape(barcode)
}

func (s *Barcodes) view(_ context.Context, b db.BarcodeBatch, p db.Product, units []db.Unit) model.BarcodeBatch {
	out := model.BarcodeBatch{
		UUID: b.Uuid,
		Product: model.BarcodeProduct{
			UUID: p.Uuid, SKU: p.Sku, Name: p.Name, UnitType: p.UnitType, UsesFixedBarcode: p.UsesFixedBarcode,
		},
		Quantity: b.Quantity, Prefix: b.Prefix,
		FirstBarcode: Barcode(b.Prefix, b.FirstSeq), LastBarcode: Barcode(b.Prefix, b.LastSeq),
		PrintCount: b.PrintCount, LabelsURL: BatchLabelsPath(b.Uuid), CreatedAt: tsTime(b.CreatedAt),
	}
	if b.Meters.Valid {
		m := numericString(b.Meters)
		out.Meters = &m
	}
	if b.LastPrintedAt.Valid {
		t := b.LastPrintedAt.Time
		out.LastPrintedAt = &t
	}
	if b.TemplateID.Valid {
		if t, err := s.q.GetLabelTemplateByID(context.Background(), b.TemplateID.Int64); err == nil {
			id := t.Uuid
			out.TemplateUUID = &id
		}
	}
	if units != nil {
		out.Units = make([]model.BarcodeBatchUnit, 0, len(units))
		for _, u := range units {
			out.Units = append(out.Units, model.BarcodeBatchUnit{UUID: u.Uuid, Barcode: u.Barcode, Status: u.Status})
		}
	}
	return out
}
