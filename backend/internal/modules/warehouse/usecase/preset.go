package usecase

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5"
)

// PresetWarehouseCode is the code of the warehouse the distributor's
// "register as warehouse" preset opens (K4, TEC-207). Rooms and locations
// are added later through the TEC-201 tree endpoints.
const PresetWarehouseCode = "MAIN"

// OpenPresetWarehouseTx opens the preset warehouse of a newly created
// distributor inside the caller's transaction (organizations
// RegisterOrganization). The warehouse takes the organization's name and
// address; only centers and distributors hold warehouses (K12).
func (s *Service) OpenPresetWarehouseTx(ctx context.Context, tx pgx.Tx, org db.Organization) error {
	switch org.Type {
	case rbac.OrgTypeCenter, rbac.OrgTypeDistributor:
	default:
		return ErrForbidden
	}
	name := strings.TrimSpace(org.Name)
	if utf8.RuneCountInString(name) > maxNameLen {
		name = string([]rune(name)[:maxNameLen])
	}
	if name == "" {
		name = PresetWarehouseCode
	}
	addr := org.Address
	_, err := s.q.WithTx(tx).CreateWarehouse(ctx, db.CreateWarehouseParams{
		OrganizationID: org.ID, Code: PresetWarehouseCode, Name: name, Address: addressArg(&addr), Active: true,
	})
	return mapDBError(err)
}
