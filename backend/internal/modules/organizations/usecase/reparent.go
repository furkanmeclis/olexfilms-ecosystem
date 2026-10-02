package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Re-parenting (K25, TEC-198). ChangeParent moves an organization, records
// the move in organization_parent_changes, runs the ParentChangeHook (the
// accounting cari transfer) and writes the audit row, all in one
// transaction: a failing hook rolls the move back.

// AuditParentChanged is the audit action (activity_events.action) of a
// re-parenting.
const AuditParentChanged = "organization.parent_changed"

// ParentChange is one re-parenting. UUID is the change id
// (organization_parent_changes.uuid).
type ParentChange struct {
	UUID           uuid.UUID
	OrganizationID int64
	BrandID        int64
	OldParentID    int64
	NewParentID    int64
	ActorUserID    *int64
}

// ParentChangeHook runs inside the re-parenting transaction (the accounting
// cari transfer, K25).
type ParentChangeHook interface {
	ParentChangedTx(ctx context.Context, tx pgx.Tx, c ParentChange) error
}

// ParentChangeHookFunc adapts a function to ParentChangeHook.
type ParentChangeHookFunc func(ctx context.Context, tx pgx.Tx, c ParentChange) error

// ParentChangedTx calls f.
func (f ParentChangeHookFunc) ParentChangedTx(ctx context.Context, tx pgx.Tx, c ParentChange) error {
	return f(ctx, tx, c)
}

// SetParentChangeHook wires the hook run by ChangeParent (nil: none).
func (s *Service) SetParentChangeHook(h ParentChangeHook) { s.parentHook = h }

// moveTx moves child under parent in one transaction (see the notes above).
func (s *Service) moveTx(ctx context.Context, child, oldParent, parent db.Organization) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if _, err := q.UpdateOrganizationParent(ctx, db.UpdateOrganizationParentParams{
		ID: child.ID, ParentID: pgtype.Int8{Int64: parent.ID, Valid: true},
	}); err != nil {
		return err
	}
	var actor pgtype.Int8
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal != 0 {
		actor = pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	change, err := q.InsertOrganizationParentChange(ctx, db.InsertOrganizationParentChangeParams{
		OrganizationID: child.ID, BrandID: child.BrandID, OldParentID: oldParent.ID,
		NewParentID: parent.ID, ActorUserID: actor,
	})
	if err != nil {
		return fmt.Errorf("organizations: parent change: %w", err)
	}
	pc := ParentChange{
		UUID: change.Uuid, OrganizationID: child.ID, BrandID: child.BrandID,
		OldParentID: oldParent.ID, NewParentID: parent.ID,
	}
	if actor.Valid {
		id := actor.Int64
		pc.ActorUserID = &id
	}
	if s.parentHook != nil {
		if err := s.parentHook.ParentChangedTx(ctx, tx, pc); err != nil {
			return err
		}
	}
	payload, err := json.Marshal(map[string]any{
		"change_uuid":       change.Uuid.String(),
		"organization_id":   child.ID,
		"brand_id":          child.BrandID,
		"old_parent_uuid":   oldParent.Uuid.String(),
		"new_parent_uuid":   parent.Uuid.String(),
		"old_parent_id":     oldParent.ID,
		"new_parent_id":     parent.ID,
		"organization_type": child.Type,
	})
	if err != nil {
		return fmt.Errorf("organizations: audit payload: %w", err)
	}
	if _, err := q.InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID: actor, Action: AuditParentChanged, Resource: "organization",
		ResourceUuid: pgtype.UUID{Bytes: child.Uuid, Valid: true}, Payload: payload,
	}); err != nil {
		return fmt.Errorf("organizations: audit: %w", err)
	}
	return tx.Commit(ctx)
}
