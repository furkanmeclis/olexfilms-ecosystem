package tools

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
)

// Deps wires the panel read tools to the module use cases; a nil
// dependency leaves its tools out.
type Deps struct {
	// Tree resolves subtree scopes (db.Queries).
	Tree          scopefilter.TreeReader
	Services      ServicesReader
	Warranties    WarrantiesReader
	Customers     CustomersReader
	Stock         StockReader
	Orders        OrdersReader
	Accounting    AccountingReader
	Appointments  AppointmentsReader
	Leads         LeadsReader
	Tasks         TasksReader
	Catalog       CatalogReader
	Organizations OrganizationsReader
	// Links shortens internal links (service_pdf_link, TEC-386).
	Links LinkMaker
	// Extensions are registered as they are. F5 registers its stock
	// forecast and performance tools here (Spec.Feature
	// features.ModuleStockForecast / ModulePerformance), so they appear only
	// when the module exists and is on for the organization.
	Extensions []Tool
}

// RegisterPanel adds the panel read tools whose use cases are wired.
func RegisterPanel(r *Registry, d Deps) {
	if d.Services != nil {
		b := servicesBase{svc: d.Services, tree: d.Tree}
		r.Register(SearchServices{b})
		r.Register(GetService{b})
		r.Register(ServiceActivity{b})
		if d.Links != nil {
			r.Register(ServicePDFLink{servicesBase: b, links: d.Links})
		}
	}
	if d.Warranties != nil {
		r.Register(LookupWarranties{warranties: d.Warranties, tree: d.Tree})
	}
	if d.Customers != nil {
		r.Register(SearchCustomers{customers: d.Customers, tree: d.Tree})
	}
	if d.Stock != nil {
		b := stockBase{stock: d.Stock, tree: d.Tree}
		r.Register(StockSummary{b})
		r.Register(StockUnits{b})
	}
	if d.Orders != nil {
		b := ordersBase{orders: d.Orders, tree: d.Tree}
		r.Register(ListOrders{b})
		r.Register(GetOrder{b})
	}
	if d.Accounting != nil {
		r.Register(BalanceSummary{accounting: d.Accounting, tree: d.Tree})
	}
	if d.Appointments != nil {
		r.Register(ListAppointments{appointments: d.Appointments, tree: d.Tree})
	}
	if d.Leads != nil {
		r.Register(ListLeads{leads: d.Leads, tree: d.Tree})
	}
	if d.Tasks != nil {
		r.Register(MyTasks{tasks: d.Tasks})
	}
	if d.Catalog != nil {
		r.Register(SearchProducts{catalog: d.Catalog})
	}
	if d.Organizations != nil {
		r.Register(ListSubOrganizations{orgs: d.Organizations, tree: d.Tree})
	}
	for _, t := range d.Extensions {
		r.Register(t)
	}
}
