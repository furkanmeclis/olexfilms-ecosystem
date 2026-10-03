package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/labels"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-202 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL; CI
// runs PG18). The PDF engine is faked: CI has no Gotenberg, so the renderer
// records the HTML it received and answers PDF bytes; the real Gotenberg
// round trip is a manual check (TEC-140).

type fakeRenderer struct {
	html  []string
	fail  error
	calls int
}

func (f *fakeRenderer) Convert(_ context.Context, req pdfrender.Request) ([]byte, error) {
	f.calls++
	f.html = append(f.html, req.HTML)
	if f.fail != nil {
		return nil, f.fail
	}
	return []byte("%PDF-1.4\n% fake " + fmt.Sprint(f.calls)), nil
}

type labelEnv struct {
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	brand  db.Brand
	center db.Organization
	dist   db.Organization
	roll   db.Product
	piece  db.Product
	suffix string
	pdf    *fakeRenderer
	bc     *stockusecase.Barcodes
	tpl    *stockusecase.LabelTemplates
	lb     *stockusecase.Labels
}

func newLabelEnv(t *testing.T) *labelEnv {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	e := &labelEnv{ctx: ctx, pool: pool, q: q, suffix: fmt.Sprintf("%d", time.Now().UnixNano()), pdf: &fakeRenderer{}}
	if e.brand, err = q.GetBrandBySlug(ctx, "olex"); err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	if e.center, err = q.GetBrandCenter(ctx, e.brand.ID); err != nil {
		t.Fatalf("olex center: %v", err)
	}
	e.dist, err = q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t202-dist-" + e.suffix, Name: "dist", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "distributor", ParentID: pgtype.Int8{Int64: e.center.ID, Valid: true},
		BrandID: e.brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("distributor: %v", err)
	}
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: e.center.ID, BrandID: e.brand.ID, Name: "t202-cat-" + e.suffix,
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product := func(sku, unitType string) db.Product {
		p, err := q.CreateProduct(ctx, db.CreateProductParams{
			OrganizationID: e.center.ID, BrandID: e.brand.ID, CategoryID: cat.ID,
			Sku: sku + "-" + e.suffix, Name: sku, Images: []byte("[]"), UnitType: unitType, Active: true,
		})
		if err != nil {
			t.Fatalf("product %s: %v", sku, err)
		}
		return p
	}
	e.roll = product("t202-roll", "roll_meter")
	e.piece = product("t202-piece", "piece")
	e.bc = stockusecase.NewBarcodes(pool, q)
	e.tpl = stockusecase.NewLabelTemplates(q)
	e.lb = stockusecase.NewLabels(q, e.pdf)
	return e
}

func (e *labelEnv) caller(o db.Organization) stockusecase.Caller {
	return stockusecase.Caller{
		Org:    orgctx.Scope{InternalID: o.ID, UUID: o.Uuid, Slug: o.Slug, OrgType: o.Type, BrandID: o.BrandID, BrandSlug: e.brand.Slug},
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{o.ID}, OrgID: o.ID},
	}
}

// The center reserves 5 roll barcodes: the units exist in status printed
// with the batch link and the roll length; the barcodes are consecutive and
// never shaped like a split barcode. A second batch continues the counter.
// A custom prefix is honoured. The distributor is refused (K14) and so is a
// center caller whose grant does not reach the organization.
func TestBarcodeBatchReserve(t *testing.T) {
	e := newLabelEnv(t)
	c := e.caller(e.center)
	prefix := "T" + e.suffix[len(e.suffix)-7:]
	b, err := e.bc.Create(e.ctx, c, stockusecase.BatchInput{ProductUUID: e.roll.Uuid.String(), Quantity: 5, Meters: "15", Prefix: prefix})
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if b.Quantity != 5 || b.FirstBarcode != prefix+"-00000001" || b.LastBarcode != prefix+"-00000005" || len(b.Units) != 5 {
		t.Fatalf("batch = %+v", b)
	}
	if b.Meters == nil || *b.Meters != "15.00" || b.Product.UUID != e.roll.Uuid || b.LabelsURL != "/v1/stock/barcodes/"+b.UUID.String()+"/labels.pdf" {
		t.Fatalf("batch view = %+v", b)
	}
	for i, u := range b.Units {
		if u.Status != "printed" || u.Barcode != fmt.Sprintf("%s-%08d", prefix, i+1) || strings.Contains(u.Barcode, "-S") {
			t.Fatalf("unit %d = %+v", i, u)
		}
		row, err := e.q.GetUnitByUUID(e.ctx, u.UUID)
		if err != nil {
			t.Fatal(err)
		}
		if row.Status != "printed" || !row.BatchID.Valid || row.OrganizationID != e.center.ID || row.BrandID != e.brand.ID ||
			row.ProductID != e.roll.ID || row.UnitKind != "serial" || row.Source != "generated" {
			t.Fatalf("unit row = %+v", row)
		}
		if cm, err := ledger.NumericToCentimeters(row.InitialMeters); err != nil || cm != 1500 {
			t.Fatalf("initial meters = %d cm, %v", cm, err)
		}
	}
	// No ledger movement: a label is not stock yet.
	var n int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM stock_movements m JOIN units u ON u.id = m.unit_id WHERE u.batch_id IS NOT NULL AND u.organization_id = $1 AND u.barcode LIKE $2`, e.center.ID, prefix+"-%").Scan(&n); err != nil || n != 0 {
		t.Fatalf("movements = %d, %v", n, err)
	}

	// Counter continues; pieces need no meters.
	b2, err := e.bc.Create(e.ctx, c, stockusecase.BatchInput{ProductUUID: e.piece.Uuid.String(), Quantity: 2, Prefix: prefix})
	if err != nil || b2.FirstBarcode != prefix+"-00000006" || b2.LastBarcode != prefix+"-00000007" || b2.Meters != nil {
		t.Fatalf("second batch = %+v %v", b2, err)
	}
	got, err := e.bc.Get(e.ctx, c, b2.UUID)
	if err != nil || len(got.Units) != 2 {
		t.Fatalf("get batch = %+v %v", got, err)
	}
	list, total, err := e.bc.List(e.ctx, c, 50, 0)
	if err != nil || total < 2 || len(list) < 2 {
		t.Fatalf("list = %d %v", total, err)
	}

	// Validation.
	var ve *stockusecase.ValidationError
	if _, err := e.bc.Create(e.ctx, c, stockusecase.BatchInput{ProductUUID: e.roll.Uuid.String(), Quantity: 3}); !errors.As(err, &ve) || ve.Field != "meters" {
		t.Fatalf("roll without meters: %v", err)
	}
	if _, err := e.bc.Create(e.ctx, c, stockusecase.BatchInput{ProductUUID: e.piece.Uuid.String(), Quantity: 0}); !errors.As(err, &ve) || ve.Field != "quantity" {
		t.Fatalf("quantity 0: %v", err)
	}
	if _, err := e.bc.Create(e.ctx, c, stockusecase.BatchInput{ProductUUID: uuid.NewString(), Quantity: 1}); !errors.As(err, &ve) || ve.Field != "product_uuid" {
		t.Fatalf("unknown product: %v", err)
	}

	// Only the center (K14).
	if _, err := e.bc.Create(e.ctx, e.caller(e.dist), stockusecase.BatchInput{ProductUUID: e.piece.Uuid.String(), Quantity: 1}); !errors.Is(err, stockusecase.ErrBarcodesCenterOnly) {
		t.Fatalf("distributor batch: %v", err)
	}
	if _, _, err := e.bc.List(e.ctx, e.caller(e.dist), 10, 0); !errors.Is(err, stockusecase.ErrBarcodesCenterOnly) {
		t.Fatalf("distributor list: %v", err)
	}
	out := c
	out.Filter = scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{e.dist.ID}}
	if _, err := e.bc.Create(e.ctx, out, stockusecase.BatchInput{ProductUUID: e.piece.Uuid.String(), Quantity: 1}); !errors.Is(err, stockusecase.ErrBarcodesCenterOnly) {
		t.Fatalf("out-of-reach grant: %v", err)
	}
}

// Template CRUD: create (defaults of the kind), list, get, update (default
// switch, logo modes), name conflict, delete; the distributor manages its
// own templates, the dealer none.
func TestLabelTemplateCRUD(t *testing.T) {
	e := newLabelEnv(t)
	c := e.caller(e.center)
	name := "Unit 70x37 " + e.suffix
	tp, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{Name: name, IsDefault: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tp.Kind != "unit" || tp.Symbology != "code128" || tp.LogoMode != "none" || tp.WidthMm != "70.0" || tp.HeightMm != "37.0" ||
		tp.Columns != 3 || !tp.ShowName || !tp.ShowCodeText || !tp.IsDefault || !tp.Active {
		t.Fatalf("template = %+v", tp)
	}
	if _, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{Name: name}); !errors.Is(err, stockusecase.ErrTemplateNameTaken) {
		t.Fatalf("duplicate name: %v", err)
	}
	var ve *stockusecase.ValidationError
	if _, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{Name: "x" + e.suffix, Kind: "location", Symbology: "code128"}); !errors.As(err, &ve) || ve.Field != "symbology" {
		t.Fatalf("location code128: %v", err)
	}
	if _, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{Name: "x" + e.suffix, LogoMode: "image", LogoImage: "http://x/logo.png"}); !errors.As(err, &ve) || ve.Field != "logo_image" {
		t.Fatalf("remote logo: %v", err)
	}
	w := "50"
	loc, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{
		Name: "Loc " + e.suffix, Kind: "location", LogoMode: "text", LogoText: "OLEX", WidthMm: &w, HeightMm: &w, IsDefault: true,
	})
	if err != nil || loc.Symbology != "qr" || loc.LogoText == nil || *loc.LogoText != "OLEX" || !loc.IsDefault {
		t.Fatalf("location template = %+v %v", loc, err)
	}
	list, err := e.tpl.List(e.ctx, c, "")
	if err != nil || len(list) < 2 {
		t.Fatalf("list = %d %v", len(list), err)
	}
	units, err := e.tpl.List(e.ctx, c, "unit")
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range units {
		if x.Kind != "unit" {
			t.Fatalf("kind filter leaked %+v", x)
		}
	}
	got, err := e.tpl.Get(e.ctx, c, tp.UUID)
	if err != nil || got.Name != name {
		t.Fatalf("get = %+v %v", got, err)
	}

	// Update: QR + image logo, two columns; a second unit default replaces the first.
	cols := 2
	up, err := e.tpl.Update(e.ctx, c, tp.UUID, stockusecase.TemplateInput{
		Name: name + " v2", Symbology: "qr", LogoMode: "image", LogoImage: "data:image/png;base64,iVBORw0KGgo=", Columns: &cols, IsDefault: true,
	})
	if err != nil || up.Symbology != "qr" || up.LogoMode != "image" || up.Columns != 2 || !up.IsDefault || up.LogoText != nil {
		t.Fatalf("update = %+v %v", up, err)
	}
	second, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{Name: "Unit B " + e.suffix, IsDefault: true})
	if err != nil || !second.IsDefault {
		t.Fatalf("second default = %+v %v", second, err)
	}
	if got, _ = e.tpl.Get(e.ctx, c, tp.UUID); got.IsDefault {
		t.Fatal("first template still default")
	}

	// Reach: the distributor sees only its own; the dealer type has none.
	dc := e.caller(e.dist)
	if _, err := e.tpl.Get(e.ctx, dc, tp.UUID); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("distributor reads center template: %v", err)
	}
	dealer := dc
	dealer.Org.OrgType = rbac.OrgTypeDealer
	if _, err := e.tpl.List(e.ctx, dealer, ""); !errors.Is(err, stockusecase.ErrLabelsForbidden) {
		t.Fatalf("dealer templates: %v", err)
	}
	if err := e.tpl.Delete(e.ctx, dc, tp.UUID); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("distributor deletes center template: %v", err)
	}
	if err := e.tpl.Delete(e.ctx, c, tp.UUID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := e.tpl.Get(e.ctx, c, tp.UUID); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("deleted template: %v", err)
	}
}

// Printing: the batch sheet goes through the renderer with one label per
// unit and counts the print; the ad-hoc unit sheet prints the issuer's
// printed units but not another organization's; location labels carry the
// OFW:LOC payload of the full_code; a renderer failure maps to
// ErrRendererUnavailable.
func TestLabelPrinting(t *testing.T) {
	e := newLabelEnv(t)
	c := e.caller(e.center)
	prefix := "P" + e.suffix[len(e.suffix)-7:]
	b, err := e.bc.Create(e.ctx, c, stockusecase.BatchInput{ProductUUID: e.roll.Uuid.String(), Quantity: 3, Meters: "20.5", Prefix: prefix})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	pdf, err := e.lb.PrintBatch(e.ctx, c, b.UUID, nil)
	if err != nil || !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Fatalf("print batch: %q %v", pdf, err)
	}
	html := e.pdf.html[len(e.pdf.html)-1]
	if strings.Count(html, `<div class="label">`) != 3 || !strings.Contains(html, prefix+"-00000001") || !strings.Contains(html, "20.50 m") ||
		!strings.Contains(html, `<img class="code"`) || !strings.Contains(html, `lang="tr`) {
		t.Fatalf("batch sheet html = %.300s", html)
	}
	if got, _ := e.bc.Get(e.ctx, c, b.UUID); got.PrintCount != 1 || got.LastPrintedAt == nil {
		t.Fatalf("print count = %+v", got)
	}
	if _, err := e.lb.PrintBatch(e.ctx, e.caller(e.dist), b.UUID, nil); !errors.Is(err, stockusecase.ErrBarcodesCenterOnly) {
		t.Fatalf("distributor prints batch: %v", err)
	}

	// Ad-hoc unit labels with a QR template.
	qr, err := e.tpl.Create(e.ctx, c, stockusecase.TemplateInput{Name: "QR " + e.suffix, Symbology: "qr"})
	if err != nil {
		t.Fatal(err)
	}
	pdf, err = e.lb.PrintUnits(e.ctx, c, []string{b.Units[0].Barcode, b.Units[2].Barcode, b.Units[0].Barcode}, &qr.UUID)
	if err != nil || len(pdf) == 0 {
		t.Fatalf("print units: %v", err)
	}
	html = e.pdf.html[len(e.pdf.html)-1]
	if strings.Count(html, `<div class="label">`) != 2 || !strings.Contains(html, `<img class="qr"`) {
		t.Fatalf("unit sheet html = %.300s", html)
	}
	if _, err := e.lb.PrintUnits(e.ctx, e.caller(e.dist), []string{b.Units[0].Barcode}, nil); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("distributor prints center's printed unit: %v", err)
	}
	if _, err := e.lb.PrintUnits(e.ctx, c, []string{"NOPE-" + e.suffix}, nil); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("unknown barcode: %v", err)
	}
	if _, err := e.lb.PrintUnits(e.ctx, c, []string{b.Units[0].Barcode}, &b.UUID); err == nil {
		t.Fatal("unknown template accepted")
	}

	// Location labels of the distributor's own warehouse.
	dc := e.caller(e.dist)
	wh, err := e.q.CreateWarehouse(e.ctx, db.CreateWarehouseParams{OrganizationID: e.dist.ID, Code: "W" + e.suffix[len(e.suffix)-5:], Name: "Main", Active: true})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	room, err := e.q.CreateRoom(e.ctx, db.CreateRoomParams{OrganizationID: e.dist.ID, WarehouseID: wh.ID, Code: "R1", Name: "Room", Active: true})
	if err != nil {
		t.Fatalf("room: %v", err)
	}
	shelf, err := e.q.CreateTypedLocation(e.ctx, db.CreateTypedLocationParams{
		OrganizationID: e.dist.ID, RoomID: pgtype.Int8{Int64: room.ID, Valid: true}, Type: pgtype.Text{String: "shelf", Valid: true}, Code: "A", Name: "Shelf A", Active: true,
	})
	if err != nil {
		t.Fatalf("shelf: %v", err)
	}
	bin, err := e.q.CreateTypedLocation(e.ctx, db.CreateTypedLocationParams{
		OrganizationID: e.dist.ID, RoomID: pgtype.Int8{Int64: room.ID, Valid: true}, ParentID: pgtype.Int8{Int64: shelf.ID, Valid: true},
		Type: pgtype.Text{String: "bin", Valid: true}, Code: "01", Name: "Bin 1", Active: true,
	})
	if err != nil {
		t.Fatalf("bin: %v", err)
	}
	pdf, err = e.lb.PrintLocations(e.ctx, dc, stockusecase.LocationPrintInput{RoomUUID: &room.Uuid})
	if err != nil || len(pdf) == 0 {
		t.Fatalf("print room: %v", err)
	}
	html = e.pdf.html[len(e.pdf.html)-1]
	if strings.Count(html, `<div class="label">`) != 2 || !strings.Contains(html, bin.FullCode.String) || !strings.Contains(html, `<img class="qr"`) {
		t.Fatalf("location sheet html = %.300s", html)
	}
	if !strings.HasPrefix(labels.Payload(labels.KindLocation, bin.FullCode.String), "OFW:LOC:"+wh.Code+"-R1-A-01") {
		t.Fatalf("location payload for %s", bin.FullCode.String)
	}
	if _, err := e.lb.PrintLocations(e.ctx, c, stockusecase.LocationPrintInput{LocationUUIDs: []uuid.UUID{bin.Uuid}}); !errors.Is(err, stockusecase.ErrNotFound) {
		t.Fatalf("center prints distributor location: %v", err)
	}

	// Renderer failure.
	e.pdf.fail = errors.New("gotenberg down")
	if _, err := e.lb.PrintBatch(e.ctx, c, b.UUID, nil); !errors.Is(err, stockusecase.ErrRendererUnavailable) {
		t.Fatalf("renderer failure: %v", err)
	}
}
