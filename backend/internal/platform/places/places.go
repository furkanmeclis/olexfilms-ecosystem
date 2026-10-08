// Package places is a small Google Places API (New) client for the dealer
// showcase Google rating (TEC-469, F5-01d). It reads only rating and
// userRatingCount of one place (field mask), never searches (cost). The
// API key comes only from env GOOGLE_PLACES_API_KEY; without it every call
// is ErrNotConfigured and the showcase stays on manual entry (F5 S7).
package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the Places API (New) endpoint.
const DefaultBaseURL = "https://places.googleapis.com/v1"

// FieldMask is the only data read: billing follows the requested fields.
const FieldMask = "rating,userRatingCount"

var (
	// ErrNotConfigured: no API key (manual entry only).
	ErrNotConfigured = errors.New("places: GOOGLE_PLACES_API_KEY not set")
	// ErrNotFound: the place id is unknown or malformed (400 / 404).
	ErrNotFound = errors.New("places: place not found")
	// ErrQuota: the key's quota or rate limit is exhausted (429).
	ErrQuota = errors.New("places: quota exceeded")
)

// StatusError is any other non-2xx answer (5xx, 401/403 key problems).
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("places: http %d: %s", e.Code, e.Body)
}

// Rating is the Google rating of a place. Rating is nil when the place has
// no reviews yet.
type Rating struct {
	Rating      *float64
	ReviewCount int64
}

// Client calls the Places API (New).
type Client struct {
	key     string
	baseURL string
	http    *http.Client
}

// Option tunes the client.
type Option func(*Client)

// WithBaseURL overrides the endpoint (tests, proxy).
func WithBaseURL(u string) Option {
	return func(c *Client) {
		if u = strings.TrimRight(strings.TrimSpace(u), "/"); u != "" {
			c.baseURL = u
		}
	}
}

// WithHTTPClient sets the HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// New creates the client; an empty key leaves it unconfigured.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		key: strings.TrimSpace(apiKey), baseURL: DefaultBaseURL,
		http: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Configured reports whether an API key is set.
func (c *Client) Configured() bool { return c != nil && c.key != "" }

// Rating reads the rating and review count of placeID.
func (c *Client) Rating(ctx context.Context, placeID string) (Rating, error) {
	if !c.Configured() {
		return Rating{}, ErrNotConfigured
	}
	placeID = strings.TrimSpace(placeID)
	if placeID == "" || strings.ContainsAny(placeID, "/?#") {
		return Rating{}, ErrNotFound
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/places/"+url.PathEscape(placeID), nil)
	if err != nil {
		return Rating{}, err
	}
	req.Header.Set("X-Goog-Api-Key", c.key)
	req.Header.Set("X-Goog-FieldMask", FieldMask)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return Rating{}, fmt.Errorf("places: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return Rating{}, fmt.Errorf("places: read: %w", err)
	}
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		return Rating{}, ErrQuota
	case res.StatusCode == http.StatusNotFound, res.StatusCode == http.StatusBadRequest:
		return Rating{}, ErrNotFound
	case res.StatusCode < 200 || res.StatusCode > 299:
		return Rating{}, &StatusError{Code: res.StatusCode, Body: truncate(string(body), 300)}
	}
	var out struct {
		Rating          *float64 `json:"rating"`
		UserRatingCount *int64   `json:"userRatingCount"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return Rating{}, fmt.Errorf("places: decode: %w", err)
	}
	r := Rating{Rating: out.Rating}
	if out.UserRatingCount != nil {
		r.ReviewCount = *out.UserRatingCount
	}
	return r, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Ref is what a Google Business / Maps URL tells about the place.
type Ref struct {
	// PlaceID is a Places API place id ("ChIJ…"), usable directly.
	PlaceID string
	// CID is the Maps customer id (?cid=…). The Places API (New) does not
	// take it without a text search, which is never done (cost), so the
	// owner still enters the place id.
	CID string
}

// ParseBusinessURL reads a place id or a cid from a Google Business / Maps
// link (organizations.google_business_url): query parameters place_id /
// placeid / query_place_id, q=place_id:<id> and cid. Anything else (short
// links, /maps/place/<name>/…) yields an empty Ref: no lookup is made.
func ParseBusinessURL(raw string) Ref {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return Ref{}
	}
	q := u.Query()
	var ref Ref
	for _, k := range []string{"place_id", "placeid", "query_place_id"} {
		if v := strings.TrimSpace(q.Get(k)); validPlaceID(v) {
			ref.PlaceID = v
			break
		}
	}
	if ref.PlaceID == "" {
		if v, ok := strings.CutPrefix(strings.TrimSpace(q.Get("q")), "place_id:"); ok && validPlaceID(v) {
			ref.PlaceID = v
		}
	}
	if v := strings.TrimSpace(q.Get("cid")); v != "" && isDigits(v) {
		ref.CID = v
	}
	return ref
}

func validPlaceID(v string) bool {
	if v == "" || len(v) > 255 {
		return false
	}
	for _, r := range v {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

func isDigits(v string) bool {
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return v != ""
}
