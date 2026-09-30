package stepup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/totp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrRateLimited        = errors.New("too many attempts")
	ErrMethodDisabled     = errors.New("verification method disabled")
	ErrNoPasskeys         = errors.New("no passkeys registered")
	ErrNoTOTP             = errors.New("authenticator is not enabled")
	ErrInvalidRequest     = errors.New("invalid request")
)

// UserRepo loads users and passkeys for step-up flows.
type UserRepo interface {
	FindUserByUUID(ctx context.Context, userUUID uuid.UUID) (model.User, error)
	ListPasskeysByUserID(ctx context.Context, userID int64) ([]model.PasskeyRecord, error)
	UpdatePasskeyCounter(ctx context.Context, credentialID string, counter int64) error
	GetUserTOTP(ctx context.Context, userID int64) (model.UserTOTP, error)
	UpdateUserTOTPRecoveryHashes(ctx context.Context, userID int64, recoveryHashes []string) error
}

// SecretBox decrypts TOTP secrets.
type SecretBox interface {
	Decrypt(encoded string) (string, error)
}

// Service coordinates step-up verification and policy.
type Service struct {
	q        *db.Queries
	store    *Store
	webauthn *WebAuthn
	repo     UserRepo
	box      SecretBox
	now      func() time.Time
}

// NewService creates a step-up service.
func NewService(q *db.Queries, store *Store, webauthn *WebAuthn, repo UserRepo) *Service {
	return &Service{
		q:        q,
		store:    store,
		webauthn: webauthn,
		repo:     repo,
		now:      time.Now,
	}
}

// SetSecretBox attaches encryption used to verify authenticator codes.
func (s *Service) SetSecretBox(box SecretBox) {
	s.box = box
}

// GetPolicy returns the current admin policy.
func (s *Service) GetPolicy(ctx context.Context) (Policy, error) {
	row, err := s.q.GetStepupSettings(ctx)
	if err != nil {
		return Policy{}, err
	}
	return mapPolicy(row), nil
}

// PasswordLoginTOTPRequired reports whether password login must use 2FA.
func (s *Service) PasswordLoginTOTPRequired(ctx context.Context) (bool, error) {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return false, err
	}
	return policy.PasswordLoginTOTPRequired, nil
}

// PatchPolicy updates admin policy with validation.
func (s *Service) PatchPolicy(ctx context.Context, in PatchPolicyInput) (Policy, error) {
	current, err := s.GetPolicy(ctx)
	if err != nil {
		return Policy{}, err
	}

	next := current
	if in.TTLHours != nil {
		if *in.TTLHours < 1 || *in.TTLHours > 168 {
			return Policy{}, fmt.Errorf("%w: ttl_hours must be between 1 and 168", ErrInvalidRequest)
		}
		next.TTLHours = *in.TTLHours
	}
	if in.PasswordEnabled != nil {
		next.PasswordEnabled = *in.PasswordEnabled
	}
	if in.PasskeyEnabled != nil {
		next.PasskeyEnabled = *in.PasskeyEnabled
	}
	if in.TOTPEnabled != nil {
		next.TOTPEnabled = *in.TOTPEnabled
	}
	if in.PasswordLoginTOTPRequired != nil {
		next.PasswordLoginTOTPRequired = *in.PasswordLoginTOTPRequired
	}
	if !next.PasswordEnabled && !next.PasskeyEnabled && !next.TOTPEnabled {
		return Policy{}, fmt.Errorf("%w: at least one verification method must stay enabled", ErrInvalidRequest)
	}

	row, err := s.q.UpdateStepupSettings(ctx, db.UpdateStepupSettingsParams{
		TtlHours:                  pgInt4(in.TTLHours),
		PasswordEnabled:           pgBool(in.PasswordEnabled),
		PasskeyEnabled:            pgBool(in.PasskeyEnabled),
		TotpEnabled:               pgBool(in.TOTPEnabled),
		PasswordLoginTotpRequired: pgBool(in.PasswordLoginTOTPRequired),
	})
	if err != nil {
		return Policy{}, err
	}
	return mapPolicyUpdate(row), nil
}

// Status returns the caller grant state and enabled methods.
func (s *Service) Status(ctx context.Context, userUUID uuid.UUID) (Status, error) {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return Status{}, err
	}
	methods := enabledMethods(policy)
	valid, expiresAt, err := s.store.GetGrant(ctx, userUUID)
	if err != nil {
		return Status{}, err
	}
	st := Status{Valid: valid, Methods: methods}
	if valid {
		t := expiresAt
		st.ExpiresAt = &t
	}
	return st, nil
}

// HasValidGrant reports whether the user has an active step-up grant.
func (s *Service) HasValidGrant(ctx context.Context, userUUID uuid.UUID) (bool, error) {
	valid, _, err := s.store.GetGrant(ctx, userUUID)
	return valid, err
}

// VerifyPassword checks the caller password and issues a grant.
func (s *Service) VerifyPassword(ctx context.Context, userUUID uuid.UUID, rawPassword string) (Grant, error) {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return Grant{}, err
	}
	if !policy.PasswordEnabled {
		return Grant{}, ErrMethodDisabled
	}
	limited, err := s.store.IsRateLimited(ctx, userUUID)
	if err != nil {
		return Grant{}, err
	}
	if limited {
		return Grant{}, ErrRateLimited
	}

	user, err := s.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return Grant{}, ErrInvalidCredentials
		}
		return Grant{}, err
	}
	ok, err := password.Verify(user.PasswordHash, rawPassword)
	if err != nil || !ok {
		_, _ = s.store.IncrRateLimit(ctx, userUUID)
		return Grant{}, ErrInvalidCredentials
	}
	_ = s.store.ClearRateLimit(ctx, userUUID)
	return s.issueGrant(ctx, userUUID, "password", policy)
}

// VerifyTOTP checks an authenticator or recovery code and issues a grant.
func (s *Service) VerifyTOTP(ctx context.Context, userUUID uuid.UUID, code string) (Grant, error) {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return Grant{}, err
	}
	if !policy.TOTPEnabled {
		return Grant{}, ErrMethodDisabled
	}
	if s.box == nil {
		return Grant{}, fmt.Errorf("%w: encryption is not configured", ErrInvalidRequest)
	}
	limited, err := s.store.IsRateLimited(ctx, userUUID)
	if err != nil {
		return Grant{}, err
	}
	if limited {
		return Grant{}, ErrRateLimited
	}

	user, err := s.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return Grant{}, ErrInvalidCredentials
		}
		return Grant{}, err
	}
	row, err := s.repo.GetUserTOTP(ctx, user.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return Grant{}, ErrNoTOTP
		}
		return Grant{}, err
	}
	if !row.Enabled {
		return Grant{}, ErrNoTOTP
	}
	secret, err := s.box.Decrypt(row.SecretEnc)
	if err != nil {
		return Grant{}, err
	}
	code = strings.TrimSpace(code)
	if totp.ValidateCode(secret, code) {
		_ = s.store.ClearRateLimit(ctx, userUUID)
		return s.issueGrant(ctx, userUUID, "totp", policy)
	}
	remaining, matched := totp.ConsumeRecoveryCode(row.RecoveryHashes, code)
	if !matched {
		_, _ = s.store.IncrRateLimit(ctx, userUUID)
		return Grant{}, ErrInvalidCredentials
	}
	if err := s.repo.UpdateUserTOTPRecoveryHashes(ctx, user.ID, remaining); err != nil {
		return Grant{}, err
	}
	_ = s.store.ClearRateLimit(ctx, userUUID)
	return s.issueGrant(ctx, userUUID, "totp", policy)
}

// RevokeGrant clears the caller step-up grant.
func (s *Service) RevokeGrant(ctx context.Context, userUUID uuid.UUID) error {
	return s.store.RevokeGrant(ctx, userUUID)
}

func (s *Service) issueGrant(ctx context.Context, userUUID uuid.UUID, method string, policy Policy) (Grant, error) {
	ttl := time.Duration(policy.TTLHours) * time.Hour
	expiresAt, err := s.store.SetGrant(ctx, userUUID, method, ttl)
	if err != nil {
		return Grant{}, err
	}
	return Grant{Valid: true, ExpiresAt: expiresAt, Method: method}, nil
}

func mapPolicy(row db.StepupSetting) Policy {
	return Policy{
		TTLHours:                  int(row.TtlHours),
		PasswordEnabled:           row.PasswordEnabled,
		PasskeyEnabled:            row.PasskeyEnabled,
		TOTPEnabled:               row.TotpEnabled,
		PasswordLoginTOTPRequired: row.PasswordLoginTotpRequired,
	}
}

func mapPolicyUpdate(row db.StepupSetting) Policy {
	return mapPolicy(row)
}

func pgInt4(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true}
}

func pgBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func enabledMethods(p Policy) []string {
	out := make([]string, 0, 3)
	if p.PasswordEnabled {
		out = append(out, "password")
	}
	if p.PasskeyEnabled {
		out = append(out, "passkey")
	}
	if p.TOTPEnabled {
		out = append(out, "totp")
	}
	return out
}
