package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMobileAPIVersion(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := MobileAPIVersion(2, 3)(ok)
	cases := map[string]int{
		"":      http.StatusUpgradeRequired,
		"1":     http.StatusUpgradeRequired,
		"4":     http.StatusUpgradeRequired,
		"x":     http.StatusUpgradeRequired,
		"-2":    http.StatusUpgradeRequired,
		"2":     http.StatusNoContent,
		"2.7":   http.StatusNoContent,
		"v3":    http.StatusNoContent,
		" 3 ":   http.StatusNoContent,
		"3.0.1": http.StatusNoContent,
	}
	for v, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/v1/mobile/auth/me", nil)
		if v != "" {
			req.Header.Set(MobileAPIVersionHeader, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("version %q = %d, want %d", v, rec.Code, want)
		}
		if got := rec.Header().Get(MobileAPIVersionHeader); got != "3" {
			t.Errorf("version %q response header = %q, want 3", v, got)
		}
	}
}
