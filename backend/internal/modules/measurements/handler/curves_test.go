package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
)

type fakeCurveSource map[string]error

func (f fakeCurveSource) Get(_ context.Context, serial string) (usecase.Curve, error) {
	if err, ok := f[serial]; ok {
		return usecase.Curve{}, err
	}
	return usecase.Curve{Serial: serial, Data: []byte("0123456789abcdef:AAAA"), ETag: `"abc"`}, nil
}

func TestCurvesHandler(t *testing.T) {
	h := NewCurves(fakeCurveSource{
		"x":    usecase.ErrCurveSerialInvalid,
		"1355": usecase.ErrCurveNotFound,
		"7":    usecase.ErrCurveUpstreamUnavailable,
		"8":    context.DeadlineExceeded,
		"9":    context.Canceled,
	}, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /c/{serial}", h.Get)
	do := func(serial, inm string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/c/"+serial, nil)
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := do("32475", "")
	if rec.Code != 200 || rec.Body.String() != "0123456789abcdef:AAAA" {
		t.Fatalf("200: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("content type %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=86400" {
		t.Fatalf("cache control %q", cc)
	}
	if rec.Header().Get("ETag") != `"abc"` {
		t.Fatal("etag")
	}
	if rec := do("32475", `W/"zzz", "abc"`); rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("304: %d", rec.Code)
	}
	// A cancelled follower gets no answer written (recorder default 200, empty body).
	if rec := do("9", ""); rec.Body.Len() != 0 {
		t.Fatalf("cancelled: %d %s", rec.Code, rec.Body.String())
	}
	for serial, want := range map[string]int{"x": 400, "1355": 404, "7": 503, "8": 503} {
		if rec := do(serial, ""); rec.Code != want {
			t.Fatalf("%s: %d, want %d", serial, rec.Code, want)
		}
	}
}

type denyLimiter struct{ subject string }

func (d *denyLimiter) Allow(_ context.Context, action, subject string, _ int, _ time.Duration) (bool, time.Duration) {
	d.subject = action + "|" + subject
	return false, 30 * time.Second
}

func TestCurvesHandlerRateLimited(t *testing.T) {
	lim := &denyLimiter{}
	h := NewCurves(fakeCurveSource{}, lim)
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest("GET", "/c/1", nil))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "30" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if lim.subject != "smartptg_curve|" {
		t.Fatalf("subject %q", lim.subject)
	}
}
