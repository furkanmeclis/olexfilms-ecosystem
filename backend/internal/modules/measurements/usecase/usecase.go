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
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
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

// ErrServiceNotFound is returned when service_uuid is not a service of the
// active organization.
var ErrServiceNotFound = errors.New("measurements: service not found")

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
}

// Service stores measurement uploads.
type Service struct{ store Store }

// New creates the use case.
func New(store Store) *Service { return &Service{store: store} }

// Caller is the uploading user in the active organization.
type Caller struct {
	UserID         int64
	OrganizationID int64
	BrandID        int64
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

// Create stores one upload, or returns the first row of a repeated
// Idempotency-Key / client_measurement_id.
func (s *Service) Create(ctx context.Context, c Caller, in Input) (Result, error) {
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
