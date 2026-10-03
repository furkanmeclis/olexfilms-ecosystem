package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/redis/go-redis/v9"
)

const goodCode = "AbCdEfGhIjKlMnOpQrSt_-"

type fakeLookup struct{ calls int }

func (f *fakeLookup) Lookup(_ context.Context, brand usecase.PublicWarrantyBrand, _ int64, code string) (usecase.PublicWarranty, error) {
	f.calls++
	if code != goodCode {
		return usecase.PublicWarranty{}, usecase.ErrPublicNotFound
	}
	return usecase.PublicWarranty{PublicCode: code, Status: "active", Brand: brand}, nil
}

func newMux(t *testing.T, lookup Lookup, limit int) *http.ServeMux {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/public/warranties/{public_code}", NewPublic(lookup, ratelimit.New(rdb, "test"), limit, time.Minute).Get)
	return mux
}

func get(mux http.Handler, code, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/public/warranties/"+code, nil)
	req.Header.Set("X-Forwarded-For", ip)
	req = req.WithContext(brandctx.WithBrand(req.Context(), brandctx.Brand{ID: 1, Slug: "olex", Name: "Olex", Status: "active"}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func errorBody(t *testing.T, rec *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Error.Code, env.Error.Message
}

func TestPublicGetFound(t *testing.T) {
	f := &fakeLookup{}
	rec := get(newMux(t, f, 30), goodCode, "10.0.0.1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Robots-Tag") == "" {
		t.Fatalf("headers = %v", rec.Header())
	}
}

// Malformed codes never reach the lookup; malformed and unknown codes give
// the same 404 body.
func TestPublicGetNotFoundIsUniform(t *testing.T) {
	f := &fakeLookup{}
	mux := newMux(t, f, 100)
	var bodies [][2]string
	for _, code := range []string{"abc", "has%20space", "x" + goodCode + "toolongtoolong", "!!!!!!!!!!!!!!!!!!!!!!"} {
		rec := get(mux, code, "10.0.0.2")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%q: status %d", code, rec.Code)
		}
		c, m := errorBody(t, rec)
		bodies = append(bodies, [2]string{c, m})
	}
	if f.calls != 0 {
		t.Fatalf("malformed codes reached the lookup %d times", f.calls)
	}
	rec := get(mux, "ZZZZZZZZZZZZZZZZZZZZZZ", "10.0.0.2")
	if rec.Code != http.StatusNotFound || f.calls != 1 {
		t.Fatalf("unknown: status %d calls %d", rec.Code, f.calls)
	}
	c, m := errorBody(t, rec)
	for _, b := range bodies {
		if b[0] != c || b[1] != m {
			t.Fatalf("404 bodies differ: %v vs %s/%s", b, c, m)
		}
	}
}

func TestPublicGetRateLimit(t *testing.T) {
	f := &fakeLookup{}
	mux := newMux(t, f, 3)
	for i := 0; i < 3; i++ {
		if rec := get(mux, goodCode, "10.0.0.3"); rec.Code != http.StatusOK {
			t.Fatalf("hit %d: %d", i+1, rec.Code)
		}
	}
	rec := get(mux, goodCode, "10.0.0.3")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("over limit: %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	if c, _ := errorBody(t, rec); c != "RATE_LIMITED" {
		t.Fatalf("code = %s", c)
	}
	// Malformed codes count against the same bucket.
	if rec := get(mux, "bad", "10.0.0.3"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("malformed over limit: %d", rec.Code)
	}
	// Another IP has its own bucket.
	if rec := get(mux, goodCode, "10.0.0.4"); rec.Code != http.StatusOK {
		t.Fatalf("other ip: %d", rec.Code)
	}
}
