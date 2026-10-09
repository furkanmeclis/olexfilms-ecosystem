package usecase

// TEC-497 (F5-05h): the member picker of the performance screens (staff
// targets grid, weak dealer rule assignee) and the uuid based rule
// assignee.

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type MemberView struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Role string    `json:"role"`
}

// ListMembers lists the active members of the active organization.
func (s *Service) ListMembers(ctx context.Context, c Caller) ([]MemberView, error) {
	rows, err := s.q.ListOrganizationMembers(ctx, c.Org.InternalID)
	if err != nil {
		return nil, err
	}
	out := make([]MemberView, 0, len(rows))
	for _, r := range rows {
		if r.Status != "active" {
			continue
		}
		out = append(out, MemberView{UUID: r.Uuid, Name: strings.TrimSpace(r.Name + " " + r.Surname), Role: r.Role})
	}
	return out, nil
}

// resolveAssignee turns assignee_user_uuid into the member's user id and
// keeps automatic tasks to center rules (Q19: a distributor rule only
// notifies). The assignee is the task's (chk_weak_dealer_rules_assignee).
func (s *Service) resolveAssignee(ctx context.Context, c Caller, in *RuleInput) error {
	if in.CreateTask && c.Org.OrgType != "center" {
		return invalid("create_task", "only center rules open tasks")
	}
	if !in.CreateTask && (in.AssigneeUserUUID != nil || in.AssigneeUserID != nil) {
		return invalid("assignee_user_uuid", "requires create_task")
	}
	if in.AssigneeUserUUID == nil {
		return nil
	}
	m, err := s.q.GetOrganizationMemberByUserUUID(ctx, db.GetOrganizationMemberByUserUUIDParams{OrganizationID: c.Org.InternalID, Uuid: *in.AssigneeUserUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid("assignee_user_uuid", "must be a member of the organization")
	}
	if err != nil {
		return err
	}
	in.AssigneeUserID = &m.UserID
	return nil
}

func (s *Service) ruleByIDView(ctx context.Context, c Caller, row db.WeakDealerRule) (RuleView, error) {
	rows, err := s.ListRules(ctx, c, RuleFilter{})
	if err != nil {
		return RuleView{}, err
	}
	for _, r := range rows {
		if r.UUID == row.Uuid {
			return r, nil
		}
	}
	return RuleView{}, ErrNotFound
}
