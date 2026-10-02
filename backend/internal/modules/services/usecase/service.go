// Package usecase holds the service (vehicle application) rules behind
// /v1/services (TEC-179, F1-05b; TEC-97 decisions 1-7).
//
// A service is opened by the active organization (dealer, distributor or
// center) for a customer in its scope and one of the customer's vehicles;
// the vehicle is copied as a snapshot (decision 3) and the brand is the
// organization's brand (decision 2). Items reference stock units the
// organization holds; a serial unit may sit in one open service only (the
// ledger has the final word on completion, TEC-180). Every status change
// writes service_status_logs and a service.* outbox event in the same
// transaction. Reads are limited by the services.read scope (dealer: own
// organization, distributor: subtree, center: brand); anything outside it
// reads as 404.
package usecase

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service; the handler maps them to HTTP codes.
var (
	ErrNotFound  = errors.New("services: service not found")
	ErrForbidden = errors.New("services: not allowed for this organization")
	// ErrInvalidTransition: the status does not allow the move (also any
	// move out of completed / cancelled).
	ErrInvalidTransition = errors.New("services: invalid status transition")
	// ErrNotEditable: the service status does not allow this change.
	ErrNotEditable = errors.New("services: service is not editable")
	// ErrUnitInUse: the serial unit is already in another open service.
	ErrUnitInUse = errors.New("services: unit is in another open service")
	// ErrUnitNotAvailable: the unit is not held (available) by the service
	// organization, or not enough of it (also a ledger refusal on
	// completion).
	ErrUnitNotAvailable = errors.New("services: unit is not available to the organization")
	// ErrTooManyImages: the image limit of a service is reached.
	ErrTooManyImages = errors.New("services: too many images")
	// ErrServiceNoExhausted: no free service number after several tries.
	ErrServiceNoExhausted = errors.New("services: could not allocate a service number")
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

// Limits.
const (
	MaxImages       = 30
	MaxItems        = 100
	MaxNoteLength   = 5000
	MaxPackageLen   = 255
	MaxKM           = 5_000_000
	MaxQuantity     = 100_000
	serviceNoPrefix = "DS"
	serviceNoLength = 8
	serviceNoTries  = 10
	serviceNoChars  = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

// TxBeginner starts a transaction (*pgxpool.Pool).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Caller is the request principal in its active organization; Filter is
// the resolved scope of the route permission (services.read, or
// services.write on create).
type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

func (c Caller) actor() pgtype.Int8 {
	if c.Principal.UserInternal == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: c.Principal.UserInternal, Valid: true}
}

func (c Caller) actorOrg() pgtype.Int8 {
	if c.Org.InternalID == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: c.Org.InternalID, Valid: true}
}

// isCenter reports whether the active organization is the brand center.
func (c Caller) isCenter() bool { return c.Org.OrgType == OrgCenter }

// allows reports whether the caller holds slug with a scope reaching the
// service: all / brand (the domain brand), subtree (the service
// organization is in the caller's tree), managed (the active organization
// is the service organization), own / assigned (additionally created by
// the caller).
func (c Caller) allows(slug string, s db.Service) bool {
	scope, ok := c.Principal.ScopeFor(slug)
	if !ok {
		return false
	}
	switch scope {
	case rbac.ScopeAll:
		return true
	case rbac.ScopeBrand:
		return s.BrandID == c.Org.BrandID
	case rbac.ScopeSubtree:
		return c.Filter.AllowsOrg(s.OrganizationID, s.BrandID)
	case rbac.ScopeManaged:
		return s.OrganizationID == c.Org.InternalID
	case rbac.ScopeOwn, rbac.ScopeAssigned:
		return s.OrganizationID == c.Org.InternalID &&
			s.CreatedByUserID.Valid && s.CreatedByUserID.Int64 == c.Principal.UserInternal
	}
	return false
}

// brandWide reports whether the caller holds slug for the whole brand (the
// center): it may edit locked (completed / cancelled) services.
func (c Caller) brandWide(slug string) bool { return c.Principal.Can(slug, rbac.ScopeBrand) }

// visible reports whether the service is inside the caller's read scope.
func visible(c Caller, s db.Service) bool {
	if s.BrandID != c.Org.BrandID || !c.Filter.AllowsOrg(s.OrganizationID, s.BrandID) {
		return false
	}
	if c.Filter.UserOnly() {
		return s.CreatedByUserID.Valid && s.CreatedByUserID.Int64 == c.Filter.UserID
	}
	return true
}

// Service implements the service use cases.
type Service struct {
	pool TxBeginner
	q    *db.Queries
	out  outbox.Enqueuer
}

// New creates the service.
func New(pool TxBeginner, q *db.Queries, out outbox.Enqueuer) *Service {
	return &Service{pool: pool, q: q, out: out}
}

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("services: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("services: commit: %w", err)
	}
	return nil
}

func textOrNull(p *string) pgtype.Text {
	if p == nil {
		return pgtype.Text{}
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func checkLen(field string, p *string, max int) error {
	if p != nil && len([]rune(strings.TrimSpace(*p))) > max {
		return invalid(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return nil
}

func kmOrNull(field string, p *int64) (pgtype.Int4, error) {
	if p == nil {
		return pgtype.Int4{}, nil
	}
	if *p < 0 || *p > MaxKM {
		return pgtype.Int4{}, invalid(field, fmt.Sprintf("must be between 0 and %d", MaxKM))
	}
	return pgtype.Int4{Int32: int32(*p), Valid: true}, nil
}

func parseUUID(field, raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, invalid(field, "must be a UUID")
	}
	return id, nil
}

// newServiceNo returns DS + 8 upper-case alphanumerics (legacy
// ServiceNumberGenerator).
func newServiceNo() (string, error) {
	var b strings.Builder
	b.WriteString(serviceNoPrefix)
	max := big.NewInt(int64(len(serviceNoChars)))
	for i := 0; i < serviceNoLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("services: service number: %w", err)
		}
		b.WriteByte(serviceNoChars[n.Int64()])
	}
	return b.String(), nil
}

func (s *Service) allocateServiceNo(ctx context.Context, q *db.Queries) (string, error) {
	for i := 0; i < serviceNoTries; i++ {
		no, err := newServiceNo()
		if err != nil {
			return "", err
		}
		exists, err := q.ServiceNoExists(ctx, no)
		if err != nil {
			return "", fmt.Errorf("services: service number check: %w", err)
		}
		if !exists {
			return no, nil
		}
	}
	return "", ErrServiceNoExhausted
}

func isUniqueViolation(err error, constraint string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == constraint
}

// --- Create ---------------------------------------------------------------------

// CreateInput opens a draft service of the active organization.
type CreateInput struct {
	CustomerUUID   string
	VehicleUUID    string
	KM             *int64
	Package        *string
	Notes          *string
	HasMeasurement bool
}

// Create opens a draft service: the customer must be in the caller's
// services.write scope, the vehicle must belong to the customer and to the
// domain brand and carry a car brand and model (snapshot, decision 3).
func (s *Service) Create(ctx context.Context, c Caller, in CreateInput) (ServiceView, error) {
	if c.Org.InternalID == 0 || !c.Principal.HasPermission(rbac.PermServicesWrite) {
		return ServiceView{}, ErrForbidden
	}
	customerID, err := parseUUID("customer_uuid", in.CustomerUUID)
	if err != nil {
		return ServiceView{}, err
	}
	vehicleID, err := parseUUID("vehicle_uuid", in.VehicleUUID)
	if err != nil {
		return ServiceView{}, err
	}
	if err := checkLen("package", in.Package, MaxPackageLen); err != nil {
		return ServiceView{}, err
	}
	if err := checkLen("notes", in.Notes, MaxNoteLength); err != nil {
		return ServiceView{}, err
	}
	km, err := kmOrNull("km", in.KM)
	if err != nil {
		return ServiceView{}, err
	}
	var created db.Service
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		org, err := q.GetOrganizationByID(ctx, c.Org.InternalID)
		if err != nil {
			return fmt.Errorf("services: organization: %w", err)
		}
		if org.BrandID != c.Org.BrandID {
			return ErrForbidden
		}
		customer, err := q.GetUserByUUID(ctx, customerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("customer_uuid", "customer not found")
		}
		if err != nil {
			return fmt.Errorf("services: customer: %w", err)
		}
		inScope, err := q.CustomerInScope(ctx, db.CustomerInScopeParams{
			UserID: customer.ID, OrgIds: c.Filter.OrgIDsArg(),
			BrandID: pgtype.Int8{Int64: org.BrandID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("services: customer scope: %w", err)
		}
		if !inScope {
			return invalid("customer_uuid", "customer not found")
		}
		if customer.Status != "active" {
			return invalid("customer_uuid", "customer account is not active")
		}
		v, err := q.GetVehicleByUUID(ctx, vehicleID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (v.UserID != customer.ID || v.BrandID != org.BrandID)) {
			return invalid("vehicle_uuid", "vehicle not found for this customer")
		}
		if err != nil {
			return fmt.Errorf("services: vehicle: %w", err)
		}
		if !v.CarBrandID.Valid || !v.CarModelID.Valid {
			return invalid("vehicle_uuid", "the vehicle needs a car brand and model")
		}
		if in.HasMeasurement && !v.Vin.Valid {
			return invalid("has_measurement", "a measurement needs the vehicle VIN")
		}
		no, err := s.allocateServiceNo(ctx, q)
		if err != nil {
			return err
		}
		row, err := q.CreateService(ctx, db.CreateServiceParams{
			ServiceNo: no, OrganizationID: org.ID, BrandID: org.BrandID,
			CustomerUserID: customer.ID, VehicleID: v.ID,
			CarBrandID: v.CarBrandID.Int64, CarModelID: v.CarModelID.Int64,
			ModelYear: v.ModelYear, Plate: v.Plate, PlateCountry: v.PlateCountry, Vin: v.Vin, Km: km,
			Package: textOrNull(in.Package), Notes: textOrNull(in.Notes), HasMeasurement: in.HasMeasurement,
			Status: StatusDraft, CreatedByUserID: c.actor(),
		})
		if isUniqueViolation(err, "uq_services_service_no") {
			return ErrServiceNoExhausted
		}
		if err != nil {
			return fmt.Errorf("services: create: %w", err)
		}
		if err := s.log(ctx, q, row, "", StatusDraft, c, nil, nil); err != nil {
			return err
		}
		created = row
		return s.emit(ctx, tx, events.ServiceCreated, row, "", c, nil)
	})
	if err != nil {
		return ServiceView{}, err
	}
	return s.view(ctx, s.q, c, created)
}

// --- Update ------------------------------------------------------------------------

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

// UpdateInput edits the free form fields of a service.
type UpdateInput struct {
	KM             Optional[int64]
	Package        Optional[string]
	Notes          Optional[string]
	HasMeasurement Optional[bool]
}

// formEditable: completed and cancelled services are locked for everyone
// but the center (TEC-97 form lock).
func formEditable(c Caller, svc db.Service) bool {
	if svc.Status == StatusCompleted || svc.Status == StatusCancelled {
		return c.isCenter() && c.brandWide(rbac.PermServicesWrite)
	}
	return true
}

func (s *Service) lockVisible(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.Service, error) {
	svc, err := q.LockServiceByUUID(ctx, db.LockServiceByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Service{}, ErrNotFound
	}
	if err != nil {
		return db.Service{}, fmt.Errorf("services: lock: %w", err)
	}
	if !visible(c, svc) {
		return db.Service{}, ErrNotFound
	}
	return svc, nil
}

// lockWritable locks a visible service the caller may write and whose form
// is editable.
func (s *Service) lockWritable(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.Service, error) {
	svc, err := s.lockVisible(ctx, q, c, id)
	if err != nil {
		return db.Service{}, err
	}
	if !c.allows(rbac.PermServicesWrite, svc) {
		return db.Service{}, ErrForbidden
	}
	if !formEditable(c, svc) {
		return db.Service{}, ErrNotEditable
	}
	return svc, nil
}

// Update edits km, package, notes and the measurement answer.
func (s *Service) Update(ctx context.Context, c Caller, id uuid.UUID, in UpdateInput) (ServiceView, error) {
	if err := checkLen("package", in.Package.Value, MaxPackageLen); err != nil {
		return ServiceView{}, err
	}
	if err := checkLen("notes", in.Notes.Value, MaxNoteLength); err != nil {
		return ServiceView{}, err
	}
	var updated db.Service
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := s.lockWritable(ctx, q, c, id)
		if err != nil {
			return err
		}
		p := db.UpdateServiceParams{
			ID: svc.ID, BrandID: svc.BrandID, CustomerUserID: svc.CustomerUserID, VehicleID: svc.VehicleID,
			CarBrandID: svc.CarBrandID, CarModelID: svc.CarModelID, ModelYear: svc.ModelYear,
			Plate: svc.Plate, PlateCountry: svc.PlateCountry, Vin: svc.Vin, Km: svc.Km,
			Package: svc.Package, Notes: svc.Notes, HasMeasurement: svc.HasMeasurement,
			MeasurementResultID: svc.MeasurementResultID, ContractID: svc.ContractID,
			UpdatedByUserID: c.actor(),
		}
		if in.KM.Set {
			if p.Km, err = kmOrNull("km", in.KM.Value); err != nil {
				return err
			}
		}
		if in.Package.Set {
			p.Package = textOrNull(in.Package.Value)
		}
		if in.Notes.Set {
			p.Notes = textOrNull(in.Notes.Value)
		}
		if in.HasMeasurement.Set {
			p.HasMeasurement = in.HasMeasurement.Value != nil && *in.HasMeasurement.Value
		}
		if p.HasMeasurement && !p.Vin.Valid {
			return invalid("has_measurement", "a measurement needs the vehicle VIN")
		}
		row, err := q.UpdateService(ctx, p)
		if err != nil {
			return fmt.Errorf("services: update: %w", err)
		}
		updated = row
		return s.emit(ctx, tx, events.ServiceUpdated, row, "", c, nil)
	})
	if err != nil {
		return ServiceView{}, err
	}
	return s.view(ctx, s.q, c, updated)
}

// --- Log / outbox ------------------------------------------------------------------

func (s *Service) log(ctx context.Context, q *db.Queries, svc db.Service, from, to string, c Caller,
	note *string, meta map[string]any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("services: log metadata: %w", err)
	}
	fromText := pgtype.Text{}
	if from != "" {
		fromText = pgtype.Text{String: from, Valid: true}
	}
	if _, err := q.InsertServiceStatusLog(ctx, db.InsertServiceStatusLogParams{
		ServiceID: svc.ID, FromStatus: fromText, ToStatus: to, ActorUserID: c.actor(),
		ActorOrgID: c.actorOrg(), Note: textOrNull(note), Metadata: raw,
	}); err != nil {
		return fmt.Errorf("services: status log: %w", err)
	}
	return nil
}

func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, svc db.Service, from string, c Caller,
	extra map[string]any) error {
	id, uid := svc.ID, svc.Uuid
	payload := map[string]any{
		"service_uuid":    svc.Uuid.String(),
		"service_no":      svc.ServiceNo,
		"status":          svc.Status,
		"organization_id": svc.OrganizationID,
		"brand_id":        svc.BrandID,
		"vehicle_id":      svc.VehicleID,
		"customer_id":     svc.CustomerUserID,
	}
	if from != "" && from != svc.Status {
		payload["from_status"] = from
	}
	for k, v := range extra {
		payload[k] = v
	}
	ev := events.New(name).WithTenant(svc.OrganizationID).WithEntity("service", &id, &uid).WithPayload(payload)
	if c.Principal.UserInternal != 0 {
		ev = ev.WithActor(c.Principal.UserInternal)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("services: outbox: %w", err)
	}
	return nil
}
