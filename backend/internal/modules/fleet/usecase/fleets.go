package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// FleetExistsError is 409 FLEET_ALREADY_EXISTS on POST /v1/fleets: the
// brand already has a fleet with this tax number. The answer names it so
// the dealer requests a link (POST /v1/fleets/{uuid}/links) instead.
type FleetExistsError struct {
	FleetUUID uuid.UUID `json:"fleet_uuid"`
	Name      string    `json:"name"`
	LegalName string    `json:"legal_name"`
	// LinkStatus is the caller's open link to the fleet (pending | active),
	// empty when there is none.
	LinkStatus string `json:"link_status"`
}

func (e *FleetExistsError) Error() string {
	return "fleet: a fleet with this tax number already exists"
}

// Unwrap keeps errors.Is(err, ErrFleetExists).
func (e *FleetExistsError) Unwrap() error { return ErrFleetExists }

// OpenInput is POST /v1/fleets.
type OpenInput struct {
	Name            string `json:"name"`
	LegalName       string `json:"legal_name"`
	TaxNumber       string `json:"tax_number"`
	TaxOffice       string `json:"tax_office"`
	ContactName     string `json:"contact_name"`
	ContactPhone    string `json:"contact_phone"`
	BillingEmail    string `json:"billing_email"`
	ReportFrequency string `json:"report_frequency"`
	ReportLocale    string `json:"report_locale"`
}

// Open opens a fleet for the caller's dealer (or serving distributor): the
// tax number is looked up first; a new one opens the fleet organization,
// its profile, the opener's active link and the fleet cari in the opener's
// ledger. An existing one is a *FleetExistsError (409, link request flow).
func (s *Service) Open(ctx context.Context, c Caller, in OpenInput) (FleetView, error) {
	opener, err := db.New(s.conn).GetOrganizationByID(ctx, c.OrgID)
	if err != nil {
		return FleetView{}, fmt.Errorf("fleet: opener: %w", err)
	}
	if opener.Type != "dealer" && opener.Type != "distributor" {
		return FleetView{}, ErrForbidden
	}
	f, err := s.CreateFleet(ctx, CreateFleetInput{
		Opener: opener, ActorUserID: c.UserID, Name: in.Name, LegalName: in.LegalName,
		TaxNumber: in.TaxNumber, TaxOffice: in.TaxOffice, ContactName: in.ContactName,
		ContactPhone: in.ContactPhone, BillingEmail: in.BillingEmail,
		ReportFrequency: in.ReportFrequency, ReportLocale: in.ReportLocale,
	})
	if errors.Is(err, ErrFleetExists) {
		return FleetView{}, s.existsError(ctx, opener, strings.TrimSpace(in.TaxNumber))
	}
	if errors.Is(err, ErrNotLinkable) {
		return FleetView{}, ErrForbidden
	}
	if err != nil {
		return FleetView{}, err
	}
	lv := linkView(f.Link, opener)
	return fleetView(f.Organization, f.Profile, &lv), nil
}

func (s *Service) existsError(ctx context.Context, opener db.Organization, tax string) error {
	repo := repository.New(s.conn)
	found, err := repo.FindByTaxNumber(ctx, opener.BrandID, tax)
	if err != nil {
		return ErrFleetExists
	}
	out := &FleetExistsError{
		FleetUUID: found.Organization.Uuid, Name: found.Organization.Name, LegalName: found.FleetProfile.LegalName,
	}
	if l, err := repo.Queries().GetOpenFleetDealerLink(ctx, db.GetOpenFleetDealerLinkParams{
		FleetOrgID: found.Organization.ID, DealerOrgID: opener.ID,
	}); err == nil {
		out.LinkStatus = l.Status
	}
	return out
}

// RequestLink is POST /v1/fleets/{uuid}/links: the caller's dealer (or
// serving distributor) asks to serve an existing fleet of the brand. The
// link starts pending; the fleet users are notified and decide in the
// portal. A second open link of the pair is ErrLinkExists (409).
func (s *Service) RequestLink(ctx context.Context, c Caller, fleetUUID uuid.UUID) (FleetView, error) {
	var out FleetView
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		dealer, err := q.GetOrganizationByID(ctx, c.OrgID)
		if err != nil {
			return fmt.Errorf("fleet: dealer: %w", err)
		}
		if dealer.Type != "dealer" && dealer.Type != "distributor" {
			return ErrForbidden
		}
		f, err := q.GetFleetByUUID(ctx, fleetUUID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && f.Organization.BrandID != dealer.BrandID) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("fleet: get: %w", err)
		}
		link, err := repository.New(tx).CreateLink(ctx, db.CreateFleetDealerLinkParams{
			FleetOrgID: f.Organization.ID, DealerOrgID: dealer.ID, BrandID: dealer.BrandID,
			Status: model.LinkPending, CreatedByOrgID: dealer.ID,
			CreatedByUserID: pgtype.Int8{Int64: c.UserID, Valid: c.UserID != 0},
		})
		if errors.Is(err, repository.ErrLinkExists) {
			return ErrLinkExists
		}
		if err != nil {
			return err
		}
		notify, err := q.ListActiveFleetUserIDs(ctx, f.Organization.ID)
		if err != nil {
			return fmt.Errorf("fleet: users: %w", err)
		}
		if err := s.emit(ctx, tx, events.FleetLinkRequested, dealer.ID, c.UserID, f.Organization, map[string]any{
			"link_uuid": link.Uuid.String(), "dealer_org_id": dealer.ID, "dealer_name": dealer.Name,
			"notify_user_ids": notify,
		}); err != nil {
			return err
		}
		lv := linkView(link, dealer)
		out = fleetView(f.Organization, f.FleetProfile, &lv)
		return nil
	})
	return out, err
}

// ListFilter narrows GET /v1/fleets (docs/list-contract.md).
type ListFilter = repository.FleetFilter

// List is GET /v1/fleets: the fleets linked to the organizations in scope,
// one row per link (pending links included; the card needs an active one).
func (s *Service) List(ctx context.Context, c Caller, f ListFilter) ([]FleetListItem, int64, error) {
	if c.BrandID == 0 {
		return nil, 0, ErrForbidden
	}
	f.BrandID = c.BrandID
	f.DealerOrgIDs = c.orgIDs()
	rows, total, err := repository.New(s.conn).ListDealerFleets(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]FleetListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, FleetListItem{
			UUID: r.FleetUuid, Name: r.Name, LegalName: r.LegalName, TaxNumber: r.TaxNumber, Status: r.FleetStatus,
			VehicleCount: r.VehicleCount, LastServiceAt: timePtr(r.LastServiceAt),
			Link: LinkView{
				UUID: r.LinkUuid, Status: r.LinkStatus, DealerUUID: r.DealerUuid, DealerName: r.DealerName,
				StartedAt: timePtr(r.StartedAt), EndedAt: timePtr(r.EndedAt), CreatedAt: r.CreatedAt.Time,
			},
		})
	}
	return out, total, nil
}

// --- Portal: the fleet decides the dealer links -------------------------------

// portalFleet is the fleet of a signed-in fleet user (ErrNotFound: none).
func portalFleet(ctx context.Context, q *db.Queries, userID int64) (db.GetFleetUserByUserIDRow, error) {
	fu, err := q.GetFleetUserByUserID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.GetFleetUserByUserIDRow{}, ErrNotFound
	}
	if err != nil {
		return db.GetFleetUserByUserIDRow{}, fmt.Errorf("fleet: portal user: %w", err)
	}
	return fu, nil
}

// PortalLinks is GET /v1/portal/fleet/links: the dealer links of the
// signed-in user's fleet (pending ones wait for the decision).
func (s *Service) PortalLinks(ctx context.Context, userID int64) ([]LinkView, error) {
	q := db.New(s.conn)
	fu, err := portalFleet(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFleetDealerLinks(ctx, db.ListFleetDealerLinksParams{FleetOrgID: fu.FleetOrgID})
	if err != nil {
		return nil, fmt.Errorf("fleet: links: %w", err)
	}
	out := make([]LinkView, 0, len(rows))
	for _, r := range rows {
		l := r.FleetDealerLink
		out = append(out, LinkView{
			UUID: l.Uuid, Status: l.Status, DealerUUID: r.DealerUuid, DealerName: r.DealerName,
			StartedAt: timePtr(l.StartedAt), EndedAt: timePtr(l.EndedAt), CreatedAt: l.CreatedAt.Time,
		})
	}
	return out, nil
}

// DecideLink is POST /v1/portal/fleet/links/{uuid}/accept|reject: a fleet
// user decides a pending link of their fleet. Accepting activates it,
// opens the fleet cari in the dealer's ledger and links the vehicle owner
// to the dealer; the requesting user is notified either way.
func (s *Service) DecideLink(ctx context.Context, userID int64, linkUUID uuid.UUID, accept bool) (LinkView, error) {
	var out LinkView
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		fu, err := portalFleet(ctx, q, userID)
		if err != nil {
			return err
		}
		link, err := q.GetFleetDealerLinkByUUID(ctx, linkUUID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && link.FleetOrgID != fu.FleetOrgID) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("fleet: link: %w", err)
		}
		to, name := model.LinkActive, events.FleetLinked
		if !accept {
			to, name = model.LinkEnded, events.FleetLinkRejected
		}
		link, err = repository.New(tx).TransitionLink(ctx, linkUUID, model.LinkPending, to)
		if errors.Is(err, repository.ErrLinkStale) {
			return ErrLinkNotPending
		}
		if err != nil {
			return err
		}
		dealer, err := q.GetOrganizationByID(ctx, link.DealerOrgID)
		if err != nil {
			return fmt.Errorf("fleet: dealer: %w", err)
		}
		f, err := q.GetOrganizationByID(ctx, fu.FleetOrgID)
		if err != nil {
			return fmt.Errorf("fleet: org: %w", err)
		}
		if accept {
			cari, err := repository.EnsureFleetCari(ctx, q, dealer, f.ID)
			if err != nil {
				return err
			}
			link.CariAccountID = pgtype.Int8{Int64: cari.ID, Valid: true}
			profile, err := q.GetFleetProfileByOrg(ctx, f.ID)
			if err != nil {
				return fmt.Errorf("fleet: profile: %w", err)
			}
			if err := repository.LinkPrimaryUser(ctx, q, profile.PrimaryUserID.Int64, dealer.ID, f.BrandID); err != nil {
				return err
			}
		}
		notify := []int64{}
		if link.CreatedByUserID.Valid {
			notify = append(notify, link.CreatedByUserID.Int64)
		}
		if err := s.emit(ctx, tx, name, f.ID, userID, f, map[string]any{
			"link_uuid": link.Uuid.String(), "dealer_org_id": dealer.ID, "dealer_name": dealer.Name,
			"notify_user_ids": notify,
		}); err != nil {
			return err
		}
		out = linkView(link, dealer)
		return nil
	})
	return out, err
}
