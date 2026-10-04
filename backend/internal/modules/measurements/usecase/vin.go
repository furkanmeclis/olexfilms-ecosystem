package usecase

// TEC-294 (F3-02b): VIN completion of vin_pending measurements.

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// strictVINRe is a full ISO 3779 VIN: 17 characters, no I, O or Q.
var strictVINRe = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)

var (
	// ErrVINAlreadySet: the measurement already carries another VIN (422
	// MEASUREMENT_VIN_ALREADY_SET).
	ErrVINAlreadySet = errors.New("measurements: vin already set")
	// ErrVINPending: a vin_pending measurement cannot be linked to a service
	// (422 MEASUREMENT_VIN_PENDING); the VIN must be completed first.
	ErrVINPending = errors.New("measurements: vin pending")
)

// NormalizeVIN upper-cases and validates a full VIN (17 characters, no I/O/Q).
func NormalizeVIN(v string) (string, error) {
	vin := strings.ToUpper(strings.TrimSpace(v))
	if vin == "" {
		return "", invalid("vin", "is required")
	}
	if !strictVINRe.MatchString(vin) {
		return "", invalid("vin", "must be 17 letters or digits without I, O or Q")
	}
	return vin, nil
}

// EnsureLinkable refuses a measurement that cannot be linked to a service:
// vin_pending ones (TEC-294; the before/after link of TEC-296 calls it).
func EnsureLinkable(status string) error {
	if status == StatusVINPending {
		return ErrVINPending
	}
	return nil
}

// CompleteVIN fills the VIN of a vin_pending measurement in the caller's
// reach (measurements.link) and makes it accepted. Repeating the same VIN
// on an accepted measurement is a no-op; another VIN is ErrVINAlreadySet.
func (s *Service) CompleteVIN(ctx context.Context, c PanelCaller, id uuid.UUID, vin string) (MeasurementDetail, error) {
	vin, err := NormalizeVIN(vin)
	if err != nil {
		return MeasurementDetail{}, err
	}
	row, err := s.store.GetMeasurementResultPanel(ctx, db.GetMeasurementResultPanelParams{
		Uuid: id, BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return MeasurementDetail{}, ErrNotFound
	}
	if err != nil {
		return MeasurementDetail{}, err
	}
	if row.Status != StatusVINPending {
		if row.Vin.String != vin {
			return MeasurementDetail{}, ErrVINAlreadySet
		}
		return s.GetMeasurement(ctx, c, id)
	}
	n, err := s.store.CompleteMeasurementResultVIN(ctx, db.CompleteMeasurementResultVINParams{
		Vin: text(vin), ID: row.ID, OrganizationID: row.OrganizationID,
	})
	if err != nil {
		return MeasurementDetail{}, err
	}
	if n == 0 {
		// A concurrent completion won: answer what is stored now.
		return s.CompleteVIN(ctx, c, id, vin)
	}
	return s.GetMeasurement(ctx, c, id)
}
