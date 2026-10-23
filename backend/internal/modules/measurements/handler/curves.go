package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// CurveSource is the curve proxy usecase (usecase.Curves).
type CurveSource interface {
	Get(ctx context.Context, serial string) (usecase.Curve, error)
}

// CurveLimiter is the platform rate limiter (ratelimit.Limiter).
type CurveLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Per-user cap on curve requests: the app keeps a curve once it has it, so
// a phone asks a handful of times; this only stops one client walking many
// serials through us to the vendor.
const (
	curveRateAction = "smartptg_curve"
	curveRateLimit  = 60
	curveRateWindow = 10 * time.Minute
)

// Curves serves GET /v1/measurements/devices/smartptg/curves/{serial}
// (TEC-934): the raw (encrypted) factory curve file of a SmartPTG/NexPTG
// gauge. Cached a day by the client with an ETag; not immutable, because a
// re-validation against the vendor may replace the stored copy.
type Curves struct {
	src     CurveSource
	limiter CurveLimiter
}

// NewCurves builds the handler; limiter may be nil (no limit).
func NewCurves(src CurveSource, limiter CurveLimiter) *Curves {
	return &Curves{src: src, limiter: limiter}
}

// Get writes the curve bytes as application/octet-stream.
func (h *Curves) Get(w http.ResponseWriter, r *http.Request) {
	if h.limiter != nil {
		subject := ""
		if p, ok := authctx.PrincipalFrom(r.Context()); ok {
			subject = p.UserID.String()
		}
		if ok, retry := h.limiter.Allow(r.Context(), curveRateAction, subject, curveRateLimit, curveRateWindow); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
			response.TooManyRequests(w, r, "")
			return
		}
	}
	curve, err := h.src.Get(r.Context(), r.PathValue("serial"))
	switch {
	case errors.Is(err, context.Canceled):
		// The client went away while waiting on a shared fetch: nobody to answer.
		return
	case errors.Is(err, usecase.ErrCurveSerialInvalid):
		response.BadRequest(w, r, "INVALID_SERIAL", "Gauge serial must be 1-9 digits")
		return
	case errors.Is(err, usecase.ErrCurveNotFound):
		response.NotFound(w, r, "Gauge curve not found")
		return
	case errors.Is(err, usecase.ErrCurveUpstreamUnavailable), errors.Is(err, context.DeadlineExceeded):
		w.Header().Set("Retry-After", "60")
		response.ServiceUnavailable(w, r, "CURVE_SOURCE_UNAVAILABLE", "Gauge curve source is unavailable")
		return
	case err != nil:
		response.InternalErr(w, r, err, "gauge curve failed")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("ETag", curve.ETag)
	if match := r.Header.Get("If-None-Match"); match != "" && etagMatches(match, curve.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(curve.Data)))
	w.Header().Set("Content-Disposition", `attachment; filename="`+curve.Serial+`.dat"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(curve.Data)
}

func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "W/"))
		if part == "*" || part == etag {
			return true
		}
	}
	return false
}
