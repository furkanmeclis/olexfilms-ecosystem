package fxrates

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Default provider endpoints.
const (
	DefaultTCMBURL = "https://www.tcmb.gov.tr/kurlar/today.xml"
	DefaultECBURL  = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"
)

// Fetcher downloads provider documents.
type Fetcher struct {
	HTTP    *http.Client
	TCMBURL string
	ECBURL  string
}

// NewFetcher builds a fetcher; empty URLs fall back to the defaults.
func NewFetcher(tcmbURL, ecbURL string) *Fetcher {
	if strings.TrimSpace(tcmbURL) == "" {
		tcmbURL = DefaultTCMBURL
	}
	if strings.TrimSpace(ecbURL) == "" {
		ecbURL = DefaultECBURL
	}
	return &Fetcher{HTTP: &http.Client{Timeout: 20 * time.Second}, TCMBURL: tcmbURL, ECBURL: ecbURL}
}

// TCMB fetches and parses today's TCMB publication. On weekends and
// holidays today.xml carries the last business day's date.
func (f *Fetcher) TCMB(ctx context.Context) (Day, error) {
	body, err := f.get(ctx, f.TCMBURL)
	if err != nil {
		return Day{}, fmt.Errorf("fxrates: tcmb: %w", err)
	}
	defer func() { _ = body.Close() }()
	return ParseTCMB(body)
}

// ECB fetches and parses the ECB daily reference rates.
func (f *Fetcher) ECB(ctx context.Context) (Day, error) {
	body, err := f.get(ctx, f.ECBURL)
	if err != nil {
		return Day{}, fmt.Errorf("fxrates: ecb: %w", err)
	}
	defer func() { _ = body.Close() }()
	return ParseECB(body)
}

func (f *Fetcher) get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/xml, text/xml")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return http.MaxBytesReader(nil, resp.Body, 2<<20), nil
}
