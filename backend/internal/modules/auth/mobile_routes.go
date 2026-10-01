package auth

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
)

// RegisterMobileRoutes mounts the mobile API (TEC-91): /v1/mobile/* (every
// route checks X-Mobile-Api-Version first; Bearer with aud=mobile, see
// middleware.RealmAllows) and the web side of the QR sign-in.
func RegisterMobileRoutes(
	mux *http.ServeMux,
	h *handler.MobileHandler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	minVersion, maxVersion int,
) {
	version := middleware.MobileAPIVersion(minVersion, maxVersion)
	authn := middleware.Authenticate(tokens, loader)
	public := func(fn http.HandlerFunc) http.Handler { return middleware.Chain(fn, version) }
	private := func(fn http.HandlerFunc) http.Handler { return middleware.Chain(fn, version, authn) }

	mux.Handle("POST /v1/mobile/auth/login", public(h.Login))
	mux.Handle("POST /v1/mobile/auth/refresh", public(h.Refresh))
	mux.Handle("GET /v1/mobile/auth/me", private(h.Me))
	mux.Handle("POST /v1/mobile/auth/logout", private(h.Logout))
	mux.Handle("POST /v1/mobile/auth/logout-all", private(h.LogoutAll))
	mux.Handle("POST /v1/mobile/auth/organization-context", private(h.SwitchOrganization))

	mux.Handle("PUT /v1/mobile/push-token", private(h.PutPushToken))
	mux.Handle("DELETE /v1/mobile/push-token", private(h.DeletePushToken))

	mux.Handle("GET /v1/mobile/auth/qr/{code}", private(h.QRScan))
	mux.Handle("POST /v1/mobile/auth/qr/{code}/approve", private(h.QRApprove))
	mux.Handle("POST /v1/mobile/auth/qr/{code}/reject", private(h.QRReject))

	// K28: contract only until F3 (501).
	mux.Handle("POST /v1/mobile/measurements", private(h.CreateMeasurement))

	mux.HandleFunc("POST /v1/auth/qr/start", h.QRStart)
	mux.HandleFunc("GET /v1/auth/qr/{code}/status", h.QRStatus)
	mux.HandleFunc("POST /v1/auth/qr/complete", h.QRComplete)
}
