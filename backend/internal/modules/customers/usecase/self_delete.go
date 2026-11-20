package usecase

// TEC-1257: the customer deletes their own account from the portal or the
// mobile app (App Store 5.1.1(v), KVKK). Deletion is the KVKK anonymization
// (AnonymizeCustomer, unchanged): personal data is overwritten, services,
// warranties and vehicles stay attached to the anonymous account, sessions
// are revoked. The handler confirms it with a phone code first.

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrNoPhone: the account has no phone to send the confirmation code to.
var ErrNoPhone = errors.New("customers: account has no phone")

// ActionSelfDeleted is written next to customers.anonymized when the
// customer deleted the account themself (resource "customers").
const ActionSelfDeleted = "customers.self_deleted"

// selfDeleteCaller runs the anonymization as the customer: the actor is the
// customer, the scope check accepts only their own account and the center
// rule of the panel action is met by construction (the person is the data
// subject, K19).
func selfDeleteCaller(userID int64) Caller {
	return Caller{UserID: userID, Org: orgctx.Scope{OrgType: rbac.OrgTypeCenter}, selfUserID: userID}
}

// SelfDeletePhone returns the phone (E.164) the confirmation code goes to.
// An anonymized or unknown account answers ErrCustomerNotFound, a panel
// account ErrPanelAccount, an account without a phone ErrNoPhone.
func (s *Service) SelfDeletePhone(ctx context.Context, userID int64) (string, error) {
	user, err := s.q.GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCustomerNotFound
	}
	if err != nil {
		return "", fmt.Errorf("customers: user: %w", err)
	}
	if user.Status == StatusAnonymized {
		return "", ErrCustomerNotFound
	}
	if err := s.requireCustomerOnly(ctx, s.q, user.ID); err != nil {
		return "", err
	}
	if !user.PhoneE164.Valid || user.PhoneE164.String == "" {
		return "", ErrNoPhone
	}
	return user.PhoneE164.String, nil
}

// DeleteOwnAccount anonymizes the signed-in customer's account. The caller
// must have verified the account_delete phone code.
func (s *Service) DeleteOwnAccount(ctx context.Context, userID int64, userUUID uuid.UUID, meta activity.Meta) (AnonymizeResult, error) {
	c := selfDeleteCaller(userID)
	res, err := s.AnonymizeCustomer(ctx, c, userUUID, meta)
	if err != nil {
		return AnonymizeResult{}, err
	}
	if res.Changed {
		actor := userID
		// No personal data in the payload (the audit log outlives the person).
		if err := activity.Write(ctx, s.q, &actor, ActionSelfDeleted, "customers", &userUUID,
			map[string]any{"channel": "portal"}, meta); err != nil {
			return AnonymizeResult{}, fmt.Errorf("customers: audit: %w", err)
		}
	}
	return res, nil
}
