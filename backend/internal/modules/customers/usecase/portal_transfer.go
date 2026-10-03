package usecase

// TEC-243 (F2-03d): vehicle transfer from the customer portal. The portal
// endpoints wrap the TEC-190 flow (same codes, attempts, expiry, events and
// audit; no new rule). The difference is the reach: a portal caller is the
// signed-in customer of the domain brand and reaches only the vehicles they
// own (vehicles.user_id) and the transfers they started (from_user_id);
// everything else answers 404 like an unknown record.
//
// The transfer row needs an organization (organization_id, the country of
// the phone number, the sender name of the code messages and the
// organization the new owner is linked to). From the portal it is the
// organization that registered the vehicle; when that is unset, the
// customer's first organization link in the brand, then the brand center.

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// PortalCaller builds the caller of a portal session: the domain brand and
// the signed-in user (internal ids). A zero brand or user reaches nothing.
func PortalCaller(brandID, userID int64, locale i18n.Locale) Caller {
	return Caller{
		UserID: userID,
		Org:    orgctx.Scope{BrandID: brandID},
		Filter: scopefilter.Filter{Scope: rbac.ScopeBrand, BrandID: brandID, UserID: userID},
		Locale: locale,
		// -1 never matches a user id: a session without a user reaches no
		// vehicle and no transfer.
		portalUserID: portalUser(userID),
	}
}

func portalUser(id int64) int64 {
	if id <= 0 {
		return -1
	}
	return id
}

// IsPortal reports whether the caller was built by PortalCaller.
func (c Caller) IsPortal() bool { return c.portalUserID != 0 }

// PortalStartTransfer (POST /v1/portal/vehicles/{uuid}/transfers): the
// owner starts the transfer of their own vehicle.
func (s *Service) PortalStartTransfer(ctx context.Context, c Caller, vehicleID uuid.UUID, in StartTransferInput,
	meta activity.Meta) (VehicleTransferView, error) {
	if !c.IsPortal() {
		return VehicleTransferView{}, ErrForbidden
	}
	v, err := s.scopedVehicle(ctx, s.q, c, vehicleID, false)
	if err != nil {
		return VehicleTransferView{}, err
	}
	org, err := s.portalTransferOrganization(ctx, c, v)
	if err != nil {
		return VehicleTransferView{}, err
	}
	c.Org = orgctx.Scope{
		InternalID: org.ID, UUID: org.Uuid, Slug: org.Slug, Name: org.Name, Status: org.Status,
		OrgType: org.Type, BrandID: org.BrandID,
	}
	return s.StartTransfer(ctx, c, vehicleID, in, meta)
}

// portalTransferOrganization picks the organization of a portal transfer
// (see the package comment above).
func (s *Service) portalTransferOrganization(ctx context.Context, c Caller, v db.Vehicle) (db.Organization, error) {
	if v.OrganizationID.Valid {
		o, err := s.q.GetOrganizationByID(ctx, v.OrganizationID.Int64)
		switch {
		case err == nil && o.BrandID == c.Org.BrandID && !o.DeletedAt.Valid:
			return o, nil
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return db.Organization{}, fmt.Errorf("customers: vehicle organization: %w", err)
		}
	}
	links, err := s.q.ListCustomerOrganizationsByUser(ctx, db.ListCustomerOrganizationsByUserParams{
		UserID: v.UserID, BrandID: pgtype.Int8{Int64: c.Org.BrandID, Valid: true},
	})
	if err != nil {
		return db.Organization{}, fmt.Errorf("customers: organization links: %w", err)
	}
	if len(links) > 0 {
		o, err := s.q.GetOrganizationByID(ctx, links[0].OrganizationID)
		if err != nil {
			return db.Organization{}, fmt.Errorf("customers: linked organization: %w", err)
		}
		return o, nil
	}
	o, err := s.q.GetBrandCenter(ctx, c.Org.BrandID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrTransferUnavailable
	}
	if err != nil {
		return db.Organization{}, fmt.Errorf("customers: brand center: %w", err)
	}
	return o, nil
}
