// Package groups binds the list scoped search specs to their module lists
// for the TEC-213 global search. Each Group calls the same use case method
// as GET /v1/<module>?q= with the Caller its route builds, so the index
// filter and the Postgres scope reload are the list's own (TEC-164 /
// TEC-209 / TEC-210). Permission and Feature are the list route's
// RequireScope slug and RequireFeature key.
package groups

import (
	"context"
	"errors"

	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	searchusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/usecase"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	stockmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

// Lists are the module use cases behind the groups; a nil one leaves its
// group out.
type Lists struct {
	Customers     *customersusecase.Service
	Services      *servicesusecase.Service
	Warranties    *warrantyusecase.Reader
	Orders        *ordersusecase.Service
	Organizations *orgusecase.Service
	Stock         *stockusecase.Service
	Leads         *leadsusecase.Service
}

func uuidsOf[T any](rows []T, key func(T) uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, key(r))
	}
	return out
}

// Build returns the groups in palette order: customers, vehicles,
// services, warranties, orders, organizations, stock units.
func Build(l Lists) []searchusecase.Group {
	var out []searchusecase.Group
	if l.Customers != nil {
		cs := l.Customers
		caller := func(ctx context.Context, c searchusecase.Caller) customersusecase.Caller {
			return customersusecase.Caller{
				UserID: c.Principal.UserInternal, Org: c.Org, Filter: c.Filter,
				Locale: i18n.FromContext(ctx).Locale,
			}
		}
		out = append(out, searchusecase.Group{
			Spec: customersusecase.SearchSpec, Permission: rbac.PermCustomersRead, Feature: features.ModuleCustomers,
			Search: func(ctx context.Context, c searchusecase.Caller, q string, limit int32) ([]uuid.UUID, error) {
				rows, _, err := cs.ListCustomers(ctx, caller(ctx, c), customersusecase.ListFilter{Q: q, Limit: limit})
				return uuidsOf(rows, func(r customersusecase.CustomerSummary) uuid.UUID { return r.UUID }), err
			},
		}, searchusecase.Group{
			Spec: searchengine.SpecVehicles, Permission: rbac.PermVehiclesRead, Feature: features.ModuleCustomers,
			Search: func(ctx context.Context, c searchusecase.Caller, q string, limit int32) ([]uuid.UUID, error) {
				rows, _, err := cs.ListVehicles(ctx, caller(ctx, c), customersusecase.VehicleFilter{Q: q, Limit: limit})
				if errors.Is(err, customersusecase.ErrForbidden) {
					return nil, nil
				}
				return uuidsOf(rows, func(r customersusecase.VehicleView) uuid.UUID { return r.UUID }), err
			},
		})
	}
	if l.Services != nil {
		ss := l.Services
		out = append(out, searchusecase.Group{
			Spec: searchengine.SpecServices, Permission: rbac.PermServicesRead, Feature: features.ModuleServices,
			Search: func(ctx context.Context, c searchusecase.Caller, q string, limit int32) ([]uuid.UUID, error) {
				rows, _, err := ss.List(ctx, servicesusecase.Caller{Principal: c.Principal, Org: c.Org, Filter: c.Filter},
					servicesusecase.ListFilter{Q: q, Limit: limit})
				return uuidsOf(rows, func(r servicesusecase.ServiceView) uuid.UUID { return r.UUID }), err
			},
		})
	}
	if l.Warranties != nil {
		wr := l.Warranties
		out = append(out, searchusecase.Group{
			Spec: searchengine.SpecWarranties, Permission: rbac.PermWarrantiesRead, Feature: features.ModuleServices,
			Search: func(ctx context.Context, c searchusecase.Caller, q string, limit int32) ([]uuid.UUID, error) {
				rows, _, err := wr.List(ctx, warrantyusecase.Caller{Principal: c.Principal, Org: c.Org, Filter: c.Filter},
					warrantyusecase.ListFilter{Q: q, Limit: limit})
				return uuidsOf(rows, func(r warrantyusecase.WarrantyListView) uuid.UUID { return r.UUID }), err
			},
		})
	}
	if l.Orders != nil {
		os := l.Orders
		out = append(out, searchusecase.Group{
			Spec: searchengine.SpecOrders, Permission: rbac.PermOrdersRead, Feature: features.ModuleOrders,
			Search: func(ctx context.Context, c searchusecase.Caller, q string, limit int32) ([]uuid.UUID, error) {
				rows, _, err := os.List(ctx, ordersusecase.Caller{Principal: c.Principal, Org: c.Org, Filter: c.Filter},
					ordersusecase.ListFilter{Side: ordersusecase.SideAll, Q: q, Limit: limit})
				return uuidsOf(rows, func(r ordersusecase.OrderView) uuid.UUID { return r.UUID }), err
			},
		})
	}
	if l.Organizations != nil {
		og := l.Organizations
		out = append(out, searchusecase.Group{
			Spec: searchengine.SpecOrganizations, Permission: rbac.PermOrganizationsRead, Feature: features.ModuleOrganizations,
			Search: func(ctx context.Context, c searchusecase.Caller, q string, limit int32) ([]uuid.UUID, error) {
				rows, err := og.ListInScope(ctx, c.Filter, orgusecase.ScopedListInput{Q: q, Limit: limit})
				return uuidsOf(rows, func(r orgusecase.Organization) uuid.UUID { return r.UUID }), err
			},
		})
	}
	if l.Stock != nil {
		st := l.Stock
		out = append(out, searchusecase.Group{
			Spec: searchengine.SpecStockUnits, Permission: rbac.PermStockRead, Feature: features.ModuleStock,
			// The palette searches the active organization's own units (the
			// unit list of GET /v1/stock/organizations/{own uuid}/units).
			Search: func(ctx context.Context, c searchusecase.Caller, q string, limit int32) ([]uuid.UUID, error) {
				v := stockusecase.UnitViewer{Principal: c.Principal, Org: c.Org, Filter: c.Filter}
				rows, _, err := st.OrganizationUnits(ctx, v, c.Org.UUID, stockmodel.UnitFilter{Q: q, Limit: limit})
				if errors.Is(err, stockusecase.ErrNotFound) {
					return nil, nil
				}
				return uuidsOf(rows, func(r stockmodel.StockUnitRow) uuid.UUID { return r.UUID }), err
			},
		})
	}
	if l.Leads != nil {
		ls := l.Leads
		out = append(out, searchusecase.Group{
			Spec: leadsusecase.SearchSpec, Permission: rbac.PermLeadsRead, Feature: features.ModuleLeads,
			Search: func(ctx context.Context, c searchusecase.Caller, q string, limit int32) ([]uuid.UUID, error) {
				rows, _, err := ls.List(ctx, leadsusecase.Caller{Principal: c.Principal, Org: c.Org, Filter: c.Filter},
					leadsusecase.ListFilter{Q: q, Limit: limit})
				return uuidsOf(rows, func(r leadsusecase.Lead) uuid.UUID { return r.UUID }), err
			},
		})
	}
	return out
}
