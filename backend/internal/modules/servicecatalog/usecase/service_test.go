package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	pricing "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	brandID    = int64(1)
	centerID   = int64(10)
	distNLID   = int64(20)
	distBEID   = int64(21)
	dealerNLID = int64(30)
	dealerBEID = int64(31)
	itemID     = int64(100)
)

type fakeStore struct {
	Store
	items         map[uuid.UUID]db.ServiceCatalogItem
	overrides     map[int64]db.ServicePriceOverride
	subscriptions int64
	modules       []string
}

func num(s string) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		panic(err)
	}
	return n
}

func catalogItem(cat string) db.ServiceCatalogItem {
	return db.ServiceCatalogItem{
		ID: itemID, Uuid: uuid.New(), OrganizationID: centerID, BrandID: brandID,
		Name: "Training", Category: cat, DefaultPrice: num("100.00"), Currency: "EUR",
		Recurrence: "monthly", CancellationFee: num("0"), IsActive: true,
	}
}

func (f *fakeStore) ListServiceCatalogItems(_ context.Context, arg db.ListServiceCatalogItemsParams) ([]db.ServiceCatalogItem, error) {
	out := []db.ServiceCatalogItem{}
	for _, item := range f.items {
		if item.BrandID == arg.BrandID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (f *fakeStore) GetServiceCatalogItemByUUID(_ context.Context, arg db.GetServiceCatalogItemByUUIDParams) (db.ServiceCatalogItem, error) {
	item, ok := f.items[arg.Uuid]
	if !ok || item.BrandID != arg.BrandID {
		return db.ServiceCatalogItem{}, pgx.ErrNoRows
	}
	return item, nil
}

func (f *fakeStore) UpdateServiceCatalogItem(_ context.Context, arg db.UpdateServiceCatalogItemParams) (db.ServiceCatalogItem, error) {
	for id, item := range f.items {
		if item.ID == arg.ID && item.BrandID == arg.BrandID {
			item.IsActive = arg.IsActive
			item.Name = arg.Name
			item.Description = arg.Description
			item.Category = arg.Category
			item.DefaultPrice = arg.DefaultPrice
			item.Currency = arg.Currency
			item.Recurrence = arg.Recurrence
			item.CancellationFee = arg.CancellationFee
			f.items[id] = item
			return item, nil
		}
	}
	return db.ServiceCatalogItem{}, pgx.ErrNoRows
}

func (f *fakeStore) CountServiceSubscriptionsByItem(_ context.Context, _ db.CountServiceSubscriptionsByItemParams) (int64, error) {
	return f.subscriptions, nil
}

func (f *fakeStore) GetServicePriceOverride(_ context.Context, arg db.GetServicePriceOverrideParams) (db.ServicePriceOverride, error) {
	ov, ok := f.overrides[arg.OrganizationID]
	if !ok || ov.ItemID != arg.ItemID {
		return db.ServicePriceOverride{}, pgx.ErrNoRows
	}
	return ov, nil
}

func (f *fakeStore) SupplierOf(_ context.Context, id int64) (db.Organization, error) {
	switch id {
	case dealerNLID:
		return db.Organization{ID: distNLID, Type: pricing.OrgDistributor, BrandID: brandID}, nil
	case dealerBEID:
		return db.Organization{ID: distBEID, Type: pricing.OrgDistributor, BrandID: brandID}, nil
	default:
		return db.Organization{}, pgx.ErrNoRows
	}
}

func (f *fakeStore) DeleteServiceCatalogModules(_ context.Context, _ int64) (int64, error) {
	f.modules = nil
	return 0, nil
}

func (f *fakeStore) AddServiceCatalogModule(_ context.Context, arg db.AddServiceCatalogModuleParams) error {
	f.modules = append(f.modules, arg.ModuleKey)
	return nil
}

func (f *fakeStore) ListServiceCatalogModules(_ context.Context, _ int64) ([]string, error) {
	return f.modules, nil
}

func TestResolvePriceDistributorOverrideInheritedByDealer(t *testing.T) {
	item := catalogItem("training")
	store := &fakeStore{
		items: map[uuid.UUID]db.ServiceCatalogItem{item.Uuid: item},
		overrides: map[int64]db.ServicePriceOverride{
			distNLID: {ItemID: item.ID, OrganizationID: distNLID, BrandID: brandID, Price: num("80.00"), Currency: "EUR"},
		},
	}
	svc := New(store)
	ctx := context.Background()

	distPrice, err := svc.ResolvePrice(ctx, item, db.Organization{ID: distNLID, Type: pricing.OrgDistributor, BrandID: brandID})
	if err != nil || distPrice.Amount != "80.00" || distPrice.Source != "override" {
		t.Fatalf("distributor price = %+v, %v", distPrice, err)
	}
	dealerPrice, err := svc.ResolvePrice(ctx, item, db.Organization{ID: dealerNLID, Type: pricing.OrgDealer, BrandID: brandID, ParentID: pgtype.Int8{Int64: distNLID, Valid: true}})
	if err != nil || dealerPrice.Amount != "80.00" || dealerPrice.Source != "override" {
		t.Fatalf("dealer price = %+v, %v", dealerPrice, err)
	}
	otherPrice, err := svc.ResolvePrice(ctx, item, db.Organization{ID: dealerBEID, Type: pricing.OrgDealer, BrandID: brandID, ParentID: pgtype.Int8{Int64: distBEID, Valid: true}})
	if err != nil || otherPrice.Amount != "100.00" || otherPrice.Source != "default" {
		t.Fatalf("other dealer price = %+v, %v", otherPrice, err)
	}

	delete(store.overrides, distNLID)
	fallback, err := svc.ResolvePrice(ctx, item, db.Organization{ID: distNLID, Type: pricing.OrgDistributor, BrandID: brandID})
	if err != nil || fallback.Amount != "100.00" || fallback.Source != "default" {
		t.Fatalf("fallback price = %+v, %v", fallback, err)
	}
}

func TestDeleteConflictAndDeactivate(t *testing.T) {
	item := catalogItem("training")
	store := &fakeStore{items: map[uuid.UUID]db.ServiceCatalogItem{item.Uuid: item}, subscriptions: 1}
	svc := New(store)
	center := orgctx.Scope{InternalID: centerID, OrgType: pricing.OrgCenter, BrandID: brandID}
	if _, err := svc.Delete(context.Background(), center, item.Uuid); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete with subscriptions = %v, want ErrInUse", err)
	}
	store.subscriptions = 0
	got, err := svc.Delete(context.Background(), center, item.Uuid)
	if err != nil || got.IsActive {
		t.Fatalf("delete/deactivate = %+v, %v", got, err)
	}
}

func TestSetModulesNeedsModuleBundle(t *testing.T) {
	item := catalogItem("training")
	store := &fakeStore{items: map[uuid.UUID]db.ServiceCatalogItem{item.Uuid: item}}
	svc := New(store)
	center := orgctx.Scope{InternalID: centerID, OrgType: pricing.OrgCenter, BrandID: brandID}
	if _, err := svc.SetModules(context.Background(), center, item.Uuid, []string{"ai"}); !errors.Is(err, ErrModuleOnly) {
		t.Fatalf("set modules = %v, want ErrModuleOnly", err)
	}
}
