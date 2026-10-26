package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// Trace returns the warehouse-layer trace of u (TEC-1006): origin
// warehouse, source class, the orders that carried it and the services
// that used it, limited to the organizations f reaches.
func (s *Service) Trace(ctx context.Context, f scopefilter.Filter, u db.Unit) (*model.UnitTrace, error) {
	row, err := s.q.GetUnitTrace(ctx, u.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stock: trace: %w", err)
	}
	out := &model.UnitTrace{Source: row.Source, Orders: []model.TraceOrder{}, Services: []model.TraceService{}}
	if row.WarehouseUuid.Valid {
		out.OriginWarehouse = &model.WarehouseRef{
			UUID: uuid.UUID(row.WarehouseUuid.Bytes), Code: row.WarehouseCode.String, Name: row.WarehouseName.String,
		}
	}
	orders, err := s.q.ListUnitTraceOrders(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("stock: trace orders: %w", err)
	}
	for _, o := range orders {
		if !f.AllowsOrg(o.BuyerID, u.BrandID) && !f.AllowsOrg(o.SellerID, u.BrandID) {
			continue
		}
		out.Orders = append(out.Orders, model.TraceOrder{
			UUID: o.Uuid, OrderNo: o.OrderNo, Status: o.Status, ExternalReference: textPtr(o.ExternalReference),
			Buyer:     model.OrgRef{UUID: o.BuyerUuid, Name: o.BuyerName, Type: o.BuyerType},
			Seller:    model.OrgRef{UUID: o.SellerUuid, Name: o.SellerName, Type: o.SellerType},
			CreatedAt: o.CreatedAt.Time, SubmittedAt: timePtr(o.SubmittedAt), ShippedAt: timePtr(o.ShippedAt),
			ReceivedAt: timePtr(o.ReceivedAt),
		})
	}
	services, err := s.q.ListUnitTraceServices(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("stock: trace services: %w", err)
	}
	for _, sv := range services {
		if !f.AllowsOrg(sv.OrganizationID, u.BrandID) {
			continue
		}
		out.Services = append(out.Services, model.TraceService{
			UUID: sv.Uuid, ServiceNo: sv.ServiceNo, Status: sv.Status, CompletedAt: timePtr(sv.CompletedAt),
			Organization: model.OrgRef{UUID: sv.OrganizationUuid, Name: sv.OrganizationName, Type: sv.OrganizationType},
			Customer: model.TraceCustomer{
				UUID: sv.CustomerUuid, Name: strings.TrimSpace(sv.CustomerName + " " + sv.CustomerSurname),
			},
		})
	}
	return out, nil
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// WarehouseUnitSort is the sort contract of GET
// /v1/stock/warehouses/{uuid}/units (TEC-1006).
var WarehouseUnitSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"barcode": "barcode", "product": "product", "source": "source",
		"created_at": "created_at", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "updated_at", Desc: true},
}

// Warehouse unit states (TEC-1006).
const (
	WarehouseUnitIn   = "in"   // placed in one of the warehouse's locations
	WarehouseUnitLeft = "left" // went out of it (dealer, service, trash, ...)
)

// TraceSources are the source filter values.
var TraceSources = []string{model.TraceSourceOrder, model.TraceSourceStock, model.TraceSourceUnknown}

// WarehouseUnitFilter is the query of the warehouse unit list.
type WarehouseUnitFilter struct {
	State         string
	Sources       []string
	ProductUUID   *uuid.UUID
	Q             string
	Sort          apiquery.ResolvedSort
	Limit, Offset int32
}

// ParseWarehouseUnitFilter reads the list contract parameters.
func ParseWarehouseUnitFilter(v url.Values) (WarehouseUnitFilter, error) {
	q := apiquery.Parse(v)
	out := WarehouseUnitFilter{Limit: int32(q.Limit), Offset: int32(q.Offset), Q: strings.TrimSpace(q.Q)}
	sort, err := apiquery.ResolveSort(q.Sort, WarehouseUnitSort)
	if err != nil {
		return out, err
	}
	out.Sort = sort
	states, err := apiquery.EnumList(v, "state", WarehouseUnitIn, WarehouseUnitLeft)
	if err != nil {
		return out, err
	}
	if len(states) == 1 {
		out.State = states[0]
	}
	if out.Sources, err = apiquery.EnumList(v, "source", TraceSources...); err != nil {
		return out, err
	}
	if p := strings.TrimSpace(v.Get("product_uuid")); p != "" {
		id, err := uuid.Parse(p)
		if err != nil {
			return out, invalidParam("product_uuid", "product_uuid is invalid")
		}
		out.ProductUUID = &id
	}
	return out, nil
}

// WarehouseUnits lists every unit that originates from a warehouse of the
// active organization, still in it or gone, with its trace (TEC-1006).
func (s *Service) WarehouseUnits(ctx context.Context, orgID int64, warehouseUUID uuid.UUID, in WarehouseUnitFilter) ([]model.WarehouseUnitRow, int64, error) {
	wh, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: warehouseUUID, OrganizationID: orgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("stock: warehouse: %w", err)
	}
	var product pgtype.Int8
	if in.ProductUUID != nil {
		id, err := s.q.GetProductIDByUUID(ctx, *in.ProductUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return []model.WarehouseUnitRow{}, 0, nil
		}
		if err != nil {
			return nil, 0, fmt.Errorf("stock: product filter: %w", err)
		}
		product = pgtype.Int8{Int64: id, Valid: true}
	}
	state, q := textArg(in.State), textArg(in.Q)
	total, err := s.q.CountWarehouseTraceUnits(ctx, db.CountWarehouseTraceUnitsParams{
		WarehouseID: wh.ID, State: state, Sources: in.Sources, ProductID: product, Q: q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: count warehouse units: %w", err)
	}
	rows, err := s.q.ListWarehouseTraceUnits(ctx, db.ListWarehouseTraceUnitsParams{
		WarehouseID: wh.ID, State: state, Sources: in.Sources, ProductID: product, Q: q,
		SortKey: in.Sort.Key, SortDesc: in.Sort.Desc, LimitCount: in.Limit, OffsetCount: in.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: warehouse units: %w", err)
	}
	out := make([]model.WarehouseUnitRow, 0, len(rows))
	for _, r := range rows {
		row := model.WarehouseUnitRow{
			UUID: r.Uuid, Barcode: r.Barcode, UnitKind: r.UnitKind, Status: r.Status,
			InitialMeters: numericPtr(r.InitialMeters), RemainingMeters: numericPtr(r.RemainingMeters),
			Product: model.ProductRef{UUID: r.ProductUuid, SKU: r.ProductSku, Name: r.ProductName},
			Source:  r.Source, InWarehouse: r.InWarehouse, LocationCode: textPtr(r.LocationCode),
			CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
		if r.OwnerType != "" {
			ot := r.OwnerType
			row.OwnerType = &ot
		}
		if r.HolderUuid.Valid {
			row.Holder = &model.OrgRef{UUID: uuid.UUID(r.HolderUuid.Bytes), Name: r.HolderName.String, Type: r.HolderType.String}
		}
		if r.OrderUuid.Valid {
			row.Order = &model.TraceOrderRef{UUID: uuid.UUID(r.OrderUuid.Bytes), OrderNo: r.OrderNo.String, BuyerName: r.OrderBuyerName.String}
		}
		if r.ServiceUuid.Valid {
			row.Service = &model.TraceServiceRef{
				UUID: uuid.UUID(r.ServiceUuid.Bytes), ServiceNo: r.ServiceNo.String,
				CompletedAt: timePtr(r.ServiceCompletedAt), CustomerName: r.CustomerName,
			}
		}
		out = append(out, row)
	}
	return out, total, nil
}
