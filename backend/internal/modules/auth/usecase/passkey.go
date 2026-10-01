package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/google/uuid"
)

// IssueSessionForUser issues API tokens for an existing user (passkey / adapter flows).
// When authMethod is "passkey", passkey_login_enabled must be true.
func (u *AuthUseCase) IssueSessionForUser(ctx context.Context, userUUID uuid.UUID, meta model.SessionMeta, authMethod string) (model.Tokens, error) {
	if strings.EqualFold(strings.TrimSpace(authMethod), "passkey") && u.authSettings != nil {
		ok, err := u.authSettings.CanPasskeyLogin(ctx)
		if err != nil {
			return model.Tokens{}, err
		}
		if !ok {
			return model.Tokens{}, fmt.Errorf("%w: passkey login is disabled", ErrForbidden)
		}
	}
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.Tokens{}, ErrInvalidCredentials
		}
		return model.Tokens{}, err
	}
	if user.Status == "disabled" {
		return model.Tokens{}, ErrUserDisabled
	}
	if user.Status != "active" {
		return model.Tokens{}, ErrUserDisabled
	}
	// Passkey and OAuth sign in to the panel only (TEC-90).
	meta.Realm = jwt.AudiencePanel
	if err := u.checkRealmAccess(ctx, user, meta.Realm); err != nil {
		return model.Tokens{}, err
	}
	if err := u.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		return model.Tokens{}, err
	}
	return u.issueTokensForUser(ctx, user, meta, nil)
}

// GetAdapterUserByEmail returns a NextAuth adapter user by email.
func (u *AuthUseCase) GetAdapterUserByEmail(ctx context.Context, email string) (model.AdapterUser, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return model.AdapterUser{}, fmt.Errorf("%w: email is required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.AdapterUser{}, ErrNotFound
		}
		return model.AdapterUser{}, err
	}
	return toAdapterUser(user), nil
}

// GetAdapterUserByUUID returns a NextAuth adapter user by public uuid.
func (u *AuthUseCase) GetAdapterUserByUUID(ctx context.Context, userUUID uuid.UUID) (model.AdapterUser, error) {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.AdapterUser{}, ErrNotFound
		}
		return model.AdapterUser{}, err
	}
	return toAdapterUser(user), nil
}

// GetAdapterAuthenticator loads a passkey by credential id.
func (u *AuthUseCase) GetAdapterAuthenticator(ctx context.Context, credentialID string) (model.AdapterAuthenticator, error) {
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return model.AdapterAuthenticator{}, fmt.Errorf("%w: credential id is required", ErrInvalidRequest)
	}
	row, err := u.repo.GetPasskeyByCredentialID(ctx, credentialID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.AdapterAuthenticator{}, ErrNotFound
		}
		return model.AdapterAuthenticator{}, err
	}
	user, err := u.repo.FindUserByID(ctx, row.UserID)
	if err != nil {
		return model.AdapterAuthenticator{}, err
	}
	return toAdapterAuthenticator(row, user.UUID), nil
}

// ListAdapterAuthenticatorsByUser lists passkeys for adapter flows.
func (u *AuthUseCase) ListAdapterAuthenticatorsByUser(ctx context.Context, userUUID uuid.UUID) ([]model.AdapterAuthenticator, error) {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := u.repo.ListPasskeysByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	out := make([]model.AdapterAuthenticator, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAdapterAuthenticator(row, user.UUID))
	}
	return out, nil
}

// CreateAdapterAuthenticator stores a new passkey for an existing user.
func (u *AuthUseCase) CreateAdapterAuthenticator(ctx context.Context, in model.CreateAdapterAuthenticatorInput) (model.AdapterAuthenticator, error) {
	userUUID, err := uuid.Parse(strings.TrimSpace(in.UserID))
	if err != nil {
		return model.AdapterAuthenticator{}, fmt.Errorf("%w: invalid user id", ErrInvalidRequest)
	}
	if strings.TrimSpace(in.CredentialID) == "" || strings.TrimSpace(in.CredentialPublicKey) == "" {
		return model.AdapterAuthenticator{}, fmt.Errorf("%w: credential fields are required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.AdapterAuthenticator{}, ErrNotFound
		}
		return model.AdapterAuthenticator{}, err
	}
	row, err := u.repo.CreatePasskey(ctx, user.ID, in)
	if err != nil {
		return model.AdapterAuthenticator{}, err
	}
	return toAdapterAuthenticator(row, user.UUID), nil
}

// UpdateAdapterAuthenticatorCounter bumps sign count after successful auth.
func (u *AuthUseCase) UpdateAdapterAuthenticatorCounter(ctx context.Context, credentialID string, counter int64) error {
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return fmt.Errorf("%w: credential id is required", ErrInvalidRequest)
	}
	if counter < 0 {
		return fmt.Errorf("%w: counter must be non-negative", ErrInvalidRequest)
	}
	return u.repo.UpdatePasskeyCounter(ctx, credentialID, counter)
}

// DeleteAdapterAuthenticator removes a passkey by credential id.
func (u *AuthUseCase) DeleteAdapterAuthenticator(ctx context.Context, credentialID string) error {
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return fmt.Errorf("%w: credential id is required", ErrInvalidRequest)
	}
	return u.repo.DeletePasskeyByCredentialID(ctx, credentialID)
}

// ListPasskeys returns passkeys for the authenticated user.
func (u *AuthUseCase) ListPasskeys(ctx context.Context, userID int64) (model.PasskeyList, error) {
	rows, err := u.repo.ListPasskeysByUserID(ctx, userID)
	if err != nil {
		return model.PasskeyList{}, err
	}
	items := make([]model.Passkey, 0, len(rows))
	for _, row := range rows {
		items = append(items, toPasskey(row))
	}
	return model.PasskeyList{Items: items, Total: int64(len(items))}, nil
}

// RenamePasskey updates the display name of a passkey owned by the user.
func (u *AuthUseCase) RenamePasskey(ctx context.Context, userUUID uuid.UUID, passkeyUUID uuid.UUID, name *string) (model.Passkey, error) {
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			name = nil
		} else {
			name = &trimmed
		}
	}
	row, err := u.repo.UpdatePasskeyName(ctx, userUUID, passkeyUUID, name)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.Passkey{}, ErrNotFound
		}
		return model.Passkey{}, err
	}
	return toPasskey(row), nil
}

// DeletePasskey removes a passkey owned by the user.
func (u *AuthUseCase) DeletePasskey(ctx context.Context, userUUID uuid.UUID, passkeyUUID uuid.UUID) error {
	err := u.repo.DeletePasskeyByUUID(ctx, userUUID, passkeyUUID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func toAdapterUser(user model.User) model.AdapterUser {
	var verified *time.Time
	if user.EmailVerified {
		now := time.Now().UTC()
		verified = &now
	}
	name := strings.TrimSpace(user.Name + " " + user.Surname)
	var namePtr *string
	if name != "" {
		namePtr = &name
	}
	return model.AdapterUser{
		ID:            user.UUID.String(),
		Email:         user.Email,
		EmailVerified: verified,
		Name:          namePtr,
	}
}

func toAdapterAuthenticator(row model.PasskeyRecord, userUUID uuid.UUID) model.AdapterAuthenticator {
	return model.AdapterAuthenticator{
		CredentialID:         row.CredentialID,
		ProviderAccountID:    row.ProviderAccountID,
		UserID:               userUUID.String(),
		CredentialPublicKey:  row.PublicKey,
		Counter:              row.Counter,
		CredentialDeviceType: row.DeviceType,
		CredentialBackedUp:   row.BackedUp,
		Transports:           row.Transports,
	}
}

func toPasskey(row model.PasskeyRecord) model.Passkey {
	return model.Passkey{
		UUID:       row.UUID,
		Name:       row.Name,
		DeviceType: row.DeviceType,
		BackedUp:   row.BackedUp,
		Transports: row.Transports,
		LastUsedAt: row.LastUsedAt,
		CreatedAt:  row.CreatedAt,
	}
}
