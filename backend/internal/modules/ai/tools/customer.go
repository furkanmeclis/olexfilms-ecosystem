package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	appointmentsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	authmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	portaluc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/portalvehicles/usecase"
	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	warrantyuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	claimsmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
)

// TEC-386 (F4-01d): customer realm tools. The principal is a signed-in
// customer (portal session or a WhatsApp number resolved to a verified
// customer, TEC-394) of one brand. Every tool calls the portal use case
// with (brand, user), the same ownership the portal pages use: the user is
// the service customer or holds one of its warranties. Another customer's
// record answers NOT_FOUND exactly like a missing one.

// PortalReader is the portal vehicles use case surface (vehicles and the
// service list).
type PortalReader interface {
	ListVehicles(ctx context.Context, c portaluc.Caller, p portaluc.Page) ([]portaluc.VehicleView, int64, error)
	ListServices(ctx context.Context, c portaluc.Caller, f portaluc.ServiceListFilter) ([]portaluc.ServiceView, int64, error)
}

// PortalServiceReader is the services use case portal surface.
type PortalServiceReader interface {
	PortalGet(ctx context.Context, brandID, userID int64, id uuid.UUID) (svcuc.PortalServiceView, error)
	PortalGetReview(ctx context.Context, brandID, userID int64, id uuid.UUID) (svcuc.PortalServiceReview, error)
}

// PortalWarrantyReader lists the warranties the user holds.
type PortalWarrantyReader interface {
	PortalList(ctx context.Context, brandID, userID int64, f warrantyuc.ListFilter) ([]warrantyuc.WarrantyListView, int64, error)
}

// PortalAppointmentsReader lists the user's appointments.
type PortalAppointmentsReader interface {
	PortalList(ctx context.Context, c appointmentsuc.PortalCaller, f appointmentsuc.PortalListFilter) ([]appointmentsuc.PortalAppointment, int64, error)
}

// PortalClaimsReader lists the user's warranty claims.
type PortalClaimsReader interface {
	PortalList(ctx context.Context, brandID, userID int64) ([]claimsmodel.PortalClaimView, error)
}

// ProfileWriter updates the user's own profile (auth use case).
type ProfileWriter interface {
	UpdateProfile(ctx context.Context, userUUID uuid.UUID, impersonatorUUID, orgUUID *uuid.UUID, in authmodel.ProfilePatch) (authmodel.Me, error)
}

// CustomerDeps wires the customer tools; a nil dependency leaves its tools
// out.
type CustomerDeps struct {
	Portal       PortalReader
	Services     PortalServiceReader
	Warranties   PortalWarrantyReader
	Appointments PortalAppointmentsReader
	Claims       PortalClaimsReader
	Profile      ProfileWriter
	Links        LinkMaker
}

// RegisterCustomer adds the customer realm tools whose use cases are wired.
func RegisterCustomer(r *Registry, d CustomerDeps) {
	if d.Portal != nil {
		r.Register(MyVehicles{portal: d.Portal})
		r.Register(MyServices{portal: d.Portal})
		if d.Services != nil {
			b := customerServices{portal: d.Portal, services: d.Services, links: d.Links}
			r.Register(MyServiceDetail{b})
			r.Register(DealerReviewLink{b})
			if d.Links != nil {
				r.Register(MyServicePDFLink{b})
			}
		}
	}
	if d.Warranties != nil {
		r.Register(MyWarranties{warranties: d.Warranties})
	}
	if d.Appointments != nil {
		r.Register(MyAppointments{appointments: d.Appointments})
	}
	if d.Claims != nil {
		r.Register(MyWarrantyClaims{claims: d.Claims})
	}
	if d.Links != nil {
		r.Register(MyProfileLink{links: d.Links})
	}
	if d.Profile != nil {
		r.Register(ChangeLanguage{profile: d.Profile})
	}
}

// customerSpec fills the fields every customer tool shares.
func customerSpec(name, desc string, schema map[string]any) Spec {
	return Spec{Name: name, Description: desc, InputSchema: schema, Kind: KindRead, Realm: RealmCustomer}
}

func portalCaller(p Principal) portaluc.Caller {
	return portaluc.Caller{BrandID: p.brandID(), UserID: p.Auth.UserInternal}
}

// MyVehicles: araçlarım.
type MyVehicles struct{ portal PortalReader }

// Spec implements Tool.
func (MyVehicles) Spec() Spec {
	return customerSpec("my_vehicles",
		"List the customer's own vehicles (from their services): plate, car brand / model, year, service count, "+
			"active warranty count and last service date.",
		object(map[string]any{"limit": limitProp()}))
}

type vehicleRow struct {
	Plate           *string    `json:"plate,omitempty"`
	CarBrand        string     `json:"car_brand,omitempty"`
	CarModel        string     `json:"car_model,omitempty"`
	ModelYear       *int16     `json:"model_year,omitempty"`
	ServiceCount    int64      `json:"service_count"`
	ActiveWarranty  int64      `json:"active_warranty_count"`
	LastServiceAt   *time.Time `json:"last_service_at,omitempty"`
	RegisteredSince time.Time  `json:"registered_since"`
}

// Run implements Tool.
func (t MyVehicles) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Limit int `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	rows, total, err := t.portal.ListVehicles(ctx, portalCaller(env.Principal), portaluc.Page{Limit: limitArg(in.Limit)})
	if err != nil {
		return Result{}, err
	}
	out := make([]vehicleRow, 0, len(rows))
	for _, v := range rows {
		r := vehicleRow{Plate: v.Plate, ModelYear: v.ModelYear, ServiceCount: v.ServiceCount,
			ActiveWarranty: v.ActiveWarrantyCount, LastServiceAt: v.LastServiceAt, RegisteredSince: v.CreatedAt}
		if v.CarBrand != nil {
			r.CarBrand = dataText(v.CarBrand.Name, maxNameChars)
		}
		if v.CarModel != nil {
			r.CarModel = dataText(v.CarModel.Name, maxNameChars)
		}
		out = append(out, r)
	}
	return JSONResult(NewList(out, total))
}

// MyServices: hizmetlerim.
type MyServices struct{ portal PortalReader }

// Spec implements Tool.
func (MyServices) Spec() Spec {
	return customerSpec("my_services",
		"List the customer's own services (film / coating jobs), newest first: service number, status, dealer, "+
			"vehicle and dates. Optionally search by plate or service number. Use my_service_detail for one service.",
		object(map[string]any{
			"query":  str("Optional plate or service number.", 100),
			"status": enum("Optional status filter.", portaluc.ServiceStatuses...),
			"limit":  limitProp(),
		}))
}

type myServiceRow struct {
	ServiceNo   string     `json:"service_no"`
	Status      string     `json:"status"`
	Dealer      string     `json:"dealer"`
	Plate       *string    `json:"plate,omitempty"`
	Vehicle     string     `json:"vehicle"`
	Package     *string    `json:"package,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Run implements Tool.
func (t MyServices) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Query  string `json:"query"`
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	c := portalCaller(env.Principal)
	f := portaluc.ServiceListFilter{Page: portaluc.Page{Limit: limitArg(in.Limit)}, Q: strings.TrimSpace(in.Query)}
	if in.Status != "" {
		f.Statuses = []string{in.Status}
	}
	rows, total, err := t.portal.ListServices(ctx, c, f)
	if err == nil && total == 0 && f.Q != "" {
		if compact := compactPlate(f.Q); compact != strings.ToUpper(f.Q) && len(compact) >= 2 {
			f.Q = compact
			rows, total, err = t.portal.ListServices(ctx, c, f)
		}
	}
	if err != nil {
		return Result{}, err
	}
	out := make([]myServiceRow, 0, len(rows))
	for _, v := range rows {
		out = append(out, myServiceRow{
			ServiceNo: v.ServiceNo, Status: v.Status, Dealer: dataText(v.Organization.Name, maxNameChars),
			Plate: v.Plate, Vehicle: dataText(v.CarBrandName+" "+v.CarModelName, maxNameChars),
			Package: textPtr(v.Package, maxNameChars), CreatedAt: v.CreatedAt, CompletedAt: v.CompletedAt,
		})
	}
	return JSONResult(NewList(out, total))
}

// customerServices resolves a service of the customer by uuid or number.
type customerServices struct {
	portal   PortalReader
	services PortalServiceReader
	links    LinkMaker
}

// own returns the uuid of the customer's service. A number is looked up
// only among the customer's own services, so another customer's number is
// not found.
func (b customerServices) own(ctx context.Context, p Principal, ref string) (uuid.UUID, bool, error) {
	if id, ok := parseID(ref); ok {
		return id, true, nil
	}
	no := strings.ToUpper(strings.TrimSpace(ref))
	rows, _, err := b.portal.ListServices(ctx, portalCaller(p), portaluc.ServiceListFilter{Page: portaluc.Page{Limit: 5}, Q: no})
	if err != nil {
		return uuid.Nil, false, err
	}
	for _, r := range rows {
		if strings.EqualFold(r.ServiceNo, no) {
			return r.UUID, true, nil
		}
	}
	return uuid.Nil, false, nil
}

// detail loads the portal view of the customer's service.
func (b customerServices) detail(ctx context.Context, p Principal, ref string) (svcuc.PortalServiceView, *Result, error) {
	id, ok, err := b.own(ctx, p, ref)
	if err != nil {
		return svcuc.PortalServiceView{}, nil, err
	}
	if !ok {
		r := notFound("service")
		return svcuc.PortalServiceView{}, &r, nil
	}
	v, err := b.services.PortalGet(ctx, p.brandID(), p.Auth.UserInternal, id)
	if errors.Is(err, svcuc.ErrNotFound) {
		r := notFound("service")
		return svcuc.PortalServiceView{}, &r, nil
	}
	return v, nil, err
}

func serviceRefSchema() map[string]any {
	return object(map[string]any{
		"service": strMin("Service number (e.g. DSAB12CD34) or uuid of one of the customer's services.", 2, 64),
	}, "service")
}

// MyServiceDetail: hizmet detayı.
type MyServiceDetail struct{ customerServices }

// Spec implements Tool.
func (MyServiceDetail) Spec() Spec {
	return customerSpec("my_service_detail",
		"Get one of the customer's own services by service number: status, vehicle, applied products and parts, "+
			"the dealer (name, city, WhatsApp) and its warranties with the days left.",
		serviceRefSchema())
}

type myWarrantyRow struct {
	PublicCode string    `json:"public_code"`
	Product    string    `json:"product"`
	Status     string    `json:"status"`
	StartAt    time.Time `json:"start_at"`
	EndAt      time.Time `json:"end_at"`
	DaysLeft   int       `json:"days_left"`
}

// Run implements Tool.
func (t MyServiceDetail) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Service string `json:"service"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	v, res, err := t.detail(ctx, env.Principal, in.Service)
	if res != nil || err != nil {
		return derefResult(res), err
	}
	type product struct {
		Name     string   `json:"name"`
		Category string   `json:"category"`
		Parts    []string `json:"applied_parts,omitempty"`
	}
	type dealer struct {
		Name     string  `json:"name"`
		City     string  `json:"city,omitempty"`
		District string  `json:"district,omitempty"`
		WhatsApp *string `json:"whatsapp,omitempty"`
	}
	out := struct {
		ServiceNo   string          `json:"service_no"`
		Status      string          `json:"status"`
		Plate       *string         `json:"plate,omitempty"`
		Vehicle     string          `json:"vehicle"`
		ModelYear   *int16          `json:"model_year,omitempty"`
		CreatedAt   time.Time       `json:"created_at"`
		CompletedAt *time.Time      `json:"completed_at,omitempty"`
		Products    []product       `json:"products"`
		Dealer      dealer          `json:"dealer"`
		Warranties  []myWarrantyRow `json:"warranties"`
	}{
		ServiceNo: v.ServiceNo, Status: v.Status, Plate: v.Plate,
		Vehicle: dataText(v.CarBrand.Name+" "+v.CarModel.Name, maxNameChars), ModelYear: v.ModelYear,
		CreatedAt: v.CreatedAt, CompletedAt: v.CompletedAt, Products: []product{}, Warranties: []myWarrantyRow{},
		Dealer: dealer{Name: dataText(v.Dealer.Name, maxNameChars), City: dataText(v.Dealer.City, maxNameChars),
			District: dataText(v.Dealer.District, maxNameChars), WhatsApp: v.Dealer.WhatsApp},
	}
	for _, p := range v.Products {
		out.Products = append(out.Products, product{Name: dataText(p.Name, maxNameChars),
			Category: dataText(p.Category, maxNameChars), Parts: p.AppliedParts})
	}
	for _, w := range v.Warranties {
		days, _ := portaluc.TimeLeft(w.StartAt, w.EndAt, env.Now)
		if w.Status != warrantyuc.StatusActive {
			days = 0
		}
		out.Warranties = append(out.Warranties, myWarrantyRow{PublicCode: w.PublicCode,
			Product: dataText(w.ProductName, maxNameChars), Status: w.Status, StartAt: w.StartAt, EndAt: w.EndAt, DaysLeft: days})
	}
	return JSONResult(out)
}

func derefResult(r *Result) Result {
	if r == nil {
		return Result{}
	}
	return *r
}

// MyServicePDFLink: hizmet PDF linki (kısa URL).
type MyServicePDFLink struct{ customerServices }

// Spec implements Tool.
func (MyServicePDFLink) Spec() Spec {
	return customerSpec("my_service_pdf_link",
		"Get short links for one of the customer's services: the warranty certificate PDF of each warranty "+
			"(opens without signing in) and the service page in the customer portal, where the service PDF is "+
			"downloaded after signing in. Send the links as they are; no file is attached.",
		serviceRefSchema())
}

// Run implements Tool.
func (t MyServicePDFLink) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Service string `json:"service"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	v, res, err := t.detail(ctx, env.Principal, in.Service)
	if res != nil || err != nil {
		return derefResult(res), err
	}
	page, err := shortLink(ctx, t.links, env.Principal, "/portal/services/"+v.UUID.String())
	if err != nil {
		return Result{}, err
	}
	certs := make([]certificate, 0, len(v.Warranties))
	for _, w := range v.Warranties {
		certs = append(certs, certificate{Code: w.PublicCode, Product: w.ProductName, Status: w.Status})
	}
	pdfs, err := certificateLinks(ctx, t.links, env.Principal, certs)
	if err != nil {
		return Result{}, err
	}
	return JSONResult(struct {
		ServiceNo    string    `json:"service_no"`
		ServicePage  string    `json:"service_page_url"`
		Certificates []pdfLink `json:"warranty_certificate_pdfs"`
	}{ServiceNo: v.ServiceNo, ServicePage: page, Certificates: pdfs})
}

// DealerReviewLink: bayinin Google değerlendirme linki.
type DealerReviewLink struct{ customerServices }

// Spec implements Tool.
func (DealerReviewLink) Spec() Spec {
	return customerSpec("dealer_review_link",
		"Get the Google review link of the dealer that did the customer's service (default: the latest completed "+
			"service) and, when the customer can still rate it, the short link of the in-app review form.",
		object(map[string]any{
			"service": strMin("Optional service number or uuid; default the latest completed service.", 2, 64),
		}))
}

// Run implements Tool.
func (t DealerReviewLink) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Service string `json:"service"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	p := env.Principal
	var id uuid.UUID
	var serviceNo, dealer string
	if in.Service != "" {
		v, res, err := t.detail(ctx, p, in.Service)
		if res != nil || err != nil {
			return derefResult(res), err
		}
		id, serviceNo, dealer = v.UUID, v.ServiceNo, v.Dealer.Name
	} else {
		rows, _, err := t.portal.ListServices(ctx, portalCaller(p), portaluc.ServiceListFilter{
			Page: portaluc.Page{Limit: 1}, Statuses: []string{svcuc.StatusCompleted},
		})
		if err != nil {
			return Result{}, err
		}
		if len(rows) == 0 {
			return ErrorResult(CodeNotFound, "The customer has no completed service yet, so there is no dealer to review."), nil
		}
		id, serviceNo, dealer = rows[0].UUID, rows[0].ServiceNo, rows[0].Organization.Name
	}
	rv, err := t.services.PortalGetReview(ctx, p.brandID(), p.Auth.UserInternal, id)
	if errors.Is(err, svcuc.ErrNotFound) {
		return notFound("service"), nil
	}
	if err != nil {
		return Result{}, err
	}
	out := struct {
		ServiceNo  string  `json:"service_no"`
		Dealer     string  `json:"dealer"`
		GoogleURL  *string `json:"google_review_url"`
		ReviewForm string  `json:"review_form_url,omitempty"`
		Note       string  `json:"note,omitempty"`
	}{ServiceNo: serviceNo, Dealer: dataText(dealer, maxNameChars), GoogleURL: rv.GoogleBusinessURL}
	if rv.CanReview && t.links != nil {
		if out.ReviewForm, err = shortLink(ctx, t.links, p, svcuc.ReviewFormTarget(id)); err != nil {
			return Result{}, err
		}
	}
	if rv.GoogleBusinessURL == nil {
		out.Note = "This dealer has no Google review link."
	}
	return JSONResult(out)
}

// MyWarranties: garantilerim (kalan gün).
type MyWarranties struct{ warranties PortalWarrantyReader }

// Spec implements Tool.
func (MyWarranties) Spec() Spec {
	return customerSpec("my_warranties",
		"List the warranties the customer holds with the days left: code, product, vehicle plate, service number, "+
			"dealer, start / end dates. Active ones only unless include_expired is true.",
		object(map[string]any{
			"include_expired": map[string]any{"type": "boolean", "description": "Also list expired warranties."},
			"limit":           limitProp(),
		}))
}

// Run implements Tool.
func (t MyWarranties) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		IncludeExpired bool `json:"include_expired"`
		Limit          int  `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	f := warrantyuc.ListFilter{Statuses: []string{warrantyuc.StatusActive}, Limit: limitArg(in.Limit)}
	if in.IncludeExpired {
		f.Statuses = append(f.Statuses, warrantyuc.StatusExpired)
	}
	p := env.Principal
	rows, total, err := t.warranties.PortalList(ctx, p.brandID(), p.Auth.UserInternal, f)
	if err != nil {
		return Result{}, err
	}
	type row struct {
		myWarrantyRow
		Plate     *string `json:"plate,omitempty"`
		ServiceNo string  `json:"service_no,omitempty"`
		Dealer    string  `json:"dealer,omitempty"`
	}
	out := make([]row, 0, len(rows))
	for _, w := range rows {
		days, _ := portaluc.TimeLeft(w.StartAt, w.EndAt, env.Now)
		if w.Status != warrantyuc.StatusActive {
			days = 0
		}
		r := row{myWarrantyRow: myWarrantyRow{PublicCode: w.PublicCode, Product: dataText(w.Product.Name, maxNameChars),
			Status: w.Status, StartAt: w.StartAt, EndAt: w.EndAt, DaysLeft: days},
			Plate: w.Vehicle.Plate, ServiceNo: w.Service.ServiceNo, Dealer: dataText(w.Organization.Name, maxNameChars)}
		out = append(out, r)
	}
	return JSONResult(NewList(out, total))
}

// MyProfileLink: portal profil linki.
type MyProfileLink struct{ links LinkMaker }

// Spec implements Tool.
func (MyProfileLink) Spec() Spec {
	return customerSpec("my_profile_link",
		"Get a short link to the customer portal, where the customer sees their vehicles, services, warranties "+
			"and appointments and manages their preferences.",
		object(map[string]any{}))
}

// Run implements Tool.
func (t MyProfileLink) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	if r := decode(t.Spec(), raw, &struct{}{}); r != nil {
		return *r, nil
	}
	u, err := shortLink(ctx, t.links, env.Principal, "/portal")
	if err != nil {
		return Result{}, err
	}
	return JSONResult(struct {
		URL string `json:"portal_url"`
	}{u})
}

// MyAppointments: randevularım.
type MyAppointments struct{ appointments PortalAppointmentsReader }

// Spec implements Tool.
func (MyAppointments) Spec() Spec {
	return customerSpec("my_appointments",
		"List the customer's appointments at dealers: date and time, status, dealer and vehicle plate. "+
			"Default upcoming; past or all on request.",
		object(map[string]any{
			"period": enum("upcoming (default), past or all.", "upcoming", "past", "all"),
			"limit":  limitProp(),
		}))
}

// Run implements Tool.
func (t MyAppointments) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Period string `json:"period"`
		Limit  int    `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	p := env.Principal
	rows, total, err := t.appointments.PortalList(ctx,
		appointmentsuc.PortalCaller{UserID: p.Auth.UserInternal, BrandID: p.brandID()},
		appointmentsuc.PortalListFilter{Period: in.Period, Limit: limitArg(in.Limit)})
	if err != nil {
		return errCases{tool: t.Spec().Name, what: "appointment", notFound: []error{appointmentsuc.ErrNotFound},
			invalid: asError[*appointmentsuc.ValidationError]}.result(err)
	}
	type row struct {
		StartsAt     time.Time `json:"starts_at"`
		EndsAt       time.Time `json:"ends_at"`
		Status       string    `json:"status"`
		Dealer       string    `json:"dealer"`
		Plate        *string   `json:"plate,omitempty"`
		CancelReason *string   `json:"cancel_reason,omitempty"`
	}
	out := make([]row, 0, len(rows))
	for _, a := range rows {
		out = append(out, row{StartsAt: a.StartsAt, EndsAt: a.EndsAt, Status: a.Status,
			Dealer: dataText(a.DealerName, maxNameChars), Plate: a.VehiclePlate,
			CancelReason: textPtr(a.CancelReason, maxTextChars)})
	}
	return JSONResult(NewList(out, total))
}

// MyWarrantyClaims: garanti talebi durumum (yalnız durum + tarih).
type MyWarrantyClaims struct{ claims PortalClaimsReader }

// Spec implements Tool.
func (MyWarrantyClaims) Spec() Spec {
	return customerSpec("my_warranty_claims",
		"List the status of the customer's warranty claims, newest first: status, opened and last updated dates "+
			"only. Decisions and notes are not shared here; the customer follows the claim in the portal.",
		object(map[string]any{"limit": limitProp()}))
}

// Run implements Tool.
func (t MyWarrantyClaims) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Limit int `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	p := env.Principal
	rows, err := t.claims.PortalList(ctx, p.brandID(), p.Auth.UserInternal)
	if err != nil {
		return Result{}, err
	}
	type row struct {
		Status    string    `json:"status"`
		OpenedAt  time.Time `json:"opened_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	limit := int(limitArg(in.Limit))
	out := make([]row, 0, min(limit, len(rows)))
	for _, c := range rows {
		if len(out) == limit {
			break
		}
		out = append(out, row{Status: c.Status, OpenedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt})
	}
	return JSONResult(NewList(out, int64(len(rows))))
}

// ChangeLanguage: dil değiştir (users.locale; kişisel tercih, onaysız).
type ChangeLanguage struct{ profile ProfileWriter }

// supportedLocales are the i18n locales the tool accepts.
func supportedLocales() []string {
	out := make([]string, 0, len(i18n.Supported))
	for _, l := range i18n.Supported {
		out = append(out, string(l))
	}
	return out
}

// Spec implements Tool.
func (ChangeLanguage) Spec() Spec {
	s := customerSpec("change_language",
		"Change the customer's preferred language (saved on their account, also used by the portal and "+
			"notifications). Call only when the customer explicitly asks to switch language; then answer in it.",
		object(map[string]any{"locale": enum("The new language.", supportedLocales()...)}, "locale"))
	s.Kind = KindSelf
	return s
}

// Run implements Tool.
func (t ChangeLanguage) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Locale string `json:"locale"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	me, err := t.profile.UpdateProfile(ctx, env.Principal.Auth.UserID, nil, nil, authmodel.ProfilePatch{Locale: &in.Locale})
	if errors.Is(err, i18n.ErrInvalidLocale) {
		return invalidArg("unsupported locale"), nil
	}
	if err != nil {
		return Result{}, err
	}
	return JSONResult(struct {
		Locale string `json:"locale"`
		Note   string `json:"note"`
	}{Locale: me.EffectiveLocale, Note: "Saved. Continue the conversation in this language."})
}
