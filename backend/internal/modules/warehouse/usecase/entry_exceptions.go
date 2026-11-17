package usecase

// TEC-1225: receiving exceptions on stock entries.
//
//   - Discrepancy record ("kabul tutanağı"): confirm takes a decision for
//     every missing line, extra barcode and damaged line. Missing lines are
//     left out (they do not enter stock), extras are added or rejected,
//     damaged lines are accepted into stock or excluded (reason and at least
//     one photo attachment required). The decisions are written once, in
//     the confirm transaction, to stock_entry_discrepancies (append-only).
//     The arrival check applies when the entry is a container entry
//     (container_no set) or the arrival scan was used: then an unscanned
//     line without a decision refuses the confirm.
//   - Attachments: waybill, invoice, photo or other document of the entry
//     (object storage), on any entry that is not cancelled.
//   - Lot and production date on manual lines (add lines / line details).
//   - Undo of a confirmed manual entry: stock.adjust (and step-up on the
//     route); every line gets a void movement through the ledger. A line
//     whose unit moved after the entry (shipment, service, transfer, ...)
//     refuses the undo and is listed.
//
// No DTO here carries a price or amount: warehouse roles never see prices.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Receiving exception errors.
var (
	ErrEntryAttachmentNotFound = errors.New("warehouse: stock entry attachment not found")
	// ErrEntryAttachmentInUse: the attachment is damage evidence of the
	// discrepancy record.
	ErrEntryAttachmentInUse = errors.New("warehouse: attachment is part of the discrepancy record")
	ErrEntryCancelled       = errors.New("warehouse: stock entry is cancelled")
	ErrEntryStorage         = errors.New("warehouse: file storage is not configured")
	// ErrEntryNotConfirmed: undo needs a confirmed entry.
	ErrEntryNotConfirmed = errors.New("warehouse: stock entry is not confirmed")
	// ErrEntryUndoImport: an import entry is undone through its import job.
	ErrEntryUndoImport = errors.New("warehouse: import entries are undone through the import")
	// ErrEntryAdjustForbidden: undo needs stock.adjust.
	ErrEntryAdjustForbidden = errors.New("warehouse: stock.adjust is required")
)

// Discrepancy kinds and decisions (stock_entry_discrepancies).
const (
	DiscrepancyMissing = "missing"
	DiscrepancyExtra   = "extra"
	DiscrepancyDamaged = "damaged"

	DecisionLeftOut  = "left_out"
	DecisionAdded    = "added"
	DecisionRejected = "rejected"
	DecisionAccepted = "accepted"
	DecisionExcluded = "excluded"
)

// Attachment kinds.
const (
	AttachmentWaybill = "waybill"
	AttachmentInvoice = "invoice"
	AttachmentPhoto   = "photo"
	AttachmentOther   = "other"
)

// Audit actions of the entry timeline (TEC-1225).
const (
	AuditStockEntryLinesAdded     = "warehouse.stock_entry.lines_added"
	AuditStockEntryLineDeleted    = "warehouse.stock_entry.line_deleted"
	AuditStockEntryLineUpdated    = "warehouse.stock_entry.line_updated"
	AuditStockEntryPlaced         = "warehouse.stock_entry.placed"
	AuditStockEntryScanned        = "warehouse.stock_entry.scanned"
	AuditStockEntryUnscanned      = "warehouse.stock_entry.unscanned"
	AuditStockEntryLabelsPrinted  = "warehouse.stock_entry.labels_printed"
	AuditStockEntryDiscrepancies  = "warehouse.stock_entry.discrepancies_recorded"
	AuditStockEntryAttachmentAdd  = "warehouse.stock_entry.attachment_added"
	AuditStockEntryAttachmentDrop = "warehouse.stock_entry.attachment_removed"
	AuditStockEntryUndone         = "warehouse.stock_entry.undone"
)

const (
	// MaxEntryAttachmentBytes caps one attachment.
	MaxEntryAttachmentBytes = 15 << 20
	// MaxEntryAttachments caps the attachments of one entry.
	MaxEntryAttachments = 50
	entryUndoSource     = "stock_entry_undo"
	maxReasonLen        = 500
)

// DiscrepancyRequiredError lists what still needs a decision at confirm.
type DiscrepancyRequiredError struct {
	Missing []string
	Extras  []string
}

func (e *DiscrepancyRequiredError) Error() string {
	return fmt.Sprintf("warehouse: %d missing line(s) and %d extra barcode(s) need a decision", len(e.Missing), len(e.Extras))
}

// UndoBlockedError lists the lines whose units moved after the entry.
type UndoBlockedError struct{ Blockers []UndoBlocker }

func (e *UndoBlockedError) Error() string {
	return fmt.Sprintf("warehouse: %d unit(s) moved after the entry", len(e.Blockers))
}

// --- views ---------------------------------------------------------------------

// EntryAttachment is a file of an entry. URL is the download path.
type EntryAttachment struct {
	UUID       uuid.UUID  `json:"uuid"`
	Kind       string     `json:"kind"`
	Filename   string     `json:"filename"`
	MimeType   string     `json:"mime_type"`
	SizeBytes  int64      `json:"size_bytes"`
	URL        string     `json:"url"`
	UploadedBy *EntryUser `json:"uploaded_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

// EntryDiscrepancy is one decision of the discrepancy record.
type EntryDiscrepancy struct {
	UUID        uuid.UUID         `json:"uuid"`
	Kind        string            `json:"kind"`
	Decision    string            `json:"decision"`
	Barcode     string            `json:"barcode"`
	Product     *EntryProduct     `json:"product"`
	Quantity    int32             `json:"quantity"`
	Reason      *string           `json:"reason"`
	Attachments []EntryAttachment `json:"attachments"`
	User        *EntryUser        `json:"user"`
	CreatedAt   time.Time         `json:"created_at"`
}

// ReceiptSupplier is the supplier on the discrepancy record.
type ReceiptSupplier struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	TaxNo *string   `json:"tax_no"`
	Phone *string   `json:"phone"`
	Email *string   `json:"email"`
}

// ReceiptSummary counts the decisions of the record.
type ReceiptSummary struct {
	Entered         int64 `json:"entered"`
	Missing         int64 `json:"missing"`
	ExtrasAdded     int64 `json:"extras_added"`
	ExtrasRejected  int64 `json:"extras_rejected"`
	DamagedAccepted int64 `json:"damaged_accepted"`
	DamagedExcluded int64 `json:"damaged_excluded"`
}

// EntryReceipt is the discrepancy record ("kabul tutanağı") of an entry.
type EntryReceipt struct {
	EntryUUID   uuid.UUID          `json:"entry_uuid"`
	Status      string             `json:"status"`
	Warehouse   *EntryWarehouse    `json:"warehouse"`
	Supplier    *ReceiptSupplier   `json:"supplier"`
	ContainerNo *string            `json:"container_no"`
	WaybillNo   *string            `json:"waybill_no"`
	ArrivalDate *string            `json:"arrival_date"`
	ConfirmedAt *time.Time         `json:"confirmed_at"`
	ConfirmedBy *EntryUser         `json:"confirmed_by"`
	Summary     ReceiptSummary     `json:"summary"`
	Items       []EntryDiscrepancy `json:"items"`
	PDFURL      string             `json:"pdf_url"`
}

// UndoBlocker is a line that keeps the entry from being undone.
type UndoBlocker struct {
	Barcode      string       `json:"barcode"`
	Product      EntryProduct `json:"product"`
	MovementType string       `json:"movement_type"`
	MovementUUID uuid.UUID    `json:"movement_uuid"`
	At           time.Time    `json:"at"`
}

// UndoCheck tells whether a confirmed entry can be undone.
type UndoCheck struct {
	Allowed  bool          `json:"allowed"`
	Reason   *string       `json:"reason"`
	Lines    int           `json:"lines"`
	Blockers []UndoBlocker `json:"blockers"`
}

// --- inputs ----------------------------------------------------------------------

// DiscrepancyDecision is one decision given at confirm. Barcode names the
// line (missing, damaged) or the extra scan (extra).
type DiscrepancyDecision struct {
	Barcode         string
	Kind            string
	Decision        string
	Reason          string
	AttachmentUUIDs []uuid.UUID
	// LocationUUID: the bin of an added extra (default: the entry's default bin).
	LocationUUID *uuid.UUID
}

// ConfirmInput carries the decisions of the discrepancy record.
type ConfirmInput struct {
	Decisions []DiscrepancyDecision
}

// AttachmentInput is an uploaded file.
type AttachmentInput struct {
	Kind     string
	Filename string
	Body     io.Reader
	Size     int64
}

// LineDetailsInput changes a draft line. A nil field keeps the value, an
// empty string clears it.
type LineDetailsInput struct {
	Lot            *string
	ProductionDate *string
	BoxNo          *string
}

// --- wiring ------------------------------------------------------------------------

// WithFiles sets the object storage (attachments) and the PDF renderer
// (discrepancy record PDF).
func (s *StockEntries) WithFiles(store platstorage.Driver, pdf PDFRenderer) *StockEntries {
	s.store, s.pdf = store, pdf
	return s
}

// --- discrepancy decisions at confirm ------------------------------------------------

type decisionKey struct{ kind, barcode string }

// arrivalChecked tells whether the arrival check applies to the entry.
func arrivalChecked(e db.StockEntry, lines []db.StockEntryLine, extras int) bool {
	if e.ContainerNo.Valid || extras > 0 {
		return true
	}
	for _, l := range lines {
		if l.ScannedAt.Valid {
			return true
		}
	}
	return false
}

// applyDecisions validates the decisions against the entry, refuses the
// confirm when a missing line or an extra has none, writes the discrepancy
// record and applies it: left out and excluded lines leave the entry, added
// extras become lines. Runs in the confirm transaction under the entry lock.
func (s *StockEntries) applyDecisions(ctx context.Context, q *db.Queries, c EntryCaller, e db.StockEntry, in ConfirmInput) error {
	lines, err := q.ListStockEntryLines(ctx, e.ID)
	if err != nil {
		return err
	}
	scans, err := q.ListStockEntryScans(ctx, e.ID)
	if err != nil {
		return err
	}
	if len(in.Decisions) > MaxEntryLines {
		return invalid("decisions", fmt.Sprintf("must hold at most %d items", MaxEntryLines))
	}
	decisions := make(map[decisionKey]DiscrepancyDecision, len(in.Decisions))
	for _, d := range in.Decisions {
		d.Barcode = strings.ToUpper(strings.TrimSpace(d.Barcode))
		d.Kind, d.Decision = strings.TrimSpace(d.Kind), strings.TrimSpace(d.Decision)
		d.Reason = strings.TrimSpace(d.Reason)
		if d.Barcode == "" {
			return invalid("decisions", "barcode is required")
		}
		switch {
		case d.Kind == DiscrepancyMissing && d.Decision == DecisionLeftOut:
		case d.Kind == DiscrepancyExtra && (d.Decision == DecisionAdded || d.Decision == DecisionRejected):
		case d.Kind == DiscrepancyDamaged && (d.Decision == DecisionAccepted || d.Decision == DecisionExcluded):
			if d.Reason == "" {
				return invalid("decisions", "damaged "+d.Barcode+": reason is required")
			}
			if len(d.AttachmentUUIDs) == 0 {
				return invalid("decisions", "damaged "+d.Barcode+": a photo is required")
			}
		default:
			return invalid("decisions", "invalid kind or decision for "+d.Barcode)
		}
		if utf8.RuneCountInString(d.Reason) > maxReasonLen {
			return invalid("decisions", "reason must be at most 500 characters")
		}
		k := decisionKey{d.Kind, d.Barcode}
		if _, dup := decisions[k]; dup {
			return invalid("decisions", "decision repeats for "+d.Barcode)
		}
		decisions[k] = d
	}
	if len(decisions) == 0 && !arrivalChecked(e, lines, len(scans)) {
		return nil
	}

	type lineRef struct {
		line db.StockEntryLine
		unit db.Unit
	}
	byBarcode := make(map[string]lineRef, len(lines))
	for _, l := range lines {
		u, err := q.GetUnit(ctx, l.UnitID)
		if err != nil {
			return err
		}
		byBarcode[strings.ToUpper(u.Barcode)] = lineRef{l, u}
	}
	extras := make(map[string]db.StockEntryScan, len(scans))
	for _, sc := range scans {
		extras[strings.ToUpper(sc.Barcode)] = sc
	}
	for k := range decisions {
		switch k.kind {
		case DiscrepancyMissing, DiscrepancyDamaged:
			if _, ok := byBarcode[k.barcode]; !ok {
				return invalid("decisions", "barcode "+k.barcode+" is not on this entry")
			}
		case DiscrepancyExtra:
			if _, ok := extras[k.barcode]; !ok {
				return invalid("decisions", "barcode "+k.barcode+" was not scanned as an extra")
			}
		}
	}
	for k := range decisions {
		if k.kind == DiscrepancyMissing {
			if _, both := decisions[decisionKey{DiscrepancyDamaged, k.barcode}]; both {
				return invalid("decisions", "barcode "+k.barcode+" cannot be missing and damaged")
			}
		}
	}

	checked := arrivalChecked(e, lines, len(scans))
	pending := &DiscrepancyRequiredError{}
	if checked {
		for code, ref := range byBarcode {
			if ref.line.ScannedAt.Valid {
				continue
			}
			_, missing := decisions[decisionKey{DiscrepancyMissing, code}]
			_, damaged := decisions[decisionKey{DiscrepancyDamaged, code}]
			if !missing && !damaged {
				pending.Missing = append(pending.Missing, ref.unit.Barcode)
			}
		}
	}
	for code, sc := range extras {
		if _, ok := decisions[decisionKey{DiscrepancyExtra, code}]; !ok {
			pending.Extras = append(pending.Extras, sc.Barcode)
		}
	}
	if len(pending.Missing) > 0 || len(pending.Extras) > 0 {
		sortStrings(pending.Missing)
		sortStrings(pending.Extras)
		return pending
	}

	attachments, err := q.ListStockEntryAttachments(ctx, e.ID)
	if err != nil {
		return err
	}
	attByUUID := make(map[uuid.UUID]db.ListStockEntryAttachmentsRow, len(attachments))
	for _, a := range attachments {
		attByUUID[a.Uuid] = a
	}

	var counts = map[string]int{}
	for _, d := range sortedDecisions(decisions) {
		arg := db.InsertStockEntryDiscrepancyParams{
			OrganizationID: e.OrganizationID, BrandID: e.BrandID, EntryID: e.ID,
			Kind: d.Kind, Decision: d.Decision, Barcode: d.Barcode, Quantity: 1,
			AttachmentIds: []int64{}, CreatedByUserID: c.actor(),
		}
		if d.Reason != "" {
			arg.Reason = pgtype.Text{String: d.Reason, Valid: true}
		}
		for _, au := range d.AttachmentUUIDs {
			a, ok := attByUUID[au]
			if !ok {
				return invalid("decisions", "attachment "+au.String()+" is not on this entry")
			}
			if !strings.HasPrefix(a.MimeType, "image/") {
				return invalid("decisions", "damage evidence must be a photo")
			}
			arg.AttachmentIds = append(arg.AttachmentIds, a.ID)
		}
		switch d.Kind {
		case DiscrepancyMissing, DiscrepancyDamaged:
			ref := byBarcode[d.Barcode]
			arg.Barcode = ref.unit.Barcode
			arg.UnitID = pgtype.Int8{Int64: ref.unit.ID, Valid: true}
			arg.ProductID = pgtype.Int8{Int64: ref.unit.ProductID, Valid: true}
			arg.Quantity = ref.line.Quantity
			if d.Decision == DecisionLeftOut || d.Decision == DecisionExcluded {
				if _, err := q.DeleteStockEntryLineByID(ctx, db.DeleteStockEntryLineByIDParams{ID: ref.line.ID, EntryID: e.ID}); err != nil {
					return err
				}
			}
		case DiscrepancyExtra:
			sc := extras[d.Barcode]
			arg.Barcode = sc.Barcode
			if sc.UnitID.Valid {
				u, err := q.GetUnit(ctx, sc.UnitID.Int64)
				if err != nil {
					return err
				}
				arg.UnitID = pgtype.Int8{Int64: u.ID, Valid: true}
				arg.ProductID = pgtype.Int8{Int64: u.ProductID, Valid: true}
				if d.Decision == DecisionAdded {
					if err := s.addExtraLine(ctx, q, c, e, u, d); err != nil {
						return err
					}
				}
			} else if d.Decision == DecisionAdded {
				return invalid("decisions", "barcode "+sc.Barcode+" is unknown; an unknown extra can only be rejected")
			}
		}
		if _, err := q.InsertStockEntryDiscrepancy(ctx, arg); err != nil {
			return err
		}
		counts[d.Kind+"."+d.Decision]++
	}
	if len(decisions) == 0 {
		return nil
	}
	extra := map[string]any{"count": len(decisions)}
	for k, v := range counts {
		extra[k] = v
	}
	return audit(ctx, q, c, AuditStockEntryDiscrepancies, e, extra)
}

// addExtraLine adds an extra barcode that arrived as a received line.
func (s *StockEntries) addExtraLine(ctx context.Context, q *db.Queries, c EntryCaller, e db.StockEntry, u db.Unit, d DiscrepancyDecision) error {
	if err := enterable(ctx, q, u); err != nil {
		return fmt.Errorf("%w: %s", err, u.Barcode)
	}
	loc := e.DefaultLocationID
	if d.LocationUUID != nil {
		l, err := s.entryLocation(ctx, q, e, d.LocationUUID, "")
		if err != nil {
			return err
		}
		loc = pgtype.Int8{Int64: l.ID, Valid: true}
	}
	if !loc.Valid {
		return ErrEntryUnplaced
	}
	line, err := q.InsertStockEntryLine(ctx, db.InsertStockEntryLineParams{EntryID: e.ID, UnitID: u.ID, Quantity: 1, LocationID: loc})
	if err != nil {
		if isUnique(err) {
			return invalid("decisions", "barcode "+u.Barcode+" is already on this entry")
		}
		return err
	}
	if c.Principal.UserInternal > 0 {
		if _, err := q.SetStockEntryLineScanned(ctx, db.SetStockEntryLineScannedParams{
			ID: line.ID, UserID: pgtype.Int8{Int64: c.Principal.UserInternal, Valid: true},
		}); err != nil {
			return err
		}
	}
	return nil
}

func sortedDecisions(m map[decisionKey]DiscrepancyDecision) []DiscrepancyDecision {
	keys := make([]decisionKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && (keys[j].kind+keys[j].barcode) < (keys[j-1].kind+keys[j-1].barcode); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	out := make([]DiscrepancyDecision, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// --- discrepancy record -----------------------------------------------------------

// Receipt returns the discrepancy record of an entry.
func (s *StockEntries) Receipt(ctx context.Context, c EntryCaller, id uuid.UUID) (EntryReceipt, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return EntryReceipt{}, err
	}
	e, err := s.q.GetStockEntryByUUID(ctx, db.GetStockEntryByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return EntryReceipt{}, notFound(err, ErrEntryNotFound)
	}
	return s.receipt(ctx, s.q, e)
}

func (s *StockEntries) receipt(ctx context.Context, q *db.Queries, e db.StockEntry) (EntryReceipt, error) {
	out := EntryReceipt{
		EntryUUID: e.Uuid, Status: e.Status, ContainerNo: textPtr(e.ContainerNo), WaybillNo: textPtr(e.WaybillNo),
		ConfirmedAt: tsPtr(e.ConfirmedAt), Items: []EntryDiscrepancy{},
		PDFURL: "/v1/warehouse/stock-entries/" + e.Uuid.String() + "/receipt.pdf",
	}
	if e.ArrivalDate.Valid {
		d := e.ArrivalDate.Time.Format(time.DateOnly)
		out.ArrivalDate = &d
	}
	if e.WarehouseID.Valid {
		w, err := q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: e.WarehouseID.Int64, OrganizationID: e.OrganizationID})
		if err != nil {
			return out, err
		}
		out.Warehouse = &EntryWarehouse{UUID: w.Uuid, Code: w.Code, Name: w.Name}
	}
	if e.SupplierID.Valid {
		sup, err := q.GetStockEntrySupplierDetails(ctx, e.SupplierID.Int64)
		if err != nil {
			return out, err
		}
		out.Supplier = &ReceiptSupplier{UUID: sup.Uuid, Name: sup.Name, TaxNo: textPtr(sup.TaxNo),
			Phone: textPtr(sup.PhoneE164), Email: textPtr(sup.Email)}
	}
	if e.ConfirmedByUserID.Valid {
		u, err := q.GetStockEntryUserRef(ctx, e.ConfirmedByUserID.Int64)
		if err != nil {
			return out, err
		}
		out.ConfirmedBy = &EntryUser{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}
	}
	n, err := q.CountStockEntryLines(ctx, e.ID)
	if err != nil {
		return out, err
	}
	if e.Status == EntryStatusConfirmed {
		out.Summary.Entered = n
	}
	items, err := s.discrepancies(ctx, q, e)
	if err != nil {
		return out, err
	}
	out.Items = items
	for _, it := range items {
		switch it.Kind + "." + it.Decision {
		case DiscrepancyMissing + "." + DecisionLeftOut:
			out.Summary.Missing++
		case DiscrepancyExtra + "." + DecisionAdded:
			out.Summary.ExtrasAdded++
		case DiscrepancyExtra + "." + DecisionRejected:
			out.Summary.ExtrasRejected++
		case DiscrepancyDamaged + "." + DecisionAccepted:
			out.Summary.DamagedAccepted++
		case DiscrepancyDamaged + "." + DecisionExcluded:
			out.Summary.DamagedExcluded++
		}
	}
	return out, nil
}

func (s *StockEntries) discrepancies(ctx context.Context, q *db.Queries, e db.StockEntry) ([]EntryDiscrepancy, error) {
	rows, err := q.ListStockEntryDiscrepancies(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	out := make([]EntryDiscrepancy, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	atts, err := s.attachmentRows(ctx, q, e, true)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		d := EntryDiscrepancy{
			UUID: r.Uuid, Kind: r.Kind, Decision: r.Decision, Barcode: r.Barcode, Quantity: r.Quantity,
			Reason: textPtr(r.Reason), Attachments: []EntryAttachment{}, CreatedAt: ts(r.CreatedAt),
		}
		if r.ProductUuid.Valid {
			d.Product = &EntryProduct{UUID: uuid.UUID(r.ProductUuid.Bytes), SKU: r.ProductSku.String, Name: r.ProductName.String}
		}
		if r.UserUuid.Valid {
			d.User = &EntryUser{UUID: uuid.UUID(r.UserUuid.Bytes), Name: strings.TrimSpace(r.UserName.String + " " + r.UserSurname.String)}
		}
		for _, id := range r.AttachmentIds {
			if a, ok := atts[id]; ok {
				d.Attachments = append(d.Attachments, a)
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// --- attachments ----------------------------------------------------------------------

func attachmentURL(entry, att uuid.UUID) string {
	return "/v1/warehouse/stock-entries/" + entry.String() + "/attachments/" + att.String()
}

// attachmentRows maps the live attachments of an entry by internal id.
func (s *StockEntries) attachmentRows(ctx context.Context, q *db.Queries, e db.StockEntry, _ bool) (map[int64]EntryAttachment, error) {
	rows, err := q.ListStockEntryAttachments(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]EntryAttachment, len(rows))
	for _, r := range rows {
		out[r.ID] = attachmentView(e.Uuid, r)
	}
	return out, nil
}

func attachmentView(entry uuid.UUID, r db.ListStockEntryAttachmentsRow) EntryAttachment {
	a := EntryAttachment{
		UUID: r.Uuid, Kind: r.Kind, Filename: r.Filename, MimeType: r.MimeType, SizeBytes: r.SizeBytes,
		URL: attachmentURL(entry, r.Uuid), CreatedAt: ts(r.CreatedAt),
	}
	if r.UploaderUuid.Valid {
		a.UploadedBy = &EntryUser{UUID: uuid.UUID(r.UploaderUuid.Bytes), Name: strings.TrimSpace(r.UploaderName.String + " " + r.UploaderSurname.String)}
	}
	return a
}

func (s *StockEntries) listAttachments(ctx context.Context, q *db.Queries, e db.StockEntry) ([]EntryAttachment, error) {
	rows, err := q.ListStockEntryAttachments(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	out := make([]EntryAttachment, 0, len(rows))
	for _, r := range rows {
		out = append(out, attachmentView(e.Uuid, r))
	}
	return out, nil
}

// lockFiled locks an entry that takes attachments: any status but cancelled,
// warehouse membership required (TEC-769).
func (s *StockEntries) lockFiled(ctx context.Context, q *db.Queries, c EntryCaller, org int64, id uuid.UUID) (db.StockEntry, error) {
	e, err := q.LockStockEntryByUUID(ctx, db.LockStockEntryByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return e, notFound(err, ErrEntryNotFound)
	}
	if e.Status == EntryStatusCancelled {
		return e, ErrEntryCancelled
	}
	if e.WarehouseID.Valid {
		if err := requireMember(ctx, q, c.Principal, org, e.WarehouseID.Int64); err != nil {
			return e, err
		}
	}
	return e, nil
}

// readAttachment reads and sniffs an upload: images (JPEG, PNG, WebP) and PDF.
func readAttachment(in AttachmentInput) ([]byte, string, string, error) {
	if in.Body == nil {
		return nil, "", "", invalid("file", "is required")
	}
	if in.Size > MaxEntryAttachmentBytes {
		return nil, "", "", invalid("file", "is too large (max 15 MB)")
	}
	data, err := io.ReadAll(io.LimitReader(in.Body, MaxEntryAttachmentBytes+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(data) == 0 {
		return nil, "", "", invalid("file", "is empty")
	}
	if len(data) > MaxEntryAttachmentBytes {
		return nil, "", "", invalid("file", "is too large (max 15 MB)")
	}
	mime := http.DetectContentType(data)
	switch {
	case strings.HasPrefix(mime, "image/jpeg"):
		return data, "image/jpeg", "jpg", nil
	case strings.HasPrefix(mime, "image/png"):
		return data, "image/png", "png", nil
	case strings.HasPrefix(mime, "image/webp"):
		return data, "image/webp", "webp", nil
	case strings.HasPrefix(mime, "application/pdf"):
		return data, "application/pdf", "pdf", nil
	}
	return nil, "", "", invalid("file", "must be a JPEG, PNG, WebP image or a PDF")
}

func cleanFilename(name, ext string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "file." + ext
	}
	if utf8.RuneCountInString(name) > 200 {
		name = string([]rune(name)[:200])
	}
	return name
}

// AddAttachment stores a waybill, invoice, photo or other file on an entry.
func (s *StockEntries) AddAttachment(ctx context.Context, c EntryCaller, id uuid.UUID, in AttachmentInput) (EntryAttachment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return EntryAttachment{}, err
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = AttachmentOther
	}
	switch kind {
	case AttachmentWaybill, AttachmentInvoice, AttachmentPhoto, AttachmentOther:
	default:
		return EntryAttachment{}, invalid("kind", "must be waybill, invoice, photo or other")
	}
	if s.store == nil {
		return EntryAttachment{}, ErrEntryStorage
	}
	data, mime, ext, err := readAttachment(in)
	if err != nil {
		return EntryAttachment{}, err
	}
	if kind == AttachmentPhoto && !strings.HasPrefix(mime, "image/") {
		return EntryAttachment{}, invalid("file", "a photo must be an image")
	}
	e, err := s.q.GetStockEntryByUUID(ctx, db.GetStockEntryByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return EntryAttachment{}, notFound(err, ErrEntryNotFound)
	}
	sum := sha256.Sum256(data)
	key := fmt.Sprintf("warehouse/stock-entries/%s/attachments/%s-%s.%s", e.Uuid, uuid.NewString(), hex.EncodeToString(sum[:4]), ext)
	name := cleanFilename(in.Filename, ext)
	// Checks before the upload; the row is written under the lock after it.
	if e.Status == EntryStatusCancelled {
		return EntryAttachment{}, ErrEntryCancelled
	}
	if e.WarehouseID.Valid {
		if err := requireMember(ctx, s.q, c.Principal, org, e.WarehouseID.Int64); err != nil {
			return EntryAttachment{}, err
		}
	}
	if err := s.store.Upload(ctx, platstorage.File{Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: mime, Filename: name}, key); err != nil {
		return EntryAttachment{}, fmt.Errorf("warehouse: upload attachment: %w", err)
	}
	var out EntryAttachment
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		e, err := s.lockFiled(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		have, err := q.ListStockEntryAttachments(ctx, e.ID)
		if err != nil {
			return err
		}
		if len(have) >= MaxEntryAttachments {
			return invalid("file", fmt.Sprintf("an entry holds at most %d attachments", MaxEntryAttachments))
		}
		row, err := q.InsertStockEntryAttachment(ctx, db.InsertStockEntryAttachmentParams{
			OrganizationID: e.OrganizationID, BrandID: e.BrandID, EntryID: e.ID, Kind: kind, StorageKey: key,
			Filename: name, MimeType: mime, SizeBytes: int64(len(data)), UploadedByUserID: c.actor(),
		})
		if err != nil {
			return err
		}
		out = EntryAttachment{UUID: row.Uuid, Kind: row.Kind, Filename: row.Filename, MimeType: row.MimeType,
			SizeBytes: row.SizeBytes, URL: attachmentURL(e.Uuid, row.Uuid), CreatedAt: ts(row.CreatedAt)}
		if c.Principal.UserInternal > 0 {
			if u, err := q.GetStockEntryUserRef(ctx, c.Principal.UserInternal); err == nil {
				out.UploadedBy = &EntryUser{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}
			}
		}
		return audit(ctx, q, c, AuditStockEntryAttachmentAdd, e, map[string]any{"kind": kind, "filename": name})
	})
	if err != nil {
		_ = s.store.Delete(context.WithoutCancel(ctx), key)
		return EntryAttachment{}, err
	}
	return out, nil
}

// AttachmentFile opens an attachment for download (read access).
func (s *StockEntries) AttachmentFile(ctx context.Context, c EntryCaller, id, att uuid.UUID) (io.ReadCloser, int64, EntryAttachment, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return nil, 0, EntryAttachment{}, err
	}
	e, err := s.q.GetStockEntryByUUID(ctx, db.GetStockEntryByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return nil, 0, EntryAttachment{}, notFound(err, ErrEntryNotFound)
	}
	a, err := s.q.GetStockEntryAttachment(ctx, db.GetStockEntryAttachmentParams{Uuid: att, EntryID: e.ID})
	if err != nil {
		return nil, 0, EntryAttachment{}, notFound(err, ErrEntryAttachmentNotFound)
	}
	if s.store == nil {
		return nil, 0, EntryAttachment{}, ErrEntryStorage
	}
	rc, size, err := s.store.Download(ctx, a.StorageKey)
	if err != nil {
		return nil, 0, EntryAttachment{}, ErrEntryAttachmentNotFound
	}
	return rc, size, EntryAttachment{UUID: a.Uuid, Kind: a.Kind, Filename: a.Filename, MimeType: a.MimeType, SizeBytes: a.SizeBytes}, nil
}

// DeleteAttachment removes an attachment (soft delete). Damage evidence of
// the discrepancy record stays.
func (s *StockEntries) DeleteAttachment(ctx context.Context, c EntryCaller, id, att uuid.UUID) error {
	org, err := guard(c.Caller)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		e, err := s.lockFiled(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		a, err := q.GetStockEntryAttachment(ctx, db.GetStockEntryAttachmentParams{Uuid: att, EntryID: e.ID})
		if err != nil {
			return notFound(err, ErrEntryAttachmentNotFound)
		}
		n, err := q.CountStockEntryAttachmentRefs(ctx, db.CountStockEntryAttachmentRefsParams{EntryID: e.ID, AttachmentID: a.ID})
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrEntryAttachmentInUse
		}
		if _, err := q.SoftDeleteStockEntryAttachment(ctx, a.ID); err != nil {
			return err
		}
		return audit(ctx, q, c, AuditStockEntryAttachmentDrop, e, map[string]any{"kind": a.Kind, "filename": a.Filename})
	})
}

// --- lot and production date ----------------------------------------------------------

func parseProductionDate(raw string) (pgtype.Date, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pgtype.Date{}, nil
	}
	t, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return pgtype.Date{}, invalid("production_date", "must be a date (YYYY-MM-DD)")
	}
	if t.After(time.Now().AddDate(0, 0, 1)) {
		return pgtype.Date{}, invalid("production_date", "cannot be in the future")
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func normLot(raw string) (pgtype.Text, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pgtype.Text{}, nil
	}
	if utf8.RuneCountInString(raw) > 64 || strings.ContainsAny(raw, "\r\n\t") {
		return pgtype.Text{}, invalid("lot", "must be at most 64 characters")
	}
	return pgtype.Text{String: raw, Valid: true}, nil
}

// setUnitLot writes the lot of a label that has not entered stock yet (a
// fixed barcode already in stock is shared and keeps its lot).
func setUnitLot(ctx context.Context, q *db.Queries, u db.Unit, lot pgtype.Text) error {
	switch ledger.Status(u.Status) {
	case ledger.StatusReserved, ledger.StatusPrinted:
		return q.SetUnitLot(ctx, db.SetUnitLotParams{ID: u.ID, Lot: lot})
	}
	return nil
}

// UpdateLine changes the lot, production date or box number of a draft line.
func (s *StockEntries) UpdateLine(ctx context.Context, c EntryCaller, id, lineID uuid.UUID, in LineDetailsInput) (StockEntry, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockEntry{}, err
	}
	var e db.StockEntry
	err = s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		if e, err = s.lockWritable(ctx, q, c, org, id); err != nil {
			return err
		}
		lines, err := q.ListStockEntryLines(ctx, e.ID)
		if err != nil {
			return err
		}
		var line *db.StockEntryLine
		for i := range lines {
			if lines[i].Uuid == lineID {
				line = &lines[i]
			}
		}
		if line == nil {
			return ErrEntryLineNotFound
		}
		arg := db.SetStockEntryLineDetailsParams{Uuid: lineID, EntryID: e.ID, ProductionDate: line.ProductionDate, BoxNo: line.BoxNo}
		if in.ProductionDate != nil {
			if arg.ProductionDate, err = parseProductionDate(*in.ProductionDate); err != nil {
				return err
			}
		}
		if in.BoxNo != nil {
			b := strings.TrimSpace(*in.BoxNo)
			if utf8.RuneCountInString(b) > maxDeliveryRefLen {
				return invalid("box_no", "must be at most 64 characters")
			}
			arg.BoxNo = pgtype.Text{String: b, Valid: b != ""}
		}
		if _, err := q.SetStockEntryLineDetails(ctx, arg); err != nil {
			return err
		}
		u, err := q.GetUnit(ctx, line.UnitID)
		if err != nil {
			return err
		}
		if in.Lot != nil {
			lot, err := normLot(*in.Lot)
			if err != nil {
				return err
			}
			if err := setUnitLot(ctx, q, u, lot); err != nil {
				return err
			}
		}
		return audit(ctx, q, c, AuditStockEntryLineUpdated, e, map[string]any{"barcode": u.Barcode})
	})
	if err != nil {
		return StockEntry{}, err
	}
	return s.view(ctx, s.q, e, true)
}

// applyLineDetails sets the lot and production date of newly added lines.
func applyLineDetails(ctx context.Context, q *db.Queries, e db.StockEntry, units []db.Unit, lot pgtype.Text, date pgtype.Date) error {
	if !lot.Valid && !date.Valid {
		return nil
	}
	ids := make([]int64, 0, len(units))
	for _, u := range units {
		ids = append(ids, u.ID)
		if lot.Valid {
			if err := setUnitLot(ctx, q, u, lot); err != nil {
				return err
			}
		}
	}
	if date.Valid {
		return q.SetStockEntryLinesProductionDate(ctx, db.SetStockEntryLinesProductionDateParams{
			ProductionDate: date, EntryID: e.ID, UnitIds: ids,
		})
	}
	return nil
}

// --- undo ----------------------------------------------------------------------------

func (s *StockEntries) requireAdjust(ctx context.Context, c EntryCaller, org int64) error {
	f, err := scopefilter.Resolve(ctx, s.q, c.Principal, &c.Org, rbac.PermStockAdjust)
	if err != nil || f.UserOnly() || !f.AllowsOrg(org, c.Org.BrandID) {
		return ErrEntryAdjustForbidden
	}
	return nil
}

// undoBlockers lists the lines whose units moved after the entry.
func undoBlockers(ctx context.Context, q *db.Queries, lines []db.StockEntryLine) ([]UndoBlocker, error) {
	out := []UndoBlocker{}
	products := map[int64]db.Product{}
	for _, l := range lines {
		if l.UndoMovementID.Valid {
			continue
		}
		last := l.PlacementMovementID
		if !last.Valid {
			last = l.EntryMovementID
		}
		if !last.Valid {
			continue
		}
		later, err := q.ListUnitMovementsAfter(ctx, db.ListUnitMovementsAfterParams{UnitID: l.UnitID, AfterID: last.Int64})
		if err != nil {
			return nil, err
		}
		if len(later) == 0 {
			continue
		}
		u, err := q.GetUnit(ctx, l.UnitID)
		if err != nil {
			return nil, err
		}
		p, ok := products[u.ProductID]
		if !ok {
			if p, err = q.GetProductByIDAnyBrand(ctx, u.ProductID); err != nil {
				return nil, err
			}
			products[u.ProductID] = p
		}
		m := later[0]
		out = append(out, UndoBlocker{
			Barcode: u.Barcode, Product: EntryProduct{UUID: p.Uuid, SKU: p.Sku, Name: p.Name},
			MovementType: m.Type, MovementUUID: m.Uuid, At: ts(m.CreatedAt),
		})
	}
	return out, nil
}

func undoable(e db.StockEntry) error {
	if e.Mode == EntryModeImport {
		return ErrEntryUndoImport
	}
	if e.Status != EntryStatusConfirmed {
		return ErrEntryNotConfirmed
	}
	return nil
}

// CheckUndo tells whether a confirmed entry can be undone and lists the
// blocking units.
func (s *StockEntries) CheckUndo(ctx context.Context, c EntryCaller, id uuid.UUID) (UndoCheck, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return UndoCheck{}, err
	}
	e, err := s.q.GetStockEntryByUUID(ctx, db.GetStockEntryByUUIDParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return UndoCheck{}, notFound(err, ErrEntryNotFound)
	}
	out := UndoCheck{Blockers: []UndoBlocker{}}
	if err := undoable(e); err != nil {
		r := "not_confirmed"
		if errors.Is(err, ErrEntryUndoImport) {
			r = "import"
		}
		out.Reason = &r
		return out, nil
	}
	lines, err := s.q.ListStockEntryLines(ctx, e.ID)
	if err != nil {
		return out, err
	}
	out.Lines = len(lines)
	if out.Blockers, err = undoBlockers(ctx, s.q, lines); err != nil {
		return out, err
	}
	out.Allowed = len(out.Blockers) == 0
	if !out.Allowed {
		r := "moved"
		out.Reason = &r
	}
	return out, nil
}

// Undo voids every line of a confirmed manual entry through the ledger (the
// reverse of the entry) and marks the entry undone. Any unit that moved
// after the entry refuses the whole undo (UndoBlockedError).
func (s *StockEntries) Undo(ctx context.Context, c EntryCaller, id uuid.UUID, reason string) (StockEntry, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return StockEntry{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return StockEntry{}, invalid("reason", "is required")
	}
	if utf8.RuneCountInString(reason) > maxReasonLen {
		return StockEntry{}, invalid("reason", "must be at most 500 characters")
	}
	if err := s.requireAdjust(ctx, c, org); err != nil {
		return StockEntry{}, err
	}
	var e db.StockEntry
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		e, err = q.LockStockEntryByUUID(ctx, db.LockStockEntryByUUIDParams{Uuid: id, OrganizationID: org})
		if err != nil {
			return notFound(err, ErrEntryNotFound)
		}
		if err := undoable(e); err != nil {
			return err
		}
		if e.WarehouseID.Valid {
			if err := requireMember(ctx, q, c.Principal, org, e.WarehouseID.Int64); err != nil {
				return err
			}
		}
		lines, err := q.ListStockEntryLines(ctx, e.ID)
		if err != nil {
			return err
		}
		for _, l := range lines {
			if _, err := q.LockUnit(ctx, l.UnitID); err != nil {
				return err
			}
		}
		blockers, err := undoBlockers(ctx, q, lines)
		if err != nil {
			return err
		}
		if len(blockers) > 0 {
			return &UndoBlockedError{Blockers: blockers}
		}
		for _, l := range lines {
			if l.UndoMovementID.Valid || !l.LocationID.Valid {
				continue
			}
			u, err := q.GetUnit(ctx, l.UnitID)
			if err != nil {
				return err
			}
			from := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: l.LocationID.Int64, OrgID: e.OrganizationID}
			m := ledger.Movement{
				Type: ledger.TypeVoid, UnitID: u.ID, From: &from,
				Source: entryUndoSource, RefType: entryRefType, RefID: l.ID,
				ActorUserID: actorPtr(c), Reason: "stock entry undo: " + reason,
				Metadata: map[string]any{"stock_entry_uuid": e.Uuid.String(), "line_uuid": l.Uuid.String()},
			}
			if u.UnitKind == ledger.KindFixed {
				m.Quantity = l.Quantity
			}
			res, err := s.ledger.Post(ctx, tx, m)
			if err != nil {
				return fmt.Errorf("%w: void %s: %w", ErrEntryLedger, u.Barcode, err)
			}
			if err := q.SetStockEntryLineUndoMovement(ctx, db.SetStockEntryLineUndoMovementParams{
				UndoMovementID: pgtype.Int8{Int64: res.Movement.ID, Valid: true}, ID: l.ID,
			}); err != nil {
				return err
			}
		}
		if e, err = q.MarkManualStockEntryUndone(ctx, db.MarkManualStockEntryUndoneParams{
			UserID: c.actor(), Reason: pgtype.Text{String: reason, Valid: true}, ID: e.ID,
		}); err != nil {
			return notFound(err, ErrEntryNotConfirmed)
		}
		return audit(ctx, q, c, AuditStockEntryUndone, e, map[string]any{"lines": len(lines), "reason": reason})
	})
	if err != nil {
		return StockEntry{}, err
	}
	return s.view(ctx, s.q, e, true)
}

// --- label prints (audit only) --------------------------------------------------------

// RecordLabelPrint records that the labels of an entry were printed (the
// timeline step; the label status itself belongs to the stock labels).
func (s *StockEntries) RecordLabelPrint(ctx context.Context, c EntryCaller, id uuid.UUID, count int) error {
	org, err := guard(c.Caller)
	if err != nil {
		return err
	}
	if count < 0 || count > MaxEntryLines {
		return invalid("count", fmt.Sprintf("must be between 0 and %d", MaxEntryLines))
	}
	return s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		e, err := s.lockFiled(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		if count == 0 {
			n, err := q.CountStockEntryLines(ctx, e.ID)
			if err != nil {
				return err
			}
			count = int(n)
		}
		return audit(ctx, q, c, AuditStockEntryLabelsPrinted, e, map[string]any{"count": count})
	})
}
