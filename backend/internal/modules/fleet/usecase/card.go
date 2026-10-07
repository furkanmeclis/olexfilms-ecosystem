package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// recentServiceLimit is the number of services on the fleet card.
const recentServiceLimit = 10

// ServiceSummary is a service on the fleet card.
type ServiceSummary struct {
	UUID             uuid.UUID  `json:"uuid"`
	ServiceNo        string     `json:"service_no"`
	Status           string     `json:"status"`
	Plate            *string    `json:"plate"`
	VehicleUUID      uuid.UUID  `json:"vehicle_uuid"`
	OrganizationUUID uuid.UUID  `json:"organization_uuid"`
	OrganizationName string     `json:"organization_name"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at"`
}

// CariView is the fleet cari in the caller organization's ledger.
type CariView struct {
	UUID     uuid.UUID `json:"uuid"`
	Currency string    `json:"currency"`
	// Balance > 0: the fleet owes the organization.
	Balance string `json:"balance"`
}

// CardView is GET /v1/fleets/{uuid}: the profile, vehicle count, active
// warranties, recent services and the caller's cari balance. Services and
// warranties count only the caller's own work (a dealer never sees another
// dealer's services or cari); links are the active links in scope.
type CardView struct {
	UUID                uuid.UUID        `json:"uuid"`
	Name                string           `json:"name"`
	Status              string           `json:"status"`
	Profile             ProfileView      `json:"profile"`
	HasPrimaryUser      bool             `json:"has_primary_user"`
	VehicleCount        int64            `json:"vehicle_count"`
	ActiveWarrantyCount int64            `json:"active_warranty_count"`
	ServiceCount        int64            `json:"service_count"`
	RecentServices      []ServiceSummary `json:"recent_services"`
	Links               []LinkView       `json:"links"`
	Cari                *CariView        `json:"cari"`
	CreatedAt           time.Time        `json:"created_at"`
}

// Card is GET /v1/fleets/{uuid}.
func (s *Service) Card(ctx context.Context, c Caller, fleetUUID uuid.UUID) (CardView, error) {
	q := db.New(s.conn)
	a, err := s.resolve(ctx, q, c, fleetUUID)
	if err != nil {
		return CardView{}, err
	}
	fleet := a.fleet.Organization
	orgs := c.orgIDs()
	out := CardView{
		UUID: fleet.Uuid, Name: fleet.Name, Status: fleet.Status, Profile: profileView(a.fleet.FleetProfile),
		HasPrimaryUser: a.fleet.FleetProfile.PrimaryUserID.Valid, CreatedAt: fleet.CreatedAt.Time,
		RecentServices: []ServiceSummary{}, Links: []LinkView{},
	}
	if out.VehicleCount, err = q.CountFleetVehicles(ctx, db.CountFleetVehiclesParams{FleetOrgID: fleet.ID}); err != nil {
		return CardView{}, fmt.Errorf("fleet: vehicles: %w", err)
	}
	if out.ActiveWarrantyCount, err = q.CountFleetActiveWarranties(ctx, db.CountFleetActiveWarrantiesParams{
		FleetOrgID: fleet.ID, ServiceOrgIds: orgs,
	}); err != nil {
		return CardView{}, fmt.Errorf("fleet: warranties: %w", err)
	}
	if out.ServiceCount, err = q.CountFleetServices(ctx, db.CountFleetServicesParams{
		FleetOrgID: fleet.ID, ServiceOrgIds: orgs,
	}); err != nil {
		return CardView{}, fmt.Errorf("fleet: services: %w", err)
	}
	rows, err := q.ListFleetRecentServices(ctx, db.ListFleetRecentServicesParams{
		FleetOrgID: fleet.ID, ServiceOrgIds: orgs, LimitCount: recentServiceLimit,
	})
	if err != nil {
		return CardView{}, fmt.Errorf("fleet: recent services: %w", err)
	}
	for _, r := range rows {
		out.RecentServices = append(out.RecentServices, ServiceSummary{
			UUID: r.Uuid, ServiceNo: r.ServiceNo, Status: r.Status, Plate: textPtr(r.Plate),
			VehicleUUID: r.VehicleUuid, OrganizationUUID: r.OrganizationUuid, OrganizationName: r.OrganizationName,
			CreatedAt: r.CreatedAt.Time, CompletedAt: timePtr(r.CompletedAt),
		})
	}
	for _, l := range a.links {
		dealer, err := q.GetOrganizationByID(ctx, l.DealerOrgID)
		if err != nil {
			return CardView{}, fmt.Errorf("fleet: dealer: %w", err)
		}
		out.Links = append(out.Links, linkView(l, dealer))
	}
	if a.own != nil && a.own.CariAccountID.Valid {
		cv, err := cariView(ctx, q, a.own.CariAccountID.Int64, c.OrgID)
		if err != nil {
			return CardView{}, err
		}
		out.Cari = &cv
	}
	return out, nil
}

func cariView(ctx context.Context, q *db.Queries, cariID, orgID int64) (CariView, error) {
	cari, err := q.GetCariAccount(ctx, db.GetCariAccountParams{ID: cariID, OrganizationID: orgID})
	if err != nil {
		return CariView{}, fmt.Errorf("fleet: cari: %w", err)
	}
	out := CariView{UUID: cari.Uuid, Currency: cari.Currency, Balance: "0.00"}
	bal, err := q.GetCariBalance(ctx, db.GetCariBalanceParams{CariID: cariID, OrganizationID: orgID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return CariView{}, fmt.Errorf("fleet: cari balance: %w", err)
	default:
		out.Balance = money(bal.Balance)
	}
	return out, nil
}

func money(n pgtype.Numeric) string {
	if !n.Valid {
		return "0.00"
	}
	return posting.FormatNumeric(n)
}
