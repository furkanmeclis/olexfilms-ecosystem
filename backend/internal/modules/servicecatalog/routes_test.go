package servicecatalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

func TestRequireCenterRejectsDealerPost(t *testing.T) {
	called := false
	next := requireCenter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/platform/service-catalog", strings.NewReader(`{}`))
	req = req.WithContext(orgctx.WithScope(req.Context(), orgctx.Scope{
		InternalID: 30, UUID: uuid.New(), OrgType: rbac.OrgTypeDealer, BrandID: 1,
	}))
	rec := httptest.NewRecorder()
	next.ServeHTTP(rec, req)
	if called {
		t.Fatal("dealer reached platform service catalog handler")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	var env response.Envelope
	if err := jsonDecode(rec.Body.String(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Code != response.CodeForbidden {
		t.Fatalf("error = %+v", env.Error)
	}
}

func jsonDecode(body string, dst any) error {
	return json.NewDecoder(strings.NewReader(body)).Decode(dst)
}
