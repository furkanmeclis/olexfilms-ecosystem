package usecase

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

// TEC-207 (K4): the "register as warehouse" preset of a distributor opens
// its first warehouse in the warehouse module. The hook runs inside the
// transaction that creates the organization, so a failing warehouse rolls
// the organization back and a saved distributor always has its warehouse.

// WarehousePresetHook opens the preset warehouse of a newly created
// organization (only called when the preset is set).
type WarehousePresetHook interface {
	OpenPresetWarehouseTx(ctx context.Context, tx pgx.Tx, org db.Organization) error
}

// WarehousePresetHookFunc adapts a function to WarehousePresetHook.
type WarehousePresetHookFunc func(ctx context.Context, tx pgx.Tx, org db.Organization) error

// OpenPresetWarehouseTx calls f.
func (f WarehousePresetHookFunc) OpenPresetWarehouseTx(ctx context.Context, tx pgx.Tx, org db.Organization) error {
	return f(ctx, tx, org)
}

// SetWarehousePresetHook wires the hook run by RegisterOrganization for the
// register_as_warehouse preset (nil: the preset is only stored in settings).
func (s *Service) SetWarehousePresetHook(h WarehousePresetHook) { s.warehouseHook = h }
