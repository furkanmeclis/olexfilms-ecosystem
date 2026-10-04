package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
)

type offChecker struct{ key string }

func (o offChecker) Enabled(_ context.Context, _ int64, key string) (bool, error) {
	return key != o.key, nil
}

func TestRequireFeature(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	ctx := orgctx.WithScope(context.Background(), orgctx.Scope{InternalID: 7})
	req := httptest.NewRequest("GET", "/x", nil).WithContext(ctx)

	cases := []struct {
		name    string
		checker FeatureChecker
		want    int
	}{
		{"nil checker allows", nil, http.StatusOK},
		{"enabled module allows", offChecker{key: "module.y"}, http.StatusOK},
		{"disabled module 403", offChecker{key: "module.x"}, http.StatusForbidden},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		RequireFeature(tc.checker, "module.x")(ok).ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s: got %d want %d", tc.name, rec.Code, tc.want)
		}
	}
}

// TEC-342: the gate applies to one organization type only.
func TestRequireFeatureForOrgType(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	off := offChecker{key: "dealer_accounting"}
	cases := []struct {
		name    string
		orgType string
		checker FeatureChecker
		want    int
	}{
		{"dealer with module off", "dealer", off, http.StatusForbidden},
		{"dealer with module on", "dealer", offChecker{key: "other"}, http.StatusOK},
		{"distributor ignores the module", "distributor", off, http.StatusOK},
		{"center ignores the module", "center", off, http.StatusOK},
	}
	for _, tc := range cases {
		ctx := orgctx.WithScope(context.Background(), orgctx.Scope{InternalID: 7, OrgType: tc.orgType})
		req := httptest.NewRequest("POST", "/x", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		RequireFeatureForOrgType(tc.checker, "dealer", "dealer_accounting")(ok).ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s: got %d want %d", tc.name, rec.Code, tc.want)
		}
	}
	// Without an organization context the request passes (platform routes).
	rec := httptest.NewRecorder()
	RequireFeatureForOrgType(off, "dealer", "dealer_accounting")(ok).ServeHTTP(rec, httptest.NewRequest("POST", "/x", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("no org context: got %d", rec.Code)
	}
}
