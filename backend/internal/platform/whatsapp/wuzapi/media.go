package wuzapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

// downloadPaths maps an inbound media type to wuzapi's download endpoint
// (stickers are webp images).
var downloadPaths = map[string]string{
	"image":    "/chat/downloadimage",
	"sticker":  "/chat/downloadimage",
	"document": "/chat/downloaddocument",
	"audio":    "/chat/downloadaudio",
	"video":    "/chat/downloadvideo",
}

// DownloadMedia implements whatsapp.MediaDownloader (TEC-395). Media wuzapi
// already copied to S3 (payload "s3.url", in m.URL) is fetched directly;
// otherwise
// wuzapi decrypts it from the WhatsApp CDN with the message's download
// reference and returns a base64 data URL.
func (c *Client) DownloadMedia(ctx context.Context, m whatsapp.InboundMedia, max int64) ([]byte, error) {
	if max <= 0 {
		max = whatsapp.MaxMediaBytes
	}
	if m.Size > max {
		return nil, whatsapp.ErrMediaTooLarge
	}
	if m.URL != "" {
		return c.fetchURL(ctx, m.URL, max)
	}
	if len(m.Download) == 0 {
		return nil, whatsapp.ErrMediaNoDownload
	}
	endpoint, ok := downloadPaths[m.Type]
	if !ok {
		return nil, whatsapp.ErrMediaType
	}
	var ref mediaRef
	if err := json.Unmarshal(m.Download, &ref); err != nil {
		return nil, fmt.Errorf("wuzapi: media reference: %w", err)
	}
	if ref.FileLength > max {
		return nil, whatsapp.ErrMediaTooLarge
	}
	token := c.UserToken()
	if c.cfg.BaseURL == "" || token == "" {
		return nil, whatsapp.ErrNotConfigured
	}
	raw, _ := json.Marshal(ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("token", token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wuzapi: media download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// base64 is 4/3 of the payload, plus the JSON envelope.
	limit := max/3*4 + 8<<10
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, whatsapp.ErrMediaTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}
	var env struct {
		Data struct {
			Data string `json:"Data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("wuzapi: media download body: %w", err)
	}
	encoded := env.Data.Data
	if i := strings.Index(encoded, ","); strings.HasPrefix(encoded, "data:") && i >= 0 {
		encoded = encoded[i+1:]
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("wuzapi: media download data: %w", err)
	}
	if int64(len(data)) > max {
		return nil, whatsapp.ErrMediaTooLarge
	}
	return data, nil
}

func (c *Client) fetchURL(ctx context.Context, url string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wuzapi: media fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wuzapi: media fetch: http %d", resp.StatusCode)
	}
	if resp.ContentLength > max {
		return nil, whatsapp.ErrMediaTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, whatsapp.ErrMediaTooLarge
	}
	return data, nil
}

var _ whatsapp.MediaDownloader = (*Client)(nil)
