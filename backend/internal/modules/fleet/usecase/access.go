package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	// ErrNotFound: the fleet (or its link, user, vehicle) does not exist or
	// is outside the caller's reach (404; a dealer reaches a fleet only
	// through an active link).
	ErrNotFound = errors.New("fleet: not found")
	// ErrForbidden: the caller's organization cannot do this (403).
	ErrForbidden = errors.New("fleet: forbidden")

	// ErrPrimaryUserRequired: the fleet has no user yet (422).
	ErrPrimaryUserRequired = &RuleError{
		Code: model.CodePrimaryUserRequired, Message: "invite the first fleet user (the vehicle owner) first",
		status: http.StatusUnprocessableEntity,
	}
	// ErrPrimaryUserLocked: the primary user is not disabled (422).
	ErrPrimaryUserLocked = &RuleError{
		Code: model.CodePrimaryUserLocked, Message: "the primary fleet user owns the fleet vehicles",
		status: http.StatusUnprocessableEntity,
	}
	// ErrUserEmailTaken: the invited e-mail has an account (409).
	ErrUserEmailTaken = &RuleError{
		Code: model.CodeUserEmailTaken, Message: "an account with this e-mail already exists",
		status: http.StatusConflict,
	}
	// ErrVehicleOtherOwner: the vehicle belongs to another customer or
	// fleet (409).
	ErrVehicleOtherOwner = &RuleError{
		Code: model.CodeVehicleOtherOwner, Message: "the vehicle belongs to another customer",
		status: http.StatusConflict,
	}
	// ErrNoDealerLink: the caller has no own link to the fleet (422).
	ErrNoDealerLink = &RuleError{
		Code: model.CodeNoDealerLink, Message: "your organization has no active link to this fleet",
		status: http.StatusUnprocessableEntity,
	}
	// ErrLinkNotPending: the link was already decided (409).
	ErrLinkNotPending = &RuleError{
		Code: model.CodeLinkNotPending, Message: "the link is not pending",
		status: http.StatusConflict,
	}
)

// VehicleExistsError is 409 FLEET_VEHICLE_EXISTS: a vehicle with this
// plate / VIN already belongs to the fleet's users (or the fleet). The
// client links it with POST /v1/fleets/{uuid}/vehicles {vehicle_uuid}.
type VehicleExistsError struct {
	VehicleUUID uuid.UUID `json:"vehicle_uuid"`
	Plate       string    `json:"plate,omitempty"`
	InFleet     bool      `json:"in_fleet"`
}

func (e *VehicleExistsError) Error() string { return "fleet: vehicle exists" }

// Caller is the panel principal in its active organization with the
// resolved scope of the route's permission (fleets.read / fleets.manage).
type Caller struct {
	UserID  int64
	OrgID   int64
	BrandID int64
	OrgType string
	Filter  scopefilter.Filter
}

// orgIDs is the organization reach (nil: the whole brand).
func (c Caller) orgIDs() []int64 { return c.Filter.OrgIDsArg() }

// brandWide reports a center (brand) or platform (all) scope: no link is
// needed to reach a fleet of the brand.
func (c Caller) brandWide() bool {
	return c.Filter.Scope == rbac.ScopeBrand || c.Filter.Scope == rbac.ScopeAll
}

// access is the caller's reach of one fleet.
type access struct {
	fleet db.GetFleetByUUIDRow
	// links are the active links to organizations in scope.
	links []db.FleetDealerLink
	// own is the caller organization's active link (nil: none).
	own *db.FleetDealerLink
}

func (a access) orgID() int64 { return a.fleet.Organization.ID }

// resolve loads a fleet of the caller's brand and checks the caller
// reaches it: an active link to an organization in scope, or a brand
// scope. Anything else is ErrNotFound.
func (s *Service) resolve(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (access, error) {
	f, err := q.GetFleetByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return access{}, ErrNotFound
	}
	if err != nil {
		return access{}, fmt.Errorf("fleet: get: %w", err)
	}
	if c.BrandID == 0 || f.Organization.BrandID != c.BrandID {
		return access{}, ErrNotFound
	}
	links, err := q.ListActiveFleetLinksInScope(ctx, db.ListActiveFleetLinksInScopeParams{
		FleetOrgID: f.Organization.ID, OrgIds: c.orgIDs(),
	})
	if err != nil {
		return access{}, fmt.Errorf("fleet: links: %w", err)
	}
	if len(links) == 0 && !c.brandWide() {
		return access{}, ErrNotFound
	}
	a := access{fleet: f, links: links}
	for i := range links {
		if links[i].DealerOrgID == c.OrgID {
			a.own = &links[i]
		}
	}
	return a, nil
}

// emit writes a fleet.* event in tx (no-op without an outbox). tenant is
// the acting organization.
func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, tenant, actor int64, fleet db.Organization, extra map[string]any) error {
	if s.out == nil {
		return nil
	}
	payload := map[string]any{
		"fleet_uuid": fleet.Uuid.String(), "fleet_id": fleet.ID, "brand_id": fleet.BrandID, "fleet_name": fleet.Name,
		// The search sync refreshes the fleet's organizations document.
		"organization_uuid": fleet.Uuid.String(),
	}
	for k, v := range extra {
		payload[k] = v
	}
	id, u := fleet.ID, fleet.Uuid
	ev := events.New(name).WithTenant(tenant).WithEntity("organization", &id, &u).WithPayload(payload)
	if actor != 0 {
		ev = ev.WithActor(actor)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("fleet: outbox: %w", err)
	}
	return nil
}

// inTx runs fn in one transaction on the pool.
func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(db.New(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Check reports whether the caller reaches the fleet (ErrNotFound when not):
// the vehicle import upload checks it before the job is created.
func (s *Service) Check(ctx context.Context, c Caller, id uuid.UUID) error {
	_, err := s.resolve(ctx, db.New(s.conn), c, id)
	return err
}
