package usecase

// TEC-939: SmartPTG / NexPTG zero (foil) calibrations.
//
// The phone runs the calibration (docs/smartptg/REVERSE_ENGINEERING.md
// §6.1): with a 90 µm (curve comment has cal1/cal2/cal3) or 150 µm foil on
// the plate it waits for 10 steady frames and stores
// offset = curve frequency of the foil point - measured frequency, per
// serial, for the Fe and the Al channel. The backend keeps every result as
// an append-only row of the active organization (gauge_calibrations), so the
// other phones of the organization start from the same offsets; the
// current one is the latest row of the serial.

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Calibration limits.
const (
	// MaxCalibrationOffsetHz bounds an offset: a whole factory curve spans
	// ~6 kHz, anything near this is a wrong foil or a wrong plate.
	MaxCalibrationOffsetHz = 20000
	// maxCalibrationSamples bounds each channel's sample list (the app
	// sends 10).
	maxCalibrationSamples = 64
	// calibrationFutureSkew tolerates a phone clock slightly ahead.
	calibrationFutureSkew = 5 * time.Minute
)

// ErrCalibrationNotFound is returned when the organization never recorded a
// calibration for the serial.
var ErrCalibrationNotFound = errors.New("measurements: gauge calibration not found")

// CalibrationStore is the subset of db.Querier the calibrations need.
type CalibrationStore interface {
	InsertGaugeCalibration(ctx context.Context, arg db.InsertGaugeCalibrationParams) (int64, error)
	FindGaugeCalibrationByClientID(ctx context.Context, arg db.FindGaugeCalibrationByClientIDParams) (int64, error)
	GetGaugeCalibration(ctx context.Context, arg db.GetGaugeCalibrationParams) (db.GetGaugeCalibrationRow, error)
	GetLatestGaugeCalibration(ctx context.Context, arg db.GetLatestGaugeCalibrationParams) (db.GetLatestGaugeCalibrationRow, error)
}

// Calibrations records and reads gauge calibrations.
type Calibrations struct {
	store CalibrationStore
	now   func() time.Time
}

// NewCalibrations builds the use case.
func NewCalibrations(store CalibrationStore) *Calibrations {
	return &Calibrations{store: store, now: time.Now}
}

// CalibrationSamples are the measured frequencies (Hz) of the steady frames.
type CalibrationSamples struct {
	Fe []int `json:"fe"`
	Al []int `json:"al"`
}

// CalibrationInput is the POST body.
type CalibrationInput struct {
	Serial       int                 `json:"serial"`
	FeOffsetHz   *int                `json:"fe_offset_hz"`
	AlOffsetHz   *int                `json:"al_offset_hz"`
	TargetUm     int                 `json:"target_um"`
	Samples      *CalibrationSamples `json:"samples"`
	ClientID     *uuid.UUID          `json:"client_id"`
	CalibratedAt *time.Time          `json:"calibrated_at"`
}

// CalibrationUser is who calibrated.
type CalibrationUser struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// CalibrationView is one recorded calibration.
type CalibrationView struct {
	UUID         uuid.UUID          `json:"uuid"`
	Serial       int                `json:"serial"`
	FeOffsetHz   *int               `json:"fe_offset_hz"`
	AlOffsetHz   *int               `json:"al_offset_hz"`
	TargetUm     int                `json:"target_um"`
	Samples      CalibrationSamples `json:"samples"`
	CalibratedAt time.Time          `json:"calibrated_at"`
	CreatedAt    time.Time          `json:"created_at"`
	CreatedBy    CalibrationUser    `json:"created_by"`
}

// ParseGaugeSerial reads a path serial: 1-9 digits, not zero (leading
// zeros ignored), like the curve proxy.
func ParseGaugeSerial(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 9 {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func validSerial(n int) bool { return n > 0 && n <= 999_999_999 }

func validOffset(p *int) bool {
	return p == nil || (*p >= -MaxCalibrationOffsetHz && *p <= MaxCalibrationOffsetHz)
}

func validSamples(v []int) bool {
	if len(v) > maxCalibrationSamples {
		return false
	}
	for _, f := range v {
		if f <= 0 || f > 0xffff { // the frame carries a 16-bit frequency
			return false
		}
	}
	return true
}

func int4(p *int) pgtype.Int4 {
	if p == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*p), Valid: true} //nolint:gosec // bounded by validOffset
}

func intPtr(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

// Record appends a calibration of the caller's organization. replayed is
// true when the client_id was already recorded (nothing is written; the
// first row is returned).
func (c *Calibrations) Record(ctx context.Context, caller Caller, in CalibrationInput) (CalibrationView, bool, error) {
	switch {
	case !validSerial(in.Serial):
		return CalibrationView{}, false, invalid("serial", "must be 1-9 digits, not zero")
	case in.FeOffsetHz == nil && in.AlOffsetHz == nil:
		return CalibrationView{}, false, invalid("fe_offset_hz", "fe_offset_hz or al_offset_hz is required")
	case !validOffset(in.FeOffsetHz):
		return CalibrationView{}, false, invalid("fe_offset_hz", "is out of range")
	case !validOffset(in.AlOffsetHz):
		return CalibrationView{}, false, invalid("al_offset_hz", "is out of range")
	case in.TargetUm != 90 && in.TargetUm != 150:
		return CalibrationView{}, false, invalid("target_um", "must be 90 or 150")
	}
	samples := CalibrationSamples{Fe: []int{}, Al: []int{}}
	if in.Samples != nil {
		if !validSamples(in.Samples.Fe) || !validSamples(in.Samples.Al) {
			return CalibrationView{}, false, invalid("samples", "must be at most 64 frequencies (1-65535 Hz) per channel")
		}
		if in.Samples.Fe != nil {
			samples.Fe = in.Samples.Fe
		}
		if in.Samples.Al != nil {
			samples.Al = in.Samples.Al
		}
	}
	now := c.now().UTC()
	at := now
	if in.CalibratedAt != nil {
		if in.CalibratedAt.After(now.Add(calibrationFutureSkew)) {
			return CalibrationView{}, false, invalid("calibrated_at", "is in the future")
		}
		at = in.CalibratedAt.UTC()
	}
	raw, err := json.Marshal(samples)
	if err != nil {
		return CalibrationView{}, false, err
	}
	var client pgtype.UUID
	if in.ClientID != nil && *in.ClientID != uuid.Nil {
		client = pgtype.UUID{Bytes: *in.ClientID, Valid: true}
	}
	id, err := c.store.InsertGaugeCalibration(ctx, db.InsertGaugeCalibrationParams{
		OrganizationID: caller.OrganizationID,
		BrandID:        caller.BrandID,
		Serial:         int32(in.Serial), //nolint:gosec // validSerial
		FeOffsetHz:     int4(in.FeOffsetHz),
		AlOffsetHz:     int4(in.AlOffsetHz),
		TargetUm:       int16(in.TargetUm), //nolint:gosec // 90 or 150
		Samples:        raw,
		ClientID:       client,
		CalibratedAt:   pgtype.Timestamptz{Time: at, Valid: true},
		CreatedBy:      caller.UserID,
	})
	replayed := false
	if errors.Is(err, pgx.ErrNoRows) && client.Valid {
		// ON CONFLICT DO NOTHING: the client_id is already recorded.
		replayed = true
		id, err = c.store.FindGaugeCalibrationByClientID(ctx, db.FindGaugeCalibrationByClientIDParams{
			OrganizationID: caller.OrganizationID, ClientID: client,
		})
	}
	if err != nil {
		return CalibrationView{}, false, err
	}
	row, err := c.store.GetGaugeCalibration(ctx, db.GetGaugeCalibrationParams{ID: id, OrganizationID: caller.OrganizationID})
	if err != nil {
		return CalibrationView{}, false, err
	}
	return calibrationView(db.GetLatestGaugeCalibrationRow(row)), replayed, nil
}

// Latest is the current calibration of a serial in the organization.
func (c *Calibrations) Latest(ctx context.Context, organizationID int64, serial int) (CalibrationView, error) {
	if !validSerial(serial) {
		return CalibrationView{}, invalid("serial", "must be 1-9 digits, not zero")
	}
	row, err := c.store.GetLatestGaugeCalibration(ctx, db.GetLatestGaugeCalibrationParams{
		OrganizationID: organizationID, Serial: int32(serial), //nolint:gosec // validSerial
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return CalibrationView{}, ErrCalibrationNotFound
	}
	if err != nil {
		return CalibrationView{}, err
	}
	return calibrationView(row), nil
}

func calibrationView(r db.GetLatestGaugeCalibrationRow) CalibrationView {
	samples := CalibrationSamples{Fe: []int{}, Al: []int{}}
	_ = json.Unmarshal(r.Samples, &samples)
	if samples.Fe == nil {
		samples.Fe = []int{}
	}
	if samples.Al == nil {
		samples.Al = []int{}
	}
	return CalibrationView{
		UUID:         r.Uuid,
		Serial:       int(r.Serial),
		FeOffsetHz:   intPtr(r.FeOffsetHz),
		AlOffsetHz:   intPtr(r.AlOffsetHz),
		TargetUm:     int(r.TargetUm),
		Samples:      samples,
		CalibratedAt: r.CalibratedAt.Time.UTC(),
		CreatedAt:    r.CreatedAt.Time.UTC(),
		CreatedBy: CalibrationUser{
			UUID: r.CreatedByUuid,
			Name: strings.TrimSpace(r.CreatedByName + " " + r.CreatedBySurname),
		},
	}
}
