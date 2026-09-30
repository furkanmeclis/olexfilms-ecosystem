package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidRequest = errors.New("invalid request")
	ErrNotFound       = errors.New("not found")
)

type settingsStore interface {
	GetAuthSettings(ctx context.Context) (db.GetAuthSettingsRow, error)
	UpdateAuthSettings(ctx context.Context, arg db.UpdateAuthSettingsParams) (db.AuthSetting, error)
	GetRoleByUUID(ctx context.Context, id uuid.UUID) (db.Role, error)
}

// Service manages the auth_settings singleton (registration policy).
type Service struct {
	q settingsStore
}

// New creates an auth settings service.
func New(q settingsStore) *Service {
	return &Service{q: q}
}

// DefaultRoleSummary is the optional default role assigned on self-register.
type DefaultRoleSummary struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

// Settings is the admin payload for registration / method policy.
type Settings struct {
	RegistrationEnabled     bool                `json:"registration_enabled"`
	DefaultRole             *DefaultRoleSummary `json:"default_role"`
	PasswordLoginEnabled    bool                `json:"password_login_enabled"`
	PasswordRegisterEnabled bool                `json:"password_register_enabled"`
	PasskeyLoginEnabled     bool                `json:"passkey_login_enabled"`
}

// PatchInput partially updates auth settings.
type PatchInput struct {
	RegistrationEnabled     *bool      `json:"registration_enabled"`
	DefaultRoleUUID         *uuid.UUID `json:"default_role_uuid"`
	ClearDefaultRole        *bool      `json:"clear_default_role"`
	PasswordLoginEnabled    *bool      `json:"password_login_enabled"`
	PasswordRegisterEnabled *bool      `json:"password_register_enabled"`
	PasskeyLoginEnabled     *bool      `json:"passkey_login_enabled"`
}

// Policy is the runtime gate used by Register / Login / OAuth.
type Policy struct {
	RegistrationEnabled     bool
	DefaultRoleID           *int64
	PasswordLoginEnabled    bool
	PasswordRegisterEnabled bool
	PasskeyLoginEnabled     bool
}

func mapSettings(row db.GetAuthSettingsRow) Settings {
	out := Settings{
		RegistrationEnabled:     row.RegistrationEnabled,
		PasswordLoginEnabled:    row.PasswordLoginEnabled,
		PasswordRegisterEnabled: row.PasswordRegisterEnabled,
		PasskeyLoginEnabled:     row.PasskeyLoginEnabled,
	}
	if row.DefaultRoleUuid.Valid {
		id, err := uuid.FromBytes(row.DefaultRoleUuid.Bytes[:])
		if err == nil && row.DefaultRoleName.Valid && row.DefaultRoleSlug.Valid {
			out.DefaultRole = &DefaultRoleSummary{
				UUID: id,
				Name: row.DefaultRoleName.String,
				Slug: row.DefaultRoleSlug.String,
			}
		}
	}
	return out
}

func mapPolicy(row db.GetAuthSettingsRow) Policy {
	p := Policy{
		RegistrationEnabled:     row.RegistrationEnabled,
		PasswordLoginEnabled:    row.PasswordLoginEnabled,
		PasswordRegisterEnabled: row.PasswordRegisterEnabled,
		PasskeyLoginEnabled:     row.PasskeyLoginEnabled,
	}
	if row.DefaultRoleID.Valid {
		id := row.DefaultRoleID.Int64
		p.DefaultRoleID = &id
	}
	return p
}

// Get returns admin settings.
func (s *Service) Get(ctx context.Context) (Settings, error) {
	row, err := s.q.GetAuthSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	return mapSettings(row), nil
}

// Policy returns runtime registration / method gates.
func (s *Service) Policy(ctx context.Context) (Policy, error) {
	row, err := s.q.GetAuthSettings(ctx)
	if err != nil {
		return Policy{}, err
	}
	return mapPolicy(row), nil
}

// CanPasswordRegister reports whether password self-register is allowed.
func (s *Service) CanPasswordRegister(ctx context.Context) (bool, error) {
	p, err := s.Policy(ctx)
	if err != nil {
		return false, err
	}
	return p.RegistrationEnabled && p.PasswordRegisterEnabled, nil
}

// CanPasswordLogin reports whether password login is allowed.
func (s *Service) CanPasswordLogin(ctx context.Context) (bool, error) {
	p, err := s.Policy(ctx)
	if err != nil {
		return false, err
	}
	return p.PasswordLoginEnabled, nil
}

// CanPasskeyLogin reports whether passkey login is allowed.
func (s *Service) CanPasskeyLogin(ctx context.Context) (bool, error) {
	p, err := s.Policy(ctx)
	if err != nil {
		return false, err
	}
	return p.PasskeyLoginEnabled, nil
}

// RegistrationEnabled reports the global registration master switch.
func (s *Service) RegistrationEnabled(ctx context.Context) (bool, error) {
	p, err := s.Policy(ctx)
	if err != nil {
		return false, err
	}
	return p.RegistrationEnabled, nil
}

// DefaultRoleID returns the configured default role internal id, if any.
func (s *Service) DefaultRoleID(ctx context.Context) (*int64, error) {
	p, err := s.Policy(ctx)
	if err != nil {
		return nil, err
	}
	return p.DefaultRoleID, nil
}

// Patch updates auth settings.
func (s *Service) Patch(ctx context.Context, in PatchInput) (Settings, error) {
	current, err := s.q.GetAuthSettings(ctx)
	if err != nil {
		return Settings{}, err
	}

	params := db.UpdateAuthSettingsParams{
		RegistrationEnabled:     current.RegistrationEnabled,
		DefaultRoleID:           current.DefaultRoleID,
		PasswordLoginEnabled:    current.PasswordLoginEnabled,
		PasswordRegisterEnabled: current.PasswordRegisterEnabled,
		PasskeyLoginEnabled:     current.PasskeyLoginEnabled,
	}
	if in.RegistrationEnabled != nil {
		params.RegistrationEnabled = *in.RegistrationEnabled
	}
	if in.PasswordLoginEnabled != nil {
		params.PasswordLoginEnabled = *in.PasswordLoginEnabled
	}
	if in.PasswordRegisterEnabled != nil {
		params.PasswordRegisterEnabled = *in.PasswordRegisterEnabled
	}
	if in.PasskeyLoginEnabled != nil {
		params.PasskeyLoginEnabled = *in.PasskeyLoginEnabled
	}
	if in.ClearDefaultRole != nil && *in.ClearDefaultRole {
		params.DefaultRoleID = pgtype.Int8{}
	} else if in.DefaultRoleUUID != nil {
		role, err := s.q.GetRoleByUUID(ctx, *in.DefaultRoleUUID)
		if err != nil {
			return Settings{}, fmt.Errorf("%w: default role not found", ErrNotFound)
		}
		params.DefaultRoleID = pgtype.Int8{Int64: role.ID, Valid: true}
	}

	if _, err := s.q.UpdateAuthSettings(ctx, params); err != nil {
		return Settings{}, err
	}
	return s.Get(ctx)
}
