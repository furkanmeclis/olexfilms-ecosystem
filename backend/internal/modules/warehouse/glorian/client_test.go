package glorian_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
)

// fakeClock records waits instead of sleeping.
type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
	return ctx.Err()
}

func newClient(t *testing.T, srv *fake.Server, clock *fakeClock) *glorian.HTTPClient {
	t.Helper()
	c, err := glorian.NewHTTPClient(glorian.Options{
		BaseURL:    srv.URL,
		APIKey:     fake.APIKey,
		HTTPClient: srv.Client(),
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}
	return c
}

func assertContractHeaders(t *testing.T, r fake.Request, wantJSONBody bool) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer "+fake.APIKey {
		t.Errorf("%s %s Authorization = %q", r.Method, r.Path, got)
	}
	if got := r.Header.Get(glorian.HeaderAPIKey); got != fake.APIKey {
		t.Errorf("%s %s %s = %q", r.Method, r.Path, glorian.HeaderAPIKey, got)
	}
	if got := r.Header.Get(glorian.HeaderAPIVersion); got != "1" {
		t.Errorf("%s %s %s = %q, want 1", r.Method, r.Path, glorian.HeaderAPIVersion, got)
	}
	if got := r.Header.Get("Accept"); got != "application/json" {
		t.Errorf("%s %s Accept = %q", r.Method, r.Path, got)
	}
	ct := r.Header.Get("Content-Type")
	if wantJSONBody && ct != "application/json" {
		t.Errorf("%s %s Content-Type = %q", r.Method, r.Path, ct)
	}
	if !wantJSONBody && len(r.Body) != 0 {
		t.Errorf("%s %s unexpected body %s", r.Method, r.Path, r.Body)
	}
}

func onlyRequest(t *testing.T, srv *fake.Server) fake.Request {
	t.Helper()
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	return reqs[0]
}

func TestContract_ListEndpoints(t *testing.T) {
	since := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	params := glorian.ListParams{UpdatedSince: since, PerPage: 25, Cursor: "1"}

	cases := []struct {
		name      string
		path      string
		call      func(c glorian.InventoryClient) (int, error)
		wantItems int
	}{
		{"categories", "/product-categories", func(c glorian.InventoryClient) (int, error) {
			p, err := c.ListCategories(context.Background(), params)
			return len(p.Items), err
		}, 2},
		{"products", "/products", func(c glorian.InventoryClient) (int, error) {
			p, err := c.ListProducts(context.Background(), params)
			return len(p.Items), err
		}, 2},
		{"dealers", "/dealers", func(c glorian.InventoryClient) (int, error) {
			p, err := c.ListDealers(context.Background(), params)
			return len(p.Items), err
		}, 1},
		{"stock items", "/stock-items", func(c glorian.InventoryClient) (int, error) {
			p, err := c.ListStockItems(context.Background(), params)
			return len(p.Items), err
		}, 2},
		{"orders", "/orders", func(c glorian.InventoryClient) (int, error) {
			p, err := c.ListOrders(context.Background(), params)
			return len(p.Items), err
		}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.New(t)
			n, err := tc.call(newClient(t, srv, newClock()))
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if n != tc.wantItems {
				t.Errorf("items = %d, want %d", n, tc.wantItems)
			}
			r := onlyRequest(t, srv)
			if r.Method != http.MethodGet || r.Path != tc.path {
				t.Errorf("request = %s %s, want GET %s", r.Method, r.Path, tc.path)
			}
			if got := r.Query.Get("updated_since"); got != "2026-09-02T00:00:00Z" {
				t.Errorf("updated_since = %q", got)
			}
			if r.Query.Get("page") != "1" || r.Query.Get("per_page") != "25" {
				t.Errorf("pagination query = %v", r.Query)
			}
			assertContractHeaders(t, r, false)
		})
	}
}

func TestContract_ListDecodesResourcesAndFollowsCursor(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())
	ctx := context.Background()

	first, err := c.ListProducts(ctx, glorian.ListParams{PerPage: 2, Filters: map[string]string{"category_id": "1"}})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(first.Items) != 2 || first.NextCursor != "" {
		t.Fatalf("filtered page = %d items, next %q", len(first.Items), first.NextCursor)
	}
	if p := first.Items[0]; p.ID != "10" || p.SKU != "HYDRA-150" || p.MicronThickness == nil || *p.MicronThickness != 150 {
		t.Errorf("product decode = %+v", p)
	}

	all, err := c.ListProducts(ctx, glorian.ListParams{PerPage: 2})
	if err != nil {
		t.Fatalf("unfiltered: %v", err)
	}
	if all.NextCursor != "2" || all.Pagination.Total != 3 || all.Pagination.LastPage != 2 {
		t.Fatalf("pagination = %+v next %q", all.Pagination, all.NextCursor)
	}
	second, err := c.ListProducts(ctx, glorian.ListParams{PerPage: 2, Cursor: all.NextCursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].SKU != "TINT-35" || second.NextCursor != "" {
		t.Errorf("page 2 = %+v next %q", second.Items, second.NextCursor)
	}

	dealers, err := c.ListDealers(ctx, glorian.ListParams{})
	if err != nil {
		t.Fatalf("dealers: %v", err)
	}
	if d := dealers.Items[0]; d.DealerCode == nil || *d.DealerCode != "AB12CD34" || d.Latitude == nil {
		t.Errorf("dealer decode = %+v", d)
	}

	if _, err := c.ListProducts(ctx, glorian.ListParams{Cursor: "abc"}); !errors.Is(err, glorian.ErrInvalidInput) {
		t.Errorf("bad cursor err = %v, want ErrInvalidInput", err)
	}
}

func TestContract_UpsertBarcodes(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())

	res, err := c.UpsertBarcodes(context.Background(), []glorian.BarcodeUpsert{
		{Barcode: "GLR-0001", ProductID: "10"},
		{Barcode: "GLR-0002", ProductID: "10"},
		{Barcode: "GLR-NEW", ProductID: "11", Location: glorian.StockLocationCenter, Status: glorian.StockStatusAvailable},
	})
	if err != nil {
		t.Fatalf("UpsertBarcodes: %v", err)
	}
	if len(res.Created) != 1 || len(res.Updated) != 1 || len(res.Conflicts) != 1 || res.Conflicts[0].Barcode != "GLR-0002" {
		t.Errorf("result = %+v", res)
	}

	r := onlyRequest(t, srv)
	if r.Method != http.MethodPost || r.Path != "/stock-items/bulk" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	assertContractHeaders(t, r, true)
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := r.JSON(&body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if len(body.Items) != 3 {
		t.Fatalf("items = %d", len(body.Items))
	}
	if _, ok := body.Items[0]["location"]; ok {
		t.Errorf("empty location must be omitted so the hub defaults it: %v", body.Items[0])
	}
	if body.Items[2]["barcode"] != "GLR-NEW" || body.Items[2]["product_id"] != "11" || body.Items[2]["status"] != "available" {
		t.Errorf("item 2 = %v", body.Items[2])
	}
	if _, ok := srv.StockItem("GLR-NEW"); !ok {
		t.Error("fake did not store the created barcode")
	}
}

func TestContract_UpsertBarcodesRejectsOversizedBatchLocally(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())
	items := make([]glorian.BarcodeUpsert, glorian.MaxBulkItems+1)
	for i := range items {
		items[i] = glorian.BarcodeUpsert{Barcode: "B", ProductID: "1"}
	}
	if _, err := c.UpsertBarcodes(context.Background(), items); !errors.Is(err, glorian.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("requests = %d, want 0", n)
	}
}

func TestContract_PatchStockItemByBarcode(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())

	item, err := c.PatchStockItemByBarcode(context.Background(), "GLR-0001",
		glorian.StockItemPatch{Status: glorian.StockStatusExternalOutbound, Location: glorian.StockLocationCenter})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if item.Status != glorian.StockStatusExternalOutbound || item.Barcode != "GLR-0001" {
		t.Errorf("item = %+v", item)
	}
	r := onlyRequest(t, srv)
	if r.Method != http.MethodPatch || r.Path != "/stock-items/by-barcode/GLR-0001" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	assertContractHeaders(t, r, true)
	if got := string(r.Body); got != `{"status":"external_outbound","location":"center"}` {
		t.Errorf("body = %s", got)
	}

	// reserved -> 409 envelope -> typed conflict.
	_, err = c.PatchStockItemByBarcode(context.Background(), "GLR-0002", glorian.StockItemPatch{Status: glorian.StockStatusExternalOutbound})
	var apiErr *glorian.APIError
	if !errors.As(err, &apiErr) || !errors.Is(err, glorian.ErrConflict) || apiErr.Status != http.StatusConflict {
		t.Fatalf("err = %v, want 409 ErrConflict", err)
	}
	if apiErr.Details["barcode"] != "GLR-0002" {
		t.Errorf("details = %v", apiErr.Details)
	}
}

func TestContract_CreateOrder(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())
	in := glorian.CreateOrderInput{
		DealerID:          "1",
		ExternalReference: "wh-order-0001",
		Items:             []glorian.OrderItemInput{{ProductID: "10", Quantity: 1, Barcodes: []string{"GLR-0001"}}},
	}

	order, err := c.CreateOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.ID == "" || order.Status != "processing" || order.ExternalReference == nil || *order.ExternalReference != "wh-order-0001" {
		t.Errorf("order = %+v", order)
	}
	if len(order.Items) != 1 || len(order.Items[0].StockItemIDs) != 1 || order.Items[0].StockItemIDs[0] != "99" {
		t.Errorf("order items = %+v", order.Items)
	}

	r := onlyRequest(t, srv)
	if r.Method != http.MethodPost || r.Path != "/orders" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	assertContractHeaders(t, r, true)
	if got := r.Header.Get(glorian.HeaderIdempotencyKey); got != "wh-order-0001" {
		t.Errorf("Idempotency-Key = %q", got)
	}
	var body map[string]any
	if err := r.JSON(&body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["dealer_id"] != "1" || body["external_reference"] != "wh-order-0001" {
		t.Errorf("body = %v", body)
	}
	if _, ok := body["auto_prepare"]; ok {
		t.Errorf("nil auto_prepare must be omitted (hub default true): %v", body)
	}

	// Replay with the same external_reference returns the same order.
	again, err := c.CreateOrder(context.Background(), in)
	if err != nil || again.ID != order.ID {
		t.Errorf("replay = %+v, %v; want id %s", again, err, order.ID)
	}
}

func TestContract_TransitionOrder(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())
	ctx := context.Background()
	order, err := c.CreateOrder(ctx, glorian.CreateOrderInput{
		DealerID: "1", ExternalReference: "wh-order-0002",
		Items: []glorian.OrderItemInput{{ProductID: "10", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	shipped, err := c.TransitionOrder(ctx, order.ID, glorian.OrderActionShip,
		glorian.TransitionInput{CargoCompany: "Yurtiçi", TrackingNumber: "TRK1"})
	if err != nil {
		t.Fatalf("ship: %v", err)
	}
	if shipped.Status != "shipped" || shipped.CargoCompany == nil || *shipped.CargoCompany != "Yurtiçi" {
		t.Errorf("shipped = %+v", shipped)
	}
	reqs := srv.RequestsTo(http.MethodPost, "/orders/"+order.ID+"/ship")
	if len(reqs) != 1 {
		t.Fatalf("ship requests = %d", len(reqs))
	}
	assertContractHeaders(t, reqs[0], true)
	if got := string(reqs[0].Body); got != `{"cargo_company":"Yurtiçi","tracking_number":"TRK1"}` {
		t.Errorf("ship body = %s", got)
	}
	if reqs[0].Header.Get(glorian.HeaderIdempotencyKey) != "" {
		t.Error("transition must not carry an Idempotency-Key")
	}

	for _, a := range []glorian.OrderAction{glorian.OrderActionDeliver, glorian.OrderActionReceive} {
		if _, err := c.TransitionOrder(ctx, order.ID, a, glorian.TransitionInput{}); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		rs := srv.RequestsTo(http.MethodPost, "/orders/"+order.ID+"/"+string(a))
		if len(rs) != 1 || string(rs[0].Body) != "{}" {
			t.Errorf("%s requests = %d body %q", a, len(rs), rs[0].Body)
		}
	}
	// A delivered order cannot be cancelled (hub rule, mirrored by the fake).
	if _, err := c.TransitionOrder(ctx, order.ID, glorian.OrderActionCancel, glorian.TransitionInput{}); !errors.Is(err, glorian.ErrValidation) {
		t.Errorf("cancel after deliver err = %v; want validation", err)
	}
	other, err := c.CreateOrder(ctx, glorian.CreateOrderInput{
		DealerID: "1", ExternalReference: "wh-order-0003",
		Items: []glorian.OrderItemInput{{ProductID: "10", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	for _, a := range []glorian.OrderAction{glorian.OrderActionPrepare, glorian.OrderActionCancel} {
		if _, err := c.TransitionOrder(ctx, other.ID, a, glorian.TransitionInput{}); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		rs := srv.RequestsTo(http.MethodPost, "/orders/"+other.ID+"/"+string(a))
		if len(rs) != 1 || string(rs[0].Body) != "{}" {
			t.Errorf("%s requests = %d body %q", a, len(rs), rs[0].Body)
		}
	}
	// Ship after cancel is refused; a second cancel is a no-op.
	if _, err := c.TransitionOrder(ctx, other.ID, glorian.OrderActionShip, glorian.TransitionInput{CargoCompany: "X"}); !errors.Is(err, glorian.ErrValidation) {
		t.Errorf("ship after cancel err = %v; want validation", err)
	}
	if again, err := c.TransitionOrder(ctx, other.ID, glorian.OrderActionCancel, glorian.TransitionInput{}); err != nil || again.Status != "cancelled" {
		t.Errorf("second cancel = %+v, %v", again, err)
	}

	if _, err := c.TransitionOrder(ctx, order.ID, "explode", glorian.TransitionInput{}); !errors.Is(err, glorian.ErrInvalidInput) {
		t.Errorf("unknown action err = %v", err)
	}
	if _, err := c.TransitionOrder(ctx, order.ID, glorian.OrderActionShip, glorian.TransitionInput{}); !errors.Is(err, glorian.ErrInvalidInput) {
		t.Errorf("ship without cargo err = %v", err)
	}
}

func TestRetry_429HonoursRetryAfterSeconds(t *testing.T) {
	srv := fake.New(t)
	clock := newClock()
	c := newClient(t, srv, clock)
	srv.Inject(http.MethodGet, "/products", fake.RateLimited("7"))

	page, err := c.ListProducts(context.Background(), glorian.ListParams{})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(page.Items) != 3 {
		t.Errorf("items = %d", len(page.Items))
	}
	if n := len(srv.RequestsTo(http.MethodGet, "/products")); n != 2 {
		t.Errorf("attempts = %d, want 2", n)
	}
	if len(clock.sleeps) != 1 || clock.sleeps[0] != 7*time.Second {
		t.Errorf("sleeps = %v, want [7s]", clock.sleeps)
	}
}

func TestRetry_429HonoursRetryAfterHTTPDate(t *testing.T) {
	srv := fake.New(t)
	clock := newClock()
	c := newClient(t, srv, clock)
	srv.Inject(http.MethodGet, "/dealers", fake.RateLimited(clock.now.Add(4*time.Second).Format(http.TimeFormat)))

	if _, err := c.ListDealers(context.Background(), glorian.ListParams{}); err != nil {
		t.Fatalf("ListDealers: %v", err)
	}
	if len(clock.sleeps) != 1 || clock.sleeps[0] != 4*time.Second {
		t.Errorf("sleeps = %v, want [4s]", clock.sleeps)
	}
}

func TestRetry_429BeyondMaxRetryAfterIsReturned(t *testing.T) {
	srv := fake.New(t)
	clock := newClock()
	c := newClient(t, srv, clock)
	srv.Inject(http.MethodGet, "/products", fake.RateLimited("3600"))

	_, err := c.ListProducts(context.Background(), glorian.ListParams{})
	var apiErr *glorian.APIError
	if !errors.Is(err, glorian.ErrRateLimited) || !errors.As(err, &apiErr) || apiErr.RetryAfter != time.Hour {
		t.Fatalf("err = %v, want ErrRateLimited with RetryAfter 1h", err)
	}
	if len(clock.sleeps) != 0 {
		t.Errorf("sleeps = %v, want none", clock.sleeps)
	}
}

func TestRetry_Get500TriesThreeTimesWithBackoff(t *testing.T) {
	srv := fake.New(t)
	clock := newClock()
	c := newClient(t, srv, clock)
	srv.Inject(http.MethodGet, "/stock-items", fake.Scenario{
		Status: http.StatusInternalServerError, Times: 5,
		Body: map[string]any{"success": false, "error": map[string]any{"code": "INTERNAL", "message": "boom"}},
	})

	_, err := c.ListStockItems(context.Background(), glorian.ListParams{})
	if !errors.Is(err, glorian.ErrServer) {
		t.Fatalf("err = %v, want ErrServer", err)
	}
	if n := len(srv.RequestsTo(http.MethodGet, "/stock-items")); n != 3 {
		t.Errorf("attempts = %d, want 3", n)
	}
	want := []time.Duration{200 * time.Millisecond, 400 * time.Millisecond}
	if len(clock.sleeps) != 2 || clock.sleeps[0] != want[0] || clock.sleeps[1] != want[1] {
		t.Errorf("sleeps = %v, want %v", clock.sleeps, want)
	}
}

func TestRetry_Get500RecoversWithinBudget(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())
	sc := fake.ServerError()
	sc.Times = 2
	srv.Inject(http.MethodGet, "/product-categories", sc)

	page, err := c.ListCategories(context.Background(), glorian.ListParams{})
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("ListCategories = %d items, %v", len(page.Items), err)
	}
	if n := len(srv.RequestsTo(http.MethodGet, "/product-categories")); n != 3 {
		t.Errorf("attempts = %d, want 3", n)
	}
}

func TestRetry_WritesTryOnce(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		call   func(c glorian.InventoryClient) error
	}{
		{"bulk upsert", http.MethodPost, "/stock-items/bulk", func(c glorian.InventoryClient) error {
			_, err := c.UpsertBarcodes(context.Background(), []glorian.BarcodeUpsert{{Barcode: "X", ProductID: "10"}})
			return err
		}},
		{"patch stock", http.MethodPatch, "/stock-items/by-barcode/GLR-0001", func(c glorian.InventoryClient) error {
			_, err := c.PatchStockItemByBarcode(context.Background(), "GLR-0001", glorian.StockItemPatch{Status: "external_outbound"})
			return err
		}},
		{"create order without reference", http.MethodPost, "/orders", func(c glorian.InventoryClient) error {
			_, err := c.CreateOrder(context.Background(), glorian.CreateOrderInput{
				DealerID: "1", Items: []glorian.OrderItemInput{{ProductID: "10", Quantity: 1}},
			})
			return err
		}},
		{"transition", http.MethodPost, "/orders/5/deliver", func(c glorian.InventoryClient) error {
			_, err := c.TransitionOrder(context.Background(), "5", glorian.OrderActionDeliver, glorian.TransitionInput{})
			return err
		}},
	}
	for _, tc := range cases {
		for _, sc := range []struct {
			name     string
			scenario fake.Scenario
			want     error
		}{
			{"500", fake.ServerError(), glorian.ErrServer},
			{"429", fake.RateLimited("1"), glorian.ErrRateLimited},
		} {
			t.Run(tc.name+"/"+sc.name, func(t *testing.T) {
				srv := fake.New(t)
				clock := newClock()
				c := newClient(t, srv, clock)
				s := sc.scenario
				s.Times = 5
				srv.Inject(tc.method, tc.path, s)

				if err := tc.call(c); !errors.Is(err, sc.want) {
					t.Fatalf("err = %v, want %v", err, sc.want)
				}
				if n := len(srv.RequestsTo(tc.method, tc.path)); n != 1 {
					t.Errorf("attempts = %d, want 1", n)
				}
				if len(clock.sleeps) != 0 {
					t.Errorf("sleeps = %v, want none", clock.sleeps)
				}
			})
		}
	}
}

func TestRetry_IdempotentCreateOrderIsRetried(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())
	srv.Inject(http.MethodPost, "/orders", fake.ServerError())

	order, err := c.CreateOrder(context.Background(), glorian.CreateOrderInput{
		DealerID: "1", ExternalReference: "wh-order-retry",
		Items: []glorian.OrderItemInput{{ProductID: "10", Quantity: 1}},
	})
	if err != nil || order.ID == "" {
		t.Fatalf("CreateOrder = %+v, %v", order, err)
	}
	reqs := srv.RequestsTo(http.MethodPost, "/orders")
	if len(reqs) != 2 {
		t.Fatalf("attempts = %d, want 2", len(reqs))
	}
	for _, r := range reqs {
		if r.Header.Get(glorian.HeaderIdempotencyKey) != "wh-order-retry" {
			t.Errorf("attempt without Idempotency-Key")
		}
	}
}

type flakyTransport struct {
	failures int
	next     http.RoundTripper
}

func (f *flakyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if f.failures > 0 {
		f.failures--
		return nil, errors.New("connection reset by peer")
	}
	return f.next.RoundTrip(r)
}

func TestRetry_GetTransportErrorIsRetried(t *testing.T) {
	srv := fake.New(t)
	clock := newClock()
	c, err := glorian.NewHTTPClient(glorian.Options{
		BaseURL:    srv.URL,
		APIKey:     fake.APIKey,
		HTTPClient: &http.Client{Transport: &flakyTransport{failures: 1, next: srv.Client().Transport}},
		Clock:      clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListProducts(context.Background(), glorian.ListParams{}); err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(clock.sleeps) != 1 {
		t.Errorf("sleeps = %v, want one backoff", clock.sleeps)
	}
}

func TestRetry_StopsWhenContextIsCancelled(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())
	srv.Inject(http.MethodGet, "/products", fake.Scenario{Status: 500, Times: 5})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ListProducts(ctx, glorian.ListParams{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestErrors_EnvelopeBecomesTypedError(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())
	srv.Inject(http.MethodPost, "/orders", fake.ErrorEnvelope(http.StatusUnprocessableEntity, glorian.CodeValidation,
		"The given data was invalid.", map[string]any{"items.0.barcodes": []any{"Barcode not available."}}))

	_, err := c.CreateOrder(context.Background(), glorian.CreateOrderInput{
		DealerID: "1", Items: []glorian.OrderItemInput{{ProductID: "10", Quantity: 1}},
	})
	var apiErr *glorian.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T %v, want *APIError", err, err)
	}
	if !errors.Is(err, glorian.ErrValidation) || errors.Is(err, glorian.ErrConflict) {
		t.Errorf("class mismatch: %v", err)
	}
	if apiErr.Status != 422 || apiErr.Code != glorian.CodeValidation || apiErr.Message != "The given data was invalid." {
		t.Errorf("apiErr = %+v", apiErr)
	}
	if apiErr.RequestID == "" || apiErr.Method != http.MethodPost || apiErr.Path != "/orders" {
		t.Errorf("apiErr context = %+v", apiErr)
	}
	if _, ok := apiErr.Details["items.0.barcodes"]; !ok {
		t.Errorf("details = %v", apiErr.Details)
	}
	if !strings.Contains(err.Error(), "VALIDATION_ERROR") {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestErrors_StatusClasses(t *testing.T) {
	cases := []struct {
		scenario fake.Scenario
		want     error
	}{
		{fake.ErrorEnvelope(400, glorian.CodeUnsupportedVersion, "Unsupported.", nil), glorian.ErrUnsupportedVersion},
		{fake.ErrorEnvelope(403, glorian.CodeForbidden, "Actor not configured.", nil), glorian.ErrForbidden},
		{fake.ErrorEnvelope(404, glorian.CodeNotFound, "Resource not found.", nil), glorian.ErrNotFound},
		{fake.ErrorEnvelope(501, "NOT_IMPLEMENTED", "FAZ 2.", nil), glorian.ErrNotImplemented},
		{fake.Scenario{Status: 502, Body: "<html>bad gateway</html>", Times: 3}, glorian.ErrServer},
		// 200 with success=false is still an error.
		{fake.Scenario{Status: 200, Body: map[string]any{"success": false, "error": map[string]any{"code": "CONFLICT", "message": "dup"}}}, glorian.ErrConflict},
	}
	for _, tc := range cases {
		t.Run(tc.want.Error(), func(t *testing.T) {
			srv := fake.New(t)
			c := newClient(t, srv, newClock())
			srv.Inject(http.MethodGet, "/products", tc.scenario)
			if _, err := c.ListProducts(context.Background(), glorian.ListParams{}); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestErrors_WrongKeyIsUnauthorized(t *testing.T) {
	srv := fake.New(t)
	c, err := glorian.NewHTTPClient(glorian.Options{BaseURL: srv.URL, APIKey: "wrong-placeholder", HTTPClient: srv.Client(), Clock: newClock()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListDealers(context.Background(), glorian.ListParams{}); !errors.Is(err, glorian.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Errorf("401 must not be retried, attempts = %d", n)
	}
}

func TestErrors_NonEnvelopeSuccessIsInvalidResponse(t *testing.T) {
	srv := fake.New(t)
	c := newClient(t, srv, newClock())
	srv.Inject(http.MethodGet, "/products", fake.Scenario{Status: 200, Body: json.RawMessage(`[1,2,3]`)})
	if _, err := c.ListProducts(context.Background(), glorian.ListParams{}); !errors.Is(err, glorian.ErrInvalidResponse) {
		t.Fatalf("err = %v, want ErrInvalidResponse", err)
	}
}

func TestNewHTTPClient_RequiresBaseURLAndKey(t *testing.T) {
	for _, o := range []glorian.Options{
		{APIKey: "k"},
		{BaseURL: "https://hub.example.test"},
		{BaseURL: "not a url", APIKey: "k"},
	} {
		if _, err := glorian.NewHTTPClient(o); !errors.Is(err, glorian.ErrMisconfigured) {
			t.Errorf("NewHTTPClient(%+v) err = %v", o, err)
		}
	}
}
