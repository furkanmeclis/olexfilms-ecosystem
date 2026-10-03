package usecase

// TEC-161 (F1-08c): KVKK/GDPR anonymization and personal data export (K19,
// TEC-100 decision 2).
//
// Anonymization never deletes a row (20 foreign keys cascade from users):
// the users row keeps its id and uuid, so vehicles, plates/VINs, services and
// warranties stay attached and queryable. Name, surname, e-mail, phone,
// timezone and every profile field (address, company, tax office, encrypted
// identity numbers with their masks) are overwritten irreversibly; the
// account gets status anonymized and a password nobody knows, refresh tokens
// are revoked and live access tokens are cut by the identity loader (status
// check) and the revocation store. A second call changes nothing.
//
// Every anonymization and export request is written to activity_events; the
// anonymization audit row commits in the same transaction as the change.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine/adapters"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrPanelAccount: the customer also has a panel account (organization
// membership or a staff role); anonymizing it would lock out an organization
// user, so it is refused (the account must be detached first).
var ErrPanelAccount = errors.New("customers: customer has a panel account")

// Audit actions (activity_events.action, resource "customers").
const (
	ActionAnonymized          = "customers.anonymized"
	ActionDataExportRequested = "customers.data_export_requested"
)

// Revoker cuts live access tokens of a user (*authrevoke.Store).
type Revoker interface {
	RevokeUser(ctx context.Context, user uuid.UUID) error
}

// SearchIndexer refreshes a search document (*searchengine.Indexer).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// SetRevoker enables immediate access token revocation (nil: refresh tokens
// are still revoked and the identity loader refuses anonymized users).
func (s *Service) SetRevoker(r Revoker) { s.revoker = r }

// SetSearchIndexer refreshes the users search document after anonymization
// so the old name, e-mail and phone leave the index, and keeps the customers
// index in sync (TEC-164).
func (s *Service) SetSearchIndexer(i SearchIndexer) { s.search = i }

// AnonymizeResult is the outcome of an anonymization. Changed is false when
// the customer was already anonymized (idempotent call).
type AnonymizeResult struct {
	UUID         uuid.UUID `json:"uuid"`
	Status       string    `json:"status"`
	Changed      bool      `json:"changed"`
	AnonymizedAt time.Time `json:"anonymized_at"`
}

// AnonymizeCustomer anonymizes a customer of the caller's brand (center only,
// customers.anonymize). meta is the request origin for the audit row.
func (s *Service) AnonymizeCustomer(ctx context.Context, c Caller, id uuid.UUID, meta activity.Meta) (AnonymizeResult, error) {
	if c.Org.OrgType != rbac.OrgTypeCenter {
		return AnonymizeResult{}, ErrForbidden
	}
	var res AnonymizeResult
	var userUUID uuid.UUID
	err := s.inTx(ctx, func(q *db.Queries) error {
		user, err := q.LockUserByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCustomerNotFound
		}
		if err != nil {
			return fmt.Errorf("customers: lock user: %w", err)
		}
		if err := s.requireInScope(ctx, q, c, user.ID); err != nil {
			return err
		}
		userUUID = user.Uuid
		res = AnonymizeResult{UUID: user.Uuid, Status: StatusAnonymized}
		if user.Status == StatusAnonymized {
			prof, err := q.GetCustomerProfile(ctx, user.ID)
			if err == nil && prof.AnonymizedAt.Valid {
				res.AnonymizedAt = prof.AnonymizedAt.Time
			} else {
				res.AnonymizedAt = user.UpdatedAt.Time
			}
			return s.audit(ctx, q, c, user.Uuid, ActionAnonymized, map[string]any{"changed": false}, meta)
		}
		if err := s.requireCustomerOnly(ctx, q, user.ID); err != nil {
			return err
		}
		// TEC-263: the old hub's archived messages to this person (by
		// account and by phone, read before the phone is cleared) are
		// masked; the only change legacy_messages allows.
		if _, err := q.AnonymizeLegacyMessages(ctx, db.AnonymizeLegacyMessagesParams{
			UserID: pgtype.Int8{Int64: user.ID, Valid: true}, Phone: user.PhoneE164,
		}); err != nil {
			return fmt.Errorf("customers: anonymize legacy messages: %w", err)
		}
		hash, err := unusablePasswordHash()
		if err != nil {
			return fmt.Errorf("customers: password: %w", err)
		}
		if _, err := q.AnonymizeUser(ctx, db.AnonymizeUserParams{ID: user.ID, PasswordHash: hash}); err != nil {
			return fmt.Errorf("customers: anonymize user: %w", err)
		}
		if err := q.EnsureCustomerProfile(ctx, user.ID); err != nil {
			return fmt.Errorf("customers: profile: %w", err)
		}
		prof, err := q.AnonymizeCustomerProfile(ctx, user.ID)
		if err != nil {
			return fmt.Errorf("customers: anonymize profile: %w", err)
		}
		if err := q.RevokeAllRefreshTokensForUser(ctx, user.ID); err != nil {
			return fmt.Errorf("customers: revoke sessions: %w", err)
		}
		res.Changed = true
		res.AnonymizedAt = prof.AnonymizedAt.Time
		// No personal data in the payload: the audit log outlives the person.
		return s.audit(ctx, q, c, user.Uuid, ActionAnonymized, map[string]any{"changed": true}, meta)
	})
	if err != nil {
		return AnonymizeResult{}, err
	}
	if res.Changed {
		if s.revoker != nil {
			// Best effort: the identity loader already refuses the account.
			_ = s.revoker.RevokeUser(ctx, userUUID)
		}
		if s.search != nil {
			s.search.EnqueueUpsert(ctx, adapters.SpecUsers, userUUID.String())
			// TEC-164: an anonymized customer leaves the customers index.
			s.search.EnqueueDelete(ctx, SearchSpec, userUUID.String())
			// TEC-209: the services / warranties documents lose the
			// person, the vehicles leave the index (K19).
			s.indexCustomerRecords(ctx, userUUID)
		}
	}
	return res, nil
}

// requireCustomerOnly refuses users with an organization membership or a
// role other than customer/fleet.
func (s *Service) requireCustomerOnly(ctx context.Context, q *db.Queries, userID int64) error {
	members, err := q.CountOrganizationMembershipsByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("customers: memberships: %w", err)
	}
	if members > 0 {
		return ErrPanelAccount
	}
	roles, err := q.ListUserRoleSlugs(ctx, userID)
	if err != nil {
		return fmt.Errorf("customers: roles: %w", err)
	}
	for _, r := range roles {
		if r != rbac.RoleCustomer && r != rbac.RoleFleet {
			return ErrPanelAccount
		}
	}
	return nil
}

func (s *Service) audit(ctx context.Context, q *db.Queries, c Caller, target uuid.UUID, action string, payload map[string]any, meta activity.Meta) error {
	actor := c.UserID
	var actorPtr *int64
	if actor != 0 {
		actorPtr = &actor
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if c.Org.UUID != uuid.Nil {
		payload["organization_uuid"] = c.Org.UUID.String()
	}
	if err := activity.Write(ctx, q, actorPtr, action, "customers", &target, payload, meta); err != nil {
		return fmt.Errorf("customers: audit: %w", err)
	}
	return nil
}

// ResolveExportTarget checks that the panel caller may export the customer's
// data (center organization, customer linked to the brand) and returns the
// customer's internal id.
func (s *Service) ResolveExportTarget(ctx context.Context, c Caller, id uuid.UUID) (int64, error) {
	if c.Org.OrgType != rbac.OrgTypeCenter {
		return 0, ErrForbidden
	}
	user, err := s.scopedUser(ctx, s.q, c, id)
	if err != nil {
		return 0, err
	}
	return user.ID, nil
}

// AuditExportRequest writes the audit row of a data export request (actor,
// target customer, export job, format, channel).
func (s *Service) AuditExportRequest(ctx context.Context, actorID int64, org uuid.UUID, target, job uuid.UUID, format, channel string, meta activity.Meta) error {
	c := Caller{UserID: actorID}
	c.Org.UUID = org
	return s.audit(ctx, s.q, c, target, ActionDataExportRequested, map[string]any{
		"export_job_uuid": job.String(), "format": format, "channel": channel,
	}, meta)
}
