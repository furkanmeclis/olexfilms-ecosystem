package tools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	customersuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	warrantyuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// CustomersReader is the customers use case surface the tools use.
type CustomersReader interface {
	ListCustomers(ctx context.Context, c customersuc.Caller, f customersuc.ListFilter) ([]customersuc.CustomerSummary, int64, error)
}

// SearchCustomers: müşteri ara (scope'lu).
type SearchCustomers struct {
	customers CustomersReader
	tree      scopefilter.TreeReader
}

// Spec implements Tool.
func (SearchCustomers) Spec() Spec {
	return Spec{
		Name: "search_customers",
		Description: "Search the customers your organization can see by name, company, phone or e-mail. " +
			"Use search_services with the customer's name or phone for their services.",
		InputSchema: object(map[string]any{
			"query": strMin("Name, company name, phone or e-mail.", 2, 100),
			"limit": limitProp(),
		}, "query"),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleCustomers,
		Permissions: []string{rbac.PermCustomersRead},
	}
}

type customerRow struct {
	UUID           uuid.UUID  `json:"uuid"`
	Name           string     `json:"name"`
	CompanyName    *string    `json:"company_name,omitempty"`
	Type           string     `json:"type"`
	Phone          *string    `json:"phone,omitempty"`
	Email          *string    `json:"email,omitempty"`
	Status         string     `json:"status"`
	FirstServiceAt *time.Time `json:"first_service_at,omitempty"`
}

// Run implements Tool.
func (t SearchCustomers) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := errCases{tool: t.Spec().Name, what: "customer", forbidden: []error{customersuc.ErrForbidden},
		notFound: []error{customersuc.ErrCustomerNotFound}, invalid: asError[*customersuc.ValidationError]}
	p := env.Principal
	f, err := resolveScope(ctx, t.tree, p, rbac.PermCustomersRead)
	if err != nil {
		return errs.result(err)
	}
	c := customersuc.Caller{UserID: p.Auth.UserInternal, Org: *p.Org, Filter: f, Locale: i18n.FromContext(ctx).Locale}
	rows, total, err := t.customers.ListCustomers(ctx, c, customersuc.ListFilter{Q: strings.TrimSpace(in.Query), Limit: limitArg(in.Limit)})
	if err != nil {
		return errs.result(err)
	}
	out := make([]customerRow, 0, len(rows))
	for _, r := range rows {
		row := customerRow{
			UUID: r.UUID, Name: dataText(strings.TrimSpace(r.Name+" "+r.Surname), maxNameChars),
			CompanyName: textPtr(r.CompanyName, maxNameChars), Type: r.Type, Status: r.Status,
			FirstServiceAt: r.FirstServiceAt,
		}
		if !r.Anonymized {
			row.Phone, row.Email = r.Phone, r.Email
		}
		out = append(out, row)
	}
	return JSONResult(NewList(out, total))
}

// WarrantiesReader is the warranty list use case surface the tools use.
type WarrantiesReader interface {
	List(ctx context.Context, c warrantyuc.Caller, f warrantyuc.ListFilter) ([]warrantyuc.WarrantyListView, int64, error)
}

// LookupWarranties: garanti sorgula (kod / plaka / hizmet no / ürün).
type LookupWarranties struct {
	warranties WarrantiesReader
	tree       scopefilter.TreeReader
}

// Spec implements Tool.
func (LookupWarranties) Spec() Spec {
	return Spec{
		Name: "lookup_warranties",
		Description: "Look up warranties within your access by warranty code, plate, service number or product " +
			"name: status, start and end dates, product, vehicle and the service that issued it.",
		InputSchema: object(map[string]any{
			"query":  strMin("Warranty code, plate, service number or product name.", 2, 100),
			"status": enum("Optional status filter.", warrantyuc.StatusActive, warrantyuc.StatusExpired, warrantyuc.StatusVoid),
			"limit":  limitProp(),
		}, "query"),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleServices,
		Permissions: []string{rbac.PermWarrantiesRead},
	}
}

type warrantyRow struct {
	PublicCode   string     `json:"public_code"`
	Status       string     `json:"status"`
	Product      string     `json:"product"`
	StartAt      time.Time  `json:"start_at"`
	EndAt        time.Time  `json:"end_at"`
	DaysLeft     int        `json:"days_left"`
	VoidedAt     *time.Time `json:"voided_at,omitempty"`
	VoidReason   *string    `json:"void_reason,omitempty"`
	ServiceNo    string     `json:"service_no"`
	Organization string     `json:"organization"`
	Vehicle      string     `json:"vehicle"`
	Plate        *string    `json:"plate,omitempty"`
	Holder       string     `json:"holder,omitempty"`
}

// Run implements Tool.
func (t LookupWarranties) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Query  string `json:"query"`
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	errs := errCases{tool: t.Spec().Name, what: "warranty", notFound: []error{warrantyuc.ErrWarrantyNotFound},
		invalid: asError[*warrantyuc.ValidationError]}
	p := env.Principal
	f, err := resolveScope(ctx, t.tree, p, rbac.PermWarrantiesRead)
	if err != nil {
		return errs.result(err)
	}
	lf := warrantyuc.ListFilter{Q: strings.TrimSpace(in.Query), Limit: limitArg(in.Limit)}
	if in.Status != "" {
		lf.Statuses = []string{in.Status}
	}
	rows, total, err := t.warranties.List(ctx, warrantyuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}, lf)
	if err != nil {
		return errs.result(err)
	}
	out := make([]warrantyRow, 0, len(rows))
	for _, w := range rows {
		row := warrantyRow{
			PublicCode: w.PublicCode, Status: w.Status, Product: dataText(w.Product.Name, maxNameChars),
			StartAt: w.StartAt, EndAt: w.EndAt, VoidedAt: w.VoidedAt, VoidReason: textPtr(w.VoidReason, maxTextChars),
			ServiceNo: w.Service.ServiceNo, Organization: dataText(w.Organization.Name, maxNameChars),
			Vehicle: dataText(w.Vehicle.BrandName+" "+w.Vehicle.ModelName, maxNameChars), Plate: w.Vehicle.Plate,
		}
		if days := int(w.EndAt.Sub(env.Now).Hours() / 24); days > 0 && w.Status == warrantyuc.StatusActive {
			row.DaysLeft = days
		}
		if w.Holder != nil {
			row.Holder = dataText(strings.TrimSpace(w.Holder.Name+" "+w.Holder.Surname), maxNameChars)
		}
		out = append(out, row)
	}
	return JSONResult(NewList(out, total))
}
