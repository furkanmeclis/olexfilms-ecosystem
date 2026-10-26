package usecase

// TEC-1004: a hand-entered service number (the legacy ServiceNumberInput).
// Printed warranty cards carry a QR code https://olexfilms.app/warranty/XXXX;
// the service is opened with that number so the card resolves to it. The
// number is free until the service is completed or cancelled; then it is
// locked, because the warranties and their public links are keyed on it.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	// ErrServiceNoTaken: another service, warranty code or alias holds the
	// number.
	ErrServiceNoTaken = errors.New("services: service number is taken")
	// ErrServiceNoLocked: the service is completed or cancelled.
	ErrServiceNoLocked = errors.New("services: service number is locked")
)

// serviceNoRe is the shape of a hand-entered number: a URL path segment of
// 4..32 characters, the same rule as a warranty public code
// (chk_warranties_public_code), so /warranty/{no} always resolves. It is
// kept as printed (case-sensitive); only surrounding space is trimmed.
var serviceNoRe = regexp.MustCompile(`^[A-Za-z0-9_-]{4,32}$`)

// NormalizeServiceNo trims a hand-entered number and checks its shape.
func NormalizeServiceNo(raw string) (string, error) {
	no := strings.TrimSpace(raw)
	if !serviceNoRe.MatchString(no) {
		return "", invalid("service_no", "must be 4 to 32 letters, digits, '-' or '_'")
	}
	return no, nil
}

// serviceNoLocked: the number of a completed or cancelled service is final.
func serviceNoLocked(status string) bool {
	return status == StatusCompleted || status == StatusCancelled
}

// claimServiceNo checks that a hand-entered number is free for the service
// (exceptID 0: a new service).
func claimServiceNo(ctx context.Context, q *db.Queries, no string, exceptID int64) error {
	taken, err := q.ServiceNoTaken(ctx, db.ServiceNoTakenParams{ServiceNo: no, ExceptServiceID: exceptID})
	if err != nil {
		return fmt.Errorf("services: service number check: %w", err)
	}
	if taken {
		return ErrServiceNoTaken
	}
	return nil
}

// ServiceNoAvailability is GET /v1/services/service-no-availability.
type ServiceNoAvailability struct {
	ServiceNo string `json:"service_no"`
	Valid     bool   `json:"valid"`
	Available bool   `json:"available"`
}

// CheckServiceNo reports whether a number may be given to a new service or,
// with serviceID, to that service (its own number is available to it). The
// answer is a yes / no only: whose number it is stays hidden.
func (s *Service) CheckServiceNo(ctx context.Context, c Caller, raw string, serviceID *uuid.UUID) (ServiceNoAvailability, error) {
	if c.Org.InternalID == 0 || !c.Principal.HasPermission(rbac.PermServicesWrite) {
		return ServiceNoAvailability{}, ErrForbidden
	}
	no, err := NormalizeServiceNo(raw)
	if err != nil {
		return ServiceNoAvailability{ServiceNo: strings.TrimSpace(raw)}, nil
	}
	except := int64(0)
	if serviceID != nil {
		svc, err := s.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: *serviceID, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !visible(c, svc)) {
			return ServiceNoAvailability{}, ErrNotFound
		}
		if err != nil {
			return ServiceNoAvailability{}, fmt.Errorf("services: service: %w", err)
		}
		except = svc.ID
	}
	err = claimServiceNo(ctx, s.q, no, except)
	if err != nil && !errors.Is(err, ErrServiceNoTaken) {
		return ServiceNoAvailability{}, err
	}
	return ServiceNoAvailability{ServiceNo: no, Valid: true, Available: err == nil}, nil
}

// setServiceNo changes the number of a service the caller has locked and
// may write; the number of a completed or cancelled service is final.
func setServiceNo(ctx context.Context, q *db.Queries, svc db.Service, raw string, actor pgtype.Int8) (db.Service, error) {
	no, err := NormalizeServiceNo(raw)
	if err != nil {
		return svc, err
	}
	if no == svc.ServiceNo {
		return svc, nil
	}
	if serviceNoLocked(svc.Status) {
		return svc, ErrServiceNoLocked
	}
	if err := claimServiceNo(ctx, q, no, svc.ID); err != nil {
		return svc, err
	}
	row, err := q.UpdateServiceNo(ctx, db.UpdateServiceNoParams{
		ServiceNo: no, UpdatedByUserID: actor, ID: svc.ID, BrandID: svc.BrandID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return svc, ErrServiceNoLocked
	}
	if isUniqueViolation(err, "uq_services_service_no") {
		return svc, ErrServiceNoTaken
	}
	if err != nil {
		return svc, fmt.Errorf("services: service number: %w", err)
	}
	return row, nil
}
