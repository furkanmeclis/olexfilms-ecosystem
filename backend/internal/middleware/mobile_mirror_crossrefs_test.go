package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// TEC-1216: the cross-record detail endpoints the mobile RefRow opens are
// mirrored (organization and user summaries, the contract list and the
// single appointment / announcement / contract / warehouse reads); look-alike
// paths are not.
func TestMobilePanelMirrorTEC1216CrossRefs(t *testing.T) {
	id := uuid.NewString()
	for _, path := range []string{
		"tenant/organizations/" + id + "/summary",
		"tenant/users/" + id + "/summary",
		"contracts",
		"contracts/" + id,
		"appointments/" + id,
		"announcements/" + id,
		"warehouse/warehouses/" + id,
		"tasks/" + id,
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/"+path, nil)
		req.Header.Set(MobileAPIVersionHeader, "1")
		called := false
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called = true
			if r.URL.Path != "/v1/"+path || !IsMobilePanelMirror(r.Context()) {
				t.Errorf("rewrite = %s", r.URL.Path)
			}
		})).ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Errorf("%s was not allowed", path)
		}
	}
	for _, path := range []string{"tenant/usersx", "tenant/settings", "platform/users/" + id + "/overview"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/mobile/panel/"+path, nil)
		rec := httptest.NewRecorder()
		MobilePanelMirror(1, 1)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Errorf("%s reached next", path)
		})).ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d", path, rec.Code)
		}
	}
}
