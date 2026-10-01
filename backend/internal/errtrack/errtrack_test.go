package errtrack_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack/errtracktest"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

// initReceiver points the SDK at a fake envelope receiver (sync transport).
func initReceiver(t *testing.T) *errtracktest.Receiver {
	t.Helper()
	rcv := errtracktest.NewReceiver(t)
	tr := sentry.NewHTTPSyncTransport()
	tr.Timeout = 5 * time.Second
	on, err := errtrack.Init(errtrack.Options{
		DSN:         rcv.DSN(),
		Environment: "test",
		Release:     "v0.test",
		ServiceName: "server",
		Transport:   tr,
	})
	if err != nil || !on {
		t.Fatalf("init: on=%v err=%v", on, err)
	}
	t.Cleanup(func() { _, _ = errtrack.Init(errtrack.Options{}) })
	return rcv
}

func oneEvent(t *testing.T, rcv *errtracktest.Receiver) errtracktest.Event {
	t.Helper()
	errtrack.Flush(2 * time.Second)
	events := rcv.WaitEvents(1, 2*time.Second)
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	return events[0]
}

// chain mirrors httpserver: ServerErrors(RequestID(Middleware(Recover(mux)))).
func chain(h http.Handler) http.Handler {
	return middleware.ServerErrors(nil)(middleware.RequestID(errtrack.Middleware(errtrack.Recover(nil)(h))))
}

var (
	orgUUID  = uuid.MustParse("6f1c1c1e-1111-4a2b-9c3d-000000000001")
	userUUID = uuid.MustParse("6f1c1c1e-2222-4a2b-9c3d-000000000002")
)

// withTenant simulates Authenticate + RequireOrganization deep in the chain.
func withTenant(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := authctx.WithPrincipal(r.Context(), authctx.Principal{
			UserID: userUUID, Email: "owner@olexfilms.app",
		})
		ctx = orgctx.WithScope(ctx, orgctx.Scope{UUID: orgUUID, BrandID: 1, Name: "Ali Bayi"})
		next(w, r.WithContext(ctx))
	}
}

func TestHTTPInternalErrCarriesModuleAndOrganization(t *testing.T) {
	rcv := initReceiver(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/platform/organizations/{id}", withTenant(func(w http.ResponseWriter, r *http.Request) {
		response.InternalErr(w, r, errors.New("db failed for ali@example.com +90 555 123 45 67"), "failed")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/organizations/abc?phone=%2B905551234567", nil)
	req.Header.Set(middleware.RequestIDHeader, "req-123")
	rr := httptest.NewRecorder()
	chain(mux).ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rr.Code)
	}

	ev := oneEvent(t, rcv)
	want := map[string]string{
		"module":          "org",
		"organization_id": orgUUID.String(),
		"brand_id":        "1",
		"request_id":      "req-123",
		"component":       "server",
	}
	for k, v := range want {
		if ev.Tags[k] != v {
			t.Errorf("tag %s = %q, want %q (tags=%v)", k, ev.Tags[k], v, ev.Tags)
		}
	}
	if ev.Environment != "test" || ev.Release != "v0.test" {
		t.Errorf("env/release = %q/%q", ev.Environment, ev.Release)
	}
	if len(ev.Exceptions()) == 0 {
		t.Fatal("no exception")
	}
	val := ev.Exceptions()[len(ev.Exceptions())-1].Value
	if strings.Contains(val, "ali@example.com") || strings.Contains(val, "555 123") {
		t.Errorf("PII leaked in exception: %q", val)
	}
	if !strings.Contains(val, "[email]") || !strings.Contains(val, "[phone]") {
		t.Errorf("masks missing: %q", val)
	}
	if len(ev.User) != 1 || ev.User["id"] != userUUID.String() {
		t.Errorf("user must keep only id: %v", ev.User)
	}
	raw := string(ev.Raw)
	for _, leak := range []string{"owner@olexfilms.app", "905551234567", "Ali Bayi"} {
		if strings.Contains(raw, leak) {
			t.Errorf("payload leaks %q", leak)
		}
	}
	if u, _ := ev.Request["url"].(string); strings.Contains(u, "?") {
		t.Errorf("query kept in request url: %q", u)
	}
	if len(ev.Fingerprint) == 0 || ev.Fingerprint[0] != "org" {
		t.Errorf("fingerprint = %v", ev.Fingerprint)
	}
}

func TestHTTPPanicIsRecoveredAndReported(t *testing.T) {
	rcv := initReceiver(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/tenant/orders", func(http.ResponseWriter, *http.Request) {
		panic("nil order")
	})
	rr := httptest.NewRecorder()
	chain(mux).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tenant/orders", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rr.Code)
	}
	ev := oneEvent(t, rcv)
	if ev.Tags["module"] != "order" {
		t.Errorf("module = %q", ev.Tags["module"])
	}
	if ev.Tags["request_id"] == "" || ev.Tags["request_id"] != rr.Header().Get(middleware.RequestIDHeader) {
		t.Errorf("request_id = %q", ev.Tags["request_id"])
	}
	if v := ev.Exceptions(); len(v) == 0 || !strings.Contains(v[len(v)-1].Value, "panic: nil order") {
		t.Errorf("exception = %+v", v)
	}
}

func TestUnknownPathAndPinnedModule(t *testing.T) {
	rcv := initReceiver(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/platform/storage/x", func(w http.ResponseWriter, r *http.Request) {
		response.InternalErr(w, r, errors.New("unknown boom"), "failed")
	})
	mux.HandleFunc("GET /api/v1/platform/storage/pdf", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(errtrack.WithModule(r.Context(), errtrack.ModuleDocs))
		response.InternalErr(w, r, errors.New("pinned boom"), "failed")
	})
	chain(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/platform/storage/x", nil))
	chain(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/platform/storage/pdf", nil))
	errtrack.Flush(2 * time.Second)
	events := rcv.WaitEvents(2, 2*time.Second)
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d", len(events))
	}
	if events[0].Tags["module"] != "unknown" || events[1].Tags["module"] != "docs" {
		t.Errorf("modules = %q, %q", events[0].Tags["module"], events[1].Tags["module"])
	}
	if _, ok := events[0].Tags["organization_id"]; ok {
		t.Errorf("no org scope → no organization_id tag")
	}
}

func TestCaptureTaskTags(t *testing.T) {
	rcv := initReceiver(t)
	errtrack.CaptureTask(context.Background(), errtrack.TaskInfo{
		Type: "app:notification:deliver", Queue: "notifications", Retry: 2, MaxRetry: 25,
	}, errors.New("smtp down"))
	ev := oneEvent(t, rcv)
	for k, v := range map[string]string{
		"module": "notification", "task_type": "app:notification:deliver",
		"queue": "notifications", "retry": "2", "max_retry": "25",
	} {
		if ev.Tags[k] != v {
			t.Errorf("tag %s = %q, want %q", k, ev.Tags[k], v)
		}
	}
}

func TestDisabledIsNoop(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	on, err := errtrack.Init(errtrack.OptionsFromEnv("development", "server"))
	if on || err != nil {
		t.Fatalf("empty DSN must disable: on=%v err=%v", on, err)
	}
	if errtrack.Enabled() {
		t.Fatal("enabled without DSN")
	}
	if id := errtrack.Capture(context.Background(), errtrack.ModuleAuth, errors.New("x"), nil); id != nil {
		t.Fatal("capture must be no-op")
	}
	// Middleware/Recover still serve; panics still become 500.
	h := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("x") }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rr.Code)
	}
	if !errtrack.Flush(time.Millisecond) {
		t.Fatal("flush must be true when disabled")
	}
}

func TestOptionsFromEnv(t *testing.T) {
	t.Setenv("SENTRY_DSN", " https://k@e.example/1 ")
	t.Setenv("SENTRY_ENVIRONMENT", "")
	t.Setenv("SENTRY_RELEASE", "")
	errtrack.Release = "v0.9"
	t.Cleanup(func() { errtrack.Release = "" })
	o := errtrack.OptionsFromEnv("production", "worker")
	if o.DSN != "https://k@e.example/1" || o.Environment != "production" || o.Release != "v0.9" || o.ServiceName != "worker" {
		t.Fatalf("%+v", o)
	}
	t.Setenv("SENTRY_ENVIRONMENT", "staging")
	t.Setenv("SENTRY_RELEASE", "v1.0")
	o = errtrack.OptionsFromEnv("production", "worker")
	if o.Environment != "staging" || o.Release != "v1.0" {
		t.Fatalf("%+v", o)
	}
}

func TestNoDefaultPIICollection(t *testing.T) {
	initReceiver(t)
	dc := sentry.CurrentHub().Client().GetDataCollection()
	if dc.UserInfo.Value || len(dc.HTTPBodies) != 0 ||
		dc.Cookies.Mode != sentry.CollectionOff || dc.QueryParams.Mode != sentry.CollectionOff ||
		dc.HTTPHeaders.Request.Mode != sentry.CollectionOff {
		t.Fatalf("PII collection must be off: %+v", dc)
	}
}
