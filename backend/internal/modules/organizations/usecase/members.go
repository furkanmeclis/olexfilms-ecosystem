package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TEC-1036 membership guards.
var (
	// ErrLastOwner: the only owner of an organization cannot be demoted or
	// removed.
	ErrLastOwner = errors.New("organization must keep at least one owner")
	// ErrSelfMembership: the caller cannot remove their own membership or
	// take roles away from it.
	ErrSelfMembership = errors.New("own membership cannot be removed or narrowed")
)

// UpdateMemberInput replaces the membership kind and organization roles of
// a member (TEC-1036). Empty RoleSlugs means the default role of the kind.
type UpdateMemberInput struct {
	UserUUID  uuid.UUID
	Role      string
	RoleSlugs []string
}

// MemberView is a member after an update.
type MemberView struct {
	UserUUID  uuid.UUID `json:"uuid"`
	Role      string    `json:"role"`
	RoleSlugs []string  `json:"role_slugs"`
}

// UpdateMember changes the membership kind (owner/staff) and replaces the
// organization roles of a member (platform only). The last owner cannot be
// demoted and the caller cannot narrow their own membership. Writes the
// organizations.member_updated audit event and tenant.member_updated.
func (s *Service) UpdateMember(ctx context.Context, orgUUID uuid.UUID, actorID int64, meta activity.Meta, in UpdateMemberInput) (MemberView, error) {
	tree, err := s.brandOrg(ctx, orgUUID)
	if err != nil {
		return MemberView{}, err
	}
	org := tree.Organization
	role := strings.TrimSpace(in.Role)
	if role != "owner" && role != "staff" {
		return MemberView{}, fmt.Errorf("%w: invalid role", ErrInvalidRequest)
	}
	roleSlugs, err := validMemberRoles(org.Type, in.RoleSlugs)
	if err != nil {
		return MemberView{}, err
	}
	if len(roleSlugs) == 0 {
		roleSlugs = []string{rbac.DefaultMemberRole(org.Type, role)}
	}
	slices.Sort(roleSlugs)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MemberView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	member, err := q.GetOrganizationMemberByUserUUID(ctx, db.GetOrganizationMemberByUserUUIDParams{
		OrganizationID: org.ID, Uuid: in.UserUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MemberView{}, fmt.Errorf("%w: member not found", ErrNotFound)
		}
		return MemberView{}, err
	}
	before, err := q.ListMemberRoleSlugs(ctx, db.ListMemberRoleSlugsParams{UserID: member.UserID, OrganizationUuid: org.Uuid})
	if err != nil {
		return MemberView{}, err
	}
	if member.UserID == actorID {
		narrowed := member.Role == "owner" && role != "owner"
		for _, slug := range before {
			if !slices.Contains(roleSlugs, slug) {
				narrowed = true
			}
		}
		if narrowed {
			return MemberView{}, ErrSelfMembership
		}
	}
	if member.Role == "owner" && role != "owner" {
		if err := lockMembers(ctx, tx, org.ID); err != nil {
			return MemberView{}, err
		}
		if err := ensureAnotherOwner(ctx, q, org.ID); err != nil {
			return MemberView{}, err
		}
	}
	if role != member.Role {
		if err := q.UpdateOrganizationMemberKind(ctx, db.UpdateOrganizationMemberKindParams{Role: role, ID: member.ID}); err != nil {
			return MemberView{}, err
		}
	}
	if err := q.DeleteMemberRoles(ctx, member.ID); err != nil {
		return MemberView{}, err
	}
	for _, slug := range roleSlugs {
		if err := q.AssignMemberRoleBySlug(ctx, db.AssignMemberRoleBySlugParams{MemberID: member.ID, Slug: slug}); err != nil {
			return MemberView{}, err
		}
	}
	userUUID := member.UserUuid
	if err := activity.Write(ctx, q, &actorID, "organizations.member_updated", "organizations", &org.Uuid, map[string]any{
		"organization_name": org.Name, "user_uuid": userUUID.String(), "email": member.Email,
		"role_before": member.Role, "role": role, "role_slugs_before": before, "role_slugs": roleSlugs,
	}, meta); err != nil {
		return MemberView{}, fmt.Errorf("organizations: activity: %w", err)
	}
	if s.out != nil {
		if err := s.out.Enqueue(ctx, tx, memberEvent(events.TenantMemberUpdated, org.ID, member.UserID, role)); err != nil {
			return MemberView{}, fmt.Errorf("organizations: outbox: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return MemberView{}, err
	}
	return MemberView{UserUUID: userUUID, Role: role, RoleSlugs: roleSlugs}, nil
}

// RemoveMember removes a user from an organization (platform only). Its
// organization roles, warehouse memberships and staff targets go with it
// (FK cascade). The last owner and the caller's own membership cannot be
// removed. Writes organizations.member_removed and tenant.member_removed.
func (s *Service) RemoveMember(ctx context.Context, orgUUID uuid.UUID, actorID int64, meta activity.Meta, userUUID uuid.UUID) error {
	tree, err := s.brandOrg(ctx, orgUUID)
	if err != nil {
		return err
	}
	org := tree.Organization
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	member, err := q.GetOrganizationMemberByUserUUID(ctx, db.GetOrganizationMemberByUserUUIDParams{
		OrganizationID: org.ID, Uuid: userUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: member not found", ErrNotFound)
		}
		return err
	}
	if member.UserID == actorID {
		return ErrSelfMembership
	}
	if member.Role == "owner" {
		if err := lockMembers(ctx, tx, org.ID); err != nil {
			return err
		}
		if err := ensureAnotherOwner(ctx, q, org.ID); err != nil {
			return err
		}
	}
	before, err := q.ListMemberRoleSlugs(ctx, db.ListMemberRoleSlugsParams{UserID: member.UserID, OrganizationUuid: org.Uuid})
	if err != nil {
		return err
	}
	if err := q.DeleteOrganizationMember(ctx, member.ID); err != nil {
		return err
	}
	if err := activity.Write(ctx, q, &actorID, "organizations.member_removed", "organizations", &org.Uuid, map[string]any{
		"organization_name": org.Name, "user_uuid": member.UserUuid.String(), "email": member.Email,
		"role": member.Role, "role_slugs": before,
	}, meta); err != nil {
		return fmt.Errorf("organizations: activity: %w", err)
	}
	if s.out != nil {
		if err := s.out.Enqueue(ctx, tx, memberEvent(events.TenantMemberRemoved, org.ID, member.UserID, member.Role)); err != nil {
			return fmt.Errorf("organizations: outbox: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// lockMembers serializes owner changes of one organization so two parallel
// demotions cannot both pass the last-owner check.
func lockMembers(ctx context.Context, tx pgx.Tx, orgID int64) error {
	_, err := tx.Exec(ctx, `SELECT 1 FROM organization_members WHERE organization_id = $1 AND role = 'owner' FOR UPDATE`, orgID)
	return err
}

func ensureAnotherOwner(ctx context.Context, q *db.Queries, orgID int64) error {
	owners, err := q.CountOrganizationOwners(ctx, orgID)
	if err != nil {
		return err
	}
	if owners <= 1 {
		return ErrLastOwner
	}
	return nil
}

func memberEvent(name string, orgID, userID int64, memberRole string) events.Event {
	return events.New(name).WithTenant(orgID).WithPayload(map[string]any{
		"organization_id": orgID, "user_id": userID, "role": memberRole,
	})
}
