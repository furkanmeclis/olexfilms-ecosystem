package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var ExpectationSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"part_key": "part_key", "product": "product", "category": "category", "body_type": "body_type",
		"expected_meters": "expected_meters", "source": "source", "sample_size": "sample_size", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "updated_at", Desc: true},
}

type ExpectationFilter struct {
	Limit       int32
	Offset      int32
	Q           string
	Source      string
	BodyType    string
	ProductIDs  []int64
	CategoryIDs []int64
	PartKeys    []string
	Sort        []apiquery.SortField
}

type ExpectationInput struct {
	ProductUUID    *uuid.UUID `json:"product_uuid"`
	CategoryUUID   *uuid.UUID `json:"category_uuid"`
	BodyType       *string    `json:"body_type"`
	PartKey        string     `json:"part_key"`
	ExpectedMeters string     `json:"expected_meters"`
}

type ExpectationRow struct {
	UUID           uuid.UUID  `json:"uuid"`
	ProductUUID    *uuid.UUID `json:"product_uuid"`
	CategoryUUID   *uuid.UUID `json:"category_uuid"`
	ProductName    *string    `json:"product_name"`
	CategoryName   *string    `json:"category_name"`
	BodyType       *string    `json:"body_type"`
	PartKey        string     `json:"part_key"`
	ExpectedMeters string     `json:"expected_meters"`
	Source         string     `json:"source"`
	SampleSize     int32      `json:"sample_size"`
}

func (s *Service) ListExpectations(ctx context.Context, c Caller, f ExpectationFilter) ([]ExpectationRow, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, ExpectationSort)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListPartConsumptionExpectations(ctx, db.ListPartConsumptionExpectationsParams{
		BrandID: c.Org.BrandID, Q: text(f.Q), ProductIds: f.ProductIDs, CategoryIds: f.CategoryIDs,
		PartKeys: f.PartKeys, Source: text(f.Source), BodyType: text(f.BodyType),
		SortKey: sort.Key, SortDesc: sort.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("efficiency expectations: %w", err)
	}
	out := make([]ExpectationRow, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.TotalCount
		out = append(out, expectationView(expectationFromList(r), r.ProductName, r.ProductUuid, r.CategoryName, r.CategoryUuid))
	}
	return out, total, nil
}

func (s *Service) CreateExpectation(ctx context.Context, c Caller, in ExpectationInput) (ExpectationRow, error) {
	arg, err := s.expectationParams(ctx, c, in)
	if err != nil {
		return ExpectationRow{}, err
	}
	row, err := s.q.CreatePartConsumptionExpectation(ctx, arg)
	if err != nil {
		return ExpectationRow{}, fmt.Errorf("efficiency create expectation: %w", err)
	}
	return expectationView(row, pgtype.Text{}, pgtype.UUID{}, pgtype.Text{}, pgtype.UUID{}), nil
}

func (s *Service) UpdateExpectation(ctx context.Context, c Caller, id uuid.UUID, in ExpectationInput) (ExpectationRow, error) {
	cur, err := s.getExpectation(ctx, c, id)
	if err != nil {
		return ExpectationRow{}, err
	}
	meters, err := scanPositive(in.ExpectedMeters)
	if err != nil {
		return ExpectationRow{}, invalid("expected_meters", "must be a positive decimal")
	}
	body := pgtype.Text{}
	if in.BodyType != nil {
		body = text(*in.BodyType)
	}
	row, err := s.q.UpdatePartConsumptionExpectation(ctx, db.UpdatePartConsumptionExpectationParams{
		ID: cur.ID, BrandID: c.Org.BrandID, BodyType: body, ExpectedMeters: meters, Source: "manual", SampleSize: 1,
	})
	if err != nil {
		return ExpectationRow{}, fmt.Errorf("efficiency update expectation: %w", err)
	}
	return expectationView(row, pgtype.Text{}, pgtype.UUID{}, pgtype.Text{}, pgtype.UUID{}), nil
}

func (s *Service) DeleteExpectation(ctx context.Context, c Caller, id uuid.UUID) error {
	cur, err := s.getExpectation(ctx, c, id)
	if err != nil {
		return err
	}
	n, err := s.q.DeletePartConsumptionExpectation(ctx, db.DeletePartConsumptionExpectationParams{ID: cur.ID, BrandID: c.Org.BrandID})
	if err != nil {
		return fmt.Errorf("efficiency delete expectation: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) expectationParams(ctx context.Context, c Caller, in ExpectationInput) (db.CreatePartConsumptionExpectationParams, error) {
	if (in.ProductUUID == nil) == (in.CategoryUUID == nil) {
		return db.CreatePartConsumptionExpectationParams{}, invalid("product_uuid", "exactly one of product_uuid or category_uuid is required")
	}
	part := strings.TrimSpace(in.PartKey)
	if part == "" {
		return db.CreatePartConsumptionExpectationParams{}, invalid("part_key", "is required")
	}
	meters, err := scanPositive(in.ExpectedMeters)
	if err != nil {
		return db.CreatePartConsumptionExpectationParams{}, invalid("expected_meters", "must be a positive decimal")
	}
	arg := db.CreatePartConsumptionExpectationParams{
		OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID, PartKey: part,
		ExpectedMeters: meters, Source: "manual", SampleSize: 1,
	}
	if in.BodyType != nil {
		arg.BodyType = text(*in.BodyType)
	}
	if in.ProductUUID != nil {
		p, err := s.q.GetProductByUUID(ctx, db.GetProductByUUIDParams{Uuid: *in.ProductUUID, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.CreatePartConsumptionExpectationParams{}, ErrNotFound
		}
		if err != nil {
			return db.CreatePartConsumptionExpectationParams{}, err
		}
		arg.ProductID = pgtype.Int8{Int64: p.ID, Valid: true}
	} else {
		cat, err := s.q.GetProductCategoryByUUID(ctx, db.GetProductCategoryByUUIDParams{Uuid: *in.CategoryUUID, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.CreatePartConsumptionExpectationParams{}, ErrNotFound
		}
		if err != nil {
			return db.CreatePartConsumptionExpectationParams{}, err
		}
		arg.CategoryID = pgtype.Int8{Int64: cat.ID, Valid: true}
	}
	return arg, nil
}

func (s *Service) getExpectation(ctx context.Context, c Caller, id uuid.UUID) (db.PartConsumptionExpectation, error) {
	row, err := s.q.GetPartConsumptionExpectationByUUID(ctx, db.GetPartConsumptionExpectationByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PartConsumptionExpectation{}, ErrNotFound
	}
	return row, err
}

func expectationView(e db.PartConsumptionExpectation, productName pgtype.Text, productUUID pgtype.UUID, categoryName pgtype.Text, categoryUUID pgtype.UUID) ExpectationRow {
	var product, category *uuid.UUID
	if productUUID.Valid {
		v := uuid.UUID(productUUID.Bytes)
		product = &v
	}
	if categoryUUID.Valid {
		v := uuid.UUID(categoryUUID.Bytes)
		category = &v
	}
	return ExpectationRow{
		UUID: e.Uuid, ProductUUID: product, CategoryUUID: category,
		ProductName: textPtr(productName), CategoryName: textPtr(categoryName), BodyType: textPtr(e.BodyType),
		PartKey: e.PartKey, ExpectedMeters: numeric(e.ExpectedMeters), Source: e.Source, SampleSize: e.SampleSize,
	}
}

func expectationFromList(r db.ListPartConsumptionExpectationsRow) db.PartConsumptionExpectation {
	return db.PartConsumptionExpectation{
		ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, BrandID: r.BrandID,
		ProductID: r.ProductID, CategoryID: r.CategoryID, BodyType: r.BodyType,
		PartKey: r.PartKey, ExpectedMeters: r.ExpectedMeters, Source: r.Source,
		SampleSize: r.SampleSize, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func scanPositive(raw string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(strings.TrimSpace(raw)); err != nil {
		return n, err
	}
	if !n.Valid {
		return n, errors.New("empty numeric")
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid || f.Float64 <= 0 {
		return n, errors.New("not positive")
	}
	return n, nil
}

type ExpectationsImportAdapter struct{ svc *Service }

func NewExpectationsImportAdapter(s *Service) *ExpectationsImportAdapter {
	return &ExpectationsImportAdapter{svc: s}
}

func (a *ExpectationsImportAdapter) Resource() string { return ResourceExpectations }

func (a *ExpectationsImportAdapter) ExportColumns() []ioengine.Column { return nil }

func (a *ExpectationsImportAdapter) Export(context.Context, ioengine.ExportQuery, i18n.Locale) (ioengine.Dataset, error) {
	return ioengine.Dataset{}, errors.New("expectations import is import only")
}

func (a *ExpectationsImportAdapter) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "uuid", LabelKey: "catalog.products.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "product_sku", LabelKey: "stock_import.product_sku", Type: ioengine.ColumnTypeString},
		{Key: "category", LabelKey: "catalog.products.category", Type: ioengine.ColumnTypeString},
		{Key: "body_type", LabelKey: "measurements.pdf.body_type", Type: ioengine.ColumnTypeString},
		{Key: "part_key", LabelKey: "warranty_claims.reports.part_key", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "expected_meters", LabelKey: "warehouse.eod.meters_out", Type: ioengine.ColumnTypeString, Required: true},
	}
}

func (a *ExpectationsImportAdapter) SampleRows() []map[string]any {
	return []map[string]any{
		{"product_sku": "PPF-GLOSS", "category": "", "body_type": "sedan", "part_key": "hood", "expected_meters": "2.40"},
		{"product_sku": "", "category": "PPF", "body_type": "", "part_key": "roof", "expected_meters": "3.10"},
	}
}

func (a *ExpectationsImportAdapter) ApplyRow(ctx context.Context, row map[string]any, _ map[string]any) (ioengine.RowResult, error) {
	org, ok := orgctx.ScopeFrom(ctx)
	if !ok {
		return ioengine.RowResult{OK: false, Error: "organization is required"}, nil
	}
	c := Caller{Org: org}
	idRaw := importCell(row, "uuid")
	in, perr := a.importInput(ctx, c, row)
	if perr != "" {
		return ioengine.RowResult{OK: false, Error: perr}, nil
	}
	if idRaw == "" {
		created, err := a.svc.CreateExpectation(ctx, c, in)
		if err != nil {
			return expectationRowError(err)
		}
		return ioengine.RowResult{OK: true, EntityType: ResourceExpectations, EntityUUID: created.UUID.String(), Op: "create"}, nil
	}
	id, err := uuid.Parse(idRaw)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: "uuid is invalid"}, nil
	}
	cur, err := a.svc.getExpectation(ctx, c, id)
	if errors.Is(err, ErrNotFound) {
		return ioengine.RowResult{OK: false, Error: "expectation not found"}, nil
	}
	if err != nil {
		return ioengine.RowResult{}, err
	}
	updated, err := a.svc.UpdateExpectation(ctx, c, id, in)
	if err != nil {
		return expectationRowError(err)
	}
	return ioengine.RowResult{
		OK: true, EntityType: ResourceExpectations, EntityUUID: updated.UUID.String(), Op: "update",
		Previous: map[string]any{"body_type": textPtr(cur.BodyType), "expected_meters": numeric(cur.ExpectedMeters)},
	}, nil
}

func (a *ExpectationsImportAdapter) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != ResourceExpectations {
		return fmt.Errorf("unsupported entity")
	}
	org, ok := orgctx.ScopeFrom(ctx)
	if !ok {
		return ErrForbidden
	}
	c := Caller{Org: org}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	if len(previous) == 0 {
		err := a.svc.DeleteExpectation(ctx, c, id)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	body, _ := previous["body_type"].(string)
	meters, _ := previous["expected_meters"].(string)
	in := ExpectationInput{BodyType: &body, ExpectedMeters: meters}
	_, err = a.svc.UpdateExpectation(ctx, c, id, in)
	return err
}

func (a *ExpectationsImportAdapter) importInput(ctx context.Context, c Caller, row map[string]any) (ExpectationInput, string) {
	part := importCell(row, "part_key")
	meters := importCell(row, "expected_meters")
	body := importCell(row, "body_type")
	productSKU := importCell(row, "product_sku")
	categoryName := importCell(row, "category")
	if part == "" || meters == "" {
		return ExpectationInput{}, "part_key and expected_meters are required"
	}
	if (productSKU == "") == (categoryName == "") {
		return ExpectationInput{}, "exactly one of product_sku or category is required"
	}
	in := ExpectationInput{PartKey: part, ExpectedMeters: meters}
	if body != "" {
		in.BodyType = &body
	}
	if productSKU != "" {
		p, err := a.svc.q.GetProductBySKU(ctx, db.GetProductBySKUParams{BrandID: c.Org.BrandID, Sku: productSKU})
		if errors.Is(err, pgx.ErrNoRows) {
			return ExpectationInput{}, "unknown product_sku: " + productSKU
		}
		if err != nil {
			return ExpectationInput{}, err.Error()
		}
		in.ProductUUID = &p.Uuid
		return in, ""
	}
	cat, err := a.svc.q.GetProductCategoryByName(ctx, db.GetProductCategoryByNameParams{BrandID: c.Org.BrandID, Name: categoryName})
	if errors.Is(err, pgx.ErrNoRows) {
		return ExpectationInput{}, "unknown category: " + categoryName
	}
	if err != nil {
		return ExpectationInput{}, err.Error()
	}
	in.CategoryUUID = &cat.Uuid
	return in, ""
}

func expectationRowError(err error) (ioengine.RowResult, error) {
	var ve *ValidationError
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &ve):
		return ioengine.RowResult{OK: false, Error: ve.Error()}, nil
	case errors.Is(err, ErrNotFound):
		return ioengine.RowResult{OK: false, Error: "referenced record not found"}, nil
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return ioengine.RowResult{OK: false, Error: "expectation already exists"}, nil
	default:
		return ioengine.RowResult{}, err
	}
}

func importCell(row map[string]any, key string) string {
	if row == nil || row[key] == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(row[key]))
}
