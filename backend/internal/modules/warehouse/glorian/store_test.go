package glorian

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeQuerier implements only the queries the store calls; any other call
// panics through the nil embedded interface.
type fakeQuerier struct {
	db.Querier
	created  db.CreateIntegrationConnectionParams
	byKey    []db.IntegrationConnection
	createFn func(db.CreateIntegrationConnectionParams) (db.IntegrationConnection, error)
}

func (f *fakeQuerier) CreateIntegrationConnection(_ context.Context, arg db.CreateIntegrationConnectionParams) (db.IntegrationConnection, error) {
	f.created = arg
	if f.createFn != nil {
		return f.createFn(arg)
	}
	return db.IntegrationConnection{
		ID: 1, Uuid: uuid.New(), OrganizationID: arg.OrganizationID, BrandID: arg.BrandID,
		Key: arg.Key, BaseUrl: arg.BaseUrl, ApiKeyEnc: arg.ApiKeyEnc, Active: arg.Active, ApiVersion: arg.ApiVersion,
	}, nil
}

func (f *fakeQuerier) ListIntegrationConnectionsByKey(context.Context, string) ([]db.IntegrationConnection, error) {
	return f.byKey, nil
}

func testBox(t *testing.T) *crypto.SecretBox {
	t.Helper()
	box, err := crypto.NewSecretBox("test-encryption-key-32-bytes!!!!")
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestStoreCreateEncryptsAPIKey(t *testing.T) {
	q := &fakeQuerier{}
	s := NewStore(q, testBox(t))
	row, err := s.Create(context.Background(), ConnectionInput{
		OrganizationID: 1, BrandID: 2, Key: ConnectionKey, BaseURL: "https://hub.example",
		APIKey: "plain-secret-key", Active: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if q.created.ApiKeyEnc == "" || q.created.ApiKeyEnc == "plain-secret-key" {
		t.Fatalf("api key stored as %q; want ciphertext", q.created.ApiKeyEnc)
	}
	if q.created.ApiVersion != APIVersion {
		t.Fatalf("api version = %q; want default %q", q.created.ApiVersion, APIVersion)
	}
	conn, err := s.Decrypt(row)
	if err != nil {
		t.Fatal(err)
	}
	if conn.APIKey != "plain-secret-key" || conn.Key != ConnectionKey || !conn.usable() {
		t.Fatalf("decrypted connection = %+v", conn)
	}
}

func TestStoreCreateMapsDuplicateKeyTo409(t *testing.T) {
	q := &fakeQuerier{createFn: func(db.CreateIntegrationConnectionParams) (db.IntegrationConnection, error) {
		return db.IntegrationConnection{}, &pgconn.PgError{Code: "23505", ConstraintName: constraintBrandKey}
	}}
	_, err := NewStore(q, testBox(t)).Create(context.Background(), ConnectionInput{
		OrganizationID: 1, BrandID: 2, Key: ConnectionKey, BaseURL: "https://hub.example", APIKey: "k",
	})
	if !errors.Is(err, ErrConnectionKeyTaken) {
		t.Fatalf("err = %v; want ErrConnectionKeyTaken", err)
	}
	status, code, ok := HTTPStatus(err)
	if !ok || status != http.StatusConflict || code != CodeConnectionKeyTaken {
		t.Fatalf("HTTPStatus = %d %q %v; want 409", status, code, ok)
	}
}

func TestMapStoreError(t *testing.T) {
	if err := mapStoreError(pgx.ErrNoRows); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("no rows = %v", err)
	}
	for _, code := range []string{"23001", "23503"} {
		err := mapStoreError(&pgconn.PgError{Code: code, ConstraintName: "fk_order_outbounds_connection"})
		if !errors.Is(err, ErrConnectionInUse) {
			t.Fatalf("%s = %v; want ErrConnectionInUse", code, err)
		}
	}
	other := &pgconn.PgError{Code: "23505", ConstraintName: "uq_integration_connections_uuid"}
	if err := mapStoreError(other); !errors.Is(err, other) || errors.Is(err, ErrConnectionKeyTaken) {
		t.Fatalf("other unique = %v", err)
	}
	if _, _, ok := HTTPStatus(errors.New("boom")); ok {
		t.Fatal("unmapped error must not have a status")
	}
}

func TestStoreConnectionByKey(t *testing.T) {
	box := testBox(t)
	enc, err := box.Encrypt("k1")
	if err != nil {
		t.Fatal(err)
	}
	row := db.IntegrationConnection{Uuid: uuid.New(), Key: ConnectionKey, BaseUrl: "https://hub.example", ApiKeyEnc: enc, Active: true}

	q := &fakeQuerier{}
	s := NewStore(q, box)
	if _, err := s.ConnectionByKey(context.Background(), ConnectionKey); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("empty = %v; want ErrConnectionNotFound", err)
	}

	q.byKey = []db.IntegrationConnection{row}
	conn, err := s.ConnectionByKey(context.Background(), ConnectionKey)
	if err != nil || conn.APIKey != "k1" || conn.ID != row.Uuid.String() {
		t.Fatalf("one = %+v, %v", conn, err)
	}

	q.byKey = []db.IntegrationConnection{row, row}
	if _, err := s.ConnectionByKey(context.Background(), ConnectionKey); !errors.Is(err, ErrAmbiguousConnection) {
		t.Fatalf("two = %v; want ErrAmbiguousConnection", err)
	}

	// The resolver treats a missing connection as inactive.
	q.byKey = nil
	r := NewClientResolver(s, func(Connection) (InventoryClient, error) { return nil, nil })
	if _, err := r.ForKey(context.Background(), ConnectionKey); !errors.Is(err, ErrInactiveConnection) {
		t.Fatalf("resolver = %v; want ErrInactiveConnection", err)
	}
}
