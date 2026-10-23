package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// SmartPTG / NexPTG factory curves (TEC-934).
//
// The gauge only reports a frequency; every serial has a factory curve file
// `{SN}.dat` on the vendor's servers. Those servers are not usable from the
// phone (mysdprog.pl answers 404 over HTTPS, smartptg.pl only works over
// plain http), so the backend fetches the file server side, keeps it in the
// object store and serves it to the app.
//
// The curve is vendor data, not tenant data: the same serial has the same
// factory file whatever organization measures with it, so it is stored under
// a global object key (no organization_id / brand_id, no table) and only the
// route is permission-gated.
//
// The primary source is plain http, so a stored copy could have been forged
// in transit, and the factory may update a curve (the official app
// re-downloads it on every connection). A stored copy is therefore
// re-validated against the vendor after RevalidateAfter (default 24 h); a
// different valid file replaces it (and is logged); a vendor outage keeps
// serving the stored copy. A sidecar `{serial}.meta.json` records when it was fetched and
// when to check next. Manual purge: delete smartptg/curves/{serial}.dat and
// its .meta.json.
//
// The bytes are served as is (encrypted `IV:Base64(AES-128-CBC(csv))`); the
// client keeps the decryption so the protocol findings can extend it.

var (
	// ErrCurveSerialInvalid: the serial is not 1–9 digits.
	ErrCurveSerialInvalid = errors.New("measurements: invalid gauge serial")
	// ErrCurveNotFound: no source has a valid curve for the serial.
	ErrCurveNotFound = errors.New("measurements: gauge curve not found")
	// ErrCurveUpstreamUnavailable: no cached copy and every source failed
	// with a transport / server error (not a clean "not found").
	ErrCurveUpstreamUnavailable = errors.New("measurements: gauge curve sources unavailable")
)

// DefaultSmartPTGCurveSources are the vendor URLs tried in order; %s is the
// serial. Plain http first: it is the only one that serves the file today.
var DefaultSmartPTGCurveSources = []string{
	"http://www.smartptg.pl/char/%s.dat",
	"https://mysdprog.pl/char/%s.dat",
}

const (
	// CurveMaxBytes caps an upstream file (a real one is ~0.5 KB).
	CurveMaxBytes = 64 << 10
	curveMinBytes = 16 + 1 + 24 // IV, colon, base64 of at least one block
	// curveMaxMisses caps the in-memory negative cache.
	curveMaxMisses      = 10_000
	curveMaxRedirects   = 5
	curveMetaMaxBytes   = 4 << 10
	curveDefaultRecheck = 24 * time.Hour
)

// SmartPTGCurveObjectKey is the object store key of a serial's curve.
func SmartPTGCurveObjectKey(serial string) string {
	return "smartptg/curves/" + serial + ".dat"
}

// SmartPTGCurveMetaKey is the sidecar of a stored curve (fetch / check times).
func SmartPTGCurveMetaKey(serial string) string {
	return "smartptg/curves/" + serial + ".meta.json"
}

// CurveStore is the persistent cache (object store); nil = no persistence.
type CurveStore interface {
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Upload(ctx context.Context, file storage.File, path string) error
}

// CurveFetcher fetches one URL; implemented by *http.Client.
type CurveFetcher interface {
	Do(req *http.Request) (*http.Response, error)
}

// CurveConfig tunes the proxy. Zero values take the defaults.
type CurveConfig struct {
	Sources         []string      // ordered URL templates with one %s
	Timeout         time.Duration // per source request
	NotFoundTTL     time.Duration // remember a miss this long
	ErrorTTL        time.Duration // remember an upstream outage this long
	RevalidateAfter time.Duration // re-check a stored copy against the vendor after this
}

// Curve is a served curve file.
type Curve struct {
	Serial string
	Data   []byte
	// ETag is the quoted hex SHA-256 of the raw file.
	ETag string
}

// curveMeta is the sidecar of a stored curve.
type curveMeta struct {
	FetchedAt   time.Time `json:"fetched_at"`
	NextCheckAt time.Time `json:"next_check_at"`
	Source      string    `json:"source,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
}

// Curves serves factory curves: object store first, then the vendor
// sources; misses are remembered in memory so the vendor is not hammered.
type Curves struct {
	store   CurveStore
	fetcher CurveFetcher
	cfg     CurveConfig
	log     *slog.Logger
	now     func() time.Time

	mu       sync.Mutex
	misses   map[string]curveMiss
	inflight map[string]*curveCall
}

type curveMiss struct {
	err   error
	until time.Time
}

type curveCall struct {
	done  chan struct{}
	curve Curve
	err   error
}

// NewCurves builds the proxy. fetcher nil = NewCurveHTTPClient().
func NewCurves(store CurveStore, fetcher CurveFetcher, cfg CurveConfig, log *slog.Logger) *Curves {
	log = orDiscard(log)
	sources := make([]string, 0, len(cfg.Sources))
	for _, s := range cfg.Sources {
		// One %s and an http(s) URL; anything else is a misconfiguration.
		if strings.Count(s, "%") == 1 && strings.Count(s, "%s") == 1 &&
			(strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")) {
			sources = append(sources, s)
		} else {
			log.Warn("smartptg_curve_source_ignored", "source", s)
		}
	}
	cfg.Sources = sources
	if len(cfg.Sources) == 0 {
		cfg.Sources = DefaultSmartPTGCurveSources
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.NotFoundTTL <= 0 {
		cfg.NotFoundTTL = time.Hour
	}
	if cfg.ErrorTTL <= 0 {
		cfg.ErrorTTL = time.Minute
	}
	if cfg.RevalidateAfter <= 0 {
		cfg.RevalidateAfter = curveDefaultRecheck
	}
	if fetcher == nil {
		fetcher = NewCurveHTTPClient()
	}
	return &Curves{
		store: store, fetcher: fetcher, cfg: cfg, log: log, now: time.Now,
		misses: map[string]curveMiss{}, inflight: map[string]*curveCall{},
	}
}

// NewCurveHTTPClient follows a redirect only to the same scheme and host as
// the original request (at most 5); any other redirect is not followed and
// its 3xx answer counts as "this source has no file".
func NewCurveHTTPClient() *http.Client {
	return &http.Client{CheckRedirect: sameOriginRedirect}
}

func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if len(via) > curveMaxRedirects {
		return http.ErrUseLastResponse
	}
	first := via[0].URL
	if !sameOrigin(first, req.URL) {
		return http.ErrUseLastResponse
	}
	return nil
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

// NormalizeCurveSerial validates a path serial (1–9 digits, not all zeros)
// and returns its canonical form (no leading zeros).
func NormalizeCurveSerial(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 9 {
		return "", ErrCurveSerialInvalid
	}
	for _, c := range raw {
		if c < '0' || c > '9' {
			return "", ErrCurveSerialInvalid
		}
	}
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || n == 0 {
		return "", ErrCurveSerialInvalid
	}
	return strconv.FormatUint(n, 10), nil
}

// Get returns the curve of a serial.
func (c *Curves) Get(ctx context.Context, rawSerial string) (Curve, error) {
	serial, err := NormalizeCurveSerial(rawSerial)
	if err != nil {
		return Curve{}, err
	}

	c.mu.Lock()
	// The negative cache comes first: a remembered miss costs no store read.
	if m, ok := c.misses[serial]; ok {
		if c.now().Before(m.until) {
			c.mu.Unlock()
			return Curve{}, m.err
		}
		delete(c.misses, serial)
	}
	if call, ok := c.inflight[serial]; ok {
		c.mu.Unlock()
		select {
		case <-call.done:
			return call.curve, call.err
		case <-ctx.Done():
			return Curve{}, ctx.Err()
		}
	}
	call := &curveCall{done: make(chan struct{})}
	c.inflight[serial] = call
	c.mu.Unlock()

	// Detached from the request: a cancelled caller must not poison the
	// shared result of the others waiting on it.
	call.curve, call.err = c.load(context.WithoutCancel(ctx), serial)

	c.mu.Lock()
	delete(c.inflight, serial)
	switch {
	case errors.Is(call.err, ErrCurveNotFound):
		c.remember(serial, call.err, c.cfg.NotFoundTTL)
	case call.err != nil:
		c.remember(serial, call.err, c.cfg.ErrorTTL)
	}
	c.mu.Unlock()
	close(call.done)
	return call.curve, call.err
}

// remember stores a miss; c.mu is held. At the cap, expired entries go
// first, then the one expiring soonest.
func (c *Curves) remember(serial string, err error, ttl time.Duration) {
	now := c.now()
	if _, exists := c.misses[serial]; !exists && len(c.misses) >= curveMaxMisses {
		for k, m := range c.misses {
			if !now.Before(m.until) {
				delete(c.misses, k)
			}
		}
		for len(c.misses) >= curveMaxMisses {
			var oldest string
			var oldestUntil time.Time
			for k, m := range c.misses {
				if oldest == "" || m.until.Before(oldestUntil) {
					oldest, oldestUntil = k, m.until
				}
			}
			delete(c.misses, oldest)
		}
	}
	c.misses[serial] = curveMiss{err: err, until: now.Add(ttl)}
}

// load serves the stored copy (re-validating it when due) or fetches it.
func (c *Curves) load(ctx context.Context, serial string) (Curve, error) {
	stored, meta, ok := c.cached(ctx, serial)
	if ok && c.now().Before(meta.NextCheckAt) {
		return newCurve(serial, stored), nil
	}
	fresh, source, err := c.fetch(ctx, serial)
	if !ok {
		if err != nil {
			return Curve{}, err
		}
		c.save(ctx, serial, fresh, source, true)
		return newCurve(serial, fresh), nil
	}
	// Re-validation of a stored copy: the vendor is down or lost the file,
	// keep serving ours and look again later.
	if err != nil {
		c.log.Info("smartptg_curve_revalidate_failed", "serial", serial, "error", err)
		c.saveMeta(ctx, serial, curveMeta{
			FetchedAt: meta.FetchedAt, Source: meta.Source, SHA256: meta.SHA256,
			NextCheckAt: c.now().Add(c.cfg.NotFoundTTL),
		})
		return newCurve(serial, stored), nil
	}
	if !bytes.Equal(fresh, stored) {
		c.log.Warn("smartptg_curve_changed", "serial", serial, "source", source,
			"old_sha256", sha256Hex(stored), "new_sha256", sha256Hex(fresh))
		c.save(ctx, serial, fresh, source, true)
		return newCurve(serial, fresh), nil
	}
	c.save(ctx, serial, fresh, source, false)
	return newCurve(serial, stored), nil
}

func (c *Curves) cached(ctx context.Context, serial string) ([]byte, curveMeta, bool) {
	if c.store == nil {
		return nil, curveMeta{}, false
	}
	data, err := c.read(ctx, SmartPTGCurveObjectKey(serial), CurveMaxBytes)
	if err != nil || ValidCurveFile(data) != nil {
		return nil, curveMeta{}, false
	}
	// A missing or broken sidecar (zero NextCheckAt) re-validates now.
	var meta curveMeta
	if raw, err := c.read(ctx, SmartPTGCurveMetaKey(serial), curveMetaMaxBytes); err == nil {
		_ = json.Unmarshal(raw, &meta)
	}
	return data, meta, true
}

func (c *Curves) read(ctx context.Context, key string, max int64) ([]byte, error) {
	rc, _, err := c.store.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(io.LimitReader(rc, max+1))
}

// save writes the curve (when writeData) and its sidecar. Store errors are
// logged only: the curve is still served and the next request retries.
func (c *Curves) save(ctx context.Context, serial string, data []byte, source string, writeData bool) {
	if c.store == nil {
		return
	}
	if writeData {
		if err := c.store.Upload(ctx, storage.File{
			Body: bytes.NewReader(data), Size: int64(len(data)),
			ContentType: "application/octet-stream", Filename: serial + ".dat",
		}, SmartPTGCurveObjectKey(serial)); err != nil {
			c.log.Warn("smartptg_curve_store_failed", "serial", serial, "error", err)
			return
		}
	}
	now := c.now()
	c.saveMeta(ctx, serial, curveMeta{
		FetchedAt: now, NextCheckAt: now.Add(c.cfg.RevalidateAfter), Source: source, SHA256: sha256Hex(data),
	})
}

func (c *Curves) saveMeta(ctx context.Context, serial string, meta curveMeta) {
	if c.store == nil {
		return
	}
	raw, _ := json.Marshal(meta)
	if err := c.store.Upload(ctx, storage.File{
		Body: bytes.NewReader(raw), Size: int64(len(raw)),
		ContentType: "application/json", Filename: serial + ".meta.json",
	}, SmartPTGCurveMetaKey(serial)); err != nil {
		c.log.Warn("smartptg_curve_meta_store_failed", "serial", serial, "error", err)
	}
}

// fetch tries the sources in order; it returns the file and the template
// that served it.
func (c *Curves) fetch(ctx context.Context, serial string) ([]byte, string, error) {
	unavailable := false
	for _, tmpl := range c.cfg.Sources {
		data, status, err := c.fetchOne(ctx, fmt.Sprintf(tmpl, serial))
		switch {
		case err != nil:
			unavailable = true
			c.log.Warn("smartptg_curve_source_failed", "serial", serial, "source", tmpl, "error", err)
			continue
		case status == http.StatusNotFound || status == http.StatusGone:
			continue
		case status != http.StatusOK:
			if status >= 500 || status == http.StatusTooManyRequests {
				unavailable = true
			}
			c.log.Warn("smartptg_curve_source_status", "serial", serial, "source", tmpl, "status", status)
			continue
		}
		if verr := ValidCurveFile(data); verr != nil {
			// Vendors answer 200 with an HTML error page for unknown serials.
			c.log.Info("smartptg_curve_source_invalid", "serial", serial, "source", tmpl, "error", verr)
			continue
		}
		return data, tmpl, nil
	}
	if unavailable {
		return nil, "", ErrCurveUpstreamUnavailable
	}
	return nil, "", ErrCurveNotFound
}

func (c *Curves) fetchOne(ctx context.Context, url string) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "OlexFilms-CurveProxy/1")
	res, err := c.fetcher.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4<<10))
		return nil, res.StatusCode, nil
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, CurveMaxBytes+1))
	if err != nil {
		return nil, 0, err
	}
	return data, res.StatusCode, nil
}

// ValidCurveFile checks the shape the mobile decoder expects without
// decrypting: `IV:Base64`, a 16 byte IV and a non-empty cipher that is a
// whole number of AES blocks, within CurveMaxBytes.
func ValidCurveFile(data []byte) error {
	if len(data) > CurveMaxBytes {
		return errors.New("curve file too large")
	}
	text := strings.TrimSpace(string(data))
	if len(text) < curveMinBytes {
		return errors.New("curve file too short")
	}
	iv, b64, ok := strings.Cut(text, ":")
	if !ok || len(iv) != 16 {
		return errors.New("curve file has no 16 byte IV")
	}
	for i := 0; i < len(iv); i++ {
		if iv[i] < 0x20 || iv[i] > 0x7e {
			return errors.New("curve IV is not printable")
		}
	}
	cipher, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return fmt.Errorf("curve cipher is not base64: %w", err)
	}
	if len(cipher) == 0 || len(cipher)%16 != 0 {
		return errors.New("curve cipher is not whole AES blocks")
	}
	return nil
}

func orDiscard(log *slog.Logger) *slog.Logger {
	if log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return log
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newCurve(serial string, data []byte) Curve {
	return Curve{Serial: serial, Data: data, ETag: `"` + sha256Hex(data) + `"`}
}
