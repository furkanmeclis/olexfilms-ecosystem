package glorian_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian/fake"
	"github.com/google/uuid"
)

// mapSource is a ConnectionSource keyed by product id.
type mapSource struct {
	byProduct map[uuid.UUID]glorian.Connection
	err       error
}

func (m mapSource) ConnectionForProduct(_ context.Context, id uuid.UUID) (glorian.Connection, error) {
	if m.err != nil {
		return glorian.Connection{}, m.err
	}
	c, ok := m.byProduct[id]
	if !ok {
		return glorian.Connection{}, glorian.ErrConnectionNotFound
	}
	return c, nil
}

func (m mapSource) ConnectionByKey(_ context.Context, key string) (glorian.Connection, error) {
	for _, c := range m.byProduct {
		if c.Key == key {
			return c, nil
		}
	}
	return glorian.Connection{}, glorian.ErrConnectionNotFound
}

func TestResolver(t *testing.T) {
	srv := fake.New(t)
	active := glorian.Connection{ID: "1", Key: "glorian", BaseURL: srv.URL, APIKey: fake.APIKey, Active: true}
	inactive := active
	inactive.Active = false
	noKey := active
	noKey.APIKey = ""

	pActive, pInactive, pNoKey, pMissing := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	src := mapSource{byProduct: map[uuid.UUID]glorian.Connection{pActive: active, pInactive: inactive, pNoKey: noKey}}
	factoryCalls := 0
	base := glorian.HTTPClientFactory(glorian.Options{HTTPClient: srv.Client(), Clock: newClock()})
	r := glorian.NewClientResolver(src, func(c glorian.Connection) (glorian.InventoryClient, error) {
		factoryCalls++
		return base(c)
	})
	ctx := context.Background()

	for name, id := range map[string]uuid.UUID{"inactive": pInactive, "missing api key": pNoKey, "no connection": pMissing} {
		if _, err := r.ForProduct(ctx, id); !errors.Is(err, glorian.ErrInactiveConnection) {
			t.Errorf("%s: err = %v, want ErrInactiveConnection", name, err)
		}
	}
	if factoryCalls != 0 {
		t.Errorf("factory called %d times for unusable connections", factoryCalls)
	}

	c, err := r.ForProduct(ctx, pActive)
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if _, err := c.ListCategories(ctx, glorian.ListParams{}); err != nil {
		t.Errorf("resolved client call: %v", err)
	}

	if _, err := r.ForKey(ctx, "olex"); !errors.Is(err, glorian.ErrInactiveConnection) {
		t.Errorf("unknown key err = %v", err)
	}

	// A lookup failure other than not-found is surfaced as is, not masked.
	boom := errors.New("db down")
	r2 := glorian.NewClientResolver(mapSource{err: boom}, base)
	if _, err := r2.ForProduct(ctx, pActive); !errors.Is(err, boom) || errors.Is(err, glorian.ErrInactiveConnection) {
		t.Errorf("lookup failure err = %v", err)
	}
}

func TestResolver_EnvConnectionIsInactiveUntilEnabled(t *testing.T) {
	cfg := config.GlorianConfig{BaseURL: "https://hub.example.test", APIKey: "placeholder-key"}
	factory := glorian.HTTPClientFactory(glorian.OptionsFromConfig(cfg))

	r := glorian.NewClientResolver(glorian.StaticSource{Conn: glorian.ConnectionFromConfig(cfg)}, factory)
	if _, err := r.ForKey(context.Background(), glorian.ConnectionKey); !errors.Is(err, glorian.ErrInactiveConnection) {
		t.Fatalf("disabled env connection err = %v", err)
	}

	cfg.Enabled = true
	r = glorian.NewClientResolver(glorian.StaticSource{Conn: glorian.ConnectionFromConfig(cfg)}, factory)
	if _, err := r.ForProduct(context.Background(), uuid.New()); err != nil {
		t.Fatalf("enabled env connection: %v", err)
	}

	empty := glorian.NewClientResolver(glorian.StaticSource{}, factory)
	if _, err := empty.ForProduct(context.Background(), uuid.New()); !errors.Is(err, glorian.ErrInactiveConnection) {
		t.Fatalf("empty source err = %v", err)
	}
}
