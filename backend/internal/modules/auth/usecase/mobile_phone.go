package usecase

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/google/uuid"
)

// MobileOTP is the existing phone OTP service with a pre-consumption MFA check.
type MobileOTP interface {
	Request(context.Context, otp.RequestInput) (otp.RequestResult, error)
	VerifyMobile(context.Context, otp.VerifyInput, func(otp.Verified) error) (otp.Verified, error)
}

type MobileIdentifyResult struct {
	Kind      string     `json:"kind"`
	Next      string     `json:"next"`
	PhoneE164 string     `json:"phone_e164,omitempty"`
	ResendAt  *time.Time `json:"resend_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// NormalizeMobileIdentifier never looks up an email or phone account.
func NormalizeMobileIdentifier(identifier, region string) (kind, normalized string, err error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", "", ErrInvalidRequest
	}
	if strings.Contains(identifier, "@") {
		addr, err := mail.ParseAddress(identifier)
		if err != nil || addr.Address != identifier {
			return "", "", ErrInvalidRequest
		}
		return "email", strings.ToLower(identifier), nil
	}
	num, err := phone.Parse(identifier, region)
	if err != nil {
		return "", "", otp.ErrInvalidPhone
	}
	return "phone", num.E164, nil
}

func (u *AuthUseCase) MobileIdentify(ctx context.Context, svc MobileOTP, identifier, region, locale string, meta model.SessionMeta) (MobileIdentifyResult, error) {
	kind, normalized, err := NormalizeMobileIdentifier(identifier, region)
	if err != nil {
		return MobileIdentifyResult{}, err
	}
	if kind == "email" {
		return MobileIdentifyResult{Kind: kind, Next: "password"}, nil
	}
	if svc == nil {
		return MobileIdentifyResult{}, otp.ErrDeliveryFailed
	}
	res, err := svc.Request(ctx, otp.RequestInput{Phone: normalized, Purpose: otp.PurposeMobileLogin, Locale: locale, IP: meta.IP, UserAgent: meta.UserAgent})
	if err != nil {
		return MobileIdentifyResult{}, err
	}
	return MobileIdentifyResult{Kind: kind, Next: "otp", PhoneE164: normalized, ResendAt: &res.ResendAt, ExpiresAt: &res.ExpiresAt}, nil
}

// MobileVerifyOTP never creates accounts or grants roles. The verified OTP is
// retained on MFA_REQUIRED; successful MFA consumes it before tokens are issued.
func (u *AuthUseCase) MobileVerifyOTP(ctx context.Context, svc MobileOTP, phoneNumber, code, totpCode, organizationSlug string, device model.DeviceInfo, meta model.SessionMeta) (model.Tokens, error) {
	dev, err := NormalizeDevice(device)
	if err != nil {
		return model.Tokens{}, err
	}
	if svc == nil || u.phoneRepo == nil {
		return model.Tokens{}, otp.ErrDeliveryFailed
	}
	var user model.User
	_, err = svc.VerifyMobile(ctx, otp.VerifyInput{Phone: phoneNumber, Code: code, IP: meta.IP}, func(v otp.Verified) error {
		var err error
		user, err = u.phoneRepo.FindUserByPhone(ctx, v.Phone)
		if errors.Is(err, repository.ErrNotFound) {
			return otp.ErrInvalidCode
		}
		if err != nil {
			return err
		}
		if user.Status != "active" {
			return otp.ErrInvalidCode
		}
		if err := u.checkRealmAccess(ctx, user, jwt.AudienceMobile); err != nil {
			return err
		}
		return u.checkLoginMFA(ctx, user.ID, totpCode)
	})
	if err != nil {
		return model.Tokens{}, err
	}
	var orgUUID *uuid.UUID
	if strings.TrimSpace(organizationSlug) != "" {
		if u.orgResolver == nil {
			return model.Tokens{}, ErrNoTenantMembership
		}
		resolved, err := u.orgResolver.ResolveLoginOrganization(ctx, user.ID, organizationSlug)
		if err != nil {
			return model.Tokens{}, mapOrganizationError(err)
		}
		orgUUID = &resolved
	}
	if err := u.phoneRepo.MarkPhoneVerified(ctx, user.ID); err != nil {
		return model.Tokens{}, err
	}
	if err := u.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		return model.Tokens{}, err
	}
	meta.Realm, meta.Client, meta.Device = jwt.AudienceMobile, model.ClientMobile, &dev
	meta.FamilyID, meta.ImpersonatorUserID = uuid.Nil, nil
	tokens, err := u.issueTokensForUser(ctx, user, meta, orgUUID)
	if err != nil {
		return model.Tokens{}, err
	}
	claims, err := u.tokens.ParseAccess(tokens.AccessToken)
	if err != nil {
		return model.Tokens{}, err
	}
	u.endDeviceSessions(ctx, user.ID, dev.ID, claims.SessionUUID())
	return tokens, nil
}
