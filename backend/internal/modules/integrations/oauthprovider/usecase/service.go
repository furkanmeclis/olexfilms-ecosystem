package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidRequest  = errors.New("invalid request")
	ErrUnknownProvider = errors.New("unknown oauth provider")
)

var knownProviders = map[string]struct{}{
	"google":   {},
	"facebook": {},
	"apple":    {},
}

// IsKnownProvider reports whether provider is google/facebook/apple.
func IsKnownProvider(provider string) bool {
	_, ok := knownProviders[strings.ToLower(strings.TrimSpace(provider))]
	return ok
}

type settingsStore interface {
	GetOAuthProviderSettings(ctx context.Context, provider string) (db.OauthProviderSetting, error)
	ListOAuthProviderSettings(ctx context.Context) ([]db.OauthProviderSetting, error)
	UpdateOAuthProviderSettings(ctx context.Context, arg db.UpdateOAuthProviderSettingsParams) (db.OauthProviderSetting, error)
}

// Service manages oauth_provider_settings rows.
type Service struct {
	q   settingsStore
	box *crypto.SecretBox
}

// New creates an OAuth provider settings service.
func New(q settingsStore, box *crypto.SecretBox) *Service {
	return &Service{q: q, box: box}
}

// Settings is the public admin payload (no secret values).
type Settings struct {
	Provider               string `json:"provider"`
	LoginEnabled           bool   `json:"login_enabled"`
	RegisterEnabled        bool   `json:"register_enabled"`
	ClientID               string `json:"client_id"`
	ClientSecretConfigured bool   `json:"client_secret_configured"`
}

// AdapterOAuthConfig is returned to NextAuth via the internal adapter API.
type AdapterOAuthConfig struct {
	Enabled         bool   `json:"enabled"`
	RegisterAllowed bool   `json:"register_allowed"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"`
}

// PatchInput partial update for provider settings.
type PatchInput struct {
	LoginEnabled    *bool   `json:"login_enabled"`
	RegisterEnabled *bool   `json:"register_enabled"`
	ClientID        *string `json:"client_id"`
	ClientSecret    *string `json:"client_secret"`
}

func normalizeProvider(provider string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(provider))
	if !IsKnownProvider(p) {
		return "", ErrUnknownProvider
	}
	return p, nil
}

func mapSettings(row db.OauthProviderSetting) Settings {
	return Settings{
		Provider:               row.Provider,
		LoginEnabled:           row.LoginEnabled,
		RegisterEnabled:        row.RegisterEnabled,
		ClientID:               row.ClientID,
		ClientSecretConfigured: row.ClientSecretEnc.Valid && row.ClientSecretEnc.String != "",
	}
}

func hasCredentials(row db.OauthProviderSetting) bool {
	return strings.TrimSpace(row.ClientID) != "" &&
		row.ClientSecretEnc.Valid &&
		row.ClientSecretEnc.String != ""
}

// Get returns masked settings for one provider.
func (s *Service) Get(ctx context.Context, provider string) (Settings, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return Settings{}, err
	}
	row, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return Settings{}, err
	}
	return mapSettings(row), nil
}

// IsLoginEnabled reports whether provider sign-in is active (credentials + flag).
func (s *Service) IsLoginEnabled(ctx context.Context, provider string) (bool, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return false, err
	}
	row, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return false, err
	}
	return row.LoginEnabled && hasCredentials(row), nil
}

// IsRegisterEnabled reports whether provider self-register is active (credentials + flag).
func (s *Service) IsRegisterEnabled(ctx context.Context, provider string) (bool, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return false, err
	}
	row, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return false, err
	}
	return row.RegisterEnabled && hasCredentials(row), nil
}

// GetAdapterOAuthConfig returns decrypted OAuth credentials when login or register is enabled.
func (s *Service) GetAdapterOAuthConfig(ctx context.Context, provider string) (AdapterOAuthConfig, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return AdapterOAuthConfig{}, err
	}
	row, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return AdapterOAuthConfig{}, err
	}
	if (!row.LoginEnabled && !row.RegisterEnabled) || !hasCredentials(row) {
		return AdapterOAuthConfig{Enabled: false}, nil
	}
	secret, err := s.box.Decrypt(row.ClientSecretEnc.String)
	if err != nil {
		return AdapterOAuthConfig{}, fmt.Errorf("decrypt client secret: %w", err)
	}
	return AdapterOAuthConfig{
		Enabled:         true,
		RegisterAllowed: row.RegisterEnabled,
		ClientID:        strings.TrimSpace(row.ClientID),
		ClientSecret:    secret,
	}, nil
}

// Patch updates provider settings.
func (s *Service) Patch(ctx context.Context, provider string, in PatchInput) (Settings, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return Settings{}, err
	}
	current, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return Settings{}, err
	}

	params := db.UpdateOAuthProviderSettingsParams{Provider: p}
	if in.LoginEnabled != nil {
		params.LoginEnabled = pgtype.Bool{Bool: *in.LoginEnabled, Valid: true}
	}
	if in.RegisterEnabled != nil {
		params.RegisterEnabled = pgtype.Bool{Bool: *in.RegisterEnabled, Valid: true}
	}
	if in.ClientID != nil {
		params.ClientID = pgtype.Text{String: strings.TrimSpace(*in.ClientID), Valid: true}
	}
	if in.ClientSecret != nil {
		trimmed := strings.TrimSpace(*in.ClientSecret)
		if trimmed != "" {
			enc, err := s.box.Encrypt(trimmed)
			if err != nil {
				return Settings{}, err
			}
			params.ClientSecretEnc = pgtype.Text{String: enc, Valid: true}
		}
	}

	nextLogin := current.LoginEnabled
	if in.LoginEnabled != nil {
		nextLogin = *in.LoginEnabled
	}
	nextRegister := current.RegisterEnabled
	if in.RegisterEnabled != nil {
		nextRegister = *in.RegisterEnabled
	}
	nextClientID := current.ClientID
	if in.ClientID != nil {
		nextClientID = strings.TrimSpace(*in.ClientID)
	}
	hasSecret := current.ClientSecretEnc.Valid && current.ClientSecretEnc.String != ""
	if in.ClientSecret != nil && strings.TrimSpace(*in.ClientSecret) != "" {
		hasSecret = true
	}
	if (nextLogin || nextRegister) && (nextClientID == "" || !hasSecret) {
		return Settings{}, fmt.Errorf("%w: client_id and client_secret are required to enable OAuth", ErrInvalidRequest)
	}

	row, err := s.q.UpdateOAuthProviderSettings(ctx, params)
	if err != nil {
		return Settings{}, err
	}
	return mapSettings(row), nil
}
