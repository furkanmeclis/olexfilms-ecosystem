package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-1265 against a migrated PostgreSQL (TEST_DATABASE_URL), in a
// rolled-back transaction: the first package sets the key, the next one
// waits as the pending key until the hub accepts it.
func TestCreatePackageAndRotation(t *testing.T) {
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
	svc := usecase.New(q, box, nil, nil, nil)
	store := glorian.NewStore(q, box)

	in := usecase.PackageInput{BaseURL: "https://glorian.example.test", Password: "uzun-bir-sahte-sifre"}

	// Short password: 400, nothing created.
	var verr *usecase.ValidationError
	if _, err := svc.CreatePackage(ctx, brand, usecase.PackageInput{BaseURL: in.BaseURL, Password: "kisa"}, "https://eco.example.test"); !errors.As(err, &verr) {
		t.Fatalf("short password = %v", err)
	}

	first, err := svc.CreatePackage(ctx, brand, in, "https://eco.example.test")
	if err != nil {
		t.Fatalf("first package: %v", err)
	}
	if first.Rotated || !first.Connection.APIKeySet || first.Connection.PendingKey || first.Connection.Active {
		t.Fatalf("first = %+v", first.Connection)
	}
	if strings.Contains(string(first.Package), "api_key") {
		t.Fatal("envelope must not carry a plain api_key field")
	}
	var env map[string]any
	if err := json.Unmarshal(first.Package, &env); err != nil || env["format"] != usecase.PackageFormat {
		t.Fatalf("envelope = %s (%v)", first.Package, err)
	}
	row, err := q.GetIntegrationConnectionByKey(ctx, db.GetIntegrationConnectionByKeyParams{BrandID: brand.ID, Key: glorian.ConnectionKey})
	if err != nil {
		t.Fatal(err)
	}
	conn1, err := store.Decrypt(row)
	if err != nil {
		t.Fatal(err)
	}
	if usecase.KeyFingerprint(conn1.APIKey) != first.KeyFingerprint || conn1.PendingAPIKey != "" {
		t.Fatalf("first key not active: %+v", conn1)
	}

	second, err := svc.CreatePackage(ctx, brand, in, "https://eco.example.test")
	if err != nil {
		t.Fatalf("second package: %v", err)
	}
	if !second.Rotated || !second.Connection.PendingKey {
		t.Fatalf("second = %+v", second)
	}
	row, _ = q.GetIntegrationConnectionByKey(ctx, db.GetIntegrationConnectionByKeyParams{BrandID: brand.ID, Key: glorian.ConnectionKey})
	conn2, err := store.Decrypt(row)
	if err != nil {
		t.Fatal(err)
	}
	if conn2.APIKey != conn1.APIKey || usecase.KeyFingerprint(conn2.PendingAPIKey) != second.KeyFingerprint || conn2.PendingAccepted == nil {
		t.Fatalf("old key must stay active until the hub accepts the new one")
	}

	// The hub accepted the pending key.
	if err := conn2.PendingAccepted(ctx); err != nil {
		t.Fatal(err)
	}
	row, _ = q.GetIntegrationConnectionByKey(ctx, db.GetIntegrationConnectionByKeyParams{BrandID: brand.ID, Key: glorian.ConnectionKey})
	conn3, _ := store.Decrypt(row)
	if conn3.APIKey != conn2.PendingAPIKey || conn3.PendingAPIKey != "" {
		t.Fatalf("pending key not promoted")
	}

	// A stale promote (a newer package was issued meanwhile) changes nothing.
	third, err := svc.CreatePackage(ctx, brand, in, "https://eco.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := conn2.PendingAccepted(ctx); err != nil {
		t.Fatal(err)
	}
	row, _ = q.GetIntegrationConnectionByKey(ctx, db.GetIntegrationConnectionByKeyParams{BrandID: brand.ID, Key: glorian.ConnectionKey})
	conn4, _ := store.Decrypt(row)
	if usecase.KeyFingerprint(conn4.PendingAPIKey) != third.KeyFingerprint || conn4.APIKey != conn3.APIKey {
		t.Fatalf("stale promote overwrote the newer pending key")
	}

	// A key typed in by hand drops the pending one.
	manual := "elle-girilen-sahte-anahtar-1234567890"
	if _, _, err := svc.Put(ctx, brand, usecase.PutInput{BaseURL: in.BaseURL, APIKey: &manual}); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Get(ctx, brand)
	if err != nil || v.PendingKey {
		t.Fatalf("manual key: %+v %v", v, err)
	}
}
