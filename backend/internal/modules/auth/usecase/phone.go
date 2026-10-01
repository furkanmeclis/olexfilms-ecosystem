package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// PhoneRepository is the persistence port for phone (WhatsApp OTP) login.
type PhoneRepository interface {
	FindUserByPhone(ctx context.Context, e164 string) (model.User, error)
	CreatePhoneUser(ctx context.Context, e164, passwordHash, locale, roleSlug string) (model.User, error)
	MarkPhoneVerified(ctx context.Context, userID int64) error
}

// SetPhoneRepository enables LoginWithVerifiedPhone.
func (u *AuthUseCase) SetPhoneRepository(repo PhoneRepository) {
	u.phoneRepo = repo
}

// LoginWithVerifiedPhone issues tokens for the owner of a phone number whose
// OTP was just verified (K11: customers sign in with WhatsApp OTP only).
// An unknown number becomes a new customer account; created reports that.
// The caller must have verified the OTP; this method trusts e164.
func (u *AuthUseCase) LoginWithVerifiedPhone(ctx context.Context, e164, locale string, meta model.SessionMeta) (model.Tokens, bool, error) {
	if u.phoneRepo == nil {
		return model.Tokens{}, false, fmt.Errorf("%w: phone login is not configured", ErrForbidden)
	}
	if e164 == "" {
		return model.Tokens{}, false, ErrInvalidRequest
	}
	created := false
	user, err := u.phoneRepo.FindUserByPhone(ctx, e164)
	if errors.Is(err, repository.ErrNotFound) {
		hash, herr := unusablePasswordHash()
		if herr != nil {
			return model.Tokens{}, false, herr
		}
		user, err = u.phoneRepo.CreatePhoneUser(ctx, e164, hash, supportedLocale(locale), rbac.RoleCustomer)
		if err != nil {
			// A concurrent verify may have created it first.
			if existing, ferr := u.phoneRepo.FindUserByPhone(ctx, e164); ferr == nil {
				user, err = existing, nil
			} else {
				return model.Tokens{}, false, err
			}
		} else {
			created = true
			u.indexUserSearch(ctx, user.UUID.String())
		}
	} else if err != nil {
		return model.Tokens{}, false, err
	}
	if user.Status != "active" {
		return model.Tokens{}, false, ErrUserDisabled
	}
	if err := u.phoneRepo.MarkPhoneVerified(ctx, user.ID); err != nil {
		return model.Tokens{}, false, err
	}
	if err := u.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		return model.Tokens{}, false, err
	}
	meta.Realm = jwt.AudiencePortal
	tokens, err := u.issueTokensForUser(ctx, user, meta, nil)
	if err != nil {
		return model.Tokens{}, false, err
	}
	return tokens, created, nil
}

// supportedLocale maps to the users.locale CHECK (tr, en).
func supportedLocale(locale string) string {
	if len(locale) >= 2 && (locale[:2] == "en" || locale[:2] == "EN") {
		return "en"
	}
	return "tr"
}

// unusablePasswordHash hashes 32 random bytes nobody knows: phone accounts
// cannot sign in with a password until one is set explicitly.
func unusablePasswordHash() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return password.Hash(base64.RawURLEncoding.EncodeToString(b))
}
