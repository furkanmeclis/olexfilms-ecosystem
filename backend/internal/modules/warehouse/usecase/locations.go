package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Location is the API view of a typed location (aisle, shelf, bin).
type Location struct {
	UUID          uuid.UUID  `json:"uuid"`
	WarehouseUUID uuid.UUID  `json:"warehouse_uuid"`
	RoomUUID      uuid.UUID  `json:"room_uuid"`
	ParentUUID    *uuid.UUID `json:"parent_uuid"`
	Type          string     `json:"type"`
	Code          string     `json:"code"`
	FullCode      string     `json:"full_code"`
	Name          string     `json:"name"`
	Active        bool       `json:"active"`
	SortOrder     int32      `json:"sort_order"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func locationView(l db.WarehouseLocation, warehouse, room uuid.UUID, parent *uuid.UUID) Location {
	return Location{
		UUID: l.Uuid, WarehouseUUID: warehouse, RoomUUID: room, ParentUUID: parent,
		Type: l.Type.String, Code: l.Code, FullCode: l.FullCode.String, Name: l.Name, Active: l.Active,
		SortOrder: l.SortOrder, CreatedAt: ts(l.CreatedAt), UpdatedAt: ts(l.UpdatedAt),
	}
}

// LocationInput creates one location.
type LocationInput struct {
	RoomUUID   uuid.UUID
	ParentUUID *uuid.UUID
	Type       string
	Code       string
	Name       string
}

// LocationPatch updates a location; nil fields stay. A new code
// re-derives the full_code of the location and everything below it.
type LocationPatch struct {
	Code   *string
	Name   *string
	Active *bool
}

// GenerateLevel is one level of a bulk generation: an explicit code list,
// or a numeric range From..To rendered with Prefix and zero padding Pad
// (Pad 2: 1 -> "01").
type GenerateLevel struct {
	Type   string
	Codes  []string
	From   *int
	To     *int
	Pad    int
	Prefix string
}

// GenerateInput builds a location tree under a room (or under one of its
// locations): every node of a level gets every code of the next level.
type GenerateInput struct {
	RoomUUID   uuid.UUID
	ParentUUID *uuid.UUID
	Levels     []GenerateLevel
}

// GenerateResult counts the nodes of a bulk generation; nodes that already
// existed (same parent, same code) are reused, not duplicated.
type GenerateResult struct {
	Created  int `json:"created"`
	Existing int `json:"existing"`
}

// childTypes: the location types allowed under a parent type ("" = room
// root): aisle -> shelf -> bin, shelves may also sit at the room root.
func childTypes(parent string) []string {
	switch parent {
	case "":
		return []string{TypeAisle, TypeShelf}
	case TypeAisle:
		return []string{TypeShelf}
	case TypeShelf:
		return []string{TypeBin}
	default:
		return nil
	}
}

func allowedChild(parent, typ string) bool {
	for _, t := range childTypes(parent) {
		if t == typ {
			return true
		}
	}
	return false
}

func validType(t string) bool { return t == TypeAisle || t == TypeShelf || t == TypeBin }

func uuidPtr(u uuid.UUID) *uuid.UUID { return &u }

func (s *Service) location(ctx context.Context, org int64, id uuid.UUID) (db.WarehouseLocation, error) {
	l, err := s.q.GetTypedLocationByUUID(ctx, db.GetTypedLocationByUUIDParams{Uuid: id, OrganizationID: org})
	return l, notFound(err, ErrLocationNotFound)
}

// view resolves the warehouse, room and parent uuids of one location.
func (s *Service) view(ctx context.Context, org int64, l db.WarehouseLocation) (Location, error) {
	r, err := s.q.GetRoomByID(ctx, db.GetRoomByIDParams{ID: l.RoomID.Int64, OrganizationID: org})
	if err != nil {
		return Location{}, notFound(err, ErrRoomNotFound)
	}
	wu, err := s.roomWarehouseUUID(ctx, org, r)
	if err != nil {
		return Location{}, err
	}
	var parent *uuid.UUID
	if l.ParentID.Valid {
		p, err := s.q.GetWarehouseLocation(ctx, db.GetWarehouseLocationParams{ID: l.ParentID.Int64, OrganizationID: org})
		if err != nil {
			return Location{}, notFound(err, ErrLocationNotFound)
		}
		parent = uuidPtr(p.Uuid)
	}
	return locationView(l, wu, r.Uuid, parent), nil
}

// ListRoomLocations returns the whole location tree of a room as a flat
// list (parent_uuid links; siblings ordered by sort_order).
func (s *Service) ListRoomLocations(ctx context.Context, c Caller, roomID uuid.UUID) ([]Location, error) {
	org, err := guard(c)
	if err != nil {
		return nil, err
	}
	r, err := s.room(ctx, org, roomID)
	if err != nil {
		return nil, err
	}
	wu, err := s.roomWarehouseUUID(ctx, org, r)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListRoomLocations(ctx, db.ListRoomLocationsParams{
		RoomID: pgtype.Int8{Int64: r.ID, Valid: true}, OrganizationID: org,
	})
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]uuid.UUID, len(rows))
	for _, l := range rows {
		byID[l.ID] = l.Uuid
	}
	out := make([]Location, 0, len(rows))
	for _, l := range rows {
		var parent *uuid.UUID
		if l.ParentID.Valid {
			if pu, ok := byID[l.ParentID.Int64]; ok {
				parent = uuidPtr(pu)
			}
		}
		out = append(out, locationView(l, wu, r.Uuid, parent))
	}
	return out, nil
}

// GetLocation returns one typed location of the active organization.
func (s *Service) GetLocation(ctx context.Context, c Caller, id uuid.UUID) (Location, error) {
	org, err := guard(c)
	if err != nil {
		return Location{}, err
	}
	l, err := s.location(ctx, org, id)
	if err != nil {
		return Location{}, err
	}
	return s.view(ctx, org, l)
}

// resolveParent loads the room and the optional parent location and
// returns the parent's id ("" type for the room root).
func (s *Service) resolveParent(ctx context.Context, org int64, roomID uuid.UUID, parentID *uuid.UUID) (db.Room, pgtype.Int8, string, error) {
	if roomID == uuid.Nil {
		return db.Room{}, pgtype.Int8{}, "", invalid("room_uuid", "is required")
	}
	r, err := s.room(ctx, org, roomID)
	if err != nil {
		return db.Room{}, pgtype.Int8{}, "", err
	}
	if parentID == nil || *parentID == uuid.Nil {
		return r, pgtype.Int8{}, "", nil
	}
	p, err := s.location(ctx, org, *parentID)
	if err != nil {
		return db.Room{}, pgtype.Int8{}, "", err
	}
	if p.RoomID.Int64 != r.ID {
		return db.Room{}, pgtype.Int8{}, "", invalid("parent_uuid", "must be a location of the room")
	}
	return r, pgtype.Int8{Int64: p.ID, Valid: true}, p.Type.String, nil
}

// CreateLocation adds one location to a room.
func (s *Service) CreateLocation(ctx context.Context, c Caller, in LocationInput) (Location, error) {
	org, err := guard(c)
	if err != nil {
		return Location{}, err
	}
	typ := strings.TrimSpace(in.Type)
	if !validType(typ) {
		return Location{}, invalid("type", "must be aisle, shelf or bin")
	}
	code, err := normCode("code", in.Code)
	if err != nil {
		return Location{}, err
	}
	name, err := normName("name", in.Name, code)
	if err != nil {
		return Location{}, err
	}
	r, parent, parentType, err := s.resolveParent(ctx, org, in.RoomUUID, in.ParentUUID)
	if err != nil {
		return Location{}, err
	}
	if !allowedChild(parentType, typ) {
		return Location{}, invalid("parent_uuid", "does not fit the location type")
	}
	l, err := s.q.CreateTypedLocation(ctx, db.CreateTypedLocationParams{
		OrganizationID: org, RoomID: pgtype.Int8{Int64: r.ID, Valid: true}, ParentID: parent,
		Type: pgtype.Text{String: typ, Valid: true}, Code: code, Name: name, Active: true,
	})
	if err != nil {
		return Location{}, mapDBError(err)
	}
	return s.view(ctx, org, l)
}

// UpdateLocation edits a location.
func (s *Service) UpdateLocation(ctx context.Context, c Caller, id uuid.UUID, p LocationPatch) (Location, error) {
	org, err := guard(c)
	if err != nil {
		return Location{}, err
	}
	l, err := s.location(ctx, org, id)
	if err != nil {
		return Location{}, err
	}
	arg := db.UpdateTypedLocationParams{ID: l.ID, OrganizationID: org, Code: l.Code, Name: l.Name, Active: l.Active}
	if p.Code != nil {
		if arg.Code, err = normCode("code", *p.Code); err != nil {
			return Location{}, err
		}
	}
	if p.Name != nil {
		if arg.Name, err = normName("name", *p.Name, ""); err != nil {
			return Location{}, err
		}
	}
	if p.Active != nil {
		arg.Active = *p.Active
	}
	var out db.WarehouseLocation
	err = s.inTx(ctx, func(q *db.Queries) error {
		var e error
		out, e = q.UpdateTypedLocation(ctx, arg)
		return e
	})
	if err != nil {
		return Location{}, mapDBError(notFound(err, ErrLocationNotFound))
	}
	return s.view(ctx, org, out)
}

// DeleteLocation removes a location without children and without stock
// history; otherwise ErrInUse (deactivate it instead).
func (s *Service) DeleteLocation(ctx context.Context, c Caller, id uuid.UUID) error {
	org, err := guard(c)
	if err != nil {
		return err
	}
	l, err := s.location(ctx, org, id)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteTypedLocation(ctx, db.DeleteTypedLocationParams{ID: l.ID, OrganizationID: org})
	if err != nil {
		return mapDBError(err)
	}
	if n == 0 {
		return ErrLocationNotFound
	}
	return nil
}

// ReorderLocations sets sort_order to the position in uuids; the locations
// are siblings (same room, same parent).
func (s *Service) ReorderLocations(ctx context.Context, c Caller, uuids []uuid.UUID) error {
	org, err := guard(c)
	if err != nil {
		return err
	}
	rows, err := s.q.ListLocationsByUUIDs(ctx, db.ListLocationsByUUIDsParams{Uuids: uuids, OrganizationID: org})
	if err != nil {
		return err
	}
	found := make(map[uuid.UUID]int64, len(rows))
	var first *db.WarehouseLocation
	for i, l := range rows {
		if !l.RoomID.Valid {
			continue // legacy locations are not part of the tree
		}
		if first == nil {
			first = &rows[i]
		}
		if l.RoomID != first.RoomID || l.ParentID != first.ParentID {
			return invalid("uuids", "must be siblings (same room and parent)")
		}
		found[l.Uuid] = l.ID
	}
	ids, err := orderedIDs(uuids, found)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(q *db.Queries) error {
		for i, id := range ids {
			if _, err := q.SetLocationSortOrder(ctx, db.SetLocationSortOrderParams{
				ID: id, OrganizationID: org, SortOrder: int32(i), //nolint:gosec // bounded by MaxReorder
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// levelCodes expands one generation level into its codes.
func levelCodes(i int, lv GenerateLevel) ([]string, error) {
	field := fmt.Sprintf("levels[%d]", i)
	var raw []string
	switch {
	case len(lv.Codes) > 0:
		if lv.From != nil || lv.To != nil {
			return nil, invalid(field, "use either codes or from/to")
		}
		raw = lv.Codes
	case lv.From != nil && lv.To != nil:
		from, to := *lv.From, *lv.To
		if from < 0 || to < from || to > 9999 {
			return nil, invalid(field+".to", "range must satisfy 0 <= from <= to <= 9999")
		}
		if lv.Pad < 0 || lv.Pad > 4 {
			return nil, invalid(field+".pad", "must be between 0 and 4")
		}
		if to-from+1 > MaxGenerated {
			return nil, invalid(field, "range is too large")
		}
		for n := from; n <= to; n++ {
			raw = append(raw, fmt.Sprintf("%s%0*d", lv.Prefix, lv.Pad, n))
		}
	default:
		return nil, invalid(field, "codes or from/to is required")
	}
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		code, err := normCode(field+".codes", r)
		if err != nil {
			return nil, err
		}
		if seen[code] {
			return nil, invalid(field+".codes", "must not repeat")
		}
		seen[code] = true
		out = append(out, code)
	}
	return out, nil
}

// GenerateLocations builds a tree in one transaction, e.g. aisle A x shelf
// 1-10 x bin 1-5. Existing siblings with the same code are reused.
func (s *Service) GenerateLocations(ctx context.Context, c Caller, in GenerateInput) (GenerateResult, error) {
	org, err := guard(c)
	if err != nil {
		return GenerateResult{}, err
	}
	if len(in.Levels) == 0 {
		return GenerateResult{}, invalid("levels", "is required")
	}
	r, parent, parentType, err := s.resolveParent(ctx, org, in.RoomUUID, in.ParentUUID)
	if err != nil {
		return GenerateResult{}, err
	}
	codes := make([][]string, len(in.Levels))
	types := make([]string, len(in.Levels))
	prev, nodes, width := parentType, 0, 1
	for i, lv := range in.Levels {
		typ := strings.TrimSpace(lv.Type)
		if !validType(typ) {
			return GenerateResult{}, invalid(fmt.Sprintf("levels[%d].type", i), "must be aisle, shelf or bin")
		}
		if !allowedChild(prev, typ) {
			return GenerateResult{}, invalid(fmt.Sprintf("levels[%d].type", i), "does not fit under "+typeLabel(prev))
		}
		if codes[i], err = levelCodes(i, lv); err != nil {
			return GenerateResult{}, err
		}
		width *= len(codes[i])
		nodes += width
		if nodes > MaxGenerated {
			return GenerateResult{}, invalid("levels", fmt.Sprintf("would touch more than %d locations", MaxGenerated))
		}
		types[i], prev = typ, typ
	}

	var res GenerateResult
	room := pgtype.Int8{Int64: r.ID, Valid: true}
	err = s.inTx(ctx, func(q *db.Queries) error {
		frontier := []pgtype.Int8{parent}
		for i := range types {
			next := make([]pgtype.Int8, 0, len(frontier)*len(codes[i]))
			for _, p := range frontier {
				for _, code := range codes[i] {
					l, err := q.FindTypedLocationChild(ctx, db.FindTypedLocationChildParams{RoomID: room, ParentID: p, Code: code})
					switch {
					case err == nil:
						if l.Type.String != types[i] {
							return invalid(fmt.Sprintf("levels[%d].type", i),
								"location "+l.FullCode.String+" already exists as "+l.Type.String)
						}
						res.Existing++
					case errors.Is(err, pgx.ErrNoRows):
						l, err = q.CreateTypedLocation(ctx, db.CreateTypedLocationParams{
							OrganizationID: org, RoomID: room, ParentID: p,
							Type: pgtype.Text{String: types[i], Valid: true}, Code: code, Name: code, Active: true,
						})
						if err != nil {
							return mapDBError(err)
						}
						res.Created++
					default:
						return err
					}
					next = append(next, pgtype.Int8{Int64: l.ID, Valid: true})
				}
			}
			frontier = next
		}
		return nil
	})
	if err != nil {
		return GenerateResult{}, err
	}
	return res, nil
}

func typeLabel(t string) string {
	if t == "" {
		return "the room"
	}
	return "a " + t
}
