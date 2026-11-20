// Package fake serves the hub's Inventory API (/api/v1/inventory) from an
// httptest server so warehouse code can be tested against the real wire
// contract without network access. Endpoints, envelopes and status codes
// follow olexfilms docs/inventory-api.md and the warehouse
// InventoryApiClient.php; list data comes from embedded JSON fixtures.
//
// Every request is recorded (Requests) and failures can be injected per
// endpoint (Inject with RateLimited, ServerError, ErrorEnvelope).
package fake

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
)

// APIKey is the placeholder key the fake accepts by default. Not a secret.
const APIKey = "fake-inventory-api-key-placeholder"

// Fixture names accepted by SetFixture.
const (
	FixtureCategories = "product_categories"
	FixtureProducts   = "products"
	FixtureDealers    = "dealers"
	FixtureStockItems = "stock_items"
)

//go:embed fixtures/*.json
var fixtureFS embed.FS

// Row is one resource as served on the wire.
type Row = map[string]any

// Request is a recorded inbound request. Path is relative to
// glorian.BasePath (e.g. "/stock-items/bulk").
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

// JSON decodes the recorded body into v.
func (r Request) JSON(v any) error { return json.Unmarshal(r.Body, v) }

// Scenario is an injected response. Body nil means a default error envelope
// for Status (a 429 is answered outside the envelope, like Laravel's
// throttle). Times is how many matching requests it answers (default 1).
type Scenario struct {
	Status int
	Header http.Header
	Body   any
	Times  int
}

// RateLimited is a 429 with the given Retry-After header value.
func RateLimited(retryAfter string) Scenario {
	h := http.Header{}
	if retryAfter != "" {
		h.Set("Retry-After", retryAfter)
	}
	return Scenario{Status: http.StatusTooManyRequests, Header: h, Body: map[string]any{"message": "Too Many Attempts."}}
}

// ServerError is a 500 INTERNAL envelope.
func ServerError() Scenario {
	return ErrorEnvelope(http.StatusInternalServerError, glorian.CodeInternal, "Server error.", nil)
}

// ErrorEnvelope is an arbitrary {success:false, error:{...}} response.
func ErrorEnvelope(status int, code, message string, details map[string]any) Scenario {
	if details == nil {
		details = map[string]any{}
	}
	return Scenario{Status: status, Body: errorBody(code, message, details, "req-injected")}
}

// Server is the fake hub.
type Server struct {
	*httptest.Server
	// APIKey is the key requests must carry (Bearer or X-Inventory-Api-Key).
	APIKey string

	mu        sync.Mutex
	now       time.Time
	seq       int
	requests  []Request
	scenarios map[string][]Scenario
	fixtures  map[string][]Row
	stock     map[string]Row // by barcode
	nextStock int
	orders    map[string]Row
	orderRefs map[string]string
	nextOrder int
	prices    map[string]Price // by product id (TEC-1265)
}

// New starts a fake hub and closes it when the test ends.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{
		APIKey:    APIKey,
		now:       time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
		scenarios: map[string][]Scenario{},
		fixtures:  map[string][]Row{},
		stock:     map[string]Row{},
		orders:    map[string]Row{},
		orderRefs: map[string]string{},
		prices:    map[string]Price{},
		nextStock: 1000,
		nextOrder: 1,
	}
	for _, name := range []string{FixtureCategories, FixtureProducts, FixtureDealers, FixtureStockItems} {
		rows, err := loadFixture(name)
		if err != nil {
			t.Fatalf("fake: load fixture %s: %v", name, err)
		}
		s.SetFixture(name, rows)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func loadFixture(name string) ([]Row, error) {
	raw, err := fixtureFS.ReadFile("fixtures/" + name + ".json")
	if err != nil {
		return nil, err
	}
	var rows []Row
	return rows, json.Unmarshal(raw, &rows)
}

// SetFixture replaces the rows of a fixture (stock items also reset the
// barcode store the write endpoints work on).
func (s *Server) SetFixture(name string, rows []Row) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == FixtureStockItems {
		s.stock = map[string]Row{}
		for _, r := range rows {
			s.stock[str(r["barcode"])] = r
		}
		return
	}
	s.fixtures[name] = rows
}

// SetNow fixes the timestamp written into updated_at of mutated rows.
func (s *Server) SetNow(t time.Time) {
	s.mu.Lock()
	s.now = t
	s.mu.Unlock()
}

// Inject queues sc for requests matching method and path (relative to
// glorian.BasePath, e.g. "/orders/5/ship").
func (s *Server) Inject(method, path string, sc Scenario) {
	if sc.Times <= 0 {
		sc.Times = 1
	}
	s.mu.Lock()
	key := method + " " + path
	s.scenarios[key] = append(s.scenarios[key], sc)
	s.mu.Unlock()
}

// Requests returns a copy of every recorded request in arrival order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// RequestsTo returns the recorded requests for method and relative path.
func (s *Server) RequestsTo(method, path string) []Request {
	var out []Request
	for _, r := range s.Requests() {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// StockItem returns the current row of barcode.
func (s *Server) StockItem(barcode string) (Row, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.stock[barcode]
	return r, ok
}

// Price is the hub's stored price of a product (TEC-1265). MeterPrice is
// nil when cleared.
type Price struct {
	Price      string
	MeterPrice *string
}

// ProductPrice returns the stored price of a hub product id.
func (s *Server) ProductPrice(id string) (Price, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.prices[id]
	return p, ok
}

// Order returns the current row of an order id.
func (s *Server) Order(id string) (Row, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.orders[id]
	return r, ok
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	path := strings.TrimPrefix(r.URL.Path, glorian.BasePath)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	reqID := "req-" + strconv.Itoa(s.seq)
	s.requests = append(s.requests, Request{
		Method: r.Method, Path: path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body,
	})

	if !strings.HasPrefix(r.URL.Path, glorian.BasePath+"/") {
		s.fail(w, http.StatusNotFound, glorian.CodeNotFound, "Resource not found.", reqID)
		return
	}
	if !s.authorized(r) {
		s.fail(w, http.StatusUnauthorized, glorian.CodeUnauthorized, "Invalid or missing API key.", reqID)
		return
	}
	if r.Header.Get(glorian.HeaderAPIVersion) != glorian.APIVersion {
		s.fail(w, http.StatusBadRequest, glorian.CodeUnsupportedVersion, "Unsupported Inventory API version.", reqID)
		return
	}
	if sc, ok := s.popScenario(r.Method + " " + path); ok {
		for k, vs := range sc.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		b := sc.Body
		if b == nil {
			b = errorBody(glorian.CodeHTTPError, http.StatusText(sc.Status), map[string]any{}, reqID)
		}
		writeJSON(w, sc.Status, b)
		return
	}
	s.route(w, r.Method, path, r.URL.Query(), body, reqID)
}

func (s *Server) authorized(r *http.Request) bool {
	if r.Header.Get(glorian.HeaderAPIKey) == s.APIKey {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+s.APIKey
}

func (s *Server) popScenario(key string) (Scenario, bool) {
	q := s.scenarios[key]
	if len(q) == 0 {
		return Scenario{}, false
	}
	sc := q[0]
	q[0].Times--
	if q[0].Times <= 0 {
		s.scenarios[key] = q[1:]
	}
	return sc, true
}

func (s *Server) route(w http.ResponseWriter, method, path string, q url.Values, body []byte, reqID string) {
	seg := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case method == http.MethodGet && path == "/health":
		s.ok(w, http.StatusOK, Row{"status": "ok", "service": "inventory"}, reqID)
	case method == http.MethodGet && path == "/product-categories":
		s.list(w, s.fixtures[FixtureCategories], q, reqID)
	case method == http.MethodGet && path == "/products":
		s.list(w, s.fixtures[FixtureProducts], q, reqID)
	case method == http.MethodGet && path == "/dealers":
		s.list(w, s.fixtures[FixtureDealers], q, reqID)
	case method == http.MethodGet && path == "/stock-items":
		s.list(w, s.stockRows(), q, reqID)
	case method == http.MethodGet && path == "/orders":
		s.list(w, s.orderRows(), q, reqID)
	case method == http.MethodPut && path == "/products/prices":
		s.updatePrices(w, body, reqID)
	case method == http.MethodPost && path == "/stock-items/bulk":
		s.bulkUpsert(w, body, reqID)
	case len(seg) == 3 && seg[0] == "stock-items" && seg[1] == "by-barcode":
		barcode, _ := url.PathUnescape(seg[2])
		switch method {
		case http.MethodGet:
			if row, ok := s.stock[barcode]; ok {
				s.ok(w, http.StatusOK, row, reqID)
				return
			}
			s.fail(w, http.StatusNotFound, glorian.CodeNotFound, "Resource not found.", reqID)
		case http.MethodPatch:
			s.patchStock(w, barcode, body, reqID)
		default:
			s.fail(w, http.StatusMethodNotAllowed, glorian.CodeHTTPError, "Method not allowed.", reqID)
		}
	case method == http.MethodPost && path == "/orders":
		s.createOrder(w, body, reqID)
	case method == http.MethodGet && len(seg) == 2 && seg[0] == "orders":
		if row, ok := s.orders[seg[1]]; ok {
			s.ok(w, http.StatusOK, row, reqID)
			return
		}
		s.fail(w, http.StatusNotFound, glorian.CodeNotFound, "Resource not found.", reqID)
	case method == http.MethodPost && len(seg) == 3 && seg[0] == "orders":
		s.transition(w, seg[1], seg[2], body, reqID)
	default:
		s.fail(w, http.StatusNotFound, glorian.CodeNotFound, "Resource not found.", reqID)
	}
}

// list paginates rows with updated_since (inclusive), equality filters on
// any other resource field, updated_at/id ascending order.
func (s *Server) list(w http.ResponseWriter, rows []Row, q url.Values, reqID string) {
	var since time.Time
	if v := q.Get("updated_since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			s.validation(w, "updated_since", "The updated_since must be a valid date.", reqID)
			return
		}
		since = t
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if !since.IsZero() {
			ts, err := time.Parse(time.RFC3339, str(r["updated_at"]))
			if err != nil || ts.Before(since) {
				continue
			}
		}
		if !matches(r, q) {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := str(out[i]["updated_at"]), str(out[j]["updated_at"])
		if a != b {
			return a < b
		}
		ai, _ := strconv.Atoi(str(out[i]["id"]))
		bi, _ := strconv.Atoi(str(out[j]["id"]))
		return ai < bi
	})
	perPage := 50
	if v, err := strconv.Atoi(q.Get("per_page")); err == nil && v > 0 {
		perPage = min(v, glorian.MaxPerPage)
	}
	page := 1
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		page = v
	}
	total := len(out)
	lastPage := max(1, (total+perPage-1)/perPage)
	start := min((page-1)*perPage, total)
	end := min(start+perPage, total)
	writeJSON(w, http.StatusOK, Row{
		"success": true,
		"data":    out[start:end],
		"meta": Row{
			"request_id": reqID,
			"timestamp":  s.ts(),
			"pagination": Row{"current_page": page, "per_page": perPage, "total": total, "last_page": lastPage},
		},
	})
}

var listControlKeys = map[string]bool{"page": true, "per_page": true, "updated_since": true, "q": true}

func matches(r Row, q url.Values) bool {
	for k := range q {
		if listControlKeys[k] {
			continue
		}
		v, ok := r[k]
		if !ok {
			continue
		}
		want := q.Get(k)
		if b, isBool := v.(bool); isBool {
			if (want == "1" || want == "true") != b {
				return false
			}
			continue
		}
		if str(v) != want {
			return false
		}
	}
	return true
}

func (s *Server) stockRows() []Row {
	rows := make([]Row, 0, len(s.stock))
	for _, r := range s.stock {
		rows = append(rows, r)
	}
	return rows
}

func (s *Server) orderRows() []Row {
	rows := make([]Row, 0, len(s.orders))
	for _, r := range s.orders {
		rows = append(rows, r)
	}
	return rows
}

type bulkItem struct {
	Barcode   string  `json:"barcode"`
	ProductID string  `json:"product_id"`
	DealerID  *string `json:"dealer_id"`
	Location  string  `json:"location"`
	Status    string  `json:"status"`
	// TEC-1252 (inventory-api.md §10.6) meter fields.
	InitialMeters   *float64 `json:"initial_meters"`
	RemainingMeters *float64 `json:"remaining_meters"`
	SourceBarcode   string   `json:"source_barcode"`
	Credits         *int     `json:"credits"`
	Opened          bool     `json:"opened"`
}

// applyMeters records the meter fields of a bulk row (or an order piece)
// on a hub row the way the hub does (§10.6): a closed box with meters
// becomes an opened box, a new barcode with a source is a cut piece, the
// absolute remaining wins. A new cut piece lowers its source's remaining.
func (s *Server) applyMeters(row Row, it bulkItem, created bool) {
	if it.InitialMeters == nil && it.RemainingMeters == nil && it.SourceBarcode == "" && it.Credits == nil && !it.Opened {
		return
	}
	if row["initial_meters"] == nil && it.InitialMeters != nil {
		row["initial_meters"] = *it.InitialMeters
		row["remaining_meters"] = *it.InitialMeters
	}
	if it.RemainingMeters != nil {
		row["remaining_meters"] = *it.RemainingMeters
	}
	if it.Credits != nil {
		row["credits"] = *it.Credits
	}
	switch {
	case it.SourceBarcode != "" && row["source_barcode"] == nil:
		row["kind"], row["source_barcode"] = glorian.StockKindCutPiece, it.SourceBarcode
		if src, ok := s.stock[it.SourceBarcode]; ok && created && it.InitialMeters != nil {
			if rem, ok := src["remaining_meters"].(float64); ok {
				src["remaining_meters"] = max(0, rem-*it.InitialMeters)
			}
			if src["kind"] == nil || src["kind"] == glorian.StockKindBox {
				src["kind"] = glorian.StockKindOpenedBox
			}
		}
	case row["kind"] == nil || row["kind"] == glorian.StockKindBox:
		row["kind"] = glorian.StockKindOpenedBox
	}
}

func (s *Server) bulkUpsert(w http.ResponseWriter, body []byte, reqID string) {
	var in struct {
		Items []bulkItem `json:"items"`
	}
	if err := json.Unmarshal(body, &in); err != nil || len(in.Items) == 0 || len(in.Items) > glorian.MaxBulkItems {
		s.validation(w, "items", "The items field must have between 1 and 500 items.", reqID)
		return
	}
	created, updated, conflicts := []any{}, []any{}, []any{}
	// The hub records the cut pieces of a request first (§10.6).
	sort.SliceStable(in.Items, func(a, b int) bool {
		return in.Items[a].SourceBarcode != "" && in.Items[b].SourceBarcode == ""
	})
	for i, it := range in.Items {
		if it.Barcode == "" || it.ProductID == "" {
			s.validation(w, fmt.Sprintf("items.%d", i), "barcode and product_id are required.", reqID)
			return
		}
	}
	for _, it := range in.Items {
		loc, status := it.Location, it.Status
		if loc == "" {
			loc = glorian.StockLocationCenter
		}
		if status == "" {
			status = glorian.StockStatusAvailable
		}
		var dealer any
		if it.DealerID != nil {
			dealer = *it.DealerID
		}
		if cur, ok := s.stock[it.Barcode]; ok {
			if str(cur["status"]) != glorian.StockStatusAvailable {
				conflicts = append(conflicts, Row{"barcode": it.Barcode, "reason": "status_" + str(cur["status"])})
				continue
			}
			cur["product_id"], cur["dealer_id"], cur["location"], cur["status"] = it.ProductID, dealer, loc, status
			s.applyMeters(cur, it, false)
			cur["updated_at"] = s.ts()
			updated = append(updated, it.Barcode)
			continue
		}
		s.nextStock++
		row := Row{
			"id": strconv.Itoa(s.nextStock), "barcode": it.Barcode, "product_id": it.ProductID,
			"dealer_id": dealer, "status": status, "location": loc, "updated_at": s.ts(),
		}
		s.applyMeters(row, it, true)
		s.stock[it.Barcode] = row
		created = append(created, it.Barcode)
	}
	s.ok(w, http.StatusOK, Row{"created": created, "updated": updated, "conflicts": conflicts}, reqID)
}

// updatePrices mirrors PUT /products/prices (glorian docs/inventory-api.md
// §10.7): USD only, absolute values, unknown products in not_found, a
// missing meter_price key leaves the stored one.
func (s *Server) updatePrices(w http.ResponseWriter, body []byte, reqID string) {
	var in struct {
		Currency string                       `json:"currency"`
		Items    []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.Currency != "USD" {
		s.validation(w, "currency", "The selected currency is invalid.", reqID)
		return
	}
	if len(in.Items) == 0 || len(in.Items) > glorian.MaxBulkItems {
		s.validation(w, "items", "The items field must have between 1 and 500 items.", reqID)
		return
	}
	known := map[string]bool{}
	for _, r := range s.fixtures[FixtureProducts] {
		known[str(r["id"])] = true
	}
	updated, unchanged, notFound := []string{}, []string{}, []string{}
	for i, it := range in.Items {
		var id, price string
		if json.Unmarshal(it["product_id"], &id) != nil || json.Unmarshal(it["price"], &price) != nil || id == "" || price == "" {
			s.validation(w, fmt.Sprintf("items.%d", i), "product_id and price are required.", reqID)
			return
		}
		if !known[id] {
			notFound = append(notFound, id)
			continue
		}
		next := s.prices[id]
		before := next
		next.Price = price
		if raw, ok := it["meter_price"]; ok {
			var mp *string
			if err := json.Unmarshal(raw, &mp); err != nil {
				s.validation(w, fmt.Sprintf("items.%d.meter_price", i), "meter_price must be a string or null.", reqID)
				return
			}
			next.MeterPrice = mp
		}
		if before.Price == next.Price && equalPtr(before.MeterPrice, next.MeterPrice) {
			unchanged = append(unchanged, id)
			continue
		}
		s.prices[id] = next
		updated = append(updated, id)
	}
	s.ok(w, http.StatusOK, Row{"currency": "USD", "updated": updated, "unchanged": unchanged, "not_found": notFound}, reqID)
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s *Server) patchStock(w http.ResponseWriter, barcode string, body []byte, reqID string) {
	cur, ok := s.stock[barcode]
	if !ok {
		s.fail(w, http.StatusNotFound, glorian.CodeNotFound, "Resource not found.", reqID)
		return
	}
	var in struct {
		Status          string   `json:"status"`
		Location        string   `json:"location"`
		RemainingMeters *float64 `json:"remaining_meters"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		s.validation(w, "status", "Invalid body.", reqID)
		return
	}
	if in.RemainingMeters != nil {
		cur["remaining_meters"] = *in.RemainingMeters
	}
	switch str(cur["status"]) {
	case glorian.StockStatusReserved, glorian.StockStatusUsed:
		s.failDetails(w, http.StatusConflict, glorian.CodeConflict, "Stock item cannot be changed in its current status.",
			Row{"barcode": barcode, "status": cur["status"]}, reqID)
		return
	}
	if in.Status != "" {
		cur["status"] = in.Status
	}
	if in.Location != "" {
		cur["location"] = in.Location
	}
	cur["updated_at"] = s.ts()
	s.ok(w, http.StatusOK, cur, reqID)
}

// orderPiece is a piece of a meter line of POST /orders (§10.6).
type orderPiece struct {
	Barcode       string   `json:"barcode"`
	Meters        *float64 `json:"meters"`
	SourceBarcode string   `json:"source_barcode"`
	Credits       *int     `json:"credits"`
}

func (s *Server) createOrder(w http.ResponseWriter, body []byte, reqID string) {
	var in struct {
		DealerID          string  `json:"dealer_id"`
		Notes             *string `json:"notes"`
		ExternalReference string  `json:"external_reference"`
		AutoPrepare       *bool   `json:"auto_prepare"`
		Items             []struct {
			ProductID string   `json:"product_id"`
			Quantity  int      `json:"quantity"`
			Barcodes  []string `json:"barcodes"`
			// TEC-1252 meter line (§10.6).
			SaleMeasure    string         `json:"sale_measure"`
			Meters         *float64       `json:"meters"`
			LineCredits    *int           `json:"line_credits"`
			Plotter        bool           `json:"plotter"`
			Pieces         []orderPiece   `json:"pieces"`
			BarcodeCredits map[string]int `json:"barcode_credits"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.DealerID == "" || len(in.Items) == 0 {
		s.validation(w, "items", "dealer_id and items are required.", reqID)
		return
	}
	if id, ok := s.orderRefs[in.ExternalReference]; ok && in.ExternalReference != "" {
		s.ok(w, http.StatusOK, s.orders[id], reqID)
		return
	}
	seenMeasure := map[string]bool{}
	for _, it := range in.Items {
		measure := it.SaleMeasure
		if measure == "" {
			measure = glorian.SaleMeasureBox
		}
		if seenMeasure[it.ProductID+"|"+measure] {
			s.validation(w, "items", "One line per product and sale measure.", reqID)
			return
		}
		seenMeasure[it.ProductID+"|"+measure] = true
		if measure == glorian.SaleMeasureBox && (it.Meters != nil || it.LineCredits != nil || len(it.Pieces) > 0 || len(it.BarcodeCredits) > 0) {
			s.validation(w, "items", "A box line takes no meter fields.", reqID)
			return
		}
		if measure == glorian.SaleMeasureMeter && (it.Meters == nil || *it.Meters <= 0) {
			s.validation(w, "items", "A meter line needs meters.", reqID)
			return
		}
		if len(it.Barcodes) > 0 && len(it.Barcodes) != it.Quantity {
			s.validation(w, "items", "Barcodes count must match quantity for each item.", reqID)
			return
		}
	}
	for _, it := range in.Items {
		for _, pc := range it.Pieces {
			if _, ok := s.stock[pc.Barcode]; ok {
				s.applyMeters(s.stock[pc.Barcode], bulkItem{RemainingMeters: pc.Meters, Opened: true}, false)
				continue
			}
			s.nextStock++
			row := Row{
				"id": strconv.Itoa(s.nextStock), "barcode": pc.Barcode, "product_id": it.ProductID, "dealer_id": nil,
				"status": glorian.StockStatusAvailable, "location": glorian.StockLocationCenter, "updated_at": s.ts(),
			}
			s.applyMeters(row, bulkItem{InitialMeters: pc.Meters, SourceBarcode: pc.SourceBarcode, Opened: true}, true)
			s.stock[pc.Barcode] = row
		}
		for _, bc := range it.Barcodes {
			row, ok := s.stock[bc]
			if !ok || str(row["status"]) != glorian.StockStatusAvailable || str(row["location"]) != glorian.StockLocationCenter {
				s.validation(w, "items", "Barcode "+bc+" is not available at center.", reqID)
				return
			}
		}
	}
	prepare := in.AutoPrepare == nil || *in.AutoPrepare
	status := "pending"
	if prepare {
		status = "processing"
	}
	id := strconv.Itoa(s.nextOrder)
	s.nextOrder++
	items := make([]any, 0, len(in.Items))
	for i, it := range in.Items {
		ids := []any{}
		barcodes := []any{}
		for _, bc := range it.Barcodes {
			row := s.stock[bc]
			if prepare {
				row["status"] = glorian.StockStatusReserved
				row["updated_at"] = s.ts()
			}
			if c, ok := it.BarcodeCredits[bc]; ok {
				row["credits"] = c
			}
			ids = append(ids, row["id"])
			barcodes = append(barcodes, bc)
		}
		line := Row{
			"id": strconv.Itoa(i + 1), "product_id": it.ProductID, "quantity": it.Quantity,
			"stock_item_ids": ids, "barcodes": barcodes, "sale_measure": glorian.SaleMeasureBox,
			"meters": nil, "line_credits": nil, "plotter": it.Plotter,
		}
		if it.SaleMeasure == glorian.SaleMeasureMeter {
			line["sale_measure"], line["meters"] = glorian.SaleMeasureMeter, *it.Meters
			if it.LineCredits != nil {
				line["line_credits"] = *it.LineCredits
			}
		}
		items = append(items, line)
	}
	var ref any
	if in.ExternalReference != "" {
		ref = in.ExternalReference
		s.orderRefs[in.ExternalReference] = id
	}
	var notes any
	if in.Notes != nil {
		notes = *in.Notes
	}
	row := Row{
		"id": id, "dealer_id": in.DealerID, "status": status, "cargo_company": nil, "tracking_number": nil,
		"notes": notes, "external_reference": ref, "items": items, "created_at": s.ts(), "updated_at": s.ts(),
	}
	s.orders[id] = row
	s.ok(w, http.StatusCreated, row, reqID)
}

var actionStatus = map[string]string{
	"prepare": "processing",
	"ship":    "shipped",
	"deliver": "delivered",
	"receive": "delivered",
	"cancel":  "cancelled",
}

// actionFrom lists the statuses an action may start from, as in the hub's
// InventoryOrderService: an action that finds the order already in its
// target status returns it unchanged (200; ship keeps the first cargo
// data), any other start is a 422. cancel works from anything but
// delivered.
var actionFrom = map[string][]string{
	"prepare": {"pending", "processing"},
	"ship":    {"processing"},
	"deliver": {"shipped"},
	"receive": {"shipped"},
	"cancel":  {"pending", "processing", "shipped"},
}

func allowedFrom(action, status string) bool {
	for _, s := range actionFrom[action] {
		if s == status {
			return true
		}
	}
	return false
}

func (s *Server) transition(w http.ResponseWriter, id, action string, body []byte, reqID string) {
	status, ok := actionStatus[action]
	if !ok {
		s.fail(w, http.StatusNotFound, glorian.CodeNotFound, "Resource not found.", reqID)
		return
	}
	row, ok := s.orders[id]
	if !ok {
		s.fail(w, http.StatusNotFound, glorian.CodeNotFound, "Resource not found.", reqID)
		return
	}
	cur := str(row["status"])
	if cur == status && action != "prepare" {
		s.ok(w, http.StatusOK, row, reqID)
		return
	}
	if !allowedFrom(action, cur) {
		s.validation(w, "status", "Order cannot be "+action+" from "+cur+" status.", reqID)
		return
	}
	var in struct {
		CargoCompany   string `json:"cargo_company"`
		TrackingNumber string `json:"tracking_number"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			s.validation(w, "body", "Invalid body.", reqID)
			return
		}
	}
	if action == "ship" {
		if in.CargoCompany == "" {
			s.validation(w, "cargo_company", "The cargo company field is required.", reqID)
			return
		}
		row["cargo_company"] = in.CargoCompany
		if in.TrackingNumber != "" {
			row["tracking_number"] = in.TrackingNumber
		}
	}
	if action == "cancel" {
		// Cancelling releases the reserved stock items back to the center.
		s.releaseOrderStock(row)
	}
	row["status"] = status
	row["updated_at"] = s.ts()
	s.ok(w, http.StatusOK, row, reqID)
}

func (s *Server) releaseOrderStock(order Row) {
	items, _ := order["items"].([]any)
	for _, it := range items {
		item, _ := it.(Row)
		barcodes, _ := item["barcodes"].([]any)
		for _, bc := range barcodes {
			if row, ok := s.stock[str(bc)]; ok && str(row["status"]) == glorian.StockStatusReserved {
				row["status"] = glorian.StockStatusAvailable
				row["updated_at"] = s.ts()
			}
		}
	}
}

func (s *Server) ts() string { return s.now.Format(time.RFC3339) }

func (s *Server) ok(w http.ResponseWriter, status int, data any, reqID string) {
	writeJSON(w, status, Row{"success": true, "data": data, "meta": Row{"request_id": reqID, "timestamp": s.ts()}})
}

func (s *Server) fail(w http.ResponseWriter, status int, code, msg, reqID string) {
	s.failDetails(w, status, code, msg, Row{}, reqID)
}

func (s *Server) failDetails(w http.ResponseWriter, status int, code, msg string, details Row, reqID string) {
	writeJSON(w, status, errorBody(code, msg, details, reqID))
}

func (s *Server) validation(w http.ResponseWriter, field, msg, reqID string) {
	s.failDetails(w, http.StatusUnprocessableEntity, glorian.CodeValidation, "The given data was invalid.",
		Row{field: []string{msg}}, reqID)
}

func errorBody(code, msg string, details map[string]any, reqID string) Row {
	return Row{
		"success": false,
		"error":   Row{"code": code, "message": msg, "details": details},
		"meta":    Row{"request_id": reqID, "timestamp": time.Now().UTC().Format(time.RFC3339)},
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprint(t)
	}
}
