package tools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// ServicesReader is the services use case surface the tools use.
type ServicesReader interface {
	List(ctx context.Context, c svcuc.Caller, f svcuc.ListFilter) ([]svcuc.ServiceView, int64, error)
	Get(ctx context.Context, c svcuc.Caller, id uuid.UUID) (svcuc.ServiceView, error)
	ActivitySummary(ctx context.Context, c svcuc.Caller, from, to time.Time) (svcuc.Activity, error)
}

type servicesBase struct {
	svc  ServicesReader
	tree scopefilter.TreeReader
}

func (b servicesBase) caller(ctx context.Context, p Principal) (svcuc.Caller, error) {
	f, err := resolveScope(ctx, b.tree, p, rbac.PermServicesRead)
	if err != nil {
		return svcuc.Caller{}, err
	}
	return svcuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}, nil
}

func serviceError(err error, tool string) (Result, error) {
	return errCases{tool: tool, what: "service", notFound: []error{svcuc.ErrNotFound},
		forbidden: []error{svcuc.ErrForbidden}, invalid: asError[*svcuc.ValidationError]}.result(err)
}

// serviceRow is the compact service of a list.
type serviceRow struct {
	UUID         uuid.UUID  `json:"uuid"`
	ServiceNo    string     `json:"service_no"`
	Status       string     `json:"status"`
	Organization string     `json:"organization"`
	Customer     string     `json:"customer"`
	Plate        *string    `json:"plate,omitempty"`
	VIN          *string    `json:"vin,omitempty"`
	Vehicle      string     `json:"vehicle"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

func customerName(c svcuc.CustomerRef) string {
	return dataText(strings.TrimSpace(c.Name+" "+c.Surname), maxNameChars)
}

func toServiceRow(v svcuc.ServiceView) serviceRow {
	return serviceRow{
		UUID: v.UUID, ServiceNo: v.ServiceNo, Status: v.Status,
		Organization: dataText(v.Organization.Name, maxNameChars), Customer: customerName(v.Customer),
		Plate: v.Plate, VIN: v.VIN, Vehicle: dataText(v.CarBrand.Name+" "+v.CarModel.Name, maxNameChars),
		CreatedAt: v.CreatedAt, CompletedAt: v.CompletedAt,
	}
}

// SearchServices: hizmet ara (plaka / VIN / hizmet no / müşteri).
type SearchServices struct{ servicesBase }

// Spec implements Tool.
func (SearchServices) Spec() Spec {
	return Spec{
		Name: "search_services",
		Description: "Search services (vehicle film / coating jobs) the user can see by plate, VIN, service number " +
			"or customer name / phone. Returns at most 50 rows, newest first. Use get_service for the details of one service.",
		InputSchema: object(map[string]any{
			"query":  strMin("Plate, VIN, service number, customer name or phone.", 2, 100),
			"status": enum("Optional status filter.", svcuc.StatusDraft, svcuc.StatusPending, svcuc.StatusProcessing, svcuc.StatusReady, svcuc.StatusCompleted, svcuc.StatusCancelled),
			"limit":  limitProp(),
		}, "query"),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleServices,
		Permissions: []string{rbac.PermServicesRead},
	}
}

// Run implements Tool.
func (t SearchServices) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Query  string `json:"query"`
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return serviceError(err, t.Spec().Name)
	}
	f := svcuc.ListFilter{Q: strings.TrimSpace(in.Query), Limit: limitArg(in.Limit)}
	if in.Status != "" {
		f.Statuses = []string{in.Status}
	}
	rows, total, err := t.svc.List(ctx, c, f)
	if err == nil && total == 0 {
		// Plates are stored without spaces ("34 ABC 123" -> "34ABC123").
		if compact := compactPlate(in.Query); compact != strings.ToUpper(f.Q) && len(compact) >= 2 {
			f.Q = compact
			rows, total, err = t.svc.List(ctx, c, f)
		}
	}
	if err != nil {
		return serviceError(err, t.Spec().Name)
	}
	out := make([]serviceRow, 0, len(rows))
	for _, v := range rows {
		out = append(out, toServiceRow(v))
	}
	return JSONResult(NewList(out, total))
}

// GetService: hizmet detayı.
type GetService struct{ servicesBase }

// Spec implements Tool.
func (GetService) Spec() Spec {
	return Spec{
		Name: "get_service",
		Description: "Get one service with its applied products, status history and warranties. " +
			"Identify it by its uuid or its service number (e.g. DSAB12CD34).",
		InputSchema: object(map[string]any{
			"service": strMin("Service uuid or service number.", 2, 64),
		}, "service"),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleServices,
		Permissions: []string{rbac.PermServicesRead},
	}
}

type serviceItem struct {
	Product  string   `json:"product"`
	SKU      string   `json:"sku"`
	Barcode  string   `json:"barcode"`
	Kind     string   `json:"kind"`
	Quantity *int32   `json:"quantity,omitempty"`
	Meters   *string  `json:"meters,omitempty"`
	Parts    []string `json:"applied_parts,omitempty"`
}

type serviceWarranty struct {
	PublicCode string    `json:"public_code"`
	Product    string    `json:"product"`
	Status     string    `json:"status"`
	EndAt      time.Time `json:"end_at"`
}

type serviceStatusLog struct {
	From *string   `json:"from,omitempty"`
	To   string    `json:"to"`
	At   time.Time `json:"at"`
}

type serviceDetail struct {
	serviceRow
	ModelYear      *int16             `json:"model_year,omitempty"`
	KM             *int32             `json:"km,omitempty"`
	Package        *string            `json:"package,omitempty"`
	Notes          *string            `json:"notes,omitempty"`
	CancelReason   *string            `json:"cancel_reason,omitempty"`
	HasMeasurement bool               `json:"has_measurement"`
	Items          []serviceItem      `json:"items"`
	Warranties     []serviceWarranty  `json:"warranties"`
	StatusHistory  []serviceStatusLog `json:"status_history"`
}

// Run implements Tool.
func (t GetService) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Service string `json:"service"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return serviceError(err, t.Spec().Name)
	}
	id, ok := parseID(in.Service)
	if !ok {
		// A service number: find it in the caller's own scope.
		no := strings.ToUpper(strings.TrimSpace(in.Service))
		rows, _, err := t.svc.List(ctx, c, svcuc.ListFilter{Q: no, Limit: 5})
		if err != nil {
			return serviceError(err, t.Spec().Name)
		}
		for _, r := range rows {
			if strings.EqualFold(r.ServiceNo, no) {
				id, ok = r.UUID, true
				break
			}
		}
		if !ok {
			return notFound("service"), nil
		}
	}
	v, err := t.svc.Get(ctx, c, id)
	if err != nil {
		return serviceError(err, t.Spec().Name)
	}
	d := serviceDetail{
		serviceRow: toServiceRow(v), ModelYear: v.ModelYear, KM: v.KM,
		Package: textPtr(v.Package, maxNameChars), Notes: textPtr(v.Notes, maxTextChars),
		CancelReason: textPtr(v.CancelReason, maxTextChars), HasMeasurement: v.HasMeasurement,
		Items: []serviceItem{}, Warranties: []serviceWarranty{}, StatusHistory: []serviceStatusLog{},
	}
	for _, it := range v.Items {
		d.Items = append(d.Items, serviceItem{
			Product: dataText(it.Product.Name, maxNameChars), SKU: it.Product.SKU, Barcode: it.Barcode,
			Kind: it.Kind, Quantity: it.Quantity, Meters: it.Meters, Parts: it.AppliedParts,
		})
	}
	for _, w := range v.Warranties {
		d.Warranties = append(d.Warranties, serviceWarranty{
			PublicCode: w.PublicCode, Product: dataText(w.ProductName, maxNameChars), Status: w.Status, EndAt: w.EndAt,
		})
	}
	for _, l := range v.StatusLogs {
		d.StatusHistory = append(d.StatusHistory, serviceStatusLog{From: l.FromStatus, To: l.ToStatus, At: l.CreatedAt})
	}
	return JSONResult(d)
}

// ServiceActivity: bu ay hizmet sayısı + marka kırılımı + en çok kullanılan
// ürünler (legacy chatbot dealer/* parity).
type ServiceActivity struct{ servicesBase }

// Spec implements Tool.
func (ServiceActivity) Spec() Spec {
	return Spec{
		Name: "service_activity_summary",
		Description: "Count services opened and completed in a period within the user's access, with the car brand " +
			"breakdown and the most used products of the completed services. Defaults to the current month. " +
			"Use for questions like 'how many cars did we do this month', 'how many BMWs', 'which product did we use most'.",
		InputSchema: object(map[string]any{
			"from": date("Period start (inclusive), YYYY-MM-DD. Default: first day of the current month."),
			"to":   date("Period end (inclusive), YYYY-MM-DD. Default: today."),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleServices,
		Permissions: []string{rbac.PermServicesRead},
	}
}

// Run implements Tool.
func (t ServiceActivity) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	loc := env.Loc()
	now := env.Now.In(loc)
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	if in.From != "" {
		d, _ := time.ParseInLocation(time.DateOnly, in.From, loc)
		from = d
	}
	if in.To != "" {
		d, _ := time.ParseInLocation(time.DateOnly, in.To, loc)
		to = d.AddDate(0, 0, 1)
	}
	if !to.After(from) {
		return invalidArg("to must not be before from"), nil
	}
	if to.Sub(from) > 366*24*time.Hour {
		return invalidArg("the period may be at most one year"), nil
	}
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return serviceError(err, t.Spec().Name)
	}
	a, err := t.svc.ActivitySummary(ctx, c, from, to)
	if err != nil {
		return serviceError(err, t.Spec().Name)
	}
	type out struct {
		From           string                   `json:"from"`
		To             string                   `json:"to"`
		CreatedCount   int64                    `json:"created_count"`
		CompletedCount int64                    `json:"completed_count"`
		CarBrands      []svcuc.ActivityCarBrand `json:"car_brands_of_completed"`
		TopProducts    []svcuc.ActivityProduct  `json:"top_products_of_completed"`
	}
	return JSONResult(out{
		From: from.Format(time.DateOnly), To: to.AddDate(0, 0, -1).Format(time.DateOnly),
		CreatedCount: a.CreatedCount, CompletedCount: a.CompletedCount, CarBrands: a.CarBrands, TopProducts: a.TopProducts,
	})
}

// ServicePDFLink: hizmetin garanti sertifikası PDF linkleri (eski
// dealer_service_pdf paritesi, TEC-386). Dosya üretilmez; mevcut
// /garanti/{code}/pdf çıktısına kısa link verilir.
type ServicePDFLink struct {
	servicesBase
	links LinkMaker
}

// Spec implements Tool.
func (ServicePDFLink) Spec() Spec {
	return Spec{
		Name: "service_pdf_link",
		Description: "Get short links to the warranty certificate PDFs of one service (by uuid or service number), " +
			"e.g. to forward to the customer. The links open without signing in; no file is attached.",
		InputSchema: object(map[string]any{
			"service": strMin("Service uuid or service number.", 2, 64),
		}, "service"),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleServices,
		Permissions: []string{rbac.PermServicesRead},
	}
}

// Run implements Tool.
func (t ServicePDFLink) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Service string `json:"service"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return serviceError(err, t.Spec().Name)
	}
	id, ok := parseID(in.Service)
	if !ok {
		no := strings.ToUpper(strings.TrimSpace(in.Service))
		rows, _, err := t.svc.List(ctx, c, svcuc.ListFilter{Q: no, Limit: 5})
		if err != nil {
			return serviceError(err, t.Spec().Name)
		}
		for _, r := range rows {
			if strings.EqualFold(r.ServiceNo, no) {
				id, ok = r.UUID, true
				break
			}
		}
		if !ok {
			return notFound("service"), nil
		}
	}
	v, err := t.svc.Get(ctx, c, id)
	if err != nil {
		return serviceError(err, t.Spec().Name)
	}
	certs := make([]certificate, 0, len(v.Warranties))
	for _, w := range v.Warranties {
		certs = append(certs, certificate{Code: w.PublicCode, Product: w.ProductName, Status: w.Status})
	}
	pdfs, err := certificateLinks(ctx, t.links, env.Principal, certs)
	if err != nil {
		return Result{}, err
	}
	out := struct {
		ServiceNo    string    `json:"service_no"`
		Certificates []pdfLink `json:"warranty_certificate_pdfs"`
		Hint         string    `json:"hint,omitempty"`
	}{ServiceNo: v.ServiceNo, Certificates: pdfs}
	if len(pdfs) == 0 {
		out.Hint = "This service has no warranty certificate yet (warranties are issued when the service is completed)."
	}
	return JSONResult(out)
}
