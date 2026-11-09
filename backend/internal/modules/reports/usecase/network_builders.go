package usecase

// TEC-1090: network reports by organization for the mobile home screens
// (TEC-1068 sales home: top buyers and dealers without orders; TEC-1073
// center home: services by organization).

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5/pgtype"
)

// orderAmountsVisible is the order price rule of the reports: an order
// total is the buyer's purchase price and the seller's sale price, so the
// amount needs pricing.purchase.read or pricing.sale.read (same rule as the
// order observer elsewhere and the mobile price visibility, TEC-1051).
func orderAmountsVisible(p authctx.Principal) bool {
	return p.HasPermission(rbac.PermPricingPurchaseRead) || p.HasPermission(rbac.PermPricingSaleRead)
}

// ordersByBuyer: the buyers that ordered in the range, most orders first;
// total / currency are null without a pricing read permission.
func (s *Service) ordersByBuyer(ctx context.Context, c Caller, q query, a args, env *Envelope) error {
	rows, err := s.q.ReportOrdersByBuyer(ctx, db.ReportOrdersByBuyerParams{
		BrandID: a.brandID, OrgIds: a.orgIDs, CreatedByUserID: a.createdBy, SelfOrgID: c.Org.InternalID,
		RangeFrom: ts(q.from), RangeTo: ts(q.to), RowLimit: q.limit,
	})
	if err != nil {
		return fmt.Errorf("reports: orders by buyer: %w", err)
	}
	amounts := orderAmountsVisible(c.Principal)
	sr := q.series("order_count", unitCount)
	cols := [][2]string{{"rank", "integer"}, {"organization", "string"}, {"city", "string"}, {"order_count", "integer"}}
	if amounts {
		cols = append(cols, [2]string{"total", "money"})
	}
	cols = append(cols, [2]string{"last_order_at", "datetime"})
	env.Columns = q.columns(cols)
	for i, r := range rows {
		sr.Points = append(sr.Points, Point{Key: r.OrganizationUuid.String(), Label: r.OrganizationName, Value: float64(r.OrderCount)})
		row := map[string]any{
			"rank": i + 1, "organization_uuid": r.OrganizationUuid, "organization": r.OrganizationName,
			"organization_type": r.OrganizationType, "city": r.City, "order_count": r.OrderCount,
			"total": nil, "currency": nil, "last_order_at": r.LastOrderAt.Time.UTC(),
		}
		if amounts {
			row["total"], row["currency"] = moneyText(r.Total), r.Currency
		}
		env.Rows = append(env.Rows, row)
	}
	env.Series = []Series{sr}
	return nil
}

// inactiveBuyers: active dealers without an order in the range (all_time:
// never), with their last order date; the cards series carries the full
// count (rows stop at limit).
func (s *Service) inactiveBuyers(ctx context.Context, c Caller, f scopefilter.Filter, q query, a args, env *Envelope) error {
	p := db.ReportInactiveBuyersParams{
		BrandID: a.brandID, OrgIds: a.orgIDs, AllTime: q.allTime,
		RangeFrom: ts(q.from), RangeTo: ts(q.to), RowLimit: q.limit,
	}
	// A managed reach (a distributor's own sales) looks at its direct dealers.
	if f.Scope == rbac.ScopeManaged {
		p.OrgIds, p.ParentID = nil, pgtype.Int8{Int64: c.Org.InternalID, Valid: true}
	}
	rows, err := s.q.ReportInactiveBuyers(ctx, p)
	if err != nil {
		return fmt.Errorf("reports: inactive buyers: %w", err)
	}
	var count int64
	env.Columns = q.columns([][2]string{{"organization", "string"}, {"city", "string"}, {"last_order_at", "datetime"}})
	for _, r := range rows {
		count = r.TotalCount
		row := map[string]any{
			"organization_uuid": r.OrganizationUuid, "organization": r.OrganizationName, "city": r.City, "last_order_at": nil,
		}
		if r.LastOrderAt.Valid {
			row["last_order_at"] = r.LastOrderAt.Time.UTC()
		}
		env.Rows = append(env.Rows, row)
	}
	cards := q.series("cards", unitCount)
	cards.Points = []Point{q.card("inactive_dealers", float64(count))}
	env.Series = []Series{cards}
	return nil
}

// servicesByOrg: organizations by services completed in the range. The
// center's own services are one row named by its service point when that
// is on (is_service_point), so it stands apart from the dealers.
func (s *Service) servicesByOrg(ctx context.Context, q query, a args, env *Envelope) error {
	rows, err := s.q.ReportServicesByOrg(ctx, db.ReportServicesByOrgParams{
		BrandID: a.brandID, OrgIds: a.orgIDs, CreatedByUserID: a.createdBy,
		RangeFrom: ts(q.from), RangeTo: ts(q.to), RowLimit: q.limit,
	})
	if err != nil {
		return fmt.Errorf("reports: services by organization: %w", err)
	}
	sr := q.series("service_count", unitCount)
	env.Columns = q.columns([][2]string{{"rank", "integer"}, {"organization", "string"}, {"city", "string"}, {"services_count", "integer"}})
	for i, r := range rows {
		name := r.OrganizationName
		point := r.OrganizationType == rbac.OrgTypeCenter && r.ServicePointEnabled && r.ServicePointName.Valid && r.ServicePointName.String != ""
		if point {
			name = r.ServicePointName.String
		}
		sr.Points = append(sr.Points, Point{Key: r.OrganizationUuid.String(), Label: name, Value: float64(r.ServiceCount)})
		env.Rows = append(env.Rows, map[string]any{
			"rank": i + 1, "organization_uuid": r.OrganizationUuid, "organization": name,
			"organization_name": r.OrganizationName, "organization_type": r.OrganizationType,
			"is_service_point": r.OrganizationType == rbac.OrgTypeCenter, "city": r.City, "services_count": r.ServiceCount,
		})
	}
	env.Series = []Series{sr}
	return nil
}

// moneyText is a NUMERIC(18,2) sum as text ("0.00" when empty).
func moneyText(v string) string {
	if v == "" {
		return "0.00"
	}
	return v
}
