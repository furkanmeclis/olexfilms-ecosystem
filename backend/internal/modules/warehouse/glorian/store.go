package glorian

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Store errors (TEC-266).
var (
	// ErrConnectionKeyTaken: the brand already has a connection with this
	// key (uq_integration_connections_brand_key). HTTP 409.
	ErrConnectionKeyTaken = errors.New("glorian: connection key already exists for this brand")
	// ErrConnectionInUse: rows (e.g. order outbounds) still reference the
	// connection. HTTP 409.
	ErrConnectionInUse = errors.New("glorian: connection is still in use")
	// ErrAmbiguousConnection: more than one brand has a connection with the
	// key, so a key-only lookup cannot pick one (no silent fallback).
	ErrAmbiguousConnection = errors.New("glorian: more than one connection has this key")
)

// Error codes for the HTTP layer.
const (
	CodeConnectionKeyTaken = "INTEGRATION_CONNECTION_EXISTS"
	CodeConnectionInUse    = "INTEGRATION_CONNECTION_IN_USE"
)

// HTTPStatus maps a Store error to its HTTP status and error code; ok is
// false for errors without a dedicated mapping (500).
func HTTPStatus(err error) (status int, code string, ok bool) {
	switch {
	case errors.Is(err, ErrConnectionKeyTaken):
		return http.StatusConflict, CodeConnectionKeyTaken, true
	case errors.Is(err, ErrConnectionInUse):
		return http.StatusConflict, CodeConnectionInUse, true
	case errors.Is(err, ErrConnectionNotFound):
		return http.StatusNotFound, CodeNotFound, true
	}
	return 0, "", false
}

const constraintBrandKey = "uq_integration_connections_brand_key"

// SecretBox encrypts connection API keys at rest (crypto.SecretBox with
// APP_ENCRYPTION_KEY). The plain key never reaches the database.
type SecretBox interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(encoded string) (string, error)
}

// Store is the DB-backed integration_connections repository and the
// ConnectionSource used instead of StaticSource once a connection row
// exists.
type Store struct {
	q   db.Querier
	box SecretBox
}

var _ ConnectionSource = (*Store)(nil)

// NewStore wires a store.
func NewStore(q db.Querier, box SecretBox) *Store {
	return &Store{q: q, box: box}
}

// ConnectionInput creates a connection. APIKey is the plain key; it is
// encrypted before the insert.
type ConnectionInput struct {
	OrganizationID     int64
	BrandID            int64
	Key                string
	BaseURL            string
	APIKey             string
	DefaultWarehouseID *int64
	Active             bool
	APIVersion         string
}

// ConnectionUpdate changes the non-secret fields of a connection.
type ConnectionUpdate struct {
	BaseURL            string
	DefaultWarehouseID *int64
	Active             bool
	APIVersion         string
}

// Create inserts a connection with an encrypted API key.
func (s *Store) Create(ctx context.Context, in ConnectionInput) (db.IntegrationConnection, error) {
	enc, err := s.encrypt(in.APIKey)
	if err != nil {
		return db.IntegrationConnection{}, err
	}
	version := strings.TrimSpace(in.APIVersion)
	if version == "" {
		version = APIVersion
	}
	row, err := s.q.CreateIntegrationConnection(ctx, db.CreateIntegrationConnectionParams{
		OrganizationID:     in.OrganizationID,
		BrandID:            in.BrandID,
		Key:                strings.TrimSpace(in.Key),
		BaseUrl:            strings.TrimSpace(in.BaseURL),
		ApiKeyEnc:          enc,
		DefaultWarehouseID: optInt8(in.DefaultWarehouseID),
		Active:             in.Active,
		ApiVersion:         version,
	})
	if err != nil {
		return db.IntegrationConnection{}, mapStoreError(err)
	}
	return row, nil
}

// Get returns the connection of the brand by uuid.
func (s *Store) Get(ctx context.Context, brandID int64, id uuid.UUID) (db.IntegrationConnection, error) {
	row, err := s.q.GetIntegrationConnectionByUUID(ctx, db.GetIntegrationConnectionByUUIDParams{Uuid: id, BrandID: brandID})
	return row, mapStoreError(err)
}

// List returns the connections of the brand.
func (s *Store) List(ctx context.Context, brandID int64) ([]db.IntegrationConnection, error) {
	return s.q.ListIntegrationConnections(ctx, brandID)
}

// Update changes the non-secret fields.
func (s *Store) Update(ctx context.Context, id int64, in ConnectionUpdate) (db.IntegrationConnection, error) {
	version := strings.TrimSpace(in.APIVersion)
	if version == "" {
		version = APIVersion
	}
	row, err := s.q.UpdateIntegrationConnection(ctx, db.UpdateIntegrationConnectionParams{
		ID:                 id,
		BaseUrl:            strings.TrimSpace(in.BaseURL),
		DefaultWarehouseID: optInt8(in.DefaultWarehouseID),
		Active:             in.Active,
		ApiVersion:         version,
	})
	return row, mapStoreError(err)
}

// SetAPIKey replaces the API key (encrypted).
func (s *Store) SetAPIKey(ctx context.Context, id int64, apiKey string) (db.IntegrationConnection, error) {
	enc, err := s.encrypt(apiKey)
	if err != nil {
		return db.IntegrationConnection{}, err
	}
	row, err := s.q.SetIntegrationConnectionAPIKey(ctx, db.SetIntegrationConnectionAPIKeyParams{ID: id, ApiKeyEnc: enc})
	return row, mapStoreError(err)
}

// Delete removes a connection; its maps, runs and parties go with it, an
// outbound order keeps it (ErrConnectionInUse).
func (s *Store) Delete(ctx context.Context, id int64) error {
	n, err := s.q.DeleteIntegrationConnection(ctx, id)
	if err != nil {
		return mapStoreError(err)
	}
	if n == 0 {
		return ErrConnectionNotFound
	}
	return nil
}

// Decrypt turns a row into a client Connection with the plain API key.
func (s *Store) Decrypt(row db.IntegrationConnection) (Connection, error) {
	key, err := s.box.Decrypt(row.ApiKeyEnc)
	if err != nil {
		return Connection{}, fmt.Errorf("glorian: decrypt api key of connection %s: %w", row.Uuid, err)
	}
	return Connection{
		ID:      row.Uuid.String(),
		Key:     row.Key,
		BaseURL: row.BaseUrl,
		APIKey:  key,
		Active:  row.Active,
	}, nil
}

// ConnectionForProduct implements ConnectionSource: the glorian connection
// of the product's brand.
func (s *Store) ConnectionForProduct(ctx context.Context, productID uuid.UUID) (Connection, error) {
	row, err := s.q.GetIntegrationConnectionForProduct(ctx, db.GetIntegrationConnectionForProductParams{
		ProductUuid: productID, Key: ConnectionKey,
	})
	if err != nil {
		return Connection{}, mapStoreError(err)
	}
	return s.Decrypt(row)
}

// ConnectionByKey implements ConnectionSource. The key is unique per brand;
// a key held by several brands is ambiguous and refused.
func (s *Store) ConnectionByKey(ctx context.Context, key string) (Connection, error) {
	rows, err := s.q.ListIntegrationConnectionsByKey(ctx, key)
	if err != nil {
		return Connection{}, err
	}
	switch len(rows) {
	case 0:
		return Connection{}, ErrConnectionNotFound
	case 1:
		return s.Decrypt(rows[0])
	default:
		return Connection{}, fmt.Errorf("%w: %q", ErrAmbiguousConnection, key)
	}
}

func (s *Store) encrypt(apiKey string) (string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", nil
	}
	enc, err := s.box.Encrypt(apiKey)
	if err != nil {
		return "", fmt.Errorf("glorian: encrypt api key: %w", err)
	}
	return enc, nil
}

func optInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

// mapStoreError turns constraint violations into store errors.
func mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConnectionNotFound
	}
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return err
	}
	switch pg.Code {
	case "23505":
		if pg.ConstraintName == constraintBrandKey {
			return fmt.Errorf("%w: %s", ErrConnectionKeyTaken, pg.Detail)
		}
	case "23001", "23503":
		// ON DELETE RESTRICT answers 23001 on PostgreSQL 18, 23503 before.
		// An insert/update pointing at a missing warehouse also lands here;
		// only a delete can be "in use".
		if strings.HasPrefix(pg.ConstraintName, "fk_order_outbounds_") {
			return ErrConnectionInUse
		}
	}
	return err
}
