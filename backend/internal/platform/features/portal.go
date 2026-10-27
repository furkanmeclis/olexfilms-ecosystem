package features

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// TEC-1023: the customer / fleet portal has no active organization, so a
// module's portal state is derived from the organizations behind the
// session. PortalModules lists the customer-facing modules a portal
// session may use (GET /v1/portal/features):
//
//   - a module closed system wide is never listed;
//   - appointments: at least one dealer of the brand that takes portal
//     bookings (active, access window open, portal bookings on) has the
//     module on. Booking is brand wide (any dealer from the dealer finder),
//     so a new customer without a linked organization can still book;
//   - warranty_claims, reviews: at least one organization the customer is
//     linked to (customer_organizations, or the organization of a service
//     the user owns / holds a warranty of) has the module on;
//   - fleet: the user is an active fleet user of the brand and at least one
//     actively linked dealer of the fleet has the module on (the rule that
//     opens /v1/portal/fleet/*).
//
// Vehicle transfer and the other portal areas ride on core modules
// (customers, services), which are always on, so they are not listed.

// PortalModuleKeys are the customer-facing modules, in response order.
var PortalModuleKeys = []string{ModuleAppointments, ModuleWarrantyClaims, ModuleReviews, ModuleFleet}

// PortalModules returns the enabled customer-facing module keys of a portal
// session (user and request brand), in PortalModuleKeys order.
func (s *Service) PortalModules(ctx context.Context, userID, brandID int64) ([]string, error) {
	out := []string{}
	if userID <= 0 || brandID <= 0 {
		return out, nil
	}
	var linked []int64
	linkedLoaded := false
	for _, key := range PortalModuleKeys {
		on, err := s.SystemEnabled(ctx, key)
		if err != nil {
			return nil, err
		}
		if !on {
			continue
		}
		var orgs []int64
		switch key {
		case ModuleAppointments:
			orgs, err = s.q.ListPortalBookableDealerIDs(ctx, brandID)
		case ModuleFleet:
			orgs, err = s.q.ListPortalFleetActiveDealerIDs(ctx, db.ListPortalFleetActiveDealerIDsParams{
				UserID: userID, BrandID: brandID,
			})
		default:
			if !linkedLoaded {
				linked, err = s.q.ListPortalCustomerOrgIDs(ctx, db.ListPortalCustomerOrgIDsParams{
					UserID: userID, BrandID: brandID,
				})
				linkedLoaded = err == nil
			}
			orgs = linked
		}
		if err != nil {
			return nil, fmt.Errorf("features: portal %s organizations: %w", key, err)
		}
		on, err = s.anyEnabled(ctx, orgs, key)
		if err != nil {
			return nil, err
		}
		if on {
			out = append(out, key)
		}
	}
	return out, nil
}

// anyEnabled reports whether key is on for at least one organization.
func (s *Service) anyEnabled(ctx context.Context, orgIDs []int64, key string) (bool, error) {
	for _, id := range orgIDs {
		on, err := s.Enabled(ctx, id, key)
		if err != nil {
			return false, fmt.Errorf("features: portal %s: %w", key, err)
		}
		if on {
			return true, nil
		}
	}
	return false, nil
}
