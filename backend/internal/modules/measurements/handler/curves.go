package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// CurveSource is the curve proxy usecase (usecase.Curves).
type CurveSource interface {
	Get(ctx context.Context, serial string) (usecase.Curve, error)
}

// Curves serves GET /v1/measurements/devices/smartptg/curves/{serial}
// (TEC-934): the raw (encrypted) factory curve file of a SmartPTG/NexPTG
// gauge. A serial's factory file never changes, so the answer is immutable.
type Curves struct {
	src CurveSource
}

// NewCurves builds the handler.
func NewCurves(src CurveSource) *Curves { return &Curves{src: src} }

// Get writes the curve bytes as application/octet-stream.
func (h *Curves) Get(w http.ResponseWriter, r *http.Request) {
	curve, err := h.src.Get(r.Context(), r.PathValue("serial"))
	switch {
	case errors.Is(err, usecase.ErrCurveSerialInvalid):
		response.BadRequest(w, r, "INVALID_SERIAL", "Gauge serial must be 1-9 digits")
		return
	case errors.Is(err, usecase.ErrCurveNotFound):
		response.NotFound(w, r, "Gauge curve not found")
		return
	case errors.Is(err, usecase.ErrCurveUpstreamUnavailable):
		w.Header().Set("Retry-After", "60")
		response.ServiceUnavailable(w, r, "CURVE_SOURCE_UNAVAILABLE", "Gauge curve source is unavailable")
		return
	case err != nil:
		response.InternalErr(w, r, err, "gauge curve failed")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
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
