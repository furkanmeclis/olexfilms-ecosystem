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
		{"allow-all allows", AllowAllFeatures{}, http.StatusOK},
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
