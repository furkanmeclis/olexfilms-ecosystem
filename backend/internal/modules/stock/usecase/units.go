package usecase

// TEC-216 (F1-12a): unit list of an organization. The distributor reads the
// units of its dealers through the stock.read subtree grant (000061); the
// purchase price column follows pricing.purchase.read (K8) and is null
// otherwise.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrWriteOutOfReach: the unit is inside the caller's stock.read reach but
// outside its stock.write reach (a distributor on dealer stock, TEC-216).
var ErrWriteOutOfReach = errors.New("stock: unit outside the write scope")

// UnitViewer is the caller of the unit list: the stock.read filter plus
// the principal, from which the purchase price visibility is resolved.
type UnitViewer struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

// purchaseVisible: the viewer reads the purchase prices of organization o.
// pricing.purchase.read must reach o; a direct parent also sees the
// purchase price of its child, which is the parent's own sale price (K8),
// through pricing.sale.read on itself.
func (s *Service) purchaseVisible(ctx context.Context, v UnitViewer, o db.Organization) (bool, error) {
	pf, err := scopefilter.Resolve(ctx, s.q, v.Principal, &v.Org, rbac.PermPricingPurchaseRead)
	switch {
	case err == nil && pf.AllowsOrg(o.ID, o.BrandID):
		return true, nil
	case err != nil && !errors.Is(err, scopefilter.ErrForbidden) && !errors.Is(err, scopefilter.ErrOrganizationRequired):
		return false, fmt.Errorf("stock: purchase scope: %w", err)
	}
	if o.ParentID.Valid && o.ParentID.Int64 == v.Org.InternalID && o.ID != v.Org.InternalID {
		return v.Principal.Can(rbac.PermPricingSaleRead, rbac.ScopeManaged), nil
	}
	return false, nil
}

// OrganizationUnits serves GET /v1/stock/organizations/{uuid}/units.
func (s *Service) OrganizationUnits(ctx context.Context, v UnitViewer, orgUUID uuid.UUID, in model.UnitFilter) ([]model.StockUnitRow, int64, error) {
	o, err := s.q.GetOrganizationByUUID(ctx, orgUUID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (o.DeletedAt.Valid || !v.Filter.AllowsOrg(o.ID, o.BrandID))) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("stock: organization: %w", err)
	}
	var product pgtype.Int8
	if in.ProductUUID != nil {
		id, err := s.q.GetProductIDByUUID(ctx, *in.ProductUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return []model.StockUnitRow{}, 0, nil
		}
		if err != nil {
			return nil, 0, fmt.Errorf("stock: product filter: %w", err)
		}
		product = pgtype.Int8{Int64: id, Valid: true}
	}
	status, barcode, q := textArg(in.Status), textArg(in.Barcode), textArg(in.Q)
	brand := v.Filter.BrandIDArg()
	lp := db.ListOrganizationStockUnitRowsParams{
		OrganizationID: o.ID, BrandID: brand, ProductID: product, Status: status, Barcode: barcode, Q: q,
		LimitCount: in.Limit, OffsetCount: in.Offset,
	}
	var (
		rows    []db.ListOrganizationStockUnitRowsRow
		total   int64
		indexed bool
	)
	// TEC-210: a text search goes to the stock units index when it is up;
	// the exact barcode filter stays on SQL.
	if q.Valid && !barcode.Valid && s.indexEnabled() {
		rows, total, indexed = s.searchUnitsIndexed(ctx, lp)
	}
	if !indexed {
		total, err = s.q.CountOrganizationStockUnitRows(ctx, db.CountOrganizationStockUnitRowsParams{
			OrganizationID: o.ID, BrandID: brand, ProductID: product, Status: status, Barcode: barcode, Q: q,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("stock: count units: %w", err)
		}
		rows, err = s.q.ListOrganizationStockUnitRows(ctx, lp)
		if err != nil {
			return nil, 0, fmt.Errorf("stock: units: %w", err)
		}
	}
	prices, err := s.unitPurchasePrices(ctx, v, o, rows)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.StockUnitRow, 0, len(rows))
	for _, r := range rows {
		row := model.StockUnitRow{
			UUID: r.Uuid, Barcode: r.Barcode, UnitKind: r.UnitKind, Status: r.Status, Quantity: r.Quantity,
			InitialMeters: numericPtr(r.InitialMeters), RemainingMeters: numericPtr(r.RemainingMeters),
			Product: model.ProductRef{
				UUID: r.ProductUuid, SKU: r.Sku, Name: r.ProductName,
				UnitType: r.UnitType, UsesFixedBarcode: r.UsesFixedBarcode,
			},
			UpdatedAt: r.UpdatedAt.Time,
		}
		if r.LocationUuid.Valid {
			row.Location = &model.LocationRef{UUID: uuid.UUID(r.LocationUuid.Bytes), Code: r.LocationCode.String, Name: r.LocationName.String}
		}
		if p, ok := prices[r.ProductID]; ok {
			price := p
			row.PurchasePrice = &price
		}
		out = append(out, row)
	}
	return out, total, nil
}

func textArg(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// unitPurchasePrices resolves the purchase prices of the page's products
// for the holding organization, or nothing when the viewer may not read
// them. The center has no purchase chain above it (K6): no price.
func (s *Service) unitPurchasePrices(ctx context.Context, v UnitViewer, o db.Organization,
	rows []db.ListOrganizationStockUnitRowsRow) (map[int64]model.UnitPurchasePrice, error) {
	out := map[int64]model.UnitPurchasePrice{}
	if len(rows) == 0 || (o.Type != pricingusecase.OrgDistributor && o.Type != pricingusecase.OrgDealer) {
		return out, nil
	}
	ok, err := s.purchaseVisible(ctx, v, o)
	if err != nil || !ok {
		return out, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ProductID)
	}
	prices, err := pricingusecase.New(s.q).BuyerPurchasePrices(ctx, o.ID, o.Type, o.BrandID, uniq(ids), o.Currency)
	if err != nil {
		return nil, fmt.Errorf("stock: purchase prices: %w", err)
	}
	for id, p := range prices {
		out[id] = model.UnitPurchasePrice{Amount: p.Price, Currency: o.Currency, Source: p.Source}
	}
	return out, nil
}

// checkWriteReach refuses a write on a unit the caller may read (stock.read
// subtree) but not write (stock.write managed): a distributor acting on
// dealer stock gets 403 instead of the generic "not on hand" conflict.
func checkWriteReach(ctx context.Context, q *db.Queries, c Caller, unitID int64) error {
	st, err := currentState(ctx, q, unitID)
	if err != nil || st == nil || c.Filter.AllowsOrg(st.HolderOrgID, st.BrandID) {
		return err
	}
	rf, err := scopefilter.Resolve(ctx, q, c.Principal, &c.Org, rbac.PermStockRead)
	if err != nil {
		return nil //nolint:nilerr // without stock.read reach the ledger's own on-hand check answers
	}
	if rf.AllowsOrg(st.HolderOrgID, st.BrandID) {
		return ErrWriteOutOfReach
	}
	return nil
}
