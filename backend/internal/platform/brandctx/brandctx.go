// Package brandctx carries the request brand (K1, K3, K20).
//
// The brand of a request comes from the host it was made on: the BFF forwards
// the browser host as X-Forwarded-Host, and brand_domains maps hosts to
// brands. Unknown hosts fall back to the configured default brand.
package brandctx

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ctxKey int

const keyBrand ctxKey = 1

// Brand is the brand resolved for a request.
type Brand struct {
	ID     int64
	UUID   uuid.UUID
	Slug   string
	Name   string
	Status string
}

// Active reports whether the brand accepts sign-ups and tenant traffic.
func (b Brand) Active() bool { return b.Status == "active" }

// WithBrand stores the brand on the context.
func WithBrand(ctx context.Context, b Brand) context.Context {
	return context.WithValue(ctx, keyBrand, b)
}

// From returns the request brand when present.
func From(ctx context.Context) (Brand, bool) {
	b, ok := ctx.Value(keyBrand).(Brand)
	return b, ok
}

// NormalizeHost lowercases a Host / X-Forwarded-Host value and strips the
// port. For a comma separated X-Forwarded-Host the first (client-most) value
// wins.
func NormalizeHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, ','); i >= 0 {
		raw = strings.TrimSpace(raw[:i])
	}
	if raw == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(raw); err == nil {
		raw = h
	}
	raw = strings.TrimSuffix(strings.Trim(raw, "[]"), ".")
	return strings.ToLower(raw)
}

// Catalog is a snapshot of brands and their hosts.
type Catalog struct {
	BySlug map[string]Brand
	ByHost map[string]Brand
}

// Loader reads the full catalog (brands are a handful of rows).
type Loader func(ctx context.Context) (Catalog, error)

// Resolver resolves hosts to brands with a short in-memory cache.
type Resolver struct {
	load        Loader
	defaultSlug string
	ttl         time.Duration
	now         func() time.Time

	mu       sync.Mutex
	catalog  Catalog
	loadedAt time.Time
}

// NewResolver builds a resolver. ttl <= 0 uses 60 seconds.
func NewResolver(load Loader, defaultSlug string, ttl time.Duration) *Resolver {
	if ttl <= 0 {
		ttl = time.Minute
	}
	defaultSlug = strings.ToLower(strings.TrimSpace(defaultSlug))
	if defaultSlug == "" {
		defaultSlug = "olex"
	}
	return &Resolver{load: load, defaultSlug: defaultSlug, ttl: ttl, now: time.Now}
}

func (r *Resolver) snapshot(ctx context.Context) (Catalog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.catalog.BySlug != nil && r.now().Sub(r.loadedAt) < r.ttl {
		return r.catalog, nil
	}
	c, err := r.load(ctx)
	if err != nil {
		if r.catalog.BySlug != nil {
			// Serve the stale snapshot rather than fail every request.
			return r.catalog, nil
		}
		return Catalog{}, err
	}
	r.catalog, r.loadedAt = c, r.now()
	return c, nil
}

// Resolve maps a host to its brand, falling back to the default brand.
func (r *Resolver) Resolve(ctx context.Context, host string) (Brand, bool, error) {
	c, err := r.snapshot(ctx)
	if err != nil {
		return Brand{}, false, err
	}
	if b, ok := c.ByHost[NormalizeHost(host)]; ok {
		return b, true, nil
	}
	b, ok := c.BySlug[r.defaultSlug]
	return b, ok, nil
}

// Invalidate drops the cached catalog.
func (r *Resolver) Invalidate() {
	r.mu.Lock()
	r.catalog = Catalog{}
	r.mu.Unlock()
}
