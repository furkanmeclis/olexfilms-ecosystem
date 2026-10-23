package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
)

// SmartPTGCurveObjectKey is the object store key of a serial's curve.
func SmartPTGCurveObjectKey(serial string) string {
	return "smartptg/curves/" + serial + ".dat"
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
	Sources     []string      // ordered URL templates with one %s
	Timeout     time.Duration // per source request
	NotFoundTTL time.Duration // remember a miss this long
	ErrorTTL    time.Duration // remember an upstream outage this long
}

// Curve is a served curve file.
type Curve struct {
	Serial string
	Data   []byte
	// ETag is the quoted SHA-256 of Data.
	ETag string
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

// NewCurves builds the proxy. fetcher nil = an http.Client (no redirects to
// other hosts are followed beyond the default policy; the timeout is per
// request via the context).
func NewCurves(store CurveStore, fetcher CurveFetcher, cfg CurveConfig, log *slog.Logger) *Curves {
	sources := make([]string, 0, len(cfg.Sources))
	for _, s := range cfg.Sources {
		// One %s and an http(s) URL; anything else is a misconfiguration.
		if strings.Count(s, "%") == 1 && strings.Count(s, "%s") == 1 &&
			(strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")) {
			sources = append(sources, s)
		} else {
			log = cmpLog(log)
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
	if fetcher == nil {
		fetcher = &http.Client{}
	}
	log = cmpLog(log)
	return &Curves{
		store: store, fetcher: fetcher, cfg: cfg, log: log, now: time.Now,
		misses: map[string]curveMiss{}, inflight: map[string]*curveCall{},
	}
}

// NormalizeCurveSerial validates a path serial (1–9 digits) and returns its
// canonical form (no leading zeros).
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
	if data, ok := c.cached(ctx, serial); ok {
		return newCurve(serial, data), nil
	}

	c.mu.Lock()
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
	call.curve, call.err = c.fetch(context.WithoutCancel(ctx), serial)

	c.mu.Lock()
	delete(c.inflight, serial)
	switch {
	case errors.Is(call.err, ErrCurveNotFound):
		c.misses[serial] = curveMiss{err: call.err, until: c.now().Add(c.cfg.NotFoundTTL)}
	case call.err != nil:
		c.misses[serial] = curveMiss{err: call.err, until: c.now().Add(c.cfg.ErrorTTL)}
	}
	c.mu.Unlock()
	close(call.done)
	return call.curve, call.err
}

func (c *Curves) cached(ctx context.Context, serial string) ([]byte, bool) {
	if c.store == nil {
		return nil, false
	}
	rc, _, err := c.store.Download(ctx, SmartPTGCurveObjectKey(serial))
	if err != nil {
		return nil, false
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, CurveMaxBytes+1))
	if err != nil || ValidCurveFile(data) != nil {
		return nil, false
	}
	return data, true
}

func (c *Curves) fetch(ctx context.Context, serial string) (Curve, error) {
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
		if c.store != nil {
			if err := c.store.Upload(ctx, storage.File{
				Body: bytes.NewReader(data), Size: int64(len(data)),
				ContentType: "application/octet-stream", Filename: serial + ".dat",
			}, SmartPTGCurveObjectKey(serial)); err != nil {
				// Still serve it; the next request fetches again.
				c.log.Warn("smartptg_curve_store_failed", "serial", serial, "error", err)
			}
		}
		return newCurve(serial, data), nil
	}
	if unavailable {
		return Curve{}, ErrCurveUpstreamUnavailable
	}
	return Curve{}, ErrCurveNotFound
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

func cmpLog(log *slog.Logger) *slog.Logger {
	if log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return log
}

func newCurve(serial string, data []byte) Curve {
	sum := sha256.Sum256(data)
	return Curve{Serial: serial, Data: data, ETag: `"` + hex.EncodeToString(sum[:16]) + `"`}
}
