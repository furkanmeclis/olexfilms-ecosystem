package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
)

// Publisher sends messages to Centrifugo channels.
type Publisher interface {
	Publish(ctx context.Context, channel string, data any) error
}

// CentrifugoClient talks to Centrifugo HTTP API.
type CentrifugoClient struct {
	apiURL     string
	apiKey     string
	httpClient *http.Client
}

// NewCentrifugoClient builds a publisher from config. Returns nil when disabled.
func NewCentrifugoClient(cfg config.CentrifugoConfig) *CentrifugoClient {
	if !cfg.Enabled {
		return nil
	}
	apiURL := strings.TrimRight(cfg.APIURL, "/")
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8000"
	}
	return &CentrifugoClient{
		apiURL: apiURL,
		apiKey: cfg.APIKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type publishRequest struct {
	Method string        `json:"method"`
	Params publishParams `json:"params"`
}

type publishParams struct {
	Channel string `json:"channel"`
	Data    any    `json:"data"`
}

// Publish pushes a JSON payload to a Centrifugo channel.
func (c *CentrifugoClient) Publish(ctx context.Context, channel string, data any) error {
	if c == nil {
		return nil
	}
	if channel == "" {
		return fmt.Errorf("realtime: channel is required")
	}
	body, err := json.Marshal(publishRequest{
		Method: "publish",
		Params: publishParams{Channel: channel, Data: data},
	})
	if err != nil {
		return fmt.Errorf("realtime: marshal publish: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("realtime: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "apikey "+c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("realtime: publish request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("realtime: publish status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// NoopPublisher discards publishes (tests / disabled realtime).
type NoopPublisher struct{}

// Publish implements Publisher.
func (NoopPublisher) Publish(context.Context, string, any) error { return nil }

var (
	_ Publisher = (*CentrifugoClient)(nil)
	_ Publisher = NoopPublisher{}
)

// Ping calls the server API's info method with the API key (TEC-275
// preflight): it proves the endpoint the publisher uses answers and accepts
// the key. A nil client (realtime disabled) is an error.
func (c *CentrifugoClient) Ping(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("realtime: centrifugo is disabled")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api",
		strings.NewReader(`{"method":"info","params":{}}`))
	if err != nil {
		return fmt.Errorf("realtime: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "apikey "+c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("realtime: info request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("realtime: info status %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("realtime: info response: %w", err)
	}
	if out.Error != nil {
		return fmt.Errorf("realtime: info error %d: %s", out.Error.Code, out.Error.Message)
	}
	return nil
}
