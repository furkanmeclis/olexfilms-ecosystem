package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	accountinguc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	appointmentsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	catalogmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	customersuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	leadsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	ordersuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	stockmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	stockuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	tasksuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	warrantyuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

// Fakes: every reader answers one canned row.

type fakeTree struct{}

func (fakeTree) Descendants(context.Context, int64) ([]db.Organization, error) { return nil, nil }

var (
	orgUUID     = uuid.MustParse("00000000-0000-0000-0000-000000000010")
	productUUID = uuid.MustParse("00000000-0000-0000-0000-000000000020")
	serviceUUID = uuid.MustParse("00000000-0000-0000-0000-000000000030")
)

type fakeServices struct{}

func (fakeServices) List(context.Context, svcuc.Caller, svcuc.ListFilter) ([]svcuc.ServiceView, int64, error) {
	return []svcuc.ServiceView{{UUID: serviceUUID, ServiceNo: "DSAB12CD34", Status: "completed"}}, 1, nil
}

func (fakeServices) Get(_ context.Context, _ svcuc.Caller, id uuid.UUID) (svcuc.ServiceView, error) {
	if id != serviceUUID {
		return svcuc.ServiceView{}, svcuc.ErrNotFound
	}
	return svcuc.ServiceView{UUID: id, ServiceNo: "DSAB12CD34", Status: "completed"}, nil
}

func (fakeServices) ActivitySummary(_ context.Context, _ svcuc.Caller, from, to time.Time) (svcuc.Activity, error) {
	return svcuc.Activity{From: from, To: to, CompletedCount: 3}, nil
}

type fakeWarranties struct{}

func (fakeWarranties) List(context.Context, warrantyuc.Caller, warrantyuc.ListFilter) ([]warrantyuc.WarrantyListView, int64, error) {
	return []warrantyuc.WarrantyListView{{PublicCode: "W1", Status: warrantyuc.StatusActive}}, 1, nil
}

type fakeCustomers struct{}

func (fakeCustomers) ListCustomers(context.Context, customersuc.Caller, customersuc.ListFilter) ([]customersuc.CustomerSummary, int64, error) {
	return []customersuc.CustomerSummary{{Name: "Ada"}}, 1, nil
}

type fakeStock struct{}

func (fakeStock) OrganizationStock(context.Context, scopefilter.Filter, uuid.UUID, stockmodel.StockFilter) ([]stockmodel.ProductStock, int64, error) {
	return []stockmodel.ProductStock{{Product: stockmodel.StockProduct{UUID: productUUID, Name: "Film"}, Quantity: 2}}, 1, nil
}

func (fakeStock) OrganizationUnits(context.Context, stockuc.UnitViewer, uuid.UUID, stockmodel.UnitFilter) ([]stockmodel.StockUnitRow, int64, error) {
	return []stockmodel.StockUnitRow{{
		Barcode: "B1", Status: "available", Quantity: 1, Product: stockmodel.ProductRef{UUID: productUUID, Name: "Film"},
		PurchasePrice: &stockmodel.UnitPurchasePrice{Amount: "120.00", Currency: "TRY", Source: "distributor"},
	}}, 1, nil
}

type fakeOrders struct{}

func (fakeOrders) order() ordersuc.OrderView {
	return ordersuc.OrderView{UUID: serviceUUID, OrderNo: "SIP-1", Role: string(ordersuc.PartyBuyer),
		Currency: "TRY", Subtotal: "100.00", TaxTotal: "20.00", Total: "120.00",
		Items: []ordersuc.ItemView{{UnitPrice: "100.00", LineTotal: "100.00"}}}
}

func (f fakeOrders) List(context.Context, ordersuc.Caller, ordersuc.ListFilter) ([]ordersuc.OrderView, int64, error) {
	return []ordersuc.OrderView{f.order()}, 1, nil
}

func (f fakeOrders) Get(context.Context, ordersuc.Caller, uuid.UUID) (ordersuc.OrderView, error) {
	return f.order(), nil
}

type fakeAccounting struct{}

func (fakeAccounting) GetBalanceReport(context.Context, accountinguc.Caller, *uuid.UUID, *time.Time) (accountinguc.BalanceReport, error) {
	return accountinguc.BalanceReport{Currency: "TRY", Cari: []accountinguc.CariBalance{
		{Counterparty: accountinguc.Counterparty{Name: "Small"}, Balance: "10.00"},
		{Counterparty: accountinguc.Counterparty{Name: "Big"}, Balance: "-500.00"},
	}}, nil
}

type fakeAppointments struct{ got appointmentsuc.ListFilter }

func (f *fakeAppointments) List(_ context.Context, _ appointmentsuc.Caller, lf appointmentsuc.ListFilter) ([]appointmentsuc.Appointment, int64, error) {
	f.got = lf
	return []appointmentsuc.Appointment{{Status: appointmentsuc.StatusScheduled}}, 1, nil
}

type fakeLeads struct{}

func (fakeLeads) List(context.Context, leadsuc.Caller, leadsuc.ListFilter) ([]leadsuc.Lead, int64, error) {
	return []leadsuc.Lead{{Status: leadsuc.StatusNew}}, 1, nil
}

type fakeTasks struct{}

func (fakeTasks) List(context.Context, tasksuc.Caller, tasksuc.Filter) ([]tasksuc.Task, int64, error) {
	return []tasksuc.Task{{Title: "Call"}}, 1, nil
}

type fakeCatalog struct{}

func (fakeCatalog) ListProducts(context.Context, orgctx.Scope, catalogmodel.ProductFilter) ([]catalogmodel.Product, int64, error) {
	return []catalogmodel.Product{{UUID: productUUID, SKU: "F1", Name: "Film"}}, 1, nil
}

type fakeOrgs struct{}

func (fakeOrgs) ListInScope(context.Context, scopefilter.Filter, orgusecase.ScopedListInput) ([]orgusecase.Organization, error) {
	return []orgusecase.Organization{{UUID: uuid.New(), Slug: "d1", Name: "Dealer", Type: "dealer"}}, nil
}

type fakeFeatures map[string]bool

func (f fakeFeatures) Enabled(_ context.Context, _ int64, key string) (bool, error) {
	on, set := f[key]
	return !set || on, nil
}

type fakeToggles map[string]bool

func (f fakeToggles) ToolToggles(context.Context) (map[string]bool, error) { return f, nil }

func testRegistry(fs FeatureChecker) *Registry {
	r := NewRegistry(fs).WithClock(func() time.Time { return time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC) }, nil)
	RegisterPanel(r, Deps{
		Tree: fakeTree{}, Services: fakeServices{}, Warranties: fakeWarranties{}, Customers: fakeCustomers{},
		Stock: fakeStock{}, Orders: fakeOrders{}, Accounting: fakeAccounting{}, Appointments: &fakeAppointments{},
		Leads: fakeLeads{}, Tasks: fakeTasks{}, Catalog: fakeCatalog{}, Organizations: fakeOrgs{},
	})
	return r
}

func principal(orgType string, scopes map[string]rbac.Scope) Principal {
	return Principal{
		Auth:  authctx.Principal{UserID: uuid.New(), UserInternal: 7, PermissionScopes: scopes},
		Org:   &orgctx.Scope{InternalID: 10, UUID: orgUUID, OrgType: orgType, BrandID: 1, BrandSlug: "olex"},
		Realm: RealmPanel,
	}
}

func superAdmin() Principal {
	p := principal(OrgCenter, nil)
	p.Auth.IsSuperAdmin = true
	return p
}

func call(t *testing.T, r *Registry, p Principal, name, input string) Result {
	t.Helper()
	res, err := r.Call(context.Background(), p, name, json.RawMessage(input))
	if err != nil && !errors.Is(err, ErrToolNotAllowed) {
		t.Fatalf("%s(%s): %v", name, input, err)
	}
	return res
}

func required(spec Spec) []string {
	out, _ := spec.InputSchema["required"].([]string)
	return out
}

func props(spec Spec) map[string]any {
	p, _ := spec.InputSchema["properties"].(map[string]any)
	return p
}

// validValue builds a value that passes a property schema.
func validValue(schema map[string]any) any {
	if e, ok := schema["enum"].([]string); ok {
		return e[0]
	}
	switch schema["type"] {
	case "integer":
		return 5
	case "boolean":
		return true
	}
	if schema["format"] == "date" {
		return "2026-10-01"
	}
	if strings.Contains(fmt.Sprint(schema["description"]), "uuid") {
		return serviceUUID.String()
	}
	return "DSAB12CD34"
}

// wrongValue builds a value of the wrong JSON type for a property.
func wrongValue(schema map[string]any) any {
	if schema["type"] == "string" {
		return 42
	}
	return "x"
}

// TEC-385 acceptance: every tool validates its schema. A missing required
// field, a wrong type, an unknown property or broken JSON answer an
// INVALID_INPUT result the model can act on (naming the field), never a
// panic or an internal error; a valid input runs.
func TestEveryToolValidatesItsSchema(t *testing.T) {
	r := testRegistry(nil)
	p := superAdmin()
	all := r.All()
	if len(all) != 15 {
		t.Fatalf("registered tools = %d, want 15", len(all))
	}
	for _, tool := range all {
		spec := tool.Spec()
		t.Run(spec.Name, func(t *testing.T) {
			if spec.Description == "" || spec.Kind != KindRead || spec.Realm != RealmPanel || len(spec.Permissions) == 0 {
				t.Fatalf("incomplete spec: %+v", spec)
			}
			if spec.InputSchema["type"] != "object" || spec.InputSchema["additionalProperties"] != false {
				t.Fatalf("schema must be a closed object: %v", spec.InputSchema)
			}
			invalid := func(input, want string) {
				t.Helper()
				res := call(t, r, p, spec.Name, input)
				if !res.IsError || res.Code != CodeInvalidInput || !strings.Contains(res.Content, want) {
					t.Fatalf("input %s: want INVALID_INPUT containing %q, got %+v", input, want, res)
				}
			}
			valid := map[string]any{}
			for _, f := range required(spec) {
				schema, _ := props(spec)[f].(map[string]any)
				valid[f] = validValue(schema)
				// Each required field missing on its own.
				without := map[string]any{}
				for k, v := range valid {
					if k != f {
						without[k] = v
					}
				}
				raw, _ := json.Marshal(without)
				invalid(string(raw), f+" is required")
				invalid("", f+" is required")
				invalid(`{"`+f+`":null}`, f+" is required")
			}
			for name, s := range props(spec) {
				schema := s.(map[string]any)
				bad := map[string]any{}
				for k, v := range valid {
					bad[k] = v
				}
				bad[name] = wrongValue(schema)
				raw, _ := json.Marshal(bad)
				invalid(string(raw), name+" must be")
			}
			extra := map[string]any{"unexpected_field": 1}
			for k, v := range valid {
				extra[k] = v
			}
			rawExtra, _ := json.Marshal(extra)
			invalid(string(rawExtra), "unexpected_field is not an allowed property")
			invalid(`{"limit":`, "not valid JSON")
			invalid(`[]`, "must be an object")

			raw, _ := json.Marshal(valid)
			res := call(t, r, p, spec.Name, string(raw))
			if res.IsError {
				t.Fatalf("valid input %s: %+v", raw, res)
			}
			if !json.Valid([]byte(res.Content)) {
				t.Fatalf("result is not JSON: %s", res.Content)
			}
		})
	}
}

// TEC-385 acceptance: a dealer staff without pricing.purchase.read gets no
// purchase price in the stock tool output, even when the use case returns
// one; with the grant it is there.
func TestStockUnitsPurchasePriceNeedsPermission(t *testing.T) {
	r := testRegistry(nil)
	staff := principal(OrgDealer, map[string]rbac.Scope{rbac.PermStockRead: rbac.ScopeManaged})
	res := call(t, r, staff, "stock_units", `{}`)
	if res.IsError || !strings.Contains(res.Content, `"barcode":"B1"`) || strings.Contains(res.Content, "purchase_price") ||
		strings.Contains(res.Content, "120.00") {
		t.Fatalf("staff without pricing.purchase.read: %+v", res)
	}
	// pricing.sale.read alone does not reveal the own organization's
	// purchase price.
	staff.Auth.PermissionScopes[rbac.PermPricingSaleRead] = rbac.ScopeManaged
	if res := call(t, r, staff, "stock_units", `{}`); strings.Contains(res.Content, "purchase_price") {
		t.Fatalf("sale.read revealed the own purchase price: %s", res.Content)
	}
	// It does for a sub-organization (the parent's own sale price).
	if res := call(t, r, staff, "stock_units", `{"organization":"`+uuid.NewString()+`"}`); !strings.Contains(res.Content, `"purchase_price":{"amount":"120.00"`) {
		t.Fatalf("parent sale.read on a child: %s", res.Content)
	}
	owner := principal(OrgDealer, map[string]rbac.Scope{
		rbac.PermStockRead: rbac.ScopeManaged, rbac.PermPricingPurchaseRead: rbac.ScopeManaged,
	})
	if res := call(t, r, owner, "stock_units", `{}`); !strings.Contains(res.Content, `"purchase_price":{"amount":"120.00","currency":"TRY"}`) {
		t.Fatalf("owner with pricing.purchase.read: %s", res.Content)
	}
}

// Order totals and line prices are the buyer's purchase price.
func TestOrderPricesNeedPermission(t *testing.T) {
	r := testRegistry(nil)
	staff := principal(OrgDealer, map[string]rbac.Scope{rbac.PermOrdersRead: rbac.ScopeManaged})
	for _, name := range []string{"list_orders", "get_order"} {
		input := `{}`
		if name == "get_order" {
			input = `{"order":"` + serviceUUID.String() + `"}`
		}
		res := call(t, r, staff, name, input)
		if res.IsError || strings.Contains(res.Content, "120.00") || strings.Contains(res.Content, "100.00") ||
			strings.Contains(res.Content, `"total":"`) || strings.Contains(res.Content, "currency") {
			t.Fatalf("%s without pricing: %+v", name, res)
		}
	}
	staff.Auth.PermissionScopes[rbac.PermPricingPurchaseRead] = rbac.ScopeManaged
	res := call(t, r, staff, "get_order", `{"order":"SIP-1"}`)
	if !strings.Contains(res.Content, `"total":"120.00"`) || !strings.Contains(res.Content, `"unit_price":"100.00"`) {
		t.Fatalf("get_order with pricing.purchase.read: %s", res.Content)
	}
}

func names(ts []Tool) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Spec().Name)
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TEC-385 acceptance: Available and Call run the same check. A module
// switched off for the organization drops its tool from Available and a
// call answers TOOL_NOT_ALLOWED; so do a missing permission, a wrong
// organization type, another realm, a platform toggle and an unknown name.
func TestAvailableAndCallShareTheGate(t *testing.T) {
	ctx := context.Background()
	dealer := principal(OrgDealer, map[string]rbac.Scope{
		rbac.PermServicesRead: rbac.ScopeManaged, rbac.PermLeadsRead: rbac.ScopeManaged,
		rbac.PermTasksRead: rbac.ScopeBrand, rbac.PermOrganizationsRead: rbac.ScopeManaged,
	})
	notAllowedCall := func(t *testing.T, r *Registry, p Principal, name string) {
		t.Helper()
		res, err := r.Call(ctx, p, name, json.RawMessage(`{}`))
		if !errors.Is(err, ErrToolNotAllowed) || !res.IsError || res.Code != CodeToolNotAllowed ||
			!strings.Contains(res.Content, "TOOL_NOT_ALLOWED") {
			t.Fatalf("%s: want TOOL_NOT_ALLOWED, got %+v %v", name, res, err)
		}
	}

	on := testRegistry(fakeFeatures{})
	got, err := on.Available(ctx, dealer)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"get_service", "list_leads", "search_services", "service_activity_summary"}
	if fmt.Sprint(names(got)) != fmt.Sprint(want) {
		t.Fatalf("dealer tools = %v, want %v (no permission: stock; center only: my_tasks; distributor+: list_sub_organizations)", names(got), want)
	}
	if res := call(t, on, dealer, "list_leads", `{}`); res.IsError {
		t.Fatalf("list_leads with the module on: %+v", res)
	}
	for _, name := range []string{"stock_units", "my_tasks", "list_sub_organizations", "no_such_tool"} {
		notAllowedCall(t, on, dealer, name)
	}

	off := testRegistry(fakeFeatures{features.ModuleLeads: false})
	got, _ = off.Available(ctx, dealer)
	if contains(names(got), "list_leads") || !contains(names(got), "search_services") {
		t.Fatalf("leads off: %v", names(got))
	}
	notAllowedCall(t, off, dealer, "list_leads")

	toggled := testRegistry(nil).WithToggles(fakeToggles{"search_services": false, "get_service": true})
	got, _ = toggled.Available(ctx, dealer)
	if contains(names(got), "search_services") || !contains(names(got), "get_service") {
		t.Fatalf("toggled: %v", names(got))
	}
	notAllowedCall(t, toggled, dealer, "search_services")

	customer := dealer
	customer.Realm = RealmCustomer
	if got, _ := on.Available(ctx, customer); len(got) != 0 {
		t.Fatalf("panel tools offered to the customer realm: %v", names(got))
	}
	notAllowedCall(t, on, customer, "search_services")

	noOrg := dealer
	noOrg.Org = nil
	notAllowedCall(t, on, noOrg, "search_services")

	// K20: a request on another brand's domain reaches no tool.
	glorian := brandctx.WithBrand(ctx, brandctx.Brand{ID: 2, Slug: "glorian"})
	if got, _ := on.Available(glorian, dealer); len(got) != 0 {
		t.Fatalf("tools across brands: %v", names(got))
	}
	if _, err := on.Call(glorian, dealer, "search_services", json.RawMessage(`{"query":"34AB"}`)); !errors.Is(err, ErrToolNotAllowed) {
		t.Fatalf("call across brands: %v", err)
	}
}

type stubTool struct {
	spec Spec
	run  func() (Result, error)
}

func (s stubTool) Spec() Spec { return s.spec }

func (s stubTool) Run(context.Context, Env, json.RawMessage) (Result, error) { return s.run() }

func TestCallRecoversPanicsAndCapsResults(t *testing.T) {
	spec := func(name string) Spec {
		return Spec{Name: name, Description: "x", InputSchema: object(map[string]any{}), Kind: KindRead, Realm: RealmPanel}
	}
	r := NewRegistry(nil).WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.Register(stubTool{spec: spec("boom"), run: func() (Result, error) { panic("nil map") }})
	r.Register(stubTool{spec: spec("big"), run: func() (Result, error) {
		return Result{Content: strings.Repeat("x", MaxResultBytes+1)}, nil
	}})
	r.Register(stubTool{spec: spec("fails"), run: func() (Result, error) { return Result{}, errors.New("db down") }})
	p := superAdmin()

	res, err := r.Call(context.Background(), p, "boom", nil)
	if err == nil || !res.IsError || res.Code != CodeToolFailed || strings.Contains(res.Content, "nil map") {
		t.Fatalf("panic: %+v %v", res, err)
	}
	res, err = r.Call(context.Background(), p, "fails", nil)
	if err == nil || res.Code != CodeToolFailed || strings.Contains(res.Content, "db down") {
		t.Fatalf("internal error leaked: %+v %v", res, err)
	}
	res, err = r.Call(context.Background(), p, "big", nil)
	if err != nil || !res.IsError || res.Code != CodeTooLarge || !strings.Contains(res.Content, "Narrow") {
		t.Fatalf("cap: %+v %v", res, err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate tool name must panic at registration")
		}
	}()
	r.Register(stubTool{spec: spec("big")})
}

func TestListEnvelopeAsksToNarrow(t *testing.T) {
	l := NewList([]int{1, 2}, 120)
	if !l.Truncated || l.Returned != 2 || !strings.Contains(l.Hint, "Narrow") {
		t.Fatalf("truncated list: %+v", l)
	}
	if l := NewList[int](nil, 0); l.Truncated || l.Items == nil || l.Hint != "" {
		t.Fatalf("empty list: %+v", l)
	}
	if limitArg(0) != DefaultRows || limitArg(500) != MaxRows || limitArg(7) != 7 {
		t.Fatal("limitArg")
	}
}

func TestBalanceSummarySortsByAbsoluteBalance(t *testing.T) {
	r := testRegistry(nil)
	p := principal(OrgDealer, map[string]rbac.Scope{rbac.PermAccountingRead: rbac.ScopeManaged})
	res := call(t, r, p, "balance_summary", `{"limit":1}`)
	if res.IsError || !strings.Contains(res.Content, `"Big"`) || strings.Contains(res.Content, `"Small"`) ||
		!strings.Contains(res.Content, `"total":2`) {
		t.Fatalf("balance summary: %s", res.Content)
	}
}

func TestAppointmentsWeekRange(t *testing.T) {
	fa := &fakeAppointments{}
	r := NewRegistry(nil).WithClock(func() time.Time { return time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC) }, time.UTC)
	RegisterPanel(r, Deps{Tree: fakeTree{}, Appointments: fa})
	p := principal(OrgDealer, map[string]rbac.Scope{rbac.PermAppointmentsRead: rbac.ScopeManaged})
	if res := call(t, r, p, "list_appointments", `{"range":"week"}`); res.IsError {
		t.Fatalf("week: %+v", res)
	}
	// 2026-10-07 is a Wednesday: Monday 5th to Monday 12th.
	if fa.got.From.Format(time.DateOnly) != "2026-10-05" || fa.got.To.Format(time.DateOnly) != "2026-10-12" {
		t.Fatalf("week range %v - %v", fa.got.From, fa.got.To)
	}
}

func TestDataTextFlattensStoredText(t *testing.T) {
	if got := dataText("line1\n\n  line2\t\x00end", 100); got != "line1 line2 end" {
		t.Fatalf("dataText = %q", got)
	}
	if got := dataText(strings.Repeat("a", 10), 4); got != "aaaa…" {
		t.Fatalf("clip = %q", got)
	}
}
