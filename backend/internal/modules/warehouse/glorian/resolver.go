package glorian

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Connection is one Inventory API target (integration_connections row in
// TEC-266; key=glorian). APIKey is the decrypted secret and must never be
// logged.
type Connection struct {
	ID      string
	Key     string
	BaseURL string
	APIKey  string
	Active  bool
}

// usable reports whether the connection may be called at all.
func (c Connection) usable() bool {
	return c.Active && strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.APIKey) != ""
}

// ErrConnectionNotFound is what a ConnectionSource returns when a product
// has no connection. The resolver reports it as ErrInactiveConnection.
var ErrConnectionNotFound = errors.New("glorian: connection not found")

// ConnectionSource looks connections up. The DB-backed implementation
// arrives with the integration_connections schema (TEC-266); until then
// StaticSource serves a single env-configured connection.
type ConnectionSource interface {
	// ConnectionForProduct returns the connection the product syncs with,
	// or ErrConnectionNotFound.
	ConnectionForProduct(ctx context.Context, productID uuid.UUID) (Connection, error)
	// ConnectionByKey returns the connection with the given key (glorian),
	// or ErrConnectionNotFound.
	ConnectionByKey(ctx context.Context, key string) (Connection, error)
}

// ClientFactory builds a client for a usable connection.
type ClientFactory func(Connection) (InventoryClient, error)

// HTTPClientFactory returns a ClientFactory producing HTTPClients that share
// base (timeouts, retry tuning, transport, clock); BaseURL and APIKey come
// from the connection.
func HTTPClientFactory(base Options) ClientFactory {
	return func(conn Connection) (InventoryClient, error) {
		opts := base
		opts.BaseURL = conn.BaseURL
		opts.APIKey = conn.APIKey
		return NewHTTPClient(opts)
	}
}

// ClientResolver picks the client for a product's connection. A missing or
// inactive connection is ErrInactiveConnection; there is no silent fallback
// to a default connection (warehouse InventoryClientResolver lesson).
type ClientResolver struct {
	source  ConnectionSource
	factory ClientFactory
}

// NewClientResolver wires a resolver.
func NewClientResolver(source ConnectionSource, factory ClientFactory) *ClientResolver {
	return &ClientResolver{source: source, factory: factory}
}

// ForProduct resolves the client of the product's connection.
func (r *ClientResolver) ForProduct(ctx context.Context, productID uuid.UUID) (InventoryClient, error) {
	conn, err := r.source.ConnectionForProduct(ctx, productID)
	if err != nil {
		return nil, r.lookupErr(err, "product "+productID.String())
	}
	return r.ForConnection(conn)
}

// ForKey resolves the client of the connection with key (e.g. "glorian").
func (r *ClientResolver) ForKey(ctx context.Context, key string) (InventoryClient, error) {
	conn, err := r.source.ConnectionByKey(ctx, key)
	if err != nil {
		return nil, r.lookupErr(err, "key "+key)
	}
	return r.ForConnection(conn)
}

// ForConnection builds a client for conn, refusing an unusable one.
func (r *ClientResolver) ForConnection(conn Connection) (InventoryClient, error) {
	if !conn.usable() {
		return nil, fmt.Errorf("%w: connection %q", ErrInactiveConnection, conn.Key)
	}
	return r.factory(conn)
}

func (r *ClientResolver) lookupErr(err error, what string) error {
	if errors.Is(err, ErrConnectionNotFound) {
		return fmt.Errorf("%w: no connection for %s", ErrInactiveConnection, what)
	}
	return fmt.Errorf("glorian: resolve connection for %s: %w", what, err)
}

// StaticSource serves one connection for every product, e.g. the
// env-configured Glorian hub. An empty source (zero Connection) resolves to
// ErrConnectionNotFound.
type StaticSource struct {
	Conn Connection
}

// ConnectionForProduct implements ConnectionSource.
func (s StaticSource) ConnectionForProduct(context.Context, uuid.UUID) (Connection, error) {
	if s.Conn.Key == "" {
		return Connection{}, ErrConnectionNotFound
	}
	return s.Conn, nil
}

// ConnectionByKey implements ConnectionSource.
func (s StaticSource) ConnectionByKey(_ context.Context, key string) (Connection, error) {
	if s.Conn.Key == "" || s.Conn.Key != key {
		return Connection{}, ErrConnectionNotFound
	}
	return s.Conn, nil
}
