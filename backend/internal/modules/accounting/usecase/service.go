// Package usecase holds the accounting rules behind /v1/accounting (TEC-172,
// F1-07b): cash/bank accounts, cari read models, manual entries,
// collections/payments and voids. Every write goes through accounting/posting
// in one transaction; the ledger stays append-only.
//
// Book: every request works on one organization's ledger. Reads default to
// the active organization and may name an organization below it
// (organization_uuid) when the permission scope reaches it; writes always go
// to the active organization's own book (K9: nobody writes another ledger by
// hand).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Errors returned by the service; the handler maps them to HTTP codes.
var (
	ErrForbidden       = errors.New("accounting: not allowed for this organization")
	ErrBookNotFound    = errors.New("accounting: organization not found")
	ErrAccountNotFound = errors.New("accounting: account not found")
	ErrCariNotFound    = errors.New("accounting: cari account not found")
	ErrEntryNotFound   = errors.New("accounting: entry not found")
	ErrCounterparty    = errors.New("accounting: counterparty not found")
	// ErrNotVoidable: the entry is sourced by another module, a reversal, or
	// already reversed.
	ErrNotVoidable = errors.New("accounting: entry cannot be voided")
	// ErrIdempotencyConflict: the idempotency key was used for another write.
	ErrIdempotencyConflict = errors.New("accounting: idempotency key already used with different values")
	// ErrRateNotFound: no exchange rate for a foreign-currency entry.
	ErrRateNotFound = errors.New("accounting: exchange rate not found")
)

// ValidationError is a field-level input error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// Organization types (organizations.type).
const (
	OrgCenter      = "center"
	OrgDistributor = "distributor"
	OrgDealer      = "dealer"
)

// SourceManual is the source_type of entries written through this API.
const SourceManual = "manual"

// TxBeginner starts a transaction (*pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// FeatureChecker answers whether a module is on (*features.Service).
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// Caller is the request principal in its active organization, with the
// resolved scope of the route's permission.
type Caller struct {
	UserID int64 // internal user id (actor); 0 when unknown
	Org    orgctx.Scope
	Filter scopefilter.Filter
}

func (c Caller) actor() *int64 {
	if c.UserID == 0 {
		return nil
	}
	id := c.UserID
	return &id
}

// Service implements the accounting use cases.
type Service struct {
	pool     TxBeginner
	q        *db.Queries
	poster   *posting.Poster
	features FeatureChecker
}

// New creates the service. checker may be nil (dealer writes stay closed).
func New(pool TxBeginner, q *db.Queries, poster *posting.Poster, checker FeatureChecker) *Service {
	return &Service{pool: pool, q: q, poster: poster, features: checker}
}

// readBook returns the organization whose ledger a read targets: the active
// organization, or (orgUUID) an organization below it that the permission
// scope reaches. Anything else reads as not found.
func (s *Service) readBook(ctx context.Context, c Caller, orgUUID *uuid.UUID) (db.Organization, error) {
	active, err := s.activeOrg(ctx, c)
	if err != nil {
		return db.Organization{}, err
	}
	if orgUUID == nil || *orgUUID == active.Uuid {
		return active, nil
	}
	target, err := s.q.GetOrganizationByUUID(ctx, *orgUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrBookNotFound
	}
	if err != nil {
		return db.Organization{}, fmt.Errorf("accounting: organization: %w", err)
	}
	if !c.Filter.AllowsOrg(target.ID, target.BrandID) {
		return db.Organization{}, ErrBookNotFound
	}
	below, err := s.q.Descendants(ctx, active.ID)
	if err != nil {
		return db.Organization{}, fmt.Errorf("accounting: descendants: %w", err)
	}
	for _, o := range below {
		if o.ID == target.ID {
			return target, nil
		}
	}
	return db.Organization{}, ErrBookNotFound
}

func (s *Service) activeOrg(ctx context.Context, c Caller) (db.Organization, error) {
	if c.Org.InternalID == 0 || !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) {
		return db.Organization{}, ErrForbidden
	}
	o, err := s.q.GetOrganizationByID(ctx, c.Org.InternalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, ErrBookNotFound
	}
	if err != nil {
		return db.Organization{}, fmt.Errorf("accounting: organization: %w", err)
	}
	return o, nil
}

// writeBook returns the active organization when it may write its own
// ledger by hand. Dealers are read-only in F1 (TEC-99 decision 7); the F3
// dealer_accounting module (TEC-118) opens their manual writes.
func (s *Service) writeBook(ctx context.Context, c Caller) (db.Organization, error) {
	o, err := s.activeOrg(ctx, c)
	if err != nil {
		return db.Organization{}, err
	}
	switch o.Type {
	case OrgCenter, OrgDistributor:
		return o, nil
	case OrgDealer:
		if s.features == nil {
			return db.Organization{}, ErrForbidden
		}
		on, err := s.features.Enabled(ctx, o.ID, features.ModuleDealerAccounting)
		if err != nil {
			return db.Organization{}, fmt.Errorf("accounting: feature check: %w", err)
		}
		if !on {
			return db.Organization{}, ErrForbidden
		}
		return o, nil
	default:
		return db.Organization{}, ErrForbidden
	}
}

// inTx runs fn in a transaction and commits when it succeeds.
func (s *Service) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("accounting: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("accounting: commit: %w", err)
	}
	return nil
}

func trimmedPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}
