package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
)

// Room is the API view of a room.
type Room struct {
	UUID          uuid.UUID `json:"uuid"`
	WarehouseUUID uuid.UUID `json:"warehouse_uuid"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Active        bool      `json:"active"`
	SortOrder     int32     `json:"sort_order"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func roomView(r db.Room, warehouse uuid.UUID) Room {
	return Room{
		UUID: r.Uuid, WarehouseUUID: warehouse, Code: r.Code, Name: r.Name, Active: r.Active,
		SortOrder: r.SortOrder, CreatedAt: ts(r.CreatedAt), UpdatedAt: ts(r.UpdatedAt),
	}
}

// RoomInput creates a room.
type RoomInput struct {
	Code string
	Name string
}

// RoomPatch updates a room; nil fields stay.
type RoomPatch struct {
	Code   *string
	Name   *string
	Active *bool
}

func (s *Service) room(ctx context.Context, org int64, id uuid.UUID) (db.Room, error) {
	r, err := s.q.GetRoomByUUID(ctx, db.GetRoomByUUIDParams{Uuid: id, OrganizationID: org})
	return r, notFound(err, ErrRoomNotFound)
}

func (s *Service) roomWarehouseUUID(ctx context.Context, org int64, r db.Room) (uuid.UUID, error) {
	w, err := s.q.GetWarehouseByID(ctx, db.GetWarehouseByIDParams{ID: r.WarehouseID, OrganizationID: org})
	if err != nil {
		return uuid.Nil, notFound(err, ErrWarehouseNotFound)
	}
	return w.Uuid, nil
}

// ListRooms lists the rooms of a warehouse.
func (s *Service) ListRooms(ctx context.Context, c Caller, warehouseID uuid.UUID) ([]Room, error) {
	org, err := guard(c)
	if err != nil {
		return nil, err
	}
	w, err := s.warehouse(ctx, org, warehouseID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListRooms(ctx, db.ListRoomsParams{WarehouseID: w.ID, OrganizationID: org})
	if err != nil {
		return nil, err
	}
	out := make([]Room, 0, len(rows))
	for _, r := range rows {
		out = append(out, roomView(r, w.Uuid))
	}
	return out, nil
}

// GetRoom returns one room of the active organization.
func (s *Service) GetRoom(ctx context.Context, c Caller, id uuid.UUID) (Room, error) {
	org, err := guard(c)
	if err != nil {
		return Room{}, err
	}
	r, err := s.room(ctx, org, id)
	if err != nil {
		return Room{}, err
	}
	wu, err := s.roomWarehouseUUID(ctx, org, r)
	if err != nil {
		return Room{}, err
	}
	return roomView(r, wu), nil
}

// CreateRoom adds a room to a warehouse.
func (s *Service) CreateRoom(ctx context.Context, c Caller, warehouseID uuid.UUID, in RoomInput) (Room, error) {
	org, err := guard(c)
	if err != nil {
		return Room{}, err
	}
	w, err := s.warehouse(ctx, org, warehouseID)
	if err != nil {
		return Room{}, err
	}
	code, err := normCode("code", in.Code)
	if err != nil {
		return Room{}, err
	}
	name, err := normName("name", in.Name, code)
	if err != nil {
		return Room{}, err
	}
	r, err := s.q.CreateRoom(ctx, db.CreateRoomParams{
		OrganizationID: org, WarehouseID: w.ID, Code: code, Name: name, Active: true,
	})
	if err != nil {
		return Room{}, mapDBError(err)
	}
	return roomView(r, w.Uuid), nil
}

// UpdateRoom edits a room; a new code re-derives the full_code of every
// location in it.
func (s *Service) UpdateRoom(ctx context.Context, c Caller, id uuid.UUID, p RoomPatch) (Room, error) {
	org, err := guard(c)
	if err != nil {
		return Room{}, err
	}
	r, err := s.room(ctx, org, id)
	if err != nil {
		return Room{}, err
	}
	arg := db.UpdateRoomParams{ID: r.ID, OrganizationID: org, Code: r.Code, Name: r.Name, Active: r.Active}
	if p.Code != nil {
		if arg.Code, err = normCode("code", *p.Code); err != nil {
			return Room{}, err
		}
	}
	if p.Name != nil {
		if arg.Name, err = normName("name", *p.Name, ""); err != nil {
			return Room{}, err
		}
	}
	if p.Active != nil {
		arg.Active = *p.Active
	}
	var out db.Room
	err = s.inTx(ctx, func(q *db.Queries) error {
		var e error
		out, e = q.UpdateRoom(ctx, arg)
		return e
	})
	if err != nil {
		return Room{}, mapDBError(notFound(err, ErrRoomNotFound))
	}
	wu, err := s.roomWarehouseUUID(ctx, org, out)
	if err != nil {
		return Room{}, err
	}
	return roomView(out, wu), nil
}

// DeleteRoom removes an empty room (no locations).
func (s *Service) DeleteRoom(ctx context.Context, c Caller, id uuid.UUID) error {
	org, err := guard(c)
	if err != nil {
		return err
	}
	r, err := s.room(ctx, org, id)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteRoom(ctx, db.DeleteRoomParams{ID: r.ID, OrganizationID: org})
	if err != nil {
		return mapDBError(err)
	}
	if n == 0 {
		return ErrRoomNotFound
	}
	return nil
}

// ReorderRooms sets sort_order to the position in uuids; the rooms belong
// to one warehouse.
func (s *Service) ReorderRooms(ctx context.Context, c Caller, uuids []uuid.UUID) error {
	org, err := guard(c)
	if err != nil {
		return err
	}
	rows, err := s.q.ListRoomsByUUIDs(ctx, db.ListRoomsByUUIDsParams{Uuids: uuids, OrganizationID: org})
	if err != nil {
		return err
	}
	found := make(map[uuid.UUID]int64, len(rows))
	for _, r := range rows {
		if r.WarehouseID != rows[0].WarehouseID {
			return invalid("uuids", "must belong to one warehouse")
		}
		found[r.Uuid] = r.ID
	}
	ids, err := orderedIDs(uuids, found)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(q *db.Queries) error {
		for i, id := range ids {
			if _, err := q.SetRoomSortOrder(ctx, db.SetRoomSortOrderParams{
				ID: id, OrganizationID: org, SortOrder: int32(i), //nolint:gosec // bounded by MaxReorder
			}); err != nil {
				return err
			}
		}
		return nil
	})
}
