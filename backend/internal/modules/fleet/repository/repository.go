// Package repository wraps the fleet persistence (TEC-472, F5-02a): the
// dealer fleet and fleet vehicle list contracts, the open-link uniqueness
// and the link status compare-and-set.
package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	// ErrLinkExists: the (fleet, dealer) pair already has a pending or
	// active link (uq_fleet_dealer_links_open).
	ErrLinkExists = errors.New("fleet: an open link to this dealer already exists")
	// ErrFleetExists: the brand already has a fleet with this tax number
	// (uq_fleet_profiles_brand_tax_number).
	ErrFleetExists = errors.New("fleet: a fleet with this tax number already exists")
	// ErrLinkStale: the link is no longer in the expected status (a
	// concurrent change won the compare-and-set) or does not exist.
	ErrLinkStale = errors.New("fleet: link status changed")
)

// FleetSort is the sort contract of the dealer fleet list.
var FleetSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"name": "name", "vehicle_count": "vehicle_count",
		"last_service_at": "last_service_at", "created_at": "created_at",
	},
	Default: apiquery.SortField{Field: "name"},
}

// VehicleSort is the sort contract of the fleet vehicle list.
var VehicleSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"plate": "plate", "car_brand": "car_brand", "last_service_at": "last_service_at",
		"active_warranty_count": "active_warranty_count", "created_at": "created_at",
	},
	Default: apiquery.SortField{Field: "plate"},
}

// Store is the fleet repository.
type Store struct {
	q *db.Queries
}

// New creates a repository on a pool or a transaction.
func New(conn db.DBTX) *Store { return &Store{q: db.New(conn)} }

// Queries returns the generated query set.
func (s *Store) Queries() *db.Queries { return s.q }

// FleetFilter narrows the dealer fleet list. DealerOrgIDs nil: every dealer
// of the brand (brand / all scope); empty: nothing.
type FleetFilter struct {
	BrandID         int64
	DealerOrgIDs    []int64
	Statuses        []string
	Q               string
	VehicleCountMin *int64
	VehicleCountMax *int64
	Sort            []apiquery.SortField
	Limit, Offset   int32
}

// ListDealerFleets returns a page of fleet links and the total. An unknown
// sort field is an *apiquery.ValidationError.
func (s *Store) ListDealerFleets(ctx context.Context, f FleetFilter) ([]db.ListDealerFleetsRow, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, FleetSort)
	if err != nil {
		return nil, 0, err
	}
	if f.DealerOrgIDs != nil && len(f.DealerOrgIDs) == 0 {
		return []db.ListDealerFleetsRow{}, 0, nil
	}
	q := textNarg(f.Q)
	minN, maxN := int8Narg(f.VehicleCountMin), int8Narg(f.VehicleCountMax)
	rows, err := s.q.ListDealerFleets(ctx, db.ListDealerFleetsParams{
		BrandID: f.BrandID, DealerOrgIds: f.DealerOrgIDs, Statuses: f.Statuses, Q: q,
		VehicleCountMin: minN, VehicleCountMax: maxN,
		SortKey: sort.Key, SortDesc: sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountDealerFleets(ctx, db.CountDealerFleetsParams{
		BrandID: f.BrandID, DealerOrgIds: f.DealerOrgIDs, Statuses: f.Statuses, Q: q,
		VehicleCountMin: minN, VehicleCountMax: maxN,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// VehicleFilter narrows the fleet vehicle list. ServiceOrgIDs limits the
// last service and active warranty columns to those organizations (nil:
// every organization).
type VehicleFilter struct {
	FleetOrgID    int64
	ServiceOrgIDs []int64
	Q             string
	Sort          []apiquery.SortField
	Limit, Offset int32
}

// ListFleetVehicles returns a page of fleet vehicles and the total.
func (s *Store) ListFleetVehicles(ctx context.Context, f VehicleFilter) ([]db.ListFleetVehiclesRow, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, VehicleSort)
	if err != nil {
		return nil, 0, err
	}
	q := textNarg(f.Q)
	var qPlate pgtype.Text
	if q.Valid {
		if p := geo.NormalizePlate(q.String); p != "" {
			qPlate = pgtype.Text{String: p, Valid: true}
		}
	}
	rows, err := s.q.ListFleetVehicles(ctx, db.ListFleetVehiclesParams{
		FleetOrgID: f.FleetOrgID, ServiceOrgIds: f.ServiceOrgIDs, Q: q, QPlate: qPlate,
		SortKey: sort.Key, SortDesc: sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountFleetVehicles(ctx, db.CountFleetVehiclesParams{
		FleetOrgID: f.FleetOrgID, Q: q, QPlate: qPlate,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// FindByTaxNumber returns the fleet of the brand with this exact tax
// number (pgx.ErrNoRows when none).
func (s *Store) FindByTaxNumber(ctx context.Context, brandID int64, taxNumber string) (db.FindFleetByTaxNumberRow, error) {
	return s.q.FindFleetByTaxNumber(ctx, db.FindFleetByTaxNumberParams{BrandID: brandID, TaxNumber: taxNumber})
}

// CreateProfile inserts the profile of a fleet organization; a duplicate
// tax number of the brand is ErrFleetExists.
func (s *Store) CreateProfile(ctx context.Context, arg db.CreateFleetProfileParams) (db.FleetProfile, error) {
	p, err := s.q.CreateFleetProfile(ctx, arg)
	if isUnique(err, "uq_fleet_profiles_brand_tax_number") {
		return db.FleetProfile{}, ErrFleetExists
	}
	return p, err
}

// CreateLink inserts a fleet-dealer link; a second open (pending or active)
// link of the same pair is ErrLinkExists.
func (s *Store) CreateLink(ctx context.Context, arg db.CreateFleetDealerLinkParams) (db.FleetDealerLink, error) {
	l, err := s.q.CreateFleetDealerLink(ctx, arg)
	if isUnique(err, "uq_fleet_dealer_links_open") {
		return db.FleetDealerLink{}, ErrLinkExists
	}
	return l, err
}

// TransitionLink moves a link from one status to another with a
// compare-and-set; ErrLinkStale when the link is not in from (anymore)
// or already ended (ended is final).
func (s *Store) TransitionLink(ctx context.Context, id uuid.UUID, from, to string) (db.FleetDealerLink, error) {
	l, err := s.q.TransitionFleetDealerLink(ctx, db.TransitionFleetDealerLinkParams{Uuid: id, FromStatus: from, ToStatus: to})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.FleetDealerLink{}, ErrLinkStale
	}
	return l, err
}

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func textNarg(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

func int8Narg(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
