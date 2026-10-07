package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
	"github.com/google/uuid"
)

type fakeConsent struct {
	to  string
	err error
	got model.AuthorizeInput
}

func (f *fakeConsent) Authorize(_ context.Context, _ string, in model.AuthorizeInput) (string, error) {
	f.got = in
	return f.to, f.err
}
func (f *fakeConsent) Consent(context.Context, usecase.Actor, uuid.UUID) (model.Consent, error) {
	return model.Consent{}, nil
}
func (f *fakeConsent) Decide(context.Context, usecase.Actor, uuid.UUID, model.DecideInput) (model.Decision, error) {
	return model.Decision{}, nil
}
func (f *fakeConsent) ListGrants(context.Context, int64, usecase.ListFilter) ([]model.Grant, int64, error) {
	return nil, 0, nil
}
func (f *fakeConsent) RevokeGrant(context.Context, usecase.Actor, uuid.UUID) error { return nil }
func (f *fakeConsent) ListClients(context.Context, usecase.ListFilter) ([]model.ClientSummary, int64, error) {
	return nil, 0, nil
}
func (f *fakeConsent) RevokeClient(context.Context, usecase.Actor, uuid.UUID) error { return nil }

// TEC-401: /oauth/authorize redirects on success and renders an escaped
// error page (never a redirect) for non-redirectable errors.
func TestAuthorizeEndpoint(t *testing.T) {
	f := &fakeConsent{to: "https://olexfilms.test/oauth/consent?request=abc"}
	c := NewConsent(f, New(nil, nil))
	rec := httptest.NewRecorder()
	c.Authorize(rec, httptest.NewRequest(http.MethodGet, "/oauth/authorize?client_id=mcp_x&redirect_uri=https%3A%2F%2Fc.example%2Fcb&response_type=code&code_challenge=ch&code_challenge_method=S256&state=s&resource=r&scope=mcp", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != f.to {
		t.Fatalf("authorize = %d %v", rec.Code, rec.Header())
	}
	want := model.AuthorizeInput{ClientID: "mcp_x", RedirectURI: "https://c.example/cb", ResponseType: "code",
		CodeChallenge: "ch", CodeChallengeMethod: "S256", State: "s", Scope: "mcp", Resource: "r"}
	if f.got != want {
		t.Fatalf("input = %+v", f.got)
	}

	f.to, f.err = "", model.BadRequest(model.ErrInvalidRequest, `redirect_uri <script>`)
	rec = httptest.NewRecorder()
	c.Authorize(rec, httptest.NewRequest(http.MethodGet, "/oauth/authorize", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusBadRequest || rec.Header().Get("Location") != "" ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") || strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("error page = %d %v %s", rec.Code, rec.Header(), body)
	}

	f.err = &model.Error{Status: http.StatusTooManyRequests, Code: model.ErrSlowDown, RetryAfter: 30 * time.Second}
	rec = httptest.NewRecorder()
	c.Authorize(rec, httptest.NewRequest(http.MethodGet, "/oauth/authorize", nil))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limited = %d %v", rec.Code, rec.Header())
	}
}
