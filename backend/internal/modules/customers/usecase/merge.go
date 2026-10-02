package usecase

// TEC-193 (F1-08c2): admin customer merge (TEC-100 decision 2, K26).
//
// Two users of the same person are folded into one: the source keeps its
// row (20 foreign keys cascade from users, nothing is deleted), gets
// merged_into_user_id = target, status disabled and an unusable password;
// its refresh tokens are revoked and live access tokens are cut (identity
// loader status check + revocation store). Vehicles, services, warranties,
// organization links, consents and the profile move to the target:
//
//   - organization links: a link the target already has for the same
//     organization is folded into the target's row (earliest dates kept);
//   - services follow their vehicle; a service whose vehicle was later
//     transferred to a third person stays with the source (the services
//     trigger requires the vehicle to belong to the customer);
//   - consents: a decision the target already made for the same legal text
//     wins, the source's stays as a record;
//   - profile: moved when the target has none, otherwise the target only
//     fills its empty fields;
//   - phone / e-mail: handed to the target when it has none, so the person
//     can still sign in with them;
//   - customer cari accounts (ledgers) are not moved, only reported.
//
// The preview (dry run) runs exactly the same statements in a transaction
// that is rolled back, so its counts are those of the apply; it writes no
// audit row and no event. The apply commits everything, the audit row and
// the customer.merged outbox event in one transaction.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine/adapters"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Merge refusals (409, one code each).
var (
	// ErrMergeSelf: source and target are the same user.
	ErrMergeSelf = errors.New("customers: cannot merge a customer into itself")
	// ErrMergeChain: the source or the target was already merged.
	ErrMergeChain = errors.New("customers: customer was already merged")
	// ErrMergeAnonymized: the source or the target is anonymized (K19).
	ErrMergeAnonymized = errors.New("customers: anonymized customers cannot be merged")
	// ErrMergePendingTransfer: a vehicle transfer of the source is open.
	ErrMergePendingTransfer = errors.New("customers: customer has a pending vehicle transfer")
)

// ActionMerged is the audit action of an applied merge.
const ActionMerged = "customers.merged"

// errDryRun rolls the preview transaction back.
var errDryRun = errors.New("customers: dry run")

// SetOutbox enables the customer.merged event (nil: no event).
func (s *Service) SetOutbox(out outbox.Enqueuer) { s.out = out }

// MergeCounts are the records moved (or, in a preview, to be moved) to the
// target.
type MergeCounts struct {
	Vehicles          int64 `json:"vehicles"`
	Services          int64 `json:"services"`
	Warranties        int64 `json:"warranties"`
	OrganizationLinks int64 `json:"organization_links"`
	Consents          int64 `json:"consents"`
}

// MergeConflicts are unique-key collisions resolved by folding, and records
// that stay with the source.
type MergeConflicts struct {
	// OrganizationLinks folded into the target's link of the same
	// organization.
	OrganizationLinks int64 `json:"organization_links"`
	// ConsentsKept: the target already decided the same legal text.
	ConsentsKept int64 `json:"consents_kept"`
	// ServicesKept: the vehicle now belongs to a third person.
	ServicesKept int64 `json:"services_kept"`
	// CariAccountsKept: customer cari accounts are ledgers and stay.
	CariAccountsKept int64 `json:"cari_accounts_kept"`
}

// MergeResult is the outcome of a merge preview or apply.
type MergeResult struct {
	SourceUUID uuid.UUID      `json:"source_uuid"`
	TargetUUID uuid.UUID      `json:"target_uuid"`
	DryRun     bool           `json:"dry_run"`
	Moved      MergeCounts    `json:"moved"`
	Conflicts  MergeConflicts `json:"conflicts"`
	// Profile: moved (target had none), merged (target filled its empty
	// fields), kept (only the target has one) or none.
	Profile         string     `json:"profile"`
	PhoneMoved      bool       `json:"phone_moved"`
	EmailMoved      bool       `json:"email_moved"`
	SessionsRevoked int64      `json:"sessions_revoked"`
	MergedAt        *time.Time `json:"merged_at,omitempty"`
}

// Profile outcomes.
const (
	ProfileMoved  = "moved"
	ProfileMerged = "merged"
	ProfileKept   = "kept"
	ProfileNone   = "none"
)

// PreviewMerge reports what MergeCustomer would move, without writing.
func (s *Service) PreviewMerge(ctx context.Context, c Caller, source, target uuid.UUID) (MergeResult, error) {
	return s.merge(ctx, c, source, target, true, activity.Meta{})
}

// MergeCustomer merges source into target (center only, customers.merge).
func (s *Service) MergeCustomer(ctx context.Context, c Caller, source, target uuid.UUID, meta activity.Meta) (MergeResult, error) {
	return s.merge(ctx, c, source, target, false, meta)
}

func (s *Service) merge(ctx context.Context, c Caller, sourceID, targetID uuid.UUID, dryRun bool, meta activity.Meta) (MergeResult, error) {
	if c.Org.OrgType != rbac.OrgTypeCenter {
		return MergeResult{}, ErrForbidden
	}
	if sourceID == targetID {
		return MergeResult{}, ErrMergeSelf
	}
	var res MergeResult
	err := s.inTxRaw(ctx, func(q *db.Queries, tx pgx.Tx) error {
		src, dst, err := s.lockMergePair(ctx, q, c, sourceID, targetID)
		if err != nil {
			return err
		}
		res, err = s.applyMerge(ctx, q, src, dst)
		if err != nil {
			return err
		}
		res.DryRun = dryRun
		if dryRun {
			return errDryRun
		}
		now := time.Now().UTC()
		res.MergedAt = &now
		payload := mergePayload(res)
		if err := s.audit(ctx, q, c, src.Uuid, ActionMerged, payload, meta); err != nil {
			return err
		}
		if s.out != nil {
			id, uid := src.ID, src.Uuid
			evPayload := mergePayload(res)
			evPayload["source_user_id"] = src.ID
			evPayload["target_user_id"] = dst.ID
			evPayload["brand_id"] = c.Org.BrandID
			ev := events.New(events.CustomerMerged).WithTenant(c.Org.InternalID).
				WithEntity("user", &id, &uid).WithPayload(evPayload)
			if c.UserID != 0 {
				ev = ev.WithActor(c.UserID)
			}
			if err := s.out.Enqueue(ctx, tx, ev); err != nil {
				return fmt.Errorf("customers: outbox: %w", err)
			}
		}
		return nil
	})
	if errors.Is(err, errDryRun) {
		return res, nil
	}
	if err != nil {
		return MergeResult{}, err
	}
	if s.revoker != nil {
		// Best effort: the identity loader already refuses the account.
		_ = s.revoker.RevokeUser(ctx, res.SourceUUID)
	}
	if s.search != nil {
		s.search.EnqueueUpsert(ctx, adapters.SpecUsers, res.SourceUUID.String())
		s.search.EnqueueUpsert(ctx, adapters.SpecUsers, res.TargetUUID.String())
	}
	return res, nil
}

// lockMergePair locks both users and checks every refusal. Order: existence
// and scope of the target, then the source (a merged source has no links
// any more, so its scope is checked through the target it was merged
// into), then chain, anonymized, panel account and open transfers.
func (s *Service) lockMergePair(ctx context.Context, q *db.Queries, c Caller, sourceID, targetID uuid.UUID) (db.User, db.User, error) {
	srcRef, err := q.GetUserByUUID(ctx, sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, db.User{}, ErrCustomerNotFound
	}
	if err != nil {
		return db.User{}, db.User{}, fmt.Errorf("customers: source: %w", err)
	}
	dstRef, err := q.GetUserByUUID(ctx, targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, db.User{}, ErrCustomerNotFound
	}
	if err != nil {
		return db.User{}, db.User{}, fmt.Errorf("customers: target: %w", err)
	}
	rows, err := q.LockUsersForMerge(ctx, []int64{srcRef.ID, dstRef.ID})
	if err != nil {
		return db.User{}, db.User{}, fmt.Errorf("customers: lock users: %w", err)
	}
	var src, dst db.User
	for _, u := range rows {
		switch u.ID {
		case srcRef.ID:
			src = u
		case dstRef.ID:
			dst = u
		}
	}
	if src.ID == 0 || dst.ID == 0 {
		return db.User{}, db.User{}, ErrCustomerNotFound
	}

	if err := s.requireInScope(ctx, q, c, dst.ID); err != nil {
		// A merged target has no links either: report the chain when the
		// user it points to is in scope.
		if dst.MergedIntoUserID.Valid && s.requireInScope(ctx, q, c, dst.MergedIntoUserID.Int64) == nil {
			return db.User{}, db.User{}, ErrMergeChain
		}
		return db.User{}, db.User{}, err
	}
	if src.MergedIntoUserID.Valid {
		if src.MergedIntoUserID.Int64 == dst.ID || s.requireInScope(ctx, q, c, src.MergedIntoUserID.Int64) == nil {
			return db.User{}, db.User{}, ErrMergeChain
		}
		return db.User{}, db.User{}, ErrCustomerNotFound
	}
	if err := s.requireInScope(ctx, q, c, src.ID); err != nil {
		return db.User{}, db.User{}, err
	}
	if dst.MergedIntoUserID.Valid {
		return db.User{}, db.User{}, ErrMergeChain
	}
	if src.Status == StatusAnonymized || dst.Status == StatusAnonymized {
		return db.User{}, db.User{}, ErrMergeAnonymized
	}
	if dst.Status != StatusActive {
		return db.User{}, db.User{}, ErrInactive
	}
	if err := s.requireCustomerOnly(ctx, q, src.ID); err != nil {
		return db.User{}, db.User{}, err
	}
	if err := s.requireCustomerOnly(ctx, q, dst.ID); err != nil {
		return db.User{}, db.User{}, err
	}
	pending, err := q.CountPendingVehicleTransfersForUser(ctx, src.ID)
	if err != nil {
		return db.User{}, db.User{}, fmt.Errorf("customers: pending transfers: %w", err)
	}
	if pending > 0 {
		return db.User{}, db.User{}, ErrMergePendingTransfer
	}
	return src, dst, nil
}

// applyMerge runs every statement of the merge inside the caller's
// transaction and returns the counts.
func (s *Service) applyMerge(ctx context.Context, q *db.Queries, src, dst db.User) (MergeResult, error) {
	res := MergeResult{SourceUUID: src.Uuid, TargetUUID: dst.Uuid, Profile: ProfileNone}
	var err error
	fail := func(step string, e error) (MergeResult, error) {
		return MergeResult{}, fmt.Errorf("customers: merge %s: %w", step, e)
	}

	// 1. Organization links: fold duplicates, move the rest.
	if _, err = q.MergeConflictingCustomerOrganizations(ctx, db.MergeConflictingCustomerOrganizationsParams{
		SourceUserID: src.ID, TargetUserID: dst.ID}); err != nil {
		return fail("organization links", err)
	}
	if res.Conflicts.OrganizationLinks, err = q.DeleteMergedCustomerOrganizations(ctx, db.DeleteMergedCustomerOrganizationsParams{
		SourceUserID: src.ID, TargetUserID: dst.ID}); err != nil {
		return fail("organization links", err)
	}
	if res.Moved.OrganizationLinks, err = q.MoveCustomerOrganizations(ctx, db.MoveCustomerOrganizationsParams{
		SourceUserID: src.ID, TargetUserID: dst.ID}); err != nil {
		return fail("organization links", err)
	}

	// 2. Vehicles, then the services that follow them, then warranties.
	s1, t1 := src.ID, dst.ID
	if res.Moved.Vehicles, err = q.MoveVehiclesToUser(ctx, db.MoveVehiclesToUserParams{SourceUserID: s1, TargetUserID: t1}); err != nil {
		return fail("vehicles", err)
	}
	if res.Moved.Services, err = q.MoveServicesToUser(ctx, db.MoveServicesToUserParams{SourceUserID: s1, TargetUserID: t1}); err != nil {
		return fail("services", err)
	}
	if res.Conflicts.ServicesKept, err = q.CountServicesOfUser(ctx, src.ID); err != nil {
		return fail("services", err)
	}
	if res.Moved.Warranties, err = q.MoveWarrantiesToHolder(ctx, db.MoveWarrantiesToHolderParams{SourceUserID: s1, TargetUserID: t1}); err != nil {
		return fail("warranties", err)
	}

	// 3. Consents and cari accounts (reported only).
	if res.Moved.Consents, err = q.MoveConsentsToUser(ctx, db.MoveConsentsToUserParams{SourceUserID: s1, TargetUserID: t1}); err != nil {
		return fail("consents", err)
	}
	if res.Conflicts.ConsentsKept, err = q.CountConsentsOfUser(ctx, src.ID); err != nil {
		return fail("consents", err)
	}
	if res.Conflicts.CariAccountsKept, err = q.CountUserCariAccounts(ctx, pgtype.Int8{Int64: src.ID, Valid: true}); err != nil {
		return fail("cari accounts", err)
	}

	// 4. Profile.
	if res.Profile, err = s.mergeProfile(ctx, q, src.ID, dst.ID); err != nil {
		return fail("profile", err)
	}

	// 5. Close the source; hand over the identifiers the target lacks.
	if res.SessionsRevoked, err = q.CountActiveRefreshTokensForUser(ctx, src.ID); err != nil {
		return fail("sessions", err)
	}
	res.PhoneMoved = src.PhoneE164.Valid && !dst.PhoneE164.Valid
	res.EmailMoved = src.Email.Valid && !dst.Email.Valid
	keepPhone, keepEmail := src.PhoneE164, src.Email
	if res.PhoneMoved {
		keepPhone = pgtype.Text{}
	}
	if res.EmailMoved {
		keepEmail = pgtype.Text{}
	}
	if !keepPhone.Valid && !keepEmail.Valid {
		keepEmail = pgtype.Text{String: "merged+" + src.Uuid.String() + "@merged.invalid", Valid: true}
	}
	hash, err := unusablePasswordHash()
	if err != nil {
		return fail("password", err)
	}
	if _, err = q.CloseMergedUser(ctx, db.CloseMergedUserParams{
		ID: src.ID, TargetUserID: pgtype.Int8{Int64: dst.ID, Valid: true}, PasswordHash: hash,
		PhoneE164: keepPhone, Email: keepEmail,
	}); err != nil {
		return fail("close source", err)
	}
	if res.PhoneMoved || res.EmailMoved {
		in := db.TakeOverMergedIdentityParams{ID: dst.ID}
		if res.PhoneMoved {
			in.PhoneE164, in.PhoneVerifiedAt = src.PhoneE164, src.PhoneVerifiedAt
		}
		if res.EmailMoved {
			in.Email, in.EmailVerifiedAt = src.Email, src.EmailVerifiedAt
		}
		if err = q.TakeOverMergedIdentity(ctx, in); err != nil {
			return fail("identity", err)
		}
	}
	if err = q.RevokeAllRefreshTokensForUser(ctx, src.ID); err != nil {
		return fail("sessions", err)
	}
	return res, nil
}

func (s *Service) mergeProfile(ctx context.Context, q *db.Queries, srcID, dstID int64) (string, error) {
	_, srcErr := q.GetCustomerProfile(ctx, srcID)
	if srcErr != nil && !errors.Is(srcErr, pgx.ErrNoRows) {
		return "", srcErr
	}
	_, dstErr := q.GetCustomerProfile(ctx, dstID)
	if dstErr != nil && !errors.Is(dstErr, pgx.ErrNoRows) {
		return "", dstErr
	}
	hasSrc, hasDst := srcErr == nil, dstErr == nil
	switch {
	case hasSrc && !hasDst:
		if _, err := q.MoveCustomerProfile(ctx, db.MoveCustomerProfileParams{SourceUserID: srcID, TargetUserID: dstID}); err != nil {
			return "", err
		}
		return ProfileMoved, nil
	case hasSrc && hasDst:
		if _, err := q.FillCustomerProfileFromSource(ctx, db.FillCustomerProfileFromSourceParams{SourceUserID: srcID, TargetUserID: dstID}); err != nil {
			return "", err
		}
		return ProfileMerged, nil
	case hasDst:
		return ProfileKept, nil
	default:
		return ProfileNone, nil
	}
}

// mergePayload carries no personal data (the audit log outlives people).
func mergePayload(r MergeResult) map[string]any {
	return map[string]any{
		"target_uuid":        r.TargetUUID.String(),
		"vehicles":           r.Moved.Vehicles,
		"services":           r.Moved.Services,
		"warranties":         r.Moved.Warranties,
		"organization_links": r.Moved.OrganizationLinks,
		"consents":           r.Moved.Consents,
		"links_folded":       r.Conflicts.OrganizationLinks,
		"services_kept":      r.Conflicts.ServicesKept,
		"consents_kept":      r.Conflicts.ConsentsKept,
		"cari_accounts_kept": r.Conflicts.CariAccountsKept,
		"profile":            r.Profile,
		"phone_moved":        r.PhoneMoved,
		"email_moved":        r.EmailMoved,
		"sessions_revoked":   r.SessionsRevoked,
	}
}

// inTxRaw is inTx with the transaction handle (outbox writes need it).
func (s *Service) inTxRaw(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("customers: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("customers: commit: %w", err)
	}
	return nil
}
