package usecase

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// WarehouseSummary serves GET /v1/stock/warehouse-summary (TEC-1066): the
// in-stock totals of the active organization per warehouse (products,
// pieces, rolls, meters), for the warehouse home. The brand bound follows
// the stock.read scope (the center warehouse holds it at scope all, K20).
func (s *Service) WarehouseSummary(ctx context.Context, f scopefilter.Filter, orgID int64) ([]model.WarehouseStockSummary, error) {
	rows, err := s.q.WarehouseStockSummary(ctx, db.WarehouseStockSummaryParams{
		OrganizationID: orgID, BrandID: f.BrandIDArg(),
	})
	if err != nil {
		return nil, fmt.Errorf("stock: warehouse summary: %w", err)
	}
	out := make([]model.WarehouseStockSummary, 0, len(rows))
	for _, r := range rows {
		row := model.WarehouseStockSummary{Products: r.Products, Pieces: r.Pieces, Rolls: r.Rolls, Meters: r.Meters}
		if r.WarehouseUuid.Valid {
			row.Warehouse = &model.WarehouseSummaryRef{
				UUID: uuid.UUID(r.WarehouseUuid.Bytes), Code: r.WarehouseCode.String,
				Name: r.WarehouseName.String, IsPrimary: r.IsPrimary,
			}
		}
		out = append(out, row)
	}
	return out, nil
}
