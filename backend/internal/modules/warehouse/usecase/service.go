// Package usecase holds the warehouse tree rules behind /v1/warehouse
// (TEC-201, F1-03a): warehouses, rooms and typed locations (aisle -> shelf
// -> bin) with a derived, organization-unique full_code.
//
// Reach: the full warehouse module belongs to the center and the
// distributor (K4, K12); a dealer gets ErrForbidden even with a custom role
// that holds warehouse.*. Every request works on the active organization's
// own warehouses, so a distributor only ever sees its own. The warehouse
// side is brand-independent (K20): rows carry no brand_id.
//
// The ledger is not touched here; units are placed into locations by the
// stock flows (TEC-95d/e) through ledger.Post.
package usecase

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service; the handler maps them to HTTP codes.
var (
	ErrForbidden         = errors.New("warehouse: not available for this organization")
	ErrWarehouseNotFound = errors.New("warehouse: warehouse not found")
	ErrRoomNotFound      = errors.New("warehouse: room not found")
	ErrLocationNotFound  = errors.New("warehouse: location not found")
	// ErrCodeTaken: the code (or the derived full_code) already exists.
	ErrCodeTaken = errors.New("warehouse: code already used")
	// ErrInUse: rooms, locations or stock still reference the row.
	ErrInUse = errors.New("warehouse: still in use")
)

// Location types (warehouse_locations.type).
const (
	TypeAisle = "aisle"
	TypeShelf = "shelf"
	TypeBin   = "bin"
)

// Limits.
const (
	maxNameLen = 200
	// MaxGenerated caps the nodes one bulk generation may touch.
	MaxGenerated = 2000
	// MaxReorder caps the items of one reorder request.
	MaxReorder = 1000
)

var codePattern = regexp.MustCompile(`^[A-Z0-9_]{1,32}$`)

// ValidationError is a field-level input error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// TxBeginner starts a transaction (*pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Caller is the request principal in its active organization, with the
// resolved scope of the route's permission.
type Caller struct {
	Org    orgctx.Scope
	Filter scopefilter.Filter
}

// Service implements the warehouse tree use cases.
type Service struct {
	pool TxBeginner
	q    *db.Queries
}

// New builds the service.
func New(pool TxBeginner, q *db.Queries) *Service {
	return &Service{pool: pool, q: q}
}

// guard returns the active organization id when the caller may use the
// warehouse module there (center or distributor, inside the grant scope).
func guard(c Caller) (int64, error) {
	switch c.Org.OrgType {
	case rbac.OrgTypeCenter, rbac.OrgTypeDistributor:
	default:
		return 0, ErrForbidden
	}
	if c.Org.InternalID == 0 || !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) || c.Filter.UserOnly() {
		return 0, ErrForbidden
	}
	return c.Org.InternalID, nil
}

// normCode upper-cases and validates a code. The hyphen is the full_code
// separator, so codes hold letters, digits and underscores only.
func normCode(field, raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if code == "" {
		return "", invalid(field, "is required")
	}
	if !codePattern.MatchString(code) {
		return "", invalid(field, "must be 1-32 characters: A-Z, 0-9 or _")
	}
	return code, nil
}

// normName trims a name; an empty name falls back to def.
func normName(field, raw, def string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = def
	}
	if name == "" {
		return "", invalid(field, "is required")
	}
	if utf8.RuneCountInString(name) > maxNameLen {
		return "", invalid(field, "must be at most 200 characters")
	}
	return name, nil
}

// mapDBError turns constraint violations into service errors.
func mapDBError(err error) error {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return err
	}
	switch pg.Code {
	case "23505":
		return ErrCodeTaken
	case "23001", "23503":
		// ON DELETE RESTRICT answers 23001 on PostgreSQL 18, 23503 before.
		return ErrInUse
	case "23514":
		if pg.ConstraintName == "chk_warehouse_locations_tree" {
			return invalid("parent_uuid", "does not fit the location type")
		}
	}
	return err
}

func notFound(err, nf error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return nf
	}
	return err
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func ts(t pgtype.Timestamptz) time.Time { return t.Time }

// inTx runs fn in one transaction.
func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// orderedIDs validates a reorder request: unique, non-empty uuids that all
// resolve (found holds the resolved ids by uuid).
func orderedIDs(uuids []uuid.UUID, found map[uuid.UUID]int64) ([]int64, error) {
	if len(uuids) == 0 {
		return nil, invalid("uuids", "is required")
	}
	if len(uuids) > MaxReorder {
		return nil, invalid("uuids", "must hold at most 1000 items")
	}
	seen := make(map[uuid.UUID]bool, len(uuids))
	out := make([]int64, 0, len(uuids))
	for _, u := range uuids {
		if seen[u] {
			return nil, invalid("uuids", "must not repeat")
		}
		seen[u] = true
		id, ok := found[u]
		if !ok {
			return nil, invalid("uuids", "contains an unknown item")
		}
		out = append(out, id)
	}
	return out, nil
}
