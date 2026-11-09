package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
)

// WarehouseStock serves GET /v1/stock/warehouses/{uuid}/products (TEC-1090,
// TEC-1069): the product totals of one warehouse of the active organization
// (quantity, roll count, meters), with the product stock list filters and
// sort. A warehouse of another organization is 404 (same rule as
// WarehouseUnits).
func (s *Service) WarehouseStock(ctx context.Context, f scopefilter.Filter, orgID int64, warehouseUUID uuid.UUID, in model.StockFilter) ([]model.WarehouseProductStock, int64, error) {
	wh, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: warehouseUUID, OrganizationID: orgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("stock: warehouse: %w", err)
	}
	a, ok, err := s.resolveFilter(ctx, f, in)
	if err != nil || !ok {
		return []model.WarehouseProductStock{}, 0, err
	}
	total, err := s.q.CountWarehouseProductStockRows(ctx, db.CountWarehouseProductStockRowsParams{
		WarehouseID: wh.ID, OrganizationID: wh.OrganizationID,
		BrandID: a.brand, ProductID: a.product, CategoryID: a.category, InStock: a.inStock, Q: a.q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: count warehouse stock: %w", err)
	}
	rows, err := s.q.ListWarehouseProductStockRows(ctx, db.ListWarehouseProductStockRowsParams{
		WarehouseID: wh.ID, OrganizationID: wh.OrganizationID,
		BrandID: a.brand, ProductID: a.product, CategoryID: a.category, InStock: a.inStock, Q: a.q,
		SortKey: a.sortKey, SortDesc: a.sortDesc, LimitCount: in.Limit, OffsetCount: in.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: warehouse stock: %w", err)
	}
	base := make([]db.ListOrganizationProductStockRowsRow, len(rows))
	for i, r := range rows {
		base[i] = db.ListOrganizationProductStockRowsRow{
			ProductID: r.ProductID, Quantity: r.Quantity, Meters: r.Meters, UpdatedAt: r.UpdatedAt,
			ProductUuid: r.ProductUuid, Sku: r.Sku, ProductName: r.ProductName, UnitType: r.UnitType,
			UsesFixedBarcode: r.UsesFixedBarcode, ProductActive: r.ProductActive,
			CategoryUuid: r.CategoryUuid, CategoryName: r.CategoryName,
		}
	}
	fixed, err := s.q.ListFixedBarcodeQuantitiesByWarehouse(ctx, db.ListFixedBarcodeQuantitiesByWarehouseParams{
		WarehouseID: wh.ID, OrganizationID: wh.OrganizationID, ProductIds: fixedProductIDs(base),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("stock: fixed barcodes: %w", err)
	}
	held := make([]db.ListFixedBarcodeQuantitiesByHolderRow, len(fixed))
	for i, r := range fixed {
		held[i] = db.ListFixedBarcodeQuantitiesByHolderRow(r)
	}
	items := stockItems(base, fixedByProduct(held))
	out := make([]model.WarehouseProductStock, len(items))
	for i, it := range items {
		out[i] = model.WarehouseProductStock{ProductStock: it, RollCount: rows[i].RollCount}
	}
	return out, total, nil
}
