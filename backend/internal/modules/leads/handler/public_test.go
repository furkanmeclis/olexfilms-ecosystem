package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/google/uuid"
)

type fakeApps struct {
	enabled   bool
	submitted int
	spam      int
}

func (f *fakeApps) Enabled(context.Context) (bool, error) { return f.enabled, nil }

func (f *fakeApps) Validate(_ context.Context, in usecase.ApplicationInput) (usecase.Application, error) {
	if in.Honeypot != "" {
		return usecase.Application{Spam: true}, nil
	}
	if !in.KVKKConsent {
		return usecase.Application{}, &usecase.ValidationError{Field: "kvkk_consent", Message: "must be accepted"}
	}
	if in.Phone != "+905551234567" {
		return usecase.Application{}, &usecase.ValidationError{Field: "phone", Message: "must be a valid phone number"}
	}
	return usecase.Application{PhoneE164: in.Phone}, nil
}

func (f *fakeApps) Submit(_ context.Context, _ int64, app usecase.Application) (usecase.ApplicationResult, error) {
	if app.Spam {
		f.spam++
	} else {
		f.submitted++
	}
	return usecase.ApplicationResult{}, nil
}

func (f *fakeApps) ShowcaseLeadConfig(context.Context, int64, string, string, string, string) (usecase.ShowcaseLeadConfig, error) {
	return usecase.ShowcaseLeadConfig{}, nil
}

func (f *fakeApps) ValidateShowcaseLead(context.Context, int64, string, usecase.ShowcaseLeadInput) (usecase.ShowcaseLead, db.GetShowcaseLeadTargetBySlugRow, error) {
	return usecase.ShowcaseLead{}, db.GetShowcaseLeadTargetBySlugRow{}, nil
}

func (f *fakeApps) SubmitShowcaseLead(context.Context, db.GetShowcaseLeadTargetBySlugRow, usecase.ShowcaseLead) (usecase.ShowcaseLeadResult, error) {
	return usecase.ShowcaseLeadResult{}, nil
}

// countLimiter is an in-memory fixed window.
type countLimiter map[string]int

func (c countLimiter) Allow(_ context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration) {
	c[action+"|"+subject]++
	return c[action+"|"+subject] <= limit, window
}

func post(h *Public, ip, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/public/dealer-applications", strings.NewReader(body))
	req.Header.Set("X-Forwarded-For", ip)
	req = req.WithContext(brandctx.WithBrand(req.Context(), brandctx.Brand{ID: 1, Slug: "olex", Status: "active"}))
	rec := httptest.NewRecorder()
	h.Submit(rec, req)
	return rec
}

const validBody = `{"company_name":"A","contact_name":"B","phone":"+905551234567","country_id":1,"kvkk_consent":true}`

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return env.Error.Code
}

// TEC-317 acceptance: the closed form is 404 (and the config says so).
func TestPublicApplicationClosedIs404(t *testing.T) {
	apps := &fakeApps{}
	h := NewPublic(apps, countLimiter{}, RateLimits{IPLimit: 5, PhoneLimit: 3, Window: time.Hour})
	if rec := post(h, "203.0.113.1", validBody); rec.Code != http.StatusNotFound || apps.submitted != 0 {
		t.Fatalf("closed: %d submitted %d", rec.Code, apps.submitted)
	}
	rec := httptest.NewRecorder()
	h.Config(rec, httptest.NewRequest(http.MethodGet, "/v1/public/dealer-applications/config", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("config without brand: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/public/dealer-applications/config", nil)
	req = req.WithContext(brandctx.WithBrand(req.Context(), brandctx.Brand{ID: 1, Slug: "olex", Status: "active"}))
	rec = httptest.NewRecorder()
	h.Config(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("config: %d %s", rec.Code, rec.Body)
	}
}

// TEC-317 acceptance: invalid phone / no KVKK consent / bad JSON are 400
// VALIDATION_ERROR; a filled honeypot looks accepted but stores nothing.
func TestPublicApplicationValidation(t *testing.T) {
	apps := &fakeApps{enabled: true}
	h := NewPublic(apps, countLimiter{}, RateLimits{})
	for _, body := range []string{
		`{"company_name":"A","contact_name":"B","phone":"12","country_id":1,"kvkk_consent":true}`,
		`{"company_name":"A","contact_name":"B","phone":"+905551234567","country_id":1,"kvkk_consent":false}`,
		`{"unknown":1}`,
		`not json`,
	} {
		rec := post(h, "203.0.113.2", body)
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "VALIDATION_ERROR" {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	rec := post(h, "203.0.113.2", `{"company_name":"A","contact_name":"B","phone":"x","country_id":1,"website":"http://spam"}`)
	if rec.Code != http.StatusAccepted || apps.spam != 1 || apps.submitted != 0 {
		t.Fatalf("honeypot: %d spam %d submitted %d", rec.Code, apps.spam, apps.submitted)
	}
	if rec := post(h, "203.0.113.2", validBody); rec.Code != http.StatusAccepted || apps.submitted != 1 {
		t.Fatalf("valid: %d %s", rec.Code, rec.Body)
	}
}

// TEC-317 acceptance: over the per-IP or per-phone limit the form answers
// 429 with Retry-After.
func TestPublicApplicationRateLimits(t *testing.T) {
	apps := &fakeApps{enabled: true}
	h := NewPublic(apps, countLimiter{}, RateLimits{IPLimit: 3, PhoneLimit: 2, Window: time.Hour})
	for i := 0; i < 2; i++ {
		if rec := post(h, "198.51.100.1", validBody); rec.Code != http.StatusAccepted {
			t.Fatalf("hit %d: %d", i, rec.Code)
		}
	}
	// Same phone from another IP: the phone limit (2) is reached.
	rec := post(h, "198.51.100.2", validBody)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "3600" {
		t.Fatalf("phone limit: %d retry-after %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	// Third hit of the first IP still passes the IP limit (3) but not the
	// phone; the fourth is over the IP limit even with a bad body.
	_ = post(h, "198.51.100.1", validBody)
	rec = post(h, "198.51.100.1", `not json`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("ip limit: %d", rec.Code)
	}
	if apps.submitted != 2 {
		t.Fatalf("submitted = %d, want 2", apps.submitted)
	}
}

type fakePublicQuotes struct{ token uuid.UUID }

func (f fakePublicQuotes) PublicQuote(context.Context, int64, uuid.UUID, string) (usecase.PublicQuote, error) {
	return usecase.PublicQuote{}, nil
}

func (f fakePublicQuotes) PublicQuoteOwner(_ context.Context, _ int64, token uuid.UUID) (uuid.UUID, int64, int64, error) {
	if token != f.token {
		return uuid.Nil, 0, 0, usecase.ErrQuoteNotFound
	}
	return uuid.New(), 7, 1, nil
}

// fakeQuoteDocs renders on the second request and serves the bytes of
// organization 7 only.
type fakeQuoteDocs struct {
	calls  int
	render uuid.UUID
}

func (f *fakeQuoteDocs) RequestRender(context.Context, docmodel.Viewer, docusecase.RenderInput) (docmodel.RenderView, bool, error) {
	f.calls++
	return docmodel.RenderView{UUID: f.render, Status: "pending"}, f.calls > 1, nil
}

func (f *fakeQuoteDocs) Download(_ context.Context, orgID int64, id uuid.UUID) (io.ReadCloser, string, error) {
	if orgID != 7 || id != f.render {
		return nil, "", errors.New("wrong render")
	}
	return io.NopCloser(strings.NewReader("%PDF-1.7")), "Q-000042.pdf", nil
}

func getPDFFile(h *Public, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/public/quotes/"+token+"/pdf/file?locale=en", nil)
	req.SetPathValue("token", token)
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	req = req.WithContext(brandctx.WithBrand(req.Context(), brandctx.Brand{ID: 1, Slug: "olex", Status: "active"}))
	rec := httptest.NewRecorder()
	h.QuotePDFFile(rec, req)
	return rec
}

// TEC-320: the public quote PDF file route answers 202 while the render is
// queued, streams the PDF once ready, 404s an unknown token and shares the
// /pdf per-IP limit.
func TestPublicQuotePDFFile(t *testing.T) {
	token := uuid.New()
	docs := &fakeQuoteDocs{render: uuid.New()}
	h := NewPublic(&fakeApps{}, countLimiter{}, RateLimits{IPLimit: 3, Window: time.Hour}).
		WithQuotes(fakePublicQuotes{token: token}, docs)

	if rec := getPDFFile(h, token.String()); rec.Code != http.StatusAccepted {
		t.Fatalf("queued: %d", rec.Code)
	}
	rec := getPDFFile(h, token.String())
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" ||
		rec.Body.String() != "%PDF-1.7" || !strings.Contains(rec.Header().Get("Content-Disposition"), "Q-000042.pdf") {
		t.Fatalf("ready: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if rec := getPDFFile(h, uuid.NewString()); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown token: %d", rec.Code)
	}
	if rec := getPDFFile(h, token.String()); rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("limit: %d", rec.Code)
	}
}
