package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// WarehousesStep imports the legacy warehouse structure into the Olex center
// (TEC-257): warehouses -> warehouses, warehouse_sites -> rooms and
// warehouse_locations -> typed warehouse_locations (aisle / shelf / bin).
//
// The warehouse side is brand-independent (K20), so every legacy warehouse
// lands in the center organization. Codes are normalised to the new code
// pattern (upper case, [A-Z0-9_]); a legacy row whose normalised code or
// full code already exists in the center is linked to that row instead of
// duplicated. The new tree allows a bin only under a shelf: a legacy bin
// directly under an aisle becomes a shelf (same code and full code, so stock
// can still be placed there) and is reported.
//
// The step always reads every legacy row (a few dozen); reruns are
// idempotent through migration_map checksums.
type WarehousesStep struct {
	// WHSystem is the migration_map source system of the warehouse rows;
	// empty means SourceWH.
	WHSystem string
}

// Name implements Step.
func (WarehousesStep) Name() string { return "warehouses" }

func (s WarehousesStep) whSystem() string {
	if s.WHSystem == "" {
		return SourceWH
	}
	return s.WHSystem
}

const whWarehousesQuery = `SELECT CAST(id AS CHAR(36)), code, name, COALESCE(country, ''), COALESCE(city, ''),
	COALESCE(district, ''), is_active, sort_order, deleted_at, created_at, updated_at
FROM warehouses ORDER BY sort_order, code`

const whSitesQuery = `SELECT CAST(id AS CHAR(36)), CAST(warehouse_id AS CHAR(36)), code, name, is_active, sort_order,
	deleted_at, created_at, updated_at
FROM warehouse_sites ORDER BY sort_order, code`

const whLocationsQuery = `SELECT CAST(id AS CHAR(36)), CAST(warehouse_site_id AS CHAR(36)), CAST(parent_id AS CHAR(36)),
	type, code, COALESCE(name, ''), sort_order, is_active, deleted_at, created_at, updated_at
FROM warehouse_locations ORDER BY sort_order, code, id`

// Column limits of warehouses / rooms / warehouse_locations (000059).
const (
	whCodeMax = 32
	whNameMax = 200
)

// Location types of the new tree (000059).
const (
	locAisle = "aisle"
	locShelf = "shelf"
	locBin   = "bin"
)

type legacyWarehouse struct {
	ID, Code, Name, Country, City, District string
	Active                                  bool
	SortOrder                               int32
	DeletedAt, CreatedAt, UpdatedAt         sql.NullTime
}

type legacySite struct {
	ID, WarehouseID, Code, Name     string
	Active                          bool
	SortOrder                       int32
	DeletedAt, CreatedAt, UpdatedAt sql.NullTime
}

type legacyLocation struct {
	ID, SiteID                      string
	ParentID                        sql.NullString
	Type, Code, Name                string
	SortOrder                       int32
	Active                          bool
	DeletedAt, CreatedAt, UpdatedAt sql.NullTime
}

// Run implements Step.
func (s WarehousesStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	wh, err := src.Get(SourceWH)
	if err != nil {
		return StepResult{}, err
	}
	centerID, err := olexCenterID(ctx, dst.Q)
	if err != nil {
		return StepResult{}, err
	}
	warehouses, err := readWarehouses(ctx, wh)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	sites, err := readSites(ctx, wh)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	locations, err := readLocations(ctx, wh)
	if err != nil {
		return StepResult{Counts: c}, err
	}

	var watermark time.Time
	seen := func(ts ...sql.NullTime) {
		if t := latest(ts...); t.After(watermark) {
			watermark = t
		}
	}

	whIDs := map[string]int64{}
	for _, w := range warehouses {
		c.inc("warehouses_read")
		seen(w.CreatedAt, w.UpdatedAt, w.DeletedAt)
		id, err := s.importWarehouse(ctx, dst.Q, m, centerID, w, c)
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("warehouse %s: %w", w.ID, err)
		}
		if id != 0 {
			whIDs[w.ID] = id
		}
	}

	roomIDs := map[string]int64{}
	for _, r := range sites {
		c.inc("rooms_read")
		seen(r.CreatedAt, r.UpdatedAt, r.DeletedAt)
		warehouseID, ok := whIDs[r.WarehouseID]
		if !ok {
			c.inc("room_skipped_warehouse_unmapped:" + r.ID)
			continue
		}
		id, err := s.importRoom(ctx, dst.Q, m, centerID, warehouseID, r, c)
		if err != nil {
			return StepResult{Counts: c}, fmt.Errorf("warehouse site %s: %w", r.ID, err)
		}
		if id != 0 {
			roomIDs[r.ID] = id
		}
	}

	locs := map[string]importedLocation{}
	for _, l := range orderLocations(locations) {
		c.inc("locations_read")
		seen(l.CreatedAt, l.UpdatedAt, l.DeletedAt)
		if err := s.importLocation(ctx, dst.Q, m, centerID, roomIDs, locs, l, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("warehouse location %s: %w", l.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

// olexCenterID returns the Olex center organization (created by the
// organizations step).
func olexCenterID(ctx context.Context, q *db.Queries) (int64, error) {
	brand, err := q.GetBrandBySlug(ctx, OlexBrandSlug)
	if err != nil {
		return 0, fmt.Errorf("brand %q: %w", OlexBrandSlug, err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		return 0, fmt.Errorf("olex center (run the organizations step first): %w", err)
	}
	return center.ID, nil
}

func readWarehouses(ctx context.Context, wh source.LegacySource) ([]legacyWarehouse, error) {
	rows, err := wh.Query(ctx, whWarehousesQuery)
	if err != nil {
		return nil, err
	}
	var out []legacyWarehouse
	for rows.Next() {
		var w legacyWarehouse
		if err := rows.Scan(&w.ID, &w.Code, &w.Name, &w.Country, &w.City, &w.District, &w.Active, &w.SortOrder,
			&w.DeletedAt, &w.CreatedAt, &w.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan warehouse: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read warehouses: %w", err)
	}
	return out, nil
}

func readSites(ctx context.Context, wh source.LegacySource) ([]legacySite, error) {
	rows, err := wh.Query(ctx, whSitesQuery)
	if err != nil {
		return nil, err
	}
	var out []legacySite
	for rows.Next() {
		var r legacySite
		if err := rows.Scan(&r.ID, &r.WarehouseID, &r.Code, &r.Name, &r.Active, &r.SortOrder,
			&r.DeletedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan warehouse site: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read warehouse sites: %w", err)
	}
	return out, nil
}

func readLocations(ctx context.Context, wh source.LegacySource) ([]legacyLocation, error) {
	rows, err := wh.Query(ctx, whLocationsQuery)
	if err != nil {
		return nil, err
	}
	var out []legacyLocation
	for rows.Next() {
		var l legacyLocation
		if err := rows.Scan(&l.ID, &l.SiteID, &l.ParentID, &l.Type, &l.Code, &l.Name, &l.SortOrder, &l.Active,
			&l.DeletedAt, &l.CreatedAt, &l.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan warehouse location: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read warehouse locations: %w", err)
	}
	return out, nil
}

// orderLocations returns parents before their children (stable within a
// level). A location in a parent cycle or under a missing parent comes last;
// importLocation then reports it as unmapped.
func orderLocations(in []legacyLocation) []legacyLocation {
	byID := make(map[string]legacyLocation, len(in))
	for _, l := range in {
		byID[l.ID] = l
	}
	depth := make(map[string]int, len(in))
	var depthOf func(id string, guard int) int
	depthOf = func(id string, guard int) int {
		if d, ok := depth[id]; ok {
			return d
		}
		l, ok := byID[id]
		if !ok || guard > len(in) {
			return len(in) + 1
		}
		d := 0
		if l.ParentID.Valid && l.ParentID.String != "" {
			d = depthOf(l.ParentID.String, guard+1) + 1
		}
		depth[id] = d
		return d
	}
	out := append([]legacyLocation(nil), in...)
	sort.SliceStable(out, func(i, j int) bool { return depthOf(out[i].ID, 0) < depthOf(out[j].ID, 0) })
	return out
}

// whCode turns a legacy code into the new code pattern ^[A-Z0-9_]{1,32}$:
// upper case, every other character becomes '_'.
func whCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return truncate(b.String(), whCodeMax)
}

// whAddress joins the legacy district, city and country.
func whAddress(w legacyWarehouse) pgtype.Text {
	var parts []string
	for _, p := range []string{w.District, w.City, w.Country} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.Join(parts, ", "), Valid: true}
}

func (s WarehousesStep) importWarehouse(ctx context.Context, q *db.Queries, m *Mapper, centerID int64, w legacyWarehouse, c counts) (int64, error) {
	code := whCode(w.Code)
	name := truncate(strings.TrimSpace(w.Name), whNameMax)
	if name == "" {
		name = code
	}
	if code == "" {
		c.inc("warehouse_skipped_no_code:" + w.ID)
		return 0, nil
	}
	active := w.Active && !w.DeletedAt.Valid
	key := Key{System: s.whSystem(), Table: "warehouses", ID: w.ID, TargetTable: "warehouses"}
	sum := Checksum(w.Code, w.Name, w.Country, w.City, w.District, w.Active, w.SortOrder, w.DeletedAt.Valid)

	linked, err := linkExisting(ctx, m, key, sum, c, "warehouses_linked_existing", func() (uuid.UUID, error) {
		return q.MigratorFindWarehouse(ctx, db.MigratorFindWarehouseParams{OrganizationID: centerID, Code: code})
	})
	if err != nil {
		return 0, err
	}
	res, err := m.Upsert(ctx, key, sum)
	if err != nil {
		return 0, err
	}
	id, err := q.MigratorWarehouseIDByUUID(ctx, db.MigratorWarehouseIDByUUIDParams{Uuid: res.UUID, OrganizationID: centerID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		id, err = q.MigratorInsertWarehouse(ctx, db.MigratorInsertWarehouseParams{
			Uuid: res.UUID, OrganizationID: centerID, Code: code, Name: name, Address: whAddress(w),
			Active: active, SortOrder: w.SortOrder, CreatedAt: pgTime(w.CreatedAt),
		})
		if err != nil {
			return 0, fmt.Errorf("insert warehouse: %w", err)
		}
		c.inc("warehouses_created")
		return id, nil
	case err != nil:
		return 0, fmt.Errorf("read warehouse: %w", err)
	}
	if !res.Changed {
		if !linked {
			c.inc("warehouses_unchanged")
		}
		return id, nil
	}
	if err := q.MigratorUpdateWarehouse(ctx, db.MigratorUpdateWarehouseParams{
		ID: id, OrganizationID: centerID, Code: code, Name: name, Address: whAddress(w), Active: active, SortOrder: w.SortOrder,
	}); err != nil {
		return 0, fmt.Errorf("update warehouse: %w", err)
	}
	c.inc("warehouses_updated")
	return id, nil
}

func (s WarehousesStep) importRoom(ctx context.Context, q *db.Queries, m *Mapper, centerID, warehouseID int64, r legacySite, c counts) (int64, error) {
	code := whCode(r.Code)
	if code == "" {
		c.inc("room_skipped_no_code:" + r.ID)
		return 0, nil
	}
	name := truncate(strings.TrimSpace(r.Name), whNameMax)
	if name == "" {
		name = code
	}
	active := r.Active && !r.DeletedAt.Valid
	key := Key{System: s.whSystem(), Table: "warehouse_sites", ID: r.ID, TargetTable: "rooms"}
	sum := Checksum(r.WarehouseID, r.Code, r.Name, r.Active, r.SortOrder, r.DeletedAt.Valid)

	linked, err := linkExisting(ctx, m, key, sum, c, "rooms_linked_existing", func() (uuid.UUID, error) {
		return q.MigratorFindRoom(ctx, db.MigratorFindRoomParams{WarehouseID: warehouseID, Code: code})
	})
	if err != nil {
		return 0, err
	}
	res, err := m.Upsert(ctx, key, sum)
	if err != nil {
		return 0, err
	}
	room, err := q.MigratorRoomByUUID(ctx, db.MigratorRoomByUUIDParams{Uuid: res.UUID, OrganizationID: centerID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		id, err := q.MigratorInsertRoom(ctx, db.MigratorInsertRoomParams{
			Uuid: res.UUID, OrganizationID: centerID, WarehouseID: warehouseID, Code: code, Name: name,
			Active: active, SortOrder: r.SortOrder, CreatedAt: pgTime(r.CreatedAt),
		})
		if err != nil {
			return 0, fmt.Errorf("insert room: %w", err)
		}
		c.inc("rooms_created")
		return id, nil
	case err != nil:
		return 0, fmt.Errorf("read room: %w", err)
	}
	if room.WarehouseID != warehouseID {
		// A room cannot move to another warehouse; report, keep it.
		c.inc("room_warehouse_changed:" + r.ID)
	}
	if !res.Changed {
		if !linked {
			c.inc("rooms_unchanged")
		}
		return room.ID, nil
	}
	if err := q.MigratorUpdateRoom(ctx, db.MigratorUpdateRoomParams{
		ID: room.ID, OrganizationID: centerID, Code: code, Name: name, Active: active, SortOrder: r.SortOrder,
	}); err != nil {
		return 0, fmt.Errorf("update room: %w", err)
	}
	c.inc("rooms_updated")
	return room.ID, nil
}

// importedLocation is a location this run mapped: its id, type and full code.
type importedLocation struct {
	ID       int64
	RoomID   int64
	Type     string
	FullCode string
}

// locationType maps a legacy location type onto the new tree under parent
// (nil: room root). ok false means the location cannot be placed.
func locationType(legacy string, parent *importedLocation) (typ string, asShelf, ok bool) {
	t := strings.ToLower(strings.TrimSpace(legacy))
	switch {
	case parent == nil && (t == locAisle || t == locShelf):
		return t, false, true
	case parent == nil && t == locBin:
		// A bin needs a shelf parent; a root bin becomes a shelf.
		return locShelf, true, true
	case parent != nil && parent.Type == locAisle && t == locShelf:
		return locShelf, false, true
	case parent != nil && parent.Type == locAisle && t == locBin:
		return locShelf, true, true
	case parent != nil && parent.Type == locShelf && t == locBin:
		return locBin, false, true
	}
	return "", false, false
}

func (s WarehousesStep) importLocation(ctx context.Context, q *db.Queries, m *Mapper, centerID int64,
	roomIDs map[string]int64, locs map[string]importedLocation, l legacyLocation, c counts,
) error {
	roomID, ok := roomIDs[l.SiteID]
	if !ok {
		c.inc("location_skipped_room_unmapped:" + l.ID)
		return nil
	}
	var parent *importedLocation
	if l.ParentID.Valid && l.ParentID.String != "" {
		p, ok := locs[l.ParentID.String]
		if !ok {
			c.inc("location_skipped_parent_unmapped:" + l.ID)
			return nil
		}
		if p.RoomID != roomID {
			c.inc("location_skipped_parent_other_room:" + l.ID)
			return nil
		}
		parent = &p
	}
	typ, asShelf, ok := locationType(l.Type, parent)
	if !ok {
		c.inc("location_skipped_invalid_tree:" + l.ID)
		return nil
	}
	if asShelf {
		c.inc("location_bin_as_shelf:" + l.ID)
	}
	code := whCode(l.Code)
	if code == "" {
		c.inc("location_skipped_no_code:" + l.ID)
		return nil
	}
	name := truncate(strings.TrimSpace(l.Name), whNameMax)
	if name == "" {
		name = code
	}
	active := l.Active && !l.DeletedAt.Valid
	parentID := pgtype.Int8{}
	if parent != nil {
		parentID = pgInt8(parent.ID)
	}

	key := Key{System: s.whSystem(), Table: "warehouse_locations", ID: l.ID, TargetTable: "warehouse_locations"}
	sum := Checksum(l.SiteID, l.ParentID.String, l.Type, l.Code, l.Name, l.SortOrder, l.Active, l.DeletedAt.Valid)

	linked, err := linkExisting(ctx, m, key, sum, c, "locations_linked_existing", func() (uuid.UUID, error) {
		// The full code the new row would get (warehouse-room-...-code).
		full, err := expectedFullCode(ctx, q, centerID, roomID, parent, code)
		if err != nil {
			return uuid.UUID{}, err
		}
		return q.MigratorFindLocation(ctx, db.MigratorFindLocationParams{OrganizationID: centerID, FullCode: full})
	})
	if err != nil {
		return err
	}
	res, err := m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	row, err := q.MigratorLocationByUUID(ctx, db.MigratorLocationByUUIDParams{Uuid: res.UUID, OrganizationID: centerID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		ins, err := q.MigratorInsertLocation(ctx, db.MigratorInsertLocationParams{
			Uuid: res.UUID, OrganizationID: centerID, RoomID: pgInt8(roomID), ParentID: parentID,
			Type: pgtype.Text{String: typ, Valid: true}, Code: code, Name: name, Active: active,
			SortOrder: l.SortOrder, CreatedAt: pgTime(l.CreatedAt),
		})
		if err != nil {
			return fmt.Errorf("insert location: %w", err)
		}
		c.inc("locations_created")
		c.inc("locations_" + typ)
		locs[l.ID] = importedLocation{ID: ins.ID, RoomID: roomID, Type: typ, FullCode: ins.FullCode.String}
		return nil
	case err != nil:
		return fmt.Errorf("read location: %w", err)
	}
	if !row.RoomID.Valid {
		// Linked to an untyped (pre-tree) location: keep it as it is.
		c.inc("location_linked_untyped:" + l.ID)
		return nil
	}
	if !res.Changed {
		if !linked {
			c.inc("locations_unchanged")
		}
		locs[l.ID] = importedLocation{ID: row.ID, RoomID: row.RoomID.Int64, Type: typ, FullCode: row.FullCode.String}
		return nil
	}
	if err := q.MigratorUpdateLocation(ctx, db.MigratorUpdateLocationParams{
		ID: row.ID, OrganizationID: centerID, RoomID: pgInt8(roomID), ParentID: parentID,
		Type: pgtype.Text{String: typ, Valid: true}, Code: code, Name: name, Active: active, SortOrder: l.SortOrder,
	}); err != nil {
		return fmt.Errorf("update location: %w", err)
	}
	c.inc("locations_updated")
	row, err = q.MigratorLocationByUUID(ctx, db.MigratorLocationByUUIDParams{Uuid: res.UUID, OrganizationID: centerID})
	if err != nil {
		return fmt.Errorf("read location: %w", err)
	}
	locs[l.ID] = importedLocation{ID: row.ID, RoomID: roomID, Type: typ, FullCode: row.FullCode.String}
	return nil
}

// expectedFullCode is the full_code trg_warehouse_locations_derive gives a
// location with code under parent (nil: room root).
func expectedFullCode(ctx context.Context, q *db.Queries, centerID, roomID int64, parent *importedLocation, code string) (string, error) {
	if parent != nil {
		return parent.FullCode + "-" + code, nil
	}
	prefix, err := q.MigratorRoomFullCodePrefix(ctx, db.MigratorRoomFullCodePrefixParams{ID: roomID, OrganizationID: centerID})
	if err != nil {
		return "", fmt.Errorf("room code: %w", err)
	}
	return prefix + "-" + code, nil
}

// linkExisting maps a not yet mapped key to the row find returns (a row of
// this database with the same natural key), so a rerun after a lost mapping
// or a row created by hand is not duplicated. find returns pgx.ErrNoRows
// when there is none. linked reports a new link.
func linkExisting(ctx context.Context, m *Mapper, key Key, sum string, c counts, counter string,
	find func() (uuid.UUID, error),
) (bool, error) {
	_, mapped, err := m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil || mapped {
		return false, err
	}
	existing, err := find()
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	linked, err := m.Link(ctx, key, existing, sum)
	if err != nil {
		return false, err
	}
	if linked {
		c.inc(counter)
	}
	return linked, nil
}
