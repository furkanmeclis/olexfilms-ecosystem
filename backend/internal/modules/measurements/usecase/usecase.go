// Package usecase stores mobile measurement uploads (TEC-233, F2-05a, K28).
//
// This is the minimal F2 storage: the upload is kept as a raw record so the
// mobile measurement flow keeps working through the cut-over. F3-02
// (TEC-113) adds mandatory VIN, the device registry, before/after pairing,
// the difference table and the PDF on top of the same table.
//
// An upload is idempotent per organization by the Idempotency-Key header
// or the body's client_measurement_id: a repeat returns the first row and
// writes nothing. Without a VIN the row is vin_pending ("tamamlanacak").
package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Measurement statuses (measurement_results.status).
const (
	StatusAccepted   = "accepted"
	StatusVINPending = "vin_pending"
)

// Measurement sources (measurement_results.source).
const (
	SourceMobile       = "mobile"
	SourceLegacyImport = "legacy_import"
)

const (
	maxKey    = 128
	maxSerial = 64
)

var vinRe = regexp.MustCompile(`^[A-Z0-9]{11,17}$`)

var (
	// ErrServiceNotFound is returned when service_uuid is not a service of the
	// active organization.
	ErrServiceNotFound = errors.New("measurements: service not found")
	ErrNotFound        = errors.New("measurements: not found")
	ErrSerialExists    = errors.New("measurements: device serial exists")
)

// ValidationError is a 400 VALIDATION_ERROR on one field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

// Store is the subset of db.Querier the use case needs.
type Store interface {
	InsertMeasurementResult(ctx context.Context, arg db.InsertMeasurementResultParams) (db.MeasurementResult, error)
	FindMeasurementResultByKeys(ctx context.Context, arg db.FindMeasurementResultByKeysParams) (db.MeasurementResult, error)
	GetServiceForMeasurement(ctx context.Context, arg db.GetServiceForMeasurementParams) (db.GetServiceForMeasurementRow, error)
	ListMeasurementDevices(ctx context.Context, organizationID int64) ([]db.MeasurementDevice, error)
	GetMeasurementDeviceByUUID(ctx context.Context, arg db.GetMeasurementDeviceByUUIDParams) (db.MeasurementDevice, error)
	CreateMeasurementDevice(ctx context.Context, arg db.CreateMeasurementDeviceParams) (db.MeasurementDevice, error)
	UpdateMeasurementDevice(ctx context.Context, arg db.UpdateMeasurementDeviceParams) (db.MeasurementDevice, error)
	ListMeasurementResultsPanel(ctx context.Context, arg db.ListMeasurementResultsPanelParams) ([]db.ListMeasurementResultsPanelRow, error)
	GetMeasurementResultPanel(ctx context.Context, arg db.GetMeasurementResultPanelParams) (db.GetMeasurementResultPanelRow, error)
	ListMeasurementValues(ctx context.Context, arg db.ListMeasurementValuesParams) ([]db.MeasurementValue, error)
	ListMeasurementTires(ctx context.Context, arg db.ListMeasurementTiresParams) ([]db.MeasurementTire, error)
	// TEC-294: VIN completion.
	CompleteMeasurementResultVIN(ctx context.Context, arg db.CompleteMeasurementResultVINParams) (int64, error)
}

// Service stores measurement uploads.
type Service struct {
	store   Store
	matcher Matcher // TEC-296 (nil: no before/after matching)
	// TEC-294: with a transaction source an upload is normalized in its
	// insert transaction (normalize.go).
	tx  TxBeginner
	log *slog.Logger
}

// New creates the use case.
func New(store Store) *Service { return &Service{store: store} }

// Matcher runs the before/after matching for the services of an
// organization with the VIN (TEC-296, Linker.MatchVIN).
type Matcher interface {
	MatchVIN(ctx context.Context, organizationID int64, vin string) error
}

// SetMatcher wires the before/after matching run after an accepted upload.
func (s *Service) SetMatcher(m Matcher) { s.matcher = m }

// matchAccepted computes the before/after suggestions of an accepted
// measurement (TEC-296). The upload is stored whatever the matching does;
// the matcher logs its own failures.
func (s *Service) matchAccepted(ctx context.Context, orgID int64, row db.MeasurementResult) {
	if s.matcher == nil || row.Status != StatusAccepted || !row.Vin.Valid {
		return
	}
	_ = s.matcher.MatchVIN(ctx, orgID, row.Vin.String)
}

// Caller is the uploading user in the active organization.
type Caller struct {
	UserID         int64
	OrganizationID int64
	BrandID        int64
}

type PanelCaller struct {
	Org    orgctx.Scope
	Filter scopefilter.Filter
}

// Input is one upload: the Idempotency-Key header and the JSON body as
// received.
type Input struct {
	IdempotencyKey string
	Body           []byte
}

// Result is the 202 answer; Replayed tells a repeated key.
type Result struct {
	UUID     uuid.UUID `json:"uuid"`
	Status   string    `json:"status"`
	Replayed bool      `json:"-"`
}

// request is the part of the body the use case reads; everything else stays
// in the raw record only.
type request struct {
	ClientMeasurementID *string `json:"client_measurement_id"`
	VIN                 *string `json:"vin"`
	ServiceUUID         *string `json:"service_uuid"`
	MeasuredAt          *string `json:"measured_at"`
	Device              *struct {
		Serial *string `json:"serial"`
	} `json:"device"`
	Raw json.RawMessage `json:"raw"`
}

type parsed struct {
	raw         []byte
	vin         string
	serviceUUID *uuid.UUID
	clientID    string
	key         string
	serial      string
}

func parse(in Input) (parsed, error) {
	var out parsed
	body := bytes.TrimSpace(in.Body)
	if len(body) == 0 || body[0] != '{' {
		return out, invalid("body", "must be a JSON object")
	}
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		return out, invalid("body", "is not valid for this endpoint")
	}
	raw := bytes.TrimSpace(req.Raw)
	if len(raw) == 0 || raw[0] != '{' {
		return out, invalid("raw", "is required and must be an object")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, body); err != nil {
		return out, invalid("body", "must be a JSON object")
	}
	out.raw = compact.Bytes()

	if req.VIN != nil {
		vin := strings.ToUpper(strings.TrimSpace(*req.VIN))
		if vin != "" && !vinRe.MatchString(vin) {
			return out, invalid("vin", "must be 11 to 17 letters or digits")
		}
		out.vin = vin
	}
	if req.ServiceUUID != nil && strings.TrimSpace(*req.ServiceUUID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*req.ServiceUUID))
		if err != nil {
			return out, invalid("service_uuid", "must be a UUID")
		}
		out.serviceUUID = &id
	}
	if req.MeasuredAt != nil && strings.TrimSpace(*req.MeasuredAt) != "" {
		if _, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.MeasuredAt)); err != nil {
			return out, invalid("measured_at", "must be an RFC 3339 date-time")
		}
	}
	if req.ClientMeasurementID != nil {
		out.clientID = strings.TrimSpace(*req.ClientMeasurementID)
		if utf8.RuneCountInString(out.clientID) > maxKey {
			return out, invalid("client_measurement_id", "must be at most 128 characters")
		}
	}
	out.key = strings.TrimSpace(in.IdempotencyKey)
	if utf8.RuneCountInString(out.key) > maxKey {
		return out, invalid("Idempotency-Key", "must be at most 128 characters")
	}
	if req.Device != nil && req.Device.Serial != nil {
		out.serial = strings.TrimSpace(*req.Device.Serial)
		if utf8.RuneCountInString(out.serial) > maxSerial {
			return out, invalid("device.serial", "must be at most 64 characters")
		}
	}
	return out, nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func textPtr(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return text(*v)
}

func timePtr(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *v, Valid: true}
}

func textOut(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func timeOut(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

func uuidOut(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}

func numericOut(v pgtype.Numeric) *string {
	if !v.Valid {
		return nil
	}
	s := posting.FormatNumeric(v)
	return &s
}

// create stores one upload, or returns the first row of a repeated
// Idempotency-Key / client_measurement_id (Create wraps it, normalize.go).
func (s *Service) create(ctx context.Context, c Caller, in Input) (Result, error) {
	p, err := parse(in)
	if err != nil {
		return Result{}, err
	}
	keys := db.FindMeasurementResultByKeysParams{
		OrganizationID: c.OrganizationID, IdempotencyKey: text(p.key), ClientMeasurementID: text(p.clientID),
	}
	hasKey := p.key != "" || p.clientID != ""
	if hasKey {
		if prev, err := s.store.FindMeasurementResultByKeys(ctx, keys); err == nil {
			return Result{UUID: prev.Uuid, Status: prev.Status, Replayed: true}, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return Result{}, err
		}
	}

	arg := db.InsertMeasurementResultParams{
		OrganizationID: c.OrganizationID, BrandID: c.BrandID,
		Vin: text(p.vin), Status: StatusVINPending, Raw: p.raw,
		ClientMeasurementID: text(p.clientID), IdempotencyKey: text(p.key),
		DeviceSerial: text(p.serial), Source: SourceMobile,
		CreatedBy: pgtype.Int8{Int64: c.UserID, Valid: true},
	}
	if p.vin != "" {
		arg.Status = StatusAccepted
	}
	if p.serviceUUID != nil {
		svc, err := s.store.GetServiceForMeasurement(ctx, db.GetServiceForMeasurementParams{
			Uuid: *p.serviceUUID, OrganizationID: c.OrganizationID, BrandID: c.BrandID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, ErrServiceNotFound
		}
		if err != nil {
			return Result{}, err
		}
		arg.ServiceID = pgtype.Int8{Int64: svc.ID, Valid: true}
		arg.VehicleID = pgtype.Int8{Int64: svc.VehicleID, Valid: true}
	}

	row, err := s.store.InsertMeasurementResult(ctx, arg)
	if err == nil {
		s.matchAccepted(ctx, c.OrganizationID, row)
		return Result{UUID: row.Uuid, Status: row.Status}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || !hasKey {
		return Result{}, err
	}
	// A concurrent upload with the same key won the insert.
	prev, err := s.store.FindMeasurementResultByKeys(ctx, keys)
	if err != nil {
		return Result{}, err
	}
	return Result{UUID: prev.Uuid, Status: prev.Status, Replayed: true}, nil
}
