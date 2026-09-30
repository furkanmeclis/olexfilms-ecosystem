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
	ErrInvalidRequest = errors.New("invalid request")
	ErrNotConfigured  = errors.New("github app is not configured")
)

type settingsStore interface {
	GetGitHubAppSettings(ctx context.Context) (db.GithubAppSetting, error)
	UpdateGitHubAppSettings(ctx context.Context, arg db.UpdateGitHubAppSettingsParams) (db.GithubAppSetting, error)
}

// Service manages github_app_settings singleton.
type Service struct {
	q   settingsStore
	box *crypto.SecretBox
}

// New creates a GitHub integration service.
func New(q settingsStore, box *crypto.SecretBox) *Service {
	return &Service{q: q, box: box}
}

// Settings is the public admin payload (no secret values).
type Settings struct {
	Enabled                bool   `json:"enabled"`
	RegisterEnabled        bool   `json:"register_enabled"`
	AppID                  string `json:"app_id"`
	ClientID               string `json:"client_id"`
	ClientSecretConfigured bool   `json:"client_secret_configured"`
	PrivateKeyConfigured   bool   `json:"private_key_configured"`
}

// AdapterOAuthConfig is returned to NextAuth via the internal adapter API.
type AdapterOAuthConfig struct {
	Enabled         bool   `json:"enabled"`
	RegisterAllowed bool   `json:"register_allowed"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"`
}

// PatchInput partial update for GitHub App settings.
type PatchInput struct {
	Enabled         *bool   `json:"enabled"`
	RegisterEnabled *bool   `json:"register_enabled"`
	AppID           *string `json:"app_id"`
	ClientID        *string `json:"client_id"`
	ClientSecret    *string `json:"client_secret"`
	PrivateKey      *string `json:"private_key"`
}

func mapSettings(row db.GithubAppSetting) Settings {
	return Settings{
		Enabled:                row.Enabled,
		RegisterEnabled:        row.RegisterEnabled,
		AppID:                  row.AppID,
		ClientID:               row.ClientID,
		ClientSecretConfigured: row.ClientSecretEnc.Valid && row.ClientSecretEnc.String != "",
		PrivateKeyConfigured:   row.PrivateKeyEnc.Valid && row.PrivateKeyEnc.String != "",
	}
}

func hasCredentials(row db.GithubAppSetting) bool {
	return strings.TrimSpace(row.ClientID) != "" &&
		row.ClientSecretEnc.Valid &&
		row.ClientSecretEnc.String != ""
}

// Get returns masked settings for super admins.
func (s *Service) Get(ctx context.Context) (Settings, error) {
	row, err := s.q.GetGitHubAppSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	return mapSettings(row), nil
}

// IsAuthEnabled reports whether GitHub sign-in/linking is active.
func (s *Service) IsAuthEnabled(ctx context.Context) (bool, error) {
	row, err := s.q.GetGitHubAppSettings(ctx)
	if err != nil {
		return false, err
	}
	return row.Enabled && hasCredentials(row), nil
}

// IsRegisterEnabled reports whether GitHub self-register is active.
func (s *Service) IsRegisterEnabled(ctx context.Context) (bool, error) {
	row, err := s.q.GetGitHubAppSettings(ctx)
	if err != nil {
		return false, err
	}
	return row.RegisterEnabled && hasCredentials(row), nil
}

// GetAdapterOAuthConfig returns decrypted OAuth credentials for NextAuth.
func (s *Service) GetAdapterOAuthConfig(ctx context.Context) (AdapterOAuthConfig, error) {
	row, err := s.q.GetGitHubAppSettings(ctx)
	if err != nil {
		return AdapterOAuthConfig{}, err
	}
	if (!row.Enabled && !row.RegisterEnabled) || !hasCredentials(row) {
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

// Patch updates GitHub App settings.
func (s *Service) Patch(ctx context.Context, in PatchInput) (Settings, error) {
	current, err := s.q.GetGitHubAppSettings(ctx)
	if err != nil {
		return Settings{}, err
	}

	params := db.UpdateGitHubAppSettingsParams{}
	if in.Enabled != nil {
		params.Enabled = pgtype.Bool{Bool: *in.Enabled, Valid: true}
	}
	if in.RegisterEnabled != nil {
		params.RegisterEnabled = pgtype.Bool{Bool: *in.RegisterEnabled, Valid: true}
	}
	if in.AppID != nil {
		params.AppID = pgtype.Text{String: strings.TrimSpace(*in.AppID), Valid: true}
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
	if in.PrivateKey != nil {
		trimmed := strings.TrimSpace(*in.PrivateKey)
		if trimmed != "" {
			enc, err := s.box.Encrypt(trimmed)
			if err != nil {
				return Settings{}, err
			}
			params.PrivateKeyEnc = pgtype.Text{String: enc, Valid: true}
		}
	}

	nextEnabled := current.Enabled
	if in.Enabled != nil {
		nextEnabled = *in.Enabled
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
	if (nextEnabled || nextRegister) && (nextClientID == "" || !hasSecret) {
		return Settings{}, fmt.Errorf("%w: client_id and client_secret are required to enable GitHub auth", ErrInvalidRequest)
	}

	row, err := s.q.UpdateGitHubAppSettings(ctx, params)
	if err != nil {
		return Settings{}, err
	}
	return mapSettings(row), nil
}
