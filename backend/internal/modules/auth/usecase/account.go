package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authrevoke"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/google/uuid"
)

const (
	otpTypePasswordReset     = "password_reset"
	otpTypeEmailVerification = "email_verification"
	otpTTL                   = 15 * time.Minute
)

var (
	ErrInvalidResetCode  = errors.New("invalid reset code")
	ErrInvalidVerifyCode = errors.New("invalid verification code")
	ErrPasswordResetFail = errors.New("password reset failed")
)

// Notifier enqueues outbound notifications (auth never calls SMTP directly).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// SetNotifier attaches the notification pipeline (optional).
func (u *AuthUseCase) SetNotifier(n Notifier) {
	u.notifier = n
}

// SetLogger attaches a logger for OTP debug output.
func (u *AuthUseCase) SetLogger(log *slog.Logger) {
	u.log = log
}

// SetRevocations enables immediate access-token revocation (optional).
func (u *AuthUseCase) SetRevocations(s *authrevoke.Store) {
	u.revocations = s
}

// RevokeAccessSession rejects access tokens of one session before they expire.
func (u *AuthUseCase) RevokeAccessSession(ctx context.Context, sid uuid.UUID) {
	if err := u.revocations.RevokeSession(ctx, sid); err != nil && u.log != nil {
		u.log.Warn("auth_revoke_session_failed", "error", err)
	}
}

// AccessRevoked reports whether an access token was revoked before expiry.
// Redis errors fail open: the token signature and expiry still hold, and an
// outage should not sign everyone out.
func (u *AuthUseCase) AccessRevoked(ctx context.Context, user, sid uuid.UUID, issuedAt time.Time) bool {
	// Bound the wait so a hung Redis costs a fraction of a second per
	// request, not the whole request budget.
	ctx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	revoked, err := u.revocations.IsRevoked(ctx, user, sid, issuedAt)
	if err != nil {
		if u.log != nil {
			u.log.Warn("auth_revocation_check_failed", "error", err)
		}
		return false
	}
	return revoked
}

// revokeAllSessions ends every refresh session and every live access token of the user.
func (u *AuthUseCase) revokeAllSessions(ctx context.Context, user model.User) error {
	if err := u.revocations.RevokeUser(ctx, user.UUID); err != nil && u.log != nil {
		u.log.Warn("auth_revoke_user_failed", "error", err)
	}
	return u.repo.RevokeAllRefresh(ctx, user.ID)
}

// SetSearchIndexer attaches the command-palette search indexer (optional).
func (u *AuthUseCase) SetSearchIndexer(indexer SearchIndexer) {
	u.searchIndexer = indexer
}

// UpdateProfile patches the caller's name/surname/locale and returns refreshed Me.
func (u *AuthUseCase) UpdateProfile(
	ctx context.Context,
	userUUID uuid.UUID,
	impersonatorUUID *uuid.UUID,
	name, surname, locale *string,
) (model.Me, error) {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		return model.Me{}, ErrNotFound
	}
	if name != nil {
		v := strings.TrimSpace(*name)
		if v == "" {
			return model.Me{}, fmt.Errorf("%w: name cannot be empty", ErrInvalidRequest)
		}
		name = &v
	}
	if surname != nil {
		v := strings.TrimSpace(*surname)
		if v == "" {
			return model.Me{}, fmt.Errorf("%w: surname cannot be empty", ErrInvalidRequest)
		}
		surname = &v
	}
	if locale != nil {
		v := strings.TrimSpace(*locale)
		if v != "tr" && v != "en" {
			return model.Me{}, fmt.Errorf("%w: locale must be tr or en", ErrInvalidRequest)
		}
		locale = &v
	}
	if name == nil && surname == nil && locale == nil {
		return model.Me{}, fmt.Errorf("%w: name, surname, or locale is required", ErrInvalidRequest)
	}
	if _, err := u.repo.UpdateProfile(ctx, userUUID, name, surname, locale); err != nil {
		return model.Me{}, err
	}
	if u.notifier != nil && (name != nil || surname != nil) {
		uid := user.ID
		lang := user.Locale
		if locale != nil {
			lang = *locale
		}
		_, _ = u.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			UserID: &uid, Channels: []string{notifmodel.ChannelInapp},
			TemplateCode: "auth.profile_updated", SourceEvent: "auth.profile_updated",
			Language:     lang,
			TemplateVars: map[string]string{"name": user.Name},
		})
	}
	return u.Me(ctx, userUUID, impersonatorUUID)
}

// ForgotPassword always returns accepted; enqueues reset email when user exists.
func (u *AuthUseCase) ForgotPassword(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return fmt.Errorf("%w: email is required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return ErrPasswordResetFail
	}
	code, err := generateOTP()
	if err != nil {
		return ErrPasswordResetFail
	}
	_ = u.repo.InvalidateOTPs(ctx, email, otpTypePasswordReset)
	uid := user.ID
	if err := u.repo.CreateOTP(ctx, &uid, email, hashOTP(code), otpTypePasswordReset, u.now().UTC().Add(otpTTL)); err != nil {
		return ErrPasswordResetFail
	}
	if u.log != nil {
		u.log.Debug("otp_issued", "type", otpTypePasswordReset, "email", email)
	}
	if u.notifier != nil {
		_, err = u.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			UserID: &uid, Channels: []string{notifmodel.ChannelEmail},
			TemplateCode: "auth.password_reset", SourceEvent: "auth.password_reset",
			SecurityEmail: true,
			Recipient:     &email,
			TemplateVars: map[string]string{
				"name": user.Name, "code": code, "expires_minutes": strconv.Itoa(int(otpTTL.Minutes())),
			},
		})
		if err != nil {
			return ErrPasswordResetFail
		}
	}
	return nil
}

// ResetPassword validates OTP and sets a new password; revokes all refresh tokens.
func (u *AuthUseCase) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	code = strings.TrimSpace(code)
	if email == "" || code == "" || newPassword == "" {
		return fmt.Errorf("%w: email, code, and password are required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return ErrInvalidResetCode
	}
	if err := u.verifyOTP(ctx, email, otpTypePasswordReset, code); err != nil {
		return err
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	if err := u.repo.UpdatePassword(ctx, user.ID, hash); err != nil {
		return ErrPasswordResetFail
	}
	_ = u.revokeAllSessions(ctx, user)
	return nil
}

// ChangePassword verifies current password and sets a new one; revokes all other sessions.
func (u *AuthUseCase) ChangePassword(ctx context.Context, userUUID uuid.UUID, currentPassword, newPassword string) error {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		return ErrNotFound
	}
	ok, err := password.Verify(user.PasswordHash, currentPassword)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	if err := u.repo.UpdatePassword(ctx, user.ID, hash); err != nil {
		return err
	}
	// Revoke all sessions (including the current one).
	return u.revokeAllSessions(ctx, user)
}

// RequestEmailVerification issues a verification OTP email.
func (u *AuthUseCase) RequestEmailVerification(ctx context.Context, userUUID uuid.UUID, emailOverride string) error {
	// Always act on the caller's own account: an arbitrary email override let
	// any signed-in user send verification codes to (and reset the OTP
	// attempt budget of) other accounts.
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	if o := strings.ToLower(strings.TrimSpace(emailOverride)); o != "" && o != strings.ToLower(user.Email) {
		return nil
	}
	if user.EmailVerified {
		return nil
	}
	return u.issueEmailVerification(ctx, user)
}

// VerifyEmail consumes a verification OTP.
func (u *AuthUseCase) VerifyEmail(ctx context.Context, email, code string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	code = strings.TrimSpace(code)
	if email == "" || code == "" {
		return fmt.Errorf("%w: email and code are required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return ErrInvalidVerifyCode
	}
	if err := u.verifyOTP(ctx, email, otpTypeEmailVerification, code); err != nil {
		if errors.Is(err, ErrInvalidResetCode) {
			return ErrInvalidVerifyCode
		}
		return err
	}
	_, err = u.repo.SetEmailVerified(ctx, user.ID)
	return err
}

func (u *AuthUseCase) issueEmailVerification(ctx context.Context, user model.User) error {
	code, err := generateOTP()
	if err != nil {
		return err
	}
	_ = u.repo.InvalidateOTPs(ctx, user.Email, otpTypeEmailVerification)
	uid := user.ID
	if err := u.repo.CreateOTP(ctx, &uid, user.Email, hashOTP(code), otpTypeEmailVerification, u.now().UTC().Add(otpTTL)); err != nil {
		return err
	}
	if u.log != nil {
		u.log.Debug("otp_issued", "type", otpTypeEmailVerification, "email", user.Email)
	}
	if u.notifier == nil {
		return nil
	}
	email := user.Email
	_, err = u.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
		UserID: &uid, Channels: []string{notifmodel.ChannelEmail},
		TemplateCode: "auth.email_verification", SourceEvent: "auth.email_verification",
		SecurityEmail: true, Recipient: &email,
		TemplateVars: map[string]string{
			"name": user.Name, "code": code, "expires_minutes": strconv.Itoa(int(otpTTL.Minutes())),
		},
	})
	return err
}

func (u *AuthUseCase) notifyWelcome(ctx context.Context, user model.User) {
	if u.notifier == nil {
		return
	}
	uid := user.ID
	_, _ = u.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
		UserID:       &uid,
		Channels:     []string{notifmodel.ChannelInapp, notifmodel.ChannelEmail},
		TemplateCode: "auth.welcome", SourceEvent: "auth.register",
		TemplateVars: map[string]string{"name": user.Name},
	})
	_ = u.issueEmailVerification(ctx, user)
}

func (u *AuthUseCase) verifyOTP(ctx context.Context, email, otpType, code string) error {
	id, codeHash, attempts, maxAttempts, err := u.repo.GetActiveOTP(ctx, email, otpType)
	if err != nil {
		return ErrInvalidResetCode
	}
	if attempts >= maxAttempts {
		_ = u.repo.ConsumeOTP(ctx, id)
		return ErrInvalidResetCode
	}
	if hashOTP(code) != codeHash {
		_, _, _ = u.repo.IncrementOTPAttempts(ctx, id)
		return ErrInvalidResetCode
	}
	if err := u.repo.ConsumeOTP(ctx, id); err != nil {
		return ErrInvalidResetCode
	}
	return nil
}

func generateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func hashOTP(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
