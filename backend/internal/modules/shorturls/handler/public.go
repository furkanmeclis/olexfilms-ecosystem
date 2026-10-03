// Package handler holds the short URL HTTP handlers (TEC-249).
package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// CodeShortURLExpired is the 410 error code of an expired token.
const CodeShortURLExpired = "SHORT_URL_EXPIRED"

// publicRateAction is the limiter bucket of the public resolver.
const publicRateAction = "short_url_public"

// notFoundMessage is shared by every 404 so a malformed, unknown and
// other-brand token answer the same body.
const notFoundMessage = "Short URL was not found"

// Limiter is the per-IP fixed window limiter (ratelimit.Limiter).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Resolver resolves a token inside a brand.
type Resolver interface {
	Resolve(ctx context.Context, brandID int64, token string) (usecase.Resolved, error)
}

// Public serves GET /v1/public/short-urls/{token}.
type Public struct {
	resolver Resolver
	limiter  Limiter
	limit    int
	window   time.Duration
}

// NewPublic builds the handler; limit hits per window per client IP.
func NewPublic(resolver Resolver, limiter Limiter, limit int, window time.Duration) *Public {
	return &Public{resolver: resolver, limiter: limiter, limit: limit, window: window}
}

// Get resolves a token of the request brand (domain, K3). No
// authentication; the per-IP limit runs first so guessing is throttled.
// 200: the internal target path (the frontend /s/{token} answers 302);
// 404: malformed, unknown or another brand's token; 410: expired.
func (h *Public) Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if !h.allow(w, r) {
		return
	}
	token := r.PathValue("token")
	b, ok := brandctx.From(r.Context())
	if !ok || !usecase.ValidToken(token) {
		response.NotFound(w, r, notFoundMessage)
		return
	}
	out, err := h.resolver.Resolve(r.Context(), b.ID, token)
	switch {
	case err == nil:
		response.JSON(w, r, http.StatusOK, out)
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, notFoundMessage)
	case errors.Is(err, usecase.ErrExpired):
		response.Error(w, r, http.StatusGone, CodeShortURLExpired, "Short URL has expired")
	default:
		response.InternalErr(w, r, err, "short url lookup failed")
	}
}

// WriteCreateError answers a failed usecase.Create: a rejected input (an
// external or not allowed target, a negative TTL) is 400 VALIDATION_ERROR,
// anything else 500. For the handlers that create short URLs.
func WriteCreateError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Code: ve.Code, Message: ve.Message}})
		return
	}
	response.InternalErr(w, r, err, "short url could not be created")
}

func (h *Public) allow(w http.ResponseWriter, r *http.Request) bool {
	if h.limiter == nil {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), publicRateAction, clientIP(r), h.limit, h.window)
	if ok {
		return true
	}
	secs := int(retry.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	response.TooManyRequests(w, r, "")
	return false
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
