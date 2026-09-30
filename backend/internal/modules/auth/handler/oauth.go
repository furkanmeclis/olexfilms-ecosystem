package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	githubusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/github/usecase"
	oauthproviderusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/oauthprovider/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

func (h *Handler) AdapterGetOAuthConfig(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	provider := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	registrationEnabled := false
	if h.authSettings != nil {
		enabled, err := h.authSettings.RegistrationEnabled(r.Context())
		if err == nil {
			registrationEnabled = enabled
		}
	}
	switch provider {
	case model.OAuthProviderGitHub:
		if h.github == nil {
			response.JSON(w, r, http.StatusOK, githubusecase.AdapterOAuthConfig{Enabled: false})
			return
		}
		cfg, err := h.github.GetAdapterOAuthConfig(r.Context())
		if err != nil {
			response.InternalErr(w, r, err, "failed to load GitHub OAuth config")
			return
		}
		cfg.RegisterAllowed = cfg.RegisterAllowed && registrationEnabled
		response.JSON(w, r, http.StatusOK, cfg)
	case model.OAuthProviderGoogle, model.OAuthProviderFacebook, model.OAuthProviderApple:
		if h.oauthProviders == nil {
			response.JSON(w, r, http.StatusOK, oauthproviderusecase.AdapterOAuthConfig{Enabled: false})
			return
		}
		cfg, err := h.oauthProviders.GetAdapterOAuthConfig(r.Context(), provider)
		if err != nil {
			response.InternalErr(w, r, err, "failed to load OAuth config")
			return
		}
		cfg.RegisterAllowed = cfg.RegisterAllowed && registrationEnabled
		response.JSON(w, r, http.StatusOK, cfg)
	default:
		response.NotFound(w, r, "Unknown OAuth provider")
	}
}

// AdapterGetGitHubOAuthConfig keeps the legacy path for compatibility.
func (h *Handler) AdapterGetGitHubOAuthConfig(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("provider", model.OAuthProviderGitHub)
	h.AdapterGetOAuthConfig(w, r)
}

func (h *Handler) AdapterCreateOAuthUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	var in struct {
		Email         string `json:"email"`
		Name          string `json:"name"`
		Surname       string `json:"surname"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	user, err := h.uc.RegisterOAuthUser(r.Context(), in.Email, in.Name, in.Surname, in.EmailVerified)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, user)
}

func (h *Handler) AdapterGetOAuthAccount(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	providerAccountID := strings.TrimSpace(r.URL.Query().Get("provider_account_id"))
	if h.oauth == nil {
		response.NotFound(w, r, "OAuth account not found")
		return
	}
	user, err := h.oauth.GetAdapterUserByOAuthAccount(r.Context(), provider, providerAccountID)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, user)
}

func (h *Handler) AdapterLinkOAuthAccount(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	var in struct {
		UserID            string  `json:"userId"`
		Provider          string  `json:"provider"`
		ProviderAccountID string  `json:"providerAccountId"`
		Type              string  `json:"type"`
		AccessToken       *string `json:"access_token"`
		RefreshToken      *string `json:"refresh_token"`
		ExpiresAt         *int64  `json:"expires_at"`
		TokenType         *string `json:"token_type"`
		Scope             *string `json:"scope"`
		GitHubLogin       *string `json:"github_login"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	var expiresAt *time.Time
	if in.ExpiresAt != nil && *in.ExpiresAt > 0 {
		t := time.Unix(*in.ExpiresAt, 0).UTC()
		expiresAt = &t
	}
	if h.oauth == nil {
		response.Internal(w, r, "OAuth integration is not configured")
		return
	}
	err := h.oauth.LinkOAuthAccount(r.Context(), model.LinkOAuthAccountInput{
		UserID:            in.UserID,
		Provider:          in.Provider,
		ProviderAccountID: in.ProviderAccountID,
		Type:              in.Type,
		AccessToken:       in.AccessToken,
		RefreshToken:      in.RefreshToken,
		ExpiresAt:         expiresAt,
		TokenType:         in.TokenType,
		Scope:             in.Scope,
		GitHubLogin:       in.GitHubLogin,
	})
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]string{"status": "linked"})
}

func (h *Handler) AdapterUnlinkOAuthAccount(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdapterKey(w, r) {
		return
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	providerAccountID := strings.TrimSpace(r.URL.Query().Get("provider_account_id"))
	if h.oauth == nil {
		response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	if err := h.oauth.UnlinkOAuthAccountByProviderAccount(r.Context(), provider, providerAccountID); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) ListIdentities(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		response.Unauthorized(w, r, "Authentication is required")
		return
	}
	if h.oauth == nil {
		response.JSON(w, r, http.StatusOK, model.IdentityList{Items: []model.LinkedIdentity{}, Total: 0})
		return
	}
	list, err := h.oauth.ListIdentities(r.Context(), p.UserID)
	if err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, list)
}

func (h *Handler) UnlinkIdentity(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		response.Unauthorized(w, r, "Authentication is required")
		return
	}
	provider := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	if !model.IsKnownOAuthProvider(provider) {
		response.NotFound(w, r, "Unknown OAuth provider")
		return
	}
	if h.oauth == nil {
		response.NotFound(w, r, "Linked identity not found")
		return
	}
	if err := h.oauth.UnlinkOAuthAccountForUser(r.Context(), p.UserID, provider); err != nil {
		writeUsecaseError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "unlinked"})
}

// UnlinkGitHubIdentity keeps the legacy path.
func (h *Handler) UnlinkGitHubIdentity(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("provider", model.OAuthProviderGitHub)
	h.UnlinkIdentity(w, r)
}
