package usecase

// TEC-1229: ledger movement list of the active organization (and of one of
// its warehouses) with type, product, warehouse, actor and date filters,
// plus its export. Before this only the barcode history listed movements.
// The list never carries a price.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ResourceMovementsExport is the movement list export (export_jobs.resource).
const ResourceMovementsExport = "stock.movements"

// MovementTypes are the stock_movements.type values the list filters on.
var MovementTypes = []string{
	"entry", "placement", "transfer_out", "transfer_in", "transfer_cancel_restore",
	"order_out", "received", "order_cancel_restore", "consumption", "partial_consumption",
	"return", "sale", "reclassification", "count_adjustment", "void", "external_outbound", "split",
	"measure_conversion", "quarantine", "quarantine_release", "found",
}

// MovementSort is the sort contract of the movement list.
var MovementSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "type": "type", "product": "product"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// MovementFilter filters the movement list.
type MovementFilter struct {
	Types         []string
	ProductUUID   *uuid.UUID
	WarehouseUUID *uuid.UUID
	ActorUUID     *uuid.UUID
	CreatedFrom   *time.Time
	CreatedBefore *time.Time
	Q             string
	Sort          apiquery.ResolvedSort
	Limit         int32
	Offset        int32
}

// MovementRef names a product, unit, location, warehouse, organization or
// actor of a movement row.
type MovementRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// MovementUnit is the unit of a movement row.
type MovementUnit struct {
	UUID     uuid.UUID `json:"uuid"`
	Barcode  string    `json:"barcode"`
	UnitKind string    `json:"unit_kind"`
}

// MovementProduct is the product of a movement row.
type MovementProduct struct {
	UUID     uuid.UUID `json:"uuid"`
	SKU      string    `json:"sku"`
	Name     string    `json:"name"`
	UnitType string    `json:"unit_type"`
}

// MovementEnd is one side (from / to) of a movement row.
type MovementEnd struct {
	OwnerType    string       `json:"owner_type"`
	Status       *string      `json:"status"`
	Location     *MovementRef `json:"location"`
	Warehouse    *MovementRef `json:"warehouse"`
	Organization *MovementRef `json:"organization"`
}

// MovementRow is one row of GET /v1/stock/movements.
type MovementRow struct {
	UUID          uuid.UUID       `json:"uuid"`
	Type          string          `json:"type"`
	QuantityDelta int32           `json:"quantity_delta"`
	MetersDelta   string          `json:"meters_delta"`
	Unit          MovementUnit    `json:"unit"`
	Product       MovementProduct `json:"product"`
	From          *MovementEnd    `json:"from"`
	To            *MovementEnd    `json:"to"`
	Actor         *MovementRef    `json:"actor"`
	ReferenceType *string         `json:"reference_type"`
	Reason        *string         `json:"reason"`
	CreatedAt     time.Time       `json:"created_at"`
}

func movementInvalid(field, msg string) error {
	return &apiquery.ValidationError{Details: []apiquery.Detail{{Field: field, Message: msg, Code: "invalid"}}}
}

// ParseMovementFilter reads type (CSV), product_uuid, warehouse_uuid,
// actor_uuid, created_from / created_to, q, sort, limit and offset.
func ParseMovementFilter(values url.Values) (MovementFilter, error) {
	q := apiquery.Parse(values)
	f := MovementFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	types, err := apiquery.EnumList(values, "type", MovementTypes...)
	if err != nil {
		return f, err
	}
	f.Types = types
	for name, dst := range map[string]**uuid.UUID{
		"product_uuid": &f.ProductUUID, "warehouse_uuid": &f.WarehouseUUID, "actor_uuid": &f.ActorUUID,
	} {
		raw := strings.TrimSpace(values.Get(name))
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, movementInvalid(name, "is invalid")
		}
		*dst = &id
	}
	rng, err := apiquery.DateRange(values, "created")
	if err != nil {
		return f, err
	}
	f.CreatedFrom, f.CreatedBefore = rng.From, rng.Before
	f.Sort, err = apiquery.ResolveSort(q.Sort, MovementSort)
	return f, err
}

// Movements lists the ledger movements of organization orgID (the active
// organization; the stock.read filter must reach it).
func (s *Service) Movements(ctx context.Context, f scopefilter.Filter, orgID, brandID int64, in MovementFilter) ([]MovementRow, int64, error) {
	if !f.AllowsOrg(orgID, brandID) || f.UserOnly() {
		return nil, 0, ErrNotFound
	}
	var warehouse, product, actor pgtype.Int8
	if in.WarehouseUUID != nil {
		wh, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: *in.WarehouseUUID, OrganizationID: orgID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrNotFound
		}
		if err != nil {
			return nil, 0, fmt.Errorf("stock: movements warehouse: %w", err)
		}
		warehouse = pgtype.Int8{Int64: wh.ID, Valid: true}
	}
	if in.ProductUUID != nil {
		id, err := s.q.GetProductIDByUUID(ctx, *in.ProductUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return []MovementRow{}, 0, nil
		}
		if err != nil {
			return nil, 0, fmt.Errorf("stock: movements product: %w", err)
		}
		product = pgtype.Int8{Int64: id, Valid: true}
	}
	if in.ActorUUID != nil {
		u, err := s.q.GetUserByUUID(ctx, *in.ActorUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return []MovementRow{}, 0, nil
		}
		if err != nil {
			return nil, 0, fmt.Errorf("stock: movements actor: %w", err)
		}
		actor = pgtype.Int8{Int64: u.ID, Valid: true}
	}
	key, desc := resolvedSort(in.Sort, MovementSort)
	total, err := s.q.CountStockMovementRows(ctx, db.CountStockMovementRowsParams{
		OrganizationID: orgID, WarehouseID: warehouse, Types: in.Types, ProductID: product, ActorUserID: actor,
		CreatedFrom: tsArg(in.CreatedFrom), CreatedTo: tsArg(in.CreatedBefore), Q: textArg(in.Q),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: count movements: %w", err)
	}
	rows, err := s.q.ListStockMovementRows(ctx, db.ListStockMovementRowsParams{
		OrganizationID: orgID, WarehouseID: warehouse, Types: in.Types, ProductID: product, ActorUserID: actor,
		CreatedFrom: tsArg(in.CreatedFrom), CreatedTo: tsArg(in.CreatedBefore), Q: textArg(in.Q),
		SortKey: key, SortDesc: desc, LimitCount: in.Limit, OffsetCount: in.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: movements: %w", err)
	}
	out := make([]MovementRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, movementRowOf(r))
	}
	return out, total, nil
}

func ref(id pgtype.UUID, name pgtype.Text) *MovementRef {
	if !id.Valid {
		return nil
	}
	return &MovementRef{UUID: uuid.UUID(id.Bytes), Name: name.String}
}

func movementEnd(ownerType, status pgtype.Text, loc pgtype.UUID, locCode pgtype.Text,
	wh pgtype.UUID, whName pgtype.Text, org pgtype.UUID, orgName pgtype.Text) *MovementEnd {
	if !ownerType.Valid {
		return nil
	}
	return &MovementEnd{
		OwnerType: ownerType.String, Status: textPtr(status),
		Location: ref(loc, locCode), Warehouse: ref(wh, whName), Organization: ref(org, orgName),
	}
}

func movementRowOf(r db.ListStockMovementRowsRow) MovementRow {
	row := MovementRow{
		UUID: r.Uuid, Type: r.Type, QuantityDelta: r.QuantityDelta, MetersDelta: r.MetersDelta,
		Unit:          MovementUnit{UUID: r.UnitUuid, Barcode: r.Barcode, UnitKind: r.UnitKind},
		Product:       MovementProduct{UUID: r.ProductUuid, SKU: r.Sku, Name: r.ProductName, UnitType: r.UnitType},
		ReferenceType: textPtr(r.ReferenceType), Reason: textPtr(r.Reason), CreatedAt: r.CreatedAt.Time,
		From: movementEnd(r.FromOwnerType, r.FromStatus, r.FromLocationUuid, r.FromLocationCode,
			r.FromWarehouseUuid, r.FromWarehouseName, r.FromOrgUuid, r.FromOrgName),
		To: movementEnd(r.ToOwnerType, r.ToStatus, r.ToLocationUuid, r.ToLocationCode,
			r.ToWarehouseUuid, r.ToWarehouseName, r.ToOrgUuid, r.ToOrgName),
	}
	if r.ActorUuid.Valid {
		row.Actor = &MovementRef{UUID: uuid.UUID(r.ActorUuid.Bytes), Name: r.ActorName}
	}
	return row
}

// MovementsExportQuery builds the job query of the movement list export
// (same filters as the list; the job runs against the job organization).
func (x *StockExporter) MovementsExportQuery(ctx context.Context, v UnitViewer, body map[string]string) (ioengine.ExportQuery, error) {
	values := stockValues(body)
	in, err := ParseMovementFilter(values)
	if err != nil {
		return nil, err
	}
	in.Limit = 1
	if _, _, err := x.svc.Movements(ctx, v.Filter, v.Org.InternalID, v.Org.BrandID, in); err != nil {
		return nil, err
	}
	out, err := scopeQuery(v.Filter)
	if err != nil {
		return nil, err
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, nil
}

var movementColumns = []ioengine.Column{
	{Key: "created_at", LabelKey: "stock.export.date", Type: ioengine.ColumnTypeDatetime},
	{Key: "type", LabelKey: "stock.export.movement_type", Type: ioengine.ColumnTypeString, Weight: 1.1},
	{Key: "barcode", LabelKey: "stock.export.barcode", Type: ioengine.ColumnTypeString, Weight: 1.1},
	{Key: "product", LabelKey: "stock.export.product", Type: ioengine.ColumnTypeString, Weight: 1.4},
	{Key: "sku", LabelKey: "stock.export.sku", Type: ioengine.ColumnTypeString},
	{Key: "quantity_delta", LabelKey: "stock.export.quantity_delta", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "meters_delta", LabelKey: "stock.export.meters_delta", Type: ioengine.ColumnTypeString, AlignRight: true, Weight: 0.6},
	{Key: "from_owner", LabelKey: "stock.export.from_owner", Type: ioengine.ColumnTypeString, Weight: 1.3},
	{Key: "to_owner", LabelKey: "stock.export.to_owner", Type: ioengine.ColumnTypeString, Weight: 1.3},
	{Key: "actor", LabelKey: "stock.export.actor", Type: ioengine.ColumnTypeString},
	{Key: "reason", LabelKey: "stock.export.reason", Type: ioengine.ColumnTypeString},
}

func endLabel(e *MovementEnd, loc i18n.Locale) string {
	if e == nil {
		return ""
	}
	var parts []string
	if e.OwnerType == ownerService || e.OwnerType == ownerTrash {
		parts = append(parts, translated(loc, "stock.export.owner.", e.OwnerType))
	}
	if e.Organization != nil {
		parts = append(parts, e.Organization.Name)
	}
	if e.Warehouse != nil {
		parts = append(parts, e.Warehouse.Name)
	}
	if e.Location != nil {
		parts = append(parts, e.Location.Name)
	}
	return strings.Join(parts, " · ")
}

func (x *StockExporter) movements(ctx context.Context, v UnitViewer, q ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	in, err := ParseMovementFilter(stockValues(q))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	in.Limit, in.Offset = stockExportPage, 0
	var rows []map[string]any
	for {
		page, total, err := x.svc.Movements(ctx, v.Filter, v.Org.InternalID, v.Org.BrandID, in)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, m := range page {
			actor, reason := "", ""
			if m.Actor != nil {
				actor = m.Actor.Name
			}
			if m.Reason != nil {
				reason = *m.Reason
			}
			rows = append(rows, map[string]any{
				"created_at": m.CreatedAt.UTC(), "type": translated(loc, "warehouse.eod.type.", m.Type),
				"barcode": m.Unit.Barcode, "product": m.Product.Name, "sku": m.Product.SKU,
				"quantity_delta": strconv.Itoa(int(m.QuantityDelta)), "meters_delta": m.MetersDelta,
				"from_owner": endLabel(m.From, loc), "to_owner": endLabel(m.To, loc),
				"actor": actor, "reason": reason,
			})
		}
		in.Offset += int32(len(page)) //nolint:gosec // bounded by stockExportPage
		if len(page) == 0 || int64(in.Offset) >= total {
			break
		}
	}
	return ioengine.Dataset{Rows: rows, Info: []ioengine.InfoLine{{LabelKey: "stock.export.organization", Value: v.Org.Name}}}, nil
}
