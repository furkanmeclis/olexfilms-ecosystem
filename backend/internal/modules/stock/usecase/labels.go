package usecase

// Label printing (TEC-202): a batch, a list of unit barcodes or a set of
// warehouse locations rendered as a label sheet PDF through the single PDF
// engine (pdfrender / Gotenberg). The template is the one asked for, the
// batch's, the organization's default of the kind, or the built-in one.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/credits"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/labels"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Renderer converts HTML to PDF (satisfied by *pdfrender.Client).
type Renderer interface {
	Convert(ctx context.Context, req pdfrender.Request) ([]byte, error)
}

// ErrRendererUnavailable: the PDF engine is not configured or failed.
var ErrRendererUnavailable = errors.New("stock: label renderer unavailable")

// MaxLabelsPerSheet bounds one ad-hoc print request.
const MaxLabelsPerSheet = 1000

// Labels implements label printing.
type Labels struct {
	q   *db.Queries
	pdf Renderer
}

// NewLabels builds the use case.
func NewLabels(q *db.Queries, pdf Renderer) *Labels { return &Labels{q: q, pdf: pdf} }

// PrintBatch renders every unit of the batch (center only) and counts the
// print.
func (s *Labels) PrintBatch(ctx context.Context, c Caller, batchID uuid.UUID, templateID *uuid.UUID) ([]byte, error) {
	org, err := centerOrg(c)
	if err != nil {
		return nil, err
	}
	b, err := s.q.GetBarcodeBatchByUUID(ctx, db.GetBarcodeBatchByUUIDParams{Uuid: batchID, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stock: batch: %w", err)
	}
	units, err := s.q.ListUnitsByBatch(ctx, i8(b.ID))
	if err != nil {
		return nil, fmt.Errorf("stock: batch units: %w", err)
	}
	printable := units[:0]
	reserved := int64(0)
	for _, u := range units {
		if u.Status != string(ledger.StatusVoid) {
			printable = append(printable, u)
			if u.Status == string(ledger.StatusReserved) {
				reserved++
			}
		}
	}
	if len(printable) == 0 {
		return nil, ErrNotFound
	}
	var fallback *int64
	if b.TemplateID.Valid {
		fallback = &b.TemplateID.Int64
	}
	tpl, err := s.resolveTemplate(ctx, org, labels.KindUnit, templateID, fallback)
	if err != nil {
		return nil, err
	}
	items, err := s.unitItems(ctx, org, printable)
	if err != nil {
		return nil, err
	}
	pdf, err := s.render(ctx, org, tpl, items)
	if err != nil {
		return nil, err
	}
	marked, err := s.q.MarkBatchUnitsPrinted(ctx, i8(b.ID))
	if err != nil {
		return nil, fmt.Errorf("stock: mark labels printed: %w", err)
	}
	if marked != reserved {
		return nil, ErrNotFound
	}
	if _, err := s.q.MarkBarcodeBatchPrinted(ctx, b.ID); err != nil {
		return nil, fmt.Errorf("stock: batch printed: %w", err)
	}
	return pdf, nil
}

// PrintUnits renders the labels of the given barcodes. A unit is printable
// when its holder is inside the caller's stock.read reach, or, before it
// enters stock (reserved or printed, no state), when the caller is its issuer.
// TEC-1224: rendering never changes a unit; the client confirms the print with
// MarkUnitsPrinted (POST /v1/stock/labels/units/mark-printed), so a preview
// leaves reserved units (and their stock entry) cancellable.
func (s *Labels) PrintUnits(ctx context.Context, c Caller, barcodes []string, templateID *uuid.UUID) ([]byte, error) {
	org, units, err := s.printableUnits(ctx, c, barcodes)
	if err != nil {
		return nil, err
	}
	tpl, err := s.resolveTemplate(ctx, org, labels.KindUnit, templateID, nil)
	if err != nil {
		return nil, err
	}
	items, err := s.unitItems(ctx, org, units)
	if err != nil {
		return nil, err
	}
	return s.render(ctx, org, tpl, items)
}

// MarkUnitsPrinted records that the labels of the given barcodes were printed
// (TEC-1224): reserved units become printed. The same barcodes as PrintUnits
// are accepted; units already printed or in stock are left as they are, so the
// call is idempotent. It returns how many units changed.
func (s *Labels) MarkUnitsPrinted(ctx context.Context, c Caller, barcodes []string) (int64, error) {
	_, units, err := s.printableUnits(ctx, c, barcodes)
	if err != nil {
		return 0, err
	}
	ids := make([]int64, 0, len(units))
	for _, u := range units {
		if u.Status == string(ledger.StatusReserved) {
			ids = append(ids, u.ID)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	marked, err := s.q.MarkSelectedUnitsPrinted(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("stock: mark labels printed: %w", err)
	}
	return marked, nil
}

// printableUnits resolves the barcodes to units the caller may print, in the
// order given; any unknown or out-of-reach barcode is ErrNotFound.
func (s *Labels) printableUnits(ctx context.Context, c Caller, barcodes []string) (int64, []db.Unit, error) {
	org, err := labelOrg(c)
	if err != nil {
		return 0, nil, err
	}
	codes := uniqueTrimmed(barcodes)
	switch {
	case len(codes) == 0:
		return 0, nil, invalid("barcode", "at least one barcode is required")
	case len(codes) > MaxLabelsPerSheet:
		return 0, nil, invalid("barcode", fmt.Sprintf("at most %d barcodes per sheet", MaxLabelsPerSheet))
	}
	rows, err := s.q.ListUnitsByBarcodes(ctx, codes)
	if err != nil {
		return 0, nil, fmt.Errorf("stock: units: %w", err)
	}
	byCode := map[string]db.Unit{}
	for _, u := range rows {
		if _, seen := byCode[u.Barcode]; seen && u.BrandID != c.Org.BrandID {
			continue // barcodes are unique per brand; the active brand wins
		}
		ok, err := s.printable(ctx, c, org, u)
		if err != nil {
			return 0, nil, err
		}
		if ok {
			byCode[u.Barcode] = u
		}
	}
	units := make([]db.Unit, 0, len(codes))
	for _, code := range codes {
		u, ok := byCode[code]
		if !ok {
			return 0, nil, ErrNotFound
		}
		units = append(units, u)
	}
	return org, units, nil
}

func (s *Labels) printable(ctx context.Context, c Caller, org int64, u db.Unit) (bool, error) {
	if u.Status == string(ledger.StatusVoid) {
		return false, nil
	}
	st, err := currentState(ctx, s.q, u.ID)
	if err != nil {
		return false, err
	}
	if st == nil {
		if u.UnitKind == ledger.KindFixed {
			// Fixed barcodes have holdings, not one state: the issuer prints.
			return u.OrganizationID == org, nil
		}
		return u.OrganizationID == org && (u.Status == string(ledger.StatusPrinted) || u.Status == string(ledger.StatusReserved)), nil
	}
	return c.Filter.AllowsOrg(st.HolderOrgID, st.BrandID), nil
}

// LocationPrintInput names the locations to print: explicit uuids and/or
// every typed location of a room.
type LocationPrintInput struct {
	LocationUUIDs []uuid.UUID
	RoomUUID      *uuid.UUID
	WarehouseUUID *uuid.UUID
	TemplateUUID  *uuid.UUID
}

// PrintLocations renders OFW:LOC QR labels of the active organization's
// typed locations (center or distributor; the caller holds warehouse.read).
func (s *Labels) PrintLocations(ctx context.Context, c Caller, in LocationPrintInput) ([]byte, error) {
	org, err := labelOrg(c)
	if err != nil {
		return nil, err
	}
	var rows []db.WarehouseLocation
	if in.WarehouseUUID != nil {
		warehouse, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: *in.WarehouseUUID, OrganizationID: org})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("stock: warehouse: %w", err)
		}
		rooms, err := s.q.ListRooms(ctx, db.ListRoomsParams{WarehouseID: warehouse.ID, OrganizationID: org})
		if err != nil {
			return nil, fmt.Errorf("stock: rooms: %w", err)
		}
		for _, room := range rooms {
			locations, err := s.q.ListRoomLocations(ctx, db.ListRoomLocationsParams{RoomID: i8(room.ID), OrganizationID: org})
			if err != nil {
				return nil, fmt.Errorf("stock: locations: %w", err)
			}
			rows = append(rows, locations...)
		}
	}
	if in.RoomUUID != nil {
		r, err := s.q.GetRoomByUUID(ctx, db.GetRoomByUUIDParams{Uuid: *in.RoomUUID, OrganizationID: org})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("stock: room: %w", err)
		}
		if rows, err = s.q.ListRoomLocations(ctx, db.ListRoomLocationsParams{RoomID: i8(r.ID), OrganizationID: org}); err != nil {
			return nil, fmt.Errorf("stock: locations: %w", err)
		}
	}
	if len(in.LocationUUIDs) > 0 {
		more, err := s.q.ListLocationsByUUIDs(ctx, db.ListLocationsByUUIDsParams{Uuids: in.LocationUUIDs, OrganizationID: org})
		if err != nil {
			return nil, fmt.Errorf("stock: locations: %w", err)
		}
		if len(more) != len(uniqueUUIDs(in.LocationUUIDs)) {
			return nil, ErrNotFound
		}
		rows = append(rows, more...)
	}
	items := make([]labels.Item, 0, len(rows))
	seen := map[int64]bool{}
	for _, l := range rows {
		if !l.FullCode.Valid || seen[l.ID] {
			continue
		}
		seen[l.ID] = true
		items = append(items, labels.Item{Code: l.FullCode.String, Title: l.Name, Subtitle: l.Type.String})
	}
	switch {
	case len(items) == 0:
		return nil, invalid("location", "at least one location with a full_code is required")
	case len(items) > MaxLabelsPerSheet:
		return nil, invalid("location", fmt.Sprintf("at most %d locations per sheet", MaxLabelsPerSheet))
	}
	tpl, err := s.resolveTemplate(ctx, org, labels.KindLocation, in.TemplateUUID, nil)
	if err != nil {
		return nil, err
	}
	return s.render(ctx, org, tpl, items)
}

// resolveTemplate picks the template: the requested one (must be of the
// kind), the fallback id, the organization's default, else the built-in.
func (s *Labels) resolveTemplate(ctx context.Context, org int64, kind string, requested *uuid.UUID, fallback *int64) (labels.Template, error) {
	if requested != nil {
		t, err := s.q.GetLabelTemplateByUUID(ctx, db.GetLabelTemplateByUUIDParams{Uuid: *requested, OrganizationID: org})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.Kind != kind) {
			return labels.Template{}, invalid("template", kind+" label template not found")
		}
		if err != nil {
			return labels.Template{}, fmt.Errorf("stock: template: %w", err)
		}
		return templateOf(t), nil
	}
	if fallback != nil {
		t, err := s.q.GetLabelTemplateByID(ctx, *fallback)
		if err == nil && t.OrganizationID == org && t.Kind == kind {
			return templateOf(t), nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return labels.Template{}, fmt.Errorf("stock: template: %w", err)
		}
	}
	t, err := s.q.GetDefaultLabelTemplate(ctx, db.GetDefaultLabelTemplateParams{OrganizationID: org, Kind: kind})
	switch {
	case err == nil:
		return templateOf(t), nil
	case !errors.Is(err, pgx.ErrNoRows):
		return labels.Template{}, fmt.Errorf("stock: default template: %w", err)
	case kind == labels.KindLocation:
		return labels.DefaultLocation, nil
	default:
		return labels.DefaultUnit, nil
	}
}

// unitItems adds the product name, SKU and meters to each unit. A cut
// piece that carries credits (a piece of a meter line, TEC-1241) also
// prints its remaining credits.
func (s *Labels) unitItems(ctx context.Context, org int64, units []db.Unit) ([]labels.Item, error) {
	products := map[int64]db.Product{}
	items := make([]labels.Item, 0, len(units))
	locale := i18n.Locale("")
	for _, u := range units {
		p, ok := products[u.ProductID]
		if !ok {
			var err error
			if p, err = s.q.GetProductByIDAnyBrand(ctx, u.ProductID); err != nil {
				return nil, fmt.Errorf("stock: product: %w", err)
			}
			products[u.ProductID] = p
		}
		sub := p.Sku
		if m := labelMeters(u); m.Valid {
			sub += " · " + numericString(m) + " m"
			b, ok, err := credits.Get(ctx, s.q, u)
			if err != nil {
				return nil, err
			}
			if ok {
				if locale == "" {
					locale = i18n.FallbackLocale
					if o, err := s.q.GetOrganizationByID(ctx, org); err == nil && o.Locale != "" {
						locale = i18n.Normalize(o.Locale)
					}
				}
				sub += " · " + i18n.TranslateParams(locale, "labels.unit.credits",
					map[string]string{"n": strconv.Itoa(int(b.Remaining))})
			}
		}
		items = append(items, labels.Item{Code: u.Barcode, Title: p.Name, Subtitle: sub})
	}
	return items, nil
}

// labelMeters is the length printed on a roll label: the remaining meters, so
// a reprint after a cut shows what is left (TEC-1224); the initial length
// only when no remaining value is recorded.
func labelMeters(u db.Unit) pgtype.Numeric {
	if u.RemainingMeters.Valid {
		return u.RemainingMeters
	}
	return u.InitialMeters
}

func (s *Labels) render(ctx context.Context, org int64, tpl labels.Template, items []labels.Item) ([]byte, error) {
	lang := "en"
	if o, err := s.q.GetOrganizationByID(ctx, org); err == nil && o.Locale != "" {
		lang = o.Locale
	}
	req := pdfrender.Request{}
	var err error
	switch tpl.Output {
	case labels.OutputZPL:
		// TEC-1230: ZPL is text, the PDF engine is not involved.
		out, err := labels.ZPL(tpl, items)
		if err != nil {
			return nil, fmt.Errorf("stock: labels: %w", err)
		}
		return out, nil
	case labels.OutputSingle:
		req.HTML, err = labels.Single(lang, tpl, items)
		req.PageWidthMm, req.PageHeightMm = labels.PageSize(tpl)
	default:
		req.HTML, err = labels.Sheet(lang, tpl, items)
	}
	if err != nil {
		return nil, fmt.Errorf("stock: labels: %w", err)
	}
	if s.pdf == nil {
		return nil, ErrRendererUnavailable
	}
	pdf, err := s.pdf.Convert(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRendererUnavailable, err)
	}
	return pdf, nil
}

func uniqueTrimmed(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func uniqueUUIDs(in []uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(in))
	seen := map[uuid.UUID]bool{}
	for _, id := range in {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
