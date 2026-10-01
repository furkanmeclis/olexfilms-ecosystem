package auth

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/stepup"
)

// RegisterRoutes mounts auth and platform routes on mux.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	stepUp *stepup.Service,
) {
	authn := middleware.Authenticate(tokens, loader)
	requirePerm := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}
	var requireStepUp func(http.Handler) http.Handler
	if stepUp != nil {
		requireStepUp = middleware.RequireStepUp(stepUp)
	} else {
		requireStepUp = func(next http.Handler) http.Handler { return next }
	}

	mux.HandleFunc("POST /v1/auth/register", h.Register)
	mux.HandleFunc("POST /v1/auth/login", h.Login)
	mux.HandleFunc("POST /v1/auth/refresh", h.Refresh)
	mux.HandleFunc("POST /v1/auth/password/forgot", h.ForgotPassword)
	mux.HandleFunc("POST /v1/auth/password/reset", h.ResetPassword)
	mux.HandleFunc("POST /v1/auth/email/verify", h.VerifyEmail)
	mux.HandleFunc("POST /v1/auth/otp/request", h.OTPRequest)
	mux.HandleFunc("POST /v1/auth/otp/verify", h.OTPVerify)

	mux.HandleFunc("GET /v1/internal/auth/users/by-email", h.AdapterGetUserByEmail)
	mux.HandleFunc("GET /v1/internal/auth/users/{uuid}", h.AdapterGetUserByUUID)
	mux.HandleFunc("GET /v1/internal/auth/users/{uuid}/authenticators", h.AdapterListAuthenticators)
	mux.HandleFunc("GET /v1/internal/auth/authenticators/{credential_id}", h.AdapterGetAuthenticator)
	mux.HandleFunc("POST /v1/internal/auth/authenticators", h.AdapterCreateAuthenticator)
	mux.HandleFunc("PATCH /v1/internal/auth/authenticators/{credential_id}/counter", h.AdapterUpdateAuthenticatorCounter)
	mux.HandleFunc("DELETE /v1/internal/auth/authenticators/{credential_id}", h.AdapterDeleteAuthenticator)
	mux.HandleFunc("POST /v1/internal/auth/session", h.AdapterIssueSession)

	mux.HandleFunc("GET /v1/internal/auth/oauth/{provider}", h.AdapterGetOAuthConfig)
	mux.HandleFunc("POST /v1/internal/auth/users", h.AdapterCreateOAuthUser)
	mux.HandleFunc("GET /v1/internal/auth/accounts", h.AdapterGetOAuthAccount)
	mux.HandleFunc("POST /v1/internal/auth/accounts", h.AdapterLinkOAuthAccount)
	mux.HandleFunc("DELETE /v1/internal/auth/accounts", h.AdapterUnlinkOAuthAccount)

	mux.Handle("GET /v1/auth/identities", middleware.Chain(http.HandlerFunc(h.ListIdentities), authn))
	mux.Handle("DELETE /v1/auth/identities/{provider}", middleware.Chain(http.HandlerFunc(h.UnlinkIdentity), authn))

	mux.Handle("POST /v1/auth/logout", middleware.Chain(http.HandlerFunc(h.Logout), authn))
	mux.Handle("POST /v1/auth/organization-context", middleware.Chain(http.HandlerFunc(h.SwitchOrganizationContext), authn))
	mux.Handle("GET /v1/auth/me", middleware.Chain(http.HandlerFunc(h.Me), authn))
	mux.Handle("GET /v1/auth/passkeys", middleware.Chain(http.HandlerFunc(h.ListPasskeys), authn))
	mux.Handle("PATCH /v1/auth/passkeys/{uuid}", middleware.Chain(http.HandlerFunc(h.PatchPasskey), authn))
	mux.Handle("DELETE /v1/auth/passkeys/{uuid}", middleware.Chain(http.HandlerFunc(h.DeletePasskey), authn))
	mux.Handle("PATCH /v1/auth/profile", middleware.Chain(http.HandlerFunc(h.UpdateProfile), authn))
	mux.Handle("POST /v1/auth/password/change", middleware.Chain(http.HandlerFunc(h.ChangePassword), authn))
	mux.Handle("POST /v1/auth/email/verify-request", middleware.Chain(http.HandlerFunc(h.VerifyEmailRequest), authn))

	mux.Handle("GET /v1/auth/sessions", middleware.Chain(http.HandlerFunc(h.ListSessions), authn))
	mux.Handle("POST /v1/auth/sessions/revoke-others", middleware.Chain(http.HandlerFunc(h.RevokeOtherSessions), authn))
	mux.Handle("DELETE /v1/auth/sessions/{uuid}", middleware.Chain(http.HandlerFunc(h.RevokeSession), authn))

	mux.Handle("GET /v1/auth/step-up", middleware.Chain(http.HandlerFunc(h.StepUpStatus), authn))
	mux.Handle("POST /v1/auth/step-up/password", middleware.Chain(http.HandlerFunc(h.StepUpPassword), authn))
	mux.Handle("POST /v1/auth/step-up/passkey/options", middleware.Chain(http.HandlerFunc(h.StepUpPasskeyOptions), authn))
	mux.Handle("POST /v1/auth/step-up/passkey/verify", middleware.Chain(http.HandlerFunc(h.StepUpPasskeyVerify), authn))
	mux.Handle("POST /v1/auth/step-up/totp", middleware.Chain(http.HandlerFunc(h.StepUpTOTP), authn))

	mux.Handle("GET /v1/auth/totp", middleware.Chain(http.HandlerFunc(h.TOTPStatus), authn))
	mux.Handle("POST /v1/auth/totp/setup", middleware.Chain(http.HandlerFunc(h.TOTPSetup), authn))
	mux.Handle("POST /v1/auth/totp/confirm", middleware.Chain(http.HandlerFunc(h.TOTPConfirm), authn))
	mux.Handle("POST /v1/auth/totp/disable", middleware.Chain(http.HandlerFunc(h.TOTPDisable), authn))

	mux.Handle("GET /v1/platform/users/meta", middleware.Chain(
		http.HandlerFunc(h.PlatformUsersMeta), authn, requirePerm(rbac.PermPlatformUsersRead),
	))
	mux.Handle("GET /v1/platform/users", middleware.Chain(
		http.HandlerFunc(h.ListPlatformUsers), authn, requirePerm(rbac.PermPlatformUsersRead),
	))
	mux.Handle("POST /v1/platform/users", middleware.Chain(
		http.HandlerFunc(h.CreatePlatformUser), authn, requirePerm(rbac.PermPlatformUsersWrite),
	))
	mux.Handle("GET /v1/platform/users/{uuid}", middleware.Chain(
		http.HandlerFunc(h.GetPlatformUser), authn, requirePerm(rbac.PermPlatformUsersRead),
	))
	mux.Handle("PATCH /v1/platform/users/{uuid}", middleware.Chain(
		http.HandlerFunc(h.PatchPlatformUser), authn, requirePerm(rbac.PermPlatformUsersWrite),
	))
	mux.Handle("POST /v1/platform/users/{uuid}/password", middleware.Chain(
		http.HandlerFunc(h.SetPlatformUserPassword), authn, requirePerm(rbac.PermPlatformUsersWrite), requireStepUp,
	))
	mux.Handle("POST /v1/platform/users/{uuid}/impersonate", middleware.Chain(
		http.HandlerFunc(h.ImpersonatePlatformUser), authn, requirePerm(rbac.PermPlatformUsersImpersonate), requireStepUp,
	))

	mux.Handle("POST /v1/auth/impersonation/stop", middleware.Chain(
		http.HandlerFunc(h.StopImpersonation), authn,
	))

	mux.Handle("GET /v1/platform/roles/meta", middleware.Chain(
		http.HandlerFunc(h.PlatformRolesMeta), authn, requirePerm(rbac.PermPlatformRolesRead),
	))
	mux.Handle("GET /v1/platform/roles", middleware.Chain(
		http.HandlerFunc(h.ListPlatformRoles), authn, requirePerm(rbac.PermPlatformRolesRead),
	))
	mux.Handle("POST /v1/platform/roles", middleware.Chain(
		http.HandlerFunc(h.CreatePlatformRole), authn, requirePerm(rbac.PermPlatformRolesWrite),
	))
	mux.Handle("GET /v1/platform/roles/{uuid}", middleware.Chain(
		http.HandlerFunc(h.GetPlatformRole), authn, requirePerm(rbac.PermPlatformRolesRead),
	))
	mux.Handle("PATCH /v1/platform/roles/{uuid}", middleware.Chain(
		http.HandlerFunc(h.PatchPlatformRole), authn, requirePerm(rbac.PermPlatformRolesWrite),
	))
	mux.Handle("DELETE /v1/platform/roles/{uuid}", middleware.Chain(
		http.HandlerFunc(h.DeletePlatformRole), authn, requirePerm(rbac.PermPlatformRolesWrite),
	))

	mux.Handle("GET /v1/platform/permissions", middleware.Chain(
		http.HandlerFunc(h.ListPlatformPermissions), authn, requirePerm(rbac.PermPlatformRolesRead),
	))
}
