package usecase

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-769 (Depo F4.4): Glorian remote location -> local warehouse / bin.
//
// The legacy ConnectionLocationMap names a remote location (Glorian's
// location label) and the local warehouse plus an optional bin it stands
// for. Today's Inventory API contract only knows the hub locations
// "center" and "dealer" (pull_stock mirrors the remote status without
// touching the ledger, outbound and push always send location=center), so
// no sync path reads the map yet; it is kept and edited here so a pull
// that places units can resolve a remote location once the contract
// carries one.

// ErrRemoteTaken: the remote location already has a map row (409).
var ErrRemoteTaken = errors.New("glorian admin: remote location already mapped")

const maxRemoteLocation = 64

// LocationMapsSortSpec is the sort whitelist of GET .../location-maps.
var LocationMapsSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"remote_location_code": "remote_location_code", "warehouse": "warehouse", "created_at": "created_at",
	},
	Default: apiquery.SortField{Field: "remote_location_code"},
}

// MapWarehouse names the warehouse of a map row or a picker option.
type MapWarehouse struct {
	UUID   uuid.UUID `json:"uuid"`
	Code   string    `json:"code"`
	Name   string    `json:"name"`
	Active bool      `json:"active"`
}

// MapLocation names the bin of a map row or a picker option.
type MapLocation struct {
	UUID     uuid.UUID `json:"uuid"`
	Code     string    `json:"code"`
	FullCode *string   `json:"full_code"`
	Name     string    `json:"name,omitempty"`
}

// LocationMapView is one map row.
type LocationMapView struct {
	UUID               uuid.UUID    `json:"uuid"`
	RemoteLocationCode string       `json:"remote_location_code"`
	Warehouse          MapWarehouse `json:"warehouse"`
	Location           *MapLocation `json:"location"`
	CreatedAt          *time.Time   `json:"created_at"`
	UpdatedAt          *time.Time   `json:"updated_at"`
}

// LocationMapInput creates or replaces a map row.
type LocationMapInput struct {
	RemoteLocationCode string     `json:"remote_location_code"`
	WarehouseUUID      string     `json:"warehouse_uuid"`
	LocationUUID       *uuid.UUID `json:"location_uuid"`
}

// LocationMapFilter is the list query.
type LocationMapFilter struct {
	Q        string
	SortKey  string
	SortDesc bool
	Limit    int
	Offset   int
}

func (s *Service) resolveMapTarget(ctx context.Context, conn db.IntegrationConnection, in LocationMapInput) (string, db.Warehouse, pgtype.Int8, error) {
	remote := strings.TrimSpace(in.RemoteLocationCode)
	if remote == "" {
		return "", db.Warehouse{}, pgtype.Int8{}, invalid("remote_location_code", "is required")
	}
	if utf8.RuneCountInString(remote) > maxRemoteLocation {
		return "", db.Warehouse{}, pgtype.Int8{}, invalid("remote_location_code", "must be at most 64 characters")
	}
	wid, err := uuid.Parse(strings.TrimSpace(in.WarehouseUUID))
	if err != nil {
		return "", db.Warehouse{}, pgtype.Int8{}, invalid("warehouse_uuid", "must be a uuid")
	}
	w, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: wid, OrganizationID: conn.OrganizationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", db.Warehouse{}, pgtype.Int8{}, invalid("warehouse_uuid", "is not a warehouse of the connection's organization")
	}
	if err != nil {
		return "", db.Warehouse{}, pgtype.Int8{}, err
	}
	var loc pgtype.Int8
	if in.LocationUUID != nil {
		l, err := s.q.GetWarehouseLocationByUUID(ctx, *in.LocationUUID)
		if err != nil || l.OrganizationID != conn.OrganizationID || !l.WarehouseID.Valid || l.WarehouseID.Int64 != w.ID {
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return "", db.Warehouse{}, pgtype.Int8{}, err
			}
			return "", db.Warehouse{}, pgtype.Int8{}, invalid("location_uuid", "is not a location of the warehouse")
		}
		loc = pgtype.Int8{Int64: l.ID, Valid: true}
	}
	return remote, w, loc, nil
}

func mapDBErr(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		if pg.ConstraintName == "uq_connection_location_maps_location" {
			return invalid("location_uuid", "is already mapped to another remote location")
		}
		return ErrRemoteTaken
	}
	return err
}

func (s *Service) mapView(ctx context.Context, m db.ConnectionLocationMap) (LocationMapView, error) {
	w, err := s.q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: m.WarehouseID, OrganizationID: m.OrganizationID})
	if err != nil {
		return LocationMapView{}, err
	}
	v := LocationMapView{
		UUID: m.Uuid, RemoteLocationCode: m.RemoteLocationCode,
		Warehouse: MapWarehouse{UUID: w.Uuid, Code: w.Code, Name: w.Name, Active: w.Active},
		CreatedAt: ts(m.CreatedAt), UpdatedAt: ts(m.UpdatedAt),
	}
	if m.WarehouseLocationID.Valid {
		l, err := s.q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: m.WarehouseLocationID.Int64, OrganizationID: m.OrganizationID})
		if err != nil {
			return LocationMapView{}, err
		}
		v.Location = &MapLocation{UUID: l.Uuid, Code: l.Code, FullCode: text(l.FullCode)}
	}
	return v, nil
}

// ListLocationMaps lists the map rows of the brand's connection.
func (s *Service) ListLocationMaps(ctx context.Context, brand db.Brand, f LocationMapFilter) ([]LocationMapView, int64, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return nil, 0, err
	}
	q := optText(strings.TrimSpace(f.Q))
	rows, err := s.q.ListConnectionLocationMaps(ctx, db.ListConnectionLocationMapsParams{
		ConnectionID: conn.ID, Q: q, SortKey: f.SortKey, SortDesc: f.SortDesc,
		PageLimit: int32(ClampLimit(f.Limit)), PageOffset: int32(max(f.Offset, 0)),
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountConnectionLocationMaps(ctx, db.CountConnectionLocationMapsParams{ConnectionID: conn.ID, Q: q})
	if err != nil {
		return nil, 0, err
	}
	out := make([]LocationMapView, 0, len(rows))
	for _, r := range rows {
		v := LocationMapView{
			UUID: r.Uuid, RemoteLocationCode: r.RemoteLocationCode,
			Warehouse: MapWarehouse{UUID: r.WarehouseUuid, Code: r.WarehouseCode, Name: r.WarehouseName, Active: true},
			CreatedAt: ts(r.CreatedAt), UpdatedAt: ts(r.UpdatedAt),
		}
		if r.LocationUuid.Valid {
			v.Location = &MapLocation{UUID: r.LocationUuid.Bytes, Code: r.LocationCode.String, FullCode: text(r.LocationFullCode)}
		}
		out = append(out, v)
	}
	return out, total, nil
}

// CreateLocationMap adds a map row.
func (s *Service) CreateLocationMap(ctx context.Context, brand db.Brand, in LocationMapInput) (LocationMapView, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return LocationMapView{}, err
	}
	remote, w, loc, err := s.resolveMapTarget(ctx, conn, in)
	if err != nil {
		return LocationMapView{}, err
	}
	m, err := s.q.CreateConnectionLocationMap(ctx, db.CreateConnectionLocationMapParams{
		OrganizationID: conn.OrganizationID, BrandID: conn.BrandID, ConnectionID: conn.ID,
		WarehouseID: w.ID, WarehouseLocationID: loc, RemoteLocationCode: remote,
	})
	if err != nil {
		return LocationMapView{}, mapDBErr(err)
	}
	return s.mapView(ctx, m)
}

// UpdateLocationMap replaces a map row.
func (s *Service) UpdateLocationMap(ctx context.Context, brand db.Brand, id uuid.UUID, in LocationMapInput) (LocationMapView, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return LocationMapView{}, err
	}
	remote, w, loc, err := s.resolveMapTarget(ctx, conn, in)
	if err != nil {
		return LocationMapView{}, err
	}
	m, err := s.q.UpdateConnectionLocationMap(ctx, db.UpdateConnectionLocationMapParams{
		ConnectionID: conn.ID, Uuid: id, WarehouseID: w.ID, WarehouseLocationID: loc, RemoteLocationCode: remote,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return LocationMapView{}, ErrNotFound
	}
	if err != nil {
		return LocationMapView{}, mapDBErr(err)
	}
	return s.mapView(ctx, m)
}

// DeleteLocationMap removes a map row.
func (s *Service) DeleteLocationMap(ctx context.Context, brand db.Brand, id uuid.UUID) error {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteConnectionLocationMap(ctx, db.DeleteConnectionLocationMapParams{ConnectionID: conn.ID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveRemoteLocation returns the warehouse and optional bin a remote
// location maps to (ok false when unmapped). Sync code calls it once the
// contract carries remote locations.
func (s *Service) ResolveRemoteLocation(ctx context.Context, connectionID int64, remote string) (warehouseID int64, locationID *int64, ok bool, err error) {
	m, err := s.q.GetConnectionLocationMapByRemote(ctx, db.GetConnectionLocationMapByRemoteParams{
		ConnectionID: connectionID, RemoteLocationCode: strings.TrimSpace(remote),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, err
	}
	if m.WarehouseLocationID.Valid {
		id := m.WarehouseLocationID.Int64
		locationID = &id
	}
	return m.WarehouseID, locationID, true, nil
}

// MapWarehouses lists the warehouses of the connection's organization
// (the picker of the map form).
func (s *Service) MapWarehouses(ctx context.Context, brand db.Brand) ([]MapWarehouse, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListWarehouses(ctx, db.ListWarehousesParams{OrganizationID: conn.OrganizationID})
	if err != nil {
		return nil, err
	}
	out := make([]MapWarehouse, 0, len(rows))
	for _, w := range rows {
		out = append(out, MapWarehouse{UUID: w.Uuid, Code: w.Code, Name: w.Name, Active: w.Active})
	}
	return out, nil
}

// MapLocations searches the active locations of one warehouse of the
// connection's organization (at most 50).
func (s *Service) MapLocations(ctx context.Context, brand db.Brand, warehouseUUID uuid.UUID, q string) ([]MapLocation, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return nil, err
	}
	w, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: warehouseUUID, OrganizationID: conn.OrganizationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.q.SearchWarehouseLocationsForMap(ctx, db.SearchWarehouseLocationsForMapParams{
		OrganizationID: conn.OrganizationID, WarehouseID: pgtype.Int8{Int64: w.ID, Valid: true},
		Q: optText(strings.TrimSpace(q)), RowLimit: 50,
	})
	if err != nil {
		return nil, err
	}
	out := make([]MapLocation, 0, len(rows))
	for _, l := range rows {
		out = append(out, MapLocation{UUID: l.Uuid, Code: l.Code, FullCode: text(l.FullCode), Name: l.Name})
	}
	return out, nil
}
