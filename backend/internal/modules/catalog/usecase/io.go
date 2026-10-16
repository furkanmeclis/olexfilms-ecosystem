package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// IOResource is the I/O engine resource of products (import + export).
const IOResource = "tenant.catalog.products"

const exportPage = 500

// OrgReader loads the organization of an export job.
type OrgReader interface {
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
}

// PriceReader loads the effective price views of products (pricing module).
type PriceReader interface {
	ViewsByUUIDs(ctx context.Context, v pricingusecase.Viewer, uuids []uuid.UUID) ([]pricingusecase.ProductPriceView, error)
}

// IOAdapter exports and imports the products of one brand. Export reads the
// brand of the job's organization; import writes only when the job belongs
// to the brand center (K4). Rows are matched by SKU inside the brand.
//
// TEC-211: the price columns (purchase, sale, recommended) are behind their
// pricing.* permission (ioengine.Column.Permission). The export job stores
// the grants of the requester; the adapter reads prices only for the
// granted columns (the pricing viewer mirrors the grants) and the export
// worker removes the ungranted columns from the file.
type IOAdapter struct {
	svc    *Service
	orgs   OrgReader
	prices PriceReader
}

// NewIOAdapter creates the products I/O adapter.
func NewIOAdapter(svc *Service, orgs OrgReader) *IOAdapter {
	return &IOAdapter{svc: svc, orgs: orgs}
}

// WithPrices adds the price columns source (nil leaves them empty).
func (a *IOAdapter) WithPrices(pr PriceReader) *IOAdapter {
	a.prices = pr
	return a
}

// Export column keys of the price columns.
const (
	ColumnPurchasePrice    = "purchase_price"
	ColumnSalePrice        = "sale_price"
	ColumnRecommendedPrice = "recommended_sale_price"
)

// Resource implements ioengine.ResourceAdapter.
func (a *IOAdapter) Resource() string { return IOResource }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *IOAdapter) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "uuid", LabelKey: "catalog.products.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "sku", LabelKey: "catalog.products.sku", Type: ioengine.ColumnTypeString},
		{Key: "name", LabelKey: "catalog.products.name", Type: ioengine.ColumnTypeString},
		{Key: "category", LabelKey: "catalog.products.category", Type: ioengine.ColumnTypeString},
		{Key: "unit_type", LabelKey: "catalog.products.unit", Type: ioengine.ColumnTypeEnum},
		{Key: "warranty_duration_months", LabelKey: "catalog.products.warranty_months", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "micron_thickness", LabelKey: "catalog.products.micron_thickness", Type: ioengine.ColumnTypeString, AlignRight: true},
		{Key: "uses_fixed_barcode", LabelKey: "catalog.products.uses_fixed_barcode", Type: ioengine.ColumnTypeBoolean},
		{Key: "active", LabelKey: "catalog.products.is_active", Type: ioengine.ColumnTypeBoolean},
		{Key: "description_md", LabelKey: "catalog.products.description", Type: ioengine.ColumnTypeString},
		{
			Key: ColumnPurchasePrice, LabelKey: "catalog.products.purchase_price", Type: ioengine.ColumnTypeString,
			AlignRight: true, Permission: rbac.PermPricingPurchaseRead,
		},
		{
			Key: ColumnSalePrice, LabelKey: "catalog.products.sale_price", Type: ioengine.ColumnTypeString,
			AlignRight: true, Permission: rbac.PermPricingSaleRead,
		},
		{
			Key: ColumnRecommendedPrice, LabelKey: "catalog.products.recommended_price", Type: ioengine.ColumnTypeString,
			AlignRight: true, Permission: rbac.PermPricingRecommendedRead,
		},
	}
}

// ProductListKeys are the product list parameters the export carries
// (TEC-369: the same filters, q and sort as GET /v1/catalog/products).
var ProductListKeys = []string{
	"q", "sort", "active", "category_uuid", "unit_type", "uses_fixed_barcode",
	"warranty_duration_months_min", "warranty_duration_months_max",
	"micron_thickness_min", "micron_thickness_max", "created_from", "created_to",
}

// Export implements ioengine.ResourceAdapter. Query keys: ProductListKeys.
func (a *IOAdapter) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := strconv.ParseInt(query[ioengine.QueryOrganizationID], 10, 64)
	if err != nil || orgID <= 0 {
		return ioengine.Dataset{}, errors.New("catalog export: organization is required")
	}
	org, err := a.orgs.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	scope := orgctx.Scope{InternalID: org.ID, BrandID: org.BrandID, OrgType: org.Type}
	values := url.Values{}
	for _, k := range ProductListKeys {
		if v := strings.TrimSpace(query[k]); v != "" {
			values.Set(k, v)
		}
	}
	f, err := ParseProductFilter(values)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Limit, f.Offset = exportPage, 0
	rows := []map[string]any{}
	viewer := pricingusecase.Viewer{
		OrgID: org.ID, OrgType: org.Type, BrandID: org.BrandID,
		PurchaseRead:    ioengine.Granted(query, rbac.PermPricingPurchaseRead),
		SaleRead:        ioengine.Granted(query, rbac.PermPricingSaleRead),
		RecommendedRead: ioengine.Granted(query, rbac.PermPricingRecommendedRead),
	}
	for {
		page, total, err := a.svc.ListProducts(ctx, scope, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		pageRows := make([]map[string]any, 0, len(page))
		for _, p := range page {
			pageRows = append(pageRows, exportRow(p))
		}
		if err := a.addPrices(ctx, viewer, page, pageRows); err != nil {
			return ioengine.Dataset{}, err
		}
		rows = append(rows, pageRows...)
		f.Offset += int32(len(page))
		if len(page) == 0 || int64(f.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: IOResource, Columns: a.ExportColumns(), Rows: rows}, nil
}

// addPrices fills the granted price columns of one page ("CUR amount",
// several currencies joined with "; ").
func (a *IOAdapter) addPrices(ctx context.Context, v pricingusecase.Viewer, page []model.Product, rows []map[string]any) error {
	if a.prices == nil || (!v.PurchaseRead && !v.SaleRead && !v.RecommendedRead) {
		return nil
	}
	uuids := make([]uuid.UUID, 0, len(page))
	for _, p := range page {
		uuids = append(uuids, p.UUID)
	}
	views, err := a.prices.ViewsByUUIDs(ctx, v, uuids)
	if err != nil {
		return fmt.Errorf("catalog export: prices: %w", err)
	}
	byUUID := make(map[uuid.UUID]pricingusecase.ProductPriceView, len(views))
	for _, pv := range views {
		byUUID[pv.ProductUUID] = pv
	}
	for i, p := range page {
		pv := byUUID[p.UUID]
		if v.PurchaseRead {
			rows[i][ColumnPurchasePrice] = joinPrices(pv.Prices, func(e pricingusecase.EffectivePrice) *string { return e.PurchasePrice })
		}
		if v.SaleRead {
			rows[i][ColumnSalePrice] = joinPrices(pv.Prices, func(e pricingusecase.EffectivePrice) *string { return e.SalePrice })
		}
		if v.RecommendedRead {
			rows[i][ColumnRecommendedPrice] = joinPrices(pv.Prices, func(e pricingusecase.EffectivePrice) *string { return e.RecommendedSalePrice })
		}
	}
	return nil
}

func joinPrices(prices []pricingusecase.EffectivePrice, pick func(pricingusecase.EffectivePrice) *string) string {
	parts := make([]string, 0, len(prices))
	for _, e := range prices {
		if v := pick(e); v != nil && *v != "" {
			parts = append(parts, e.Currency+" "+*v)
		}
	}
	return strings.Join(parts, "; ")
}

func exportRow(p model.Product) map[string]any {
	warranty, micron := "", ""
	if p.WarrantyDurationMonths != nil {
		warranty = strconv.Itoa(int(*p.WarrantyDurationMonths))
	}
	if p.MicronThickness != nil {
		micron = strconv.FormatFloat(*p.MicronThickness, 'f', -1, 64)
	}
	return map[string]any{
		"uuid": p.UUID.String(), "sku": p.SKU, "name": p.Name, "category": p.Category.Name,
		"unit_type": p.UnitType, "warranty_duration_months": warranty, "micron_thickness": micron,
		"uses_fixed_barcode": p.UsesFixedBarcode, "active": p.Active, "description_md": p.DescriptionMD,
	}
}

// ImportSchema implements ioengine.ResourceAdapter.
func (a *IOAdapter) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "sku", LabelKey: "catalog.products.sku", Type: ioengine.ColumnTypeString, Required: true,
			Kind: ioengine.FieldCode, HintKey: "catalog.products.import_hint.sku"},
		{Key: "name", LabelKey: "catalog.products.name", Type: ioengine.ColumnTypeString, Required: true,
			HintKey: "catalog.products.import_hint.name"},
		{Key: "category", LabelKey: "catalog.products.category", Type: ioengine.ColumnTypeString, Required: true,
			HintKey: "catalog.products.import_hint.category"},
		{Key: "unit_type", LabelKey: "catalog.products.unit", Type: ioengine.ColumnTypeEnum, DefaultHint: model.UnitPiece,
			Enum: []string{model.UnitPiece, model.UnitRollMeter}, HintKey: "catalog.products.import_hint.unit_type"},
		{Key: "warranty_duration_months", LabelKey: "catalog.products.warranty_months", Type: ioengine.ColumnTypeString,
			Kind: ioengine.FieldInteger, HintKey: "catalog.products.import_hint.warranty_duration_months"},
		{Key: "micron_thickness", LabelKey: "catalog.products.micron_thickness", Type: ioengine.ColumnTypeString,
			Kind: ioengine.FieldDecimal, HintKey: "catalog.products.import_hint.micron_thickness"},
		{Key: "uses_fixed_barcode", LabelKey: "catalog.products.uses_fixed_barcode", Type: ioengine.ColumnTypeBoolean, DefaultHint: "false",
			HintKey: "catalog.products.import_hint.uses_fixed_barcode"},
		{Key: "active", LabelKey: "catalog.products.is_active", Type: ioengine.ColumnTypeBoolean, DefaultHint: "true"},
		{Key: "description_md", LabelKey: "catalog.products.description", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows is the template example (TEC-757).
func (a *IOAdapter) SampleRows() []map[string]any {
	return []map[string]any{
		{"sku": "PPF-GLOSS-152", "name": "PPF Gloss 152 cm", "category": "PPF", "unit_type": model.UnitRollMeter,
			"warranty_duration_months": "120", "micron_thickness": "190", "uses_fixed_barcode": "false", "active": "true"},
		{"sku": "CLEANER-500", "name": "Cleaner 500 ml", "category": "Care", "unit_type": model.UnitPiece,
			"warranty_duration_months": "", "micron_thickness": "", "uses_fixed_barcode": "true", "active": "true"},
	}
}

// SampleLookups implements ioengine.SampleLookuper: the brand's active
// categories.
func (a *IOAdapter) SampleLookups(ctx context.Context) (map[string][]string, error) {
	org, ok := orgctx.ScopeFrom(ctx)
	if !ok || org.BrandID == 0 {
		return nil, nil
	}
	names, err := a.svc.store.ListActiveProductCategoryNames(ctx, org.BrandID)
	if err != nil {
		return nil, fmt.Errorf("catalog import: category lookup: %w", err)
	}
	return map[string][]string{"category": names}, nil
}

// ImportPermission implements ioengine.ImportPermissioner.
func (a *IOAdapter) ImportPermission() string { return rbac.PermCatalogWrite }

// ValidateRow implements ioengine.RowValidator (TEC-757): the preview
// reports the rows ApplyRow would refuse (lengths, ranges, unknown
// category) before anything is written. Required cells, numbers, booleans
// and the unit type are checked by the schema.
func (a *IOAdapter) ValidateRow(ctx context.Context, row, defaults map[string]any, loc i18n.Locale) ([]ioengine.RowError, error) {
	fields := map[string]ioengine.ImportField{}
	for _, f := range a.ImportSchema() {
		fields[f.Key] = f
	}
	get := func(k string) string { return ioengine.Cell(row, defaults, k) }
	var errs []ioengine.RowError
	errs = append(errs, ioengine.CheckLength(loc, fields["sku"], get("sku"), maxSKULen)...)
	errs = append(errs, ioengine.CheckLength(loc, fields["name"], get("name"), maxNameLen)...)
	errs = append(errs, ioengine.CheckLength(loc, fields["description_md"], get("description_md"), maxDescriptionLen)...)
	if v := get("warranty_duration_months"); v != "" {
		if n, ok := ioengine.ParseInteger(v); ok && (n < 0 || n > maxWarrantyMonths) {
			errs = append(errs, ioengine.NewRowError(loc, fields["warranty_duration_months"], ioengine.CodeOutOfRange,
				map[string]string{"min": "0", "max": strconv.Itoa(maxWarrantyMonths)}))
		}
	}
	if v := get("micron_thickness"); v != "" {
		if n, ok := ioengine.ParseDecimal(v); ok && (n <= 0 || n > maxMicron) {
			errs = append(errs, ioengine.NewRowError(loc, fields["micron_thickness"], ioengine.CodeOutOfRange,
				map[string]string{"min": "0", "max": strconv.FormatFloat(maxMicron, 'f', -1, 64)}))
		}
	}
	if name := get("category"); name != "" {
		org, ok := orgctx.ScopeFrom(ctx)
		if !ok {
			return errs, nil
		}
		_, err := a.svc.store.GetProductCategoryByName(ctx, db.GetProductCategoryByNameParams{BrandID: org.BrandID, Name: name})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			errs = append(errs, ioengine.NewRowError(loc, fields["category"], ioengine.CodeNotFound, map[string]string{"value": name}))
		case err != nil:
			return nil, err
		}
	}
	return errs, nil
}

// ApplyRow implements ioengine.ResourceAdapter: creates the product, or
// updates the one with the same SKU in the brand. The category is matched by
// name and must exist.
func (a *IOAdapter) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	org, ok := orgctx.ScopeFrom(ctx)
	if !ok || requireCenter(org) != nil {
		return ioengine.RowResult{OK: false, Error: ErrCenterOnly.Error()}, nil
	}
	get := func(key string) string {
		if v := cell(row, key); v != "" {
			return v
		}
		return cell(defaults, key)
	}
	sku := get("sku")
	if sku == "" || get("name") == "" || get("category") == "" {
		return ioengine.RowResult{OK: false, Error: "sku, name, category required"}, nil
	}
	cat, err := a.svc.store.GetProductCategoryByName(ctx, db.GetProductCategoryByNameParams{BrandID: org.BrandID, Name: get("category")})
	if errors.Is(err, pgx.ErrNoRows) {
		return ioengine.RowResult{OK: false, Error: "unknown category: " + get("category")}, nil
	}
	if err != nil {
		return ioengine.RowResult{}, err
	}
	in, perr := importInput(get, cat.Uuid)
	if perr != "" {
		return ioengine.RowResult{OK: false, Error: perr}, nil
	}
	existing, err := a.svc.store.GetProductBySKU(ctx, db.GetProductBySKUParams{BrandID: org.BrandID, Sku: strings.TrimSpace(sku)})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		created, err := a.svc.CreateProduct(ctx, org, in)
		if err != nil {
			return rowError(err)
		}
		return ioengine.RowResult{OK: true, EntityType: "product", EntityUUID: created.UUID.String(), Op: "create"}, nil
	case err != nil:
		return ioengine.RowResult{}, err
	}
	previous := snapshot(existing)
	updated, err := a.svc.UpdateProduct(ctx, org, existing.Uuid, in)
	if err != nil {
		return rowError(err)
	}
	return ioengine.RowResult{
		OK: true, EntityType: "product", EntityUUID: updated.UUID.String(), Op: "update", Previous: previous,
	}, nil
}

// RevertRow implements ioengine.ResourceAdapter: a created product is
// deleted, an updated one gets its previous fields back.
func (a *IOAdapter) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "product" {
		return fmt.Errorf("unsupported entity")
	}
	org, ok := orgctx.ScopeFrom(ctx)
	if !ok || requireCenter(org) != nil {
		return ErrCenterOnly
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	if len(previous) == 0 {
		err := a.svc.DeleteProduct(ctx, org, id)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	raw, _ := json.Marshal(previous["fields"])
	var p productFields
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	catID, _ := previous["category_id"].(float64)
	cur, err := a.svc.product(ctx, org.BrandID, id)
	if err != nil {
		return err
	}
	if catID == 0 {
		catID = float64(cur.CategoryID)
	}
	if _, err := a.svc.store.UpdateProduct(ctx, p.updateParams(cur.ID, org.BrandID, int64(catID))); err != nil {
		return mapDBError(err)
	}
	a.svc.reindex(ctx, id)
	return nil
}

// snapshot is the RowResult.Previous of an update (JSON round-trips).
func snapshot(row db.Product) map[string]any {
	raw, _ := json.Marshal(fieldsOf(row))
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	return map[string]any{"category_id": float64(row.CategoryID), "fields": fields}
}

func importInput(get func(string) string, categoryUUID uuid.UUID) (model.ProductInput, string) {
	str := func(k string) *string { v := get(k); return &v }
	in := model.ProductInput{
		CategoryUUID: &categoryUUID, SKU: str("sku"), Name: str("name"),
		WarrantySet: true, MicronSet: true,
	}
	if v := get("description_md"); v != "" {
		in.DescriptionMD = &v
	}
	if v := get("unit_type"); v != "" {
		in.UnitType = &v
	}
	if v := get("warranty_duration_months"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return in, "invalid warranty_duration_months: " + v
		}
		m := int32(n)
		in.WarrantyDurationMonths = &m
	}
	if v := get("micron_thickness"); v != "" {
		f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
		if err != nil {
			return in, "invalid micron_thickness: " + v
		}
		in.MicronThickness = &f
	}
	for key, dst := range map[string]**bool{"uses_fixed_barcode": &in.UsesFixedBarcode, "active": &in.Active} {
		v := get(key)
		if v == "" {
			continue
		}
		b, ok := parseBool(v)
		if !ok {
			return in, "invalid " + key + ": " + v
		}
		*dst = &b
	}
	return in, ""
}

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "evet", "aktif", "active", "x":
		return true, true
	case "0", "false", "no", "hayır", "hayir", "pasif", "inactive":
		return false, true
	}
	return false, false
}

func cell(row map[string]any, key string) string {
	if row == nil {
		return ""
	}
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// rowError turns a validation/conflict error into a failed row; other
// errors abort the import.
func rowError(err error) (ioengine.RowResult, error) {
	var verr *ValidationError
	var conflict *ConflictError
	var locked *LockedError
	switch {
	case errors.As(err, &verr), errors.As(err, &conflict), errors.As(err, &locked), errors.Is(err, ErrCenterOnly):
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	return ioengine.RowResult{}, err
}

var (
	_ ioengine.ResourceAdapter = (*IOAdapter)(nil)
	_ ioengine.RowValidator    = (*IOAdapter)(nil)
	_ ioengine.SampleLookuper  = (*IOAdapter)(nil)
)
