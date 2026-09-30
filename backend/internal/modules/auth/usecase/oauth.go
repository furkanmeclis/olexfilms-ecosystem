package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/google/uuid"
)

// OAuthUseCase coordinates linked OAuth provider flows.
type OAuthUseCase struct {
	repo Repository
	box  *crypto.SecretBox
}

// NewOAuth creates an OAuth use case.
func NewOAuth(repo Repository, box *crypto.SecretBox) *OAuthUseCase {
	return &OAuthUseCase{repo: repo, box: box}
}

// GetAdapterUserByOAuthAccount resolves a user by linked provider account.
func (u *OAuthUseCase) GetAdapterUserByOAuthAccount(ctx context.Context, provider, providerAccountID string) (model.AdapterUser, error) {
	provider = strings.TrimSpace(provider)
	providerAccountID = strings.TrimSpace(providerAccountID)
	if provider == "" || providerAccountID == "" {
		return model.AdapterUser{}, fmt.Errorf("%w: provider and provider account id are required", ErrInvalidRequest)
	}
	row, err := u.repo.GetOAuthAccountByProviderAccount(ctx, provider, providerAccountID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.AdapterUser{}, ErrNotFound
		}
		return model.AdapterUser{}, err
	}
	user, err := u.repo.FindUserByUUID(ctx, row.UserUUID)
	if err != nil {
		return model.AdapterUser{}, err
	}
	return toAdapterUser(user), nil
}

// LinkOAuthAccount persists a linked provider for an existing user.
func (u *OAuthUseCase) LinkOAuthAccount(ctx context.Context, in model.LinkOAuthAccountInput) error {
	userUUID, err := uuid.Parse(strings.TrimSpace(in.UserID))
	if err != nil {
		return fmt.Errorf("%w: invalid user id", ErrInvalidRequest)
	}
	provider := strings.TrimSpace(in.Provider)
	providerAccountID := strings.TrimSpace(in.ProviderAccountID)
	if provider == "" || providerAccountID == "" {
		return fmt.Errorf("%w: provider and provider account id are required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if user.Status != "active" {
		return ErrUserDisabled
	}

	existing, err := u.repo.GetOAuthAccountByProviderAccount(ctx, provider, providerAccountID)
	if err == nil {
		if existing.UserUUID != userUUID {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return err
	}

	owned, err := u.repo.GetOAuthAccountByUserProvider(ctx, user.ID, provider)
	if err == nil && owned.ProviderAccountID != providerAccountID {
		return ErrConflict
	}
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}

	record := model.CreateOAuthAccountInput{
		UserID:            user.ID,
		Provider:          provider,
		ProviderAccountID: providerAccountID,
		Type:              strings.TrimSpace(in.Type),
		GitHubLogin:       in.GitHubLogin,
	}
	if record.Type == "" {
		record.Type = "oauth"
	}
	if in.AccessToken != nil && strings.TrimSpace(*in.AccessToken) != "" {
		enc, err := u.box.Encrypt(strings.TrimSpace(*in.AccessToken))
		if err != nil {
			return err
		}
		record.AccessTokenEnc = &enc
	}
	if in.RefreshToken != nil && strings.TrimSpace(*in.RefreshToken) != "" {
		enc, err := u.box.Encrypt(strings.TrimSpace(*in.RefreshToken))
		if err != nil {
			return err
		}
		record.RefreshTokenEnc = &enc
	}
	record.ExpiresAt = in.ExpiresAt
	record.TokenType = in.TokenType
	record.Scope = in.Scope

	_, err = u.repo.CreateOAuthAccount(ctx, record)
	if err != nil {
		return err
	}
	return nil
}

// UnlinkOAuthAccountByProviderAccount removes a linked account by provider account id.
func (u *OAuthUseCase) UnlinkOAuthAccountByProviderAccount(ctx context.Context, provider, providerAccountID string) error {
	provider = strings.TrimSpace(provider)
	providerAccountID = strings.TrimSpace(providerAccountID)
	if provider == "" || providerAccountID == "" {
		return fmt.Errorf("%w: provider and provider account id are required", ErrInvalidRequest)
	}
	return u.repo.DeleteOAuthAccountByProviderAccount(ctx, provider, providerAccountID)
}

// UnlinkOAuthAccountForUser removes the current user's linked provider.
func (u *OAuthUseCase) UnlinkOAuthAccountForUser(ctx context.Context, userUUID uuid.UUID, provider string) error {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if err := u.repo.DeleteOAuthAccountByUserProvider(ctx, user.ID, provider); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// ListIdentities returns linked providers for a user.
func (u *OAuthUseCase) ListIdentities(ctx context.Context, userUUID uuid.UUID) (model.IdentityList, error) {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.IdentityList{}, ErrNotFound
		}
		return model.IdentityList{}, err
	}
	rows, err := u.repo.ListOAuthAccountsByUserID(ctx, user.ID)
	if err != nil {
		return model.IdentityList{}, err
	}
	items := make([]model.LinkedIdentity, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.LinkedIdentity{
			Provider:    row.Provider,
			GitHubLogin: row.GitHubLogin,
			LinkedAt:    row.CreatedAt,
		})
	}
	return model.IdentityList{Items: items, Total: int64(len(items))}, nil
}

// OAuthAccountExists reports whether a provider account is already linked.
func (u *OAuthUseCase) OAuthAccountExists(ctx context.Context, provider, providerAccountID string) (bool, error) {
	_, err := u.repo.GetOAuthAccountByProviderAccount(ctx, strings.TrimSpace(provider), strings.TrimSpace(providerAccountID))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
