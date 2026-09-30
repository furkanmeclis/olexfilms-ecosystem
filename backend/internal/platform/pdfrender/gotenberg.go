package pdfrender

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Client renders HTML to PDF via Gotenberg Chromium.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New builds a Gotenberg client. baseURL is e.g. http://127.0.0.1:3001.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// HTMLToPDF converts an HTML document to PDF bytes.
func (c *Client) HTMLToPDF(ctx context.Context, html string) ([]byte, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("pdfrender: gotenberg URL is not configured")
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, fmt.Errorf("pdfrender: form file: %w", err)
	}
	if _, err := io.WriteString(part, html); err != nil {
		return nil, fmt.Errorf("pdfrender: write html: %w", err)
	}
	_ = w.WriteField("paperWidth", "8.27")
	_ = w.WriteField("paperHeight", "11.7")
	_ = w.WriteField("marginTop", "0.4")
	_ = w.WriteField("marginBottom", "0.4")
	_ = w.WriteField("marginLeft", "0.4")
	_ = w.WriteField("marginRight", "0.4")
	_ = w.WriteField("printBackground", "true")
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("pdfrender: close form: %w", err)
	}

	url := c.baseURL + "/forms/chromium/convert/html"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("pdfrender: request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

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
		return nil, fmt.Errorf("pdfrender: gotenberg status %d: %s", res.StatusCode, msg)
	}
	return data, nil
}
