package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/totp"
	"github.com/google/uuid"
)

var (
	ErrMFARequired     = errors.New("mfa required")
	ErrMFANotEnrolled  = errors.New("mfa not enrolled")
	ErrInvalidMFACode  = errors.New("invalid mfa code")
	ErrTOTPNotEnabled  = errors.New("totp is not enabled")
	ErrTOTPAlreadyOn   = errors.New("totp is already enabled")
	ErrTOTPSetupNeeded = errors.New("totp setup required")
)

// TOTPStatus returns whether the caller has confirmed authenticator 2FA.
func (u *AuthUseCase) TOTPStatus(ctx context.Context, userUUID uuid.UUID) (model.TOTPStatus, error) {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		return model.TOTPStatus{}, ErrNotFound
	}
	row, err := u.repo.GetUserTOTP(ctx, user.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.TOTPStatus{Enabled: false}, nil
		}
		return model.TOTPStatus{}, err
	}
	return model.TOTPStatus{Enabled: row.Enabled, ConfirmedAt: row.ConfirmedAt}, nil
}

// BeginTOTPSetup creates (or replaces) an unconfirmed authenticator secret.
func (u *AuthUseCase) BeginTOTPSetup(ctx context.Context, userUUID uuid.UUID) (model.TOTPSetupResult, error) {
	if u.box == nil {
		return model.TOTPSetupResult{}, fmt.Errorf("%w: encryption is not configured", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		return model.TOTPSetupResult{}, ErrNotFound
	}
	existing, err := u.repo.GetUserTOTP(ctx, user.ID)
	if err == nil && existing.Enabled {
		return model.TOTPSetupResult{}, ErrTOTPAlreadyOn
	}
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return model.TOTPSetupResult{}, err
	}

	key, err := totp.GenerateSecret(u.totpIssuer, user.Email)
	if err != nil {
		return model.TOTPSetupResult{}, err
	}
	enc, err := u.box.Encrypt(key.Secret())
	if err != nil {
		return model.TOTPSetupResult{}, err
	}
	if _, err := u.repo.UpsertUserTOTPSetup(ctx, user.ID, enc); err != nil {
		return model.TOTPSetupResult{}, err
	}
	return model.TOTPSetupResult{
		Secret:     key.Secret(),
		OTPAuthURL: key.URL(),
	}, nil
}

// ConfirmTOTPSetup verifies the first code and enables 2FA with recovery codes.
func (u *AuthUseCase) ConfirmTOTPSetup(ctx context.Context, userUUID uuid.UUID, code string) (model.TOTPConfirmResult, error) {
	if u.box == nil {
		return model.TOTPConfirmResult{}, fmt.Errorf("%w: encryption is not configured", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		return model.TOTPConfirmResult{}, ErrNotFound
	}
	row, err := u.repo.GetUserTOTP(ctx, user.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.TOTPConfirmResult{}, ErrTOTPSetupNeeded
		}
		return model.TOTPConfirmResult{}, err
	}
	if row.Enabled {
		return model.TOTPConfirmResult{}, ErrTOTPAlreadyOn
	}
	secret, err := u.box.Decrypt(row.SecretEnc)
	if err != nil {
		return model.TOTPConfirmResult{}, err
	}
	if !totp.ValidateCode(secret, code) {
		return model.TOTPConfirmResult{}, ErrInvalidMFACode
	}
	plain, hashes, err := totp.GenerateRecoveryCodes()
	if err != nil {
		return model.TOTPConfirmResult{}, err
	}
	if _, err := u.repo.ConfirmUserTOTP(ctx, user.ID, hashes); err != nil {
		return model.TOTPConfirmResult{}, err
	}
	return model.TOTPConfirmResult{Enabled: true, RecoveryCodes: plain}, nil
}

// DisableTOTP turns off authenticator 2FA after verifying a code or recovery code.
func (u *AuthUseCase) DisableTOTP(ctx context.Context, userUUID uuid.UUID, code string) error {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		return ErrNotFound
	}
	ok, remaining, err := u.verifyUserTOTPCode(ctx, user.ID, code, true)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidMFACode
	}
	if remaining != nil {
		_ = u.repo.UpdateUserTOTPRecoveryHashes(ctx, user.ID, remaining)
	}
	return u.repo.DeleteUserTOTP(ctx, user.ID)
}

// verifyUserTOTPCode validates a TOTP or recovery code for an enabled binding.
// When consumeRecovery is true, a matching recovery code is removed.
func (u *AuthUseCase) verifyUserTOTPCode(
	ctx context.Context,
	userID int64,
	code string,
	consumeRecovery bool,
) (ok bool, remainingHashes []string, err error) {
	if u.box == nil {
		return false, nil, fmt.Errorf("%w: encryption is not configured", ErrInvalidRequest)
	}
	row, err := u.repo.GetUserTOTP(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil, ErrTOTPNotEnabled
		}
		return false, nil, err
	}
	if !row.Enabled {
		return false, nil, ErrTOTPNotEnabled
	}
	secret, err := u.box.Decrypt(row.SecretEnc)
	if err != nil {
		return false, nil, err
	}
	code = strings.TrimSpace(code)
	if totp.ValidateCode(secret, code) {
		return true, nil, nil
	}
	remaining, matched := totp.ConsumeRecoveryCode(row.RecoveryHashes, code)
	if !matched {
		return false, nil, nil
	}
	if consumeRecovery {
		if err := u.repo.UpdateUserTOTPRecoveryHashes(ctx, userID, remaining); err != nil {
			return false, nil, err
		}
	}
	return true, remaining, nil
}

// UserHasEnabledTOTP reports whether the user completed authenticator enrollment.
func (u *AuthUseCase) UserHasEnabledTOTP(ctx context.Context, userID int64) (bool, error) {
	row, err := u.repo.GetUserTOTP(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return row.Enabled, nil
}
