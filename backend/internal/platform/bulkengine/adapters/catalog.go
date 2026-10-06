package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TEC-369: bulk actions of the catalog category list (tenant, center only)
// and of the global vehicle catalog (platform, vehicle_catalog.write).

// --- catalog.categories -----------------------------------------------------

const ResourceCatalogCategories = "catalog.categories"

// CatalogCategoriesAdapter activates, deactivates or deletes product
// categories of the active brand (center only, K4). A brand whose
// categories come from an integration connection refuses every action
// (TEC-268: name, active and the row itself are owned by the hub).
type CatalogCategoriesAdapter struct {
	q *db.Queries
}

func NewCatalogCategories(q *db.Queries) *CatalogCategoriesAdapter {
	return &CatalogCategoriesAdapter{q: q}
}

func (a *CatalogCategoriesAdapter) Resource() string { return ResourceCatalogCategories }

func (a *CatalogCategoriesAdapter) WithQueries(q *db.Queries) bulkengine.BulkAdapter {
	return &CatalogCategoriesAdapter{q: q}
}

func (a *CatalogCategoriesAdapter) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "activate", LabelKey: "bulk.actions.catalog_categories.activate",
			Permission: rbac.PermCatalogWrite, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions.catalog_categories.deactivate",
			Permission: rbac.PermCatalogWrite, Destructive: true, Reversible: true,
			ConfirmKey: "bulk.confirm.catalog_categories.deactivate",
		},
		{
			// Only categories without products are deleted; the others
			// fail per item ("in use"). Not undoable (a new uuid would
			// break references kept elsewhere).
			ID: "delete", LabelKey: "bulk.actions.catalog_categories.delete",
			Permission: rbac.PermCatalogWrite, Destructive: true,
			ConfirmKey: "bulk.confirm.catalog_categories.delete",
		},
	}
}

func (a *CatalogCategoriesAdapter) ResolveTargets(_ context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
	return idsOnly(target)
}

// center loads the run organization; it must be a brand center whose
// categories are local (no integration connection).
func (a *CatalogCategoriesAdapter) center(ctx context.Context) (db.Organization, error) {
	org, err := runOrganization(ctx, a.q)
	if err != nil {
		return org, err
	}
	if org.Type != "center" {
		return org, fmt.Errorf("catalog bulk actions are center only")
	}
	return org, nil
}

func (a *CatalogCategoriesAdapter) synced(ctx context.Context, brandID int64) (bool, error) {
	return a.q.BrandHasIntegrationConnection(ctx, brandID)
}

func (a *CatalogCategoriesAdapter) category(ctx context.Context, brandID int64, entityUUID string) (db.ProductCategory, error) {
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return db.ProductCategory{}, bulkengine.ErrEntityGone
	}
	c, err := a.q.GetProductCategoryByUUID(ctx, db.GetProductCategoryByUUIDParams{Uuid: id, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ProductCategory{}, bulkengine.ErrEntityGone
	}
	return c, err
}

func (a *CatalogCategoriesAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	var want bool
	switch action {
	case "activate":
		want = true
	case "deactivate", "delete":
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	org, err := a.center(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	c, err := a.category(ctx, org.BrandID, entityUUID)
	if errors.Is(err, bulkengine.ErrEntityGone) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	synced, err := a.synced(ctx, org.BrandID)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	if synced {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "locked"}, nil
	}
	if action == "delete" {
		n, err := a.q.DeleteUnusedProductCategory(ctx, db.DeleteUnusedProductCategoryParams{ID: c.ID, BrandID: org.BrandID})
		if err != nil {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
		}
		if n == 0 {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "in use"}, nil
		}
		return bulkengine.BulkItemResult{
			EntityUUID: entityUUID, EntityType: "product_category", OK: true, Op: "delete",
			Previous: map[string]any{"name": c.Name, "active": c.Active, "sort": c.Sort},
		}, nil
	}
	res := bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "product_category", OK: true, Op: "update",
		Previous: map[string]any{"active": c.Active},
		Applied:  map[string]any{"active": want},
	}
	if c.Active == want {
		return res, nil
	}
	if _, err := a.q.SetProductCategoryActiveByUUID(ctx, db.SetProductCategoryActiveByUUIDParams{
		Active: want, Uuid: c.Uuid, BrandID: org.BrandID,
	}); err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
	}
	return res, nil
}

func (a *CatalogCategoriesAdapter) CurrentState(ctx context.Context, _ string, entityUUID string) (map[string]any, error) {
	org, err := a.center(ctx)
	if err != nil {
		return nil, err
	}
	c, err := a.category(ctx, org.BrandID, entityUUID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"active": c.Active}, nil
}

func (a *CatalogCategoriesAdapter) RevertItem(ctx context.Context, action string, entityUUID string, previous map[string]any) error {
	if action == "delete" {
		return fmt.Errorf("delete is not reversible")
	}
	org, err := a.center(ctx)
	if err != nil {
		return err
	}
	c, err := a.category(ctx, org.BrandID, entityUUID)
	if err != nil {
		return err
	}
	prev, ok := previous["active"].(bool)
	if !ok {
		return fmt.Errorf("missing category snapshot")
	}
	_, err = a.q.SetProductCategoryActiveByUUID(ctx, db.SetProductCategoryActiveByUUIDParams{
		Active: prev, Uuid: c.Uuid, BrandID: org.BrandID,
	})
	return err
}

// --- vehicle_catalog.brands / vehicle_catalog.models -------------------------

const (
	ResourceVehicleBrands = "vehicle_catalog.brands"
	ResourceVehicleModels = "vehicle_catalog.models"
)

// VehicleCatalogAdapter activates / deactivates car brands or car models
// (global reference data, platform bulk, vehicle_catalog.write).
type VehicleCatalogAdapter struct {
	q        *db.Queries
	resource string
}

// NewVehicleBrands is the car brands adapter.
func NewVehicleBrands(q *db.Queries) *VehicleCatalogAdapter {
	return &VehicleCatalogAdapter{q: q, resource: ResourceVehicleBrands}
}

// NewVehicleModels is the car models adapter.
func NewVehicleModels(q *db.Queries) *VehicleCatalogAdapter {
	return &VehicleCatalogAdapter{q: q, resource: ResourceVehicleModels}
}

func (a *VehicleCatalogAdapter) Resource() string { return a.resource }

func (a *VehicleCatalogAdapter) WithQueries(q *db.Queries) bulkengine.BulkAdapter {
	return &VehicleCatalogAdapter{q: q, resource: a.resource}
}

func (a *VehicleCatalogAdapter) labelPrefix() string {
	if a.resource == ResourceVehicleBrands {
		return "vehicle_brands"
	}
	return "vehicle_models"
}

func (a *VehicleCatalogAdapter) BulkActions() []bulkengine.BulkActionDef {
	p := a.labelPrefix()
	return []bulkengine.BulkActionDef{
		{
			ID: "activate", LabelKey: "bulk.actions." + p + ".activate",
			Permission: rbac.PermVehicleCatalogWrite, Reversible: true,
		},
		{
			ID: "deactivate", LabelKey: "bulk.actions." + p + ".deactivate",
			Permission: rbac.PermVehicleCatalogWrite, Destructive: true, Reversible: true,
			ConfirmKey: "bulk.confirm." + p + ".deactivate",
		},
	}
}

func (a *VehicleCatalogAdapter) ResolveTargets(_ context.Context, _ string, target bulkengine.BulkTarget) ([]string, error) {
	return idsOnly(target)
}

// active reads the live active flag of one row.
func (a *VehicleCatalogAdapter) active(ctx context.Context, id uuid.UUID) (bool, error) {
	if a.resource == ResourceVehicleBrands {
		b, err := a.q.GetCarBrandByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, bulkengine.ErrEntityGone
		}
		return b.Active, err
	}
	m, err := a.q.GetCarModelByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, bulkengine.ErrEntityGone
	}
	return m.Active, err
}

func (a *VehicleCatalogAdapter) setActive(ctx context.Context, id uuid.UUID, active bool) error {
	var err error
	if a.resource == ResourceVehicleBrands {
		_, err = a.q.SetCarBrandActiveByUUID(ctx, db.SetCarBrandActiveByUUIDParams{Active: active, Uuid: id})
	} else {
		_, err = a.q.SetCarModelActiveByUUID(ctx, db.SetCarModelActiveByUUIDParams{Active: active, Uuid: id})
	}
	return err
}

func (a *VehicleCatalogAdapter) entityType() string {
	if a.resource == ResourceVehicleBrands {
		return "car_brand"
	}
	return "car_model"
}

func (a *VehicleCatalogAdapter) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	var want bool
	switch action {
	case "activate":
		want = true
	case "deactivate":
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	cur, err := a.active(ctx, id)
	if errors.Is(err, bulkengine.ErrEntityGone) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res := bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: a.entityType(), OK: true, Op: "update",
		Previous: map[string]any{"active": cur},
		Applied:  map[string]any{"active": want},
	}
	if cur == want {
		return res, nil
	}
	if err := a.setActive(ctx, id, want); err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
	}
	return res, nil
}

func (a *VehicleCatalogAdapter) CurrentState(ctx context.Context, _ string, entityUUID string) (map[string]any, error) {
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return nil, bulkengine.ErrEntityGone
	}
	cur, err := a.active(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"active": cur}, nil
}

func (a *VehicleCatalogAdapter) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	prev, ok := previous["active"].(bool)
	if !ok {
		return fmt.Errorf("missing %s snapshot", a.entityType())
	}
	return a.setActive(ctx, id, prev)
}
