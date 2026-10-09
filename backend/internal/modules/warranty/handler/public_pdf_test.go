package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/redis/go-redis/v9"
)

// legacyCode is an old hub service number ("DS" + 8), kept by the migrator.
const legacyCode = "DS7K2M9QX4"

type pdfLookup struct{ calls int }

func strp(s string) *string { return &s }

func (f *pdfLookup) Lookup(_ context.Context, brand usecase.PublicWarrantyBrand, _ int64, code string) (usecase.PublicWarranty, error) {
	f.calls++
	status := "active"
	switch code {
	case goodCode, legacyCode:
	case "ExpiredCode1234":
		status = "expired"
	default:
		return usecase.PublicWarranty{}, usecase.ErrPublicNotFound
	}
	return usecase.PublicWarranty{
		PublicCode: code, Status: status, Brand: brand,
		StartAt: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC), EndAt: time.Date(2031, 1, 15, 20, 59, 59, 0, time.UTC),
		Product: usecase.PublicProduct{Name: "Olex PPF Gloss"},
		Dealer:  usecase.PublicDealer{Name: "Dealer A", City: "İstanbul"},
		Vehicle: usecase.PublicWarrantyCar{BrandName: "BMW", ModelName: "M3",
			PlateMasked: strp("34 *** 12"), VINLast4: strp("6752")},
	}, nil
}

type fakeRenderer struct {
	html string
	fail bool
}

func (f *fakeRenderer) Configured() bool { return true }

func (f *fakeRenderer) HTMLToPDF(_ context.Context, html string) ([]byte, error) {
	if f.fail {
		return nil, errors.New("gotenberg down")
	}
	f.html = html
	return []byte("%PDF-1.7 fake"), nil
}

func newPDFMux(t *testing.T, lookup Lookup, r PDFRenderer, pdfLimit int) *http.ServeMux {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	h := NewPublic(lookup, ratelimit.New(rdb, "test"), 100, time.Minute).WithPDF(r, "https://olexfilms.app/", pdfLimit)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/public/warranties/{public_code}", h.Get)
	mux.HandleFunc("GET /v1/public/warranties/{public_code}/pdf", h.PDF)
	return mux
}

func getPDF(mux http.Handler, code, ip, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/public/warranties/"+code+"/pdf"+query, nil)
	req.Header.Set("X-Forwarded-For", ip)
	req = req.WithContext(brandctx.WithBrand(req.Context(), brandctx.Brand{ID: 1, Slug: "olex", Name: "Olex", Status: "active"}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestPublicPDFValidCode(t *testing.T) {
	r := &fakeRenderer{}
	rec := getPDF(newPDFMux(t, &pdfLookup{}, r, 5), goodCode, "10.1.0.1", "?lang=en&tz=Europe/Istanbul")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("content type %q", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", rec.Header())
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF")) {
		t.Fatalf("body = %q", rec.Body.String())
	}
	for _, want := range []string{"34 *** 12", "6752", "Olex PPF Gloss", "https://olexfilms.app/garanti/" + goodCode, `lang="en"`} {
		if !strings.Contains(r.html, want) {
			t.Errorf("document lacks %q", want)
		}
	}
	// Dates are printed in the requested zone: 20:59:59Z is 23:59:59 in Istanbul.
	if !strings.Contains(r.html, "2031-01-15") {
		t.Error("end date not in the requested zone")
	}
}

func TestPublicPDFNotFound(t *testing.T) {
	f := &pdfLookup{}
	r := &fakeRenderer{}
	mux := newPDFMux(t, f, r, 100)
	for _, code := range []string{"ZZZZZZZZZZZZZZZZZZZZZZ", "abc", "!!!!!!!!!!!!", "ExpiredCode1234"} {
		rec := getPDF(mux, code, "10.1.0.2", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%q: status %d", code, rec.Code)
		}
		if c, _ := errorBody(t, rec); c != "NOT_FOUND" {
			t.Fatalf("%q: code %s", code, c)
		}
	}
	if r.html != "" {
		t.Fatal("a miss reached the renderer")
	}
}

func TestPublicPDFRateLimit(t *testing.T) {
	mux := newPDFMux(t, &pdfLookup{}, &fakeRenderer{}, 2)
	for i := 0; i < 2; i++ {
		if rec := getPDF(mux, goodCode, "10.1.0.3", ""); rec.Code != http.StatusOK {
			t.Fatalf("hit %d: %d", i+1, rec.Code)
		}
	}
	rec := getPDF(mux, goodCode, "10.1.0.3", "")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("over limit: %d %v", rec.Code, rec.Header())
	}
	if c, _ := errorBody(t, rec); c != "RATE_LIMITED" {
		t.Fatalf("code = %s", c)
	}
	// The PDF bucket is separate: the page lookup still answers.
	if rec := get(mux, goodCode, "10.1.0.3"); rec.Code != http.StatusOK {
		t.Fatalf("lookup after pdf limit: %d", rec.Code)
	}
}

// The old hub's warranty numbers pass the path check and reach the lookup
// on both the page endpoint and the PDF.
func TestPublicLegacyCodeAccepted(t *testing.T) {
	f := &pdfLookup{}
	mux := newPDFMux(t, f, &fakeRenderer{}, 5)
	if rec := get(mux, legacyCode, "10.1.0.4"); rec.Code != http.StatusOK {
		t.Fatalf("lookup: %d %s", rec.Code, rec.Body)
	}
	if rec := getPDF(mux, legacyCode, "10.1.0.4", ""); rec.Code != http.StatusOK {
		t.Fatalf("pdf: %d %s", rec.Code, rec.Body)
	}
	if f.calls != 2 {
		t.Fatalf("lookup calls = %d", f.calls)
	}
}

func TestPublicPDFRendererFailure(t *testing.T) {
	rec := getPDF(newPDFMux(t, &pdfLookup{}, &fakeRenderer{fail: true}, 5), goodCode, "10.1.0.5", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestPublicPDFNotConfigured(t *testing.T) {
	rec := getPDF(newPDFMux(t, &pdfLookup{}, nil, 5), goodCode, "10.1.0.6", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
}

// TEC-521: the issued-at line is printed in the tz query zone; a missing or
// invalid tz is Europe/Istanbul, never UTC.
func TestPublicPDFIssuedAtTimezone(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	r := &fakeRenderer{}
	h := NewPublic(&pdfLookup{}, ratelimit.New(rdb, "test"), 100, time.Minute).WithPDF(r, "https://olexfilms.app/", 100)
	h.now = func() time.Time { return time.Date(2026, 10, 9, 11, 32, 0, 0, time.UTC) }
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/public/warranties/{public_code}/pdf", h.PDF)

	for _, tc := range []struct{ name, query, want string }{
		{"istanbul tr", "?lang=tr&tz=Europe%2FIstanbul", "Düzenlenme: 2026-10-09 14:32 (Europe/Istanbul)"},
		{"istanbul ar", "?lang=ar&tz=Europe%2FIstanbul", "2026-10-09 14:32 (Europe/Istanbul)"},
		{"other zone", "?lang=en&tz=America%2FNew_York", "2026-10-09 07:32 (America/New_York)"},
		{"missing tz", "?lang=tr", "Düzenlenme: 2026-10-09 14:32 (Europe/Istanbul)"},
		{"invalid tz", "?lang=tr&tz=Not%2FAZone", "Düzenlenme: 2026-10-09 14:32 (Europe/Istanbul)"},
		{"local tz", "?lang=tr&tz=Local", "Düzenlenme: 2026-10-09 14:32 (Europe/Istanbul)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := getPDF(mux, goodCode, "10.1.9.1", tc.query)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if !strings.Contains(r.html, tc.want) {
				t.Fatalf("document lacks %q", tc.want)
			}
			if strings.Contains(r.html, "11:32") || strings.Contains(r.html, " UTC") {
				t.Fatal("issued-at is still printed in UTC")
			}
		})
	}
}
