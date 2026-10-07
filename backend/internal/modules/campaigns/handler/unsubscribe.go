package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/campaigns/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// unsubscribeRateAction is the limiter bucket of the public unsubscribe.
const unsubscribeRateAction = "campaign_unsubscribe"

// Limiter is the per-IP fixed window limiter (ratelimit.Limiter).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Unsubscribe serves POST /v1/public/campaigns/unsubscribe (TEC-407): the
// e-mail unsubscribe link's page posts its token here.
type Unsubscribe struct {
	uc      *usecase.Unsubscriber
	limiter Limiter
	limit   int
	window  time.Duration
}

// NewUnsubscribe builds the handler; limit hits per window per client IP.
func NewUnsubscribe(uc *usecase.Unsubscriber, limiter Limiter, limit int, window time.Duration) *Unsubscribe {
	return &Unsubscribe{uc: uc, limiter: limiter, limit: limit, window: window}
}

type unsubscribeBody struct {
	Token string `json:"token"`
}

// Post records the marketing opt-out of the token's user. No
// authentication; 200 {unsubscribed: true} (also when already opted out),
// 400 without a token, 404 for an invalid token.
func (h *Unsubscribe) Post(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.limiter != nil {
		if ok, retry := h.limiter.Allow(r.Context(), unsubscribeRateAction, clientIP(r), h.limit, h.window); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
			response.TooManyRequests(w, r, "")
			return
		}
	}
	var body unsubscribeBody
	if !decode(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Token) == "" {
		response.ValidationError(w, r, []response.Detail{{Field: "token", Message: "is required"}})
		return
	}
	err := h.uc.Unsubscribe(r.Context(), body.Token)
	switch {
	case err == nil:
		response.JSON(w, r, http.StatusOK, map[string]bool{"unsubscribed": true})
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "unsubscribe link is not valid")
	default:
		response.InternalErr(w, r, err, "unsubscribe failed")
	}
}

// clientIP is the client address the BFF forwards (first X-Forwarded-For
// hop), else the socket peer.
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
