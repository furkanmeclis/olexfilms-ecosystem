package glorian

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// InventoryClient is the warehouse module's view of one Inventory API
// connection. Sync jobs and order flows depend on this interface only.
type InventoryClient interface {
	ListCategories(ctx context.Context, p ListParams) (Page[Category], error)
	ListProducts(ctx context.Context, p ListParams) (Page[Product], error)
	ListDealers(ctx context.Context, p ListParams) (Page[Dealer], error)
	ListStockItems(ctx context.Context, p ListParams) (Page[StockItem], error)

	UpsertBarcodes(ctx context.Context, items []BarcodeUpsert) (BulkUpsertResult, error)
	PatchStockItemByBarcode(ctx context.Context, barcode string, patch StockItemPatch) (StockItem, error)

	CreateOrder(ctx context.Context, in CreateOrderInput) (Order, error)
	TransitionOrder(ctx context.Context, orderID string, action OrderAction, in TransitionInput) (Order, error)
}

// Clock abstracts time so retry waits are testable without sleeping.
type Clock interface {
	Now() time.Time
	// Sleep blocks for d or until ctx is done, returning ctx.Err() then.
	Sleep(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Defaults of Options; the config package feeds the env overrides.
const (
	DefaultTimeout        = 15 * time.Second
	DefaultWriteTimeout   = 30 * time.Second
	DefaultMaxAttempts    = 3
	DefaultRetryBaseDelay = 200 * time.Millisecond
	DefaultMaxRetryAfter  = 60 * time.Second
)

// maxResponseBytes caps how much of a response body is read.
const maxResponseBytes = 16 << 20

// Options configures an HTTPClient.
type Options struct {
	// BaseURL is the hub root (https://hub.example); BasePath is appended.
	BaseURL string
	// APIKey is sent as Bearer token and as X-Inventory-Api-Key.
	APIKey string
	// Timeout bounds one read attempt, WriteTimeout one write attempt.
	Timeout      time.Duration
	WriteTimeout time.Duration
	// MaxAttempts is the total number of tries of a retryable request
	// (GETs and idempotent CreateOrder). Writes always try once.
	MaxAttempts int
	// RetryBaseDelay is the first exponential backoff step (doubles).
	RetryBaseDelay time.Duration
	// MaxRetryAfter is the longest Retry-After the client waits in place;
	// a longer one is returned as ErrRateLimited for the caller to
	// reschedule.
	MaxRetryAfter time.Duration
	HTTPClient    *http.Client
	Clock         Clock
}

// HTTPClient implements InventoryClient over the hub's REST contract.
type HTTPClient struct {
	base  string
	opts  Options
	http  *http.Client
	clock Clock
}

var _ InventoryClient = (*HTTPClient)(nil)

// NewHTTPClient validates opts and applies defaults.
func NewHTTPClient(opts Options) (*HTTPClient, error) {
	base := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if base == "" || strings.TrimSpace(opts.APIKey) == "" {
		return nil, fmt.Errorf("%w: base url and api key are required", ErrMisconfigured)
	}
	if u, err := url.Parse(base); err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("%w: invalid base url", ErrMisconfigured)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = DefaultWriteTimeout
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = DefaultMaxAttempts
	}
	if opts.RetryBaseDelay <= 0 {
		opts.RetryBaseDelay = DefaultRetryBaseDelay
	}
	if opts.MaxRetryAfter <= 0 {
		opts.MaxRetryAfter = DefaultMaxRetryAfter
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{}
	}
	clock := opts.Clock
	if clock == nil {
		clock = realClock{}
	}
	return &HTTPClient{base: base + BasePath, opts: opts, http: hc, clock: clock}, nil
}

// ListCategories pulls GET /product-categories.
func (c *HTTPClient) ListCategories(ctx context.Context, p ListParams) (Page[Category], error) {
	return list[Category](ctx, c, "/product-categories", p)
}

// ListProducts pulls GET /products.
func (c *HTTPClient) ListProducts(ctx context.Context, p ListParams) (Page[Product], error) {
	return list[Product](ctx, c, "/products", p)
}

// ListDealers pulls GET /dealers.
func (c *HTTPClient) ListDealers(ctx context.Context, p ListParams) (Page[Dealer], error) {
	return list[Dealer](ctx, c, "/dealers", p)
}

// ListStockItems pulls GET /stock-items.
func (c *HTTPClient) ListStockItems(ctx context.Context, p ListParams) (Page[StockItem], error) {
	return list[StockItem](ctx, c, "/stock-items", p)
}

// UpsertBarcodes pushes POST /stock-items/bulk (idempotent by barcode on
// the hub, but never retried here: a lost response is reconciled, not
// replayed).
func (c *HTTPClient) UpsertBarcodes(ctx context.Context, items []BarcodeUpsert) (BulkUpsertResult, error) {
	var out BulkUpsertResult
	if len(items) == 0 || len(items) > MaxBulkItems {
		return out, fmt.Errorf("%w: bulk upsert takes 1..%d items, got %d", ErrInvalidInput, MaxBulkItems, len(items))
	}
	for i, it := range items {
		if strings.TrimSpace(it.Barcode) == "" || strings.TrimSpace(it.ProductID) == "" {
			return out, fmt.Errorf("%w: item %d needs barcode and product_id", ErrInvalidInput, i)
		}
	}
	body := struct {
		Items []BarcodeUpsert `json:"items"`
	}{items}
	err := c.call(ctx, request{method: http.MethodPost, path: "/stock-items/bulk", body: body}, &out, nil)
	return out, err
}

// PatchStockItemByBarcode sends PATCH /stock-items/by-barcode/{barcode}.
func (c *HTTPClient) PatchStockItemByBarcode(ctx context.Context, barcode string, patch StockItemPatch) (StockItem, error) {
	var out StockItem
	if strings.TrimSpace(barcode) == "" {
		return out, fmt.Errorf("%w: barcode is required", ErrInvalidInput)
	}
	if patch.Status == "" && patch.Location == "" {
		return out, fmt.Errorf("%w: patch needs status or location", ErrInvalidInput)
	}
	err := c.call(ctx, request{
		method: http.MethodPatch,
		path:   "/stock-items/by-barcode/" + url.PathEscape(barcode),
		body:   patch,
	}, &out, nil)
	return out, err
}

// CreateOrder sends POST /orders. With an ExternalReference the request is
// idempotent on the hub (replay returns the existing order), so it carries
// an Idempotency-Key and is retried like a GET; without one it is tried
// once.
func (c *HTTPClient) CreateOrder(ctx context.Context, in CreateOrderInput) (Order, error) {
	var out Order
	if strings.TrimSpace(in.DealerID) == "" || len(in.Items) == 0 {
		return out, fmt.Errorf("%w: order needs dealer_id and at least one item", ErrInvalidInput)
	}
	if in.Notes != nil && utf8.RuneCountInString(*in.Notes) > MaxOrderNotesRunes {
		return out, fmt.Errorf("%w: notes exceed %d characters", ErrInvalidInput, MaxOrderNotesRunes)
	}
	for i, it := range in.Items {
		if strings.TrimSpace(it.ProductID) == "" || it.Quantity < 1 {
			return out, fmt.Errorf("%w: item %d needs product_id and quantity >= 1", ErrInvalidInput, i)
		}
		if len(it.Barcodes) > 0 && len(it.Barcodes) != it.Quantity {
			return out, fmt.Errorf("%w: item %d barcode count must equal quantity", ErrInvalidInput, i)
		}
	}
	ref := strings.TrimSpace(in.ExternalReference)
	err := c.call(ctx, request{
		method:         http.MethodPost,
		path:           "/orders",
		body:           in,
		retry:          ref != "",
		idempotencyKey: ref,
	}, &out, nil)
	return out, err
}

// TransitionOrder sends POST /orders/{id}/{action}. Never retried.
func (c *HTTPClient) TransitionOrder(ctx context.Context, orderID string, action OrderAction, in TransitionInput) (Order, error) {
	var out Order
	if strings.TrimSpace(orderID) == "" {
		return out, fmt.Errorf("%w: order id is required", ErrInvalidInput)
	}
	if !action.Valid() {
		return out, fmt.Errorf("%w: unknown order action %q", ErrInvalidInput, action)
	}
	if action == OrderActionShip && strings.TrimSpace(in.CargoCompany) == "" {
		return out, fmt.Errorf("%w: ship needs cargo_company", ErrInvalidInput)
	}
	err := c.call(ctx, request{
		method: http.MethodPost,
		path:   "/orders/" + url.PathEscape(orderID) + "/" + string(action),
		body:   in,
	}, &out, nil)
	return out, err
}

func list[T any](ctx context.Context, c *HTTPClient, path string, p ListParams) (Page[T], error) {
	var page Page[T]
	q := url.Values{}
	for k, v := range p.Filters {
		q.Set(k, v)
	}
	if !p.UpdatedSince.IsZero() {
		q.Set("updated_since", p.UpdatedSince.UTC().Format(time.RFC3339))
	}
	if p.Cursor != "" {
		n, err := strconv.Atoi(p.Cursor)
		if err != nil || n < 1 {
			return page, fmt.Errorf("%w: invalid cursor %q", ErrInvalidInput, p.Cursor)
		}
		q.Set("page", p.Cursor)
	}
	if p.PerPage > 0 {
		q.Set("per_page", strconv.Itoa(min(p.PerPage, MaxPerPage)))
	}
	var meta envelopeMeta
	if err := c.call(ctx, request{method: http.MethodGet, path: path, query: q, retry: true}, &page.Items, &meta); err != nil {
		return Page[T]{}, err
	}
	if page.Items == nil {
		page.Items = []T{}
	}
	if meta.Pagination != nil {
		page.Pagination = *meta.Pagination
		if meta.Pagination.CurrentPage < meta.Pagination.LastPage {
			page.NextCursor = strconv.Itoa(meta.Pagination.CurrentPage + 1)
		}
	}
	return page, nil
}

type request struct {
	method         string
	path           string
	query          url.Values
	body           any
	retry          bool
	idempotencyKey string
}

type envelopeMeta struct {
	RequestID  string      `json:"request_id"`
	Pagination *Pagination `json:"pagination"`
}

type envelope struct {
	Success *bool           `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
	Meta envelopeMeta `json:"meta"`
}

// call runs req with the retry policy and decodes data into out.
func (c *HTTPClient) call(ctx context.Context, req request, out any, meta *envelopeMeta) error {
	var payload []byte
	if req.body != nil {
		b, err := json.Marshal(req.body)
		if err != nil {
			return fmt.Errorf("%w: encode body: %v", ErrInvalidInput, err)
		}
		payload = b
	}
	attempts := 1
	if req.retry {
		attempts = c.opts.MaxAttempts
	}
	for attempt := 1; ; attempt++ {
		env, err := c.once(ctx, req, payload)
		if err == nil {
			if meta != nil {
				*meta = env.Meta
			}
			if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
				if err := json.Unmarshal(env.Data, out); err != nil {
					return fmt.Errorf("%w: %s %s: decode data: %v", ErrInvalidResponse, req.method, req.path, err)
				}
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		wait, ok := c.retryDelay(err, attempt)
		if !ok || attempt >= attempts {
			return err
		}
		if err := c.clock.Sleep(ctx, wait); err != nil {
			return err
		}
	}
}

// retryDelay decides whether err is retryable and how long to wait first.
func (c *HTTPClient) retryDelay(err error, attempt int) (time.Duration, bool) {
	backoff := c.opts.RetryBaseDelay << (attempt - 1)
	if errors.Is(err, ErrTransport) {
		return backoff, true
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return 0, false
	}
	switch apiErr.Status {
	case http.StatusTooManyRequests:
		if apiErr.RetryAfter > c.opts.MaxRetryAfter {
			return 0, false
		}
		if apiErr.RetryAfter > 0 {
			return apiErr.RetryAfter, true
		}
		return backoff, true
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return backoff, true
	}
	return 0, false
}

func (c *HTTPClient) once(ctx context.Context, req request, payload []byte) (envelope, error) {
	var env envelope
	timeout := c.opts.Timeout
	if req.method != http.MethodGet {
		timeout = c.opts.WriteTimeout
	}
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := c.base + req.path
	if len(req.query) > 0 {
		target += "?" + req.query.Encode()
	}
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	hreq, err := http.NewRequestWithContext(actx, req.method, target, body)
	if err != nil {
		return env, fmt.Errorf("%w: build request: %v", ErrMisconfigured, err)
	}
	hreq.Header.Set("Accept", "application/json")
	if payload != nil {
		hreq.Header.Set("Content-Type", "application/json")
	}
	hreq.Header.Set("Authorization", "Bearer "+c.opts.APIKey)
	hreq.Header.Set(HeaderAPIKey, c.opts.APIKey)
	hreq.Header.Set(HeaderAPIVersion, APIVersion)
	if req.idempotencyKey != "" {
		hreq.Header.Set(HeaderIdempotencyKey, req.idempotencyKey)
	}

	resp, err := c.http.Do(hreq)
	if err != nil {
		return env, fmt.Errorf("%w: %s %s: %v", ErrTransport, req.method, req.path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return env, fmt.Errorf("%w: %s %s: read body: %v", ErrTransport, req.method, req.path, err)
	}
	decodeErr := json.Unmarshal(raw, &env)
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300

	if !ok || (decodeErr == nil && env.Success != nil && !*env.Success) {
		apiErr := &APIError{
			Status:    resp.StatusCode,
			Method:    req.method,
			Path:      req.path,
			RequestID: env.Meta.RequestID,
		}
		if decodeErr == nil && env.Error != nil {
			apiErr.Code = env.Error.Code
			apiErr.Message = env.Error.Message
			apiErr.Details = env.Error.Details
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			if apiErr.Code == "" {
				apiErr.Code = CodeRateLimited
			}
			apiErr.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), c.clock.Now())
		}
		if apiErr.Code == "" {
			apiErr.Code = CodeHTTPError
		}
		return env, apiErr
	}
	if decodeErr != nil || env.Success == nil {
		return env, fmt.Errorf("%w: %s %s: HTTP %d without envelope", ErrInvalidResponse, req.method, req.path, resp.StatusCode)
	}
	return env, nil
}

// parseRetryAfter reads delta-seconds or an HTTP-date; zero when absent.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n < 0 {
			return 0
		}
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}
