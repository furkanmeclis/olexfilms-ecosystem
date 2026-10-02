// Package usecase holds the customer and vehicle rules behind /v1/customers
// and /v1/vehicles (TEC-160, F1-08b; K11, K19, K20, K26, K29).
//
// A customer is a users row (K11) plus a global customer_profiles row; which
// organization serves which customer lives in customer_organizations. Every
// read and write is limited by that link: a caller sees a customer only when
// the customer is linked to an organization inside the permission scope
// (dealer: its own organization, distributor: its subtree, center: the
// brand). The brand is always the domain brand, also for the center (K20).
//
// One phone is one user: creating a customer with a phone that already
// belongs to a user links that user instead of creating a second one. The
// identity of a shared customer (name, e-mail, profile) is never overwritten
// by another organization; only empty fields are filled.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service; the handler maps them to HTTP codes.
var (
	// ErrForbidden: the active organization may not do this (e.g. a dealer
	// upgrading a customer).
	ErrForbidden = errors.New("customers: not allowed for this organization")
	// ErrCustomerNotFound: no such customer, or not linked to the scope.
	ErrCustomerNotFound = errors.New("customers: customer not found")
	// ErrVehicleNotFound: no such vehicle, or its customer is out of scope.
	ErrVehicleNotFound = errors.New("customers: vehicle not found")
	// ErrOrganizationNotFound: the target organization of an upgrade is
	// missing or out of scope.
	ErrOrganizationNotFound = errors.New("customers: organization not found")
	// ErrAnonymized: the customer was anonymized (K19) and is read-only.
	ErrAnonymized = errors.New("customers: customer is anonymized")
	// ErrInactive: the customer account is disabled or merged.
	ErrInactive = errors.New("customers: customer account is not active")
	// ErrEmailTaken: the e-mail belongs to another user.
	ErrEmailTaken = errors.New("customers: e-mail already in use")
	// ErrAlreadyMember: the customer is already a member of the organization.
	ErrAlreadyMember = errors.New("customers: already a member of the organization")
	// ErrPIIUnavailable: CUSTOMER_PII_KEY is not configured, identity
	// numbers cannot be stored.
	ErrPIIUnavailable = errors.New("customers: identity number encryption is not configured")
)

// ValidationError is a field-level input error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// InvalidPlateError: the plate does not match its country's format.
type InvalidPlateError struct {
	Country string
}

func (e *InvalidPlateError) Error() string { return "customers: invalid plate for " + e.Country }

// Customer statuses that matter here (users.status).
const (
	StatusActive     = "active"
	StatusAnonymized = "anonymized"
)

// Identity number fields (crypto.PIIAAD binds a ciphertext to its column).
const (
	aadNationalID = "customer_profiles.national_id"
	aadTaxNo      = "customer_profiles.tax_no"
)

// AnonymizedNameKey is the catalog label shown instead of an anonymized
// customer's name.
const AnonymizedNameKey = "customers.anonymized_name"

// TxBeginner starts a transaction (*pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PlateValidator checks a plate against its country format (*geo.Service).
type PlateValidator interface {
	ValidatePlate(ctx context.Context, iso2, plate string) (geo.PlateCheck, error)
}

// Caller is the request principal in its active organization, with the
// resolved scope of the route's permission.
type Caller struct {
	UserID int64 // internal user id (actor)
	Org    orgctx.Scope
	Filter scopefilter.Filter
	Locale i18n.Locale
}

// orgIDs is the organization restriction of the scope (nil: none).
func (c Caller) orgIDs() []int64 { return c.Filter.OrgIDsArg() }

// brand is the domain brand: customers never cross brands, also for the
// center and super admin (K20).
func (c Caller) brand() pgtype.Int8 {
	return pgtype.Int8{Int64: c.Org.BrandID, Valid: c.Org.BrandID != 0}
}

// Service implements the customer and vehicle use cases.
type Service struct {
	pool    TxBeginner
	q       *db.Queries
	pii     *crypto.PIIBox
	plates  PlateValidator
	revoker Revoker
	search  SearchIndexer
	out     outbox.Enqueuer
	tr      *transfers // TEC-190: vehicle transfer (nil: disabled)
}

// New creates the service. pii may be nil (CUSTOMER_PII_KEY unset in
// development): identity numbers are then refused, never stored in clear.
func New(pool TxBeginner, q *db.Queries, pii *crypto.PIIBox, plates PlateValidator) *Service {
	return &Service{pool: pool, q: q, pii: pii, plates: plates}
}

// inTx runs fn in a transaction and commits when it succeeds.
func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("customers: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("customers: commit: %w", err)
	}
	return nil
}

// Optional is a JSON field of a PATCH body: Set reports whether the key was
// present, Value is nil for an explicit null.
type Optional[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON records presence; null leaves Value nil.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// Of builds a present Optional (tests and internal callers).
func Of[T any](v T) Optional[T] { return Optional[T]{Set: true, Value: &v} }

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func strOrNil(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}
