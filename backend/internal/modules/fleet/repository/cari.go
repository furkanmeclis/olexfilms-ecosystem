package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// EnsureFleetCari returns the fleet's cari in the dealer's ledger, opening
// it when missing (counterparty_type organization, the dealer's currency),
// and records it on the dealer's open link while the link has none. Every
// service of a fleet vehicle at this dealer books on this one cari (TEC-473).
// q must run in the caller's transaction.
func EnsureFleetCari(ctx context.Context, q *db.Queries, dealer db.Organization, fleetOrgID int64) (db.CariAccount, error) {
	counterparty := pgtype.Int8{Int64: fleetOrgID, Valid: true}
	c, err := q.CreateCariForOrgIfMissing(ctx, db.CreateCariForOrgIfMissingParams{
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, CounterpartyOrgID: counterparty, Currency: dealer.Currency,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		c, err = q.GetCariAccountByCounterpartyOrg(ctx, db.GetCariAccountByCounterpartyOrgParams{
			OrganizationID: dealer.ID, CounterpartyOrgID: counterparty,
		})
	}
	if err != nil {
		return db.CariAccount{}, fmt.Errorf("fleet: cari: %w", err)
	}
	link, err := q.GetOpenFleetDealerLink(ctx, db.GetOpenFleetDealerLinkParams{FleetOrgID: fleetOrgID, DealerOrgID: dealer.ID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return c, nil
	case err != nil:
		return db.CariAccount{}, fmt.Errorf("fleet: open link: %w", err)
	}
	if !link.CariAccountID.Valid {
		// CAS on NULL: a concurrent writer set the same cari (one per pair).
		if _, err := q.SetFleetDealerLinkCari(ctx, db.SetFleetDealerLinkCariParams{
			ID: link.ID, CariAccountID: pgtype.Int8{Int64: c.ID, Valid: true},
		}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return db.CariAccount{}, fmt.Errorf("fleet: link cari: %w", err)
		}
	}
	return c, nil
}

// LinkPrimaryUser links the fleet's primary user (the owner of the fleet
// vehicles) to the dealer as a served customer, so the dealer's customer,
// vehicle and service flows reach the fleet vehicles (customer_organizations).
func LinkPrimaryUser(ctx context.Context, q *db.Queries, primaryUserID, dealerOrgID, brandID int64) error {
	if primaryUserID == 0 {
		return nil
	}
	if _, err := q.LinkCustomerOrganization(ctx, db.LinkCustomerOrganizationParams{
		UserID: primaryUserID, OrganizationID: dealerOrgID, BrandID: brandID,
	}); err != nil {
		return fmt.Errorf("fleet: customer link: %w", err)
	}
	return nil
}
