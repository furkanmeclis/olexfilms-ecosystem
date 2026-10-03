package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
)

type fakeResolver struct {
	out     usecase.Resolved
	err     error
	brandID int64
	calls   int
}

func (f *fakeResolver) Resolve(_ context.Context, brandID int64, token string) (usecase.Resolved, error) {
	f.calls++
	f.brandID = brandID
	f.out.Token = token
	return f.out, f.err
}

type denyLimiter struct{}

func (denyLimiter) Allow(context.Context, string, string, int, time.Duration) (bool, time.Duration) {
	return false, 30 * time.Second
}

func serve(h *Public, token string, withBrand bool) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/public/short-urls/{token}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/v1/public/short-urls/"+token, nil)
	if withBrand {
		req = req.WithContext(brandctx.WithBrand(req.Context(), brandctx.Brand{ID: 7, Slug: "olex"}))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

type envelope struct {
	Success bool `json:"success"`
	Data    struct {
		Token      string `json:"token"`
		TargetPath string `json:"target_path"`
	} `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Details []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"details"`
	} `json:"error"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return e
}

func TestPublicGetResolves(t *testing.T) {
	f := &fakeResolver{out: usecase.Resolved{TargetPath: "/garanti/AbCdEfGh"}}
	rec := serve(NewPublic(f, nil, 0, 0), "aZ3kP9qX", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	e := decode(t, rec)
	if e.Data.TargetPath != "/garanti/AbCdEfGh" || e.Data.Token != "aZ3kP9qX" || f.brandID != 7 {
		t.Fatalf("body %+v brand %d", e, f.brandID)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cache-control")
	}
}

func TestPublicGetStatuses(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		token  string
		brand  bool
		status int
		code   string
	}{
		{"expired", usecase.ErrExpired, "AbCdEfGhIj", true, http.StatusGone, CodeShortURLExpired},
		{"unknown", usecase.ErrNotFound, "AbCdEfGhIj", true, http.StatusNotFound, "NOT_FOUND"},
		{"malformed", nil, "ab-cd", true, http.StatusNotFound, "NOT_FOUND"},
		{"no brand", nil, "AbCdEfGhIj", false, http.StatusNotFound, "NOT_FOUND"},
		{"failure", errors.New("db down"), "AbCdEfGhIj", true, http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		f := &fakeResolver{err: tc.err}
		rec := serve(NewPublic(f, nil, 0, 0), tc.token, tc.brand)
		if rec.Code != tc.status {
			t.Fatalf("%s: status %d", tc.name, rec.Code)
		}
		if e := decode(t, rec); e.Error.Code != tc.code {
			t.Fatalf("%s: code %q", tc.name, e.Error.Code)
		}
		if (tc.token == "ab-cd" || !tc.brand) && f.calls != 0 {
			t.Fatalf("%s: resolver called", tc.name)
		}
	}
}

func TestPublicGetRateLimited(t *testing.T) {
	f := &fakeResolver{}
	rec := serve(NewPublic(f, denyLimiter{}, 1, time.Minute), "AbCdEfGhIj", true)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "30" || f.calls != 0 {
		t.Fatalf("status %d retry %q calls %d", rec.Code, rec.Header().Get("Retry-After"), f.calls)
	}
}

func TestWriteCreateErrorExternalTarget(t *testing.T) {
	_, err := usecase.NormalizeTarget("https://evil.example/portal")
	rec := httptest.NewRecorder()
	WriteCreateError(rec, httptest.NewRequest(http.MethodPost, "/", nil), err)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	e := decode(t, rec)
	if e.Error.Code != "VALIDATION_ERROR" || len(e.Error.Details) != 1 || e.Error.Details[0].Field != "target" {
		t.Fatalf("body %+v", e)
	}
}
