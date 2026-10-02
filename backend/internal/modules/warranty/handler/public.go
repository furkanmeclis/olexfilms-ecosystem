// Package handler holds the warranty HTTP handlers.
package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// publicRateAction is the limiter bucket of the public lookup.
const publicRateAction = "warranty_public"

// notFoundMessage is shared by every 404 of the public lookup so a
// malformed, unknown and other-brand code answer the same body.
const notFoundMessage = "Warranty was not found"

// Limiter is the per-IP fixed window limiter (ratelimit.Limiter).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Lookup resolves a public code inside a brand.
type Lookup interface {
	Lookup(ctx context.Context, brand usecase.PublicWarrantyBrand, brandID int64, code string) (usecase.PublicWarranty, error)
}

// Public serves GET /v1/public/warranties/{public_code} (TEC-189).
type Public struct {
	lookup  Lookup
	limiter Limiter
	limit   int
	window  time.Duration
}

// NewPublic builds the handler; limit hits per window per client IP.
func NewPublic(lookup Lookup, limiter Limiter, limit int, window time.Duration) *Public {
	return &Public{lookup: lookup, limiter: limiter, limit: limit, window: window}
}

// Get answers the public warranty page. No authentication. The rate limit
// runs before anything else, so malformed codes count as well; every miss
// (malformed, unknown, another brand's warranty) is the same 404.
func (h *Public) Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if h.limiter != nil {
		if ok, retry := h.limiter.Allow(r.Context(), publicRateAction, clientIP(r), h.limit, h.window); !ok {
			secs := int(retry.Seconds())
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			response.TooManyRequests(w, r, "")
			return
		}
	}
	code := r.PathValue("public_code")
	b, ok := brandctx.From(r.Context())
	if !ok || !usecase.ValidPublicCode(code) {
		response.NotFound(w, r, notFoundMessage)
		return
	}
	out, err := h.lookup.Lookup(r.Context(), usecase.PublicWarrantyBrand{Name: b.Name, Slug: b.Slug}, b.ID, code)
	if err != nil {
		if errors.Is(err, usecase.ErrPublicNotFound) {
			response.NotFound(w, r, notFoundMessage)
			return
		}
		response.InternalErr(w, r, err, "warranty lookup failed")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// clientIP is the client address the BFF forwards (first X-Forwarded-For
// hop, same as the other public limits), else the socket peer.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
