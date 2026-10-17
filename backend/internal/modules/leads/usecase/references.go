package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// resolveAssignee uses the UUID in preference to the legacy internal id.
func (s *Service) resolveAssignee(ctx context.Context, orgID int64, id *int64, uid *uuid.UUID) (*int64, error) {
	bad := func() error { return invalid("assignee_user_id", "must be a member of the organization") }
	if uid != nil {
		user, err := s.q.GetCenterMemberByUUID(ctx, db.GetCenterMemberByUUIDParams{Uuid: *uid, OrganizationID: orgID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, bad()
		}
		if err != nil {
			return nil, fmt.Errorf("leads: assignee: %w", err)
		}
		return &user.ID, nil
	}
	if id == nil {
		return nil, nil
	}
	user, err := s.q.GetUserByID(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && user.DeletedAt.Valid) {
		return nil, bad()
	}
	if err != nil {
		return nil, fmt.Errorf("leads: assignee: %w", err)
	}
	_, err = s.q.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{OrganizationID: orgID, UserID: *id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, bad()
	}
	if err != nil {
		return nil, fmt.Errorf("leads: assignee: %w", err)
	}
	return id, nil
}

func (s *Service) customerVisible(ctx context.Context, c Caller, id int64) (bool, error) {
	if c.Org.BrandID == 0 {
		return false, nil
	}
	return s.q.CustomerInScope(ctx, db.CustomerInScopeParams{UserID: id, OrgIds: c.Filter.OrgIDsArg(), BrandID: pgtype.Int8{Int64: c.Org.BrandID, Valid: true}})
}

func (s *Service) resolveCustomer(ctx context.Context, c Caller, id *int64, uid *uuid.UUID) (*int64, error) {
	bad := func() error { return invalid("customer_user_id", "must be a customer in scope") }
	if uid != nil {
		user, err := s.q.GetUserByUUID(ctx, *uid)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, bad()
		}
		if err != nil {
			return nil, fmt.Errorf("leads: customer: %w", err)
		}
		id = &user.ID
	}
	if id == nil {
		return nil, nil
	}
	user, err := s.q.GetUserByID(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && user.DeletedAt.Valid) {
		return nil, bad()
	}
	if err != nil {
		return nil, fmt.Errorf("leads: customer: %w", err)
	}
	ok, err := s.customerVisible(ctx, c, *id)
	if err != nil {
		return nil, fmt.Errorf("leads: customer scope: %w", err)
	}
	if !ok {
		return nil, bad()
	}
	return id, nil
}

func (s *Service) resolveVehicle(ctx context.Context, c Caller, id *int64, uid *uuid.UUID) (*int64, error) {
	if id == nil && uid == nil {
		return nil, nil
	}
	var vehicle db.Vehicle
	var err error
	if uid != nil {
		vehicle, err = s.q.GetVehicleByUUID(ctx, *uid)
	} else {
		vehicle, err = s.q.GetVehicleByID(ctx, *id)
	}
	bad := func() error { return invalid("vehicle_id", "must be a vehicle in scope") }
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, bad()
	}
	if err != nil {
		return nil, fmt.Errorf("leads: vehicle: %w", err)
	}
	if vehicle.DeletedAt.Valid || vehicle.BrandID != c.Org.BrandID {
		return nil, bad()
	}
	ok, err := s.customerVisible(ctx, c, vehicle.UserID)
	if err != nil {
		return nil, fmt.Errorf("leads: vehicle scope: %w", err)
	}
	if !ok {
		return nil, bad()
	}
	return &vehicle.ID, nil
}

func (s *Service) resolveReferences(ctx context.Context, c Caller, org int64, in *leadInput) error {
	var err error
	in.AssigneeUserID, err = s.resolveAssignee(ctx, org, in.AssigneeUserID, in.AssigneeUserUUID)
	if err != nil {
		return err
	}
	in.CustomerUserID, err = s.resolveCustomer(ctx, c, in.CustomerUserID, in.CustomerUUID)
	if err != nil {
		return err
	}
	in.VehicleID, err = s.resolveVehicle(ctx, c, in.VehicleID, in.VehicleUUID)
	return err
}

// A present nullable UUID wins over the numeric patch, including explicit null.
func (s *Service) resolvePatchReferences(ctx context.Context, c Caller, org int64, in *PatchInput) error {
	var err error
	if in.AssigneeUserUUID.Set {
		in.AssigneeUserID = Field[int64]{Set: true}
	}
	if in.AssigneeUserID.Set {
		in.AssigneeUserID.Value, err = s.resolveAssignee(ctx, org, in.AssigneeUserID.Value, in.AssigneeUserUUID.Value)
		if err != nil {
			return err
		}
	}
	if in.CustomerUUID.Set {
		in.CustomerUserID = Field[int64]{Set: true}
	}
	if in.CustomerUserID.Set {
		in.CustomerUserID.Value, err = s.resolveCustomer(ctx, c, in.CustomerUserID.Value, in.CustomerUUID.Value)
		if err != nil {
			return err
		}
	}
	if in.VehicleUUID.Set {
		in.VehicleID = Field[int64]{Set: true}
	}
	if in.VehicleID.Set {
		in.VehicleID.Value, err = s.resolveVehicle(ctx, c, in.VehicleID.Value, in.VehicleUUID.Value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) leadReferences(ctx context.Context, lead Lead) (Lead, error) {
	for _, ref := range []struct {
		id  *int64
		dst **uuid.UUID
	}{{lead.AssigneeUserID, &lead.AssigneeUserUUID}, {lead.CustomerUserID, &lead.CustomerUUID}} {
		if ref.id == nil {
			continue
		}
		user, err := s.q.GetUserByID(ctx, *ref.id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return Lead{}, fmt.Errorf("leads: user reference: %w", err)
		}
		uid := user.Uuid
		*ref.dst = &uid
	}
	if lead.VehicleID != nil {
		vehicle, err := s.q.GetVehicleByID(ctx, *lead.VehicleID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Lead{}, fmt.Errorf("leads: vehicle reference: %w", err)
		}
		if err == nil {
			lead.VehicleUUID = &vehicle.Uuid
		}
	}
	return lead, nil
}

// AssigneeOption is a member eligible for assignment in a scoped lead organization.
type AssigneeOption struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   *string   `json:"email,omitempty"`
}

func (s *Service) Assignees(ctx context.Context, c Caller, orgUUID *uuid.UUID, query string) ([]AssigneeOption, error) {
	org := c.Org.InternalID
	if orgUUID != nil {
		row, err := s.q.GetOrganizationByUUID(ctx, *orgUUID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !c.Filter.AllowsOrg(row.ID, row.BrandID)) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		org = row.ID
	}
	if !c.Filter.AllowsOrg(org, c.Org.BrandID) {
		return nil, ErrNotFound
	}
	rows, err := s.q.ListOrganizationMembers(ctx, org)
	if err != nil {
		return nil, fmt.Errorf("leads: assignees: %w", err)
	}
	out := make([]AssigneeOption, 0, len(rows))
	query = strings.ToLower(strings.TrimSpace(query))
	for _, row := range rows {
		if query != "" && !strings.Contains(strings.ToLower(row.Name+" "+row.Surname+" "+row.Email.String), query) {
			continue
		}
		out = append(out, AssigneeOption{UUID: row.Uuid, Name: row.Name, Surname: row.Surname, Email: textPtr(row.Email)})
	}
	return out, nil
}
