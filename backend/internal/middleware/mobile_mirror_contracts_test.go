package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// TEC-1090 (TEC-1083): the contract routes (read, PDF, OTP, signatures,
// media, void) are mirrored; the template editor, the portal PDF and
// look-alike paths are not.
func TestMobilePanelMirrorTEC1090Contracts(t *testing.T) {
	id := uuid.NewString()
	for _, path := range []string{
		"contracts/" + id,
		"contracts/" + id + "/pdf",
		"contracts/" + id + "/signers/customer/otp",
		"contracts/" + id + "/signers/customer/sign",
		"contracts/" + id + "/signers/staff/sign",
		"services/" + id + "/contract",
		// TEC-1063 / TEC-1071 allowlist entries (routes in #515 / #523).
		"tenant/pending-recipients",
		"tenant/pending-recipients/" + id,
		"accounting/queue",
		"accounting/queue/" + id,
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/mobile/panel/"+path, nil)
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
	for _, path := range []string{
		"contract-templates/variables", "contractsx", "platform/contract-templates",
		"portal/contracts/" + id + "/pdf",
		"tenant/pending-recipientsx", "accounting/queuex", "accounting/accounts",
	} {
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
