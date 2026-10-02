package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Warehouse is the API view of a warehouse.
type Warehouse struct {
	UUID      uuid.UUID `json:"uuid"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Address   *string   `json:"address"`
	Active    bool      `json:"active"`
	SortOrder int32     `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func warehouseView(w db.Warehouse) Warehouse {
	return Warehouse{
		UUID: w.Uuid, Code: w.Code, Name: w.Name, Address: textPtr(w.Address),
		Active: w.Active, SortOrder: w.SortOrder, CreatedAt: ts(w.CreatedAt), UpdatedAt: ts(w.UpdatedAt),
	}
}

// WarehouseInput creates a warehouse.
type WarehouseInput struct {
	Code    string
	Name    string
	Address *string
}

// WarehousePatch updates a warehouse; nil fields stay.
type WarehousePatch struct {
	Code    *string
	Name    *string
	Address *string
	Active  *bool
}

func addressArg(a *string) pgtype.Text {
	if a == nil || strings.TrimSpace(*a) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(*a), Valid: true}
}

// ListWarehouses lists the active organization's warehouses (active filters
// when set).
func (s *Service) ListWarehouses(ctx context.Context, c Caller, active *bool) ([]Warehouse, error) {
	org, err := guard(c)
	if err != nil {
		return nil, err
	}
	arg := db.ListWarehousesParams{OrganizationID: org}
	if active != nil {
		arg.Active = pgtype.Bool{Bool: *active, Valid: true}
	}
	rows, err := s.q.ListWarehouses(ctx, arg)
	if err != nil {
		return nil, err
	}
	out := make([]Warehouse, 0, len(rows))
	for _, w := range rows {
		out = append(out, warehouseView(w))
	}
	return out, nil
}

func (s *Service) warehouse(ctx context.Context, org int64, id uuid.UUID) (db.Warehouse, error) {
	w, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: id, OrganizationID: org})
	return w, notFound(err, ErrWarehouseNotFound)
}

// GetWarehouse returns one warehouse of the active organization.
func (s *Service) GetWarehouse(ctx context.Context, c Caller, id uuid.UUID) (Warehouse, error) {
	org, err := guard(c)
	if err != nil {
		return Warehouse{}, err
	}
	w, err := s.warehouse(ctx, org, id)
	if err != nil {
		return Warehouse{}, err
	}
	return warehouseView(w), nil
}

// CreateWarehouse opens a warehouse in the active organization.
func (s *Service) CreateWarehouse(ctx context.Context, c Caller, in WarehouseInput) (Warehouse, error) {
	org, err := guard(c)
	if err != nil {
		return Warehouse{}, err
	}
	code, err := normCode("code", in.Code)
	if err != nil {
		return Warehouse{}, err
	}
	name, err := normName("name", in.Name, code)
	if err != nil {
		return Warehouse{}, err
	}
	w, err := s.q.CreateWarehouse(ctx, db.CreateWarehouseParams{
		OrganizationID: org, Code: code, Name: name, Address: addressArg(in.Address), Active: true,
	})
	if err != nil {
		return Warehouse{}, mapDBError(err)
	}
	return warehouseView(w), nil
}

// UpdateWarehouse edits a warehouse; a new code re-derives the full_code of
// every location below it.
func (s *Service) UpdateWarehouse(ctx context.Context, c Caller, id uuid.UUID, p WarehousePatch) (Warehouse, error) {
	org, err := guard(c)
	if err != nil {
		return Warehouse{}, err
	}
	w, err := s.warehouse(ctx, org, id)
	if err != nil {
		return Warehouse{}, err
	}
	arg := db.UpdateWarehouseParams{
		ID: w.ID, OrganizationID: org, Code: w.Code, Name: w.Name, Address: w.Address, Active: w.Active,
	}
	if p.Code != nil {
		if arg.Code, err = normCode("code", *p.Code); err != nil {
			return Warehouse{}, err
		}
	}
	if p.Name != nil {
		if arg.Name, err = normName("name", *p.Name, ""); err != nil {
			return Warehouse{}, err
		}
	}
	if p.Address != nil {
		arg.Address = addressArg(p.Address)
	}
	if p.Active != nil {
		arg.Active = *p.Active
	}
	var out db.Warehouse
	err = s.inTx(ctx, func(q *db.Queries) error {
		var e error
		out, e = q.UpdateWarehouse(ctx, arg)
		return e
	})
	if err != nil {
		return Warehouse{}, mapDBError(notFound(err, ErrWarehouseNotFound))
	}
	return warehouseView(out), nil
}

// DeleteWarehouse removes an empty warehouse (no rooms).
func (s *Service) DeleteWarehouse(ctx context.Context, c Caller, id uuid.UUID) error {
	org, err := guard(c)
	if err != nil {
		return err
	}
	w, err := s.warehouse(ctx, org, id)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteWarehouse(ctx, db.DeleteWarehouseParams{ID: w.ID, OrganizationID: org})
	if err != nil {
		return mapDBError(err)
	}
	if n == 0 {
		return ErrWarehouseNotFound
	}
	return nil
}

// ReorderWarehouses sets sort_order to the position in uuids.
func (s *Service) ReorderWarehouses(ctx context.Context, c Caller, uuids []uuid.UUID) error {
	org, err := guard(c)
	if err != nil {
		return err
	}
	rows, err := s.q.ListWarehousesByUUIDs(ctx, db.ListWarehousesByUUIDsParams{Uuids: uuids, OrganizationID: org})
	if err != nil {
		return err
	}
	found := make(map[uuid.UUID]int64, len(rows))
	for _, w := range rows {
		found[w.Uuid] = w.ID
	}
	ids, err := orderedIDs(uuids, found)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(q *db.Queries) error {
		for i, id := range ids {
			if _, err := q.SetWarehouseSortOrder(ctx, db.SetWarehouseSortOrderParams{
				ID: id, OrganizationID: org, SortOrder: int32(i), //nolint:gosec // bounded by the request size
			}); err != nil {
				return err
			}
		}
		return nil
	})
}
