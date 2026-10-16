package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-769 against a migrated PostgreSQL (TEST_DATABASE_URL), in a
// rolled-back transaction: the Glorian location map CRUD (remote location
// -> warehouse + optional bin of the connection's organization), its
// uniqueness rules, list search / sort and the resolver.
func TestLocationMapCRUD(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	box, err := crypto.NewSecretBox("test-encryption-key-32-bytes!!!!")
	if err != nil {
		t.Fatal(err)
	}
	brand, err := q.GetBrandBySlug(ctx, "glorian")
	if err != nil {
		t.Fatalf("glorian brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("glorian center: %v", err)
	}
	svc := usecase.New(q, box, nil, nil, nil)
	// sp runs a call that may hit a constraint in a savepoint, so the
	// failure does not abort the fixture transaction.
	sp := func(fn func(s *usecase.Service) error) error {
		t.Helper()
		stx, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := fn(usecase.New(db.New(stx), box, nil, nil, nil)); err != nil {
			_ = stx.Rollback(ctx)
			return err
		}
		return stx.Commit(ctx)
	}

	// No connection yet: 404.
	if _, _, err := svc.ListLocationMaps(ctx, brand, usecase.LocationMapFilter{}); !errors.Is(err, usecase.ErrNotConfigured) {
		t.Fatalf("list without connection = %v", err)
	}
	conn, err := glorian.NewStore(q, box).Create(ctx, glorian.ConnectionInput{
		OrganizationID: center.ID, BrandID: brand.ID, Key: glorian.ConnectionKey, BaseURL: "https://hub.example.test",
	})
	if err != nil {
		t.Fatalf("connection: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000)
	tree := wh.New(tx, q)
	c := wh.Caller{
		Org:    orgctx.Scope{InternalID: center.ID, UUID: center.Uuid, OrgType: center.Type, BrandID: brand.ID, BrandSlug: brand.Slug},
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: []int64{center.ID}, OrgID: center.ID},
	}
	newWarehouse := func(code string) (wh.Warehouse, wh.Location) {
		t.Helper()
		w, err := tree.CreateWarehouse(ctx, c, wh.WarehouseInput{Code: code + suffix, Name: "t769 " + code})
		if err != nil {
			t.Fatalf("warehouse: %v", err)
		}
		r, err := tree.CreateRoom(ctx, c, w.UUID, wh.RoomInput{Code: "R1", Name: "room"})
		if err != nil {
			t.Fatalf("room: %v", err)
		}
		l, err := tree.CreateLocation(ctx, c, wh.LocationInput{RoomUUID: r.UUID, Type: wh.TypeAisle, Code: "A1", Name: "aisle"})
		if err != nil {
			t.Fatalf("location: %v", err)
		}
		return w, l
	}
	w1, l1 := newWarehouse("GA")
	w2, _ := newWarehouse("GB")

	// Picker options.
	whs, err := svc.MapWarehouses(ctx, brand)
	if err != nil || len(whs) < 2 {
		t.Fatalf("warehouses = %+v, %v", whs, err)
	}
	locs, err := svc.MapLocations(ctx, brand, w1.UUID, "A1")
	if err != nil || len(locs) != 1 || locs[0].UUID != l1.UUID {
		t.Fatalf("locations = %+v, %v", locs, err)
	}
	if _, err := svc.MapLocations(ctx, brand, uuid.New(), ""); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("locations of unknown warehouse = %v", err)
	}

	// Create: warehouse only, then warehouse + bin.
	shelf, err := svc.CreateLocationMap(ctx, brand, usecase.LocationMapInput{RemoteLocationCode: " Raf-1 ", WarehouseUUID: w1.UUID.String(), LocationUUID: &l1.UUID})
	if err != nil {
		t.Fatalf("create with bin: %v", err)
	}
	if shelf.RemoteLocationCode != "Raf-1" || shelf.Warehouse.UUID != w1.UUID || shelf.Location == nil || shelf.Location.UUID != l1.UUID {
		t.Fatalf("created = %+v", shelf)
	}
	main, err := svc.CreateLocationMap(ctx, brand, usecase.LocationMapInput{RemoteLocationCode: "Ana Depo", WarehouseUUID: w2.UUID.String()})
	if err != nil || main.Location != nil {
		t.Fatalf("create warehouse only = %+v, %v", main, err)
	}

	// Rules: remote unique, bin unique, bin must be of the warehouse,
	// required fields.
	create := func(in usecase.LocationMapInput) error {
		return sp(func(s *usecase.Service) error { _, err := s.CreateLocationMap(ctx, brand, in); return err })
	}
	if err := create(usecase.LocationMapInput{RemoteLocationCode: "Raf-1", WarehouseUUID: w2.UUID.String()}); !errors.Is(err, usecase.ErrRemoteTaken) {
		t.Fatalf("duplicate remote = %v", err)
	}
	var ve *usecase.ValidationError
	if err := create(usecase.LocationMapInput{RemoteLocationCode: "Raf-2", WarehouseUUID: w1.UUID.String(), LocationUUID: &l1.UUID}); !errors.As(err, &ve) || ve.Fields["location_uuid"] == "" {
		t.Fatalf("bin mapped twice = %v", err)
	}
	if err := create(usecase.LocationMapInput{RemoteLocationCode: "Raf-3", WarehouseUUID: w2.UUID.String(), LocationUUID: &l1.UUID}); !errors.As(err, &ve) || ve.Fields["location_uuid"] == "" {
		t.Fatalf("bin of another warehouse = %v", err)
	}
	if err := create(usecase.LocationMapInput{RemoteLocationCode: " ", WarehouseUUID: w2.UUID.String()}); !errors.As(err, &ve) || ve.Fields["remote_location_code"] == "" {
		t.Fatalf("empty remote = %v", err)
	}
	if err := create(usecase.LocationMapInput{RemoteLocationCode: "X", WarehouseUUID: uuid.NewString()}); !errors.As(err, &ve) || ve.Fields["warehouse_uuid"] == "" {
		t.Fatalf("unknown warehouse = %v", err)
	}

	// List: search, sort, page.
	items, total, err := svc.ListLocationMaps(ctx, brand, usecase.LocationMapFilter{SortKey: "remote_location_code", Limit: 10})
	if err != nil || total != 2 || len(items) != 2 || items[0].UUID != main.UUID || items[1].UUID != shelf.UUID {
		t.Fatalf("list = %+v total %d, %v", items, total, err)
	}
	if items[1].Location == nil || items[1].Location.UUID != l1.UUID {
		t.Fatalf("list bin = %+v", items[1])
	}
	items, total, err = svc.ListLocationMaps(ctx, brand, usecase.LocationMapFilter{Q: "raf", SortKey: "remote_location_code", Limit: 10})
	if err != nil || total != 1 || items[0].UUID != shelf.UUID {
		t.Fatalf("search = %+v total %d, %v", items, total, err)
	}
	items, _, err = svc.ListLocationMaps(ctx, brand, usecase.LocationMapFilter{SortKey: "remote_location_code", SortDesc: true, Limit: 1, Offset: 1})
	if err != nil || len(items) != 1 || items[0].UUID != main.UUID {
		t.Fatalf("page = %+v, %v", items, err)
	}

	// Resolve.
	whID, locID, ok, err := svc.ResolveRemoteLocation(ctx, conn.ID, "Raf-1")
	if err != nil || !ok || whID == 0 || locID == nil {
		t.Fatalf("resolve = %d %v %v %v", whID, locID, ok, err)
	}
	if _, _, ok, err := svc.ResolveRemoteLocation(ctx, conn.ID, "Yok"); err != nil || ok {
		t.Fatalf("resolve unknown = %v %v", ok, err)
	}

	// Update: move the shelf to the other warehouse without a bin.
	upd, err := svc.UpdateLocationMap(ctx, brand, shelf.UUID, usecase.LocationMapInput{RemoteLocationCode: "Raf-1B", WarehouseUUID: w2.UUID.String()})
	if err != nil || upd.RemoteLocationCode != "Raf-1B" || upd.Warehouse.UUID != w2.UUID || upd.Location != nil {
		t.Fatalf("update = %+v, %v", upd, err)
	}
	if _, err := svc.UpdateLocationMap(ctx, brand, uuid.New(), usecase.LocationMapInput{RemoteLocationCode: "Z", WarehouseUUID: w2.UUID.String()}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("update unknown = %v", err)
	}

	// Delete.
	if err := svc.DeleteLocationMap(ctx, brand, shelf.UUID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.DeleteLocationMap(ctx, brand, shelf.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("delete twice = %v", err)
	}
	if _, total, _ := svc.ListLocationMaps(ctx, brand, usecase.LocationMapFilter{Limit: 10}); total != 1 {
		t.Fatalf("after delete total = %d", total)
	}
}
