// Package pdfrender is the single PDF engine (design §6: "tek motor"): HTML
// documents rendered by Gotenberg Chromium (/forms/chromium/convert/html).
//
// The client keeps one shared HTTP transport sized for worker-docs
// concurrency, bounds every attempt with its own timeout (below Gotenberg's
// --api-timeout) and retries transient failures (network, 429, 5xx) with
// exponential backoff. 4xx responses are not retried. Asynq retries of the
// surrounding task are separate.
package pdfrender

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"time"
)

// ErrNotConfigured is returned when GOTENBERG_URL is empty.
var ErrNotConfigured = errors.New("pdfrender: gotenberg URL is not configured")

// ErrNotPDF is returned when Gotenberg answers 2xx with a non-PDF body.
var ErrNotPDF = errors.New("pdfrender: response is not a PDF")

// StatusError is a non-2xx Gotenberg response.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("pdfrender: gotenberg status %d: %s", e.Code, e.Body)
}

// Retryable reports whether a retry may succeed (429 or 5xx).
func (e *StatusError) Retryable() bool {
	return e.Code == http.StatusTooManyRequests || e.Code >= 500
}

// Options tunes the Gotenberg client. Zero values take the defaults.
type Options struct {
	// Timeout bounds one attempt (default 20s, below --api-timeout=60s).
	Timeout time.Duration
	// MaxRetries is the number of retries after the first attempt (default 2;
	// negative disables retries).
	MaxRetries int
	// Backoff is the first retry delay, doubled per retry (default 250ms).
	Backoff time.Duration
	// MaxConnsPerHost sizes the idle connection pool (default 32, at least
	// the worker-docs concurrency).
	MaxConnsPerHost int
}

func (o Options) withDefaults() Options {
	if o.Timeout <= 0 {
		o.Timeout = 20 * time.Second
	}
	if o.MaxRetries == 0 {
		o.MaxRetries = 2
	}
	if o.MaxRetries < 0 {
		o.MaxRetries = 0
	}
	if o.Backoff <= 0 {
		o.Backoff = 250 * time.Millisecond
	}
	if o.MaxConnsPerHost <= 0 {
		o.MaxConnsPerHost = 32
	}
	return o
}

// Request is one HTML → PDF conversion (A4 portrait unless Landscape).
type Request struct {
	// HTML is the full document (see Document.HTML).
	HTML string
	// FooterHTML is an optional Gotenberg footer document; it may use the
	// pageNumber / totalPages classes.
	FooterHTML string
	Landscape  bool
}

// Client renders HTML to PDF via Gotenberg Chromium.
type Client struct {
	baseURL    string
	httpClient *http.Client
	opts       Options
}

// New builds a Gotenberg client with default options. baseURL is e.g.
// http://127.0.0.1:3001.
func New(baseURL string) *Client {
	return NewWithOptions(baseURL, Options{})
}

// NewWithOptions builds a Gotenberg client.
func NewWithOptions(baseURL string, opts Options) *Client {
	opts = opts.withDefaults()
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:        opts.MaxConnsPerHost * 2,
		MaxIdleConnsPerHost: opts.MaxConnsPerHost,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{Transport: transport},
		opts:       opts,
	}
}

// Configured reports whether a Gotenberg URL is set.
func (c *Client) Configured() bool { return c != nil && c.baseURL != "" }

// HTMLToPDF converts an HTML document to PDF bytes (A4, no footer).
func (c *Client) HTMLToPDF(ctx context.Context, html string) ([]byte, error) {
	return c.Convert(ctx, Request{HTML: html})
}

// Convert renders one request, retrying transient failures.
func (c *Client) Convert(ctx context.Context, req Request) ([]byte, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	body, contentType, err := buildForm(req)
	if err != nil {
		return nil, err
	}
	var lastErr error
	delay := c.opts.Backoff
	for attempt := 0; attempt <= c.opts.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("pdfrender: %w (last error: %v)", ctx.Err(), lastErr)
			case <-time.After(delay):
			}
			delay *= 2
		}
		data, err := c.do(ctx, body, contentType)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retryable(ctx, err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) do(ctx context.Context, body []byte, contentType string) ([]byte, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()
	url := c.baseURL + "/forms/chromium/convert/html"
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("pdfrender: request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdfrender: gotenberg call: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("pdfrender: read response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, &StatusError{Code: res.StatusCode, Body: msg}
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, ErrNotPDF
	}
	return data, nil
}

// retryable: network errors and per-attempt timeouts retry while the caller's
// context is alive; HTTP 429/5xx retry; other statuses and ErrNotPDF do not.
func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.Retryable()
	}
	return !errors.Is(err, ErrNotPDF)
}

func buildForm(req Request) ([]byte, string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, "", fmt.Errorf("pdfrender: form file: %w", err)
	}
	if _, err := io.WriteString(part, req.HTML); err != nil {
		return nil, "", fmt.Errorf("pdfrender: write html: %w", err)
	}
	marginBottom := "0.4"
	if req.FooterHTML != "" {
		footer, err := w.CreateFormFile("files", "footer.html")
		if err != nil {
			return nil, "", fmt.Errorf("pdfrender: footer file: %w", err)
		}
		if _, err := io.WriteString(footer, req.FooterHTML); err != nil {
			return nil, "", fmt.Errorf("pdfrender: write footer: %w", err)
		}
		marginBottom = "0.6"
	}
	fields := [][2]string{
		{"paperWidth", "8.27"},
		{"paperHeight", "11.7"},
		{"marginTop", "0.4"},
		{"marginBottom", marginBottom},
		{"marginLeft", "0.4"},
		{"marginRight", "0.4"},
		{"printBackground", "true"},
	}
	if req.Landscape {
		fields = append(fields, [2]string{"landscape", "true"})
	}
	for _, f := range fields {
		if err := w.WriteField(f[0], f[1]); err != nil {
			return nil, "", fmt.Errorf("pdfrender: field %s: %w", f[0], err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("pdfrender: close form: %w", err)
	}
	return body.Bytes(), w.FormDataContentType(), nil
}
